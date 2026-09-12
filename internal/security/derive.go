// Package security centraliza a derivação de chaves simétricas de
// criptografia a partir de um único segredo raiz (JWT_SECRET), para que a
// plataforma não precise gerir múltiplas variáveis de ambiente de chave.
//
// Em vez de reutilizar os bytes de JWT_SECRET diretamente como chave AES
// (o que faria com que qualquer subsistema que reutilizasse esta função
// obtivesse a MESMA chave efetiva), cada chamador fornece um "label" fixo
// e exclusivo do seu contexto de uso. A chave AES-256 real é derivada via
// HMAC-SHA256(JWT_SECRET, label): domínios diferentes (financeiro,
// comunicação, etc.) acabam com chaves efetivas diferentes, mesmo
// partilhando a mesma raiz — comprometer uma chave derivada não expõe as
// restantes.
//
// Esta função NÃO impõe nenhum requisito novo sobre JWT_SECRET (ex.:
// tamanho mínimo) além de exigir que ela esteja definida — a configuração
// de JWT_SECRET para fins de assinatura de tokens (internal/middleware,
// incluindo o fallback efêmero em desenvolvimento/teste) permanece
// exatamente como está, sem nenhuma mudança.
package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"os"
	"strings"
)

// DeriveKey deriva uma chave AES-256 (32 bytes) a partir de JWT_SECRET e
// de um rótulo fixo de contexto. O rótulo NUNCA deve ser construído a
// partir de input do usuário — apenas constantes de código (ver
// financeKeyLabel em internal/finance e comunicacaoKeyLabel em
// internal/services).
func DeriveKey(label string) ([]byte, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		return nil, errors.New("JWT_SECRET é obrigatória")
	}
	if label == "" {
		return nil, errors.New("label de derivação é obrigatório")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(label))
	return mac.Sum(nil), nil
}
