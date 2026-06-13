package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type StorageProvider interface {
	Upload(remotePath string, content io.Reader, sizeBytes int64) error
	Delete(remotePath string) error
	GetQuota() (QuotaInfo, error)
	EnsureDir(remotePath string) error
}

type QuotaInfo struct {
	TotalBytes     uint64
	UsedBytes      uint64
	AvailableBytes uint64
	ManagedBytes   uint64
	UnmanagedBytes uint64
	Academias      []AcademiaUsage
	AccountFiles   []AccountFileUsage
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

// DriveProvider implements StorageProvider for Google Drive.
//
// In production it uses the Google Drive API with GOOGLE_DRIVE_ACCESS_TOKEN.
// Local/test environments may opt into a filesystem-backed estimate by setting
// GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true; files are stored under
// GOOGLE_DRIVE_LOCAL_ROOT (default data/google_drive_storage).
type DriveProvider struct {
	root         string
	rootFolderID string
	service      *driveRESTClient
}

func NewDriveProvider() (StorageProvider, error) {
	root := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_LOCAL_ROOT"))
	if root == "" {
		root = "data/google_drive_storage"
	}
	rootFolderID := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_ROOT_FOLDER_ID"))

	if useLocalDriveFallback() {
		return &DriveProvider{root: root, rootFolderID: rootFolderID}, nil
	}
	if rootFolderID == "" {
		return nil, fmt.Errorf("configuração Google Drive incompleta: GOOGLE_DRIVE_ROOT_FOLDER_ID é obrigatório")
	}
	if strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_ACCESS_TOKEN")) == "" {
		return nil, fmt.Errorf("configuração Google Drive incompleta: GOOGLE_DRIVE_ACCESS_TOKEN é obrigatório")
	}
	return &DriveProvider{root: root, rootFolderID: rootFolderID, service: &driveRESTClient{client: &http.Client{Timeout: 2 * time.Minute}, token: strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_ACCESS_TOKEN"))}}, nil
}

func useLocalDriveFallback() bool {
	if strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE")) == "true" {
		return true
	}
	return os.Getenv("ENV") == "test"
}

func (d *DriveProvider) isLocal() bool { return d.service == nil }

func (d *DriveProvider) path(remotePath string) (string, error) {
	clean := filepath.Clean(strings.TrimPrefix(remotePath, "/"))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("caminho remoto inválido")
	}
	return filepath.Join(d.root, clean), nil
}

func (d *DriveProvider) EnsureDir(remotePath string) error {
	if d.isLocal() {
		p, err := d.path(remotePath)
		if err != nil {
			return err
		}
		return os.MkdirAll(p, 0o700)
	}
	_, err := d.ensureDriveFolderPath(remotePath)
	return err
}

