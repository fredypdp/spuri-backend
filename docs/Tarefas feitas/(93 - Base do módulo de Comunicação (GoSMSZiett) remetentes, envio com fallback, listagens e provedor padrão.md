---
criado: 2026-09-09
origem: Fredy (orquestrado via Claude)
status: feito
---

# (feito) 93 — Base do módulo de Comunicação (GoSMS/Ziett): remetentes, envio com fallback, listagens e provedor padrão

## Prompt recomendado para executar a atualização

> Implemente a base do módulo de Comunicação do Spuri (GoSMS e Ziett), exatamente como especificado neste documento: 3 migrations novas, os agregados `RemetenteComunicacao` e `MensagemComunicacao` (event sourcing), as suas projeções, os clientes HTTP de GoSMS e Ziett, os handlers, o registo das rotas em `main.go`, a validação de arranque da nova chave de cifra, e a atualização da `Documentação da API.md`. Não altere nada além do que está descrito aqui. Não crie rotas, campos ou validações que não estejam explicitamente pedidos. Ao terminar, siga o "Procedimento de conclusão" no final deste documento.

## Contexto

O Spuri vai passar a enviar SMS institucionais através de dois provedores — **GoSMS** e **Ziett** (documentação completa em `docs/Parceiros e integrações/GoSMS API - Documentação.md` e `docs/Parceiros e integrações/Ziett API - Documentação.md`). Já existe uma rota **isolada de teste** para a Ziett (`POST /integracoes/ziett/mensagens/teste`, Tarefa 20) — ela **não deve ser alterada, nem reutilizada, nem importada** por este módulo. Este módulo é a implementação real e definitiva, e é completamente independente daquela rota de teste.

Decisões de arquitetura já fechadas com o Fredy (não é preciso replaneá-las, apenas implementar):

1. **Remetente é global.** Não pertence a nenhuma academia. Existe **no máximo um remetente por provedor** (um para GOSMS, um para ZIETT). "Cadastrar remetente" nunca chama a API do provedor para criar nada — apenas grava, no nosso próprio banco, o identificador que já existe e está aprovado do lado do provedor (o nome do Sender ID no GoSMS; o UUID do `remitter_id` no Ziett) **e o token de API desse provedor**, cifrado.
2. Cadastrar um remetente para um provedor que **já tem** remetente configurado **substitui** o remetente existente (é sempre o mesmo agregado a evoluir — não um agregado novo). Isto é feito com **IDs determinísticos e fixos** (um por provedor), gerados uma única vez e fixados como constante — nunca recalculados em runtime.
3. O token de API de cada provedor é **cifrado em repouso** (AES-256-GCM) com uma chave **própria** deste módulo (`COMUNICACAO_ENCRYPTION_KEY`) — não reaproveita `FINANCE_ENCRYPTION_KEY` nem qualquer código de `internal/finance`, para não misturar os dois domínios. O token **nunca** é devolvido em nenhuma resposta HTTP, em nenhum endpoint — é *write-only*.
4. **Enviar mensagem** manda para **um único destinatário por chamada** (não é envio em lote — nem a GoSMS nem a Ziett documentam de forma consistente um envio em lote na rota simples de mensagem; lote fica fora de escopo, ver secção "Fora de escopo").
5. O envio tenta primeiro o **provedor padrão** (configuração global única, definida por um admin FPP); se esse provedor não tiver remetente configurado, ou a tentativa falhar, tenta automaticamente o **outro** provedor. A mensagem é sempre registada (sucesso ou falha), com o detalhe de cada tentativa.
6. Toda a persistência segue o padrão de event sourcing já estabelecido no projeto (agregado → evento → ledger → projeção assíncrona), no mesmo estilo de `CategoriaServico` (o exemplo mais próximo e mais simples já existente em `internal/domain/aggregates/categoria_servico.go`).

### Nota sobre testes no seu ambiente (Codex) — leia isto primeiro

Eu (Claude) já validei tudo o que dependia de PostgreSQL de verdade, no meu próprio sandbox (Postgres 16 real, não simulado):

- Apliquei as **122 migrations já existentes** do projeto e, sobre elas, as **3 migrations novas** deste documento, em sequência, sem nenhum erro.
- Testei ao vivo, com INSERT/UPDATE reais, todas as constraints novas: unicidade de remetente por provedor, o upsert determinístico substituindo o remetente antigo do mesmo provedor (nova versão, novo `last_event_id`, mesmo `id`), rejeição de provedor inválido, a regra "academia sempre com `codigo_academia`, admin sempre sem" em `projection_mensagens_comunicacao`, rejeição de status inválido, e o comportamento singleton (uma linha só, sempre `id=1`) de `projection_comunicacao_config`.
- Também escrevi todo o código Go deste documento e verifiquei a sintaxe de cada arquivo com `gofmt` (sem erros, sem diffs de formatação pendentes).

O que eu **não consegui** validar no meu sandbox é `go build`/`go vet`/`go test` — o meu ambiente não tem acesso aos domínios de módulos Go (`golang.org` e afins), então não conseguiria baixar as dependências. Isso é o motivo de este ser o **seu primeiro passo obrigatório** (ver "Testes obrigatórios" no final): rodar `go build ./...`, `go vet ./...` e os testes do projeto. Se algo não compilar, é muito provável que seja um detalhe pontual de assinatura de função que mudou desde que este documento foi escrito — corrija localmente sem alterar a arquitetura acima.

## Resumo executivo

| Área | O que muda |
|---|---|
| Migrations | 3 novas: `projection_remetentes_comunicacao`, `projection_mensagens_comunicacao`, `projection_comunicacao_config` |
| Variáveis de ambiente | Nova `COMUNICACAO_ENCRYPTION_KEY` (obrigatória, chave própria, AES-256-GCM) |
| Agregados novos | `RemetenteComunicacao`, `MensagemComunicacao` (event sourcing) |
| Projeções novas | `RemetenteComunicacaoProjection`, `MensagemComunicacaoProjection` |
| Serviços novos | cifra/decifra do token (`comunicacao_crypto.go`), cliente HTTP GoSMS (`comunicacao_gosms_client.go`), cliente HTTP Ziett (`comunicacao_ziett_client.go`) |
| Handlers novos | `comunicacao_handlers.go` (6 endpoints) |
| Rotas novas | `POST/GET /comunicacao/remetentes`, `POST/GET /comunicacao/mensagens`, `GET/PUT /admin/comunicacao/provedor-padrao` |
| Arquivos alterados | `internal/domain/aggregates/aggregate.go`, `internal/handlers/helpers.go`, `cmd/server/main.go`, `.env.example`, `Documentação da API.md` |
| Arquivos que **não** mudam | `internal/services/ziett_sms_test_client.go`, `internal/handlers/ziett_sms_test_handler.go` (rota de teste isolada — intocada) |


## 1. Migrations novas

Crie exatamente estes 3 arquivos, com este conteúdo exato, em `migrations/` (já testados de ponta a ponta com PostgreSQL real — ver nota no topo do documento).

### `migrations/123_comunicacao_remetentes.sql`

```sql
BEGIN;

CREATE TABLE IF NOT EXISTS projection_remetentes_comunicacao (
    id UUID PRIMARY KEY,
    provedor VARCHAR(10) NOT NULL,
    identificador VARCHAR(100) NOT NULL,
    token_api_cifrado TEXT NOT NULL,
    configurado_por UUID NOT NULL,
    configurado_por_tipo VARCHAR(10) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID,
    CONSTRAINT chk_remetentes_comunicacao_provedor CHECK (provedor IN ('GOSMS', 'ZIETT')),
    CONSTRAINT chk_remetentes_comunicacao_por_tipo CHECK (configurado_por_tipo = 'admin'),
    CONSTRAINT uq_remetentes_comunicacao_provedor UNIQUE (provedor)
);

COMMIT;
```

### `migrations/124_comunicacao_mensagens.sql`

```sql
BEGIN;

CREATE TABLE IF NOT EXISTS projection_mensagens_comunicacao (
    id UUID PRIMARY KEY,
    destinatario VARCHAR(20) NOT NULL,
    conteudo TEXT NOT NULL,
    provedor_tentado_1 VARCHAR(10) NOT NULL,
    provedor_tentado_2 VARCHAR(10),
    provedor_utilizado VARCHAR(10),
    status VARCHAR(20) NOT NULL,
    mensagem_externa_id VARCHAR(200),
    detalhes_tentativas JSONB NOT NULL,
    enviado_por UUID NOT NULL,
    enviado_por_tipo VARCHAR(10) NOT NULL,
    codigo_academia VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID,
    CONSTRAINT chk_mensagens_comunicacao_provedor_1 CHECK (provedor_tentado_1 IN ('GOSMS', 'ZIETT')),
    CONSTRAINT chk_mensagens_comunicacao_provedor_2 CHECK (provedor_tentado_2 IS NULL OR provedor_tentado_2 IN ('GOSMS', 'ZIETT')),
    CONSTRAINT chk_mensagens_comunicacao_provedor_utilizado CHECK (provedor_utilizado IS NULL OR provedor_utilizado IN ('GOSMS', 'ZIETT')),
    CONSTRAINT chk_mensagens_comunicacao_status CHECK (status IN ('enviada', 'falhou')),
    CONSTRAINT chk_mensagens_comunicacao_enviado_por_tipo CHECK (enviado_por_tipo IN ('admin', 'academia')),
    CONSTRAINT chk_mensagens_comunicacao_codigo_academia CHECK (
        (enviado_por_tipo = 'academia' AND codigo_academia IS NOT NULL) OR
        (enviado_por_tipo = 'admin' AND codigo_academia IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_mensagens_comunicacao_created_at ON projection_mensagens_comunicacao(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mensagens_comunicacao_codigo_academia ON projection_mensagens_comunicacao(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_mensagens_comunicacao_enviado_por ON projection_mensagens_comunicacao(enviado_por);

COMMIT;
```

### `migrations/125_comunicacao_config.sql`

```sql
BEGIN;

CREATE TABLE IF NOT EXISTS projection_comunicacao_config (
    id INTEGER PRIMARY KEY DEFAULT 1,
    provedor_padrao VARCHAR(10),
    atualizado_por UUID,
    atualizado_em TIMESTAMPTZ,
    CONSTRAINT chk_comunicacao_config_singleton CHECK (id = 1),
    CONSTRAINT chk_comunicacao_config_provedor_padrao CHECK (provedor_padrao IS NULL OR provedor_padrao IN ('GOSMS', 'ZIETT'))
);

INSERT INTO projection_comunicacao_config (id, provedor_padrao, atualizado_por, atualizado_em)
VALUES (1, NULL, NULL, NULL)
ON CONFLICT (id) DO NOTHING;

COMMIT;
```

## 2. Variável de ambiente nova

### Localizar este bloco exato (final de `.env.example`)

```
# =============================================================================
# Ziett (CPaaS de SMS) — rota isolada de teste de integração
# =============================================================================
# Usada exclusivamente por POST /integracoes/ziett/mensagens/teste.
# zk_test_: ambiente de teste, sem custo e sem envio real.
# zk_live_: produção, envia SMS real e pode gerar custo.
ZIETT_API_KEY=zk_test_substitua_pela_sua_chave
```

### Substituir por

```
# =============================================================================
# Ziett (CPaaS de SMS) — rota isolada de teste de integração
# =============================================================================
# Usada exclusivamente por POST /integracoes/ziett/mensagens/teste.
# zk_test_: ambiente de teste, sem custo e sem envio real.
# zk_live_: produção, envia SMS real e pode gerar custo.
ZIETT_API_KEY=zk_test_substitua_pela_sua_chave

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

Regra de validação desta chave (implementada em `internal/services/comunicacao_crypto.go`, secção 5): igual à de `FINANCE_ENCRYPTION_KEY` — mínimo 32 caracteres, ou Base64 de exatamente 32 bytes; caso contrário é derivada com SHA-256. **Obrigatória em qualquer ambiente** — o servidor não arranca sem ela (ver secção 8).

Em `.env` (não versionado) gere uma chave real antes de rodar localmente, por exemplo com `openssl rand -base64 32`.


## 3. Agregados novos (event sourcing)

Crie exatamente estes 2 arquivos.

### `internal/domain/aggregates/remetente_comunicacao.go`

```go
package aggregates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ProvedorComunicacaoGoSMS = "GOSMS"
	ProvedorComunicacaoZiett = "ZIETT"
)

