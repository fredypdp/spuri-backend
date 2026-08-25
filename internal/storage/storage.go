package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	mega "github.com/t3rm1n4l/go-mega"
)

// AVISO: o gerenciamento de arquivos via Mega está validado e em produção.
// Não alterar dependências, provider ou fluxo de arquivos sem solicitação
// explícita de produto/arquitetura para essa área.
type StorageProvider interface {
	Upload(remotePath string, content io.Reader, sizeBytes int64) (StoredFile, error)
	Delete(remotePath string) error
	GetQuota() (QuotaInfo, error)
	EnsureDir(remotePath string) error
	List(remotePath string) ([]StoredFile, error)
	Read(remotePath string) (io.ReadCloser, error)
	Move(fromPath string, toPath string) error
	Rename(remotePath string, newName string) (StoredFile, error)
	ProviderName() string
}

type StoredFile struct {
	Path        string
	FileURL     string
	DownloadURL string
}

type QuotaInfo struct {
	TotalBytes            uint64
	UsedBytes             uint64
	AvailableBytes        uint64
	ManagedBytes          uint64
	UnmanagedBytes        uint64
	OutsideAcademiasBytes uint64
	Academias             []AcademiaUsage
	AccountFiles          []AccountFileUsage
	AccountFolders        []AccountFolderUsage
}

type AcademiaUsage struct {
	CodigoAcademia string
	UsedBytes      uint64
}

type AccountFileUsage struct {
	Path      string
	Name      string
	SizeBytes uint64
	Managed   bool
}

type AccountFolderUsage struct {
	Path      string
	Name      string
	SizeBytes uint64
	Managed   bool
}

var (
	ErrNotFound             = errors.New("arquivo ou pasta não encontrado")
	ErrInvalidPath          = errors.New("caminho remoto inválido")
	ErrInvalidConfiguration = errors.New("configuração de storage inválida")
	ErrOperationUnsupported = errors.New("operação de storage não suportada")
)

// MegaProvider implements StorageProvider for Mega. In production it delegates
// remote operations to the go-mega API client authenticated with MEGA_EMAIL and
// MEGA_PASSWORD. Tests and local development can use the filesystem-backed mode
// with STORAGE_PROVIDER=local, MEGA_LOCAL_ROOT, or ENV=test.
type MegaProvider struct {
	root       string
	rootFolder string
	local      bool
	authMu     sync.Mutex
	client     *mega.Mega
	remoteRoot *mega.Node
}

type tempFileReadCloser struct {
	*os.File
	name string
}

func (t *tempFileReadCloser) Close() error {
	err := t.File.Close()
	removeErr := os.Remove(t.name)
	if err != nil {
		return err
	}
	return removeErr
}

func NewStorageProvider() (StorageProvider, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER")))
	if provider == "" {
		provider = "mega"
	}
	switch provider {
	case "mega":
		return NewMegaProvider()
	case "local":
		return NewLocalProvider(), nil
	default:
		return nil, fmt.Errorf("%w: STORAGE_PROVIDER %q não suportado", ErrInvalidConfiguration, provider)
	}
}

func NewMegaProvider() (StorageProvider, error) {
	root := strings.TrimSpace(os.Getenv("MEGA_LOCAL_ROOT"))
	if root == "" {
		root = "data/mega_storage"
	}
	rootFolder := strings.Trim(strings.TrimSpace(os.Getenv("MEGA_ROOT_FOLDER")), "/")
	if useLocalMegaFallback() {
		return &MegaProvider{root: root, rootFolder: rootFolder, local: true}, nil
	}
	if strings.TrimSpace(os.Getenv("MEGA_EMAIL")) == "" || strings.TrimSpace(os.Getenv("MEGA_PASSWORD")) == "" {
		return nil, fmt.Errorf("%w: MEGA_EMAIL e MEGA_PASSWORD são obrigatórios quando STORAGE_PROVIDER=mega", ErrInvalidConfiguration)
	}
	provider := &MegaProvider{root: root, rootFolder: rootFolder}
	if err := provider.login(); err != nil {
		return nil, err
	}
	return provider, nil
}

func NewLocalProvider() StorageProvider {
	root := strings.TrimSpace(os.Getenv("MEGA_LOCAL_ROOT"))
	if root == "" {
		root = "data/mega_storage"
	}
	return &MegaProvider{root: root, rootFolder: strings.Trim(strings.TrimSpace(os.Getenv("MEGA_ROOT_FOLDER")), "/"), local: true}
}