func (d *DriveProvider) Upload(remotePath string, content io.Reader, sizeBytes int64) error {
	if d.isLocal() {
		p, err := d.path(remotePath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		f, err := os.Create(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, content)
		return err
	}
	parent, name, err := d.driveParentAndName(remotePath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	existing, err := d.findDriveChild(ctx, parent, name, false)
	if err != nil {
		return err
	}
	if _, err := d.service.uploadFile(ctx, parent, name, "application/pdf", content); err != nil {
		return fmt.Errorf("falha no upload para Google Drive: %w", err)
	}
	if existing != nil {
		_ = d.service.deleteFile(ctx, existing.ID)
	}
	return nil
}

func (d *DriveProvider) Delete(remotePath string) error {
	if d.isLocal() {
		p, err := d.path(remotePath)
		if err != nil {
			return err
		}
		return os.RemoveAll(p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, err := d.resolveDrivePath(ctx, remotePath)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	return d.service.deleteFile(ctx, id)
}

func (d *DriveProvider) GetQuota() (QuotaInfo, error) {
	if d.isLocal() {
		if strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE")) != "true" && os.Getenv("ENV") != "test" {
			return QuotaInfo{}, fmt.Errorf("quota do Google Drive indisponível: configure credenciais do Google Drive e GOOGLE_DRIVE_ROOT_FOLDER_ID; para ambiente local, defina GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true para estimar apenas os arquivos em %q", d.root)
		}
		return d.getLocalQuota()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	about, err := d.service.getAbout(ctx)
	if err != nil {
		return QuotaInfo{}, fmt.Errorf("falha ao consultar quota do Google Drive: %w", err)
	}
	academias, accountFiles, managed, err := d.getDriveAccountUsage(ctx)
	if err != nil {
		return QuotaInfo{}, err
	}
	total := about.StorageQuota.Limit
	used := about.StorageQuota.Usage
	available := uint64(0)
	if total > used {
		available = total - used
	}
	unmanaged := uint64(0)
	if used > managed {
		unmanaged = used - managed
	}
	return QuotaInfo{TotalBytes: total, UsedBytes: used, AvailableBytes: available, ManagedBytes: managed, UnmanagedBytes: unmanaged, Academias: academias, AccountFiles: accountFiles}, nil
}

func (d *DriveProvider) getLocalQuota() (QuotaInfo, error) {
	var used uint64
	academias := map[string]uint64{}
	accountFiles := []AccountFileUsage{}
	_ = filepath.WalkDir(d.root, func(path string, de os.DirEntry, err error) error {
		if err == nil && !de.IsDir() {
			if info, e := de.Info(); e == nil {
				size := uint64(info.Size())
				used += size
				rel, relErr := filepath.Rel(d.root, path)
				if relErr == nil {
					path := filepath.ToSlash(rel)
					parts := strings.Split(path, "/")
					managed := len(parts) > 1 && parts[0] != "."
					if managed {
						academias[parts[0]] += size
					}
					accountFiles = append(accountFiles, AccountFileUsage{Path: path, Name: filepath.Base(path), SizeBytes: size, Managed: managed})
				}
			}
		}
		return nil
	})
	total, err := configuredQuotaTotalBytes()
	if err != nil {
		return QuotaInfo{}, err
	}
	avail := uint64(0)
	if total > used {
		avail = total - used
	}
	sort.Slice(accountFiles, func(i, j int) bool { return accountFiles[i].Path < accountFiles[j].Path })
	return QuotaInfo{TotalBytes: total, UsedBytes: used, AvailableBytes: avail, ManagedBytes: used, Academias: sortedAcademiaUsage(academias), AccountFiles: accountFiles}, nil
}

func configuredQuotaTotalBytes() (uint64, error) {
	if raw := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_QUOTA_TOTAL_BYTES")); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			return 0, fmt.Errorf("configuração Google Drive inválida: GOOGLE_DRIVE_QUOTA_TOTAL_BYTES=%q deve ser um inteiro positivo em bytes", raw)
		}
		return v, nil
	}
	if raw := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_QUOTA_TOTAL_GB")); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			return 0, fmt.Errorf("configuração Google Drive inválida: GOOGLE_DRIVE_QUOTA_TOTAL_GB=%q deve ser um inteiro positivo em GB", raw)
		}
		return v * 1024 * 1024 * 1024, nil
	}
	const defaultDriveQuota uint64 = 15 * 1024 * 1024 * 1024
	return defaultDriveQuota, nil
}

func (d *DriveProvider) driveParentAndName(remotePath string) (string, string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(remotePath, "/")))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", "", fmt.Errorf("caminho remoto inválido")
	}
	parentPath, name := filepath.ToSlash(filepath.Dir(clean)), filepath.Base(clean)
	if parentPath == "." {
		return d.rootFolderID, name, nil
	}
	parent, err := d.ensureDriveFolderPath(parentPath)
	return parent, name, err
}

func (d *DriveProvider) ensureDriveFolderPath(remotePath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	parent := d.rootFolderID
	clean := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(remotePath, "/")))
	if clean == "." || clean == "" {
		return parent, nil
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("caminho remoto inválido")
		}
		folder, err := d.findDriveChild(ctx, parent, part, true)
		if err != nil {
			return "", err
		}
		if folder == nil {
			folder, err = d.service.createFolder(ctx, parent, part)
			if err != nil {
				return "", fmt.Errorf("falha ao criar diretório no Google Drive: %w", err)
			}
		}
		parent = folder.ID
	}
	return parent, nil
}

func (d *DriveProvider) findDriveChild(ctx context.Context, parentID, name string, folder bool) (*driveFile, error) {
	mimeOp := "!="
	if folder {
		mimeOp = "="
	}
	q := fmt.Sprintf("%s in parents and name = %s and mimeType %s 'application/vnd.google-apps.folder' and trashed = false", quoteDriveQueryString(parentID), quoteDriveQueryString(name), mimeOp)
	resp, err := d.service.listFiles(ctx, q, 1, "")
	if err != nil {
		return nil, err
	}
	if len(resp.Files) == 0 {
		return nil, nil
	}
	return &resp.Files[0], nil
}

func (d *DriveProvider) resolveDrivePath(ctx context.Context, remotePath string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(remotePath, "/")))
	if clean == "." || clean == "" {
		return d.rootFolderID, nil
	}
	parent := d.rootFolderID
	parts := strings.Split(clean, "/")
	for i, part := range parts {
		folder := i < len(parts)-1
		child, err := d.findDriveChild(ctx, parent, part, folder)
		if err != nil || child == nil {
			return "", err
		}
		parent = child.ID
	}
	return parent, nil
}