// RemetenteComunicacaoIDGoSMS e RemetenteComunicacaoIDZiett são IDs
// determinísticos e fixos: existe no máximo UM RemetenteComunicacao por
// provedor. "Cadastrar remetente" para um provedor que já tem um remetente
// configurado SUBSTITUI o remetente existente (novo evento sobre o MESMO
// agregado, nunca um agregado novo) — não existe conceito de múltiplos
// remetentes para o mesmo provedor. Gerados uma única vez com
// uuid.NewSHA1(uuid.NameSpaceOID, []byte("spuri.comunicacao.remetente.GOSMS"))
// (e o equivalente para ZIETT) e fixados aqui como constantes — nunca
// recalcular em runtime nem gerar novos valores.
var (
	RemetenteComunicacaoIDGoSMS = uuid.MustParse("04b3ec64-e246-53cf-8600-dc247c1f12e2")
	RemetenteComunicacaoIDZiett = uuid.MustParse("ba19eb30-516c-5d07-a5a8-b47ada777e0c")
)

// RemetenteAggregateID retorna o ID determinístico do agregado
// RemetenteComunicacao para o provedor informado (já normalizado para
// maiúsculas), ou uuid.Nil e false se o provedor não for reconhecido.
func RemetenteAggregateID(provedor string) (uuid.UUID, bool) {
	switch provedor {
	case ProvedorComunicacaoGoSMS:
		return RemetenteComunicacaoIDGoSMS, true
	case ProvedorComunicacaoZiett:
		return RemetenteComunicacaoIDZiett, true
	default:
		return uuid.Nil, false
	}
}

type RemetenteComunicacao struct {
	BaseAggregate
	Provedor           string
	Identificador      string
	TokenAPICifrado    string
	ConfiguradoPor     uuid.UUID
	ConfiguradoPorTipo string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func NewRemetenteComunicacao() *RemetenteComunicacao {
	return &RemetenteComunicacao{BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}}}
}
func (r *RemetenteComunicacao) GetType() string { return "RemetenteComunicacao" }

type RemetenteComunicacaoConfiguradoEvent struct {
	BaseEvent
	Provedor           string
	Identificador      string
	TokenAPICifrado    string
	ConfiguradoPor     uuid.UUID
	ConfiguradoPorTipo string
	ConfiguradoEm      time.Time
}

func (e *RemetenteComunicacaoConfiguradoEvent) GetPayload() interface{} { return e }
func (e *RemetenteComunicacaoConfiguradoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

func (r *RemetenteComunicacao) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "RemetenteComunicacaoConfigurado":
		return r.applyConfigurado(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido para RemetenteComunicacao: %s", event.GetEventType())
	}
}

var identificadorGoSMSRegex = regexp.MustCompile(`^[A-Z0-9]{1,11}$`)

// Configurar cria (primeira chamada) ou substitui (chamadas seguintes) as
// credenciais de envio para um provedor. Quem chama este método é
// responsável por já ter deixado r.ID igual ao ID determinístico de
// RemetenteAggregateID(provedor) antes de chamar Configurar (via SetID
// quando o agregado é novo, ou via repository.Load quando já existe — ver
// ResolverRemetenteAggregate em internal/handlers/comunicacao_handlers.go).
// tokenAPICifrado já deve vir cifrado pelo chamador
// (services.EncryptComunicacaoSegredo) — este método nunca lida com o
// token em texto plano.
func (r *RemetenteComunicacao) Configurar(provedor, identificador, tokenAPICifrado string, configuradoPor uuid.UUID) error {
	provedor = strings.ToUpper(strings.TrimSpace(provedor))
	if provedor != ProvedorComunicacaoGoSMS && provedor != ProvedorComunicacaoZiett {
		return fmt.Errorf("provedor inválido: use GOSMS ou ZIETT")
	}
	identificador = strings.TrimSpace(identificador)
	if identificador == "" {
		return fmt.Errorf("identificador é obrigatório")
	}
	switch provedor {
	case ProvedorComunicacaoGoSMS:
		identificador = strings.ToUpper(identificador)
		if !identificadorGoSMSRegex.MatchString(identificador) {
			return fmt.Errorf("identificador do GoSMS deve ter de 1 a 11 caracteres alfanuméricos maiúsculos (padrão de Sender ID alfanumérico GSM), ex.: SPURI")
		}
	case ProvedorComunicacaoZiett:
		if _, err := uuid.Parse(identificador); err != nil {
			return fmt.Errorf("identificador do Ziett deve ser o UUID do remitter_id configurado no painel da Ziett")
		}
	}
	if strings.TrimSpace(tokenAPICifrado) == "" {
		return fmt.Errorf("token de API é obrigatório")
	}
	e := &RemetenteComunicacaoConfiguradoEvent{
		BaseEvent:          BaseEvent{EventType: "RemetenteComunicacaoConfigurado", AggregateID: r.ID},
		Provedor:           provedor,
		Identificador:      identificador,
		TokenAPICifrado:    tokenAPICifrado,
		ConfiguradoPor:     configuradoPor,
		ConfiguradoPorTipo: "admin",
		ConfiguradoEm:      time.Now(),
	}
	r.RaiseEvent(e)
	return r.Apply(e)
}

func (r *RemetenteComunicacao) applyConfigurado(e DomainEvent) error {
	b, err := json.Marshal(e.GetPayload())
	if err != nil {
		return err
	}
	var p RemetenteComunicacaoConfiguradoEvent
	if err = json.Unmarshal(b, &p); err != nil {
		return err
	}
	r.Provedor = p.Provedor
	r.Identificador = p.Identificador
	r.TokenAPICifrado = p.TokenAPICifrado
	r.ConfiguradoPor = p.ConfiguradoPor
	r.ConfiguradoPorTipo = p.ConfiguradoPorTipo
	if r.CreatedAt.IsZero() {
		r.CreatedAt = p.ConfiguradoEm
	}
	r.UpdatedAt = p.ConfiguradoEm
	return nil
}
```

### `internal/domain/aggregates/mensagem_comunicacao.go`

```go
package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	StatusMensagemComunicacaoEnviada = "enviada"
	StatusMensagemComunicacaoFalhou  = "falhou"
)

// TentativaEnvioComunicacao registra o resultado de UMA tentativa de envio
// por UM provedor (parte do array detalhes_tentativas gravado em
// MensagemComunicacaoRegistradaEvent). No máximo duas tentativas por
// mensagem: a do provedor padrão e, se ela falhar ou o provedor não tiver
// remetente configurado, a do outro provedor.
type TentativaEnvioComunicacao struct {
	Provedor          string `json:"provedor"`
	Sucesso           bool   `json:"sucesso"`
	MensagemExternaID string `json:"mensagem_externa_id,omitempty"`
	ErroCodigo        string `json:"erro_codigo,omitempty"`
	ErroMensagem      string `json:"erro_mensagem,omitempty"`
}

type MensagemComunicacao struct {
	BaseAggregate
	Destinatario       string
	Conteudo           string
	ProvedorTentado1   string
	ProvedorTentado2   string
	ProvedorUtilizado  string
	Status             string
	MensagemExternaID  string
	DetalhesTentativas []TentativaEnvioComunicacao
	EnviadoPor         uuid.UUID
	EnviadoPorTipo     string
	CodigoAcademia     string
	CreatedAt          time.Time
}

func NewMensagemComunicacao() *MensagemComunicacao {
	return &MensagemComunicacao{BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}}}
}
func (m *MensagemComunicacao) GetType() string { return "MensagemComunicacao" }

