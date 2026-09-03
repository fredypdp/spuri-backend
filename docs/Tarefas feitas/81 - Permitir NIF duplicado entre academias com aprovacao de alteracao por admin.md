---
criado: 03-09-2026 00:00
origem: Fredy + Claude (orquestração)
status: pronto para execução
tipo: backend (spuri-backend)
---

# Tarefa 81 — NIF de academia deixa de ser único; alteração exige aprovação de Admin

### Documento de execução para o Codex (orquestrado e pré-testado pelo Claude)

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Você não tem `apt`, Docker nem `psql` neste ambiente. **Não precisa disso aqui.** Eu (Claude) já validei esta tarefa inteira com PostgreSQL 16 e Go 1.24 reais no meu sandbox:

- Apliquei as 116 migrations existentes do zero + a migration 117 nova, em um banco limpo. Sem erros.
- Testei a migration por si só: duas academias com o mesmo NIF passam a coexistir sem erro; uma segunda solicitação pendente da mesma academia é bloqueada pelo índice único parcial; uma solicitação pendente de outra academia não é bloqueada; formato inválido de NIF continua rejeitado pelo `CHECK`.
- Testei concorrência real: duas transações Postgres simultâneas tentando criar solicitação pendente para a mesma academia — uma commitou, a outra recebeu `duplicate key value violates unique constraint` e fez rollback, exatamente como esperado.
- Rodei a migration duas vezes seguidas (idempotência): sem erro na segunda vez.
- Com todo o código novo já escrito (ver seção 3), rodei `go build ./...`, `go vet ./...` e `gofmt -l .` no repositório inteiro: limpos.
- Rodei a suíte de testes inteira (`go test ./...`), incluindo os testes de integração com `RUN_POSTGRES_INTEGRATION=1` e `SPURI_RUN_DB_INTEGRITY_TESTS=1` contra um banco Postgres 16 real, criado do zero com as 117 migrations: **100% verde**, `-p 1` (sequencial, sem paralelismo).

**Nota sobre paralelismo nos testes de integração**: se você rodar `go test ./...` (com as env vars de integração) e ver falhas tipo `cannot change name of view column "notas" to "deleted_at"` na migration `001_complete_schema.sql`, ou `FINANCE_ENCRYPTION_KEY é obrigatória`, **isso não tem relação com esta tarefa** — reproduzi exatamente essas duas falhas rodando a suíte completa em paralelo contra o repositório original, sem nenhuma das minhas alterações. São, respectivamente, uma condição de corrida pré-existente entre pacotes de teste de integração rodando em paralelo (rode com `-p 1` para evitar) e uma variável de ambiente que os testes do módulo financeiro exigem e que nada tem a ver com academias/NIF. Se precisar rodar os testes de integração, exporte `FINANCE_ENCRYPTION_KEY` com qualquer string e use `-p 1`.

O que **você** precisa fazer: aplicar os blocos desta seção 3 exatamente como estão, rodar `go build ./...`, `go vet ./...` e `go test ./...` (sem as env vars de integração já cobre tudo que muda aqui — os testes novos em `solicitacao_alteracao_nif_academia_test.go` são testes de unidade puros, sem banco), e seguir o checklist da seção 5. Não precisa planejar nada — o desenho já está fechado.

## 1. Prompt recomendado para executar esta correção

> Aplique exatamente os blocos "localizar/substituir" e "criar arquivo novo" da seção 3 deste documento, na ordem listada. Não refatore nada além do que está descrito. Depois, rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...` na raiz do repositório e corrija qualquer erro de compilação ou teste antes de considerar a tarefa concluída. Ao final, siga o "Procedimento de conclusão" (seção 6).

## 2. Contexto

Hoje, `projection_academias.nif` é único entre academias não deletadas (migration 111). O Fredy pediu para isso mudar: **o mesmo NIF pode passar a estar associado a mais de uma academia** na plataforma (ex.: grupos educacionais com a mesma entidade fiscal operando várias unidades).

Mas a alteração do NIF de uma academia já cadastrada não pode ser livre — precisa de aprovação:

1. A academia define o novo NIF e envia como **solicitação**. Nada muda em `projection_academias` neste momento.
2. Um Admin com role `adm` ou `fpp` aprova ou reprova.
   - **Aprovado** → o NIF da academia é alterado de fato.
   - **Reprovado** → nada muda.

O código já estava preparado para isso: `PUT /academia/dados` já bloqueia explicitamente o campo `nif` com a mensagem *"A alteração de NIF exige fluxo dedicado com validações próprias"*, e `Academia.AtualizarDados()` já aceita (mas nunca usa) um parâmetro `nif *string`. Este documento fecha esse fluxo dedicado.

## 3. O que já existe (mapeado antes de escrever o código)

- `projection_academias.nif`: hoje tem um índice único parcial `idx_projection_academias_nif_active_unique` (migration 111), escopado a academias não deletadas. A migration 114 já trata liberação de dados únicos após deleção — não mexe com isto aqui.
- `PUT /academia/dados` (`internal/handlers/academia_handlers.go` → `AtualizarDadosAcademia`) já não aceita `nif` no payload — a rejeição vem de `rejectAcademiaDadosRestrictedFields` em `internal/handlers/contact_handlers.go`, que hoje diz apenas que existe "fluxo dedicado" sem apontar para onde.
- `Academia.AtualizarDados()` (`internal/domain/aggregates/academia.go`) tem um parâmetro `nif *string` que **nenhum handler usa hoje** — é o parâmetro genérico reservado para esta tarefa. Eu não vou usá-lo; vou criar um comando dedicado (ver por quê na seção 3.5).
- `getAcademiaProjection(c).GetByNIF(nif)` (`internal/projections/academia_projection.go`) é usada em exatamente 2 lugares — `RegisterAcademia` e `RegisterAcademiaPublica`, ambos em `academia_handlers.go` — só para checar duplicidade no cadastro. Nenhum teste usa `GetByNIF`. Vou remover as duas checagens (não o método `GetByNIF` em si, que fica disponível para uso administrativo futuro).
- Padrão de referência (mimetizado ponto a ponto): `SolicitacaoEdicaoDadoEstudante` (`internal/domain/aggregates/solicitacao_edicao_dado_estudante.go` + `internal/projections/solicitacao_edicao_dado_estudante_projection.go` + `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go`, migration 095). É um aggregate de event sourcing próprio, com `Criar`/`Aprovar`/`Reprovar`, tabela de projeção própria, e um índice único parcial garantindo só 1 solicitação pendente por vez. Difere do meu caso em dois pontos deliberados: (a) não exijo documento comprobatório (a tarefa não pede isso), e (b) quem decide é um **Admin**, não a própria academia — a estudante pede, a academia decide; aqui a academia pede, o admin decide.
- Padrão de comando dedicado no aggregate-alvo: `Estudante.AlterarNomePorSolicitacao` (e irmãos `AlterarBilheteIdentidade...PorSolicitacao`) em `internal/domain/aggregates/estudante.go`. O dispatcher `Apply()` do Estudante já faz `case "DadosPessoaisAtualizados", "NomeEstudanteAlteradoPorSolicitacao", ...: return e.applyDadosPessoaisAtualizados(event)` — ou seja, reaproveita o MESMO handler de merge genérico para vários tipos de evento, desde que os campos batam por nome. Copiei esse padrão para `Academia.AlterarNIFPorSolicitacao`, reaproveitando `applyAcademiaDadosAtualizados` / `handleAcademiaDadosAtualizados` em vez de escrever um handler paralelo — o evento novo (`AcademiaNIFAlteradoPorSolicitacaoEvent`) tem só o campo `NIF *string` com o mesmo nome do campo em `AcademiaDadosAtualizadosEvent`, então o `json.Unmarshal` genérico já popula certo.
- `middleware.RequireAdm()` já cobre "role adm ou fpp" — a hierarquia interna é `fpp=3, adm=2, gerente=1`, e `RequireAdm()` exige role mínimo `adm`, então `fpp` também passa. Não precisa de um middleware novo.
- `db.NewUniqueOperationGuard` / `db.CanonicalGuardKey` / `db.ErrUniqueOperationInProgress` (`internal/db/unique_operation_guard.go`, migration 096): mecanismo genérico de guarda contra corrida na criação de registros com regra de unicidade lógica (ex.: "só uma solicitação pendente"). Usado por `solicitacao_edicao_dado_estudante_handlers.go` — copiei o mesmo padrão (`Reserve` → tenta criar → `Consume` em caso de sucesso, `Release` via `defer` em caso de erro).
- `generateUniqueCodigoSolicitacao(client)` (`internal/handlers/solicitacao_matricula_handlers.go`) gera um código aleatório de 11 caracteres; os fluxos de solicitação existentes o envolvem numa checagem extra de unicidade contra a própria tabela. Copiei o mesmo padrão em `generateUniqueCodigoSolicitacaoAlteracaoNIF`.
- Registro de aggregates novos: `DefaultAggregateFactory.Create` em `internal/domain/aggregates/aggregate.go`.
- Registro de projeções novas: `initProjections()` em `cmd/server/main.go`.
- Grupos de rotas em `cmd/server/main.go`: `academia := router.Group("/academia"); academia.Use(middleware.ValidarStatusAcademia())` (autenticado + academia ativa) e `admin := router.Group("/dominis"); admin.Use(middleware.RequireAdmin())` (qualquer admin autenticado; rotas individuais escalam para `middleware.RequireAdm()` quando precisam de `adm`/`fpp`, como já fazem `PUT /dominis/academia/:codigo/ativar` e `/desativar`).
- `registrarAcaoAdmin(c, adminID, acao, detalhes)` (`internal/handlers/helpers.go` ou arquivo de auditoria de admin) já é usado por `AtivarAcademia`/`DesativarAcademia` para registrar a ação no log de auditoria de admin — copiei a mesma chamada nas decisões de aprovar/reprovar.

## 4. Arquivos a alterar/criar, em ordem

### 4.1 — Criar `migrations/117_academia_nif_nao_unico_solicitacao.sql`

Arquivo novo. Conteúdo exato (já validado em Postgres 16 real — ver seção 0):

```sql
-- MIGRATION 117 - NIF de academia deixa de ser único; cria fluxo de
-- solicitação de alteração de NIF (academia solicita -> admin adm/fpp
-- aprova ou reprova).
--
-- Antes desta migration, projection_academias.nif tinha unicidade
-- operacional (migration 111: único apenas entre academias não deletadas).
-- Regra de negócio nova (Tarefa 81): o mesmo NIF pode estar associado a mais
-- de uma academia na plataforma — ex.: grupos educacionais com a mesma
-- entidade fiscal operando várias unidades/academias. A validação de
-- formato (exatamente 10 dígitos, check_academia_nif_10_digits) continua
-- valendo; apenas a unicidade é removida.

