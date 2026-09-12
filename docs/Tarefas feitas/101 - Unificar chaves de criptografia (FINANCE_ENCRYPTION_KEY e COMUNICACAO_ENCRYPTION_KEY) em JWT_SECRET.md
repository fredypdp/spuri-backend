---
criado: 2026-09-12 00:00
origem: decisão do Fredy — a plataforma ainda é pequena e manter três variáveis de ambiente diferentes para criptografia (JWT_SECRET, FINANCE_ENCRYPTION_KEY, COMUNICACAO_ENCRYPTION_KEY) é uma complexidade operacional desnecessária no estágio atual. As credenciais existentes serão redefinidas manualmente pelas rotas já existentes, não por uma ferramenta de migração automática. Orquestrado e pré-validado pelo Claude (ver "Nota de validação" abaixo) contra um PostgreSQL real antes de virar tarefa para o Codex.
status: feito
---

# Unificar chaves de criptografia (FINANCE_ENCRYPTION_KEY e COMUNICACAO_ENCRYPTION_KEY) em JWT_SECRET (feito)

## Prompt recomendado para executar esta tarefa

Implemente, no repositório `rastreio-backend`, exatamente o código descrito neste documento nos arquivos `internal/security/derive.go` (novo), `internal/security/derive_test.go` (novo), `internal/finance/appypay.go`, `internal/finance/appypay_test.go`, `internal/services/comunicacao_crypto.go`, `internal/services/comunicacao_crypto_test.go`, `.env.example` e `.github/workflows/ci.yml`.

O desenho foi decidido e **testado de ponta a ponta contra um PostgreSQL 16 real** numa sessão anterior de orquestração (Claude) — ver "Nota de validação". Aqui não é necessário planejar nada: todo o código está especificado abaixo exatamente como deve ficar. Sua responsabilidade é:

1. Aplicar exatamente as seções 1–7 abaixo, nesta ordem.
2. Rodar `go build ./...` e `go vet ./...` — devem terminar sem erro.
3. Rodar `go test ./internal/security/... ./internal/finance/... ./internal/services/...`. **Você não tem PostgreSQL nem Docker disponível neste ambiente** — os testes de integração (arquivos `*_integration_test.go`, e qualquer teste que comece com `if os.Getenv("RUN_POSTGRES_INTEGRATION") != "1") { t.Skip(...) }`) vão aparecer como `SKIP`, não como `FAIL`. Isso é esperado e correto — não tente instalar PostgreSQL nem contornar o skip. Os testes puramente unitários (sem banco) devem passar de verdade; são esses que validam o seu trabalho nesta sessão.
4. Rodar `go test ./...` (suíte inteira) só para garantir que nada em outro pacote quebrou por causa da mudança de assinatura/comportamento — mesma ressalva do item 3 quanto a skips de integração.
5. Preencher a seção "Nota de validação do Codex" no final deste documento com o resultado real (build/vet/testes) antes de abrir o PR.

Não crie, não remova e não atualize nenhuma dependência em `go.mod`/`go.sum` — toda esta tarefa usa apenas a biblioteca padrão do Go e pacotes internos já existentes no módulo.

**Regra de teste importante (evita um bug real que já apareceu durante a validação):** ao testar "o que acontece sem `JWT_SECRET`", use sempre `t.Setenv("JWT_SECRET", "")` — nunca `os.Unsetenv("JWT_SECRET")` diretamente. `t.Setenv` restaura automaticamente o valor original ao fim do teste; `os.Unsetenv` chamado antes de qualquer `t.Setenv` no mesmo teste faz esse valor original se perder, deixando `JWT_SECRET` vazio para todos os testes que rodarem depois dele no mesmo processo (isso já causou uma falha real de um teste completamente não relacionado, `TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr`, durante a validação desta tarefa).

## Contexto

O backend usa três segredos de criptografia distintos hoje:

