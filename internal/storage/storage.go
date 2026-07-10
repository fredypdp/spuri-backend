package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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

var (
	ErrNotFound             = errors.New("arquivo ou pasta não encontrado")
	ErrInvalidPath          = errors.New("caminho remoto inválido")
	ErrInvalidConfiguration = errors.New("configuração de storage inválida")
	ErrOperationUnsupported = errors.New("operação de storage não suportada")
)

// MegaProvider implements StorageProvider for Mega. In production it delegates
// remote operations to MEGAcmd (mega-login/mega-put/mega-get/etc.) authenticated
// with MEGA_EMAIL and MEGA_PASSWORD. Tests and local development can use the
// filesystem-backed mode with STORAGE_PROVIDER=local, MEGA_LOCAL_ROOT, or ENV=test.
type MegaProvider struct {
	root       string
	rootFolder string
	local      bool
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
	if _, err := exec.LookPath("mega-login"); err != nil {
		return nil, fmt.Errorf("%w: MEGAcmd não encontrado no PATH", ErrInvalidConfiguration)
	}
	p := &MegaProvider{root: root, rootFolder: rootFolder}
	if err := p.login(); err != nil {
		return nil, err
	}
	if rootFolder != "" {
		if err := p.EnsureDir(""); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func NewLocalProvider() StorageProvider {
	root := strings.TrimSpace(os.Getenv("MEGA_LOCAL_ROOT"))
	if root == "" {
		root = "data/mega_storage"
	}
	return &MegaProvider{root: root, rootFolder: strings.Trim(strings.TrimSpace(os.Getenv("MEGA_ROOT_FOLDER")), "/"), local: true}
}

func useLocalMegaFallback() bool {
	return os.Getenv("ENV") == "test" || strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER"))) == "local"
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

func (m *MegaProvider) megaPath(remotePath string) (string, error) {
	clean, err := m.clean(remotePath)
	if err != nil {
		return "", err
	}
	parts := []string{}
	if m.rootFolder != "" {
		parts = append(parts, m.rootFolder)
	}
	if clean != "" {
		parts = append(parts, clean)
	}
	return "/" + strings.Join(parts, "/"), nil
}

func (m *MegaProvider) login() error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "mega-login", os.Getenv("MEGA_EMAIL"), os.Getenv("MEGA_PASSWORD"))
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "already logged") {
		return fmt.Errorf("falha ao autenticar no Mega: %w", sanitizeMegaError(out, err))
	}
	return nil
}

func (m *MegaProvider) runMega(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return out, sanitizeMegaError(out, err)
	}
	return out, nil
}

func sanitizeMegaError(out []byte, err error) error {
	msg := string(out)
	msg = strings.ReplaceAll(msg, os.Getenv("MEGA_PASSWORD"), "[redacted]")
	msg = strings.ReplaceAll(msg, os.Getenv("MEGA_EMAIL"), "[redacted]")
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "not found") || strings.Contains(low, "enoent"):
		return fmt.Errorf("%w: %s", ErrNotFound, strings.TrimSpace(msg))
	case strings.Contains(low, "quota"):
		return fmt.Errorf("quota Mega excedida: %s", strings.TrimSpace(msg))
	case strings.Contains(low, "access") || strings.Contains(low, "permission") || strings.Contains(low, "login"):
		return fmt.Errorf("falha de autenticação/permissão no Mega: %s", strings.TrimSpace(msg))
	default:
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(msg))
	}
}

func (m *MegaProvider) EnsureDir(remotePath string) error {
	if m.local {
		p, err := m.path(remotePath)
		if err != nil {
			return err
		}
		return os.MkdirAll(p, 0o700)
	}
	p, err := m.megaPath(remotePath)
	if err != nil {
		return err
	}
	_, err = m.runMega(30*time.Second, "mega-mkdir", "-p", p)
	return err
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
	dest, err := m.megaPath(dir)
	if err != nil {
		return StoredFile{}, err
	}
	_, err = m.runMega(2*time.Minute, "mega-put", "-c", tmp.Name(), dest+"/"+file)
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
	p, err := m.megaPath(remotePath)
	if err != nil {
		return err
	}
	_, err = m.runMega(30*time.Second, "mega-rm", "-r", p)
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func (m *MegaProvider) List(remotePath string) ([]StoredFile, error) {
	if m.local {
		return m.listLocal(remotePath)
	}
	p, err := m.megaPath(remotePath)
	if err != nil {
		return nil, err
	}
	out, err := m.runMega(30*time.Second, "mega-ls", p)
	if err != nil {
		return nil, err
	}
	base, _ := m.clean(remotePath)
	var files []StoredFile
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		path := strings.Trim(strings.Trim(base+"/"+name, "/"), "/")
		files = append(files, StoredFile{Path: path, FileURL: path, DownloadURL: path})
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
	p, err := m.megaPath(remotePath)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "spuri-mega-download-*")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	_, err = m.runMega(2*time.Minute, "mega-get", p, tmp.Name())
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
	from, err := m.megaPath(fromPath)
	if err != nil {
		return err
	}
	to, err := m.megaPath(toPath)
	if err != nil {
		return err
	}
	_, err = m.runMega(30*time.Second, "mega-mv", from, to)
	return err
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
	academias, files, managed, outside, err := m.getMegaAccountUsage()
	if err != nil {
		return QuotaInfo{}, err
	}
	total := managed + outside
	return QuotaInfo{TotalBytes: total, UsedBytes: total, ManagedBytes: managed, OutsideAcademiasBytes: outside, Academias: academias, AccountFiles: files}, nil
}

func (m *MegaProvider) getMegaAccountUsage() ([]AcademiaUsage, []AccountFileUsage, uint64, uint64, error) {
	return nil, nil, 0, 0, ErrOperationUnsupported
}

func (m *MegaProvider) getLocalQuota() (QuotaInfo, error) {
	var used, outside uint64
	academias := map[string]uint64{}
	files := []AccountFileUsage{}
	_ = filepath.WalkDir(m.root, func(path string, de os.DirEntry, err error) error {
		if err == nil && !de.IsDir() {
			if info, e := de.Info(); e == nil {
				size := uint64(info.Size())
				used += size
				rel, _ := filepath.Rel(m.root, path)
				rp := filepath.ToSlash(rel)
				parts := strings.Split(rp, "/")
				managed := len(parts) > 1 && parts[0] != "."
				if managed {
					academias[parts[0]] += size
				} else {
					outside += size
				}
				files = append(files, AccountFileUsage{Path: rp, Name: filepath.Base(path), SizeBytes: size, Managed: managed})
			}
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return QuotaInfo{TotalBytes: used, UsedBytes: used, ManagedBytes: used - outside, OutsideAcademiasBytes: outside, Academias: sortedAcademiaUsage(academias), AccountFiles: files}, nil
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
