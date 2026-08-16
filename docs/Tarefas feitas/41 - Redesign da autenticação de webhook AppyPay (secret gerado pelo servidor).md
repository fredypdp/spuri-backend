---
criado: 2026-08-15 00:00
origem: decisão do Fredy — o segredo de webhook AppyPay passa a ser gerado pelo servidor (15 caracteres alfanuméricos) e o nome do cabeçalho HTTP deixa de ser configurável por credencial, tornando-se uma constante fixa para toda a plataforma. Continuação formal de `docs/Lista de Tarefas/Handoff — Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md`.
status: feito
---

# Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor) (feito)

## Prompt recomendado para executar esta tarefa

Implemente, no repositório `spuri-backend`, exatamente o código descrito neste documento nos arquivos `internal/finance/appypay.go`, `internal/handlers/financeiro_handlers.go`, `cmd/server/main.go`, `internal/db/safe_queries.go`, `internal/projections/financeiro_projection.go`, `internal/finance/appypay_test.go`, `internal/finance/appypay_integration_test.go`, `internal/handlers/financeiro_handlers_integration_test.go`, `Documentação da API.md` e `docs/Parceiros e integrações/AppyPay Documentação.md`.

O desenho foi decidido e revisado linha a linha contra o estado real e atual do repositório (clonado no momento da escrita deste documento). **Diferente de tarefas anteriores deste módulo, este código não foi validado nesta sessão com `go build`/`go test` reais** — o ambiente onde este documento foi escrito não tem Go nem PostgreSQL disponíveis, apenas acesso de leitura ao repositório. Por isso:

1. Aplique exatamente as seções 1–9 abaixo.
2. Rode toda a validação da seção "Critérios de aceite".
3. Execute obrigatoriamente o "Passo obrigatório de investigação" antes de finalizar — ele resolve duas pendências deixadas pela sessão anterior de orquestração (ver Contexto).
4. Preencha a "Nota de validação" com o resultado real antes de mover este arquivo para `docs/Tarefas feitas/`.

Não é necessário planejar nada além disso: todo o código está especificado abaixo exatamente como deve ficar.

## Contexto

A autenticação de webhook AppyPay já passou por duas rodadas de mudança antes desta:

1. **Tarefa 24** (`docs/Tarefas feitas/24 - Cabeçalho de webhook configurável e resource AppyPay via variável de ambiente.md`): tornou o nome do cabeçalho de webhook configurável por credencial (`webhook_header_name`, padrão `X-API-Key`) e moveu `resource` para a variável de ambiente `APPYPAY_RESOURCE`.
2. **Tarefa 30** (`docs/Tarefas feitas/30 - Simplificar autenticação de webhook AppyPay para um único método.md`, commit `8ed624f`): eliminou o modo "Basic Auth", deixando só "API Key" — mas o nome do cabeçalho continuava configurável por credencial e o segredo continuava sendo digitado pelo usuário.

Ao explicar a diferença entre os métodos para o Fredy, ele decidiu ir mais longe: como a AppyPay só oferece um único par nome/valor de cabeçalho no painel deles, não há necessidade de o nome do cabeçalho variar por credencial — pode ser um valor fixo, único, para toda a plataforma. E, aproveitando a simplificação, decidiu também tirar do usuário a responsabilidade de criar o segredo: o backend gera automaticamente um segredo aleatório quando a credencial é cadastrada, e o usuário só precisa copiá-lo para colar no painel da AppyPay.

Uma sessão anterior deste orquestrador (Claude) já discutiu e desenhou esta mudança com o Fredy, e registrou o histórico completo em `docs/Lista de Tarefas/Handoff — Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md`. Este documento formal **substitui** aquele handoff: depois desta tarefa ser concluída (ver "Procedimento de conclusão"), o handoff pode ser removido.

O frontend (`spuripainel`) tem uma tarefa própria e separada para refletir esta mudança — **não faz parte deste documento** e deve ser orquestrada só depois deste backend estar concluído e depurado.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Nome do cabeçalho de webhook | Fixo, global, `X-Spuri-Webhook-Secret` (nova constante exportada `finance.WebhookHeaderName`) | Deixa de ser configurável por credencial; `webhook_header_name` na resposta é sempre este valor |
| Segredo de webhook | Gerado pelo servidor (15 caracteres alfanuméricos, `crypto/rand` + `math/big`), nunca digitado pelo usuário | `CredentialInput` perde os campos `webhook_secret` e `webhook_header_name` |
| Exposição do segredo | Só em texto plano na criação (`POST .../credenciais`) e nas rotas dedicadas de consulta/rotação | Nunca aparece em `PUT`, listagem ou qualquer outra resposta |
| Rotação | Endpoint próprio, autorizado ao dono do contexto (academia) ou admin com permissão `fpp` | `POST .../credenciais/:id/webhook-secret/rotacionar`; grava evento `SegredoWebhookAppyPayRotacionado` no ledger |
| Consulta | Endpoint próprio, mesma autorização da rotação | `GET .../credenciais/:id/webhook-secret` |
| `ConfigureCredential` | Passa a retornar `(CredentialView, string, error)` | A `string` só vem preenchida na primeira vez que a credencial ganha um segredo (checado via `hasWebhookSecret`) |
| Retrocompatibilidade | Sem migração de dados: nenhuma tabela muda de schema | Credenciais antigas continuam existindo; seu `webhook_header_name`/`webhook_secret` antigos deixam de ser lidos — a autenticação passa a depender só do cofre (`financeiro_segredos_appypay`) e da constante global |
| Frontend (`spuripainel`) | Tarefa própria e separada, feita depois desta | Não tocar neste repositório a partir desta tarefa |
| Achado à parte (fora de escopo, ver seção própria) | A suíte de testes de integração de `internal/finance`/`internal/handlers` tem falhas pré-existentes, algumas possivelmente não relacionadas a esta mudança | Isolar a causa (passo obrigatório abaixo) e avisar o Fredy separadamente; não corrigir aqui |

---

# 1. `internal/finance/appypay.go`

## 1.1 Import novo

Localizar o bloco de import:

```go
import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)
```

Substituir por (única mudança: `"math/big"` adicionado depois de `"math"`):

```go
import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)
```

## 1.2 `CredentialInput`

Localizar:

```go
type CredentialInput struct {
	ContextoTipo      string `json:"contexto_tipo"`
	CodigoAcademia    string `json:"codigo_academia,omitempty"`
	Ambiente          string `json:"ambiente"`
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	GPOPaymentMethod  string `json:"gpo_payment_method"`
	REFPaymentMethod  string `json:"ref_payment_method"`
	WebhookSecret     string `json:"webhook_secret,omitempty"`
	WebhookHeaderName string `json:"webhook_header_name,omitempty"` // nome do cabeçalho HTTP onde a AppyPay envia webhook_secret; padrão "X-API-Key"
}
```

Substituir por:

```go
type CredentialInput struct {
	ContextoTipo     string `json:"contexto_tipo"`
	CodigoAcademia   string `json:"codigo_academia,omitempty"`
	Ambiente         string `json:"ambiente"`
	ClientID         string `json:"client_id"`
	ClientSecret     string `json:"client_secret"`
	GPOPaymentMethod string `json:"gpo_payment_method"`
	REFPaymentMethod string `json:"ref_payment_method"`
}
```

`CredentialView` **não muda de campos** — continua exatamente como está hoje, incluindo `WebhookHeaderName string \`json:"webhook_header_name,omitempty"\``. Muda apenas de onde esse valor vem (ver seções 1.4 e 1.5): deixa de vir do banco e passa a ser sempre preenchido com a constante `WebhookHeaderName` da seção 1.3.

## 1.3 Constante de cabeçalho fixo e geração do segredo

Localizar:

```go
const defaultWebhookHeaderName = "X-API-Key"

// validHTTPHeaderName aceita apenas os caracteres permitidos em um
// field-name de cabeçalho HTTP por RFC 7230 (token). Case-insensitive por
// natureza do protocolo — http.Header.Get já canonicaliza antes de comparar.
func validHTTPHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}
	return true
}
```

Substituir por:

```go
// WebhookHeaderName é o único cabeçalho HTTP em que a AppyPay pode enviar o
// segredo de webhook. É fixo para toda a plataforma (todas as academias e o
// Spuri, em qualquer ambiente) — a AppyPay só oferece um único par nome/valor
// de cabeçalho no painel deles, então não há necessidade de variar por
// credencial. Prefixado com o nome do produto, seguindo a convenção comum de
// cabeçalhos customizados (ex.: X-GitHub-*, X-Stripe-*). Se o nome precisar
// mudar no futuro, basta trocar esta constante — nenhum outro código depende
// do valor literal.
const WebhookHeaderName = "X-Spuri-Webhook-Secret"

const (
	webhookSecretLength   = 15
	webhookSecretAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// generateWebhookSecret gera um segredo alfanumérico de 15 caracteres usando
// crypto/rand (seguro criptograficamente) com math/big para evitar víes de
// módulo sobre o alfabeto.
func generateWebhookSecret() (string, error) {
	out := make([]byte, webhookSecretLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(webhookSecretAlphabet))))
		if err != nil {
			return "", fmt.Errorf("erro ao gerar segredo de webhook: %w", err)
		}
		out[i] = webhookSecretAlphabet[n.Int64()]
	}
	return string(out), nil
}

// hasWebhookSecret verifica, sem decifrar nada, se uma credencial já possui
// um segredo de webhook gravado no cofre — usado por ConfigureCredential
// para decidir se deve gerar um segredo novo (apenas na primeira vez que uma
// credencial recebe um).
func (s *Service) hasWebhookSecret(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_segredos_appypay WHERE credential_id=$1 AND secret_type='webhook_secret')`, id).Scan(&exists)
	return exists, err
}
```

## 1.4 `ConfigureCredential` — função completa

Localizar a função inteira:

```go
func (s *Service) ConfigureCredential(ctx context.Context, id *uuid.UUID, in CredentialInput, userID, userType, ip string) (CredentialView, error) {
	if s.client == nil {
		return CredentialView{}, errors.New("serviço financeiro não inicializado")
	}
	in.ContextoTipo = strings.ToLower(strings.TrimSpace(in.ContextoTipo))
	in.Ambiente = strings.ToLower(strings.TrimSpace(in.Ambiente))
	if err := validContext(in.ContextoTipo, in.CodigoAcademia); err != nil {
		return CredentialView{}, err
	}
	if in.Ambiente == "" {
		in.Ambiente = AmbienteAtual()
	}
	if in.Ambiente != AmbienteAtual() {
		return CredentialView{}, errors.New("credenciais devem pertencer ao ambiente ativo")
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(in.ClientID) == "" || strings.TrimSpace(in.ClientSecret) == "" || !strings.HasPrefix(in.GPOPaymentMethod, "GPO_") || !strings.HasPrefix(in.REFPaymentMethod, "REF_") {
		return CredentialView{}, errors.New("credenciais AppyPay incompletas ou inválidas")
	}
	if in.WebhookSecret != "" {
		in.WebhookHeaderName = strings.TrimSpace(in.WebhookHeaderName)
		if in.WebhookHeaderName == "" {
			in.WebhookHeaderName = defaultWebhookHeaderName
		}
		if !validHTTPHeaderName(in.WebhookHeaderName) {
			return CredentialView{}, errors.New("webhook_header_name deve ser um nome de cabeçalho HTTP válido")
		}
	}
	credentialID := uuid.New()
	if id != nil {
		credentialID = *id
		if err := s.credentialBelongsToScope(ctx, credentialID, in.ContextoTipo, in.CodigoAcademia, in.Ambiente); err != nil {
			return CredentialView{}, err
		}
	} else if found, err := s.findCredentialID(ctx, in.ContextoTipo, in.CodigoAcademia, in.Ambiente); err == nil {
		credentialID = found
	}
	view := CredentialView{ID: credentialID, ContextoTipo: in.ContextoTipo, CodigoAcademia: in.CodigoAcademia, Ambiente: in.Ambiente, ClientIDMask: mask(in.ClientID), GPOPaymentMethodMask: mask(in.GPOPaymentMethod), REFPaymentMethodMask: mask(in.REFPaymentMethod), WebhookHeaderName: in.WebhookHeaderName, UpdatedAt: time.Now().UTC()}
	payload := map[string]any{"credential_id": credentialID.String(), "contexto_tipo": view.ContextoTipo, "codigo_academia": view.CodigoAcademia, "ambiente": view.Ambiente, "client_id_mask": view.ClientIDMask, "gpo_payment_method_mask": view.GPOPaymentMethodMask, "ref_payment_method_mask": view.REFPaymentMethodMask, "webhook_header_name": view.WebhookHeaderName, "updated_at": view.UpdatedAt}
	if err := s.record(ctx, credentialID, "CredenciaisAppyPayConfiguradas", payload, userID, userType, ip); err != nil {
		return CredentialView{}, err
	}
	if err := s.saveSecrets(ctx, credentialID, map[string]string{"client_id": in.ClientID, "client_secret": in.ClientSecret, "gpo_method": in.GPOPaymentMethod, "ref_method": in.REFPaymentMethod, "webhook_secret": in.WebhookSecret}); err != nil {
		return CredentialView{}, err
	}
	return view, nil
}
```

Substituir por:

```go
func (s *Service) ConfigureCredential(ctx context.Context, id *uuid.UUID, in CredentialInput, userID, userType, ip string) (CredentialView, string, error) {
	if s.client == nil {
		return CredentialView{}, "", errors.New("serviço financeiro não inicializado")
	}
	in.ContextoTipo = strings.ToLower(strings.TrimSpace(in.ContextoTipo))
	in.Ambiente = strings.ToLower(strings.TrimSpace(in.Ambiente))
	if err := validContext(in.ContextoTipo, in.CodigoAcademia); err != nil {
		return CredentialView{}, "", err
	}
	if in.Ambiente == "" {
		in.Ambiente = AmbienteAtual()
	}
	if in.Ambiente != AmbienteAtual() {
		return CredentialView{}, "", errors.New("credenciais devem pertencer ao ambiente ativo")
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(in.ClientID) == "" || strings.TrimSpace(in.ClientSecret) == "" || !strings.HasPrefix(in.GPOPaymentMethod, "GPO_") || !strings.HasPrefix(in.REFPaymentMethod, "REF_") {
		return CredentialView{}, "", errors.New("credenciais AppyPay incompletas ou inválidas")
	}
	credentialID := uuid.New()
	if id != nil {
		credentialID = *id
		if err := s.credentialBelongsToScope(ctx, credentialID, in.ContextoTipo, in.CodigoAcademia, in.Ambiente); err != nil {
			return CredentialView{}, "", err
		}
	} else if found, err := s.findCredentialID(ctx, in.ContextoTipo, in.CodigoAcademia, in.Ambiente); err == nil {
		credentialID = found
	}
	// O segredo de webhook só é gerado na primeira vez que esta credencial
	// recebe um — atualizações subsequentes nunca o tocam.
	hadSecret, err := s.hasWebhookSecret(ctx, credentialID)
	if err != nil {
		return CredentialView{}, "", err
	}
	secretsToSave := map[string]string{"client_id": in.ClientID, "client_secret": in.ClientSecret, "gpo_method": in.GPOPaymentMethod, "ref_method": in.REFPaymentMethod}
	var newWebhookSecret string
	if !hadSecret {
		newWebhookSecret, err = generateWebhookSecret()
		if err != nil {
			return CredentialView{}, "", err
		}
		secretsToSave["webhook_secret"] = newWebhookSecret
	}
	view := CredentialView{ID: credentialID, ContextoTipo: in.ContextoTipo, CodigoAcademia: in.CodigoAcademia, Ambiente: in.Ambiente, ClientIDMask: mask(in.ClientID), GPOPaymentMethodMask: mask(in.GPOPaymentMethod), REFPaymentMethodMask: mask(in.REFPaymentMethod), WebhookHeaderName: WebhookHeaderName, UpdatedAt: time.Now().UTC()}
	payload := map[string]any{"credential_id": credentialID.String(), "contexto_tipo": view.ContextoTipo, "codigo_academia": view.CodigoAcademia, "ambiente": view.Ambiente, "client_id_mask": view.ClientIDMask, "gpo_payment_method_mask": view.GPOPaymentMethodMask, "ref_payment_method_mask": view.REFPaymentMethodMask, "updated_at": view.UpdatedAt}
	if err := s.record(ctx, credentialID, "CredenciaisAppyPayConfiguradas", payload, userID, userType, ip); err != nil {
		return CredentialView{}, "", err
	}
	if err := s.saveSecrets(ctx, credentialID, secretsToSave); err != nil {
		return CredentialView{}, "", err
	}
	return view, newWebhookSecret, nil
}
```

Note as duas mudanças de fundo, além da assinatura: (a) o payload gravado no ledger **não inclui mais** a chave `"webhook_header_name"` — como o valor é sempre a constante global, não há mais motivo para persisti-lo por credencial; (b) `secretsToSave` só ganha a chave `"webhook_secret"` quando `hasWebhookSecret` indica que ainda não existe uma — em uma atualização de credencial já existente, essa chave nunca é passada a `saveSecrets`, então o valor cifrado já gravado permanece intocado (confira `saveSecrets`: ele pula qualquer chave com valor vazio).

## 1.5 `ListCredentials` — expor sempre a constante global

Localizar:

```go
	out := []CredentialView{}
	for rows.Next() {
		var v CredentialView
		var raw []byte
		if err := rows.Scan(&v.ID, &v.ContextoTipo, &v.CodigoAcademia, &v.Ambiente, &raw, &v.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &v)
		out = append(out, v)
	}
	return out, rows.Err()
```

Substituir por (uma linha nova depois do `Unmarshal`):

```go
	out := []CredentialView{}
	for rows.Next() {
		var v CredentialView
		var raw []byte
		if err := rows.Scan(&v.ID, &v.ContextoTipo, &v.CodigoAcademia, &v.Ambiente, &raw, &v.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &v)
		v.WebhookHeaderName = WebhookHeaderName
		out = append(out, v)
	}
	return out, rows.Err()
```

Isso é necessário porque, como o payload deixou de armazenar `webhook_header_name` (seção 1.4), o `Unmarshal` não preenche mais esse campo sozinho.

## 1.6 Três funções novas: `CredentialScope`, `WebhookSecret`, `RotateWebhookSecret`

Localizar (fim de `credentialBelongsToScope`, início de `callJSON`):

```go
func (s *Service) credentialBelongsToScope(ctx context.Context, id uuid.UUID, contexto, academia, ambiente string) error {
	var found uuid.UUID
	err := s.client.DB().QueryRowContext(ctx, `SELECT id FROM financeiro_credenciais_appypay WHERE id=$1 AND contexto_tipo=$2 AND codigo_academia IS NOT DISTINCT FROM NULLIF($3,'') AND ambiente=$4`, id, contexto, academia, ambiente).Scan(&found)
	if err != nil {
		return fmt.Errorf("%w: credencial AppyPay não encontrada no contexto", ErrNotFound)
	}
	return nil
}

func (s *Service) callJSON(ctx context.Context, cred credentialSecrets, method, path string, body any, async bool) (map[string]any, error) {
```

Substituir por (as três funções novas inseridas entre as duas existentes):

```go
func (s *Service) credentialBelongsToScope(ctx context.Context, id uuid.UUID, contexto, academia, ambiente string) error {
	var found uuid.UUID
	err := s.client.DB().QueryRowContext(ctx, `SELECT id FROM financeiro_credenciais_appypay WHERE id=$1 AND contexto_tipo=$2 AND codigo_academia IS NOT DISTINCT FROM NULLIF($3,'') AND ambiente=$4`, id, contexto, academia, ambiente).Scan(&found)
	if err != nil {
		return fmt.Errorf("%w: credencial AppyPay não encontrada no contexto", ErrNotFound)
	}
	return nil
}

// CredentialScope resolve o contexto (spuri/academia) e a academia dona de
// uma credencial pelo seu id — usado pelos handlers de segredo de webhook
// para reaplicar a mesma autorização (authorizeFinanceScope) das demais
// rotas de credenciais.
func (s *Service) CredentialScope(ctx context.Context, id uuid.UUID) (string, string, error) {
	var contexto, academia string
	if err := s.client.DB().QueryRowContext(ctx, `SELECT contexto_tipo, COALESCE(codigo_academia,'') FROM financeiro_credenciais_appypay WHERE id=$1`, id).Scan(&contexto, &academia); err != nil {
		return "", "", fmt.Errorf("%w: credencial AppyPay não encontrada", ErrNotFound)
	}
	return contexto, academia, nil
}

// WebhookSecret devolve o segredo de webhook atual em texto plano.
func (s *Service) WebhookSecret(ctx context.Context, id uuid.UUID) (string, error) {
	secrets, err := s.loadSecrets(ctx, id)
	if err != nil {
		return "", err
	}
	secret := secrets["webhook_secret"]
	if secret == "" {
		return "", fmt.Errorf("%w: credencial sem segredo de webhook", ErrNotFound)
	}
	return secret, nil
}

// RotateWebhookSecret gera um novo segredo de webhook, substitui o anterior
// no cofre e grava um evento de auditoria no ledger. A rotação é imediata:
// o segredo anterior deixa de autenticar assim que esta função retorna sem
// erro.
func (s *Service) RotateWebhookSecret(ctx context.Context, id uuid.UUID, userID, userType, ip string) (string, error) {
	secret, err := generateWebhookSecret()
	if err != nil {
		return "", err
	}
	if err := s.record(ctx, id, "SegredoWebhookAppyPayRotacionado", map[string]any{"credential_id": id.String()}, userID, userType, ip); err != nil {
		return "", err
	}
	if err := s.saveSecrets(ctx, id, map[string]string{"webhook_secret": secret}); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Service) callJSON(ctx context.Context, cred credentialSecrets, method, path string, body any, async bool) (map[string]any, error) {
```

## 1.7 `AuthenticateWebhook` — função completa

Localizar (comentário + struct `WebhookOwner` + função inteira):

```go
// AuthenticateWebhook aceita apenas o método suportado pela AppyPay: um único
// cabeçalho HTTP (nome configurável por credencial, webhook_header_name, com
// padrão "X-API-Key" quando a credencial não define um nome próprio) cujo
// valor é comparado ao webhook_secret configurado. A AppyPay confirmou por
// e-mail que o painel de configuração de webhooks só oferece um par nome/valor
// de cabeçalho HTTP — nunca query parameter, nunca corpo do POST, e nunca
// campos separados de utilizador/senha para Basic Auth. It never reveals
// which configured account matched.
type WebhookOwner struct {
	CredentialID                 uuid.UUID
	ContextoTipo, CodigoAcademia string
}

func (s *Service) AuthenticateWebhook(ctx context.Context, headers http.Header) (WebhookOwner, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id,payload FROM financeiro_credenciais_appypay WHERE COALESCE(payload->>'webhook_header_name','') <> ''`)
	if err != nil {
		return WebhookOwner{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return WebhookOwner{}, err
		}
		var meta struct {
			WebhookHeaderName string `json:"webhook_header_name"`
			ContextoTipo      string `json:"contexto_tipo"`
			CodigoAcademia    string `json:"codigo_academia"`
		}
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		secrets, err := s.loadSecrets(ctx, id)
		if err != nil {
			continue
		}
		headerName := strings.TrimSpace(meta.WebhookHeaderName)
		if headerName == "" {
			headerName = defaultWebhookHeaderName
		}
		candidate := headers.Get(headerName)
		if candidate != "" && constantTimeEqual(candidate, secrets["webhook_secret"]) {
			return WebhookOwner{CredentialID: id, ContextoTipo: meta.ContextoTipo, CodigoAcademia: meta.CodigoAcademia}, nil
		}
	}
	return WebhookOwner{}, errors.New("webhook não autenticado")
}
```

Substituir por:

```go
// AuthenticateWebhook aceita apenas o método suportado pela AppyPay: um único
// cabeçalho HTTP fixo para toda a plataforma (WebhookHeaderName) cujo valor é
// comparado, em tempo constante, ao webhook_secret gerado pelo servidor para
// cada credencial. A AppyPay confirmou por e-mail que o painel de
// configuração de webhooks só oferece um par nome/valor de cabeçalho HTTP —
// nunca query parameter, nunca corpo do POST, e nunca campos separados de
// utilizador/senha para Basic Auth. It never reveals which configured
// account matched.
type WebhookOwner struct {
	CredentialID                 uuid.UUID
	ContextoTipo, CodigoAcademia string
}

func (s *Service) AuthenticateWebhook(ctx context.Context, headers http.Header) (WebhookOwner, error) {
	candidate := headers.Get(WebhookHeaderName)
	if candidate == "" {
		return WebhookOwner{}, errors.New("webhook não autenticado")
	}
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id,payload FROM financeiro_credenciais_appypay`)
	if err != nil {
		return WebhookOwner{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return WebhookOwner{}, err
		}
		var meta struct {
			ContextoTipo   string `json:"contexto_tipo"`
			CodigoAcademia string `json:"codigo_academia"`
		}
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		secrets, err := s.loadSecrets(ctx, id)
		if err != nil {
			continue
		}
		if secrets["webhook_secret"] != "" && constantTimeEqual(candidate, secrets["webhook_secret"]) {
			return WebhookOwner{CredentialID: id, ContextoTipo: meta.ContextoTipo, CodigoAcademia: meta.CodigoAcademia}, nil
		}
	}
	return WebhookOwner{}, errors.New("webhook não autenticado")
}
```

**Por que a query SQL perde o filtro `WHERE COALESCE(payload->>'webhook_header_name','') <> ''`:** sem esse campo no payload (seção 1.4), não há mais um sinal não-sensível para pré-filtrar candidatos por SQL; o filtro relevante agora é "esta credencial tem `webhook_secret` no cofre", que só pode ser checado depois de `loadSecrets`. A checagem `candidate == ""` logo no início evita consultar o banco quando a requisição não enviou o cabeçalho — otimização segura, já que o valor do cabeçalho não depende de qual credencial está sendo avaliada.

## 1.8 Nenhuma outra função muda

`loadSecrets`, `saveSecrets`, `loadCredential`, `findCredentialID`, `token()`, `AcceptWebhook` e todo o resto do arquivo não são afetados.

---

# 2. `internal/handlers/financeiro_handlers.go`

## 2.1 Novo helper `credentialScopeAuthorized`

Localizar:

```go
func authorizeFinanceScope(c *gin.Context, context *string, academy *string) bool {
	_, t, own, ok := financeActor(c)
	if !ok {
		return false
	}
	if t == "academia" {
		if *context != "" && *context != finance.ContextoAcademia {
			return false
		}
		if *academy != "" && *academy != own {
			return false
		}
		*context = finance.ContextoAcademia
		*academy = own
		return true
	}
	return t == "admin" && financeAdminAllowed(c)
}
func financeError(c *gin.Context, err error) {
```

Substituir por:

```go
func authorizeFinanceScope(c *gin.Context, context *string, academy *string) bool {
	_, t, own, ok := financeActor(c)
	if !ok {
		return false
	}
	if t == "academia" {
		if *context != "" && *context != finance.ContextoAcademia {
			return false
		}
		if *academy != "" && *academy != own {
			return false
		}
		*context = finance.ContextoAcademia
		*academy = own
		return true
	}
	return t == "admin" && financeAdminAllowed(c)
}

// credentialScopeAuthorized resolve o contexto/academia dono de uma
// credencial AppyPay pelo seu id e reaplica authorizeFinanceScope — o mesmo
// mecanismo que já garante que uma academia só mexe nas próprias credenciais
// e que um admin precisa da permissão "fpp". Usado pelas rotas de consulta e
// rotação do segredo de webhook. Já escreve a resposta de erro (404 ou 403)
// no contexto quando retorna false.
func credentialScopeAuthorized(c *gin.Context, id uuid.UUID) bool {
	contexto, academia, err := FinanceiroService.CredentialScope(c.Request.Context(), id)
	if err != nil {
		financeError(c, err)
		return false
	}
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para esta credencial AppyPay")
		return false
	}
	return true
}

func financeError(c *gin.Context, err error) {
```

## 2.2 Novo tipo de resposta e `ConfigurarCredencialAppyPay`

Localizar:

```go
func ConfigurarCredencialAppyPay(c *gin.Context) {
	var in finance.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar estas credenciais")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.ConfigureCredential(c.Request.Context(), nil, in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
```

Substituir por:

```go
// CredencialAppyPayCriada é a resposta exclusiva de POST .../credenciais: é a
// única vez que o segredo de webhook aparece "de graça" numa resposta, fora
// do GET .../webhook-secret dedicado — porque é a única oportunidade em que o
// usuário ainda não tem como consultá-lo de outra forma.
type CredencialAppyPayCriada struct {
	finance.CredentialView
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

func ConfigurarCredencialAppyPay(c *gin.Context) {
	var in finance.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar estas credenciais")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, webhookSecret, err := FinanceiroService.ConfigureCredential(c.Request.Context(), nil, in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, CredencialAppyPayCriada{CredentialView: out, WebhookSecret: webhookSecret})
}
```

## 2.3 `AtualizarCredencialAppyPay`

Localizar:

```go
func AtualizarCredencialAppyPay(c *gin.Context) {
	idParam, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("id inválido"))
		return
	}
	var in finance.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar estas credenciais")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.ConfigureCredential(c.Request.Context(), &idParam, in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
```

Substituir por:

```go
func AtualizarCredencialAppyPay(c *gin.Context) {
	idParam, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("id inválido"))
		return
	}
	var in finance.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar estas credenciais")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	// O segundo retorno (segredo em texto plano) só vem preenchido quando a
	// credencial ainda não tinha nenhum segredo de webhook — não deveria
	// acontecer numa atualização de credencial já existente; se acontecer, o
	// usuário ainda pode recuperá-lo em seguida via GET .../webhook-secret.
	out, _, err := FinanceiroService.ConfigureCredential(c.Request.Context(), &idParam, in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
```

## 2.4 Dois handlers novos

Localizar:

```go
func ListarCredenciaisAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para consultar credenciais financeiras")
		return
	}
	out, err := FinanceiroService.ListCredentials(c.Request.Context(), contexto, academia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func CriarCobrancaAppyPay(c *gin.Context) {
```

Substituir por (dois handlers novos inseridos entre os dois existentes):

```go
func ListarCredenciaisAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para consultar credenciais financeiras")
		return
	}
	out, err := FinanceiroService.ListCredentials(c.Request.Context(), contexto, academia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// ConsultarSegredoWebhookAppyPay devolve o segredo de webhook atual em texto
// plano. Só o dono do contexto (a própria academia, ou admin com permissão
// "fpp") pode consultar.
func ConsultarSegredoWebhookAppyPay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("id inválido"))
		return
	}
	if !credentialScopeAuthorized(c, id) {
		return
	}
	secret, err := FinanceiroService.WebhookSecret(c.Request.Context(), id)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"webhook_secret": secret, "webhook_header_name": finance.WebhookHeaderName})
}

// RotacionarSegredoWebhookAppyPay gera um novo segredo de webhook,
// invalidando o anterior imediatamente. Mesma autorização de
// ConsultarSegredoWebhookAppyPay.
func RotacionarSegredoWebhookAppyPay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("id inválido"))
		return
	}
	if !credentialScopeAuthorized(c, id) {
		return
	}
	actorID, actorType, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	secret, err := FinanceiroService.RotateWebhookSecret(c.Request.Context(), id, actorID.String(), actorType, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"webhook_secret": secret, "webhook_header_name": finance.WebhookHeaderName})
}