1. `JWT_SECRET` — assina e valida os tokens JWT de sessão (`internal/middleware/auth.go`). Sua configuração (obrigatória em produção, com fallback efêmero em desenvolvimento/teste, sem exigência de tamanho mínimo) **não muda nesta tarefa** — `internal/middleware/auth.go` não é tocado.
2. `FINANCE_ENCRYPTION_KEY` — cifra segredos financeiros do AppyPay (`client_secret`, `webhook_secret`) guardados na tabela `financeiro_segredos_appypay` (`internal/finance/appypay.go`).
3. `COMUNICACAO_ENCRYPTION_KEY` — cifra os tokens de API de GoSMS/Ziett guardados dentro do evento `RemetenteComunicacaoConfigurado` do ledger (`internal/services/comunicacao_crypto.go`).

Com a plataforma ainda pequena, o Fredy decidiu unificar tudo em `JWT_SECRET`, para reduzir o número de segredos a gerar, guardar e rotacionar. A chave efetiva de cada subsistema continua sendo **diferente** internamente (derivada com HMAC-SHA256 e um rótulo próprio por subsistema — ver seção 1) — unifica-se a variável de ambiente que o operador precisa gerir, não o material criptográfico em si. Isso evita que comprometer uma chave derivada exponha as outras, mesmo com uma única variável configurada.

**Duas decisões explícitas do Fredy definem o desenho abaixo:**

- **`JWT_SECRET` não ganha NENHUM requisito novo** (nem de tamanho mínimo, nem de formato). A mesma configuração que já existe hoje para assinatura de tokens passa a servir também de raiz para derivar as chaves de criptografia — nada além disso muda. Se o valor atual de `JWT_SECRET` em produção for curto, ele continua válido; não é necessário trocá-lo.
- **Não existe ferramenta de migração automática, nem suporte a ler dados cifrados com as chaves antigas.** Depois do deploy desta tarefa, `FINANCE_ENCRYPTION_KEY` e `COMUNICACAO_ENCRYPTION_KEY` deixam de ser lidas por qualquer código de produção — os dados que estavam cifrados com elas ficam ilegíveis a partir do deploy. O Fredy vai redefinir manualmente as credenciais financeiras e os tokens de comunicação através das rotas HTTP que já existem (`PUT /financeiro/appypay/credenciais/:id` ou apagar com `DELETE /financeiro/appypay/credenciais` e recriar com `POST`; `POST /comunicacao/admin/remetentes`, que já substitui o remetente existente do mesmo provedor). Isso está fora do escopo desta tarefa de código — ver "Fora de escopo" no final.

Isso torna esta tarefa bem mais simples do que uma migração de dados em produção: é só trocar a fonte da chave de criptografia e simplificar a validação de configuração para acompanhar.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Variável de ambiente para criptografia | Só `JWT_SECRET` a partir de agora — sem nenhum requisito novo de tamanho ou formato | `FINANCE_ENCRYPTION_KEY` e `COMUNICACAO_ENCRYPTION_KEY` deixam de ser lidas por qualquer código de produção |
| Derivação de chave | `internal/security.DeriveKey(label)` = HMAC-SHA256(JWT_SECRET, label), um label fixo por subsistema | `internal/finance` e `internal/services` acabam com chaves AES-256 efetivas diferentes, mesmo lendo a mesma variável |
| Dados já cifrados com as chaves antigas | Ficam ilegíveis após o deploy — não há fallback nem ferramenta de migração | O Fredy redefine as credenciais manualmente pelas rotas HTTP existentes (fora do escopo desta tarefa) |
| `internal/middleware/auth.go` | Não é tocado | Configuração e comportamento de `JWT_SECRET` para assinatura de tokens permanecem idênticos |
| Frontend (`rastreio-frontend`) | Nenhuma mudança — este é um segredo interno do backend, nunca exposto pela API | Não tocar neste repositório |

---

## Nota de validação (já realizada por Claude, o orquestrador, ANTES desta tarefa)

Todo o código abaixo já foi escrito, compilado e testado por Claude contra um PostgreSQL 16 real (não é só leitura de código) antes de virar este documento. Resultado, resumido:

