---
criado: 2026-08-10 00:00
origem: (1) Confirmação por e-mail da equipa de suporte AppyPay sobre autenticação de webhooks via cabeçalho HTTP; (2) decisão do Fredy de mover o parâmetro `resource` do pedido de token OAuth2 para variável de ambiente, por não ser específico de cada academia
status: feito
---

# Cabeçalho de webhook configurável e `resource` AppyPay via variável de ambiente (feito)

## Prompt recomendado para executar a atualização

Implemente as duas mudanças independentes descritas neste documento em `internal/finance/appypay.go`, `internal/handlers/financeiro_handlers.go` e `cmd/server/main.go`. As duas mudanças tocam algumas das mesmas funções (`CredentialInput`, `CredentialView`, `ConfigureCredential`), por isso a Seção 3 mostra o **código final e completo** dessas partes já com as duas mudanças aplicadas juntas — use a Seção 3 como fonte da verdade para essas funções/structs, não tente aplicar as Seções 1 e 2 como dois patches separados sobre elas. Não toque em nenhuma lógica de cobranças (`CreateCharge`), QR Code, consulta de cobrança, cifragem de segredos ou qualquer outra função do arquivo que não esteja listada neste documento. Ao final, atualize `Documentação da API.md`, `.env.example` e `docs/Parceiros e integrações/AppyPay Documentação.md` conforme a Seção 4, e adicione os testes da Seção 5.

## Contexto

### Parte 1 — Autenticação de webhooks por cabeçalho configurável

A tarefa 17 (`docs/Tarefas feitas/17 - Modulo base de gestao financeira com AppyPay.md`) implementou a recepção de webhooks AppyPay (`POST /webhooks/appypay/gpo` e `POST /webhooks/appypay/ref`) com dois modos de autenticação por credencial, ambos já baseados em **cabeçalho HTTP**, nunca em query parameter ou no corpo do POST:

- `webhook_auth_type="basic"`: lê `Authorization: Basic <base64(usuário:senha)>` via `c.Request.BasicAuth()`.
- `webhook_auth_type="api_key"`: lê um cabeçalho **fixo no código**, `X-API-Key` (`c.GetHeader("X-API-Key")`).

A equipa de suporte da AppyPay confirmou por e-mail que o painel de configuração de webhooks (campos de "nome" e "valor") corresponde a um **cabeçalho HTTP**, não a um query parameter. Isso confirma que a abordagem por cabeçalho já implementada está correta. O ponto em aberto é que o painel da AppyPay permite escolher livremente o **nome** desse cabeçalho, mas o código só reconhece `X-API-Key`. Se o campo "nome" do painel não for preenchido exatamente assim, a autenticação falha com `401` mesmo com o valor certo. Esta parte torna esse nome configurável por credencial, com `X-API-Key` como padrão para não quebrar nada já configurado. O modo `basic` não muda: já funciona se o painel for configurado com nome `Authorization` e valor `Basic <base64>` pré-calculado.

### Parte 2 — `resource` deixa de ser configurado por credencial e passa a vir de variável de ambiente

Ao revisar a seção 19.1 da documentação, o Fredy notou que o campo `resource` (enviado no pedido de token OAuth2 da AppyPay, junto de `client_id`/`client_secret`) poderia não precisar ser configurado por credencial. Investigação:

