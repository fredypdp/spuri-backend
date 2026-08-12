---
criado: 2026-08-12 00:00
origem: decisão do Fredy — eliminar o modo "Basic Auth" de webhook, mantendo apenas o método que a AppyPay de fato oferece (um único cabeçalho HTTP configurável)
status: feito
---

# Simplificar autenticação de webhook AppyPay para um único método (feito)

## Prompt recomendado para executar a atualização

Implemente, no repositório `spuri-backend`, exatamente o código descrito neste documento em `internal/finance/appypay.go`, `internal/handlers/financeiro_handlers.go`, `internal/finance/appypay_integration_test.go`, `Documentação da API.md` e `docs/Parceiros e integrações/AppyPay Documentação.md`. Todo o código já foi implementado e validado por mim (Claude, orquestrador) num clone real do repositório, com build, vet e suíte de testes completa (incluindo os testes de integração contra PostgreSQL) — use exatamente como está aqui. Confirmei com o Fredy que **não existe hoje nenhuma credencial AppyPay configurada em nenhum ambiente**, então esta é uma remoção sem risco de migração.

## Contexto

A tarefa 24 (já concluída e depurada duas vezes) deixou o webhook AppyPay com dois modos de autenticação possíveis por credencial (`webhook_auth_type`: `"basic"` ou `"api_key"`). Ao explicar a diferença entre os dois para o Fredy, ficou claro que isso é desnecessário:

- **`api_key`** mapeia exatamente no que a AppyPay oferece: o painel de webhooks deles só tem um campo de **nome** de cabeçalho e um campo de **valor**. Você põe o nome (ex.: `X-API-Key`) e o valor (o segredo), e pronto.
- **`basic`** exigiria simular `Authorization: Basic <base64(usuário:senha)>` usando esse mesmo único par nome/valor — ou seja, alguém teria que calcular o base64 de "usuário:senha" por fora e colar como "valor", com "nome" = `Authorization`. A própria documentação da AppyPay (`docs/Parceiros e integrações/AppyPay Documentação.md`, linha 1953) diz que eles suportam genericamente "Basic Auth" e "API Key" como esquemas de cabeçalho, mas isso é uma afirmação de plataforma, não uma facilidade do painel: o painel de configuração de webhooks não tem campos separados de usuário/senha, só o par nome/valor único. Na prática, ninguém configuraria Basic Auth sem esse malabarismo manual.

Como nenhuma credencial real foi configurada ainda, o Fredy decidiu remover o modo `basic` completamente, deixando só o método que a AppyPay realmente disponibiliza pelo painel: um cabeçalho HTTP com nome configurável (padrão `X-API-Key`) e um segredo.

**Decisão de desenho:** em vez de manter um campo `webhook_auth_type` que só teria um valor possível, o conceito de "tipo" foi removido inteiramente. A configuração de webhook passa a ser: `webhook_secret` (opcional — se vazio, a credencial não autentica nenhum webhook) e `webhook_header_name` (opcional, só faz sentido quando `webhook_secret` é informado; padrão `X-API-Key`).

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| `webhook_auth_type` | Removido inteiramente (campo, validação, storage) | Não existe mais escolha de "tipo" de autenticação de webhook |
| `webhook_username` / Basic Auth | Removido inteiramente | `AuthenticateWebhook` não aceita mais usuário/senha |
| `webhook_secret` | Continua opcional; se vazio, credencial não autentica webhooks | Comportamento já existente, sem mudança |
| `webhook_header_name` | Continua opcional; padrão `X-API-Key` quando `webhook_secret` está preenchido | Sem mudança de comportamento para quem já usava `api_key` |
| Retrocompatibilidade | Não aplicável — nenhuma credencial configurada em nenhum ambiente hoje (confirmado com o Fredy) | Remoção limpa, sem migração de dados |
| Arquivos de código alterados | `internal/finance/appypay.go`, `internal/handlers/financeiro_handlers.go`, `internal/finance/appypay_integration_test.go` | Nenhum outro arquivo Go tocado |
| Arquivos de documentação alterados | `Documentação da API.md`, `docs/Parceiros e integrações/AppyPay Documentação.md` | Contrato público e notas internas refletem o método único |
| Achado à parte (fora de escopo) | `TestIntegrationFinanceRejectsNonFPPAdmins` (`internal/handlers`) falha se a suíte de integração for rodada duas vezes seguidas sem recriar o banco — sujeira de teste não relacionada a AppyPay | Não corrigir nesta tarefa; só documentado aqui para not ficar perdido |