- **Build:** `go build` de todos os pacotes tocados — sem erro.
- **`go vet`:** limpo.
- **Suíte de testes completa, rodada repetidamente contra um banco limpo (drop+create+migrations a cada rodada):** `internal/finance`, `internal/services`, `internal/security`, `internal/db`, `internal/projections`, `internal/middleware`, `internal/domain/aggregates` — todos `ok`, de forma consistente e reprodutível.
- **Duas iterações de bugs reais encontrados e corrigidos durante a validação** (documentados aqui para o Codex não reintroduzi-los):
  1. Uma primeira versão exigia `JWT_SECRET` com pelo menos 32 caracteres — removida a pedido explícito do Fredy; a versão final só exige que a variável não esteja vazia.
  2. Um teste (`TestEncryptionKeyOnlyRequiresNonEmptySecret`) usava `os.Unsetenv("JWT_SECRET")` antes de qualquer `t.Setenv`, o que apagava permanentemente o valor ambiente de `JWT_SECRET` para o resto da execução de `go test` — isso derrubou um teste completamente não relacionado (`TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr`, que depende do `JWT_SECRET` do ambiente do job de CI em vez de um `t.Setenv` próprio). Corrigido usando só `t.Setenv` (ver a regra de teste destacada no topo deste documento).
- **Frontend (`rastreio-frontend`):** verificado — não há nenhuma referência a `FINANCE_ENCRYPTION_KEY`, `COMUNICACAO_ENCRYPTION_KEY` nem a este mecanismo de criptografia. Nenhuma mudança necessária lá.

Ambiente usado para esta validação: Go 1.22 (com um `go.mod` temporário, só local, para contornar a ausência de Go 1.24 nesse sandbox específico — **não leve isso para o código real**; o `go.mod` deste repositório continua exatamente como está, sem nenhuma mudança de versão/dependência) e PostgreSQL 16 real via `apt`.

---

# 1. `internal/security/derive.go` (novo arquivo)

Criar o arquivo com este conteúdo exato:

```go
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
```

# 2. `internal/security/derive_test.go` (novo arquivo)

Criar o arquivo com este conteúdo exato:

```go
package security

import "testing"

func TestDeriveKeyRequiresSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if _, err := DeriveKey("x"); err == nil {
		t.Fatal("esperava erro sem JWT_SECRET")
	}
}

func TestDeriveKeyAcceptsAnyNonEmptySecretLength(t *testing.T) {
	// JWT_SECRET não ganha nenhum requisito novo de tamanho — a mesma
	// configuração usada hoje para assinar tokens JWT também serve para
	// derivar chaves de criptografia, curta ou longa.
	t.Setenv("JWT_SECRET", "123")
	if _, err := DeriveKey("x"); err != nil {
		t.Fatalf("JWT_SECRET curto deveria ser aceito: %v", err)
	}
}

func TestDeriveKeyIsDeterministicAndLabelSeparated(t *testing.T) {
	t.Setenv("JWT_SECRET", "um-segredo-bem-longo-com-mais-de-32-caracteres")
	k1a, err := DeriveKey("finance")
	if err != nil {
		t.Fatal(err)
	}
	k1b, err := DeriveKey("finance")
	if err != nil {
		t.Fatal(err)
	}
	if string(k1a) != string(k1b) {
		t.Fatal("DeriveKey não é determinística para o mesmo label")
	}
	k2, err := DeriveKey("comunicacao")
	if err != nil {
		t.Fatal(err)
	}
	if string(k1a) == string(k2) {
		t.Fatal("labels diferentes produziram a MESMA chave — falha de separação de domínio")
	}
	if len(k1a) != 32 {
		t.Fatalf("chave derivada deveria ter 32 bytes (AES-256), tem %d", len(k1a))
	}
}
```

Este pacote não depende de banco de dados — todos os três testes acima devem passar de verdade no seu ambiente.

# 3. `internal/finance/appypay.go`

## 3.1 Import novo

Localizar o bloco de import (topo do arquivo) e adicionar `"spuri/internal/security"` à lista de imports internos, e remover `"crypto/sha256"` (deixa de ser usado neste arquivo):

Localizar:

```go
import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)
```

Substituir por:

```go
import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
	"spuri/internal/security"
)
```