type MensagemComunicacaoRegistradaEvent struct {
	BaseEvent
	Destinatario       string
	Conteudo           string
	ProvedorTentado1   string
	ProvedorTentado2   string
	ProvedorUtilizado  string
	Status             string
	MensagemExternaID  string
	DetalhesTentativas []TentativaEnvioComunicacao
	EnviadoPor         uuid.UUID
	EnviadoPorTipo     string
	CodigoAcademia     string
	CreatedAt          time.Time
}

func (e *MensagemComunicacaoRegistradaEvent) GetPayload() interface{} { return e }
func (e *MensagemComunicacaoRegistradaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

func (m *MensagemComunicacao) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "MensagemComunicacaoRegistrada":
		return m.applyRegistrada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido para MensagemComunicacao: %s", event.GetEventType())
	}
}

// Registrar grava o resultado final (sucesso ou falha, já com todas as
// tentativas de provedor esgotadas) de UM envio de mensagem. Este é o
// único evento deste agregado — MensagemComunicacao é criada uma única vez
// e nunca é alterada depois (registro de auditoria imutável). Toda a
// validação de destinatario/conteudo já deve ter acontecido ANTES de
// chamar Registrar (no handler, antes de gastar qualquer tentativa de
// envio real) — este método não valida, apenas registra o resultado.
func (m *MensagemComunicacao) Registrar(
	destinatario, conteudo string,
	provedorTentado1, provedorTentado2, provedorUtilizado string,
	status, mensagemExternaID string,
	detalhes []TentativaEnvioComunicacao,
	enviadoPor uuid.UUID,
	enviadoPorTipo, codigoAcademia string,
) error {
	e := &MensagemComunicacaoRegistradaEvent{
		BaseEvent:          BaseEvent{EventType: "MensagemComunicacaoRegistrada", AggregateID: m.ID},
		Destinatario:       destinatario,
		Conteudo:           conteudo,
		ProvedorTentado1:   provedorTentado1,
		ProvedorTentado2:   provedorTentado2,
		ProvedorUtilizado:  provedorUtilizado,
		Status:             status,
		MensagemExternaID:  mensagemExternaID,
		DetalhesTentativas: detalhes,
		EnviadoPor:         enviadoPor,
		EnviadoPorTipo:     enviadoPorTipo,
		CodigoAcademia:     codigoAcademia,
		CreatedAt:          time.Now(),
	}
	m.RaiseEvent(e)
	return m.Apply(e)
}

func (m *MensagemComunicacao) applyRegistrada(e DomainEvent) error {
	b, err := json.Marshal(e.GetPayload())
	if err != nil {
		return err
	}
	var p MensagemComunicacaoRegistradaEvent
	if err = json.Unmarshal(b, &p); err != nil {
		return err
	}
	m.Destinatario = p.Destinatario
	m.Conteudo = p.Conteudo
	m.ProvedorTentado1 = p.ProvedorTentado1
	m.ProvedorTentado2 = p.ProvedorTentado2
	m.ProvedorUtilizado = p.ProvedorUtilizado
	m.Status = p.Status
	m.MensagemExternaID = p.MensagemExternaID
	m.DetalhesTentativas = p.DetalhesTentativas
	m.EnviadoPor = p.EnviadoPor
	m.EnviadoPorTipo = p.EnviadoPorTipo
	m.CodigoAcademia = p.CodigoAcademia
	m.CreatedAt = p.CreatedAt
	return nil
}
```

## 4. Registar os dois agregados na factory

### Localizar este bloco exato (`internal/domain/aggregates/aggregate.go`)

```go
	case "SolicitacaoAlteracaoNIFAcademia":
		return NewSolicitacaoAlteracaoNIFAcademia(), nil
	case "Financeiro":
		return NewFinanceiro(), nil
	default:
		log.Printf("[ERROR] Tipo de agregado desconhecido: %s", aggregateType)
		return nil, fmt.Errorf("tipo de agregado desconhecido: %s", aggregateType)
```

### Substituir por

```go
	case "SolicitacaoAlteracaoNIFAcademia":
		return NewSolicitacaoAlteracaoNIFAcademia(), nil
	case "Financeiro":
		return NewFinanceiro(), nil
	case "RemetenteComunicacao":
		return NewRemetenteComunicacao(), nil
	case "MensagemComunicacao":
		return NewMensagemComunicacao(), nil
	default:
		log.Printf("[ERROR] Tipo de agregado desconhecido: %s", aggregateType)
		return nil, fmt.Errorf("tipo de agregado desconhecido: %s", aggregateType)
```


## 5. Projeções novas

Crie exatamente estes 2 arquivos.

### `internal/projections/remetente_comunicacao_projection.go`

```go
package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

type RemetenteComunicacaoProjection struct{ client *db.Client }