---

# 1. `internal/finance/appypay.go`

## 1.1 `CredentialInput` e `CredentialView`

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
	WebhookAuthType   string `json:"webhook_auth_type,omitempty"` // basic or api_key
	WebhookUsername   string `json:"webhook_username,omitempty"`
	WebhookSecret     string `json:"webhook_secret,omitempty"`
	WebhookHeaderName string `json:"webhook_header_name,omitempty"` // nome do cabeçalho HTTP quando webhook_auth_type="api_key"; padrão "X-API-Key"
}
type CredentialView struct {
	ID                   uuid.UUID `json:"id"`
	ContextoTipo         string    `json:"contexto_tipo"`
	CodigoAcademia       string    `json:"codigo_academia,omitempty"`
	Ambiente             string    `json:"ambiente"`
	ClientIDMask         string    `json:"client_id_mask"`
	GPOPaymentMethodMask string    `json:"gpo_payment_method_mask"`
	REFPaymentMethodMask string    `json:"ref_payment_method_mask"`
	WebhookAuthType      string    `json:"webhook_auth_type,omitempty"`
	WebhookHeaderName    string    `json:"webhook_header_name,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
}
```

Substituir por:

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
type CredentialView struct {
	ID                   uuid.UUID `json:"id"`
	ContextoTipo         string    `json:"contexto_tipo"`
	CodigoAcademia       string    `json:"codigo_academia,omitempty"`
	Ambiente             string    `json:"ambiente"`
	ClientIDMask         string    `json:"client_id_mask"`
	GPOPaymentMethodMask string    `json:"gpo_payment_method_mask"`
	REFPaymentMethodMask string    `json:"ref_payment_method_mask"`
	WebhookHeaderName    string    `json:"webhook_header_name,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
}
```

## 1.2 `ConfigureCredential` — função completa

Localizar a função inteira (de `func (s *Service) ConfigureCredential(...)` até o `}` de fechamento) e substituir por:

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

Note que a chave `"webhook_username"` some do `map[string]string` passado a `saveSecrets` — ela não é mais gravada em `financeiro_segredos_appypay`.

## 1.3 `AuthenticateWebhook`

Localizar (comentário + struct `WebhookOwner` + função inteira):

```go
// AuthenticateWebhook accepts either the configured HTTP Basic credentials or
// the HTTP header configured for API Key auth (webhook_header_name, com
// padrão "X-API-Key" quando a credencial não define um nome próprio). A
// AppyPay confirmou por e-mail que o painel de configuração de webhooks só
// envia credenciais via cabeçalho HTTP — nunca via query parameter, nunca no
// corpo do POST. It never reveals which configured account matched.
type WebhookOwner struct {
	CredentialID                 uuid.UUID
	ContextoTipo, CodigoAcademia string
}

func (s *Service) AuthenticateWebhook(ctx context.Context, basicUser, basicPassword string, headers http.Header) (WebhookOwner, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id,payload FROM financeiro_credenciais_appypay WHERE payload->>'webhook_auth_type' IN ('basic','api_key')`)
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
			WebhookAuthType   string `json:"webhook_auth_type"`
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
		if meta.WebhookAuthType == "basic" && basicUser != "" && constantTimeEqual(basicUser, secrets["webhook_username"]) && constantTimeEqual(basicPassword, secrets["webhook_secret"]) {
			return WebhookOwner{CredentialID: id, ContextoTipo: meta.ContextoTipo, CodigoAcademia: meta.CodigoAcademia}, nil
		}
		if meta.WebhookAuthType == "api_key" {
			headerName := strings.TrimSpace(meta.WebhookHeaderName)
			if headerName == "" {
				headerName = defaultWebhookHeaderName
			}
			candidate := headers.Get(headerName)
			if candidate != "" && constantTimeEqual(candidate, secrets["webhook_secret"]) {
				return WebhookOwner{CredentialID: id, ContextoTipo: meta.ContextoTipo, CodigoAcademia: meta.CodigoAcademia}, nil
			}
		}
	}
	return WebhookOwner{}, errors.New("webhook não autenticado")
}
```

Substituir por:

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

**Por que a query SQL muda de `payload->>'webhook_auth_type' IN (...)` para `COALESCE(payload->>'webhook_header_name','') <> ''`:** sem o campo `webhook_auth_type`, o sinal não-sensível que indica "esta credencial tem webhook configurado" passa a ser o próprio `webhook_header_name` — que `ConfigureCredential` (item 1.2) sempre normaliza para um valor não vazio quando `webhook_secret` é informado. Nenhuma migration é necessária: é só uma mudança na cláusula `WHERE` de uma consulta sobre uma coluna `JSONB` já existente.

## 1.4 Nenhuma outra função muda

`loadSecrets`, `credentialSecrets`, `loadCredential`, `saveSecrets`, `token()`, `AcceptWebhook` e todo o resto do arquivo não são afetados — nenhum deles referenciava `webhook_auth_type`/`webhook_username`.

---

# 2. `internal/handlers/financeiro_handlers.go`

Localizar:

```go
func ReceberWebhookAppyPay(metodo string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, password, _ := c.Request.BasicAuth()
		owner, err := FinanceiroService.AuthenticateWebhook(c.Request.Context(), user, password, c.Request.Header)
		if err != nil {
			c.Status(http.StatusUnauthorized)
			return
		}
```

Substituir por:

```go
func ReceberWebhookAppyPay(metodo string) gin.HandlerFunc {
	return func(c *gin.Context) {
		owner, err := FinanceiroService.AuthenticateWebhook(c.Request.Context(), c.Request.Header)
		if err != nil {
			c.Status(http.StatusUnauthorized)
			return
		}
```

O restante da função não muda.

---

# 3. `internal/finance/appypay_integration_test.go`

Localizar a função `TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials` inteira (do `func TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials(t *testing.T) {` até o `}` de fechamento) e substituir por:

```go
func TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()
	t.Setenv("ENV", "test")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	suffix := uuid.NewString()[:8]

	customAcademy := "INT" + uuid.NewString()[:8]
	custom, err := service.ConfigureCredential(ctx, nil, CredentialInput{
		ContextoTipo:      ContextoAcademia,
		CodigoAcademia:    customAcademy,
		ClientID:          "client-custom",
		ClientSecret:      "secret-custom",
		GPOPaymentMethod:  "GPO_CUSTOM",
		REFPaymentMethod:  "REF_CUSTOM",
		WebhookSecret:     "custom-webhook-secret-" + suffix,
		WebhookHeaderName: "X-Spuri-Webhook-Secret",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if custom.WebhookHeaderName != "X-Spuri-Webhook-Secret" {
		t.Fatalf("nome de cabeçalho não persistido: %q", custom.WebhookHeaderName)
	}
	if _, err = service.loadCredential(ctx, ContextoAcademia, customAcademy); err != nil {
		t.Fatalf("credencial sem resource no cofre não recarregou: %v", err)
	}
	customHeaders := http.Header{}
	customHeaders.Set("X-Spuri-Webhook-Secret", "custom-webhook-secret-"+suffix)
	owner, err := service.AuthenticateWebhook(ctx, customHeaders)
	if err != nil || owner.CredentialID != custom.ID {
		t.Fatalf("cabeçalho customizado não autenticou: owner=%#v err=%v", owner, err)
	}
	wrongHeaders := http.Header{}
	wrongHeaders.Set("X-API-Key", "custom-webhook-secret-"+suffix)
	if _, err = service.AuthenticateWebhook(ctx, wrongHeaders); err == nil {
		t.Fatal("X-API-Key autenticou credencial configurada para cabeçalho customizado")
	}

	legacyID := uuid.New()
	legacyAcademy := "INT" + uuid.NewString()[:8]
	legacyPayload, err := json.Marshal(map[string]any{
		"credential_id":       legacyID.String(),
		"contexto_tipo":       ContextoAcademia,
		"codigo_academia":     legacyAcademy,
		"ambiente":            AmbienteTeste,
		"webhook_header_name": "X-API-Key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.DB().ExecContext(ctx, `INSERT INTO financeiro_credenciais_appypay (id,contexto_tipo,codigo_academia,ambiente,payload) VALUES ($1,$2,$3,$4,$5::jsonb)`, legacyID, ContextoAcademia, legacyAcademy, AmbienteTeste, legacyPayload); err != nil {
		t.Fatal(err)
	}
	if err = service.saveSecrets(ctx, legacyID, map[string]string{"client_id": "legacy-client", "client_secret": "legacy-secret", "gpo_method": "GPO_LEGACY", "ref_method": "REF_LEGACY", "webhook_secret": "legacy-webhook-secret-" + suffix}); err != nil {
		t.Fatal(err)
	}
	legacyHeaders := http.Header{}
	legacyHeaders.Set("X-API-Key", "legacy-webhook-secret-"+suffix)
	owner, err = service.AuthenticateWebhook(ctx, legacyHeaders)
	if err != nil || owner.CredentialID != legacyID {
		t.Fatalf("fallback X-API-Key para credencial legada falhou: owner=%#v err=%v", owner, err)
	}

	noWebhookAcademy := "INT" + uuid.NewString()[:8]
	if _, err = service.ConfigureCredential(ctx, nil, CredentialInput{
		ContextoTipo:     ContextoAcademia,
		CodigoAcademia:   noWebhookAcademy,
		ClientID:         "client-no-webhook",
		ClientSecret:     "secret-no-webhook",
		GPOPaymentMethod: "GPO_NOWEBHOOK",
		REFPaymentMethod: "REF_NOWEBHOOK",
	}, "integration-test", "sistema", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	noWebhookHeaders := http.Header{}
	noWebhookHeaders.Set("X-API-Key", "")
	if _, err = service.AuthenticateWebhook(ctx, noWebhookHeaders); err == nil {
		t.Fatal("credencial sem webhook_secret configurado não deveria autenticar nada")
	}
}
```

O que muda em relação ao teste atual: (a) todas as chamadas a `AuthenticateWebhook` perdem os dois primeiros argumentos (`basicUser, basicPassword`), (b) o cenário de credencial legada grava `webhook_header_name` diretamente no `payload` em vez de `webhook_auth_type`, (c) o cenário inteiro de Basic Auth foi removido, (d) um novo cenário confirma que uma credencial configurada **sem** `webhook_secret` nunca autentica nada.

---

# 4. Documentação

## 4.1 `Documentação da API.md`

**Linha do resumo da seção 19 (bullet sobre webhooks públicos):**

Localizar:

```
- Os webhooks são públicos por necessidade do gateway, mas autenticados por Basic Auth ou API Key cadastrada na credencial do contexto. Eventos aceitos ou duplicados respondem `200` e são tratados de forma idempotente pelo identificador do evento.
```

Substituir por:

```
- Os webhooks são públicos por necessidade do gateway, mas autenticados pelo segredo de webhook cadastrado na credencial do contexto, enviado pela AppyPay num cabeçalho HTTP configurável (`webhook_header_name`, padrão `X-API-Key`). Eventos aceitos ou duplicados respondem `200` e são tratados de forma idempotente pelo identificador do evento.
```

**Seção 19.1 — Request JSON:** remover a linha `"webhook_auth_type": "api_key",`. Exemplo final:

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

**Seção 19.1 — Response 201:** remover a linha `"webhook_auth_type": "api_key",`. Exemplo final:

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

**Seção 19.1 — Regras de negócio:** localizar:

```
- `webhook_auth_type="basic"` exige `webhook_username` e `webhook_secret`; `webhook_auth_type="api_key"` exige `webhook_secret` e, opcionalmente, `webhook_header_name` (nome do cabeçalho HTTP em que a AppyPay deve enviar o segredo; padrão `X-API-Key` quando omitido). O nome deve corresponder exatamente ao configurado no campo "nome" do painel de webhooks da AppyPay.
```

Substituir por:

```
- `webhook_secret` é opcional: quando omitido, a credencial não autentica nenhum webhook (as rotas continuam recebendo eventos, mas nada é aceito). Quando informado, `webhook_header_name` pode ser enviado para escolher o nome do cabeçalho HTTP em que a AppyPay deve mandar esse segredo; o padrão é `X-API-Key` quando omitido. O nome deve corresponder exatamente ao configurado no campo "nome" do painel de webhooks da AppyPay — a AppyPay confirmou que esse painel só oferece um único par nome/valor de cabeçalho HTTP, por isso não existe mais um modo de autenticação alternativo (ex.: Basic Auth) para o webhook.
```

**Seção 19.3 — Response 200:** remover a linha `"webhook_auth_type": "api_key",` do item da lista (mesma estrutura da 19.1).

**Seção 19.7 — Proteção:** localizar:

```
**Proteção:** pública no roteamento HTTP, autenticada por credencial AppyPay cadastrada. Use `Authorization: Basic ...` quando `webhook_auth_type="basic"`; para `webhook_auth_type="api_key"`, o nome do cabeçalho é configurável por credencial em `webhook_header_name` (padrão `X-API-Key`). A AppyPay confirmou que a autenticação do webhook sempre viaja por cabeçalho HTTP, nunca por query parameter.
```

Substituir por:

```
**Proteção:** pública no roteamento HTTP, autenticada pelo segredo de webhook cadastrado na credencial, enviado no cabeçalho HTTP configurado em `webhook_header_name` (padrão `X-API-Key`). A AppyPay confirmou que a autenticação do webhook sempre viaja por cabeçalho HTTP, nunca por query parameter — por isso este é o único método suportado.
```

**Seção 19.8 — Proteção:** localizar:

```
**Proteção:** igual ao webhook GPO: autenticação por Basic Auth ou por API Key no cabeçalho configurado pela credencial (`webhook_header_name`, padrão `X-API-Key`). A AppyPay confirmou que essa autenticação sempre viaja por cabeçalho HTTP, nunca por query parameter.
```

Substituir por:

```
**Proteção:** igual ao webhook GPO: autenticação pelo segredo de webhook no cabeçalho HTTP configurado pela credencial (`webhook_header_name`, padrão `X-API-Key`). A AppyPay confirmou que essa autenticação sempre viaja por cabeçalho HTTP, nunca por query parameter.
```

## 4.2 `docs/Parceiros e integrações/AppyPay Documentação.md`

**Linha 8380** (nota "Webhooks (transacional)", requisitos do endpoint) — localizar:

```
- Suportar autenticação do lado do Spuri via **Basic Auth** ou **API Key** (configurável, conforme suportado pela AppyPay).
```

Substituir por:

```
- Suportar autenticação do lado do Spuri via um único cabeçalho HTTP configurável (nome + valor) — a AppyPay confirmou por e-mail que o painel de configuração de webhooks só oferece esse par nome/valor, sem campos separados de utilizador/senha; por isso o Spuri não suporta mais um modo alternativo de Basic Auth para webhook (removido — ver tarefa de simplificação do método de autenticação de webhook).
```

**Linha 8386** (nota de segurança — corrigir de passagem, achado durante esta auditoria): localizar:

```
- `client_id`, `client_secret`, `resource` e quaisquer chaves de API/Webhook (por Spuri e por academia) são gravados cifrados no banco de dados — nunca em texto plano, nunca em payloads de eventos ou em respostas públicas da API do Spuri.
```

Substituir por:

```
- `client_id`, `client_secret` e quaisquer chaves de API/Webhook (por Spuri e por academia) são gravados cifrados no banco de dados — nunca em texto plano, nunca em payloads de eventos ou em respostas públicas da API do Spuri. `resource` é exceção: não é mais gravado por credencial — vem da variável de ambiente `APPYPAY_RESOURCE`, com o mesmo valor para todas as academias e para o Spuri no mesmo ambiente.
```

**Não alterar a linha 1953** ("For authorization we support **Basic Auth** (username and password) and **API Key**") — é uma citação literal da documentação original da AppyPay, não uma afirmação do Spuri; alterá-la desvirtuaria a citação.

---

# Fora de escopo

- `go.mod` / `go.sum` — não precisam de nenhuma alteração real. Se, ao validar localmente, você notar um diff neles vindo de `replace` directives ou dependências indiretas, **não os inclua no commit** — são artefatos do ambiente de validação, não desta tarefa.
- `TestIntegrationFinanceRejectsNonFPPAdmins` (`internal/handlers/financeiro_handlers_integration_test.go`) falhar quando a suíte é rodada duas vezes seguidas contra o mesmo banco sem recriá-lo — reproduzi esse problema durante a validação desta tarefa, mas é uma sujeira de teste pré-existente, sem nenhuma relação com AppyPay/webhook. Não corrija isso aqui; é candidato a uma tarefa própria no futuro.
- Qualquer alteração em `CreateCharge`, `CreateGPOQRCode`, `ConsultCharge`, `AcceptWebhook`, cifragem/decifragem de segredos, `appyPayResource`/`ValidateAppyPayResourceConfig`, ou qualquer outra função de `internal/finance/appypay.go` não listada explicitamente nas seções 1–3.
- O frontend (`spuripainel`) tem uma tarefa própria e separada para esta mesma mudança — não é necessário (nem possível, são repositórios diferentes) tocar nele a partir daqui.

# Critérios de aceite

1. `internal/finance/appypay.go` bate exatamente com as Seções 1.1–1.3; nenhuma referência a `WebhookAuthType`, `webhook_auth_type`, `WebhookUsername` ou `webhook_username` sobra no arquivo.
2. `internal/handlers/financeiro_handlers.go` bate com a Seção 2; `c.Request.BasicAuth()` não é mais chamado neste arquivo.
3. `internal/finance/appypay_integration_test.go` bate com a Seção 3.
4. `Documentação da API.md` e `docs/Parceiros e integrações/AppyPay Documentação.md` batem com a Seção 4.
5. `go build ./...` e `go vet ./...` passam sem erro no repositório inteiro.
6. `go test ./...` passa sem `RUN_POSTGRES_INTEGRATION=1`.
7. Com `RUN_POSTGRES_INTEGRATION=1` e um banco **recém-criado** (não reaproveitado de uma execução anterior), `go test ./internal/finance/... -run TestIntegration -v` passa, incluindo o novo cenário de credencial sem `webhook_secret`.
8. `grep -rn "webhook_auth_type\|WebhookAuthType\|webhook_username\|WebhookUsername" --include="*.go" .` não retorna nenhuma linha no repositório inteiro.

## Nota de validação (Claude, antes de entregar esta tarefa)

Apliquei exatamente este código (Seções 1–4) sobre uma cópia real do repositório `spuri-backend`, com Go 1.24 (o exigido pelo `go.mod`) e as dependências reais do projeto. `go build ./...` e `go vet ./...` passaram sem erro no repositório inteiro. Rodei a suíte completa (`go test ./...`) com `RUN_POSTGRES_INTEGRATION=1` contra um PostgreSQL real recém-criado — tudo passou, incluindo o teste de webhook reescrito com o novo cenário "sem webhook configurado". Ao investigar uma falha que apareceu numa segunda execução sem recriar o banco, confirmei que ela é em `TestIntegrationFinanceRejectsNonFPPAdmins`, um teste que não toca em nada de AppyPay — registrei isso em "Fora de escopo" para não ficar perdido, mas não faz parte desta tarefa.

## Procedimento de conclusão

1. Atualizar o título interno desta tarefa para `# Simplificar autenticação de webhook AppyPay para um único método (feito)`.
2. Alterar o front matter para `status: feito`.
3. Mover este arquivo para `docs/Tarefas feitas/`.