**Atenção:** confira se `crypto/sha256` não é usado em mais nenhum outro ponto deste arquivo antes de removê-lo do import (na versão atual do arquivo, o único uso era dentro de `key()`, que esta tarefa substitui — ver 3.2). Se `go vet`/`go build` acusar `sha256` usado em outro lugar, mantenha o import.

## 3.2 `key()` / `encrypt()` / `decrypt()` / `ValidateEncryptionConfig()`

Localizar (bloco inteiro, perto do final do arquivo, logo antes de `func constantTimeEqual`):

```go
func key() ([]byte, error) {
	v := strings.TrimSpace(os.Getenv("FINANCE_ENCRYPTION_KEY"))
	if v == "" {
		return nil, errors.New("FINANCE_ENCRYPTION_KEY é obrigatória")
	}
	if decoded, err := base64.StdEncoding.DecodeString(v); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(v) < 32 {
		return nil, errors.New("FINANCE_ENCRYPTION_KEY deve ter pelo menos 32 caracteres ou ser Base64 de 32 bytes")
	}
	sum := sha256.Sum256([]byte(v))
	return sum[:], nil
}

// ValidateEncryptionConfig validates the mandatory financial-secret key at
// startup, before a request can attempt to persist any credentials.
func ValidateEncryptionConfig() error {
	_, err := key()
	return err
}
func encrypt(v string) (string, error) {
	k, err := key()
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
func decrypt(v string) (string, error) {
	k, err := key()
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
	return string(plain), err
}
```

Substituir por:

```go
// financeKeyLabel separa criptograficamente a chave financeira derivada de
// JWT_SECRET de qualquer outra chave derivada da mesma raiz (ex.:
// comunicação) — ver spuri/internal/security.
const financeKeyLabel = "spuri:finance-encryption:v1"

// key deriva a chave AES-256 atual (a partir de JWT_SECRET) usada para
// cifrar e decifrar todo segredo financeiro.
func key() ([]byte, error) {
	return security.DeriveKey(financeKeyLabel)
}

// ValidateEncryptionConfig validates the mandatory financial-secret key at
// startup, before a request can attempt to persist any credentials.
func ValidateEncryptionConfig() error {
	_, err := key()
	return err
}
func encrypt(v string) (string, error) {
	k, err := key()
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
func decrypt(v string) (string, error) {
	k, err := key()
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
	return string(plain), err
}
```

(`saveSecrets` e `loadSecrets` — que chamam `encrypt`/`decrypt` — **não mudam nesta tarefa**: continuam exatamente como estão hoje, incluindo a coluna `key_id` da tabela `financeiro_segredos_appypay`, que esta tarefa não usa nem altera.)

# 4. `internal/finance/appypay_test.go`

## 4.1 Testes de criptografia

Localizar:

```go
func TestEncryptionRoundTripAndNoFallbackKey(t *testing.T) {
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	ciphertext, err := encrypt("segredo AppyPay")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "segredo") {
		t.Fatal("ciphertext contém texto claro")
	}
	plain, err := decrypt(ciphertext)
	if err != nil || plain != "segredo AppyPay" {
		t.Fatalf("round trip inválido: %q %v", plain, err)
	}
	os.Unsetenv("FINANCE_ENCRYPTION_KEY")
	if _, err := encrypt("x"); err == nil {
		t.Fatal("esperava falha sem FINANCE_ENCRYPTION_KEY")
	}
}

func TestEncryptionKeyRequiresStrongMaterial(t *testing.T) {
	t.Setenv("FINANCE_ENCRYPTION_KEY", "123")
	if err := ValidateEncryptionConfig(); err == nil {
		t.Fatal("chave curta foi aceite")
	}
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	if err := ValidateEncryptionConfig(); err != nil {
		t.Fatalf("chave válida foi rejeitada: %v", err)
	}
}
```

Substituir por:

```go
func TestEncryptionRoundTripAndNoFallbackKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-only-secret-material-at-least-32")
	ciphertext, err := encrypt("segredo AppyPay")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "segredo") {
		t.Fatal("ciphertext contém texto claro")
	}
	plain, err := decrypt(ciphertext)
	if err != nil || plain != "segredo AppyPay" {
		t.Fatalf("round trip inválido: %q %v", plain, err)
	}
	t.Setenv("JWT_SECRET", "")
	if _, err := encrypt("x"); err == nil {
		t.Fatal("esperava falha sem JWT_SECRET")
	}
}

func TestEncryptionKeyOnlyRequiresNonEmptySecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if err := ValidateEncryptionConfig(); err == nil {
		t.Fatal("JWT_SECRET ausente foi aceite")
	}
	// JWT_SECRET não ganha nenhum requisito novo de tamanho — um valor
	// curto deve ser aceito, exatamente como já é hoje para a assinatura
	// de tokens JWT.
	t.Setenv("JWT_SECRET", "123")
	if err := ValidateEncryptionConfig(); err != nil {
		t.Fatalf("chave curta (mas não vazia) foi rejeitada: %v", err)
	}
}
```

**Importante:** note que a versão nova usa `t.Setenv("JWT_SECRET", "")` em vez de `os.Unsetenv(...)` — isto não é estilo, é funcionalmente necessário (ver a "Regra de teste importante" no topo deste documento). Não troque de volta para `os.Unsetenv`.

## 4.2 Outros testes deste arquivo (e de outros arquivos) que usam `FINANCE_ENCRYPTION_KEY`

Procure em **todo o repositório** por `t.Setenv("FINANCE_ENCRYPTION_KEY"`. No momento em que este documento foi escrito, as ocorrências (além do trecho já tratado acima) estavam em:

- `internal/finance/appypay_integration_test.go`
- `internal/finance/lista_unificada_pendencia_e_falha_integration_test.go`
- `internal/finance/mensalidades_em_aberto_integration_test.go`
- `internal/handlers/financeiro_handlers_integration_test.go`
- `internal/handlers/financeiro_matricula_consulta_test.go`

Troque cada ocorrência de:

```go
t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
```

por:

```go
t.Setenv("JWT_SECRET", "test-only-secret-material-at-least-32")
```

(mesma string de valor — só o nome da variável muda). Não altere nenhuma outra linha desses arquivos.

Depois, rode de novo a mesma busca (`git grep -n 't.Setenv("FINANCE_ENCRYPTION_KEY"'`) para confirmar que não sobrou nenhuma ocorrência — a lista acima é a que existia no momento em que este documento foi escrito, mas o repositório pode ter mudado desde então (confira também se apareceu algum uso **ambiente** desta variável, sem `t.Setenv`, como aconteceu com `internal/finance/qrcode_regression_integration_test.go` durante a validação desta tarefa — esse arquivo específico não precisa de nenhuma mudança, porque não faz nenhuma referência direta a `FINANCE_ENCRYPTION_KEY` nem a `JWT_SECRET`; ele já vai funcionar assim que o `JWT_SECRET` do ambiente do job de CI — seção 7 abaixo — estiver definido).

# 5. `internal/services/comunicacao_crypto.go`

Substituir **todo o conteúdo** deste arquivo por:

```go
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
```

# 6. `internal/services/comunicacao_crypto_test.go`

Substituir **todo o conteúdo** deste arquivo por:

```go
package services

import "testing"

func TestComunicacaoCryptoRoundTrip(t *testing.T) {
	t.Setenv("JWT_SECRET", "uma-chave-unificada-com-pelo-menos-32-caracteres")
	cifrado, err := EncryptComunicacaoSegredo("token-secreto")
	if err != nil {
		t.Fatalf("EncryptComunicacaoSegredo() error = %v", err)
	}
	if cifrado == "token-secreto" {
		t.Fatal("EncryptComunicacaoSegredo() returned plaintext")
	}
	plano, err := DecryptComunicacaoSegredo(cifrado)
	if err != nil {
		t.Fatalf("DecryptComunicacaoSegredo() error = %v", err)
	}
	if plano != "token-secreto" {
		t.Fatalf("decrypted value = %q, want token-secreto", plano)
	}
}

func TestValidateComunicacaoEncryptionConfigRequiresNonEmptyKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if err := ValidateComunicacaoEncryptionConfig(); err == nil {
		t.Fatal("ValidateComunicacaoEncryptionConfig() accepted missing key")
	}
	// JWT_SECRET não ganha nenhum requisito novo de tamanho — mesmo um
	// valor curto deve ser aceito, exatamente como já é hoje para a
	// assinatura de tokens JWT.
	t.Setenv("JWT_SECRET", "short")
	if err := ValidateComunicacaoEncryptionConfig(); err != nil {
		t.Fatalf("ValidateComunicacaoEncryptionConfig() rejected a short (but non-empty) key: %v", err)
	}
}
```