func NewRemetenteComunicacaoProjection(c *db.Client) *RemetenteComunicacaoProjection {
	return &RemetenteComunicacaoProjection{c}
}
func (p *RemetenteComunicacaoProjection) Name() string { return "remetentes_comunicacao" }
func (p *RemetenteComunicacaoProjection) GetLastProcessedEventID() (int64, error) {
	var v int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
func (p *RemetenteComunicacaoProjection) UpdateCheckpoint(id int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT(projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return err
}
func (p *RemetenteComunicacaoProjection) Handle(e db.Event) error {
	if e.AggregateType != "RemetenteComunicacao" {
		return nil
	}
	switch e.EventType {
	case "RemetenteComunicacaoConfigurado":
		return p.configurado(e)
	}
	return nil
}
func (p *RemetenteComunicacaoProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE projection_remetentes_comunicacao CASCADE`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='RemetenteComunicacao' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e db.Event
		var prev sql.NullString
		if err = rows.Scan(&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.EventVersion, &e.Payload, &e.Metadata, &e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &prev); err != nil {
			return err
		}
		if prev.Valid {
			e.PreviousHash = &prev.String
		}
		if err = p.Handle(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

// RemetenteComunicacaoDTO é a representação PÚBLICA (usada nas respostas
// HTTP de "listar remetentes"). Nunca inclui o token de API — apenas o
// indicador booleano TokenConfigurado.
type RemetenteComunicacaoDTO struct {
	ID                 uuid.UUID `json:"id"`
	Provedor           string    `json:"provedor"`
	Identificador      string    `json:"identificador"`
	TokenConfigurado   bool      `json:"token_configurado"`
	ConfiguradoPor     uuid.UUID `json:"configurado_por"`
	ConfiguradoPorTipo string    `json:"configurado_por_tipo"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// RemetenteComunicacaoCredenciais é a representação INTERNA usada apenas
// pelo fluxo de envio de mensagem (comunicacao_handlers.go), para decifrar
// o token e chamar o provedor. Nunca é serializada numa resposta HTTP.
type RemetenteComunicacaoCredenciais struct {
	Identificador   string
	TokenAPICifrado string
}

func (p *RemetenteComunicacaoProjection) configurado(e db.Event) error {
	var x struct {
		Provedor           string
		Identificador      string
		TokenAPICifrado    string
		ConfiguradoPor     uuid.UUID
		ConfiguradoPorTipo string
		ConfiguradoEm      time.Time
	}
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_remetentes_comunicacao (id, provedor, identificador, token_api_cifrado, configurado_por, configurado_por_tipo, created_at, updated_at, version, last_event_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET
			identificador = EXCLUDED.identificador,
			token_api_cifrado = EXCLUDED.token_api_cifrado,
			configurado_por = EXCLUDED.configurado_por,
			configurado_por_tipo = EXCLUDED.configurado_por_tipo,
			updated_at = EXCLUDED.updated_at,
			version = EXCLUDED.version,
			last_event_id = EXCLUDED.last_event_id
	`, e.AggregateID, x.Provedor, x.Identificador, x.TokenAPICifrado, x.ConfiguradoPor, x.ConfiguradoPorTipo, x.ConfiguradoEm, e.EventVersion, e.EventID)
	return err
}

const remetenteComunicacaoCols = `id,provedor,identificador,configurado_por,configurado_por_tipo,created_at,updated_at`

func (p *RemetenteComunicacaoProjection) scan(row interface{ Scan(...interface{}) error }) (*RemetenteComunicacaoDTO, error) {
	var d RemetenteComunicacaoDTO
	if err := row.Scan(&d.ID, &d.Provedor, &d.Identificador, &d.ConfiguradoPor, &d.ConfiguradoPorTipo, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}
	d.TokenConfigurado = true
	return &d, nil
}

// GetByProvedor devolve a representação pública (sem token) do remetente
// configurado para o provedor, ou nil se ainda não houver nenhum.
func (p *RemetenteComunicacaoProjection) GetByProvedor(provedor string) (*RemetenteComunicacaoDTO, error) {
	d, err := p.scan(p.client.DB().QueryRow(`SELECT `+remetenteComunicacaoCols+` FROM projection_remetentes_comunicacao WHERE provedor=$1`, provedor))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// GetCredenciaisByProvedor devolve o identificador e o token AINDA
// CIFRADO do remetente configurado para o provedor, ou nil se ainda não
// houver nenhum. Uso exclusivo do fluxo de envio — nunca expor em resposta
// HTTP.
func (p *RemetenteComunicacaoProjection) GetCredenciaisByProvedor(provedor string) (*RemetenteComunicacaoCredenciais, error) {
	var c RemetenteComunicacaoCredenciais
	err := p.client.DB().QueryRow(`SELECT identificador, token_api_cifrado FROM projection_remetentes_comunicacao WHERE provedor=$1`, provedor).Scan(&c.Identificador, &c.TokenAPICifrado)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List devolve todos os remetentes configurados (no máximo dois: um por
// provedor), ordenados por provedor.
func (p *RemetenteComunicacaoProjection) List() ([]RemetenteComunicacaoDTO, error) {
	rows, err := p.client.DB().Query(`SELECT ` + remetenteComunicacaoCols + ` FROM projection_remetentes_comunicacao ORDER BY provedor`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RemetenteComunicacaoDTO{}
	for rows.Next() {
		d, err := p.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar remetentes de comunicação: %w", err)
	}
	return out, nil
}
```

### `internal/projections/mensagem_comunicacao_projection.go`

```go
package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

type MensagemComunicacaoProjection struct{ client *db.Client }

func NewMensagemComunicacaoProjection(c *db.Client) *MensagemComunicacaoProjection {
	return &MensagemComunicacaoProjection{c}
}
func (p *MensagemComunicacaoProjection) Name() string { return "mensagens_comunicacao" }
func (p *MensagemComunicacaoProjection) GetLastProcessedEventID() (int64, error) {
	var v int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
func (p *MensagemComunicacaoProjection) UpdateCheckpoint(id int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT(projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return err
}
func (p *MensagemComunicacaoProjection) Handle(e db.Event) error {
	if e.AggregateType != "MensagemComunicacao" {
		return nil
	}
	switch e.EventType {
	case "MensagemComunicacaoRegistrada":
		return p.registrada(e)
	}
	return nil
}
func (p *MensagemComunicacaoProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE projection_mensagens_comunicacao CASCADE`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='MensagemComunicacao' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e db.Event
		var prev sql.NullString
		if err = rows.Scan(&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.EventVersion, &e.Payload, &e.Metadata, &e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &prev); err != nil {
			return err
		}
		if prev.Valid {
			e.PreviousHash = &prev.String
		}
		if err = p.Handle(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

type MensagemComunicacaoDTO struct {
	ID                 uuid.UUID                             `json:"id"`
	Destinatario       string                                `json:"destinatario"`
	Conteudo           string                                `json:"conteudo"`
	ProvedorTentado1   string                                `json:"provedor_tentado_1"`
	ProvedorTentado2   string                                `json:"provedor_tentado_2,omitempty"`
	ProvedorUtilizado  string                                `json:"provedor_utilizado,omitempty"`
	Status             string                                `json:"status"`
	MensagemExternaID  string                                `json:"mensagem_externa_id,omitempty"`
	DetalhesTentativas []aggregatesTentativaEnvioComunicacao `json:"detalhes_tentativas"`
	EnviadoPor         uuid.UUID                             `json:"enviado_por"`
	EnviadoPorTipo     string                                `json:"enviado_por_tipo"`
	CodigoAcademia     string                                `json:"codigo_academia,omitempty"`
	CreatedAt          time.Time                             `json:"created_at"`
}

// aggregatesTentativaEnvioComunicacao espelha
// aggregates.TentativaEnvioComunicacao (mesmos nomes/tags de JSON) — este
// pacote (projections) não importa internal/domain/aggregates para não
// criar uma dependência circular com internal/handlers, então o payload é
// decodificado para este tipo espelhado local em vez do tipo do agregado.
type aggregatesTentativaEnvioComunicacao struct {
	Provedor          string `json:"provedor"`
	Sucesso           bool   `json:"sucesso"`
	MensagemExternaID string `json:"mensagem_externa_id,omitempty"`
	ErroCodigo        string `json:"erro_codigo,omitempty"`
	ErroMensagem      string `json:"erro_mensagem,omitempty"`
}

func (p *MensagemComunicacaoProjection) registrada(e db.Event) error {
	var x struct {
		Destinatario       string
		Conteudo           string
		ProvedorTentado1   string
		ProvedorTentado2   string
		ProvedorUtilizado  string
		Status             string
		MensagemExternaID  string
		DetalhesTentativas []aggregatesTentativaEnvioComunicacao
		EnviadoPor         uuid.UUID
		EnviadoPorTipo     string
		CodigoAcademia     string
		CreatedAt          time.Time
	}
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	detalhesJSON, err := json.Marshal(x.DetalhesTentativas)
	if err != nil {
		return err
	}
	var provedorTentado2, provedorUtilizado, mensagemExternaID, codigoAcademia interface{}
	if x.ProvedorTentado2 != "" {
		provedorTentado2 = x.ProvedorTentado2
	}
	if x.ProvedorUtilizado != "" {
		provedorUtilizado = x.ProvedorUtilizado
	}
	if x.MensagemExternaID != "" {
		mensagemExternaID = x.MensagemExternaID
	}
	if x.CodigoAcademia != "" {
		codigoAcademia = x.CodigoAcademia
	}
	_, err = p.client.DB().Exec(`
		INSERT INTO projection_mensagens_comunicacao (
			id, destinatario, conteudo, provedor_tentado_1, provedor_tentado_2, provedor_utilizado,
			status, mensagem_externa_id, detalhes_tentativas, enviado_por, enviado_por_tipo, codigo_academia,
			created_at, updated_at, version, last_event_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13,$14,$15)
		ON CONFLICT (id) DO NOTHING
	`, e.AggregateID, x.Destinatario, x.Conteudo, x.ProvedorTentado1, provedorTentado2, provedorUtilizado,
		x.Status, mensagemExternaID, detalhesJSON, x.EnviadoPor, x.EnviadoPorTipo, codigoAcademia,
		x.CreatedAt, e.EventVersion, e.EventID)
	return err
}

const mensagemComunicacaoCols = `id,destinatario,conteudo,provedor_tentado_1,coalesce(provedor_tentado_2,''),coalesce(provedor_utilizado,''),status,coalesce(mensagem_externa_id,''),detalhes_tentativas,enviado_por,enviado_por_tipo,coalesce(codigo_academia,''),created_at`

func (p *MensagemComunicacaoProjection) scan(row interface{ Scan(...interface{}) error }) (*MensagemComunicacaoDTO, error) {
	var d MensagemComunicacaoDTO
	var detalhesRaw []byte
	if err := row.Scan(&d.ID, &d.Destinatario, &d.Conteudo, &d.ProvedorTentado1, &d.ProvedorTentado2, &d.ProvedorUtilizado,
		&d.Status, &d.MensagemExternaID, &detalhesRaw, &d.EnviadoPor, &d.EnviadoPorTipo, &d.CodigoAcademia, &d.CreatedAt); err != nil {
		return nil, err
	}
	if len(detalhesRaw) > 0 {
		if err := json.Unmarshal(detalhesRaw, &d.DetalhesTentativas); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

// ListFiltro controla a listagem: CodigoAcademia (quando preenchido)
// restringe às mensagens enviadas por essa academia; quando vazio E
// SomenteAcademia for false, lista de todas as origens (uso exclusivo de
// administradores).
type MensagemComunicacaoListFiltro struct {
	CodigoAcademia string
	Limit          int
	Offset         int
}

// List devolve as mensagens mais recentes primeiro. Quando filtro.CodigoAcademia
// estiver preenchido, restringe às mensagens enviadas por essa academia —
// usado tanto para "uma academia só vê as suas" quanto para o filtro
// opcional de um admin.
func (p *MensagemComunicacaoProjection) List(filtro MensagemComunicacaoListFiltro) ([]MensagemComunicacaoDTO, int, error) {
	where := ""
	args := []interface{}{}
	if filtro.CodigoAcademia != "" {
		where = "WHERE codigo_academia = $1"
		args = append(args, filtro.CodigoAcademia)
	}
	var total int
	countQuery := "SELECT COUNT(*) FROM projection_mensagens_comunicacao " + where
	if err := p.client.DB().QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("contar mensagens de comunicação: %w", err)
	}
	args = append(args, filtro.Limit, filtro.Offset)
	limitPos := len(args) - 1
	offsetPos := len(args)
	query := fmt.Sprintf(`SELECT %s FROM projection_mensagens_comunicacao %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		mensagemComunicacaoCols, where, limitPos, offsetPos)
	rows, err := p.client.DB().Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listar mensagens de comunicação: %w", err)
	}
	defer rows.Close()
	out := []MensagemComunicacaoDTO{}
	for rows.Next() {
		d, err := p.scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("listar mensagens de comunicação: %w", err)
	}
	return out, total, nil
}
```

## 6. Serviços novos (cifra do token + clientes HTTP dos provedores)

Crie exatamente estes 3 arquivos em `internal/services/`. **Não modifique nem importe** `ziett_sms_test_client.go` — são módulos independentes por design (ver Contexto).

### `internal/services/comunicacao_crypto.go`

```go
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
```

### `internal/services/comunicacao_gosms_client.go`

```go
package services

// Cliente HTTP para a API da GoSMS (docs/Parceiros e integrações/GoSMS API
// - Documentação.md, seção 5.1 — POST /v1/messages), usado pelo módulo real
// de comunicação (internal/handlers/comunicacao_handlers.go). Isolado dos
// outros domínios (não importa internal/db, internal/domain/aggregates nem
// internal/finance) — recebe sempre o token de API já decifrado pelo
// chamador, nunca lê variáveis de ambiente diretamente.
//
// Não reutiliza nem depende de ziett_sms_test_client.go — aquele arquivo é
// exclusivo da rota isolada de teste POST /integracoes/ziett/mensagens/teste
// (Tarefa 20) e deve continuar isolado.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const goSMSBaseURL = "https://api.go-sms.co.ao"

type ComunicacaoGoSMSClient struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewComunicacaoGoSMSClient recebe o token de API já decifrado (nunca lido
// de variável de ambiente — vem de projection_remetentes_comunicacao,
// decifrado com services.DecryptComunicacaoSegredo).
func NewComunicacaoGoSMSClient(token string) *ComunicacaoGoSMSClient {
	return &ComunicacaoGoSMSClient{token: token, httpClient: &http.Client{Timeout: 15 * time.Second}, baseURL: goSMSBaseURL}
}

// ComunicacaoGoSMSAPIError modela os dois formatos de erro já observados na
// GoSMS (ver seção 4 da documentação): "errors" como objeto único
// {message, code} OU como array de objetos {message}. Os dois formatos são
// tentados na decodificação (ver parseGoSMSError).
type ComunicacaoGoSMSAPIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *ComunicacaoGoSMSAPIError) Error() string {
	if e == nil {
		return "erro da GoSMS"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return fmt.Sprintf("erro da GoSMS com status %d", e.StatusCode)
}

type ComunicacaoGoSMSNetworkError struct{ Err error }

func (e *ComunicacaoGoSMSNetworkError) Error() string { return "falha ao contactar a GoSMS" }
func (e *ComunicacaoGoSMSNetworkError) Unwrap() error { return e.Err }

func parseGoSMSError(statusCode int, body []byte) *ComunicacaoGoSMSAPIError {
	// Formato 1: {"errors": {"message": "...", "code": "..."}}
	var objErr struct {
		Errors struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &objErr); err == nil && objErr.Errors.Message != "" {
		return &ComunicacaoGoSMSAPIError{StatusCode: statusCode, Code: objErr.Errors.Code, Message: objErr.Errors.Message}
	}
	// Formato 2: {"errors": [{"message": "..."}]}
	var arrErr struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &arrErr); err == nil && len(arrErr.Errors) > 0 && arrErr.Errors[0].Message != "" {
		return &ComunicacaoGoSMSAPIError{StatusCode: statusCode, Message: arrErr.Errors[0].Message}
	}
	return &ComunicacaoGoSMSAPIError{StatusCode: statusCode, Message: "a GoSMS retornou erro sem corpo reconhecido"}
}

// EnviarSMS envia uma SMS via POST /v1/messages. destinatarioNacional deve
// já vir validado no formato nacional angolano de 9 dígitos (sem "0"
// inicial, sem "+244") — a GoSMS aceita esse formato diretamente em "to"
// (ver exemplo "921939411" na documentação, seção 5.1). Retorna o "id" do
// envio (o "lote", campo `id` da resposta 201) para gravar como
// mensagem_externa_id.
func (c *ComunicacaoGoSMSClient) EnviarSMS(ctx context.Context, remetente, destinatarioNacional, conteudo string) (string, error) {
	payload := map[string]string{
		"message": conteudo,
		"from":    remetente,
		"to":      destinatarioNacional,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &ComunicacaoGoSMSNetworkError{Err: err}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		var parsed struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return "", err
		}
		if parsed.ID == "" {
			return "", errors.New("resposta da GoSMS sem id de envio")
		}
		return parsed.ID, nil
	}
	return "", parseGoSMSError(resp.StatusCode, respBody)
}
```

### `internal/services/comunicacao_ziett_client.go`

```go
package services

// Cliente HTTP para a API da Ziett (docs/Parceiros e integrações/Ziett API
// - Documentação.md, seção 5 — POST /messages), usado pelo módulo real de
// comunicação (internal/handlers/comunicacao_handlers.go). Isolado dos
// outros domínios (não importa internal/db, internal/domain/aggregates nem
// internal/finance) — recebe sempre o token de API já decifrado pelo
// chamador, nunca lê variáveis de ambiente diretamente.
//
// Não reutiliza nem depende de ziett_sms_test_client.go — aquele arquivo é
// exclusivo da rota isolada de teste POST /integracoes/ziett/mensagens/teste
// (Tarefa 20) e deve continuar isolado. A lógica de normalização de
// telefone abaixo (formatarDestinatarioZiettE164) é uma cópia adaptada e
// independente da mesma regra já validada naquele arquivo — não a mesma
// função, para não criar acoplamento entre os dois módulos.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const (
	comunicacaoZiettBaseURL    = "https://api.ziett.co/c/v1"
	comunicacaoZiettChannelSMS = "SMS"
)

type ComunicacaoZiettClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewComunicacaoZiettClient recebe o token de API já decifrado (nunca lido
// de variável de ambiente — vem de projection_remetentes_comunicacao,
// decifrado com services.DecryptComunicacaoSegredo).
func NewComunicacaoZiettClient(apiKey string) *ComunicacaoZiettClient {
	return &ComunicacaoZiettClient{apiKey: apiKey, httpClient: &http.Client{Timeout: 15 * time.Second}, baseURL: comunicacaoZiettBaseURL}
}

type ComunicacaoZiettAPIError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Status    int                    `json:"status"`
	TraceID   string                 `json:"trace_id"`
	Timestamp string                 `json:"timestamp"`
	Service   string                 `json:"service"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func (e *ComunicacaoZiettAPIError) Error() string {
	if e == nil {
		return "erro da Ziett"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return fmt.Sprintf("erro da Ziett com status %d", e.Status)
}

type ComunicacaoZiettNetworkError struct{ Err error }

func (e *ComunicacaoZiettNetworkError) Error() string { return "falha ao contactar a Ziett" }
func (e *ComunicacaoZiettNetworkError) Unwrap() error { return e.Err }

// formatarDestinatarioZiettE164 converte o formato nacional angolano de 9
// dígitos (sem "0" inicial, sem "+244") — o mesmo formato aceito
// diretamente pela GoSMS — para o E.164 completo que a Ziett exige em
// target_e164. Aceita defensivamente um "0" inicial ou um prefixo
// "+244"/"244" recebido por engano, exatamente como a normalização já
// validada em ziett_sms_test_client.go, mas reimplementada aqui de forma
// independente (ver nota de isolamento no topo do arquivo).
func formatarDestinatarioZiettE164(destinatarioNacional string) (string, error) {
	clean := strings.TrimSpace(destinatarioNacional)
	clean = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(clean)
	if clean == "" {
		return "", errors.New("destinatário é obrigatório e deve usar o número nacional angolano de 9 dígitos, exemplo: 923456789")
	}
	if strings.HasPrefix(clean, "+244") {
		clean = strings.TrimPrefix(clean, "+244")
	} else if strings.HasPrefix(clean, "244") {
		clean = strings.TrimPrefix(clean, "244")
	}
	if strings.HasPrefix(clean, "0") {
		clean = strings.TrimPrefix(clean, "0")
	}
	for _, r := range clean {
		if !unicode.IsDigit(r) {
			return "", errors.New("destinatário deve conter apenas dígitos após normalização, no formato nacional angolano de 9 dígitos, exemplo: 923456789")
		}
	}
	if len(clean) != 9 || !strings.HasPrefix(clean, "9") {
		return "", errors.New("destinatário deve ser um número móvel angolano de 9 dígitos iniciado por 9, sem 0 inicial e sem +244, exemplo: 923456789")
	}
	return "+244" + clean, nil
}

// EnviarSMS envia uma SMS via POST /messages, com channel_type sempre fixo
// em "SMS". remitterID é o identificador (UUID) do remetente configurado
// no painel da Ziett. destinatarioNacional deve vir no formato nacional
// angolano de 9 dígitos — esta função monta o E.164 completo antes de
// enviar. Retorna o message_id (202 Accepted) para gravar como
// mensagem_externa_id.
func (c *ComunicacaoZiettClient) EnviarSMS(ctx context.Context, remitterID, destinatarioNacional, conteudo string) (string, error) {
	target, err := formatarDestinatarioZiettE164(destinatarioNacional)
	if err != nil {
		return "", err
	}
	payload := map[string]string{
		"remitter_id":  remitterID,
		"channel_type": comunicacaoZiettChannelSMS,
		"target_e164":  target,
		"content":      conteudo,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &ComunicacaoZiettNetworkError{Err: err}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusAccepted {
		var parsed struct {
			MessageID string `json:"message_id"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return "", err
		}
		return parsed.MessageID, nil
	}
	var apiErr ComunicacaoZiettAPIError
	if err := json.Unmarshal(respBody, &apiErr); err != nil || (apiErr.Code == "" && apiErr.Message == "") {
		apiErr = ComunicacaoZiettAPIError{Code: "ZIETT_ERROR", Message: "a Ziett retornou erro sem corpo padronizado", Status: resp.StatusCode}
	}
	if apiErr.Status == 0 {
		apiErr.Status = resp.StatusCode
	}
	return "", &apiErr
}
```

## 7. Handlers novos

Crie exatamente este arquivo.

### `internal/handlers/comunicacao_handlers.go`

```go
package handlers

// Módulo de comunicação (base): cadastro de remetentes globais (GoSMS ou
// Ziett, com token de API cifrado), envio de mensagem com fallback
// automático de provedor, listagem de mensagens e de remetentes, e
// configuração do provedor padrão. Ver docs/Parceiros e integrações/GoSMS
// API - Documentação.md e Ziett API - Documentação.md para o contrato de
// cada provedor.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/services"
	"spuri/internal/utils"
)

// ── Helpers específicos deste módulo ──────────────────────────────────

// comunicacaoActor identifica quem está a chamar a rota: um admin (sem
// codigo_academia) ou uma academia autenticada (com o seu codigo_academia).
// Usado pelas rotas de duplo acesso (enviar mensagem, listar mensagens).
func comunicacaoActor(c *gin.Context) (userID uuid.UUID, userType string, codigoAcademia string, ok bool) {
	id, exists := middleware.GetUserID(c)
	if !exists {
		utils.RespondWithUnauthorizedError(c)
		return uuid.Nil, "", "", false
	}
	t, exists := middleware.GetUserType(c)
	if !exists {
		utils.RespondWithUnauthorizedError(c)
		return uuid.Nil, "", "", false
	}
	if t == "academia" {
		academia, err := getAcademiaProjection(c).GetByID(id)
		if err != nil || academia == nil {
			utils.RespondWithForbiddenError(c, "academia não encontrada")
			return uuid.Nil, "", "", false
		}
		return id, "academia", academia.CodigoAcademia, true
	}
	return id, "admin", "", true
}

func outroProvedorComunicacao(provedor string) string {
	if provedor == aggregates.ProvedorComunicacaoGoSMS {
		return aggregates.ProvedorComunicacaoZiett
	}
	return aggregates.ProvedorComunicacaoGoSMS
}

var destinatarioNacionalComunicacaoRegex = regexp.MustCompile(`^9\d{8}$`)

// normalizarDestinatarioComunicacao aceita o número com ou sem prefixo
// "+244"/"244"/"0" e devolve sempre o formato nacional angolano de 9
// dígitos (o mesmo aceito diretamente pela GoSMS e usado para montar o
// E.164 da Ziett).
func normalizarDestinatarioComunicacao(v string) (string, error) {
	clean := strings.TrimSpace(v)
	clean = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(clean)
	if strings.HasPrefix(clean, "+244") {
		clean = strings.TrimPrefix(clean, "+244")
	} else if strings.HasPrefix(clean, "244") && len(clean) == 12 {
		clean = strings.TrimPrefix(clean, "244")
	}
	if strings.HasPrefix(clean, "0") && len(clean) == 10 {
		clean = strings.TrimPrefix(clean, "0")
	}
	if !destinatarioNacionalComunicacaoRegex.MatchString(clean) {
		return "", fmt.Errorf("destinatário deve ser um número móvel angolano válido, no formato nacional de 9 dígitos iniciado por 9 (ex.: 923456789), com ou sem prefixo +244")
	}
	return clean, nil
}

const conteudoMensagemComunicacaoMaxLen = 1000

func enviarViaProvedorComunicacao(ctx context.Context, provedor string, credenciais *projections.RemetenteComunicacaoCredenciais, destinatario, conteudo string) (string, error) {
	tokenPlano, err := services.DecryptComunicacaoSegredo(credenciais.TokenAPICifrado)
	if err != nil {
		return "", fmt.Errorf("falha ao decifrar o token do provedor %s: %w", provedor, err)
	}
	switch provedor {
	case aggregates.ProvedorComunicacaoGoSMS:
		return services.NewComunicacaoGoSMSClient(tokenPlano).EnviarSMS(ctx, credenciais.Identificador, destinatario, conteudo)
	case aggregates.ProvedorComunicacaoZiett:
		return services.NewComunicacaoZiettClient(tokenPlano).EnviarSMS(ctx, credenciais.Identificador, destinatario, conteudo)
	default:
		return "", fmt.Errorf("provedor desconhecido: %s", provedor)
	}
}

// resolverRemetenteAggregate carrega o RemetenteComunicacao existente do
// provedor (via ledger) ou cria um novo com o ID determinístico do
// provedor, caso ainda não exista nenhum. Nunca cria um agregado com ID
// aleatório para este tipo.
func resolverRemetenteAggregate(c *gin.Context, provedor string) (*aggregates.RemetenteComunicacao, error) {
	aggID, known := aggregates.RemetenteAggregateID(provedor)
	if !known {
		return nil, fmt.Errorf("provedor inválido: use GOSMS ou ZIETT")
	}
	loaded, err := getRepository(c).Load(aggID, "RemetenteComunicacao")
	if err != nil {
		novo := aggregates.NewRemetenteComunicacao()
		novo.SetID(aggID)
		return novo, nil
	}
	agg, ok := loaded.(*aggregates.RemetenteComunicacao)
	if !ok {
		return nil, fmt.Errorf("tipo inesperado ao carregar RemetenteComunicacao")
	}
	return agg, nil
}

// ── 1. Cadastrar remetente (rota única — FPP admin, GOSMS ou ZIETT) ────

type criarRemetenteComunicacaoRequest struct {
	Provedor      string `json:"provedor" binding:"required"`
	Identificador string `json:"identificador" binding:"required"`
	TokenAPI      string `json:"token_api" binding:"required"`
}

// CriarRemetenteComunicacao cadastra (primeira vez) ou substitui
// (chamadas seguintes) as credenciais de envio de um provedor. Remetente é
// sempre global — não pertence a nenhuma academia — e existe no máximo um
// por provedor. Não chama nenhuma API do GoSMS/Ziett: apenas grava, no
// nosso próprio banco, o identificador (nome do Sender ID no GoSMS, ou
// UUID do remitter_id no Ziett) e o token de API, cifrado.
func CriarRemetenteComunicacao(c *gin.Context) {
	if err := verificarPermissaoAdmin(c, "fpp"); err != nil {
		utils.RespondWithForbiddenError(c, "apenas administradores FPP podem cadastrar remetentes de comunicação")
		return
	}
	userID, exists := middleware.GetUserID(c)
	if !exists {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	var req criarRemetenteComunicacaoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	agg, err := resolverRemetenteAggregate(c, strings.ToUpper(strings.TrimSpace(req.Provedor)))
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	eraNovo := agg.Version == 0

	tokenCifrado, err := services.EncryptComunicacaoSegredo(req.TokenAPI)
	if err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("falha ao cifrar o token de API: %w", err))
		return
	}

	if err := agg.Configurar(req.Provedor, req.Identificador, tokenCifrado, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{UserID: userID.String(), UserType: "admin", IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	status := http.StatusOK
	if eraNovo {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{
		"id":                   agg.ID,
		"provedor":             agg.Provedor,
		"identificador":        agg.Identificador,
		"token_configurado":    true,
		"configurado_por":      agg.ConfiguradoPor,
		"configurado_por_tipo": agg.ConfiguradoPorTipo,
		"created_at":           agg.CreatedAt,
		"updated_at":           agg.UpdatedAt,
	})
}

// ── 3. Listar remetentes (rota única — apenas administradores) ─────────

// ListarRemetentesComunicacao devolve os remetentes globais configurados
// (no máximo dois: um por provedor). Nunca inclui o token de API — apenas
// o indicador booleano token_configurado.
func ListarRemetentesComunicacao(c *gin.Context) {
	if err := verificarPermissaoAdmin(c, "gerente"); err != nil {
		utils.RespondWithForbiddenError(c, "apenas administradores podem listar remetentes de comunicação")
		return
	}
	lista, err := getRemetentesComunicacaoProjection(c).List()
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"remetentes": lista})
}

// ── 2. Enviar mensagem (rota única — administradores e academias) ──────

type enviarMensagemComunicacaoRequest struct {
	Destinatario string `json:"destinatario" binding:"required"`
	Conteudo     string `json:"conteudo" binding:"required"`
}

// EnviarMensagemComunicacao envia UMA mensagem para UM destinatário.
// Tenta primeiro o provedor definido como padrão (ver
// DefinirProvedorPadraoComunicacao); se esse provedor não tiver remetente
// configurado ou a tentativa de envio falhar, tenta automaticamente o
// outro provedor. A mensagem é sempre registada (sucesso ou falha) em
// projection_mensagens_comunicacao, com o detalhe de cada tentativa.
func EnviarMensagemComunicacao(c *gin.Context) {
	userID, userType, codigoAcademia, ok := comunicacaoActor(c)
	if !ok {
		return
	}

	var req enviarMensagemComunicacaoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	destinatario, err := normalizarDestinatarioComunicacao(req.Destinatario)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	conteudo := strings.TrimSpace(req.Conteudo)
	if conteudo == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("conteudo é obrigatório"))
		return
	}
	if len(conteudo) > conteudoMensagemComunicacaoMaxLen {
		utils.RespondWithValidationError(c, fmt.Errorf("conteudo excede o limite de %d caracteres", conteudoMensagemComunicacaoMaxLen))
		return
	}

	var provedorPadrao sql.NullString
	if err := getDbClient(c).DB().QueryRow(`SELECT provedor_padrao FROM projection_comunicacao_config WHERE id = 1`).Scan(&provedorPadrao); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if !provedorPadrao.Valid || provedorPadrao.String == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nenhum provedor padrão foi configurado ainda — um administrador FPP precisa de definir o provedor padrão antes de enviar mensagens"))
		return
	}

	ordemProvedores := []string{provedorPadrao.String, outroProvedorComunicacao(provedorPadrao.String)}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	var provedorTentado2, provedorUtilizado, mensagemExternaID string
	detalhes := []aggregates.TentativaEnvioComunicacao{}

	for i, provedor := range ordemProvedores {
		if i == 1 {
			provedorTentado2 = provedor
		}
		credenciais, err := getRemetentesComunicacaoProjection(c).GetCredenciaisByProvedor(provedor)
		if err != nil {
			detalhes = append(detalhes, aggregates.TentativaEnvioComunicacao{Provedor: provedor, Sucesso: false, ErroMensagem: "falha ao consultar remetente configurado"})
			continue
		}
		if credenciais == nil {
			detalhes = append(detalhes, aggregates.TentativaEnvioComunicacao{Provedor: provedor, Sucesso: false, ErroMensagem: "nenhum remetente configurado para este provedor"})
			continue
		}
		externalID, sendErr := enviarViaProvedorComunicacao(ctx, provedor, credenciais, destinatario, conteudo)
		if sendErr != nil {
			detalhes = append(detalhes, aggregates.TentativaEnvioComunicacao{Provedor: provedor, Sucesso: false, ErroMensagem: sendErr.Error()})
			continue
		}
		detalhes = append(detalhes, aggregates.TentativaEnvioComunicacao{Provedor: provedor, Sucesso: true, MensagemExternaID: externalID})
		provedorUtilizado = provedor
		mensagemExternaID = externalID
		break
	}

	status := aggregates.StatusMensagemComunicacaoFalhou
	if provedorUtilizado != "" {
		status = aggregates.StatusMensagemComunicacaoEnviada
	}

	msg := aggregates.NewMensagemComunicacao()
	if err := msg.Registrar(destinatario, conteudo, provedorPadrao.String, provedorTentado2, provedorUtilizado, status, mensagemExternaID, detalhes, userID, userType, codigoAcademia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	audit := db.AuditContext{UserID: userID.String(), UserType: userType, IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(msg, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	resposta := gin.H{
		"id":                  msg.ID,
		"destinatario":        msg.Destinatario,
		"status":              msg.Status,
		"provedor_utilizado":  msg.ProvedorUtilizado,
		"mensagem_externa_id": msg.MensagemExternaID,
		"detalhes_tentativas": msg.DetalhesTentativas,
		"created_at":          msg.CreatedAt,
	}
	if status == aggregates.StatusMensagemComunicacaoFalhou {
		utils.RespondWithErrorData(c, http.StatusBadGateway, "não foi possível enviar a mensagem em nenhum dos provedores configurados", fmt.Errorf("todas as tentativas de envio falharam"), resposta)
		return
	}
	c.JSON(http.StatusCreated, resposta)
}

// ── 3. Listar mensagens (rota única — administradores e academias) ─────

// ListarMensagensComunicacao devolve o histórico de mensagens enviadas.
// Uma academia só vê as mensagens que ela própria enviou. Um administrador
// vê todas, com filtro opcional por codigo_academia (?codigo_academia=).
func ListarMensagensComunicacao(c *gin.Context) {
	_, userType, codigoAcademia, ok := comunicacaoActor(c)
	if !ok {
		return
	}
	filtro := projections.MensagemComunicacaoListFiltro{}
	if userType == "academia" {
		filtro.CodigoAcademia = codigoAcademia
	} else {
		filtro.CodigoAcademia = strings.TrimSpace(c.Query("codigo_academia"))
	}
	filtro.Limit, filtro.Offset = getPaginationParams(c)

	lista, total, err := getMensagensComunicacaoProjection(c).List(filtro)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"mensagens": lista, "total": total, "limit": filtro.Limit, "offset": filtro.Offset})
}

// ── 2.1 Provedor padrão (FPP admin) ─────────────────────────────────────

type definirProvedorPadraoComunicacaoRequest struct {
	ProvedorPadrao string `json:"provedor_padrao" binding:"required"`
}

// DefinirProvedorPadraoComunicacao define qual provedor (GOSMS ou ZIETT)
// o sistema tenta primeiro ao enviar uma mensagem. É uma configuração
// única e global — não existe um provedor padrão por academia.
func DefinirProvedorPadraoComunicacao(c *gin.Context) {
	if err := verificarPermissaoAdmin(c, "fpp"); err != nil {
		utils.RespondWithForbiddenError(c, "apenas administradores FPP podem definir o provedor padrão")
		return
	}
	var req definirProvedorPadraoComunicacaoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	provedor := strings.ToUpper(strings.TrimSpace(req.ProvedorPadrao))
	if provedor != aggregates.ProvedorComunicacaoGoSMS && provedor != aggregates.ProvedorComunicacaoZiett {
		utils.RespondWithValidationError(c, fmt.Errorf("provedor_padrao deve ser GOSMS ou ZIETT"))
		return
	}
	userID, exists := middleware.GetUserID(c)
	if !exists {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	if _, err := getDbClient(c).DB().Exec(`UPDATE projection_comunicacao_config SET provedor_padrao = $1, atualizado_por = $2, atualizado_em = now() WHERE id = 1`, provedor, userID); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"provedor_padrao": provedor})
}

// ConsultarProvedorPadraoComunicacao devolve o provedor padrão atual
// (ou null, se nenhum admin FPP o tiver definido ainda).
func ConsultarProvedorPadraoComunicacao(c *gin.Context) {
	if err := verificarPermissaoAdmin(c, "fpp"); err != nil {
		utils.RespondWithForbiddenError(c, "apenas administradores FPP podem consultar o provedor padrão")
		return
	}
	var provedorPadrao sql.NullString
	var atualizadoEm sql.NullTime
	if err := getDbClient(c).DB().QueryRow(`SELECT provedor_padrao, atualizado_em FROM projection_comunicacao_config WHERE id = 1`).Scan(&provedorPadrao, &atualizadoEm); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	resp := gin.H{"provedor_padrao": nil, "atualizado_em": nil}
	if provedorPadrao.Valid {
		resp["provedor_padrao"] = provedorPadrao.String
	}
	if atualizadoEm.Valid {
		resp["atualizado_em"] = atualizadoEm.Time
	}
	c.JSON(http.StatusOK, resp)
}
```

## 8. Helpers de contexto (projeções via Gin)

### Localizar este bloco exato (`internal/handlers/helpers.go`)

```go
func getCategoriasServicoProjection(c *gin.Context) *projections.CategoriaServicoProjection {
	return projections.NewCategoriaServicoProjection(getDbClient(c))
}
func getSolicitacoesServicoExtraProjection(c *gin.Context) *projections.SolicitacaoServicoExtraProjection {
	return projections.NewSolicitacaoServicoExtraProjection(getDbClient(c))
}
```

### Substituir por

```go
func getCategoriasServicoProjection(c *gin.Context) *projections.CategoriaServicoProjection {
	return projections.NewCategoriaServicoProjection(getDbClient(c))
}
func getRemetentesComunicacaoProjection(c *gin.Context) *projections.RemetenteComunicacaoProjection {
	return projections.NewRemetenteComunicacaoProjection(getDbClient(c))
}
func getMensagensComunicacaoProjection(c *gin.Context) *projections.MensagemComunicacaoProjection {
	return projections.NewMensagemComunicacaoProjection(getDbClient(c))
}
func getSolicitacoesServicoExtraProjection(c *gin.Context) *projections.SolicitacaoServicoExtraProjection {
	return projections.NewSolicitacaoServicoExtraProjection(getDbClient(c))
}
```


## 9. `main.go` — import, validação de arranque, registo da projeção e rotas

### 9.1 — Localizar este bloco exato (import)

```go
	"spuri/internal/projections"
	"spuri/internal/storage"
	"spuri/internal/utils"
)
```

### Substituir por

```go
	"spuri/internal/projections"
	"spuri/internal/services"
	"spuri/internal/storage"
	"spuri/internal/utils"
)
```

### 9.2 — Localizar este bloco exato (validação de arranque em `initDB`)

```go
	if err := finance.ValidateAppyPayResourceConfig(); err != nil {
		return fmt.Errorf("configuração AppyPay inválida: %w", err)
	}
	config := db.DefaultConfig()
```

### Substituir por

```go
	if err := finance.ValidateAppyPayResourceConfig(); err != nil {
		return fmt.Errorf("configuração AppyPay inválida: %w", err)
	}
	if err := services.ValidateComunicacaoEncryptionConfig(); err != nil {
		return fmt.Errorf("configuração de criptografia de comunicação inválida: %w", err)
	}
	config := db.DefaultConfig()
```

### 9.3 — Localizar este bloco exato (registo de projeções em `initProjections`)

```go
	projManager.RegisterProjection("categorias_servico", projections.NewCategoriaServicoProjection(dbClient))
	projManager.RegisterProjection("solicitacoes_servico_extra", projections.NewSolicitacaoServicoExtraProjection(dbClient))
```

### Substituir por

```go
	projManager.RegisterProjection("categorias_servico", projections.NewCategoriaServicoProjection(dbClient))
	projManager.RegisterProjection("remetentes_comunicacao", projections.NewRemetenteComunicacaoProjection(dbClient))
	projManager.RegisterProjection("mensagens_comunicacao", projections.NewMensagemComunicacaoProjection(dbClient))
	projManager.RegisterProjection("solicitacoes_servico_extra", projections.NewSolicitacaoServicoExtraProjection(dbClient))
```

### 9.4 — Localizar este bloco exato (rotas, logo após a rota isolada de teste Ziett)

```go
	// ── Rota isolada de integração Ziett (teste de SMS) ──────────────────
	integracoes := router.Group("/integracoes")
	integracoes.Use(middleware.AuthMiddleware())
	integracoes.Use(middleware.RequireFPP())
	{
		integracoes.POST("/ziett/mensagens/teste", handlers.EnviarMensagemTesteZiettSMS)
	}

	// ── Rotas autenticadas (qualquer tipo) ────────────────────────────────
```

### Substituir por

```go
	// ── Rota isolada de integração Ziett (teste de SMS) ──────────────────
	integracoes := router.Group("/integracoes")
	integracoes.Use(middleware.AuthMiddleware())
	integracoes.Use(middleware.RequireFPP())
	{
		integracoes.POST("/ziett/mensagens/teste", handlers.EnviarMensagemTesteZiettSMS)
	}

	// ── Módulo de comunicação (base): enviar/listar mensagens ────────────
	// Disponível para administradores (qualquer role) e academias. Uma
	// academia só vê/envia em nome de si própria; um admin vê tudo, com
	// filtro opcional por codigo_academia na listagem.
	comunicacaoMensagens := router.Group("/comunicacao")
	comunicacaoMensagens.Use(middleware.AuthMiddleware())
	comunicacaoMensagens.Use(middleware.RequireAcademiaOuAdmin())
	{
		comunicacaoMensagens.POST("/mensagens", handlers.EnviarMensagemComunicacao)
		comunicacaoMensagens.GET("/mensagens", handlers.ListarMensagensComunicacao)
	}

	// ── Módulo de comunicação (base): remetentes (apenas administradores) ─
	// Cadastrar exige role FPP (checado dentro do handler); listar aceita
	// qualquer role de administrador.
	comunicacaoAdmin := router.Group("/comunicacao")
	comunicacaoAdmin.Use(middleware.AuthMiddleware())
	comunicacaoAdmin.Use(middleware.RequireAdmin())
	{
		comunicacaoAdmin.POST("/remetentes", handlers.CriarRemetenteComunicacao)
		comunicacaoAdmin.GET("/remetentes", handlers.ListarRemetentesComunicacao)
	}

	// ── Módulo de comunicação (base): provedor padrão (apenas FPP) ───────
	adminComunicacao := router.Group("/admin/comunicacao")
	adminComunicacao.Use(middleware.AuthMiddleware())
	adminComunicacao.Use(middleware.RequireAdmin())
	{
		adminComunicacao.GET("/provedor-padrao", handlers.ConsultarProvedorPadraoComunicacao)
		adminComunicacao.PUT("/provedor-padrao", handlers.DefinirProvedorPadraoComunicacao)
	}

	// ── Rotas autenticadas (qualquer tipo) ────────────────────────────────
```


## 10. `Documentação da API.md`

### Localizar este bloco exato (final do arquivo, fim da secção 23)

```
### 23.9 Pendências e pagamento
- `GET /estudante/servicos-extras/minhas-inscricoes/:id/pendencias` lista as pendências da própria inscrição.
- `GET /academia/servicos-extras/inscricoes/:id/pendencias` oferece a visão da academia.
- `POST /financeiro/servicos-extras/obrigacao/pagamento` inicia o pagamento de uma mensalidade ou preço único.
- `POST /financeiro/servicos-extras/obrigacao/anular` e `/reativar` administram uma obrigação individual da inscrição.
```

### Substituir por

```
### 23.9 Pendências e pagamento
- `GET /estudante/servicos-extras/minhas-inscricoes/:id/pendencias` lista as pendências da própria inscrição.
- `GET /academia/servicos-extras/inscricoes/:id/pendencias` oferece a visão da academia.
- `POST /financeiro/servicos-extras/obrigacao/pagamento` inicia o pagamento de uma mensalidade ou preço único.
- `POST /financeiro/servicos-extras/obrigacao/anular` e `/reativar` administram uma obrigação individual da inscrição.

## 24. Comunicação

Módulo de envio de SMS institucional via GoSMS ou Ziett. O remetente (Sender ID do GoSMS, ou remitter_id do Ziett) e o respetivo token de API são configurados uma única vez por provedor, diretamente no Spuri — nenhuma chamada é feita à API do provedor para "criar" o remetente; o Spuri apenas grava o que já existe e está aprovado do lado do provedor. Remetente é sempre global (não pertence a nenhuma academia).

### 24.1 Cadastrar remetente
`POST /comunicacao/remetentes` (admin FPP). Rota única para GOSMS ou ZIETT. **Request body:** `provedor` (`GOSMS` ou `ZIETT`), `identificador` (nome do Sender ID de 1 a 11 caracteres alfanuméricos maiúsculos para GOSMS; UUID do remitter_id para ZIETT), `token_api` (texto plano — é cifrado antes de ser gravado e nunca é devolvido em nenhuma resposta).

Existe no máximo um remetente por provedor. Cadastrar novamente para um provedor que já tem remetente configurado **substitui** o remetente existente (mesmo registo, histórico preservado no ledger de auditoria). Retorna `201` na primeira configuração de um provedor, `200` nas seguintes. A resposta nunca inclui o token — apenas `token_configurado: true`.

### 24.2 Listar remetentes
`GET /comunicacao/remetentes` (qualquer administrador). Devolve os remetentes configurados (no máximo dois — um por provedor), sem o token de API.

### 24.3 Provedor padrão
`GET /admin/comunicacao/provedor-padrao` e `PUT /admin/comunicacao/provedor-padrao` (admin FPP). Configuração única e global (não existe por academia). **Request body do PUT:** `provedor_padrao` (`GOSMS` ou `ZIETT`). Enquanto nenhum admin FPP tiver definido o provedor padrão, `POST /comunicacao/mensagens` responde `400` e não tenta enviar nada.

### 24.4 Enviar mensagem
`POST /comunicacao/mensagens` (administrador ou academia autenticada). Rota única — envia para **um** destinatário por chamada. **Request body:** `destinatario` (número móvel angolano; aceita com ou sem prefixo `+244`/`0`), `conteudo` (texto da SMS, até 1000 caracteres).

O sistema tenta primeiro o provedor padrão; se esse provedor não tiver remetente configurado, ou a tentativa de envio falhar, tenta automaticamente o outro provedor. A mensagem é sempre registada (`enviada` ou `falhou`), com o detalhe de cada tentativa (`provedor`, `sucesso`, `mensagem_externa_id` ou `erro_mensagem`) em `detalhes_tentativas`. Responde `201` quando pelo menos um provedor teve sucesso; `502` quando ambos falharam (o corpo da resposta de erro ainda inclui o registo completo da mensagem e das tentativas).

### 24.5 Listar mensagens
`GET /comunicacao/mensagens` (administrador ou academia autenticada). Aceita paginação (`?limit=&offset=`, limite padrão e máximo iguais aos demais endpoints de listagem do sistema). Uma academia só vê as mensagens que ela própria enviou; um administrador vê todas, com filtro opcional `?codigo_academia=`.

**Protecção:** `/comunicacao/mensagens` (enviar e listar) exige autenticação de administrador (qualquer role) ou academia. `/comunicacao/remetentes` exige autenticação de administrador — cadastrar exige especificamente role FPP. `/admin/comunicacao/provedor-padrao` exige role FPP tanto para consultar quanto para definir.
```


## Testes obrigatórios

1. **Primeiro passo, sem exceção:** `go build ./...` e depois `go vet ./...` na raiz do repositório. Corrija qualquer erro de compilação antes de continuar — o código foi validado por sintaxe (`gofmt`) mas não por compilação real (ver nota no topo do documento sobre a limitação do ambiente do Claude).
2. Rode a suíte de testes existente (`go test ./...`) e confirme que nada que já passava deixou de passar.
3. Rode as migrations novas contra um Postgres real (`123`, `124`, `125`) — já validei que aplicam sem erro sobre as 122 anteriores; confirme que isso também acontece no seu ambiente, se tiver acesso a um Postgres real. Se não tiver (ver nota no topo), pode pular este item — já está validado.
4. Escreva (ou confirme que já existem, se decidir reutilizar o padrão de testes de outro agregado) testes unitários para:
   - `RemetenteComunicacao.Configurar`: aceita GOSMS com identificador alfanumérico até 11 caracteres; rejeita identificador GOSMS com mais de 11 caracteres ou com caracteres não alfanuméricos; aceita ZIETT com identificador UUID válido; rejeita ZIETT com identificador que não é UUID; rejeita provedor diferente de GOSMS/ZIETT; rejeita identificador ou token vazios.
   - `MensagemComunicacao.Registrar`: grava corretamente todos os campos, incluindo quando `provedor_tentado_2`/`provedor_utilizado` ficam vazios (sucesso já na primeira tentativa).
   - `internal/services`: `EncryptComunicacaoSegredo`/`DecryptComunicacaoSegredo` fazem round-trip corretamente; `ValidateComunicacaoEncryptionConfig` rejeita chave ausente ou curta.
5. Teste manual (ou de integração) do fluxo completo de `POST /comunicacao/mensagens`, com os dois provedores configurados via `POST /comunicacao/remetentes`, simulando: (a) sucesso no provedor padrão; (b) falha no provedor padrão com sucesso no fallback; (c) falha em ambos.

## Fora de escopo (não implementar nesta tarefa)

- Envio de mensagem para múltiplos destinatários numa única chamada (lote/campanha). A rota de envio é sempre um destinatário por chamada.
- Qualquer endpoint de editar, desativar ou apagar um remetente já configurado — "cadastrar" (`POST /comunicacao/remetentes`) já substitui o remetente existente do mesmo provedor; não é preciso outro verbo.
- Consulta de status de entrega (delivery report) junto ao provedor, callbacks/webhooks de qualquer provedor, ou reenvio automático de uma mensagem já registada como `falhou`.
- Qualquer alteração na rota isolada de teste `POST /integracoes/ziett/mensagens/teste` ou nos arquivos `ziett_sms_test_client.go`/`ziett_sms_test_handler.go`.
- Provedor padrão por academia (é sempre uma configuração única e global).
- Qualquer UI ou rota de frontend — isto é assunto do documento de tarefa do `spuripainel` (frontend), a ser executado depois deste.

## Critérios de aceite

- [ ] As 3 migrations novas existem exatamente como especificado e aplicam sem erro sobre o estado atual do banco.
- [ ] `go build ./...`, `go vet ./...` e `go test ./...` passam sem erros novos.
- [ ] `POST /comunicacao/remetentes` (admin FPP) cadastra um remetente novo (`201`) e substitui um remetente existente do mesmo provedor (`200`), nos dois casos sem nunca devolver o token em texto plano na resposta.
- [ ] `POST /comunicacao/remetentes` chamado por um admin que não é FPP responde `403`.
- [ ] `GET /comunicacao/remetentes` lista os remetentes configurados (até dois), sem o token.
- [ ] `PUT /admin/comunicacao/provedor-padrao` (admin FPP) define o provedor padrão; `GET` devolve o valor atual (ou `null` antes de ser definido).
- [ ] `POST /comunicacao/mensagens` funciona tanto para um admin quanto para uma academia autenticados; tenta o provedor padrão primeiro e cai automaticamente para o outro em caso de falha ou de remetente ausente; responde `400` se nenhum provedor padrão foi definido ainda.
- [ ] `GET /comunicacao/mensagens`: uma academia só vê as suas próprias mensagens; um admin vê todas, com filtro `?codigo_academia=` funcionando.
- [ ] `COMUNICACAO_ENCRYPTION_KEY` ausente ou inválida impede o servidor de arrancar, com mensagem de erro clara.
- [ ] Nenhum arquivo relacionado com a rota isolada de teste Ziett foi alterado.
- [ ] `Documentação da API.md` reflete os novos endpoints.

## Procedimento de conclusão

1. Confirme que todos os critérios de aceite acima estão satisfeitos.
2. Mova este documento de `docs/Lista de Tarefas/` para `docs/Tarefas feitas/`, renomeando o arquivo para incluir "(feito)" no início do título dentro do arquivo, e adicione ao final um parágrafo curto "Resultado" descrevendo o que foi efetivamente feito e qualquer desvio pontual (ex.: pequenos ajustes de assinatura de função) em relação ao que este documento pedia.
3. Não abra pull request nem faça merge — deixe o commit pronto para o Fredy revisar.


## Resultado

Implementada a base do módulo de Comunicação com os provedores GoSMS e Ziett: migrations, agregados e projeções event-sourced, cifra própria de tokens, clientes HTTP, handlers, rotas, validação de arranque e documentação da API. Foram também adicionados testes unitários para os agregados e a criptografia; não houve desvios funcionais em relação ao especificado.