- Na documentação oficial da AppyPay (`docs/Parceiros e integrações/AppyPay Documentação.md`, linhas 2892–2900), `resource` é descrito como `string<uuid>`, rotulado **"Merchant_Resource"**, com o mesmo valor de exemplo/padrão (`2aed7612-de64-46b5-9e59-1f48f8902d14`) repetido em toda a documentação pública da AppyPay — não há evidência de que varie por merchant nos exemplos.
- No código (`internal/finance/appypay.go`), `resource` só é usado dentro de `token()`, como parâmetro do pedido OAuth2 `client_credentials` contra o Azure AD da própria AppyPay (`login.microsoftonline.com/...`). Semanticamente, `resource` é a **audiência do token** (identifica a API da AppyPay que está a ser acedida), diferente de `client_id`/`client_secret`, que identificam **quem** está a autenticar (o merchant).
- Por decisão do Fredy: `resource` continua a ser exigido pela AppyPay exatamente como a documentação pede (vai no mesmo pedido de token, com o mesmo nome de campo), mas deixa de ser preenchido pelo payload de `POST/PUT /financeiro/appypay/credenciais` e passa a ser lido de uma variável de ambiente (`APPYPAY_RESOURCE`), com o mesmo valor para todas as academias e para o Spuri no mesmo ambiente.

**Nota de risco a registar no commit/PR:** a nota interna em `docs/Parceiros e integrações/AppyPay Documentação.md` (linha 8249, corrigida nesta tarefa — ver Seção 4) descrevia `resource` como credencial por conta AppyPay, no mesmo grupo de `client_id`/`client_secret`. Esta tarefa segue a decisão explícita do Fredy de tratá-lo como valor fixo por ambiente. Se, ao integrar uma academia real, o valor de `resource` fornecido pela AppyPay for diferente do já configurado em `APPYPAY_RESOURCE`, isso é sinal de que a suposição estava errada e o campo precisa voltar a ser por credencial — não ignore um erro de autenticação AppyPay que mencione `resource`/`audience` sem investigar essa possibilidade primeiro.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Cabeçalho `api_key` | Nome do cabeçalho passa a ser um campo por credencial (`webhook_header_name`), lido dinamicamente; padrão `X-API-Key` | Compatível com qualquer nome definido no painel da AppyPay; credenciais já configuradas continuam a autenticar |
| Cabeçalho `basic` | Sem alteração de código | Continua funcional; só ganha nota na documentação |
| `resource` | Sai de `CredentialInput`/`CredentialView`/banco; passa a vir de `APPYPAY_RESOURCE` (env var), validada no arranque do servidor | Nenhuma academia/Spuri precisa mais informar `resource` ao configurar credenciais AppyPay |
| Arquivos de código alterados | `internal/finance/appypay.go`, `internal/handlers/financeiro_handlers.go`, `cmd/server/main.go` | Mudança cirúrgica; nenhuma outra função tocada |
| Arquivos de documentação alterados | `Documentação da API.md`, `.env.example`, `docs/Parceiros e integrações/AppyPay Documentação.md` | Contrato público e notas internas refletem o estado real |
| Arquivos removidos | Nenhum | Esta tarefa não remove nenhum arquivo, rota ou funcionalidade |
| Fora do escopo desta tarefa | Repositório `spuripainel` (frontend) | O formulário de credenciais AppyPay do frontend provavelmente tem um campo "Resource" que passa a ser inútil — ver Seção 6 |

---

# 1. Cabeçalho de webhook `api_key` configurável por credencial

## 1.1 `internal/finance/appypay.go` — `AuthenticateWebhook` deve receber os cabeçalhos completos

Localizar (bloco atual, ~35 linhas, começando em `// AuthenticateWebhook accepts either...`):

```go
// AuthenticateWebhook accepts either the configured HTTP Basic credentials or
// an X-API-Key. It never reveals which configured account matched.
type WebhookOwner struct {
	CredentialID                 uuid.UUID
	ContextoTipo, CodigoAcademia string
}

func (s *Service) AuthenticateWebhook(ctx context.Context, basicUser, basicPassword, apiKey string) (WebhookOwner, error) {
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
			WebhookAuthType string `json:"webhook_auth_type"`
			ContextoTipo    string `json:"contexto_tipo"`
			CodigoAcademia  string `json:"codigo_academia"`
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
		if meta.WebhookAuthType == "api_key" && apiKey != "" && constantTimeEqual(apiKey, secrets["webhook_secret"]) {
			return WebhookOwner{CredentialID: id, ContextoTipo: meta.ContextoTipo, CodigoAcademia: meta.CodigoAcademia}, nil
		}
	}
	return WebhookOwner{}, errors.New("webhook não autenticado")
}
```