# 7. `.env.example`

## 7.1 Bloco "Segurança / JWT"

Localizar:

```
# =============================================================================
# Segurança / JWT
# =============================================================================
JWT_SECRET=seu_segredo_muito_forte_aqui
JWT_EXPIRY_HOURS=24

# Chave obrigatória para criptografia de segredos financeiros em qualquer ambiente.
FINANCE_ENCRYPTION_KEY=troque-por-uma-chave-longa-aleatoria
```

Substituir por:

```
# =============================================================================
# Segurança / JWT
# =============================================================================
# JWT_SECRET agora tem DOIS usos: (1) assinar/validar tokens JWT de sessão
# (como sempre) e (2) servir de raiz para derivar as chaves AES-256 que
# cifram segredos financeiros (AppyPay) e tokens de comunicação
# (GoSMS/Ziett) — ver spuri/internal/security.DeriveKey. A configuração
# desta variável não muda: continua sem exigência de tamanho mínimo, com o
# mesmo comportamento de sempre.
JWT_SECRET=seu_segredo_muito_forte_aqui
JWT_EXPIRY_HOURS=24
```

(O bloco `FINANCE_ENCRYPTION_KEY` é removido — não é mais lida por nenhum código.)

## 7.2 Bloco "Comunicação"

Localizar:

```
# =============================================================================
# Comunicação (módulo real — GoSMS/Ziett)
# =============================================================================
# Chave obrigatória para cifrar os tokens de API de GoSMS/Ziett gravados em
# projection_remetentes_comunicacao (cadastrados via POST /comunicacao/remetentes).
# Independente de FINANCE_ENCRYPTION_KEY e de ZIETT_API_KEY acima — este módulo
# nunca lê ZIETT_API_KEY nem qualquer outra variável de ambiente para enviar
# mensagens reais; o token de cada provedor vem sempre do banco, cifrado.
COMUNICACAO_ENCRYPTION_KEY=troque-por-uma-chave-longa-aleatoria-diferente-da-financeira
```

Substituir por:

```
# =============================================================================
# Comunicação (módulo real — GoSMS/Ziett)
# =============================================================================
# Os tokens de API de GoSMS/Ziett (gravados em
# projection_remetentes_comunicacao, cadastrados via POST
# /comunicacao/remetentes) são cifrados com uma chave derivada de
# JWT_SECRET (ver acima) — não precisa configurar nada aqui além de
# JWT_SECRET. Este módulo nunca lê ZIETT_API_KEY nem qualquer outra
# variável de ambiente para enviar mensagens reais; o token de cada
# provedor vem sempre do banco, cifrado.
```

(O valor `COMUNICACAO_ENCRYPTION_KEY=...` é removido — não é mais lida por nenhum código.)

# 8. `.github/workflows/ci.yml`

Localizar:

```yaml
    env:
      DATABASE_URL: postgres://postgres:postgres@localhost:5432/spuri_test?sslmode=disable
      FINANCE_ENCRYPTION_KEY: ci-finance-encryption-key-at-least-32
      RUN_POSTGRES_INTEGRATION: "1"
```

Substituir por:

```yaml
    env:
      DATABASE_URL: postgres://postgres:postgres@localhost:5432/spuri_test?sslmode=disable
      JWT_SECRET: ci-unified-jwt-secret
      RUN_POSTGRES_INTEGRATION: "1"
```