func (d *DriveProvider) getDriveAccountUsage(ctx context.Context) ([]AcademiaUsage, []AccountFileUsage, uint64, error) {
	usage := map[string]uint64{}
	files := []AccountFileUsage{}
	parents := map[string]string{d.rootFolderID: ""}
	queue := []string{d.rootFolderID}
	var managed uint64
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		pageToken := ""
		for {
			q := fmt.Sprintf("%s in parents and trashed = false", quoteDriveQueryString(parent))
			resp, err := d.service.listFiles(ctx, q, 1000, pageToken)
			if err != nil {
				return nil, nil, 0, fmt.Errorf("falha ao listar arquivos do Google Drive: %w", err)
			}
			for _, f := range resp.Files {
				base := parents[parent]
				path := f.Name
				if base != "" {
					path = base + "/" + f.Name
				}
				if f.MimeType == "application/vnd.google-apps.folder" {
					parents[f.ID] = path
					queue = append(queue, f.ID)
					continue
				}
				size := f.Size
				parts := strings.Split(path, "/")
				isManaged := len(parts) > 1 && parts[0] != ""
				if isManaged {
					usage[parts[0]] += size
					managed += size
				}
				files = append(files, AccountFileUsage{Path: path, Name: f.Name, SizeBytes: size, Managed: isManaged})
			}
			if resp.NextPageToken == "" {
				break
			}
			pageToken = resp.NextPageToken
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return sortedAcademiaUsage(usage), files, managed, nil
}

type driveRESTClient struct {
	client *http.Client
	token  string
}

type driveFile struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	MimeType string   `json:"mimeType"`
	Size     uint64   `json:"size,string"`
	Parents  []string `json:"parents,omitempty"`
}

type driveListResponse struct {
	NextPageToken string      `json:"nextPageToken"`
	Files         []driveFile `json:"files"`
}

type driveAboutResponse struct {
	StorageQuota struct {
		Limit uint64 `json:"limit,string"`
		Usage uint64 `json:"usage,string"`
	} `json:"storageQuota"`
}

func (c *driveRESTClient) do(ctx context.Context, req *http.Request, dest any) error {
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Google Drive retornou HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if dest == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func (c *driveRESTClient) getAbout(ctx context.Context) (*driveAboutResponse, error) {
	req, err := http.NewRequest(http.MethodGet, "https://www.googleapis.com/drive/v3/about?fields=storageQuota", nil)
	if err != nil {
		return nil, err
	}
	var out driveAboutResponse
	if err := c.do(ctx, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *driveRESTClient) listFiles(ctx context.Context, query string, pageSize int, pageToken string) (*driveListResponse, error) {
	req, err := http.NewRequest(http.MethodGet, "https://www.googleapis.com/drive/v3/files", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("q", query)
	q.Set("fields", "nextPageToken,files(id,name,mimeType,size,parents)")
	q.Set("pageSize", strconv.Itoa(pageSize))
	q.Set("supportsAllDrives", "true")
	q.Set("includeItemsFromAllDrives", "true")
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	req.URL.RawQuery = q.Encode()
	var out driveListResponse
	if err := c.do(ctx, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *driveRESTClient) createFolder(ctx context.Context, parentID, name string) (*driveFile, error) {
	body, err := json.Marshal(map[string]any{
		"name":     name,
		"mimeType": "application/vnd.google-apps.folder",
		"parents":  []string{parentID},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, "https://www.googleapis.com/drive/v3/files?fields=id,name,mimeType", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var out driveFile
	if err := c.do(ctx, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *driveRESTClient) uploadFile(ctx context.Context, parentID, name, mimeType string, content io.Reader) (*driveFile, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	meta, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {"application/json; charset=UTF-8"}})
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(meta).Encode(map[string]any{"name": name, "parents": []string{parentID}, "mimeType": mimeType}); err != nil {
		return nil, err
	}
	filePart, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {mimeType}})
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(filePart, content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&fields=id,name,mimeType,size", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "multipart/related; boundary="+writer.Boundary())
	var out driveFile
	if err := c.do(ctx, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *driveRESTClient) deleteFile(ctx context.Context, id string) error {
	req, err := http.NewRequest(http.MethodDelete, "https://www.googleapis.com/drive/v3/files/"+id+"?supportsAllDrives=true", nil)
	if err != nil {
		return err
	}
	return c.do(ctx, req, nil)
}

func quoteDriveQueryString(v string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `'`, `\'`) + "'"
}

func sortedAcademiaUsage(usage map[string]uint64) []AcademiaUsage {
	out := make([]AcademiaUsage, 0, len(usage))
	for codigo, used := range usage {
		out = append(out, AcademiaUsage{CodigoAcademia: codigo, UsedBytes: used})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CodigoAcademia < out[j].CodigoAcademia
	})
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