-- 1) Remove a unicidade operacional do NIF.
DROP INDEX IF EXISTS idx_projection_academias_nif_active_unique;

-- Mantém um índice não-único para acelerar buscas administrativas por NIF
-- (ex.: localizar todas as academias associadas a um mesmo NIF).
CREATE INDEX IF NOT EXISTS idx_projection_academias_nif
    ON projection_academias (nif);

COMMENT ON COLUMN projection_academias.nif IS
    'NIF da academia: string obrigatória com exatamente 10 dígitos. NÃO é único — a mesma entidade fiscal pode estar associada a mais de uma academia (Tarefa 81). Alteração de NIF exige aprovação via fluxo de solicitação (ver projection_solicitacoes_alteracao_nif_academia); não pode mais ser alterado diretamente por PUT /academia/dados.';

-- 2) Tabela de solicitações de alteração de NIF — event-sourced, mesmo
--    padrão estrutural de projection_solicitacoes_edicao_dados_estudante
--    (migration 095), mas sem documento comprobatório (não exigido pela
--    regra de negócio desta tarefa) e decidida por um Admin (role adm ou
--    fpp), não pela academia.
CREATE TABLE IF NOT EXISTS projection_solicitacoes_alteracao_nif_academia (
    id UUID PRIMARY KEY,
    codigo_solicitacao VARCHAR(32) UNIQUE NOT NULL,
    codigo_academia VARCHAR(20) NOT NULL,
    nif_atual VARCHAR(10) NOT NULL,
    nif_solicitado VARCHAR(10) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('pendente','aprovada','reprovada')),
    motivo_reprovacao TEXT,
    solicitado_por VARCHAR(20) NOT NULL,
    decidido_por TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version INTEGER NOT NULL DEFAULT 1,
    last_event_id BIGINT,
    CONSTRAINT check_solicitacao_nif_10_digits CHECK (nif_solicitado ~ '^[0-9]{10}$')
);

-- Apenas uma solicitação pendente por academia por vez (mesma filosofia da
-- migration 095: idx_solicitacoes_edicao_dados_estudante_pendente).
CREATE UNIQUE INDEX IF NOT EXISTS idx_solicitacoes_alteracao_nif_academia_pendente
    ON projection_solicitacoes_alteracao_nif_academia (codigo_academia)
    WHERE status = 'pendente';

CREATE INDEX IF NOT EXISTS idx_solicitacoes_alteracao_nif_academia_academia_status
    ON projection_solicitacoes_alteracao_nif_academia (codigo_academia, status);
CREATE INDEX IF NOT EXISTS idx_solicitacoes_alteracao_nif_academia_status
    ON projection_solicitacoes_alteracao_nif_academia (status);

COMMENT ON TABLE projection_solicitacoes_alteracao_nif_academia IS
    'Projeção das solicitações de alteração de NIF de academia. A alteração real de projection_academias.nif só acontece quando um Admin (role adm ou fpp) aprova a solicitação — ver evento AcademiaNIFAlteradoPorSolicitacao.';

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 117 - NIF de academia não é mais único; fluxo de solicitação de alteração criado'; END $$;
```

**Não troque o número 117 por outro** a menos que alguém tenha adicionado uma migration 117 diferente nesse meio tempo — nesse caso, renumeie para o próximo livre e ajuste as referências a "117" nos comentários dos arquivos abaixo.

---

### 4.2 — Criar `internal/domain/aggregates/solicitacao_alteracao_nif_academia.go`

Arquivo novo, conteúdo exato:

```go
package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"spuri/internal/utils"

	"github.com/google/uuid"
)