(`JWT_SECRET` substitui `FINANCE_ENCRYPTION_KEY` aqui — sem exigência de tamanho, um valor curto como o do exemplo já é suficiente. Esta variável de ambiente do job é necessária porque pelo menos um teste, `internal/finance/qrcode_regression_integration_test.go`, depende do valor ambiente em vez de definir o seu próprio via `t.Setenv`.)

---

## Critérios de aceite

- [ ] `go build ./...` sem erro.
- [ ] `go vet ./...` sem erro.
- [ ] `go test ./internal/security/...` — os 3 testes passam de verdade (sem banco).
- [ ] `go test ./internal/finance/...` — testes unitários passam de verdade; testes de integração aparecem como `SKIP` (sem PostgreSQL neste ambiente) — isso é esperado, não é falha.
- [ ] `go test ./internal/services/...` — idem.
- [ ] `go test ./...` — nenhum outro pacote quebrou (mesma ressalva de skips de integração).
- [ ] Nenhuma mudança em `go.mod`/`go.sum`.
- [ ] `git grep -rn "FINANCE_ENCRYPTION_KEY\|COMUNICACAO_ENCRYPTION_KEY"` no repositório inteiro não deve encontrar **nenhuma** ocorrência depois desta tarefa (nem em código, nem em `.env.example`, nem em `.github/workflows/ci.yml`) — diferente de uma migração com período de transição, aqui a remoção é total e imediata.
- [ ] `internal/middleware/auth.go` não foi alterado.

## Fora de escopo (não fazer)

- **Não** criar nenhuma ferramenta de migração de dados, nem qualquer lógica de fallback/leitura dupla (chave nova + chave antiga). Depois deste deploy, dados cifrados com as chaves antigas ficam ilegíveis — isso é esperado. Redefinir essas credenciais é uma ação manual do Fredy pelas rotas HTTP já existentes, feita separadamente, fora desta tarefa de código.
- **Não** adicionar nenhuma validação de tamanho mínimo, formato, ou "força" para `JWT_SECRET`, em nenhum lugar do código (nem em `internal/security`, nem em `internal/middleware`). A configuração de `JWT_SECRET` deve continuar exatamente como está hoje.
- **Não** alterar `internal/middleware/auth.go` — nem o fallback efêmero em desenvolvimento/teste, nem a validação em produção, nem qualquer outro comportamento de assinatura de tokens.
- **Não** tocar no repositório `rastreio-frontend` — confirmado que não há nenhuma dependência lá.
- **Não** deixar `FINANCE_ENCRYPTION_KEY`/`COMUNICACAO_ENCRYPTION_KEY` em nenhum arquivo do repositório ao final desta tarefa (diferente de uma migração gradual, aqui a remoção é completa).

## Procedimento de conclusão

Depois dos critérios de aceite passarem, mova este arquivo de `docs/Lista de Tarefas/` para `docs/Tarefas feitas/`, atualize o front-matter (`status: feito`) e o título (troque `(pendente)` por `(feito)`), e preencha a seção abaixo.

## Nota de validação do Codex

- **Build:** `go build ./...` terminou sem erro.
- **Vet:** `go vet ./...` terminou sem erro.
- **Testes direcionados:** `go test ./internal/security/... ./internal/finance/... ./internal/services/...` terminou sem erro. Os testes unitários novos de `internal/security`, `internal/finance` e `internal/services` passaram; os testes de integração sem PostgreSQL mantiveram o comportamento de `SKIP` esperado.
- **Suíte completa:** `go test ./...` terminou sem erro, com os skips de integração esperados neste ambiente.
- **Verificações adicionais:** `gofmt` foi aplicado aos arquivos Go alterados e `git diff --check` terminou sem erro. `go.mod` e `go.sum` não foram alterados; `internal/middleware/auth.go` também não foi alterado.
- **Divergência documentada:** a busca literal pelas antigas variáveis ainda encontra referências históricas neste próprio documento, em outras tarefas já concluídas e nas migrations que preservam o valor histórico de `key_id`. Conforme a seção 3.2, essas migrations não foram alteradas; nenhum código de produção, teste ativo, exemplo de ambiente ou workflow de CI lê as antigas variáveis.