func useLocalMegaFallback() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "test") || strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER"))) == "local"
}

func (m *MegaProvider) ProviderName() string { return "mega" }

func (m *MegaProvider) clean(remotePath string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(remotePath, "/")))
	if clean == "." {
		clean = ""
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return "", ErrInvalidPath
	}
	return clean, nil
}

func (m *MegaProvider) path(remotePath string) (string, error) {
	clean, err := m.clean(remotePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(m.root, filepath.FromSlash(clean)), nil
}

func (m *MegaProvider) login() error {
	m.authMu.Lock()
	defer m.authMu.Unlock()

	client := mega.New()
	client.SetTimeOut(15 * time.Second)
	client.SetRetries(2)
	if err := client.Login(os.Getenv("MEGA_EMAIL"), os.Getenv("MEGA_PASSWORD")); err != nil {
		return fmt.Errorf("falha ao autenticar no Mega: %w", sanitizeMegaError(err))
	}
	m.client = client
	m.remoteRoot = client.FS.GetRoot()
	if m.rootFolder != "" {
		node, err := m.ensureMegaDirLocked(m.rootFolder)
		if err != nil {
			return err
		}
		m.remoteRoot = node
	}
	return nil
}

func (m *MegaProvider) ensureAuthenticated() error {
	if m.local {
		return nil
	}
	m.authMu.Lock()
	hasClient := m.client != nil && m.remoteRoot != nil
	m.authMu.Unlock()
	if hasClient {
		return nil
	}
	return m.login()
}

func (m *MegaProvider) withMega(timeout time.Duration, fn func(*mega.Mega) error) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := m.ensureAuthenticated(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		done := make(chan error, 1)
		go func() {
			m.authMu.Lock()
			defer m.authMu.Unlock()
			done <- fn(m.client)
		}()
		select {
		case err := <-done:
			cancel()
			if err == nil {
				return nil
			}
			lastErr = sanitizeMegaError(err)
			if attempt == maxAttempts || !isTransientMegaError(err) {
				return lastErr
			}
			m.resetMegaSession()
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		case <-ctx.Done():
			cancel()
			return fmt.Errorf("operação Mega excedeu o tempo limite de %s: %w", timeout, ctx.Err())
		}
	}
	return lastErr
}

func (m *MegaProvider) resetMegaSession() {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	m.client = nil
	m.remoteRoot = nil
}

func isTransientMegaError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	transientFragments := []string{
		"unexpected end of json input",
		"unexpected eof",
		"eof",
		"connection reset",
		"connection refused",
		"temporary",
		"timeout",
		"i/o timeout",
	}
	for _, fragment := range transientFragments {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	return false
}

func sanitizeMegaError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, os.Getenv("MEGA_PASSWORD"), "[redacted]")
	msg = strings.ReplaceAll(msg, os.Getenv("MEGA_EMAIL"), "[redacted]")
	low := strings.ToLower(msg)
	switch {
	case errors.Is(err, mega.ENOENT) || strings.Contains(low, "not found") || strings.Contains(low, "enoent"):
		return fmt.Errorf("%w: %s", ErrNotFound, strings.TrimSpace(msg))
	case errors.Is(err, mega.EOVERQUOTA) || errors.Is(err, mega.EGOINGOVERQUOTA) || strings.Contains(low, "quota"):
		return fmt.Errorf("quota Mega excedida: %s", strings.TrimSpace(msg))
	case errors.Is(err, mega.EACCESS) || errors.Is(err, mega.ESID) || errors.Is(err, mega.EMFAREQUIRED) || strings.Contains(low, "access") || strings.Contains(low, "login"):
		return fmt.Errorf("falha de autenticação/permissão no Mega: %s", strings.TrimSpace(msg))
	default:
		return fmt.Errorf("erro Mega: %s", strings.TrimSpace(msg))
	}
}

func (m *MegaProvider) splitMegaPath(remotePath string) ([]string, error) {
	clean, err := m.clean(remotePath)
	if err != nil {
		return nil, err
	}
	if clean == "" {
		return nil, nil
	}
	return strings.Split(clean, "/"), nil
}

func (m *MegaProvider) lookupMegaNodeLocked(remotePath string) (*mega.Node, error) {
	parts, err := m.splitMegaPath(remotePath)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return m.remoteRoot, nil
	}
	nodes, err := m.client.FS.PathLookup(m.remoteRoot, parts)
	if err != nil {
		return nil, err
	}
	if len(nodes) != len(parts) {
		return nil, ErrNotFound
	}
	return nodes[len(nodes)-1], nil
}