// SolicitacaoAlteracaoNIFAcademia é o aggregate de event sourcing que
// representa o fluxo de aprovação para alteração de NIF de uma academia.
//
// Espelha deliberadamente o padrão já usado por SolicitacaoEdicaoDadoEstudante
// (internal/domain/aggregates/solicitacao_edicao_dado_estudante.go): Criar
// grava o pedido no ledger sem tocar no dado real da Academia; só Aprovar
// dispara (no handler) a alteração efetiva via Academia.AlterarNIFPorSolicitacao.
// Reprovar apenas encerra a solicitação — nenhum dado da Academia muda.
//
// Diferença deliberada em relação a SolicitacaoEdicaoDadoEstudante: esta
// solicitação não exige documento comprobatório (fora de escopo da tarefa) e
// é decidida por um Admin (role "adm" ou "fpp"), não pela academia.
type SolicitacaoAlteracaoNIFAcademia struct {
	*BaseAggregate

	CodigoSolicitacao string
	CodigoAcademia    string
	NIFAtual          string
	NIFSolicitado     string
	Status            string
	MotivoReprovacao  string
	SolicitadoPor     string
	DecididoPor       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func NewSolicitacaoAlteracaoNIFAcademia() *SolicitacaoAlteracaoNIFAcademia {
	return &SolicitacaoAlteracaoNIFAcademia{
		BaseAggregate: &BaseAggregate{ID: uuid.New(), UncommittedEvents: []DomainEvent{}},
		Status:        StatusSolicitacaoPendente,
	}
}

func (s *SolicitacaoAlteracaoNIFAcademia) GetType() string { return "SolicitacaoAlteracaoNIFAcademia" }

// Criar registra o pedido de alteração de NIF. nifAtual é o NIF vigente da
// academia no momento do pedido (capturado pelo handler a partir da
// projeção); nifSolicitado é o novo valor pretendido. Nenhum dado da
// Academia é alterado aqui — apenas o pedido é gravado no ledger.
func (s *SolicitacaoAlteracaoNIFAcademia) Criar(codigo, codigoAcademia, nifAtual, nifSolicitado, solicitadoPor string) error {
	codigo = strings.TrimSpace(codigo)
	codigoAcademia = strings.TrimSpace(codigoAcademia)
	solicitadoPor = strings.TrimSpace(solicitadoPor)
	nifAtual = strings.TrimSpace(nifAtual)
	nifSolicitado = strings.TrimSpace(nifSolicitado)

	if codigo == "" || codigoAcademia == "" || solicitadoPor == "" {
		return fmt.Errorf("dados obrigatórios da solicitação inválidos")
	}
	if err := utils.ValidateNIF(nifSolicitado); err != nil {
		return err
	}
	if strings.EqualFold(nifAtual, nifSolicitado) {
		return fmt.Errorf("nif_solicitado deve ser diferente do nif atual")
	}

	now := time.Now()
	ev := &SolicitacaoAlteracaoNIFAcademiaCriadaEvent{
		BaseEvent:         BaseEvent{EventType: "SolicitacaoAlteracaoNIFAcademiaCriada", AggregateID: s.ID},
		CodigoSolicitacao: codigo,
		CodigoAcademia:    codigoAcademia,
		NIFAtual:          nifAtual,
		NIFSolicitado:     nifSolicitado,
		Status:            StatusSolicitacaoPendente,
		SolicitadoPor:     solicitadoPor,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}

// Aprovar apenas marca a solicitação como aprovada. A alteração real do NIF
// na Academia é responsabilidade do handler, que deve chamar
// Academia.AlterarNIFPorSolicitacao ANTES ou DEPOIS de persistir este
// evento, dentro da mesma requisição — ver
// DecidirSolicitacaoAlteracaoNIFAcademiaHandler.
func (s *SolicitacaoAlteracaoNIFAcademia) Aprovar(decididoPor string) error {
	if s.Status != StatusSolicitacaoPendente {
		return fmt.Errorf("solicitação já decidida")
	}
	decididoPor = strings.TrimSpace(decididoPor)
	if decididoPor == "" {
		return fmt.Errorf("decidido_por é obrigatório")
	}
	now := time.Now()
	ev := &SolicitacaoAlteracaoNIFAcademiaAprovadaEvent{
		BaseEvent:         BaseEvent{EventType: "SolicitacaoAlteracaoNIFAcademiaAprovada", AggregateID: s.ID},
		CodigoSolicitacao: s.CodigoSolicitacao,
		NIFSolicitado:     s.NIFSolicitado,
		DecididoPor:       decididoPor,
		DecididoAt:        now,
		UpdatedAt:         now,
	}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}

func (s *SolicitacaoAlteracaoNIFAcademia) Reprovar(decididoPor, motivo string) error {
	if s.Status != StatusSolicitacaoPendente {
		return fmt.Errorf("solicitação já decidida")
	}
	decididoPor = strings.TrimSpace(decididoPor)
	if decididoPor == "" {
		return fmt.Errorf("decidido_por é obrigatório")
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return fmt.Errorf("motivo_reprovacao é obrigatório")
	}
	now := time.Now()
	ev := &SolicitacaoAlteracaoNIFAcademiaReprovadaEvent{
		BaseEvent:         BaseEvent{EventType: "SolicitacaoAlteracaoNIFAcademiaReprovada", AggregateID: s.ID},
		CodigoSolicitacao: s.CodigoSolicitacao,
		MotivoReprovacao:  motivo,
		DecididoPor:       decididoPor,
		DecididoAt:        now,
		UpdatedAt:         now,
	}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}

func (s *SolicitacaoAlteracaoNIFAcademia) Apply(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	switch event.GetEventType() {
	case "SolicitacaoAlteracaoNIFAcademiaCriada":
		var ev SolicitacaoAlteracaoNIFAcademiaCriadaEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		s.CodigoSolicitacao = ev.CodigoSolicitacao
		s.CodigoAcademia = ev.CodigoAcademia
		s.NIFAtual = ev.NIFAtual
		s.NIFSolicitado = ev.NIFSolicitado
		s.Status = ev.Status
		s.SolicitadoPor = ev.SolicitadoPor
		s.CreatedAt = ev.CreatedAt
		s.UpdatedAt = ev.UpdatedAt
		return nil
	case "SolicitacaoAlteracaoNIFAcademiaAprovada":
		var ev SolicitacaoAlteracaoNIFAcademiaAprovadaEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		s.Status = StatusSolicitacaoAprovada
		s.DecididoPor = ev.DecididoPor
		s.UpdatedAt = ev.UpdatedAt
		return nil
	case "SolicitacaoAlteracaoNIFAcademiaReprovada":
		var ev SolicitacaoAlteracaoNIFAcademiaReprovadaEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		s.Status = StatusSolicitacaoReprovada
		s.MotivoReprovacao = ev.MotivoReprovacao
		s.DecididoPor = ev.DecididoPor
		s.UpdatedAt = ev.UpdatedAt
		return nil
	}
	return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
}

// ============================================================================
// Eventos
// ============================================================================

type SolicitacaoAlteracaoNIFAcademiaCriadaEvent struct {
	BaseEvent
	CodigoSolicitacao, CodigoAcademia, NIFAtual, NIFSolicitado, Status, SolicitadoPor string
	CreatedAt, UpdatedAt                                                              time.Time
}

func (e *SolicitacaoAlteracaoNIFAcademiaCriadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoAlteracaoNIFAcademiaCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoAlteracaoNIFAcademiaAprovadaEvent struct {
	BaseEvent
	CodigoSolicitacao, NIFSolicitado, DecididoPor string
	DecididoAt, UpdatedAt                         time.Time
}

func (e *SolicitacaoAlteracaoNIFAcademiaAprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoAlteracaoNIFAcademiaAprovadaEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

type SolicitacaoAlteracaoNIFAcademiaReprovadaEvent struct {
	BaseEvent
	CodigoSolicitacao, MotivoReprovacao, DecididoPor string
	DecididoAt, UpdatedAt                            time.Time
}

func (e *SolicitacaoAlteracaoNIFAcademiaReprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoAlteracaoNIFAcademiaReprovadaEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}
```

---

### 4.3 — Criar `internal/domain/aggregates/solicitacao_alteracao_nif_academia_test.go`

Arquivo novo, conteúdo exato (testes de unidade puros, sem banco — já rodei com `go test`, todos passam):

```go
package aggregates

import "testing"

func TestSolicitacaoAlteracaoNIFAcademiaTerminal(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("SOLNIF0001", "ACA001", "1234567890", "9876543210", "ACA001"); err != nil {
		t.Fatalf("criar solicitação: %v", err)
	}
	if s.Status != StatusSolicitacaoPendente {
		t.Fatalf("status inicial = %s", s.Status)
	}
	if err := s.Aprovar("ADMIN001"); err != nil {
		t.Fatalf("aprovar solicitação: %v", err)
	}
	if s.Status != StatusSolicitacaoAprovada {
		t.Fatalf("status aprovado = %s", s.Status)
	}
	if err := s.Reprovar("ADMIN001", "motivo qualquer"); err == nil {
		t.Fatalf("solicitação decidida não deve aceitar nova decisão")
	}
}

func TestSolicitacaoAlteracaoNIFAcademiaReprovarExigeMotivo(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("SOLNIF0002", "ACA001", "1234567890", "9876543210", "ACA001"); err != nil {
		t.Fatalf("criar solicitação: %v", err)
	}
	if err := s.Reprovar("ADMIN001", ""); err == nil {
		t.Fatalf("reprovar sem motivo deveria falhar")
	}
	if s.Status != StatusSolicitacaoPendente {
		t.Fatalf("status não deveria mudar após reprovação inválida: %s", s.Status)
	}
}

func TestSolicitacaoAlteracaoNIFAcademiaCriarRejeitaNIFIgual(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("SOLNIF0003", "ACA001", "1234567890", "1234567890", "ACA001"); err == nil {
		t.Fatalf("nif_solicitado igual ao nif_atual deveria ser rejeitado")
	}
}

func TestSolicitacaoAlteracaoNIFAcademiaCriarRejeitaNIFInvalido(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("SOLNIF0004", "ACA001", "1234567890", "abc", "ACA001"); err == nil {
		t.Fatalf("nif_solicitado com formato inválido deveria ser rejeitado")
	}
}

func TestSolicitacaoAlteracaoNIFAcademiaCriarRejeitaCamposObrigatorios(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("", "ACA001", "1234567890", "9876543210", "ACA001"); err == nil {
		t.Fatalf("codigo vazio deveria ser rejeitado")
	}
	if err := s.Criar("SOLNIF0005", "", "1234567890", "9876543210", "ACA001"); err == nil {
		t.Fatalf("codigo_academia vazio deveria ser rejeitado")
	}
	if err := s.Criar("SOLNIF0006", "ACA001", "1234567890", "9876543210", ""); err == nil {
		t.Fatalf("solicitado_por vazio deveria ser rejeitado")
	}
}

func TestAcademiaAlterarNIFPorSolicitacao(t *testing.T) {
	a := NewAcademia()
	a.NIF = "1234567890"
	if err := a.AlterarNIFPorSolicitacao("9876543210", "SOLNIF0001", "ADMIN001"); err != nil {
		t.Fatalf("alterar nif por solicitação: %v", err)
	}
	if a.NIF != "9876543210" {
		t.Fatalf("nif não alterado: %s", a.NIF)
	}
}

func TestAcademiaAlterarNIFPorSolicitacaoRejeitaFormatoInvalido(t *testing.T) {
	a := NewAcademia()
	a.NIF = "1234567890"
	if err := a.AlterarNIFPorSolicitacao("abc", "SOLNIF0001", "ADMIN001"); err == nil {
		t.Fatalf("nif inválido deveria ser rejeitado")
	}
	if a.NIF != "1234567890" {
		t.Fatalf("nif não deveria mudar após rejeição: %s", a.NIF)
	}
}
```

Se `NewAcademia()` não existir com essa assinatura exata no seu checkout (função construtora sem argumentos que retorna `*Academia` zerado), ajuste as duas últimas funções de teste para construir o `*Academia` do jeito que os outros testes do arquivo `academia_criar_test.go` já fazem — não invente uma forma nova.

---

### 4.4 — `internal/domain/aggregates/academia.go`

**4.4.1 — Localizar este bloco exato** (dentro do dispatcher `Apply`):

```go
	case "AcademiaDadosAtualizados":
		return a.applyAcademiaDadosAtualizados(event)
```

**Substituir por:**

```go
	case "AcademiaDadosAtualizados", "AcademiaNIFAlteradoPorSolicitacao":
		// AcademiaNIFAlteradoPorSolicitacao reaproveita applyAcademiaDadosAtualizados:
		// o evento carrega apenas o campo NIF (mesmo nome/tipo de campo), então o
		// unmarshal genérico já popula ev.NIF corretamente. Mesmo padrão usado por
		// Estudante para os eventos "*PorSolicitacao" (ver estudante.go Apply()).
		return a.applyAcademiaDadosAtualizados(event)
```

**4.4.2 — Localizar este bloco exato** (logo após o fim da função `AtualizarDados`, antes do comentário `// AlterarSenha emite o evento...`):

```go
	a.RaiseEvent(event)
	return a.Apply(event)
}

// AlterarSenha emite o evento AcademiaSenhaAlterada via event sourcing.
```

**Atenção**: esse trecho `a.RaiseEvent(event)\n\treturn a.Apply(event)\n}` também aparece no final de outras funções do arquivo — use como âncora única a combinação com a linha seguinte `// AlterarSenha emite o evento AcademiaSenhaAlterada via event sourcing.`, que só ocorre uma vez.

**Substituir por:**

```go
	a.RaiseEvent(event)
	return a.Apply(event)
}

// AlterarNIFPorSolicitacao altera o NIF da academia como resultado da
// aprovação de uma SolicitacaoAlteracaoNIFAcademia por um Admin (role "adm"
// ou "fpp"). NIF não é mais um dado único entre academias (Tarefa 81) — a
// única via de alteração é este fluxo de solicitação; PUT /academia/dados
// continua rejeitando o campo "nif" (ver rejectAcademiaDadosRestrictedFields).
//
// O evento gerado é reduzido ao campo alterado e reaproveita
// applyAcademiaDadosAtualizados / handleAcademiaDadosAtualizados através do
// mesmo dispatch multi-case usado por "AcademiaDadosAtualizados" — mesmo
// padrão de Estudante.AlterarNomePorSolicitacao.
func (a *Academia) AlterarNIFPorSolicitacao(novoNif, codigoSolicitacao, decididoPor string) error {
	nif := strings.TrimSpace(novoNif)
	if err := utils.ValidateNIF(nif); err != nil {
		return err
	}
	codigoSolicitacao = strings.TrimSpace(codigoSolicitacao)
	decididoPor = strings.TrimSpace(decididoPor)
	if codigoSolicitacao == "" || decididoPor == "" {
		return fmt.Errorf("codigo_solicitacao e decidido_por são obrigatórios")
	}

	event := &AcademiaNIFAlteradoPorSolicitacaoEvent{
		BaseEvent:         BaseEvent{EventType: "AcademiaNIFAlteradoPorSolicitacao", AggregateID: a.ID},
		NIF:               &nif,
		CodigoSolicitacao: codigoSolicitacao,
		DecididoPor:       decididoPor,
		UpdatedAt:         time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// AlterarSenha emite o evento AcademiaSenhaAlterada via event sourcing.
```

**4.4.3 — Localizar este bloco exato** (logo após a definição de `AcademiaDadosAtualizadosEvent`):

```go
func (e *AcademiaDadosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *AcademiaDadosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
```

**Substituir por:**

```go
func (e *AcademiaDadosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *AcademiaDadosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// AcademiaNIFAlteradoPorSolicitacaoEvent é intencionalmente reduzido ao campo
// NIF (mesmo nome de campo de AcademiaDadosAtualizadosEvent.NIF) para que
// Apply() e a projeção reaproveitem os handlers de "AcademiaDadosAtualizados"
// sem duplicar lógica de merge — CodigoSolicitacao e DecididoPor ficam
// gravados no ledger para auditoria, mas não são usados pelos handlers
// reaproveitados (mesmo comportamento das colunas não usadas em
// handleDadosPessoaisAtualizados para os eventos "*PorSolicitacao" do
// Estudante).
type AcademiaNIFAlteradoPorSolicitacaoEvent struct {
	BaseEvent
	NIF               *string
	CodigoSolicitacao string
	DecididoPor       string
	UpdatedAt         time.Time
}

func (e *AcademiaNIFAlteradoPorSolicitacaoEvent) GetPayload() interface{} { return e }
func (e *AcademiaNIFAlteradoPorSolicitacaoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
```

---

### 4.5 — `internal/domain/aggregates/aggregate.go`

**Localizar este bloco exato:**

```go
	case "SolicitacaoEdicaoDadoEstudante":
		return NewSolicitacaoEdicaoDadoEstudante(), nil
```

**Substituir por:**

```go
	case "SolicitacaoEdicaoDadoEstudante":
		return NewSolicitacaoEdicaoDadoEstudante(), nil
	case "SolicitacaoAlteracaoNIFAcademia":
		return NewSolicitacaoAlteracaoNIFAcademia(), nil
```

---

### 4.6 — Criar `internal/projections/solicitacao_alteracao_nif_academia_projection.go`

Arquivo novo, conteúdo exato:

```go
package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
)

type SolicitacaoAlteracaoNIFAcademiaProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewSolicitacaoAlteracaoNIFAcademiaProjection(client *db.Client) *SolicitacaoAlteracaoNIFAcademiaProjection {
	return &SolicitacaoAlteracaoNIFAcademiaProjection{client: client, ctx: context.Background()}
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) Name() string {
	return "solicitacoes_alteracao_nif_academia"
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) GetLastProcessedEventID() (int64, error) {
	var id int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) UpdateCheckpoint(id int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES ($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT (projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE projection_solicitacoes_alteracao_nif_academia`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at FROM spuri_ledger WHERE aggregate_type='SolicitacaoAlteracaoNIFAcademia' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ev db.Event
		if err := rows.Scan(&ev.ID, &ev.EventID, &ev.AggregateID, &ev.AggregateType, &ev.EventType, &ev.EventVersion, &ev.Payload, &ev.Metadata, &ev.OccurredAt, &ev.RecordedAt); err != nil {
			return err
		}
		if err := p.Handle(ev); err != nil {
			return err
		}
		_ = p.UpdateCheckpoint(ev.ID)
	}
	return rows.Err()
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) Handle(event db.Event) error {
	if event.AggregateType != "SolicitacaoAlteracaoNIFAcademia" {
		return nil
	}
	switch event.EventType {
	case "SolicitacaoAlteracaoNIFAcademiaCriada":
		return p.handleCriada(event)
	case "SolicitacaoAlteracaoNIFAcademiaAprovada":
		return p.handleAprovada(event)
	case "SolicitacaoAlteracaoNIFAcademiaReprovada":
		return p.handleReprovada(event)
	}
	return nil
}

type SolicitacaoAlteracaoNIFAcademiaDTO struct {
	ID                uuid.UUID `json:"id"`
	CodigoSolicitacao string    `json:"codigo_solicitacao"`
	CodigoAcademia    string    `json:"codigo_academia"`
	NIFAtual          string    `json:"nif_atual"`
	NIFSolicitado     string    `json:"nif_solicitado"`
	Status            string    `json:"status"`
	MotivoReprovacao  *string   `json:"motivo_reprovacao,omitempty"`
	SolicitadoPor     string    `json:"solicitado_por"`
	DecididoPor       *string   `json:"decidido_por,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Version           int       `json:"version"`
}

func (p *SolicitacaoAlteracaoNIFAcademiaProjection) handleCriada(event db.Event) error {
	var x aggregates.SolicitacaoAlteracaoNIFAcademiaCriadaEvent
	if err := json.Unmarshal(event.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`INSERT INTO projection_solicitacoes_alteracao_nif_academia (id,codigo_solicitacao,codigo_academia,nif_atual,nif_solicitado,status,solicitado_por,created_at,updated_at,version,last_event_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, updated_at=EXCLUDED.updated_at, version=EXCLUDED.version,last_event_id=EXCLUDED.last_event_id`, event.AggregateID, x.CodigoSolicitacao, x.CodigoAcademia, x.NIFAtual, x.NIFSolicitado, x.Status, x.SolicitadoPor, x.CreatedAt, x.UpdatedAt, event.EventVersion, event.ID)
	return err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) handleAprovada(event db.Event) error {
	var x aggregates.SolicitacaoAlteracaoNIFAcademiaAprovadaEvent
	_ = json.Unmarshal(event.Payload, &x)
	_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_alteracao_nif_academia SET status=$1, decidido_por=$2, updated_at=$3, version=$4,last_event_id=$5 WHERE id=$6`, aggregates.StatusSolicitacaoAprovada, x.DecididoPor, x.UpdatedAt, event.EventVersion, event.ID, event.AggregateID)
	return err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) handleReprovada(event db.Event) error {
	var x aggregates.SolicitacaoAlteracaoNIFAcademiaReprovadaEvent
	_ = json.Unmarshal(event.Payload, &x)
	_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_alteracao_nif_academia SET status=$1, motivo_reprovacao=$2, decidido_por=$3, updated_at=$4, version=$5,last_event_id=$6 WHERE id=$7`, aggregates.StatusSolicitacaoReprovada, x.MotivoReprovacao, x.DecididoPor, x.UpdatedAt, event.EventVersion, event.ID, event.AggregateID)
	return err
}

// ExistePendente reporta se a academia já tem uma solicitação de alteração
// de NIF pendente. O handler de criação consulta isto para devolver um erro
// amigável antes de tentar gravar — o índice único parcial
// idx_solicitacoes_alteracao_nif_academia_pendente (migration 117) é quem
// garante a regra de fato sob concorrência.
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) ExistePendente(codigoAcademia string) (bool, error) {
	var b bool
	err := p.client.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM projection_solicitacoes_alteracao_nif_academia WHERE codigo_academia=$1 AND status='pendente')`, codigoAcademia).Scan(&b)
	return b, err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) GetByCodigo(codigo string) (*SolicitacaoAlteracaoNIFAcademiaDTO, error) {
	rows, err := p.client.DB().Query(`SELECT id,codigo_solicitacao,codigo_academia,nif_atual,nif_solicitado,status,motivo_reprovacao,solicitado_por,decidido_por,created_at,updated_at,version FROM projection_solicitacoes_alteracao_nif_academia WHERE codigo_solicitacao=$1`, codigo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		return scanSolAlteracaoNIF(rows)
	}
	return nil, sql.ErrNoRows
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) List(status, codigoAcademia string, limit, offset int) ([]SolicitacaoAlteracaoNIFAcademiaDTO, error) {
	wh := []string{"1=1"}
	args := []interface{}{}
	add := func(cond, v string) {
		if strings.TrimSpace(v) != "" {
			args = append(args, v)
			wh = append(wh, fmt.Sprintf(cond, len(args)))
		}
	}
	add("status=$%d", status)
	add("codigo_academia=$%d", codigoAcademia)
	args = append(args, limit, offset)
	q := fmt.Sprintf(`SELECT id,codigo_solicitacao,codigo_academia,nif_atual,nif_solicitado,status,motivo_reprovacao,solicitado_por,decidido_por,created_at,updated_at,version FROM projection_solicitacoes_alteracao_nif_academia WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, strings.Join(wh, " AND "), len(args)-1, len(args))
	rows, err := p.client.DB().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SolicitacaoAlteracaoNIFAcademiaDTO
	for rows.Next() {
		d, err := scanSolAlteracaoNIF(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}
func scanSolAlteracaoNIF(r interface{ Scan(...interface{}) error }) (*SolicitacaoAlteracaoNIFAcademiaDTO, error) {
	var d SolicitacaoAlteracaoNIFAcademiaDTO
	var mot, dec sql.NullString
	err := r.Scan(&d.ID, &d.CodigoSolicitacao, &d.CodigoAcademia, &d.NIFAtual, &d.NIFSolicitado, &d.Status, &mot, &d.SolicitadoPor, &dec, &d.CreatedAt, &d.UpdatedAt, &d.Version)
	if mot.Valid {
		d.MotivoReprovacao = &mot.String
	}
	if dec.Valid {
		d.DecididoPor = &dec.String
	}
	return &d, err
}
```

---

### 4.7 — `internal/projections/academia_projection.go`

**4.7.1 — Localizar este bloco exato** (dentro do dispatch map de `Handle`, sem `tx`):

```go
			"AcademiaDadosAtualizados":                  p.handleAcademiaDadosAtualizados,
			"EmailVerificado":                           p.handleEmailVerificado,
```

**Substituir por:**

```go
			"AcademiaDadosAtualizados":                  p.handleAcademiaDadosAtualizados,
			"AcademiaNIFAlteradoPorSolicitacao":         p.handleAcademiaDadosAtualizados,
			"EmailVerificado":                           p.handleEmailVerificado,
```

**4.7.2 — Localizar este bloco exato** (dentro do dispatch map de `HandleTx`):

```go
			"AcademiaDadosAtualizados":                  p.handleAcademiaDadosAtualizadosTx,
			"EmailVerificado":                           p.handleEmailVerificadoTx,
```

**Substituir por:**

```go
			"AcademiaDadosAtualizados":                  p.handleAcademiaDadosAtualizadosTx,
			"AcademiaNIFAlteradoPorSolicitacao":         p.handleAcademiaDadosAtualizadosTx,
			"EmailVerificado":                           p.handleEmailVerificadoTx,
```

---

### 4.8 — Criar `internal/handlers/solicitacao_alteracao_nif_academia_handlers.go`

Arquivo novo, conteúdo exato:

```go
package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"
)

// ============================================================================
// Academia: criar e listar as próprias solicitações de alteração de NIF
// ============================================================================

// CriarSolicitacaoAlteracaoNIFAcademiaHandler cria um pedido de alteração de
// NIF para a academia autenticada. Nada muda em projection_academias aqui —
// apenas o pedido é gravado (ledger + projeção de solicitações) com status
// "pendente". A alteração real só acontece se um Admin (role "adm" ou "fpp")
// aprovar, via DecidirSolicitacaoAlteracaoNIFAcademiaHandler(true).
func CriarSolicitacaoAlteracaoNIFAcademiaHandler(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	academia, err := getAcademiaProjection(c).GetByID(academiaID)
	if err != nil || academia == nil {
		utils.RespondWithForbiddenError(c, "academia inválida")
		return
	}

	var req struct {
		NovoNIF string `json:"novo_nif"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	novoNif := strings.TrimSpace(req.NovoNIF)
	if novoNif == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_nif é obrigatório"))
		return
	}
	if err := utils.ValidateNIF(novoNif); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if strings.EqualFold(strings.TrimSpace(academia.NIF), novoNif) {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_nif deve ser diferente do nif atual"))
		return
	}

	guardKey := db.CanonicalGuardKey(academia.CodigoAcademia)
	guard, err := db.NewUniqueOperationGuard(getDbClient(c)).WithContext(c.Request.Context()).Reserve(
		"solicitacao_alteracao_nif_academia:pendente",
		guardKey,
		db.UniqueGuardOptions{UserID: academiaID.String(), UserType: "academia"},
	)
	if errors.Is(err, db.ErrUniqueOperationInProgress) {
		log.Printf("⚠️ [UniqueGuard] conflito scope=solicitacao_alteracao_nif_academia:pendente key_hash=%s user=%s", db.MaskGuardKey(guardKey), academiaID.String())
		utils.RespondWithConflictError(c, "já existe solicitação de alteração de NIF pendente ou em criação para esta academia")
		return
	}
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	guardConsumed := false
	defer func() {
		if !guardConsumed {
			_ = guard.Release()
		}
	}()

	pend, err := getSolicitacaoAlteracaoNIFAcademiaProjection(c).ExistePendente(academia.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if pend {
		utils.RespondWithConflictError(c, "já existe solicitação de alteração de NIF pendente para esta academia")
		return
	}

	codigo, err := generateUniqueCodigoSolicitacaoAlteracaoNIF(getDbClient(c))
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	agg := aggregates.NewSolicitacaoAlteracaoNIFAcademia()
	if err := agg.Criar(codigo, academia.CodigoAcademia, academia.NIF, novoNif, academia.CodigoAcademia); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	audit := db.AuditContext{UserID: academiaID.String(), UserType: "academia", IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if err := guard.Consume(agg.GetID()); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	guardConsumed = true

	c.JSON(http.StatusCreated, gin.H{
		"message":            "solicitação de alteração de NIF criada com sucesso",
		"codigo_solicitacao": codigo,
		"nif_atual":          academia.NIF,
		"nif_solicitado":     novoNif,
		"status":             aggregates.StatusSolicitacaoPendente,
	})
}

// ListarSolicitacoesNIFAcademia lista as solicitações de alteração de NIF da
// própria academia autenticada (qualquer status).
func ListarSolicitacoesNIFAcademia(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	academia, err := getAcademiaProjection(c).GetByID(academiaID)
	if err != nil || academia == nil {
		utils.RespondWithForbiddenError(c, "academia inválida")
		return
	}
	listarSolicitacoesAlteracaoNIF(c, c.Query("status"), academia.CodigoAcademia)
}

// ============================================================================
// Admin: listar e decidir solicitações de alteração de NIF
// ============================================================================

// ListarSolicitacoesNIFAdmin lista solicitações de alteração de NIF de
// qualquer academia. Visível a qualquer admin autenticado (role "gerente" ou
// superior); apenas a decisão (aprovar/reprovar) exige role "adm" ou "fpp"
// (ver middleware.RequireAdm() nas rotas correspondentes em main.go).
func ListarSolicitacoesNIFAdmin(c *gin.Context) {
	listarSolicitacoesAlteracaoNIF(c, c.Query("status"), c.Query("codigo_academia"))
}

func listarSolicitacoesAlteracaoNIF(c *gin.Context, status, codigoAcademia string) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	itens, err := getSolicitacaoAlteracaoNIFAcademiaProjection(c).List(status, codigoAcademia, limit, offset)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"solicitacoes": itens, "limit": limit, "offset": offset, "total": len(itens)})
}

