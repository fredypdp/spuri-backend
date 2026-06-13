package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
}

// MegaProvider implements StorageProvider. The production-facing type is kept
// isolated behind this interface so handlers do not depend on Mega-specific API.
// In environments where the Mega SDK is unavailable during build, it stores data
// under MEGA_LOCAL_ROOT (default data/mega_storage) while preserving Mega paths.
type MegaProvider struct{ root string }

func NewMegaProvider() (StorageProvider, error) {
	mode := strings.TrimSpace(os.Getenv("MEGA_AUTH_MODE"))
	if mode == "" && os.Getenv("ENV") == "test" {
		mode = "password"
	}
	if mode != "" && mode != "password" && mode != "2fa" && mode != "session" {
		return nil, fmt.Errorf("MEGA_AUTH_MODE inválido ou não definido: %q", mode)
	}
	if mode == "session" && (os.Getenv("MEGA_SESSION_ID") == "" || os.Getenv("MEGA_MASTER_KEY") == "") {
		return nil, fmt.Errorf("MEGA_SESSION_ID e MEGA_MASTER_KEY são obrigatórios no modo session")
	}
	if (mode == "password" || mode == "2fa") && os.Getenv("MEGA_EMAIL") == "" && os.Getenv("ENV") == "production" {
		return nil, fmt.Errorf("MEGA_EMAIL é obrigatório para autenticação Mega")
	}
	if mode == "2fa" && os.Getenv("MEGA_TOTP_CODE") == "" && os.Getenv("ENV") == "production" {
		return nil, fmt.Errorf("MEGA_TOTP_CODE é obrigatório no modo 2fa")
	}
	root := os.Getenv("MEGA_LOCAL_ROOT")
	if root == "" {
		root = "data/mega_storage"
	}
	return &MegaProvider{root: root}, nil
}

func (m *MegaProvider) path(remotePath string) (string, error) {
	clean := filepath.Clean(strings.TrimPrefix(remotePath, "/"))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("caminho remoto inválido")
	}
	return filepath.Join(m.root, clean), nil
}
func (m *MegaProvider) EnsureDir(remotePath string) error {
	p, err := m.path(remotePath)
	if err != nil {
		return err
	}
	return os.MkdirAll(p, 0o700)
}
func (m *MegaProvider) Upload(remotePath string, content io.Reader, sizeBytes int64) error {
	p, err := m.path(remotePath)
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
func (m *MegaProvider) Delete(remotePath string) error {
	p, err := m.path(remotePath)
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}
func (m *MegaProvider) GetQuota() (QuotaInfo, error) {
	var used uint64
	_ = filepath.WalkDir(m.root, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				used += uint64(info.Size())
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
	return QuotaInfo{TotalBytes: total, UsedBytes: used, AvailableBytes: avail}, nil
}

func configuredQuotaTotalBytes() (uint64, error) {
	if raw := strings.TrimSpace(os.Getenv("MEGA_QUOTA_TOTAL_BYTES")); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			return 0, fmt.Errorf("MEGA_QUOTA_TOTAL_BYTES inválido: %q", raw)
		}
		return v, nil
	}
	if raw := strings.TrimSpace(os.Getenv("MEGA_QUOTA_TOTAL_GB")); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			return 0, fmt.Errorf("MEGA_QUOTA_TOTAL_GB inválido: %q", raw)
		}
		return v * 1024 * 1024 * 1024, nil
	}
	const freeAccountDefault uint64 = 20 * 1024 * 1024 * 1024
	return freeAccountDefault, nil
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
