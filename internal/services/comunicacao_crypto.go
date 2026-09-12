package services

// Criptografia dos tokens de API dos provedores de comunicação (GoSMS,
// Ziett). Implementação isolada e independente de internal/finance —
// mesmo algoritmo (AES-256-GCM) que internal/finance/appypay.go usa para
// segredos financeiros, mas com uma chave efetiva DIFERENTE: ambas são
// derivadas da mesma raiz (JWT_SECRET) por spuri/internal/security, com um
// "label" próprio deste pacote, para que comprometer uma não exponha a
// outra mesmo partilhando a variável de ambiente.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"

	"spuri/internal/security"
)

// comunicacaoKeyLabel separa esta chave, na derivação a partir de
// JWT_SECRET, de qualquer outro uso (ex.: financeKeyLabel em
// internal/finance).
const comunicacaoKeyLabel = "spuri:comunicacao-encryption:v1"

// comunicacaoEncryptionKey deriva a chave AES-256 atual (a partir de
// JWT_SECRET), usada para cifrar e decifrar todo token de comunicação.
func comunicacaoEncryptionKey() ([]byte, error) {
	return security.DeriveKey(comunicacaoKeyLabel)
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
	return aesGCMSeal(k, v)
}

func aesGCMSeal(k []byte, v string) (string, error) {
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
