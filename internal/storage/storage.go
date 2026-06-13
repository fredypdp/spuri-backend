package storage

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Academias      []AcademiaUsage
}

type AcademiaUsage struct {
	CodigoAcademia string
	UsedBytes      uint64
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
		return nil, fmt.Errorf("configuração Mega inválida: MEGA_AUTH_MODE=%q não é suportado; use password, 2fa ou session", mode)
	}
	if mode == "session" && (os.Getenv("MEGA_SESSION_ID") == "" || os.Getenv("MEGA_MASTER_KEY") == "") {
		return nil, fmt.Errorf("configuração Mega incompleta: MEGA_SESSION_ID e MEGA_MASTER_KEY são obrigatórios quando MEGA_AUTH_MODE=session")
	}
	if (mode == "password" || mode == "2fa") && os.Getenv("MEGA_EMAIL") == "" && os.Getenv("ENV") == "production" {
		return nil, fmt.Errorf("configuração Mega incompleta: MEGA_EMAIL é obrigatório em produção quando MEGA_AUTH_MODE=%s", mode)
	}
	if mode == "2fa" && os.Getenv("MEGA_TOTP_CODE") == "" && os.Getenv("ENV") == "production" {
		return nil, fmt.Errorf("configuração Mega incompleta: MEGA_TOTP_CODE é obrigatório em produção quando MEGA_AUTH_MODE=2fa")
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
	if sessionID := strings.TrimSpace(os.Getenv("MEGA_SESSION_ID")); sessionID != "" {
		return m.getMegaQuota(sessionID)
	}

	if strings.TrimSpace(os.Getenv("MEGA_QUOTA_LOCAL_ESTIMATE")) != "true" {
		return QuotaInfo{}, fmt.Errorf("quota do Mega indisponível: configure MEGA_AUTH_MODE=session com MEGA_SESSION_ID e MEGA_MASTER_KEY para consultar a conta Mega; para ambiente local, defina MEGA_QUOTA_LOCAL_ESTIMATE=true para estimar apenas os arquivos em %q", m.root)
	}
	return m.getLocalQuota()
}

func (m *MegaProvider) getLocalQuota() (QuotaInfo, error) {
	var used uint64
	academias := map[string]uint64{}
	_ = filepath.WalkDir(m.root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				size := uint64(info.Size())
				used += size
				rel, relErr := filepath.Rel(m.root, path)
				if relErr == nil {
					parts := strings.Split(filepath.ToSlash(rel), "/")
					if len(parts) > 0 && parts[0] != "." {
						academias[parts[0]] += size
					}
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
	return QuotaInfo{TotalBytes: total, UsedBytes: used, AvailableBytes: avail, Academias: sortedAcademiaUsage(academias)}, nil
}

func configuredQuotaTotalBytes() (uint64, error) {
	if raw := strings.TrimSpace(os.Getenv("MEGA_QUOTA_TOTAL_BYTES")); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			return 0, fmt.Errorf("configuração Mega inválida: MEGA_QUOTA_TOTAL_BYTES=%q deve ser um inteiro positivo em bytes", raw)
		}
		return v, nil
	}
	if raw := strings.TrimSpace(os.Getenv("MEGA_QUOTA_TOTAL_GB")); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			return 0, fmt.Errorf("configuração Mega inválida: MEGA_QUOTA_TOTAL_GB=%q deve ser um inteiro positivo em GB", raw)
		}
		return v * 1024 * 1024 * 1024, nil
	}
	const freeAccountDefault uint64 = 20 * 1024 * 1024 * 1024
	return freeAccountDefault, nil
}

type megaAPIRequest struct {
	Cmd  string `json:"a"`
	Xfer int    `json:"xfer,omitempty"`
	Strg int    `json:"strg,omitempty"`
	C    int    `json:"c,omitempty"`
}

type megaQuotaResponse struct {
	TotalBytes uint64 `json:"mstrg"`
	UsedBytes  uint64 `json:"cstrg"`
}

type megaFilesResponse struct {
	Files []megaNode `json:"f"`
}

type megaNode struct {
	Hash   string `json:"h"`
	Parent string `json:"p"`
	Type   int    `json:"t"`
	Attr   string `json:"a"`
	Key    string `json:"k"`
	Size   uint64 `json:"s"`
}

func (m *MegaProvider) getMegaQuota(sessionID string) (QuotaInfo, error) {
	var quota megaQuotaResponse
	if err := megaAPI(sessionID, []megaAPIRequest{{Cmd: "uq", Xfer: 1, Strg: 1}}, &quota); err != nil {
		return QuotaInfo{}, fmt.Errorf("falha ao consultar quota real do Mega: %w", err)
	}
	academias, err := m.getMegaAcademiaUsage(sessionID)
	if err != nil {
		return QuotaInfo{}, fmt.Errorf("falha ao calcular uso por academia no Mega: %w", err)
	}
	available := uint64(0)
	if quota.TotalBytes > quota.UsedBytes {
		available = quota.TotalBytes - quota.UsedBytes
	}
	return QuotaInfo{TotalBytes: quota.TotalBytes, UsedBytes: quota.UsedBytes, AvailableBytes: available, Academias: academias}, nil
}