func (m *MegaProvider) ensureMegaDirLocked(remotePath string) (*mega.Node, error) {
	parts, err := m.splitMegaPath(remotePath)
	if err != nil {
		return nil, err
	}
	parent := m.remoteRoot
	for i, part := range parts {
		nodes, err := m.client.FS.PathLookup(parent, []string{part})
		if err == nil && len(nodes) == 1 {
			parent = nodes[0]
			continue
		}
		parent, err = m.client.CreateDir(part, parent)
		if err != nil {
			return nil, fmt.Errorf("criar pasta Mega %q: %w", strings.Join(parts[:i+1], "/"), err)
		}
	}
	return parent, nil
}

func (m *MegaProvider) EnsureDir(remotePath string) error {
	if m.local {
		p, err := m.path(remotePath)
		if err != nil {
			return err
		}
		return os.MkdirAll(p, 0o700)
	}
	return m.withMega(30*time.Second, func(_ *mega.Mega) error {
		_, err := m.ensureMegaDirLocked(remotePath)
		return err
	})
}

func (m *MegaProvider) Upload(remotePath string, content io.Reader, sizeBytes int64) (StoredFile, error) {
	clean, err := m.clean(remotePath)
	if err != nil {
		return StoredFile{}, err
	}
	if m.local {
		p, err := m.path(clean)
		if err != nil {
			return StoredFile{}, err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return StoredFile{}, err
		}
		f, err := os.Create(p)
		if err != nil {
			return StoredFile{}, err
		}
		defer f.Close()
		if _, err = io.Copy(f, content); err != nil {
			return StoredFile{}, err
		}
		abs, _ := filepath.Abs(p)
		url := "file://" + filepath.ToSlash(abs)
		return StoredFile{Path: clean, FileURL: url, DownloadURL: url}, nil
	}
	tmp, err := os.CreateTemp("", "spuri-mega-upload-*")
	if err != nil {
		return StoredFile{}, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, content); err != nil {
		return StoredFile{}, err
	}
	dir, file := filepath.ToSlash(filepath.Dir(clean)), filepath.Base(clean)
	if dir == "." {
		dir = ""
	}
	if err := m.EnsureDir(dir); err != nil {
		return StoredFile{}, err
	}
	err = m.withMega(2*time.Minute, func(client *mega.Mega) error {
		parent, err := m.ensureMegaDirLocked(dir)
		if err != nil {
			return err
		}
		_, err = client.UploadFile(tmp.Name(), parent, file, nil)
		return err
	})
	if err != nil {
		return StoredFile{}, err
	}
	return StoredFile{Path: clean, FileURL: clean, DownloadURL: clean}, nil
}

func (m *MegaProvider) Delete(remotePath string) error {
	if m.local {
		p, err := m.path(remotePath)
		if err != nil {
			return err
		}
		if _, err := os.Stat(p); err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		return os.RemoveAll(p)
	}
	err := m.withMega(30*time.Second, func(client *mega.Mega) error {
		node, err := m.lookupMegaNodeLocked(remotePath)
		if err != nil {
			return err
		}
		return client.Delete(node, true)
	})
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	if err == nil {
		// go-mega removes only the deleted node from its in-memory lookup. When a
		// folder is deleted, its former parent can still retain a reference to it,
		// which makes a subsequent recursive quota walk fail with ENOENT. Discard
		// the cached filesystem so the next operation reloads the remote tree.
		m.resetMegaSession()
	}
	return err
}