// DecidirSolicitacaoAlteracaoNIFAcademiaHandler aprova ou reprova uma
// solicitação pendente de alteração de NIF. Protegido por
// middleware.RequireAdm() na rota (role "adm" ou "fpp" — hierarquia
// fpp=3 >= adm=2 já cobre "ADM ou FPP").
//
//   - Aprovado: Academia.AlterarNIFPorSolicitacao é chamado ANTES de marcar a
//     solicitação como aprovada — só altera o dado se a solicitação puder ser
//     salva; se a alteração da Academia falhar, a solicitação continua
//     pendente.
//   - Reprovado: nenhum dado da Academia é tocado; apenas a solicitação muda
//     de status.
func DecidirSolicitacaoAlteracaoNIFAcademiaHandler(aprovar bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminID, _ := middleware.GetUserID(c)
		admin, err := getAdminProjection(c).GetByID(adminID)
		if err != nil || admin == nil {
			utils.RespondWithForbiddenError(c, "administrador inválido")
			return
		}

		sol, err := getSolicitacaoAlteracaoNIFAcademiaProjection(c).GetByCodigo(strings.TrimSpace(c.Param("codigo")))
		if err == sql.ErrNoRows {
			utils.RespondWithNotFoundError(c, "solicitação")
			return
		}
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if sol.Status != aggregates.StatusSolicitacaoPendente {
			utils.RespondWithConflictError(c, "solicitação já decidida")
			return
		}

		loaded, err := getRepository(c).WithContext(c.Request.Context()).Load(sol.ID, "SolicitacaoAlteracaoNIFAcademia")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		agg := loaded.(*aggregates.SolicitacaoAlteracaoNIFAcademia)

		if aprovar {
			if err := aplicarAlteracaoNIFAprovada(c, sol, adminID.String()); err != nil {
				return
			}
			err = agg.Aprovar(adminID.String())
		} else {
			var req struct {
				MotivoReprovacao string `json:"motivo_reprovacao"`
			}
			_ = c.ShouldBindJSON(&req)
			err = agg.Reprovar(adminID.String(), req.MotivoReprovacao)
		}
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}

		audit := db.AuditContext{UserID: adminID.String(), UserType: "admin", IP: c.ClientIP()}
		if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		_ = db.NewUniqueOperationGuard(getDbClient(c)).WithContext(c.Request.Context()).ReleaseKey(
			"solicitacao_alteracao_nif_academia:pendente", db.CanonicalGuardKey(sol.CodigoAcademia))

		acao := "reprovar_solicitacao_alteracao_nif_academia"
		novoStatus := aggregates.StatusSolicitacaoReprovada
		if aprovar {
			acao = "aprovar_solicitacao_alteracao_nif_academia"
			novoStatus = aggregates.StatusSolicitacaoAprovada
		}
		registrarAcaoAdmin(c, adminID, acao, map[string]interface{}{
			"codigo_solicitacao": sol.CodigoSolicitacao,
			"codigo_academia":    sol.CodigoAcademia,
			"nif_atual":          sol.NIFAtual,
			"nif_solicitado":     sol.NIFSolicitado,
		})

		c.JSON(http.StatusOK, gin.H{
			"message":            "solicitação decidida com sucesso",
			"codigo_solicitacao": sol.CodigoSolicitacao,
			"status":             novoStatus,
		})
	}
}

