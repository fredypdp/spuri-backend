package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type StorageProvider interface {
	Upload(remotePath string, content io.Reader, sizeBytes int64) error
	Delete(remotePath string) error
	GetQuota() (QuotaInfo, error)
	EnsureDir(remotePath string) error
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

// DriveProvider implements StorageProvider for Google Drive.
//
// In production it uses the official Google Drive API client authenticated with
// service account credentials. Local/test environments may opt into a
// filesystem-backed estimate by setting GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true;
// files are stored under GOOGLE_DRIVE_LOCAL_ROOT (default data/google_drive_storage).
type DriveProvider struct {
	root         string
	rootFolderID string
	service      *drive.Service
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

	credBytes, err := googleDriveCredentialBytes()
	if err != nil {
		return nil, err
	}
	if !isGoogleDriveServiceAccountJSON(credBytes) {
		return nil, fmt.Errorf("credencial Google Drive inválida: JSON malformado ou não é uma service account")
	}
	if rootFolderID == "" {
		return nil, fmt.Errorf("configuração Google Drive incompleta: GOOGLE_DRIVE_ROOT_FOLDER_ID é obrigatório")
	}

	ctx := context.Background()
	creds, err := google.CredentialsFromJSON(ctx, credBytes, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("credencial Google Drive inválida: JSON malformado ou não é uma service account")
	}
	svc, err := drive.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("falha ao criar Drive client: %w", err)
	}

	return &DriveProvider{root: root, rootFolderID: rootFolderID, service: svc}, nil
}

func googleDriveCredentialBytes() ([]byte, error) {
	if path := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CREDENTIALS_PATH")); path != "" {
		credBytes, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("falha ao ler credencial Google Drive: %w", err)
		}
		return credBytes, nil
	}
	if b64 := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CREDENTIALS_JSON")); b64 != "" {
		credBytes, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("credencial Google Drive inválida: JSON malformado ou não é uma service account")
		}
		return credBytes, nil
	}
	return nil, fmt.Errorf("configuração Google Drive incompleta: nenhuma credencial configurada (defina GOOGLE_DRIVE_CREDENTIALS_PATH ou GOOGLE_DRIVE_CREDENTIALS_JSON)")
}

func isGoogleDriveServiceAccountJSON(credBytes []byte) bool {
	var payload struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(credBytes, &payload) == nil && payload.Type == "service_account"
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
	if _, err := d.service.Files.Create(&drive.File{Name: name, Parents: []string{parent}}).Media(content).Fields("id").SupportsAllDrives(true).Context(ctx).Do(); err != nil {
		return fmt.Errorf("falha no upload para Google Drive: %w", err)
	}
	if existing != nil {
		_ = d.service.Files.Delete(existing.Id).SupportsAllDrives(true).Context(ctx).Do()
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
	return d.service.Files.Delete(id).SupportsAllDrives(true).Context(ctx).Do()
}

func (d *DriveProvider) GetQuota() (QuotaInfo, error) {
	if d.isLocal() {
		if strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE")) != "true" && os.Getenv("ENV") != "test" {
			return QuotaInfo{}, fmt.Errorf("quota do Google Drive indisponível: configure credenciais e GOOGLE_DRIVE_ROOT_FOLDER_ID; para ambiente local, defina GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true")
		}
		return d.getLocalQuota()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	academias, accountFiles, managed, outsideAcademias, err := d.getDriveAccountUsage(ctx)
	if err != nil {
		return QuotaInfo{}, err
	}
	total := managed + outsideAcademias
	return QuotaInfo{TotalBytes: total, UsedBytes: total, ManagedBytes: managed, OutsideAcademiasBytes: outsideAcademias, Academias: academias, AccountFiles: accountFiles}, nil
}

func (d *DriveProvider) getLocalQuota() (QuotaInfo, error) {
	var used uint64
	var outsideAcademias uint64
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
					} else {
						outsideAcademias += size
					}
					accountFiles = append(accountFiles, AccountFileUsage{Path: path, Name: filepath.Base(path), SizeBytes: size, Managed: managed})
				}
			}
		}
		return nil
	})
	sort.Slice(accountFiles, func(i, j int) bool { return accountFiles[i].Path < accountFiles[j].Path })
	return QuotaInfo{TotalBytes: used, UsedBytes: used, ManagedBytes: used - outsideAcademias, OutsideAcademiasBytes: outsideAcademias, Academias: sortedAcademiaUsage(academias), AccountFiles: accountFiles}, nil
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
			folder, err = d.service.Files.Create(&drive.File{Name: part, MimeType: "application/vnd.google-apps.folder", Parents: []string{parent}}).Fields("id,name,mimeType").SupportsAllDrives(true).Context(ctx).Do()
			if err != nil {
				return "", fmt.Errorf("falha ao criar diretório no Google Drive: %w", err)
			}
		}
		parent = folder.Id
	}
	return parent, nil
}

func (d *DriveProvider) findDriveChild(ctx context.Context, parentID, name string, folder bool) (*drive.File, error) {
	mimeOp := "!="
	if folder {
		mimeOp = "="
	}
	q := fmt.Sprintf("%s in parents and name = %s and mimeType %s 'application/vnd.google-apps.folder' and trashed = false", quoteDriveQueryString(parentID), quoteDriveQueryString(name), mimeOp)
	resp, err := d.service.Files.List().Q(q).PageSize(1).Fields("files(id,name,mimeType,size,parents)").SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if len(resp.Files) == 0 {
		return nil, nil
	}
	return resp.Files[0], nil
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
		parent = child.Id
	}
	return parent, nil
}

func (d *DriveProvider) getDriveAccountUsage(ctx context.Context) ([]AcademiaUsage, []AccountFileUsage, uint64, uint64, error) {
	usage := map[string]uint64{}
	files := []AccountFileUsage{}
	parents := map[string]string{d.rootFolderID: ""}
	queue := []string{d.rootFolderID}
	var managed uint64
	var outsideAcademias uint64
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		pageToken := ""
		for {
			q := fmt.Sprintf("%s in parents and trashed = false", quoteDriveQueryString(parent))
			resp, err := d.service.Files.List().Q(q).PageSize(1000).PageToken(pageToken).Fields("nextPageToken,files(id,name,mimeType,size,parents)").SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Context(ctx).Do()
			if err != nil {
				return nil, nil, 0, 0, fmt.Errorf("falha ao listar arquivos do Google Drive: %w", err)
			}
			for _, f := range resp.Files {
				base := parents[parent]
				path := f.Name
				if base != "" {
					path = base + "/" + f.Name
				}
				if f.MimeType == "application/vnd.google-apps.folder" {
					parents[f.Id] = path
					queue = append(queue, f.Id)
					continue
				}
				size := uint64FromDriveInt64(f.Size)
				parts := strings.Split(path, "/")
				isManaged := len(parts) > 1 && parts[0] != ""
				if isManaged {
					usage[parts[0]] += size
					managed += size
				} else {
					outsideAcademias += size
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
	return sortedAcademiaUsage(usage), files, managed, outsideAcademias, nil
}

func uint64FromDriveInt64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
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
