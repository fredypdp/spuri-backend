package services

// Criptografia dos tokens de API dos provedores de comunicação (GoSMS,
// Ziett). Implementação isolada e independente de internal/finance —
// mesmo algoritmo (AES-256-GCM) e mesmas regras de validação de chave que
// internal/finance/appypay.go usa para segredos financeiros, mas com uma
// variável de ambiente própria (COMUNICACAO_ENCRYPTION_KEY), porque os
// tokens de GoSMS/Ziett não são "segredos financeiros" e não devem
// compartilhar FINANCE_ENCRYPTION_KEY nem qualquer outro código do pacote
// internal/finance.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

func comunicacaoEncryptionKey() ([]byte, error) {
	v := strings.TrimSpace(os.Getenv("COMUNICACAO_ENCRYPTION_KEY"))
	if v == "" {
		return nil, errors.New("COMUNICACAO_ENCRYPTION_KEY é obrigatória")
	}
	if decoded, err := base64.StdEncoding.DecodeString(v); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(v) < 32 {
		return nil, errors.New("COMUNICACAO_ENCRYPTION_KEY deve ter pelo menos 32 caracteres ou ser Base64 de 32 bytes")
	}
	sum := sha256.Sum256([]byte(v))
	return sum[:], nil
}

// ValidateComunicacaoEncryptionConfig valida a chave obrigatória de
// criptografia dos tokens de comunicação no arranque do servidor, antes de
// qualquer requisição poder tentar persistir um token.
func ValidateComunicacaoEncryptionConfig() error {
	_, err := comunicacaoEncryptionKey()
	return err
}

// EncryptComunicacaoSegredo cifra um token de API (GoSMS ou Ziett) com
// AES-256-GCM. Retorna uma string Base64 (nonce + ciphertext) pronta para
// gravar em token_api_cifrado.
func EncryptComunicacaoSegredo(v string) (string, error) {
	k, err := comunicacaoEncryptionKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(append(nonce, gcm.Seal(nil, nonce, []byte(v), nil)...)), nil
}

// DecryptComunicacaoSegredo decifra um valor gravado por
// EncryptComunicacaoSegredo. Usado apenas no momento de enviar uma
// mensagem — o valor decifrado nunca deve ser logado nem devolvido em
// nenhuma resposta HTTP.
func DecryptComunicacaoSegredo(v string) (string, error) {
	k, err := comunicacaoEncryptionKey()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext inválido")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