// aplicarAlteracaoNIFAprovada altera o NIF da Academia dona da solicitação.
// Chamado apenas no caminho de aprovação, antes de marcar a solicitação como
// aprovada — se isto falhar, a solicitação permanece pendente e nenhum dado
// muda (o handler retorna sem persistir a decisão).
func aplicarAlteracaoNIFAprovada(c *gin.Context, sol *projections.SolicitacaoAlteracaoNIFAcademiaDTO, decididoPor string) error {
	academia, err := getAcademiaProjection(c).GetByCodigo(sol.CodigoAcademia)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return fmt.Errorf("academia não encontrada")
	}
	loaded, err := getRepository(c).WithContext(c.Request.Context()).Load(academia.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return err
	}
	agg := loaded.(*aggregates.Academia)
	if err := agg.AlterarNIFPorSolicitacao(sol.NIFSolicitado, sol.CodigoSolicitacao, decididoPor); err != nil {
		utils.RespondWithValidationError(c, err)
		return err
	}
	audit := db.AuditContext{UserID: decididoPor, UserType: "admin", IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return err
	}
	return nil
}

func generateUniqueCodigoSolicitacaoAlteracaoNIF(client *db.Client) (string, error) {
	for i := 0; i < 20; i++ {
		code, err := generateUniqueCodigoSolicitacao(client)
		if err != nil {
			return "", err
		}
		var exists bool
		if err := client.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM projection_solicitacoes_alteracao_nif_academia WHERE codigo_solicitacao=$1)`, code).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("não foi possível gerar codigo_solicitacao único")
}
```

---

### 4.9 — `internal/handlers/helpers.go`

**Localizar este bloco exato:**

```go
func getSolicitacaoMatriculaProjection(c *gin.Context) *projections.SolicitacaoMatriculaProjection {
	return projections.NewSolicitacaoMatriculaProjection(getDbClient(c))
}
```

**Substituir por:**

```go
func getSolicitacaoMatriculaProjection(c *gin.Context) *projections.SolicitacaoMatriculaProjection {
	return projections.NewSolicitacaoMatriculaProjection(getDbClient(c))
}

func getSolicitacaoAlteracaoNIFAcademiaProjection(c *gin.Context) *projections.SolicitacaoAlteracaoNIFAcademiaProjection {
	return projections.NewSolicitacaoAlteracaoNIFAcademiaProjection(getDbClient(c))
}
```

---

### 4.10 — `internal/handlers/contact_handlers.go`

**Localizar este bloco exato:**

```go
		"nif":             "O campo 'nif' não é aceito em PUT /academia/dados. A alteração de NIF exige fluxo dedicado com validações próprias.",
```

**Substituir por:**

```go
		"nif":             "O campo 'nif' não é aceito em PUT /academia/dados. Use POST /academia/solicitacoes-nif para solicitar a alteração — a mudança só é aplicada após aprovação de um Admin (role adm ou fpp).",
```

---

### 4.11 — `internal/handlers/academia_handlers.go`

Este bloco aparece **duas vezes** no arquivo, com o mesmo texto exato — uma dentro de `RegisterAcademia` (cadastro pelo admin), outra dentro de `RegisterAcademiaPublica` (autocadastro público). Aplique a mesma substituição **nas duas ocorrências**.

**Localizar este bloco exato** (ocorre 2x):

```go
	client := getDbClient(c)
	existing, err := getAcademiaProjection(c).GetByNIF(req.NIF)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if existing != nil {
		utils.RespondWithConflictError(c, "nif já cadastrado em outra academia")
		return
	}
	codigoAcademia, err := generateCodigoAcademia(codigoProvincia, client.DB())
```

**Substituir por** (nas duas ocorrências):

```go
	// Tarefa 81: nif deixou de ser único entre academias — mesma entidade
	// fiscal pode estar associada a mais de uma academia na plataforma.
	// Alteração de nif após o cadastro exige aprovação via
	// POST /academia/solicitacoes-nif (ver solicitacao_alteracao_nif_academia_handlers.go).
	client := getDbClient(c)
	codigoAcademia, err := generateCodigoAcademia(codigoProvincia, client.DB())
```

Não altere nada mais nessas duas funções — só este bloco. `getAcademiaProjection(c).GetByNIF(...)` (o método em si, em `academia_projection.go`) **não** deve ser removido; ele continua disponível para uso administrativo futuro, só deixa de ser chamado aqui.

---

### 4.12 — `cmd/server/main.go`

**4.12.1 — Localizar este bloco exato** (dentro de `initProjections()`):

```go
	projManager.RegisterProjection("solicitacoes_edicao_dados_estudante", projections.NewSolicitacaoEdicaoDadoEstudanteProjection(dbClient))
```

**Substituir por:**

```go
	projManager.RegisterProjection("solicitacoes_edicao_dados_estudante", projections.NewSolicitacaoEdicaoDadoEstudanteProjection(dbClient))
	projManager.RegisterProjection("solicitacoes_alteracao_nif_academia", projections.NewSolicitacaoAlteracaoNIFAcademiaProjection(dbClient))
```

**4.12.2 — Localizar este bloco exato** (dentro do grupo `academia`, logo após a rota de `PUT /dados`):

```go
		academia.PUT("/dados", handlers.AtualizarDadosAcademia)
		academia.POST("/definir-ano-letivo", handlers.DefinirAnoLetivoAcademia)
```

**Substituir por:**

```go
		academia.PUT("/dados", handlers.AtualizarDadosAcademia)
		academia.POST("/solicitacoes-nif", handlers.CriarSolicitacaoAlteracaoNIFAcademiaHandler)
		academia.GET("/solicitacoes-nif", handlers.ListarSolicitacoesNIFAcademia)
		academia.POST("/definir-ano-letivo", handlers.DefinirAnoLetivoAcademia)
```

**4.12.3 — Localizar este bloco exato** (dentro do grupo `admin` = `/dominis`, logo após a rota de deletar academia):

```go
		admin.DELETE("/academia/:codigo", middleware.RequireFPP(), handlers.DeletarAcademia)
```

**Substituir por:**

```go
		admin.DELETE("/academia/:codigo", middleware.RequireFPP(), handlers.DeletarAcademia)
		admin.GET("/solicitacoes-nif-academia", handlers.ListarSolicitacoesNIFAdmin)
		admin.PUT("/solicitacoes-nif-academia/:codigo/aprovar", middleware.RequireAdm(), handlers.DecidirSolicitacaoAlteracaoNIFAcademiaHandler(true))
		admin.PUT("/solicitacoes-nif-academia/:codigo/reprovar", middleware.RequireAdm(), handlers.DecidirSolicitacaoAlteracaoNIFAcademiaHandler(false))
```

---

### 4.13 — `Documentação da API.md`

**4.13.1 — Localizar este bloco exato** (dentro de `### POST /dominis/academia/cadastro`, primeiro parágrafo):

```
Registra uma nova academia via `multipart/form-data`. Criada com status `inativo`. `nif` é obrigatório, string única de exatamente 10 dígitos, inclusive para academias inativas. `alvara` é opcional: quando enviado, deve ser PDF válido com até 10MB e é armazenado em `{codigo_academia}/Documentação formal/`. Quando não for enviado no cadastro, envie-o depois por `POST /documentos/academias/{codigo_academia}/alvara/upload`. O front end pode ler esse documento pela rota autenticada `GET /documentos/academias/{codigo_academia}/alvara/download`.
```

**Substituir por:**

```
Registra uma nova academia via `multipart/form-data`. Criada com status `inativo`. `nif` é obrigatório, string de exatamente 10 dígitos — **não é único**: a mesma entidade fiscal pode estar associada a mais de uma academia na plataforma (ver "Solicitações de alteração de NIF de academia" para como o NIF é alterado depois do cadastro). `alvara` é opcional: quando enviado, deve ser PDF válido com até 10MB e é armazenado em `{codigo_academia}/Documentação formal/`. Quando não for enviado no cadastro, envie-o depois por `POST /documentos/academias/{codigo_academia}/alvara/upload`. O front end pode ler esse documento pela rota autenticada `GET /documentos/academias/{codigo_academia}/alvara/download`.
```

**4.13.2 — Localizar este bloco exato** (final da lista de erros de `POST /dominis/academia/cadastro`, até o início de `### POST /academia/cadastro`):

```
- `400` — `nivel` inválido, `type` inválido (`public`/`private`) ou ausente, `nif` ausente/inválido, `alvara` não PDF/acima de 10MB quando enviado, campos obrigatórios ausentes ou anos_academicos inválidos
- `409` — academia ou `nif` já existe

---

### POST /academia/cadastro

Permite que uma academia se autocadastre na plataforma **sem autenticação prévia**, via `multipart/form-data`. Usa exatamente as mesmas regras de validação de `POST /dominis/academia/cadastro`: `nif` é obrigatório, único e tem 10 dígitos; `alvara` é opcional e, quando enviado, deve ser PDF válido de até 10MB, armazenado em `{codigo_academia}/Documentação formal/`. Caso não seja enviado no cadastro, use posteriormente `POST /documentos/academias/{codigo_academia}/alvara/upload`. A academia é sempre criada com status `inativo` — apenas um admin com role `adm` ou `fpp` pode ativá-la, via `PUT /dominis/academia/:codigo/ativar`. Login antes da ativação retorna erro de "academia inativa".
```

**Substituir por:**

```
- `400` — `nivel` inválido, `type` inválido (`public`/`private`) ou ausente, `nif` ausente/inválido, `alvara` não PDF/acima de 10MB quando enviado, campos obrigatórios ausentes ou anos_academicos inválidos

---

### POST /academia/cadastro

Permite que uma academia se autocadastre na plataforma **sem autenticação prévia**, via `multipart/form-data`. Usa exatamente as mesmas regras de validação de `POST /dominis/academia/cadastro`: `nif` é obrigatório e tem 10 dígitos (não é único — ver nota acima); `alvara` é opcional e, quando enviado, deve ser PDF válido de até 10MB, armazenado em `{codigo_academia}/Documentação formal/`. Caso não seja enviado no cadastro, use posteriormente `POST /documentos/academias/{codigo_academia}/alvara/upload`. A academia é sempre criada com status `inativo` — apenas um admin com role `adm` ou `fpp` pode ativá-la, via `PUT /dominis/academia/:codigo/ativar`. Login antes da ativação retorna erro de "academia inativa".
```

**4.13.3 — Localizar este bloco exato** (final da lista de erros de `POST /academia/cadastro`):

```
- `400` — `nivel` inválido, `type` inválido, `nif` ausente/inválido, `alvara` não PDF/acima de 10MB quando enviado, campos obrigatórios ausentes, `anos_academicos` inválidos, `senha` ausente/vazia ou fora do intervalo de 6–128 caracteres
- `409` — `nif` já cadastrado em outra academia

---
```

**Substituir por:**

```
- `400` — `nivel` inválido, `type` inválido, `nif` ausente/inválido, `alvara` não PDF/acima de 10MB quando enviado, campos obrigatórios ausentes, `anos_academicos` inválidos, `senha` ausente/vazia ou fora do intervalo de 6–128 caracteres

---
```

**4.13.4 — Localizar este bloco exato** (nota final de `PUT /academia/dados`):

```
**Nota**: `telefone`, `email`, `anos_academicos`, `cursos`, `type`, `nivel_escolar` e `nif` não são aceitos nesta rota. Use `PUT /me/email` e `PUT /me/telefone` para contatos, `POST/DELETE /academia/anos-academicos` para anos acadêmicos e as rotas `/academia/curso` para cursos. Alterações de `type` e `nivel_escolar` exigem documento comprobativo pelo fluxo dedicado da tarefa 07 e ficam indisponíveis por este caminho. Se qualquer campo não permitido aparecer no payload, a requisição falha inteira com `400` e nenhum campo é alterado.
```

**Substituir por** (nota atualizada + nova subseção completa — cole exatamente este bloco):

```
**Nota**: `telefone`, `email`, `anos_academicos`, `cursos`, `type`, `nivel_escolar` e `nif` não são aceitos nesta rota. Use `PUT /me/email` e `PUT /me/telefone` para contatos, `POST/DELETE /academia/anos-academicos` para anos acadêmicos e as rotas `/academia/curso` para cursos. Alterações de `type` e `nivel_escolar` exigem documento comprobativo pelo fluxo dedicado da tarefa 07 e ficam indisponíveis por este caminho. Alteração de `nif` exige aprovação de um Admin (role `adm` ou `fpp`) pelo fluxo abaixo. Se qualquer campo não permitido aparecer no payload, a requisição falha inteira com `400` e nenhum campo é alterado.

---

### Solicitações de alteração de NIF de academia

`nif` não é um dado único entre academias — a mesma entidade fiscal pode estar associada a mais de uma academia na plataforma. Ainda assim, a alteração do `nif` de uma academia já cadastrada exige aprovação: a academia solicita, e só um Admin (role `adm` ou `fpp`) pode aprovar ou reprovar. Aprovar aplica o novo `nif` imediatamente; reprovar não altera nenhum dado. Diferente das solicitações de edição de dados de estudante, este fluxo não exige documento comprobativo.

#### POST /academia/solicitacoes-nif

Cria uma solicitação de alteração de NIF para a academia autenticada. Nada é alterado em `nif` neste momento — apenas o pedido é registrado com status `pendente`.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "novo_nif": "0098765432"
}
```

**Response 201:**

```json
{
  "message": "solicitação de alteração de NIF criada com sucesso",
  "codigo_solicitacao": "SNF12345678",
  "nif_atual": "0012345678",
  "nif_solicitado": "0098765432",
  "status": "pendente"
}
```

**Regras de negócio:** `novo_nif` precisa ter exatamente 10 dígitos e ser diferente do `nif` atual da academia. Só pode existir uma solicitação `pendente` por academia — uma segunda tentativa retorna `409` mesmo sob concorrência (índice único parcial no banco, além da guarda de operação única).

**Erros:** `400` `novo_nif` ausente/inválido ou igual ao atual; `409` já existe solicitação pendente para esta academia.

---

#### GET /academia/solicitacoes-nif

Lista as solicitações de alteração de NIF da própria academia autenticada, em qualquer status.

**Proteção**: autenticado + academia ativa

**Query Params:** `status` — filtro opcional por `pendente`, `aprovada` ou `reprovada`; `limit` (padrão 50, teto 100); `offset` (padrão 0).

**Request:** sem payload

**Response 200:** mesmo formato de `GET /dominis/solicitacoes-nif-academia` abaixo, restrito à própria academia.

---

#### GET /dominis/solicitacoes-nif-academia

Lista solicitações de alteração de NIF de qualquer academia. Visível a qualquer admin autenticado; a decisão (aprovar/reprovar) exige role `adm` ou `fpp`.

**Proteção**: autenticado + admin

**Query Params:** `status`, `codigo_academia` — ambos opcionais; `limit` (padrão 50, teto 100); `offset` (padrão 0).

**Request:** sem payload

**Response 200:**

```json
{
  "solicitacoes": [
    {
      "codigo_solicitacao": "SNF12345678",
      "codigo_academia": "LDA20261",
      "nif_atual": "0012345678",
      "nif_solicitado": "0098765432",
      "status": "pendente",
      "motivo_reprovacao": null,
      "solicitado_por": "LDA20261",
      "decidido_por": null,
      "created_at": "2026-09-03T00:00:00Z",
      "updated_at": "2026-09-03T00:00:00Z",
      "version": 1
    }
  ],
  "limit": 50,
  "offset": 0,
  "total": 1
}
```

---

#### PUT /dominis/solicitacoes-nif-academia/:codigo/aprovar

Aprova uma solicitação pendente de alteração de NIF. Aplica o `nif_solicitado` na academia imediatamente.

**Proteção**: autenticado + admin com role `adm` ou `fpp`

**Path Params:** `codigo` — `codigo_solicitacao`

**Request:** sem payload

**Response 200:**

```json
{
  "message": "solicitação decidida com sucesso",
  "codigo_solicitacao": "SNF12345678",
  "status": "aprovada"
}
```

**Regras de negócio:** a solicitação precisa estar `pendente`. Grava `AcademiaNIFAlteradoPorSolicitacao` na academia (formato revalidado) e só então marca a solicitação como `aprovada`; se a alteração na academia falhar, a solicitação permanece `pendente` e nada é persistido.

**Erros:** `403` role insuficiente (`gerente` não pode decidir); `404` solicitação inexistente; `409` solicitação já decidida.

---

#### PUT /dominis/solicitacoes-nif-academia/:codigo/reprovar

Reprova uma solicitação pendente de alteração de NIF. Nenhum dado da academia é alterado.

**Proteção**: autenticado + admin com role `adm` ou `fpp`

**Path Params:** `codigo` — `codigo_solicitacao`

**Request:**

```json
{
  "motivo_reprovacao": "NIF não confere com o registro da AGT"
}
```

**Response 200:** igual ao endpoint de aprovação, com `status = "reprovada"`.

**Regras de negócio:** exige `motivo_reprovacao` não vazio. Grava `SolicitacaoAlteracaoNIFAcademiaReprovada`; `nif` da academia permanece inalterado.

**Erros:** `400` `motivo_reprovacao` ausente/vazio; `403` role insuficiente; `404` solicitação inexistente; `409` solicitação já decidida.
```

---

## 5. Checklist de validação

- [ ] `gofmt -l .` sem saída (nenhum arquivo mal formatado).
- [ ] `go build ./...` sem erro.
- [ ] `go vet ./...` sem erro.
- [ ] `go test ./...` (sem env vars de integração) — 100% verde, cobre os testes novos de `solicitacao_alteracao_nif_academia_test.go`.
- [ ] Se rodar os testes de integração (`RUN_POSTGRES_INTEGRATION=1`), lembre-se de exportar `FINANCE_ENCRYPTION_KEY` (qualquer string) e rodar com `-p 1` — ver nota da seção 0.
- [ ] `grep -rn "GetByNIF" internal/handlers/academia_handlers.go` não retorna nada (as duas checagens de conflito foram removidas; o método em si continua em `academia_projection.go`).
- [ ] `grep -rn "nif já cadastrado" .` não retorna nada em código nem na documentação.

## 6. Critérios de aceite

1. Duas academias podem ter o mesmo `nif` — cadastro público e administrativo não bloqueiam mais por `nif` duplicado.
2. `PUT /academia/dados` continua rejeitando o campo `nif`, agora com mensagem apontando para `POST /academia/solicitacoes-nif`.
3. `POST /academia/solicitacoes-nif` cria uma solicitação `pendente` e **não** altera `projection_academias.nif`.
4. Uma segunda solicitação pendente da mesma academia é rejeitada com `409`, mesmo sob concorrência real.
5. `PUT /dominis/solicitacoes-nif-academia/:codigo/aprovar` só funciona para admin com role `adm` ou `fpp` (role `gerente` recebe `403`) e altera `projection_academias.nif` de fato.
6. `PUT /dominis/solicitacoes-nif-academia/:codigo/reprovar` exige `motivo_reprovacao` e **não** altera `projection_academias.nif`.
7. Uma solicitação já decidida (aprovada ou reprovada) não pode ser decidida de novo (`409`).
8. `go build`, `go vet`, `gofmt` e `go test ./...` limpos.

## 7. Procedimento de conclusão

1. Depois de tudo validado, mova este arquivo de `docs/Lista de Tarefas/` para `docs/Tarefas feitas/`, renomeando para o padrão já usado nesse diretório (ex.: `81 - Permitir NIF duplicado entre academias com aprovacao de alteracao por admin.md`).
2. Não apague as seções de contexto/mapeamento ao mover — elas servem de histórico, igual às tarefas anteriores.
3. Confirme que a tarefa irmã do frontend (`Tarefa - Permitir alteracao de NIF de academia mediante aprovacao (Frontend).md`, no repositório `spuripainel`) está pronta para subir junto — ver seção "Coordenação de deploy" desse documento.

## 8. Perguntas em aberto (não bloqueiam a execução, mas o Fredy deve decidir depois)

- Hoje `getAcademiaProjection(c).GetByNIF(nif)` retorna só uma academia (a mais recente, pela query). Como o mesmo NIF agora pode estar em várias academias, esse método deveria virar `ListByNIF` para uso administrativo (ex.: relatório "todas as academias com este NIF")? Não fiz essa mudança porque nada no pedido original menciona essa necessidade, e o método não é usado em nenhum outro lugar hoje.
- Vale a pena permitir que o próprio admin que está decidindo also filtre por academia específica direto na tela (`GET /dominis/solicitacoes-nif-academia?codigo_academia=...`)? O endpoint já suporta isso; só não criei uma tela dedicada de "fila global de solicitações de NIF" — o frontend expõe isso por academia, dentro da tela de detalhes (ver documento irmão do frontend, seção 6).