func CriarCobrancaAppyPay(c *gin.Context) {
```

## 2.5 `ReceberWebhookAppyPay` e o resto do arquivo

Não mudam — `AuthenticateWebhook` já é chamado só com `c.Request.Header` (isso já foi corrigido pela tarefa 30).

---

# 3. `cmd/server/main.go`

Localizar:

```go
			financeiro.POST("/appypay/credenciais", handlers.ConfigurarCredencialAppyPay)
			financeiro.PUT("/appypay/credenciais/:id", handlers.AtualizarCredencialAppyPay)
			financeiro.GET("/appypay/credenciais", handlers.ListarCredenciaisAppyPay)
			financeiro.POST("/appypay/cobrancas", handlers.CriarCobrancaAppyPay)
```

Substituir por:

```go
			financeiro.POST("/appypay/credenciais", handlers.ConfigurarCredencialAppyPay)
			financeiro.PUT("/appypay/credenciais/:id", handlers.AtualizarCredencialAppyPay)
			financeiro.GET("/appypay/credenciais", handlers.ListarCredenciaisAppyPay)
			financeiro.GET("/appypay/credenciais/:id/webhook-secret", handlers.ConsultarSegredoWebhookAppyPay)
			financeiro.POST("/appypay/credenciais/:id/webhook-secret/rotacionar", handlers.RotacionarSegredoWebhookAppyPay)
			financeiro.POST("/appypay/cobrancas", handlers.CriarCobrancaAppyPay)
```

As duas rotas novas ficam no mesmo grupo `financeiro` já protegido por `middleware.RequireAcademiaOuAdmin()` — nenhum mecanismo de autorização novo foi criado no roteamento; a autorização fina por credencial acontece dentro dos handlers via `credentialScopeAuthorized` (seção 2.1).

---

# 4. `internal/db/safe_queries.go`

Localizar:

```go
	"CredenciaisAppyPayConfiguradas":                     true,
	"CobrancaAppyPaySolicitada":                          true,
```

Substituir por:

```go
	"CredenciaisAppyPayConfiguradas":                     true,
	"SegredoWebhookAppyPayRotacionado":                   true,
	"CobrancaAppyPaySolicitada":                          true,
```

(O alinhamento exato dos dois-pontos não importa — `gofmt`/o próprio Go não formatam mapas literais por alinhamento de coluna como fazem com structs; mantenha como está no arquivo ou deixe o editor ajustar.)

---

# 5. `internal/projections/financeiro_projection.go`

Localizar:

```go
	case "CredenciaisAppyPayConfiguradas":
		contexto, _ := v["contexto_tipo"].(string)
		academia, _ := v["codigo_academia"].(string)
		ambiente, _ := v["ambiente"].(string)
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_credenciais_appypay (id,contexto_tipo,codigo_academia,ambiente,payload,updated_at) VALUES ($1,$2,NULLIF($3,''),$4,$5,CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET contexto_tipo=EXCLUDED.contexto_tipo,codigo_academia=EXCLUDED.codigo_academia,ambiente=EXCLUDED.ambiente,payload=EXCLUDED.payload,updated_at=CURRENT_TIMESTAMP`, e.AggregateID, contexto, academia, ambiente, e.Payload)
		return err
	case "CobrancaAppyPaySolicitada", "CobrancaAppyPayCriada", "CobrancaAppyPayFalhou", "CobrancaAppyPayConsultada", "CobrancaAppyPayCancelada", "CobrancaAppyPayConflitoPosCancelamento", "QRCodeAppyPaySolicitado", "QRCodeAppyPayGerado", "QRCodeAppyPayFalhou":
```

Substituir por (novo `case` inserido entre os dois existentes):

```go
	case "CredenciaisAppyPayConfiguradas":
		contexto, _ := v["contexto_tipo"].(string)
		academia, _ := v["codigo_academia"].(string)
		ambiente, _ := v["ambiente"].(string)
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_credenciais_appypay (id,contexto_tipo,codigo_academia,ambiente,payload,updated_at) VALUES ($1,$2,NULLIF($3,''),$4,$5,CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET contexto_tipo=EXCLUDED.contexto_tipo,codigo_academia=EXCLUDED.codigo_academia,ambiente=EXCLUDED.ambiente,payload=EXCLUDED.payload,updated_at=CURRENT_TIMESTAMP`, e.AggregateID, contexto, academia, ambiente, e.Payload)
		return err
	case "SegredoWebhookAppyPayRotacionado":
		_, err := p.client.DB().Exec(`UPDATE financeiro_credenciais_appypay SET updated_at=CURRENT_TIMESTAMP WHERE id=$1`, e.AggregateID)
		return err
	case "CobrancaAppyPaySolicitada", "CobrancaAppyPayCriada", "CobrancaAppyPayFalhou", "CobrancaAppyPayConsultada", "CobrancaAppyPayCancelada", "CobrancaAppyPayConflitoPosCancelamento", "QRCodeAppyPaySolicitado", "QRCodeAppyPayGerado", "QRCodeAppyPayFalhou":
```

Esse `case` é deliberadamente mínimo: a rotação não muda `contexto_tipo`/`codigo_academia`/`ambiente`/máscaras (nada disso depende do segredo de webhook), então só há sentido em tocar `updated_at` da linha já existente.

---

# 6. `internal/finance/appypay_test.go`

Localizar:

```go
func TestValidHTTPHeaderName(t *testing.T) {
	for _, name := range []string{"X-API-Key", "Authorization", "X-Spuri-Webhook-Secret"} {
		if !validHTTPHeaderName(name) {
			t.Fatalf("nome de cabeçalho válido foi rejeitado: %q", name)
		}
	}
	for _, name := range []string{"", "X API Key", "X-API-Key:"} {
		if validHTTPHeaderName(name) {
			t.Fatalf("nome de cabeçalho inválido foi aceite: %q", name)
		}
	}
}

func TestAppyPayResourceConfig(t *testing.T) {
```

Substituir por:

```go
func TestWebhookHeaderNameIsFixedGlobalConstant(t *testing.T) {
	if WebhookHeaderName != "X-Spuri-Webhook-Secret" {
		t.Fatalf("WebhookHeaderName mudou de valor sem atualizar esta expectativa: %q", WebhookHeaderName)
	}
}

func TestGenerateWebhookSecretLengthAlphabetAndUniqueness(t *testing.T) {
	first, err := generateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != webhookSecretLength {
		t.Fatalf("segredo de webhook com tamanho %d, queria %d", len(first), webhookSecretLength)
	}
	for _, r := range first {
		if !strings.ContainsRune(webhookSecretAlphabet, r) {
			t.Fatalf("segredo de webhook contém caractere fora do alfabeto esperado: %q", r)
		}
	}
	second, err := generateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("duas chamadas a generateWebhookSecret produziram o mesmo valor")
	}
}

func TestAppyPayResourceConfig(t *testing.T) {
```

(`strings` já está importado neste arquivo — nenhum import novo necessário.)

---

# 7. `internal/finance/appypay_integration_test.go`

## 7.1 `configureIntegrationCredential`

Localizar:

```go
func configureIntegrationCredential(t *testing.T, service *Service, contexto, academia string) {
	t.Helper()
	_, err := service.ConfigureCredential(context.Background(), nil, CredentialInput{
		ContextoTipo:     contexto,
		CodigoAcademia:   academia,
		ClientID:         "integration-client",
		ClientSecret:     "integration-secret",
		GPOPaymentMethod: "GPO_INTEGRATION",
		REFPaymentMethod: "REF_INTEGRATION",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
}
```

Substituir por (só a linha do retorno muda, de dois para três valores):

```go
func configureIntegrationCredential(t *testing.T, service *Service, contexto, academia string) {
	t.Helper()
	_, _, err := service.ConfigureCredential(context.Background(), nil, CredentialInput{
		ContextoTipo:     contexto,
		CodigoAcademia:   academia,
		ClientID:         "integration-client",
		ClientSecret:     "integration-secret",
		GPOPaymentMethod: "GPO_INTEGRATION",
		REFPaymentMethod: "REF_INTEGRATION",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
}
```

## 7.2 Substituir o teste de autenticação de webhook

Localizar a função `TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials` inteira (do `func TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials(t *testing.T) {` até o `}` de fechamento — veja o arquivo atual, é o teste logo antes de `TestIntegrationCancelChargeAndLateSuccessConflict`) e substituir por:

```go
func TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()
	t.Setenv("ENV", "test")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")

	academia := "INT" + uuid.NewString()[:8]
	created, firstSecret, err := service.ConfigureCredential(ctx, nil, CredentialInput{
		ContextoTipo:     ContextoAcademia,
		CodigoAcademia:   academia,
		ClientID:         "client-webhook",
		ClientSecret:     "secret-webhook",
		GPOPaymentMethod: "GPO_WEBHOOK",
		REFPaymentMethod: "REF_WEBHOOK",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if firstSecret == "" {
		t.Fatal("nenhum segredo de webhook foi gerado na criação da credencial")
	}
	if created.WebhookHeaderName != WebhookHeaderName {
		t.Fatalf("view não expõe a constante global de cabeçalho: %q", created.WebhookHeaderName)
	}

	stored, err := service.WebhookSecret(ctx, created.ID)
	if err != nil || stored != firstSecret {
		t.Fatalf("WebhookSecret() = %q, %v; queria %q", stored, err, firstSecret)
	}

	contexto, resolvedAcademia, err := service.CredentialScope(ctx, created.ID)
	if err != nil || contexto != ContextoAcademia || resolvedAcademia != academia {
		t.Fatalf("CredentialScope() = %q, %q, %v", contexto, resolvedAcademia, err)
	}

	okHeaders := http.Header{}
	okHeaders.Set(WebhookHeaderName, firstSecret)
	owner, err := service.AuthenticateWebhook(ctx, okHeaders)
	if err != nil || owner.CredentialID != created.ID {
		t.Fatalf("segredo correto não autenticou: owner=%#v err=%v", owner, err)
	}

	wrongValueHeaders := http.Header{}
	wrongValueHeaders.Set(WebhookHeaderName, "valor-errado")
	if _, err = service.AuthenticateWebhook(ctx, wrongValueHeaders); err == nil {
		t.Fatal("valor de segredo errado autenticou")
	}

	wrongHeaderNameHeaders := http.Header{}
	wrongHeaderNameHeaders.Set("X-API-Key", firstSecret)
	if _, err = service.AuthenticateWebhook(ctx, wrongHeaderNameHeaders); err == nil {
		t.Fatal("cabeçalho fora do nome global autenticou")
	}

	if _, err = service.AuthenticateWebhook(ctx, http.Header{}); err == nil {
		t.Fatal("requisição sem nenhum cabeçalho autenticou")
	}

	updated, secondSecret, err := service.ConfigureCredential(ctx, &created.ID, CredentialInput{
		ContextoTipo:     ContextoAcademia,
		CodigoAcademia:   academia,
		ClientID:         "client-webhook-atualizado",
		ClientSecret:     "secret-webhook-atualizado",
		GPOPaymentMethod: "GPO_WEBHOOK",
		REFPaymentMethod: "REF_WEBHOOK",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if secondSecret != "" {
		t.Fatalf("atualização de credencial já existente regenerou o segredo: %q", secondSecret)
	}
	if stored, err = service.WebhookSecret(ctx, updated.ID); err != nil || stored != firstSecret {
		t.Fatalf("atualização alterou o segredo existente: %q, %v", stored, err)
	}

	rotated, err := service.RotateWebhookSecret(ctx, created.ID, "integration-test", "sistema", "127.0.0.1")
	if err != nil || rotated == "" || rotated == firstSecret {
		t.Fatalf("rotação inválida: %q, %v", rotated, err)
	}
	if stored, err = service.WebhookSecret(ctx, created.ID); err != nil || stored != rotated {
		t.Fatalf("segredo pós-rotação = %q, %v; queria %q", stored, err, rotated)
	}
	oldHeaders := http.Header{}
	oldHeaders.Set(WebhookHeaderName, firstSecret)
	if _, err = service.AuthenticateWebhook(ctx, oldHeaders); err == nil {
		t.Fatal("segredo antigo continuou autenticando após rotação")
	}
	newHeaders := http.Header{}
	newHeaders.Set(WebhookHeaderName, rotated)
	if owner, err = service.AuthenticateWebhook(ctx, newHeaders); err != nil || owner.CredentialID != created.ID {
		t.Fatalf("segredo novo pós-rotação não autenticou: owner=%#v err=%v", owner, err)
	}

	var rotationEvents int
	if err = client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='SegredoWebhookAppyPayRotacionado'`, created.ID).Scan(&rotationEvents); err != nil {
		t.Fatal(err)
	}
	if rotationEvents != 1 {
		t.Fatalf("eventos de rotação registrados = %d, queria 1", rotationEvents)
	}
}
```

O que este teste cobre, em ordem: segredo gerado só na criação; a view sempre mostra a constante global de cabeçalho; `WebhookSecret()` devolve o valor atual; `CredentialScope()` resolve contexto/academia corretamente; valor certo autentica; valor errado não autentica; nome de cabeçalho errado não autentica; requisição sem nenhum cabeçalho não autentica; atualização de credencial já existente **não** regenera o segredo; `RotateWebhookSecret` gera um valor novo e diferente do anterior; o segredo antigo para de autenticar imediatamente após a rotação; o segredo novo autentica; e o evento `SegredoWebhookAppyPayRotacionado` é gravado exatamente uma vez no ledger.

## 7.3 Nenhum outro teste deste arquivo muda

`TestIntegrationAcceptWebhookIsIdempotent`, `TestIntegrationMatriculaPagamentoFixaValorImpedeDuplicidadeECancelaEmCascata`, `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito` e `TestIntegrationCancelChargeAndLateSuccessConflict` não usam `WebhookSecret`/`WebhookHeaderName` e não são afetados por esta mudança (mas veja o "Passo obrigatório de investigação" abaixo — alguns deles aparecem na lista de falhas a isolar).

---

# 8. `internal/handlers/financeiro_handlers_integration_test.go`

Localizar (dentro de `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula`):

```go
	_, err := service.ConfigureCredential(context.Background(), nil, finance.CredentialInput{
		ContextoTipo:     finance.ContextoAcademia,
		CodigoAcademia:   academia,
		ClientID:         "integration-client",
		ClientSecret:     "integration-secret",
		GPOPaymentMethod: "GPO_INTEGRATION",
		REFPaymentMethod: "REF_INTEGRATION",
		WebhookSecret:    "webhook-secret-" + codigo,
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
```

Substituir por:

```go
	_, webhookSecret, err := service.ConfigureCredential(context.Background(), nil, finance.CredentialInput{
		ContextoTipo:     finance.ContextoAcademia,
		CodigoAcademia:   academia,
		ClientID:         "integration-client",
		ClientSecret:     "integration-secret",
		GPOPaymentMethod: "GPO_INTEGRATION",
		REFPaymentMethod: "REF_INTEGRATION",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
```

E, mais abaixo, localizar:

```go
		req.Header.Set("X-API-Key", "webhook-secret-"+codigo)
```

Substituir por:

```go
		req.Header.Set(finance.WebhookHeaderName, webhookSecret)
```

O resto da função (`seedAcademiaParaMatriculaWebhook`, `seedSolicitacaoMatriculaPendenteComLedger`, o router de teste, as asserções de status pós-webhook) não muda.

---

# 9. Documentação

## 9.1 `Documentação da API.md`

**Bullet sobre segredos nunca devolvidos (linha ~7092):**

Localizar:

```
- Segredos AppyPay (`client_secret`, credenciais de webhook, métodos de pagamento sensíveis) nunca são devolvidos em resposta; a API retorna apenas máscaras e metadados.
```

Substituir por:

```
- Segredos AppyPay (`client_secret`, métodos de pagamento sensíveis) nunca são devolvidos em resposta; a API retorna apenas máscaras e metadados. A única exceção deliberada é o segredo de webhook (`webhook_secret`): como é gerado pelo servidor e o usuário precisa colá-lo no painel da AppyPay, ele é devolvido em texto plano apenas na criação da credencial (seção 19.1) e nas rotas dedicadas de consulta/rotação (seções 19.10 e 19.11) — nunca em `PUT`, listagem ou qualquer outra resposta.
```

**Bullet sobre autenticação de webhooks (linha ~7097):**

Localizar:

```
- Os webhooks são públicos por necessidade do gateway, mas autenticados pelo segredo de webhook cadastrado na credencial do contexto, enviado pela AppyPay num cabeçalho HTTP configurável (`webhook_header_name`, padrão `X-API-Key`). Eventos aceitos ou duplicados respondem `200` e são tratados de forma idempotente pelo identificador do evento.
```

Substituir por:

```
- Os webhooks são públicos por necessidade do gateway, mas autenticados pelo segredo de webhook gerado automaticamente na criação da credencial, enviado pela AppyPay num único cabeçalho HTTP fixo para toda a plataforma (`webhook_header_name`, sempre `X-Spuri-Webhook-Secret`). Eventos aceitos ou duplicados respondem `200` e são tratados de forma idempotente pelo identificador do evento.
```

**Tabela de rotas (linha ~7104):**

Localizar:

```
| `GET` | `/financeiro/appypay/credenciais` | Lista credenciais mascaradas por contexto autorizado. |
| `POST` | `/financeiro/appypay/cobrancas` | Cria cobrança AppyPay GPO ou REF genérica. |
```

Substituir por:

```
| `GET` | `/financeiro/appypay/credenciais` | Lista credenciais mascaradas por contexto autorizado. |
| `GET` | `/financeiro/appypay/credenciais/:id/webhook-secret` | Consulta o segredo de webhook atual (texto plano) de uma credencial. |
| `POST` | `/financeiro/appypay/credenciais/:id/webhook-secret/rotacionar` | Gera um novo segredo de webhook, invalidando o anterior. |
| `POST` | `/financeiro/appypay/cobrancas` | Cria cobrança AppyPay GPO ou REF genérica. |
```

**Seção 19.1 — Request JSON:**

Localizar:

```json
{
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "client_id": "appy-client-id",
  "client_secret": "appy-client-secret",
  "gpo_payment_method": "GPO_METHOD_ID",
  "ref_payment_method": "REF_METHOD_ID",
  "webhook_secret": "segredo-do-webhook",
  "webhook_header_name": "X-API-Key"
}
```

Substituir por:

```json
{
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "client_id": "appy-client-id",
  "client_secret": "appy-client-secret",
  "gpo_payment_method": "GPO_METHOD_ID",
  "ref_payment_method": "REF_METHOD_ID"
}
```

**Seção 19.1 — Response 201:**

Localizar:

```json
{
  "id": "2f0f8d8f-27a1-4b2d-9a70-8e26d208f7e4",
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "ambiente": "test",
  "client_id_mask": "appy**********id",
  "gpo_payment_method_mask": "GPO_**********_ID",
  "ref_payment_method_mask": "REF_**********_ID",
  "webhook_header_name": "X-API-Key",
  "updated_at": "2026-08-08T12:00:00Z"
}
```

Substituir por:

```json
{
  "id": "2f0f8d8f-27a1-4b2d-9a70-8e26d208f7e4",
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "ambiente": "test",
  "client_id_mask": "appy**********id",
  "gpo_payment_method_mask": "GPO_**********_ID",
  "ref_payment_method_mask": "REF_**********_ID",
  "webhook_header_name": "X-Spuri-Webhook-Secret",
  "webhook_secret": "aB3xY9kLm2PqRtZ",
  "updated_at": "2026-08-08T12:00:00Z"
}
```

**Seção 19.1 — Regras de negócio:**

Localizar:

```
- `webhook_secret` é opcional: quando omitido, a credencial não autentica nenhum webhook (as rotas continuam recebendo eventos, mas nada é aceito). Quando informado, `webhook_header_name` pode ser enviado para escolher o nome do cabeçalho HTTP em que a AppyPay deve mandar esse segredo; o padrão é `X-API-Key` quando omitido. O nome deve corresponder exatamente ao configurado no campo "nome" do painel de webhooks da AppyPay — a AppyPay confirmou que esse painel só oferece um único par nome/valor de cabeçalho HTTP, por isso não existe mais um modo de autenticação alternativo (ex.: Basic Auth) para o webhook.
```

Substituir por:

```
- O segredo de webhook não é enviado pelo cliente: o backend gera automaticamente um valor alfanumérico de 15 caracteres na criação da credencial e devolve-o em texto plano apenas nesta resposta (campo `webhook_secret`), para o usuário colar no painel de webhooks da AppyPay. O nome do cabeçalho HTTP (`webhook_header_name`) é fixo para toda a plataforma (`X-Spuri-Webhook-Secret`) e não é mais configurável por credencial — a AppyPay confirmou que o painel deles só oferece um único par nome/valor de cabeçalho HTTP, por isso também não existe modo de autenticação alternativo (ex.: Basic Auth) para o webhook.
```

**Seção 19.2 — Escopo da rota:**

Localizar:

```
**Escopo da rota:** atualização/substituição completa da credencial identificada por `:id`. Use para rotação de `client_secret`, troca de métodos GPO/REF ou alteração do modo de autenticação de webhook.
```

Substituir por:

```
**Escopo da rota:** atualização/substituição completa dos dados de conta AppyPay (`client_id`, `client_secret`, métodos GPO/REF) da credencial identificada por `:id`. Não altera o segredo de webhook — para isso, use `POST .../webhook-secret/rotacionar` (seção 19.11).
```

**Seção 19.2 — Regras de negócio:**

Localizar:

```
- A atualização é sempre uma substituição completa: reenvie `webhook_header_name` para preservar um nome customizado, pois sua omissão restaura o padrão `X-API-Key`. `resource` continua fora do corpo da requisição e vem de `APPYPAY_RESOURCE`.
```

Substituir por:

```
- A atualização é sempre uma substituição completa dos dados de conta (`client_id`, `client_secret`, métodos GPO/REF); o segredo de webhook nunca é alterado por este endpoint — ele só muda por rotação explícita (`POST .../webhook-secret/rotacionar`, seção 19.11). `resource` continua fora do corpo da requisição e vem de `APPYPAY_RESOURCE`.
```

**Seção 19.3 — Response 200:**

Localizar:

```json
[
  {
    "id": "2f0f8d8f-27a1-4b2d-9a70-8e26d208f7e4",
    "contexto_tipo": "academia",
    "codigo_academia": "LDA20261",
    "ambiente": "test",
    "client_id_mask": "appy**********id",
    "gpo_payment_method_mask": "GPO_**********_ID",
    "ref_payment_method_mask": "REF_**********_ID",
    "webhook_header_name": "X-API-Key",
    "updated_at": "2026-08-08T12:00:00Z"
  }
]
```

Substituir por:

```json
[
  {
    "id": "2f0f8d8f-27a1-4b2d-9a70-8e26d208f7e4",
    "contexto_tipo": "academia",
    "codigo_academia": "LDA20261",
    "ambiente": "test",
    "client_id_mask": "appy**********id",
    "gpo_payment_method_mask": "GPO_**********_ID",
    "ref_payment_method_mask": "REF_**********_ID",
    "webhook_header_name": "X-Spuri-Webhook-Secret",
    "updated_at": "2026-08-08T12:00:00Z"
  }
]
```

**Seção 19.3 — Regras de negócio:**

Localizar:

```
- Máscaras não devem ser usadas como segredos pelo cliente; qualquer rotação exige `PUT` com os segredos reais.
```

Substituir por:

```
- Máscaras não devem ser usadas como segredos pelo cliente; rotação de `client_secret`/métodos exige `PUT` com os segredos reais. O segredo de webhook tem rotação própria (`POST .../credenciais/:id/webhook-secret/rotacionar`, seção 19.11) e nunca aparece mascarado aqui — só em texto pleno pelas rotas dedicadas (seções 19.1, 19.10 e 19.11).
```

**Seção 19.8 (POST /webhooks/appypay/gpo) — Proteção:**

Localizar:

```
**Proteção:** pública no roteamento HTTP, autenticada pelo segredo de webhook cadastrado na credencial, enviado no cabeçalho HTTP configurado em `webhook_header_name` (padrão `X-API-Key`). A AppyPay confirmou que a autenticação do webhook sempre viaja por cabeçalho HTTP, nunca por query parameter — por isso este é o único método suportado.
```

Substituir por:

```
**Proteção:** pública no roteamento HTTP, autenticada pelo segredo de webhook gerado pelo servidor, enviado no único cabeçalho HTTP fixo da plataforma (`webhook_header_name`, sempre `X-Spuri-Webhook-Secret`). A AppyPay confirmou que a autenticação do webhook sempre viaja por cabeçalho HTTP, nunca por query parameter — por isso este é o único método suportado.
```

**Seção 19.9 (POST /webhooks/appypay/ref) — Proteção:**

Localizar:

```
**Proteção:** igual ao webhook GPO: autenticação pelo segredo de webhook no cabeçalho HTTP configurado pela credencial (`webhook_header_name`, padrão `X-API-Key`). A AppyPay confirmou que essa autenticação sempre viaja por cabeçalho HTTP, nunca por query parameter.
```

Substituir por:

```
**Proteção:** igual ao webhook GPO: autenticação pelo segredo de webhook no único cabeçalho HTTP fixo da plataforma (`webhook_header_name`, sempre `X-Spuri-Webhook-Secret`). A AppyPay confirmou que essa autenticação sempre viaja por cabeçalho HTTP, nunca por query parameter.
```

**Duas seções novas — inserir depois de "#### 19.9 POST /webhooks/appypay/ref" (depois das suas "Regras de negócio") e antes de "**Erros comuns das rotas autenticadas:**":**

```markdown
#### 19.10 GET /financeiro/appypay/credenciais/:id/webhook-secret

**Escopo da rota:** consulta do segredo de webhook atual, em texto plano, de uma credencial já cadastrada. Existe porque o segredo é gerado pelo servidor — o usuário precisa desta rota (ou da resposta de criação, seção 19.1) para saber o que colar no painel da AppyPay.

**Proteção:** autenticado + dono do contexto da credencial (a própria academia, ou admin com permissão `fpp`), resolvido a partir do `id` da credencial.

**Response 200:**

```json
{
  "webhook_secret": "aB3xY9kLm2PqRtZ",
  "webhook_header_name": "X-Spuri-Webhook-Secret"
}
```

**Regras de negócio:**

- `webhook_header_name` devolvido aqui é sempre a mesma constante fixa da plataforma; existe no corpo apenas para o cliente não precisar hardcodar o valor.
- `id` inexistente ou fora do contexto autorizado retorna `404`; falta de permissão retorna `403`.

#### 19.11 POST /financeiro/appypay/credenciais/:id/webhook-secret/rotacionar

**Escopo da rota:** gera um novo segredo de webhook para a credencial, invalidando o anterior imediatamente.

**Proteção:** igual à seção 19.10.

**Request JSON:** corpo vazio.

**Response 200:** igual à seção 19.10, com o novo valor de `webhook_secret`.

**Regras de negócio:**

- A rotação é imediata e definitiva: o segredo anterior deixa de autenticar assim que a rotação é gravada, mesmo que o painel da AppyPay ainda não tenha sido atualizado com o novo valor — trate como uma operação disruptiva, não como um agendamento.
- Cada rotação grava um evento próprio no ledger (`SegredoWebhookAppyPayRotacionado`) para auditoria, sem expor o valor do segredo no payload do evento.
```

## 9.2 `docs/Parceiros e integrações/AppyPay Documentação.md`

Localizar (nota "Webhooks (transacional)", requisitos do endpoint):

```
- Suportar autenticação do lado do Spuri via um único cabeçalho HTTP configurável (nome + valor) — a AppyPay confirmou por e-mail que o painel de configuração de webhooks só oferece esse par nome/valor, sem campos separados de utilizador/senha; por isso o Spuri não suporta mais um modo alternativo de Basic Auth para webhook (removido — ver tarefa de simplificação do método de autenticação de webhook).
```

Substituir por:

```
- Suportar autenticação do lado do Spuri via um único cabeçalho HTTP com nome fixo para toda a plataforma (`X-Spuri-Webhook-Secret`) e valor gerado automaticamente pelo servidor na criação de cada credencial — a AppyPay confirmou por e-mail que o painel de configuração de webhooks só oferece esse par nome/valor, sem campos separados de utilizador/senha; por isso o Spuri não suporta um modo alternativo de Basic Auth para webhook, nem permite escolher o nome do cabeçalho por credencial (ver `docs/Tarefas feitas/30 - Simplificar autenticação de webhook AppyPay para um único método.md` e `docs/Tarefas feitas/40 - Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md`).
```

Não alterar a linha próxima a 1953 ("For authorization we support **Basic Auth** ... and **API Key**") — é citação literal da documentação original da AppyPay, não afirmação do Spuri.

---

# Fora de escopo

- `go.mod`/`go.sum` — não precisam de alteração real. Se aparecer diff neles ao validar localmente, não inclua no commit.
- O frontend (`spuripainel`) tem tarefa própria e separada; não tocar neste repositório a partir daqui.
- `docs/Lista de Tarefas/00 - Índice e ordem de implementação.md` — não referencia as tarefas 24/30/39/40 e já está fora de sincronia com a numeração real das tarefas deste módulo; não é escopo desta tarefa mantê-lo atualizado.
- Bug pré-existente de numeração duplicada no `Documentação da API.md`: antes desta tarefa já existiam duas seções "19.8" (`POST /financeiro/appypay/cobrancas/:id/cancelar` e `POST /webhooks/appypay/gpo`). Esta tarefa **não corrige** essa duplicidade — as seções novas foram numeradas 19.10/19.11, depois da 19.9, para não exigir renumerar todo o restante do documento. Se quiser corrigir a duplicidade, trate como tarefa própria.
- Qualquer alteração em `CreateCharge`, `CreateGPOQRCode`, `ConsultCharge`, `CancelCharge`, `AcceptWebhook`, cifragem/decifragem de segredos, `appyPayResource`/`ValidateAppyPayResourceConfig`, ou qualquer outra função de `internal/finance/appypay.go` não listada explicitamente nas seções 1–8.
- Correção definitiva de qualquer falha de teste de Mensalidade/CancelCharge não relacionada a AppyPay — o passo obrigatório abaixo pede para **isolar a causa e registrar**, não para corrigir, a menos que a investigação confirme que a causa é esta própria mudança (ver critério no passo 2 abaixo).

---

# Passo obrigatório de investigação (antes de mover esta tarefa para "Tarefas feitas")

Uma sessão anterior deste orquestrador (Claude) já implementou e validou este mesmo desenho, num clone de trabalho que não foi preservado entre conversas (ver `docs/Lista de Tarefas/Handoff — Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md` para o relato completo). Nessa validação, `go build ./...` e `go vet ./...` passaram limpos, e o teste de integração dedicado passou isoladamente. Ao rodar a suíte completa dos pacotes `internal/finance` e `internal/handlers`, porém, apareceram falhas adicionais — e uma comparação com um clone limpo do `main`, sem nenhuma mudança desta tarefa, mostrou que a maioria delas **já falha hoje**, sem relação com esta mudança:

- Falham também no `main` puro, sem qualquer alteração: `internal/finance` → `TestIntegrationMatriculaPagamentoFixaValorImpedeDuplicidadeECancelaEmCascata`, `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito`, `TestIntegrationMensalidadeAnularEReativar`; `internal/handlers` → `TestIntegrationBuscaPublicaMatriculaExigeDoisCamposENaoExibePagamento`.
- Com as mudanças desta tarefa aplicadas, a lista de falhas em `internal/finance` cresce e passa a incluir também várias de Mensalidade (`TestIntegrationMensalidadeResolvePrecoHistorico`, `TestIntegrationMensalidadePrimeiraConfiguracaoRetroageSemReescreverHistorico`, `TestIntegrationMensalidadeMantemAnoAcademicoHistorico`, `TestIntegrationMensalidadeMantemCursoHistorico`, `TestIntegrationMensalidadeMantemAcademiaHistoricaAposTransferencia`, `TestIntegrationMensalidadeMesInicioEValidadePorAno`, `TestIntegrationMensalidadeConsultaRespeitaAcademia`) e `TestIntegrationCancelChargeAndLateSuccessConflict` — nenhuma delas toca diretamente em webhook/AppyPay.

Esta sessão de orquestração (a que escreveu este documento) **não teve acesso a Go nem a PostgreSQL** para reproduzir isso por conta própria — só a leitura do repositório via `codeload.github.com`. Por isso, esta investigação passa a ser sua responsabilidade (Codex) antes de considerar a tarefa concluída:

1. Aplique as seções 1–9 deste documento sobre uma cópia real do repositório.
2. Rode `RUN_POSTGRES_INTEGRATION=1 go test ./internal/finance/... -run TestIntegrationMensalidade -v` contra um banco **recém-criado** (não reaproveitado de execução anterior), e depois o mesmo comando contra um clone limpo do `main` sem as mudanças desta tarefa, também com banco recém-criado a cada rodada.
   - Se as falhas de Mensalidade só aparecerem quando testadas junto de outros testes do mesmo pacote (não isoladas com `-run`), a causa é poluição/ordem de execução compartilhando o mesmo banco entre testes — um problema pré-existente do pacote, não desta tarefa (padrão já visto antes, na depuração da tarefa 24). Registre isso na Nota de validação e siga em frente.
   - Se as falhas persistirem mesmo isoladas com `-run`, **e** só aparecerem com as mudanças desta tarefa aplicadas (não no `main` puro), a causa é real e passa a fazer parte do escopo desta tarefa: investigue a causa raiz e corrija antes de finalizar.
3. Independentemente do resultado do passo 2, rode a suíte completa de `internal/finance` e `internal/handlers` uma vez em cada lado (`main` puro vs. com as mudanças desta tarefa), cada uma contra um banco recém-criado, e liste exatamente quais testes falham em cada lado. Se a lista de falhas do `main` puro não estiver vazia, isso é uma informação nova para o Fredy, **separada desta tarefa** — registre a lista completa na Nota de validação abaixo para que ele seja avisado, mas não tente corrigi-las aqui (fora de escopo).
4. Preste atenção especial a `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` (`internal/handlers/financeiro_handlers_integration_test.go`, ajustado na seção 8): confirme que ele passa depois do ajuste desta tarefa. Se, antes do seu ajuste, ele já falhava no `main` puro por um motivo não relacionado a webhook/AppyPay, registre esse achado também — é o tipo de anomalia que a sessão anterior de orquestração deixou sinalizada sem explicação definitiva.

---

# Critérios de aceite

1. `internal/finance/appypay.go` bate exatamente com as seções 1.1–1.7; `validHTTPHeaderName`, `defaultWebhookHeaderName` e os campos `WebhookSecret`/`WebhookHeaderName` de `CredentialInput` não existem mais no arquivo. A constante exportada `WebhookHeaderName` e o campo `CredentialView.WebhookHeaderName` continuam existindo (isso é esperado).
2. `internal/handlers/financeiro_handlers.go` bate com a seção 2.
3. `cmd/server/main.go` bate com a seção 3.
4. `internal/db/safe_queries.go` bate com a seção 4.
5. `internal/projections/financeiro_projection.go` bate com a seção 5.
6. `internal/finance/appypay_test.go` bate com a seção 6.
7. `internal/finance/appypay_integration_test.go` bate com a seção 7.
8. `internal/handlers/financeiro_handlers_integration_test.go` bate com a seção 8.
9. `Documentação da API.md` e `docs/Parceiros e integrações/AppyPay Documentação.md` batem com a seção 9.
10. `gofmt -l .` não lista nenhum arquivo (formatação limpa).
11. `go build ./...` passa sem erro no repositório inteiro.
12. `go vet ./...` passa sem erro no repositório inteiro.
13. `go test ./...` passa sem `RUN_POSTGRES_INTEGRATION=1`.
14. Com `RUN_POSTGRES_INTEGRATION=1` e um banco **recém-criado**, `go test ./internal/finance/... -run TestIntegration -v` e `go test ./internal/handlers/... -run TestIntegration -v` passam, incluindo os cenários novos de webhook (seções 7.2 e 8) — exceto pelas falhas pré-existentes já mapeadas e investigadas no passo obrigatório acima, que devem estar documentadas na Nota de validação, não escondidas.
15. `grep -rn "validHTTPHeaderName\|defaultWebhookHeaderName" --include="*.go" .` não retorna nenhuma linha.
16. `grep -rn "webhook_auth_type\|WebhookAuthType\|webhook_username\|WebhookUsername" --include="*.go" .` não retorna nenhuma linha (confirma que a limpeza da tarefa 30 continua válida).
17. `grep -n "X-API-Key" "Documentação da API.md" "docs/Parceiros e integrações/AppyPay Documentação.md"` não retorna nenhuma linha associada à autenticação do webhook do Spuri (a única ocorrência aceitável seria uma citação literal de exemplo genérico da AppyPay não relacionada à nossa configuração — inspecione manualmente qualquer resultado).
18. O "Passo obrigatório de investigação" foi executado por completo e seu resultado está registrado na Nota de validação abaixo.

## Nota de validação

Execução Codex em 2026-08-16 (implementação do código):

- `gofmt -l .`: passou, sem listar arquivos.
- `go build ./...`: passou.
- `go vet ./...`: passou.
- `go test ./...`: passou sem `RUN_POSTGRES_INTEGRATION=1`.
- Critério de limpeza `rg -n "validHTTPHeaderName|defaultWebhookHeaderName" --glob '*.go' .`: passou, sem resultados.
- Critério de limpeza `rg -n "webhook_auth_type|WebhookAuthType|webhook_username|WebhookUsername" --glob '*.go' .`: passou, sem resultados.
- Inspeção de `X-API-Key` em `Documentação da API.md` e `docs/Parceiros e integrações/AppyPay Documentação.md`: passou, sem resultados nas duas documentações.

Auditoria de fechamento (Claude, 16 de agosto de 2026, tarefa 46) — passo obrigatório de investigação e
validação de integração, ambiente real (Go 1.24, PostgreSQL 16, repositório clonado do `main`, commit
`e29ea77`):

- Todas as 9 seções do documento conferidas linha a linha contra o código real do repositório: batem
  exatamente. Os 18 critérios de aceite passam.
- Com `RUN_POSTGRES_INTEGRATION=1` e banco recriado do zero, a suíte completa de `internal/finance` e de
  `internal/handlers` foi executada tanto no `main` com esta tarefa aplicada quanto num checkout do commit
  imediatamente anterior a ela (`e8cfffe`) — **nas duas execuções, 100% dos testes passaram, sem nenhum
  `FAIL`**, incluindo `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` e todos os cenários de
  mensalidade. Não há nenhuma falha pré-existente no `main` para reportar separadamente ao Fredy.
- `TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation` e as demais suítes de webhook passam
  isoladamente e dentro da suíte completa, 5 execuções seguidas contra banco recriado a cada vez.
- Reconfirmado neste ambiente (Codex) na execução do Passo 1 da tarefa 46: não reproduzido neste ambiente (Codex) — sem acesso a PostgreSQL: `psql` ausente, `apt-get install
postgresql` falhou com 403 Forbidden nos repositórios, sem Docker disponível (`docker: command not
found`). A subseção 1.A (independente de banco) foi confirmada com sucesso: gofmt/build/vet/test/greps
limpos. A validação de integração com PostgreSQL real, incluindo a comparação main puro vs. branco com
esta tarefa aplicada, permanece a executada e documentada pela auditoria acima (Claude, ambiente
separado) — aceita como evidência suficiente dado que este ambiente de execução não tem como reproduzi-la.