Substituir por (o nome do cabeçalho a procurar agora depende de qual credencial está sendo testada em cada iteração do loop, por isso a função passa a receber o `http.Header` completo em vez de um valor único pré-extraído):

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

Não é necessário nenhum novo `import`: `net/http` já está importado em `internal/finance/appypay.go`.

## 1.2 `internal/handlers/financeiro_handlers.go` — repassar os cabeçalhos completos da requisição

Localizar:

```go
func ReceberWebhookAppyPay(metodo string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, password, _ := c.Request.BasicAuth()
		owner, err := FinanceiroService.AuthenticateWebhook(c.Request.Context(), user, password, c.GetHeader("X-API-Key"))
```

Substituir por:

```go
func ReceberWebhookAppyPay(metodo string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, password, _ := c.Request.BasicAuth()
		owner, err := FinanceiroService.AuthenticateWebhook(c.Request.Context(), user, password, c.Request.Header)
```

O restante da função não muda. Não é necessário nenhum novo `import`.

## 1.3 Comportamento em `PUT /financeiro/appypay/credenciais/:id`

Esta rota já exige o conjunto completo de campos a cada chamada (substituição total, não patch parcial). Se uma credencial `api_key` com `webhook_header_name` customizado for atualizada via `PUT` sem reenviar `webhook_header_name`, o valor volta para o padrão `X-API-Key`. Comportamento intencional, consistente com o resto da credencial — não implemente um caminho de "patch parcial" só para este campo.

---

# 2. `resource` deixa de vir do payload e passa a vir de `APPYPAY_RESOURCE`

## 2.1 `internal/finance/appypay.go` — nova função de leitura da variável de ambiente

Adicionar logo após `EndpointsAtuais()` (ver Seção 3.1 para o bloco completo já posicionado):

```go
// appyPayResource retorna o "resource" (UUID) exigido pela AppyPay no pedido
// de token OAuth2 client_credentials. Ao contrário de client_id/client_secret,
// este valor identifica a própria API da AppyPay (audience do token), não o
// merchant que está a autenticar — por isso é o mesmo para todas as
// academias e para o Spuri no mesmo ambiente, e vem de variável de ambiente
// em vez de ser configurado por credencial.
func appyPayResource() (string, error) {
	v := strings.TrimSpace(os.Getenv("APPYPAY_RESOURCE"))
	if v == "" {
		return "", errors.New("APPYPAY_RESOURCE não configurada")
	}
	return v, nil
}

// ValidateAppyPayResourceConfig valida no arranque do servidor que
// APPYPAY_RESOURCE está definida, antes de qualquer tentativa de gerar um
// token AppyPay.
func ValidateAppyPayResourceConfig() error {
	_, err := appyPayResource()
	return err
}
```

Não é necessário nenhum novo `import`: `os`, `strings` e `errors` já estão importados em `internal/finance/appypay.go`.

## 2.2 `internal/finance/appypay.go` — `credentialSecrets`, `loadCredential`, `loadSecrets`, `token()`

**`credentialSecrets`** — localizar:

```go
type credentialSecrets struct {
	ID                                         uuid.UUID
	ClientID, ClientSecret, Resource, GPO, REF string
}
```

Substituir por:

```go
type credentialSecrets struct {
	ID                               uuid.UUID
	ClientID, ClientSecret, GPO, REF string
}
```

**`loadCredential`** — localizar a linha final da função:

```go
	return credentialSecrets{ID: id, ClientID: values["client_id"], ClientSecret: values["client_secret"], Resource: values["resource"], GPO: values["gpo_method"], REF: values["ref_method"]}, nil
```

Substituir por:

```go
	return credentialSecrets{ID: id, ClientID: values["client_id"], ClientSecret: values["client_secret"], GPO: values["gpo_method"], REF: values["ref_method"]}, nil
```

**`loadSecrets`** — localizar, dentro da função, a lista de campos obrigatórios do cofre de segredos:

```go
	for _, required := range []string{"client_id", "client_secret", "resource", "gpo_method", "ref_method"} {
```

Substituir por (**crítico**: se `"resource"` continuar nesta lista, `loadSecrets` vai falhar para toda e qualquer credencial nova, já que `resource` nunca mais será gravado em `financeiro_segredos_appypay` — ver 2.3):

```go
	for _, required := range []string{"client_id", "client_secret", "gpo_method", "ref_method"} {
```

**`token()`** — localizar:

```go
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {cred.ClientID}, "client_secret": {cred.ClientSecret}, "resource": {cred.Resource}}
```

Substituir por:

```go
	resource, err := appyPayResource()
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {cred.ClientID}, "client_secret": {cred.ClientSecret}, "resource": {resource}}
```

A variável `err` já existe mais abaixo na mesma função (`req, err := http.NewRequestWithContext(...)`); como `req` é nova, o `:=` continua válido, sem redeclaração.

## 2.3 `CredentialInput`, `CredentialView` e `ConfigureCredential`

Ver **Seção 3** — estas três definições são compartilhadas com a Seção 1 (`WebhookHeaderName`) e por isso são mostradas ali já no estado final, incluindo a remoção de `Resource`/`ResourceMask`/`"resource"` do `saveSecrets`.

## 2.4 `cmd/server/main.go` — validar `APPYPAY_RESOURCE` no arranque

Localizar, em `initDB()`:

```go
	if err := finance.ValidateEncryptionConfig(); err != nil {
		return fmt.Errorf("configuração de criptografia financeira inválida: %w", err)
	}
	config := db.DefaultConfig()
```

Substituir por:

```go
	if err := finance.ValidateEncryptionConfig(); err != nil {
		return fmt.Errorf("configuração de criptografia financeira inválida: %w", err)
	}
	if err := finance.ValidateAppyPayResourceConfig(); err != nil {
		return fmt.Errorf("configuração AppyPay inválida: %w", err)
	}
	config := db.DefaultConfig()
```

Nenhuma outra linha de `main.go` muda.

---

# 3. Estado final consolidado (Seções 1 + 2 já aplicadas juntas)

Esta seção é a fonte da verdade para as partes de `internal/finance/appypay.go` tocadas pelas duas mudanças ao mesmo tempo. Já foi compilada e testada com sucesso exatamente como está aqui (ver nota de validação no fim do documento).

## 3.1 `Endpoints`, `AmbienteAtual`, `EndpointsAtuais`, `appyPayResource`, `ValidateAppyPayResourceConfig`

```go
type Endpoints struct{ TokenURL, APIBaseURL string }

func AmbienteAtual() string {
	if strings.EqualFold(os.Getenv("ENV"), "production") {
		return AmbienteProducao
	}
	return AmbienteTeste
}
func EndpointsAtuais() Endpoints {
	if AmbienteAtual() == AmbienteProducao {
		return Endpoints{"https://login.microsoftonline.com/auth.appypay.co.ao/oauth2/token", "https://gwy-api.appypay.co.ao/v2.0"}
	}
	return Endpoints{"https://login.microsoftonline.com/appypaydev.onmicrosoft.com/oauth2/token", "https://gwy-api-tst.appypay.co.ao/v2.0"}
}

// appyPayResource retorna o "resource" (UUID) exigido pela AppyPay no pedido
// de token OAuth2 client_credentials. Ao contrário de client_id/client_secret,
// este valor identifica a própria API da AppyPay (audience do token), não o
// merchant que está a autenticar — por isso é o mesmo para todas as
// academias e para o Spuri no mesmo ambiente, e vem de variável de ambiente
// em vez de ser configurado por credencial.
func appyPayResource() (string, error) {
	v := strings.TrimSpace(os.Getenv("APPYPAY_RESOURCE"))
	if v == "" {
		return "", errors.New("APPYPAY_RESOURCE não configurada")
	}
	return v, nil
}

// ValidateAppyPayResourceConfig valida no arranque do servidor que
// APPYPAY_RESOURCE está definida, antes de qualquer tentativa de gerar um
// token AppyPay.
func ValidateAppyPayResourceConfig() error {
	_, err := appyPayResource()
	return err
}
```