func (m *MegaProvider) getMegaAcademiaUsage(sessionID string) ([]AcademiaUsage, error) {
	masterKey, err := megaMasterKey()
	if err != nil {
		return nil, err
	}
	var files megaFilesResponse
	if err := megaAPI(sessionID, []megaAPIRequest{{Cmd: "f", C: 1}}, &files); err != nil {
		return nil, err
	}
	nodes := map[string]megaNode{}
	names := map[string]string{}
	for _, n := range files.Files {
		nodes[n.Hash] = n
		name, err := decryptMegaNodeName(n, masterKey)
		if err == nil && name != "" {
			names[n.Hash] = name
		}
	}
	usage := map[string]uint64{}
	for _, n := range files.Files {
		if n.Type != 0 || n.Size == 0 {
			continue
		}
		if academia := topLevelMegaFolder(n, nodes, names); academia != "" {
			usage[academia] += n.Size
		}
	}
	return sortedAcademiaUsage(usage), nil
}

func megaAPI[T any](sessionID string, payload []megaAPIRequest, dest *T) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := "https://g.api.mega.co.nz/cs?id=0&sid=" + sessionID
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status HTTP inesperado da API Mega: %d", resp.StatusCode)
	}
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return err
	}
	if len(raw) > 0 && raw[0] == '-' {
		return fmt.Errorf("API Mega retornou erro %s", string(raw))
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	if len(list) == 0 {
		return fmt.Errorf("resposta vazia da API Mega")
	}
	return json.Unmarshal(list[0], dest)
}

func megaMasterKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("MEGA_MASTER_KEY"))
	if raw == "" {
		return nil, fmt.Errorf("MEGA_MASTER_KEY é obrigatório para decifrar nomes de diretórios e calcular uso por academia")
	}
	if b, err := hex.DecodeString(raw); err == nil && len(b) == aes.BlockSize {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(raw); err == nil && len(b) == aes.BlockSize {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == aes.BlockSize {
		return b, nil
	}
	return nil, fmt.Errorf("MEGA_MASTER_KEY deve estar em hex, base64 ou base64url e ter %d bytes", aes.BlockSize)
}

func decryptMegaNodeName(n megaNode, masterKey []byte) (string, error) {
	if n.Type == 2 {
		return "Cloud Drive", nil
	}
	key, err := decryptMegaNodeKey(n.Key, masterKey, n.Type)
	if err != nil {
		return "", err
	}
	attr, err := base64.RawURLEncoding.DecodeString(n.Attr)
	if err != nil {
		return "", err
	}
	if len(attr) == 0 || len(attr)%aes.BlockSize != 0 {
		return "", fmt.Errorf("atributos Mega inválidos")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	plain := make([]byte, len(attr))
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(plain, attr)
	plain = bytes.TrimRight(plain, "\x00")
	const prefix = "MEGA"
	if !bytes.HasPrefix(plain, []byte(prefix)) {
		return "", fmt.Errorf("atributos Mega sem prefixo esperado")
	}
	var meta struct {
		Name string `json:"n"`
	}
	if err := json.Unmarshal(plain[len(prefix):], &meta); err != nil {
		return "", err
	}
	return meta.Name, nil
}

func decryptMegaNodeKey(raw string, masterKey []byte, nodeType int) ([]byte, error) {
	parts := strings.Split(raw, ":")
	encoded := parts[len(parts)-1]
	if i := strings.Index(encoded, "/"); i >= 0 {
		encoded = encoded[:i]
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(encrypted)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("chave Mega inválida")
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	decrypted := make([]byte, len(encrypted))
	for offset := 0; offset < len(encrypted); offset += aes.BlockSize {
		block.Decrypt(decrypted[offset:offset+aes.BlockSize], encrypted[offset:offset+aes.BlockSize])
	}
	if nodeType == 0 {
		if len(decrypted) < 32 {
			return nil, fmt.Errorf("chave de arquivo Mega inválida")
		}
		return xor16(decrypted[:16], decrypted[16:32]), nil
	}
	if len(decrypted) < aes.BlockSize {
		return nil, fmt.Errorf("chave de pasta Mega inválida")
	}
	return decrypted[:aes.BlockSize], nil
}

func xor16(a, b []byte) []byte {
	out := make([]byte, aes.BlockSize)
	for i := range out {
		out[i] = a[i] ^ b[i]
	}
	return out
}

func topLevelMegaFolder(n megaNode, nodes map[string]megaNode, names map[string]string) string {
	current := n
	var top string
	for current.Parent != "" {
		parent, ok := nodes[current.Parent]
		if !ok {
			break
		}
		if parent.Type == 2 {
			return top
		}
		if name := names[parent.Hash]; name != "" {
			top = name
		}
		current = parent
	}
	return top
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