func (m *MegaProvider) List(remotePath string) ([]StoredFile, error) {
	if m.local {
		return m.listLocal(remotePath)
	}
	base, _ := m.clean(remotePath)
	var files []StoredFile
	err := m.withMega(30*time.Second, func(client *mega.Mega) error {
		node, err := m.lookupMegaNodeLocked(remotePath)
		if err != nil {
			return err
		}
		children, err := client.FS.GetChildren(node)
		if err != nil {
			return err
		}
		for _, child := range children {
			path := strings.Trim(strings.Trim(base+"/"+child.GetName(), "/"), "/")
			files = append(files, StoredFile{Path: path, FileURL: path, DownloadURL: path})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func (m *MegaProvider) listLocal(remotePath string) ([]StoredFile, error) {
	p, err := m.path(remotePath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	base, _ := m.clean(remotePath)
	files := []StoredFile{}
	for _, e := range entries {
		path := strings.Trim(strings.Trim(base+"/"+e.Name(), "/"), "/")
		files = append(files, StoredFile{Path: path, FileURL: "file://" + filepath.ToSlash(filepath.Join(p, e.Name())), DownloadURL: "file://" + filepath.ToSlash(filepath.Join(p, e.Name()))})
	}
	return files, nil
}

func (m *MegaProvider) Read(remotePath string) (io.ReadCloser, error) {
	if m.local {
		p, err := m.path(remotePath)
		if err != nil {
			return nil, err
		}
		return os.Open(p)
	}
	tmp, err := os.CreateTemp("", "spuri-mega-download-*")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	err = m.withMega(2*time.Minute, func(client *mega.Mega) error {
		node, err := m.lookupMegaNodeLocked(remotePath)
		if err != nil {
			return err
		}
		return client.DownloadFile(node, tmp.Name(), nil)
	})
	if err != nil {
		os.Remove(tmp.Name())
		return nil, err
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		os.Remove(tmp.Name())
		return nil, err
	}
	return &tempFileReadCloser{File: f, name: tmp.Name()}, nil
}

func (m *MegaProvider) Move(fromPath string, toPath string) error {
	if m.local {
		from, err := m.path(fromPath)
		if err != nil {
			return err
		}
		to, err := m.path(toPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		return os.Rename(from, to)
	}
	dir := filepath.ToSlash(filepath.Dir(strings.TrimPrefix(toPath, "/")))
	if dir == "." {
		dir = ""
	}
	if err := m.EnsureDir(dir); err != nil {
		return err
	}
	return m.withMega(30*time.Second, func(client *mega.Mega) error {
		from, err := m.lookupMegaNodeLocked(fromPath)
		if err != nil {
			return err
		}
		parent, err := m.ensureMegaDirLocked(dir)
		if err != nil {
			return err
		}
		if err := client.Move(from, parent); err != nil {
			return err
		}
		newName := filepath.Base(toPath)
		if newName != from.GetName() {
			return client.Rename(from, newName)
		}
		return nil
	})
}

func (m *MegaProvider) Rename(remotePath string, newName string) (StoredFile, error) {
	if strings.TrimSpace(newName) == "" || strings.Contains(newName, "/") {
		return StoredFile{}, ErrInvalidPath
	}
	clean, err := m.clean(remotePath)
	if err != nil {
		return StoredFile{}, err
	}
	dir := filepath.ToSlash(filepath.Dir(clean))
	if dir == "." {
		dir = ""
	}
	to := strings.Trim(strings.Trim(dir+"/"+newName, "/"), "/")
	if err := m.Move(clean, to); err != nil {
		return StoredFile{}, err
	}
	return StoredFile{Path: to, FileURL: to, DownloadURL: to}, nil
}

func (m *MegaProvider) GetQuota() (QuotaInfo, error) {
	if m.local {
		return m.getLocalQuota()
	}
	var quotaInfo QuotaInfo
	err := m.withMega(30*time.Second, func(client *mega.Mega) error {
		remoteQuota, err := client.GetQuota()
		if err != nil {
			return err
		}
		quotaInfo.TotalBytes = remoteQuota.Mstrg
		quotaInfo.UsedBytes = remoteQuota.Cstrg
		if remoteQuota.Mstrg > remoteQuota.Cstrg {
			quotaInfo.AvailableBytes = remoteQuota.Mstrg - remoteQuota.Cstrg
		}

		managedByAcademia := map[string]uint64{}
		files := []AccountFileUsage{}
		folders := []AccountFolderUsage{}
		accounted, outside, err := m.collectRemoteUsageLocked(client, m.remoteRoot, "", managedByAcademia, &files, &folders)
		if err != nil {
			return err
		}
		quotaInfo.ManagedBytes = accounted - outside
		quotaInfo.OutsideAcademiasBytes = outside
		if quotaInfo.UsedBytes > accounted {
			quotaInfo.UnmanagedBytes = quotaInfo.UsedBytes - accounted
		}
		quotaInfo.Academias = sortedAcademiaUsage(managedByAcademia)
		quotaInfo.AccountFiles = files
		quotaInfo.AccountFolders = folders
		return nil
	})
	if err != nil {
		return QuotaInfo{}, err
	}
	sort.Slice(quotaInfo.AccountFiles, func(i, j int) bool { return quotaInfo.AccountFiles[i].Path < quotaInfo.AccountFiles[j].Path })
	sort.Slice(quotaInfo.AccountFolders, func(i, j int) bool { return quotaInfo.AccountFolders[i].Path < quotaInfo.AccountFolders[j].Path })
	return quotaInfo, nil
}

func (m *MegaProvider) collectRemoteUsageLocked(client *mega.Mega, node *mega.Node, basePath string, academias map[string]uint64, files *[]AccountFileUsage, folders *[]AccountFolderUsage) (uint64, uint64, error) {
	children, err := client.FS.GetChildren(node)
	if err != nil {
		return 0, 0, err
	}
	var used, outside uint64
	for _, child := range children {
		name := child.GetName()
		path := strings.Trim(strings.Trim(basePath+"/"+name, "/"), "/")
		switch child.GetType() {
		case mega.FILE:
			size := uint64(0)
			if child.GetSize() > 0 {
				size = uint64(child.GetSize())
			}
			used += size
			fileOutside := accountManagedUsage(path, size, academias)
			outside += fileOutside
			*files = append(*files, AccountFileUsage{Path: path, Name: name, SizeBytes: size, Managed: fileOutside == 0})
		case mega.FOLDER, mega.ROOT, mega.INBOX:
			childUsed, childOutside, err := m.collectRemoteUsageLocked(client, child, path, academias, files, folders)
			if err != nil {
				return 0, 0, err
			}
			used += childUsed
			outside += childOutside
			if path != "" {
				*folders = append(*folders, AccountFolderUsage{Path: path, Name: name, SizeBytes: childUsed, Managed: childOutside == 0})
			}
		}
	}
	return used, outside, nil
}

func (m *MegaProvider) getLocalQuota() (QuotaInfo, error) {
	var used, outside uint64
	academias := map[string]uint64{}
	folderSizes := map[string]uint64{}
	files := []AccountFileUsage{}
	_ = filepath.WalkDir(m.root, func(path string, de os.DirEntry, err error) error {
		if err == nil && !de.IsDir() {
			if info, e := de.Info(); e == nil {
				size := uint64(info.Size())
				used += size
				rel, _ := filepath.Rel(m.root, path)
				rp := filepath.ToSlash(rel)
				fileOutside := accountManagedUsage(rp, size, academias)
				outside += fileOutside
				addFolderUsage(rp, size, folderSizes)
				files = append(files, AccountFileUsage{Path: rp, Name: filepath.Base(path), SizeBytes: size, Managed: fileOutside == 0})
			}
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	folders := sortedFolderUsage(folderSizes)
	return QuotaInfo{TotalBytes: used, UsedBytes: used, ManagedBytes: used - outside, OutsideAcademiasBytes: outside, Academias: sortedAcademiaUsage(academias), AccountFiles: files, AccountFolders: folders}, nil
}

func addFolderUsage(remotePath string, size uint64, folders map[string]uint64) {
	dir := filepath.ToSlash(filepath.Dir(remotePath))
	for dir != "." && dir != "/" && dir != "" {
		folders[dir] += size
		next := filepath.ToSlash(filepath.Dir(dir))
		if next == dir {
			break
		}
		dir = next
	}
}

func sortedFolderUsage(usage map[string]uint64) []AccountFolderUsage {
	out := make([]AccountFolderUsage, 0, len(usage))
	for path, used := range usage {
		outside := accountManagedUsage(path+"/.folder", 0, map[string]uint64{})
		out = append(out, AccountFolderUsage{Path: path, Name: filepath.Base(path), SizeBytes: used, Managed: outside == 0})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func accountManagedUsage(remotePath string, size uint64, academias map[string]uint64) uint64 {
	parts := strings.Split(remotePath, "/")
	if len(parts) > 1 && parts[0] != "." && strings.TrimSpace(parts[0]) != "" {
		academias[parts[0]] += size
		return 0
	}
	return size
}

func sortedAcademiaUsage(usage map[string]uint64) []AcademiaUsage {
	out := make([]AcademiaUsage, 0, len(usage))
	for codigo, used := range usage {
		out = append(out, AcademiaUsage{CodigoAcademia: codigo, UsedBytes: used})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CodigoAcademia < out[j].CodigoAcademia })
	return out
}

func HumanBytes(v uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f := float64(v)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", v, units[i])
	}
	return fmt.Sprintf("%.2f %s", f, units[i])
}