## 3.2 `CredentialInput`

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
```

`Resource` foi removido (Parte 2); `WebhookHeaderName` foi adicionado (Parte 1).

## 3.3 `CredentialView`

```go
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

`ResourceMask` foi removido; `WebhookHeaderName` foi adicionado. `WebhookHeaderName` não é mascarado porque não é um segredo (é só o nome do cabeçalho), do mesmo jeito que `WebhookAuthType` já não é.

## 3.4 `ConfigureCredential` — função completa

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
	if in.WebhookAuthType != "" && in.WebhookAuthType != "basic" && in.WebhookAuthType != "api_key" {
		return CredentialView{}, errors.New("webhook_auth_type inválido")
	}
	if in.WebhookAuthType == "basic" && (in.WebhookUsername == "" || in.WebhookSecret == "") {
		return CredentialView{}, errors.New("Basic Auth de webhook exige utilizador e segredo")
	}
	if in.WebhookAuthType == "api_key" && in.WebhookSecret == "" {
		return CredentialView{}, errors.New("API Key de webhook exige segredo")
	}
	if in.WebhookAuthType == "api_key" {
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
	view := CredentialView{ID: credentialID, ContextoTipo: in.ContextoTipo, CodigoAcademia: in.CodigoAcademia, Ambiente: in.Ambiente, ClientIDMask: mask(in.ClientID), GPOPaymentMethodMask: mask(in.GPOPaymentMethod), REFPaymentMethodMask: mask(in.REFPaymentMethod), WebhookAuthType: in.WebhookAuthType, WebhookHeaderName: in.WebhookHeaderName, UpdatedAt: time.Now().UTC()}
	payload := map[string]any{"credential_id": credentialID.String(), "contexto_tipo": view.ContextoTipo, "codigo_academia": view.CodigoAcademia, "ambiente": view.Ambiente, "client_id_mask": view.ClientIDMask, "gpo_payment_method_mask": view.GPOPaymentMethodMask, "ref_payment_method_mask": view.REFPaymentMethodMask, "webhook_auth_type": view.WebhookAuthType, "webhook_header_name": view.WebhookHeaderName, "updated_at": view.UpdatedAt}
	if err := s.record(ctx, credentialID, "CredenciaisAppyPayConfiguradas", payload, userID, userType, ip); err != nil {
		return CredentialView{}, err
	}
	if err := s.saveSecrets(ctx, credentialID, map[string]string{"client_id": in.ClientID, "client_secret": in.ClientSecret, "gpo_method": in.GPOPaymentMethod, "ref_method": in.REFPaymentMethod, "webhook_username": in.WebhookUsername, "webhook_secret": in.WebhookSecret}); err != nil {
		return CredentialView{}, err
	}
	return view, nil
}
```

Validação/normalização de `webhook_header_name` (Parte 1) e remoção de `Resource`/`ResourceMask`/`"resource"` (Parte 2) aplicadas juntas, sem conflito.

## 3.5 Função auxiliar nova (fora de `ConfigureCredential`)

Adicionar próximo de `mask`/`validContext` (usada pela validação de `webhook_header_name` na Seção 3.4):

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

---

# 4. Atualização obrigatória da documentação

## 4.1 `.env.example`

Localizar:

```
# A API AppyPay é selecionada automaticamente: ENV=development/test usa TEST;
# ENV=production usa PROD. Não configure URLs ou credenciais AppyPay aqui:
# elas são armazenadas cifradas por academia/Spuri pelo endpoint financeiro.
```

Substituir por:

```
# A API AppyPay é selecionada automaticamente: ENV=development/test usa TEST;
# ENV=production usa PROD. Não configure client_id/client_secret/webhook aqui:
# eles são armazenados cifrados por academia/Spuri pelo endpoint financeiro
# (POST/PUT /financeiro/appypay/credenciais).
#
# APPYPAY_RESOURCE é a exceção: é o "resource" (UUID) exigido pela AppyPay no
# pedido de token OAuth2, e identifica a API da AppyPay em si (não o
# merchant) — por isso é o mesmo valor para todas as academias/Spuri no mesmo
# ambiente, e fica aqui em vez de no endpoint financeiro. Obrigatória para
# qualquer operação AppyPay (cobrança, QR Code, consulta ou geração de token);
# o servidor recusa iniciar sem ela definida.
APPYPAY_RESOURCE=substitua-pelo-resource-fornecido-pela-appypay
```

Use um placeholder textual, não um UUID real de exemplo, para não haver risco de alguém copiar um valor "que parece funcionar" para produção sem trocar.

## 4.2 `Documentação da API.md` — seção 19.1 (`POST /financeiro/appypay/credenciais`)

No bloco **Request JSON**, remover a linha `"resource": "2aed7612-de64-46b5-9e59-1f48f8902d14",` e adicionar, logo após `"webhook_secret"`, a linha `"webhook_header_name": "X-API-Key",` (com nota de que é opcional).

No bloco **Response 201**, remover a linha `"resource_mask": "http**********2.0",` e adicionar, logo após `"webhook_auth_type": "api_key",`, a linha `"webhook_header_name": "X-API-Key",`.

Na lista de **Regras de negócio**, substituir:

> `client_id`, `client_secret`, `resource`, `gpo_payment_method` e `ref_payment_method` são obrigatórios.

por:

> `client_id`, `client_secret`, `gpo_payment_method` e `ref_payment_method` são obrigatórios. `resource` não é enviado neste endpoint: é lido da variável de ambiente `APPYPAY_RESOURCE`, com o mesmo valor para todas as academias e para o Spuri no mesmo ambiente.

E substituir:

> `webhook_auth_type="basic"` exige `webhook_username` e `webhook_secret`; `webhook_auth_type="api_key"` exige `webhook_secret` e o gateway deve enviar esse segredo em `X-API-Key`.

por:

> `webhook_auth_type="basic"` exige `webhook_username` e `webhook_secret`; `webhook_auth_type="api_key"` exige `webhook_secret` e, opcionalmente, `webhook_header_name` (nome do cabeçalho HTTP em que a AppyPay deve enviar o segredo; padrão `X-API-Key` quando omitido). O nome deve corresponder exatamente ao configurado no campo "nome" do painel de webhooks da AppyPay.

## 4.3 `Documentação da API.md` — seção 19.2 (`PUT /financeiro/appypay/credenciais/:id`)

Adicionar à lista de **Regras de negócio** a nota de que a atualização é sempre uma substituição completa, então `webhook_header_name` deve ser reenviado explicitamente para preservar um nome customizado (senão volta para `X-API-Key`), e que `resource` continua fora do corpo da requisição (vem de `APPYPAY_RESOURCE`).

## 4.4 `Documentação da API.md` — seção 19.3 (`GET /financeiro/appypay/credenciais`)

No bloco **Response 200**, remover a linha `"resource_mask": "http**********2.0",` e adicionar, logo após `"webhook_auth_type": "api_key",`, a linha `"webhook_header_name": "X-API-Key",`.

## 4.5 `Documentação da API.md` — seções 19.7 e 19.8 (webhooks `gpo`/`ref`)

Substituir, em ambas, a frase de **Proteção** que menciona `X-API-Key` fixo por uma redação que deixe claro que o nome do cabeçalho de API Key é configurável por credencial (`webhook_header_name`, padrão `X-API-Key`), e que a AppyPay confirmou que a autenticação de webhook sempre viaja por cabeçalho HTTP, nunca por query parameter.

## 4.6 `docs/Parceiros e integrações/AppyPay Documentação.md` — linha 8249

Localizar:

> Corpo do pedido de token (`application/x-www-form-urlencoded`): `grant_type=client_credentials`, `client_id`, `client_secret`, `resource` (UUID fornecido pela AppyPay). **`client_id`, `client_secret` e `resource` são credenciais por conta AppyPay** — cada academia integrada ao Spuri tem (ou terá) a sua própria conta/credenciais junto à AppyPay, e o próprio Spuri poderá ter uma conta própria. Ambos os escopos de credenciais (Spuri e academia) devem poder ser gravados no banco de dados (cifrados — ver nota de segurança abaixo).

Substituir por:

> Corpo do pedido de token (`application/x-www-form-urlencoded`): `grant_type=client_credentials`, `client_id`, `client_secret`, `resource` (UUID fornecido pela AppyPay). **`client_id` e `client_secret` são credenciais por conta AppyPay** — cada academia integrada ao Spuri tem (ou terá) a sua própria conta/credenciais junto à AppyPay, e o próprio Spuri poderá ter uma conta própria; ambos os escopos devem poder ser gravados no banco de dados (cifrados — ver nota de segurança abaixo). **`resource` é tratado como valor fixo por ambiente** (variável de ambiente `APPYPAY_RESOURCE`), não por conta: identifica a API da AppyPay (audience do token OAuth2), não o merchant. Se uma integração real revelar um `resource` diferente por academia, esta suposição precisa ser revista — ver `docs/Tarefas feitas/21 - ...md`.

---

# 5. Testes obrigatórios

## 5.1 Unitários (sem banco), em `internal/finance/appypay_test.go`

1. `validHTTPHeaderName` aceita `"X-API-Key"`, `"Authorization"` e `"X-Spuri-Webhook-Secret"`; rejeita string vazia, string com espaço e string com dois-pontos.
2. `appyPayResource()`/`ValidateAppyPayResourceConfig()`: com `APPYPAY_RESOURCE` vazia/ausente, ambas retornam erro; com `APPYPAY_RESOURCE` definida (incluindo espaços a mais), `appyPayResource()` retorna o valor já com `strings.TrimSpace`. Usar `t.Setenv`/`os.Unsetenv` com `defer` para não vazar estado entre testes.

## 5.2 Integração (banco real, `RUN_POSTGRES_INTEGRATION=1`), em `internal/finance/appypay_integration_test.go`

1. **Cabeçalho customizado configurado explicitamente**: `ConfigureCredential` com `WebhookAuthType: "api_key"`, `WebhookSecret`, `WebhookHeaderName: "X-Spuri-Webhook-Secret"`; `AuthenticateWebhook` autentica com esse cabeçalho e falha com `X-API-Key` (nome antigo).
2. **Retrocompatibilidade sem `webhook_header_name`**: inserir por SQL uma linha em `financeiro_credenciais_appypay` com `payload` contendo `webhook_auth_type: "api_key"` mas sem a chave `webhook_header_name`; `AuthenticateWebhook` autentica com `X-API-Key` (padrão).
3. **Regressão do modo `basic`**: confirmar que a nova assinatura de `AuthenticateWebhook` (`headers http.Header` no lugar de `apiKey string`) não quebrou o caminho `basic`.
4. **`ConfigureCredential` não exige mais `resource`**: chamar com um `CredentialInput` sem o campo `Resource` (ele nem existe mais no struct) e confirmar que a credencial é criada com sucesso.
5. **`loadCredential`/`loadSecrets` não exigem mais `resource` no cofre**: confirmar que uma credencial configurada por esta nova versão do código consegue ser recarregada via `loadCredential` sem erro de "cofre de credenciais AppyPay incompleto".

---

# Fora de escopo

- Qualquer mudança no modo `webhook_auth_type="basic"`.
- Assinatura/validação HMAC do corpo do webhook.
- Qualquer alteração em `CreateCharge`, `CreateGPOQRCode`, `ConsultCharge`, `AcceptWebhook`, cifragem/decifragem de segredos, ou qualquer outra função de `internal/finance/appypay.go` não listada explicitamente nas Seções 1–3.
- Migração de dados: linhas já existentes em `financeiro_segredos_appypay` com `secret_type='resource'` ficam órfãs no banco (não são lidas nem escritas por este código); não é necessário apagá-las nesta tarefa.
- Atualizar `docs/Lista de Tarefas/00 - Índice e ordem de implementação.md`.
- O repositório `spuripainel` (frontend) — ver Seção 6.
- Diferenciar `APPYPAY_RESOURCE` por ambiente (teste vs. produção). A documentação pública da AppyPay usa o mesmo valor de exemplo para ambos; se isso se revelar incorreto na prática, criar `APPYPAY_RESOURCE_TEST`/`APPYPAY_RESOURCE_PROD` fica para uma tarefa futura.

# Critérios de aceite

1. `internal/finance/appypay.go` compila e bate exatamente com o estado descrito na Seção 3 nas partes ali cobertas.
2. `AuthenticateWebhook` tem a assinatura `(ctx context.Context, basicUser, basicPassword string, headers http.Header)` e usa o nome de cabeçalho configurado por credencial, com fallback para `X-API-Key`.
3. `ReceberWebhookAppyPay` repassa `c.Request.Header`.
4. `CredentialInput`/`CredentialView` não têm mais `Resource`/`ResourceMask`; `credentialSecrets` não tem mais `Resource`; nenhuma função em `internal/finance/appypay.go` referencia `.Resource` ou `values["resource"]`.
5. `token()` usa `appyPayResource()` para o campo `resource` do pedido OAuth2.
6. `cmd/server/main.go` chama `finance.ValidateAppyPayResourceConfig()` em `initDB()`, logo após `ValidateEncryptionConfig()`.
7. `.env.example`, `Documentação da API.md` e `docs/Parceiros e integrações/AppyPay Documentação.md` atualizados conforme a Seção 4.
8. Todos os testes da Seção 5 existem e passam (`go test ./...` sem `RUN_POSTGRES_INTEGRATION=1`; com `RUN_POSTGRES_INTEGRATION=1` e Postgres disponível para os de integração).
9. `go build ./...` e `go vet ./...` passam sem erro, sem identificador redeclarado ou não utilizado.
10. Nenhuma credencial `webhook_auth_type="basic"` já configurada tem seu comportamento alterado.

## Nota de validação (Claude, antes de entregar esta tarefa)

Apliquei exatamente o código das Seções 1–3 (incluindo a mudança em `cmd/server/main.go`) sobre uma cópia real do repositório, com o Go 1.24 exigido pelo `go.mod` e as dependências reais do projeto. `go build ./...` e `go vet ./...` passaram sem erros no repositório inteiro; os 6 testes unitários já existentes em `internal/finance` continuam a passar sem regressão; testei também o comportamento de `validHTTPHeaderName` e de `appyPayResource()`/`ValidateAppyPayResourceConfig()` isoladamente (vazio → erro; definido com espaços → retorna valor limpo). Não consegui rodar os testes de integração descritos na Seção 5.2 (exigem PostgreSQL, indisponível neste ambiente de validação) — Codex deve rodá-los com `RUN_POSTGRES_INTEGRATION=1` antes de concluir.

## Procedimento de conclusão

1. atualizar o título interno desta tarefa para `# Cabeçalho de webhook configurável e resource AppyPay via variável de ambiente (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
