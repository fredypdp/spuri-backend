---
criado: 04-09-2026
origem: Fredy + Claude (orquestração)
status: pronto para execução — depende da Tarefa 09 estar concluída
tipo: backend (spuri-backend)
depende_de: Tarefa 09 (Fase 1)
---

# Tarefa 10 — Módulo de Serviços Extras — Fase 2: Solicitação, taxa de inscrição e cancelamento

### Documento de execução para o Codex (orquestrado e pré-testado pelo Claude)

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Mesma situação da Tarefa 09: sem `apt`/Docker/`psql` aqui, mas isto já foi validado por mim com PostgreSQL 16 real:

- A migration 119 desta fase (seção 4.1) aplica sem erro sobre as 117 migrations já existentes no repositório + a migration 118 da Tarefa 09.
- O índice único parcial `ux_sol_servico_extra_ativa` — a peça mais delicada do schema desta fase — foi testado com 4 passos manuais (criar → tentar duplicar → reprovar a primeira → criar de novo) e se comportou exatamente como especificado na seção 9.
- **A parte financeira (seção 6) é a de maior risco desta fase** — são edições cirúrgicas em `internal/finance/appypay.go` e `internal/finance/cobranca_geracao.go`, arquivos grandes e já existentes. Eu li e reproduzi integralmente os trechos relevantes desses arquivos antes de escrever as instruções desta seção (as funções `IniciarPagamentoMatricula`, `CodigoSolicitacaoDaCobranca`, `CancelarCobrancaMatriculaAberta`, `scanCobrancaResumo`, `origensClause`, `gerarCobranca` foram lidas por completo, não só resumidas), e confirmei — comparando o clone do repositório de antes e de depois das Tarefas 81-82 — que **nenhum desses arquivos foi alterado** por elas. As instruções da seção 6 continuam válidas linha a linha.

**Não pude compilar (`go build ./...`) nem rodar `go test` neste ambiente** — mesma limitação de rede detalhada na Tarefa 09 (seção 0 daquele documento): `golang.org/x/*`, `google.golang.org/protobuf` e `go.opentelemetry.io/auto/sdk` são bloqueados pelo proxy do meu sandbox antes mesmo de eu conseguir usar um espelho GitHub via `git insteadOf` (a checagem HTTP `?go-get=1` do próprio Go acontece antes do git entrar em cena). Esta é a fase com edições mais cirúrgicas em arquivos grandes já existentes — dedique atenção redobrada ao `go build ./...`/`go vet ./...` no seu ambiente antes de considerar concluído.

Referências de linha em `internal/finance/*.go` são **exatas e confiáveis** (arquivos confirmadamente não tocados por outras tarefas concorrentes). Referências de linha em `cmd/server/main.go`/`internal/db/safe_queries.go` são aproximadas, pela mesma razão explicada na Tarefa 09.

Se for rodar a suíte completa: `FINANCE_ENCRYPTION_KEY` precisa estar definida no ambiente (qualquer string, para teste) e use `go test -p 1 ./...` — nota herdada da Tarefa 81, não específica desta tarefa.

## 1. Prompt recomendado para executar esta tarefa

> Confirme que a Tarefa 09 já está implementada e mergeada antes de começar. Aplique exatamente o que está descrito neste documento, na ordem das seções — a seção 6 (integração financeira) é a mais sensível, siga-a passo a passo sem pular nada. Não replaneje nada do que já está decidido. Ao final, rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...`, corrija qualquer erro, e preencha o checklist da seção 11.

> **Pré-requisito obrigatório:** a Tarefa 09 (Fase 1 — cadastro/configuração de `ServicoExtra`) precisa estar implementada, mergeada e com os testes passando antes de iniciar esta fase. Este documento assume que `internal/domain/aggregates/servico_extra.go`, a migration `118_servicos_extras.sql`, `ServicoExtraProjection` e `FinanceiroService.HasCredential` já existem exatamente como especificado na Tarefa 09.
>
> Leia `docs/Tarefas feitas/tarefa-codex-backend-spuri-backend.md` e este documento por completo antes de escrever qualquer código. Todas as decisões de arquitetura já foram tomadas — não é necessário planejar, apenas implementar fielmente.

## 2. Objetivo desta fase

Implementar o mecanismo de solicitação/inscrição descrito pelo dono do produto:

1. Estudante solicita inscrição num serviço extra (com ou sem documento anexado, conforme a configuração do serviço).
2. Academia decide:
   - **Aprovada + sem taxa de inscrição:** estudante é vinculado ao serviço **imediatamente**.
   - **Aprovada + com taxa de inscrição:** estudante só é vinculado **depois** de o pagamento da taxa ser confirmado (via AppyPay — cobrança única, mesmo mecanismo já usado para a taxa de matrícula).
   - **Reprovada:** o backend guarda e devolve o motivo; nada muda no estado do estudante.
3. A academia pode cancelar a inscrição de um estudante já vinculado a qualquer momento (e o próprio estudante também pode cancelar a sua própria inscrição).

## 3. Decisões de design já tomadas

1. **Um único aggregate cobre solicitação E inscrição ativa**: `SolicitacaoServicoExtra`, em `internal/domain/aggregates/solicitacao_servico_extra.go`. Não existe uma entidade "Inscrição" separada — o mesmo registro, ao mudar de status, passa a representar o vínculo ativo. Isto espelha deliberadamente `SolicitacaoMatricula` (`internal/domain/aggregates/solicitacao_matricula.go`), que já resolve exatamente este problema (aprovação com/sem taxa, vínculo só após pagamento, cancelamento). **Leia esse arquivo antes de escrever o novo** — a máquina de estados abaixo é uma adaptação direta dele.

2. **Sem código público alfanumérico.** `SolicitacaoMatricula` tem um `codigo_solicitacao` público porque o candidato ainda não tem conta (consulta/paga anonimamente por código). Aqui o estudante **já está autenticado** — a referência é sempre o `id` (UUID) do próprio aggregate nas rotas autenticadas. Não invente um código público equivalente.

3. **Máquina de estados** (6 estados, nomes exatos a usar em código e na migration):

   ```
   pendente ──Aprovar (sem taxa)──────────────────────────► vinculada
   pendente ──Aprovar (com taxa)──► aprovada_pendente_pagamento_taxa_inscricao ──pagamento confirmado──► vinculada
   pendente ──Reprovar──► reprovada                                    (terminal, nada mais muda)
   aprovada_pendente_pagamento_taxa_inscricao ──CancelarAntesDaVinculacao──► cancelada_antes_da_vinculacao   (terminal)
   vinculada ──Cancelar──► cancelada                                    (terminal — "cancelar inscrição de um estudante")
   ```

   `aprovada_pendente_pagamento_taxa_inscricao` e `cancelada_antes_da_vinculacao` só existem no caminho COM taxa. `vinculada` é alcançada diretamente a partir de `pendente` quando o serviço não tem taxa de inscrição, ou a partir do estado de pagamento pendente quando a taxa é paga — em ambos os casos é o **mesmo** evento terminal (`SolicitacaoServicoExtraVinculada`), porque não há nenhuma outra entidade a criar neste momento (diferente da matrícula, que cria um `Estudante` novo — aqui o estudante já existe).

   Não funda `cancelada_antes_da_vinculacao` com `cancelada`: são situações de negócio diferentes (uma nunca chegou a vincular; a outra estava ativa e foi encerrada) e relatórios/estatísticas da academia vão precisar distingui-las.

4. **Duas ações de cancelamento, um único verbo de API por ator, despachado por status atual:**
   - `PUT /academia/servicos-extras/inscricoes/:id/cancelar` — a academia cancela; internamente decide, pelo `status` atual do aggregate, se chama `CancelarAntesDaVinculacao` (quando `aprovada_pendente_pagamento_taxa_inscricao`) ou `Cancelar` (quando `vinculada`). Qualquer outro status → erro de conflito.
   - `PUT /estudante/servicos-extras/minhas-inscricoes/:id/cancelar` — o próprio estudante cancela a própria solicitação/inscrição, com a mesma lógica de despacho por status. **Extensão deliberada em relação ao pedido original** (que falava de a academia cancelar a inscrição de um estudante): permitir que o estudante desista voluntariamente é um caso de uso natural e de baixo custo de implementação, reaproveitando o mesmo aggregate/método. Ambas as rotas verificam posse do recurso antes de agir (`CodigoAcademia`/`CodigoEstudante` batem com o ator autenticado).

5. **Taxa de inscrição é paga pelo módulo AppyPay já existente**, reaproveitando ao máximo `internal/finance` — nada de um novo gateway ou lógica de cobrança paralela. Isto exige estender `internal/finance` com um novo "tipo de origem" de cobrança (`servico_extra`), em tudo análogo a como `matricula` já é distinguida de `mensalidade` hoje (ver seção 6). Esta é a parte mais delicada desta fase — siga a seção 6 à risca, na ordem apresentada, porque os pontos de integração (criação de cobrança, consulta, webhook) têm de ficar coerentes entre si ou cobranças pagas via webhook deixam de vincular o estudante silenciosamente.

6. **Documento anexado é opcional por padrão da requisição, obrigatório por configuração do serviço.** Se `ServicoExtra.DocumentoObrigatorio == true`, o endpoint de solicitação exige o arquivo; caso contrário, aceita com ou sem. Reaproveita o mesmo mecanismo de upload (Mega/local) e validação de PDF já usado em `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go` (`readAndValidatePDF`, `MaxPDFUploadBytes`, `getStorageProvider(c)`).

7. **Uma solicitação/inscrição "viva" por par (serviço, estudante).** Um estudante não pode ter duas solicitações simultaneamente ativas (`pendente`, `aprovada_pendente_pagamento_taxa_inscricao` ou `vinculada`) para o **mesmo** serviço — mas pode voltar a solicitar depois de uma `reprovada` ou `cancelada`/`cancelada_antes_da_vinculacao`. Isto é garantido em duas camadas, na mesma filosofia de defesa em profundidade já usada em `solicitacao_edicao_dado_estudante_handlers.go` (guard em memória + checagem de projeção antes de gravar): (a) um `db.UniqueOperationGuard` reservado antes de criar o aggregate; (b) um índice único parcial na tabela de projeção (seção 4, já testado).

8. **Fora de escopo nesta fase:** cobrança recorrente mensal para serviços `tipo_cobranca=mensal` (Fase 3) e a cobrança do preço do serviço quando `tipo_cobranca=unico` (também fica para a Fase 3, que trata as duas juntas — ver o documento da Fase 3 para o porquê). Nesta Fase 2, a única cobrança real que existe é a **taxa de inscrição**.

## 4. Modelo de dados

### 4.1 Migration

Crie `migrations/119_solicitacoes_servico_extra.sql`. **Já testado manualmente** contra PostgreSQL 16 real, incluindo o índice único parcial (seção 8 do relatório de validação, seção 10 abaixo). Use exatamente:

```sql
CREATE TABLE IF NOT EXISTS projection_solicitacoes_servico_extra (
    id UUID PRIMARY KEY,
    servico_extra_id UUID NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    codigo_estudante VARCHAR(20) NOT NULL,
    status VARCHAR(50) NOT NULL CHECK (status IN (
        'pendente',
        'aprovada_pendente_pagamento_taxa_inscricao',
        'vinculada',
        'reprovada',
        'cancelada_antes_da_vinculacao',
        'cancelada'
    )),
    motivo_reprovacao TEXT,
    motivo_cancelamento TEXT,
    cancelada_por VARCHAR(10) CHECK (cancelada_por IN ('academia','estudante')),
    documento_path TEXT,
    documento_url TEXT,
    valor_taxa_inscricao NUMERIC(14,2),
    metodos_pagamento_taxa_inscricao TEXT[],
    aprovada_por UUID,
    reprovada_por UUID,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID
);
CREATE INDEX IF NOT EXISTS idx_sol_servico_extra_academia ON projection_solicitacoes_servico_extra(codigo_academia, status);
CREATE INDEX IF NOT EXISTS idx_sol_servico_extra_estudante ON projection_solicitacoes_servico_extra(codigo_estudante, status);
CREATE UNIQUE INDEX IF NOT EXISTS ux_sol_servico_extra_ativa
    ON projection_solicitacoes_servico_extra (servico_extra_id, codigo_estudante)
    WHERE status IN ('pendente','aprovada_pendente_pagamento_taxa_inscricao','vinculada');
```

Não remova nem "simplifique" o índice único parcial `ux_sol_servico_extra_ativa` — ele é a rede de segurança final contra o cenário de duas requisições HTTP concorrentes do mesmo estudante furando o guard em memória (que já cobre o caso comum, mas não é atômico com a escrita no banco).

### 4.2 Whitelist do ledger

Em `internal/db/safe_queries.go`:

- `validAggregateTypes`: adicione `"SolicitacaoServicoExtra": true,`
- `validEventTypes`: adicione:
  ```go
  "SolicitacaoServicoExtraCriada":                     true,
  "SolicitacaoServicoExtraAprovadaPendentePagamento":  true,
  "SolicitacaoServicoExtraVinculada":                  true,
  "SolicitacaoServicoExtraReprovada":                  true,
  "SolicitacaoServicoExtraCanceladaAntesDaVinculacao": true,
  "SolicitacaoServicoExtraCancelada":                  true,
  ```

Repare bem nesta lista — são **6** eventos, os mesmos 6 nomes usados em `Apply`/`RaiseEvent` no aggregate da seção 5. Qualquer divergência de string entre o aggregate e esta whitelist falha silenciosamente com "tipo de evento inválido" (ver aviso equivalente na Tarefa 09, seção 5.2 — mesma classe de erro, já mordeu este repositório antes).

### 4.3 Factory

Em `internal/domain/aggregates/aggregate.go`, `DefaultAggregateFactory.Create`:

```go
case "SolicitacaoServicoExtra":
    return NewSolicitacaoServicoExtra(), nil
```

## 5. Aggregate `SolicitacaoServicoExtra`

Crie `internal/domain/aggregates/solicitacao_servico_extra.go` com este conteúdo:

```go
package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	StatusInscricaoPendente                      = "pendente"
	StatusInscricaoAprovadaPendentePagamentoTaxa = "aprovada_pendente_pagamento_taxa_inscricao"
	StatusInscricaoVinculada                     = "vinculada"
	StatusInscricaoReprovada                     = "reprovada"
	StatusInscricaoCanceladaAntesDaVinculacao    = "cancelada_antes_da_vinculacao"
	StatusInscricaoCancelada                     = "cancelada"
)

// SolicitacaoServicoExtra representa, ao mesmo tempo, o pedido de inscrição
// de um estudante num ServicoExtra e — uma vez aprovado e (se aplicável)
// pago — o próprio vínculo ativo. Não existe uma entidade "Inscrição"
// separada: o mesmo registro muda de status. Ver decisão de design 1 no
// documento da Tarefa 10 para a justificativa (espelha SolicitacaoMatricula,
// que resolve o mesmo problema para matrícula).
type SolicitacaoServicoExtra struct {
	BaseAggregate

	ServicoExtraID  uuid.UUID
	CodigoAcademia  string
	CodigoEstudante string
	Status          string

	MotivoReprovacao   string
	MotivoCancelamento string
	CanceladaPor       string // "academia" | "estudante"

	DocumentoPath string
	DocumentoURL  string

	ValorTaxaInscricao            float64
	MetodosPagamentoTaxaInscricao []string

	AprovadaPor  uuid.UUID
	ReprovadaPor uuid.UUID

	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewSolicitacaoServicoExtra() *SolicitacaoServicoExtra {
	return &SolicitacaoServicoExtra{
		BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}},
	}
}

func (s *SolicitacaoServicoExtra) GetType() string { return "SolicitacaoServicoExtra" }

// ---------------------------------------------------------------------------
// Eventos
// ---------------------------------------------------------------------------

type SolicitacaoServicoExtraCriadaEvent struct {
	BaseEvent
	ServicoExtraID  uuid.UUID
	CodigoAcademia  string
	CodigoEstudante string
	DocumentoPath   string
	DocumentoURL    string
	CreatedAt       time.Time
}

func (e *SolicitacaoServicoExtraCriadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoServicoExtraAprovadaPendentePagamentoEvent struct {
	BaseEvent
	ValorTaxaInscricao            float64
	MetodosPagamentoTaxaInscricao []string
	AprovadaPor                   uuid.UUID
	UpdatedAt                     time.Time
}

func (e *SolicitacaoServicoExtraAprovadaPendentePagamentoEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraAprovadaPendentePagamentoEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// SolicitacaoServicoExtraVinculadaEvent é o evento terminal de vínculo,
// alcançado tanto pelo caminho sem taxa (a partir de "pendente", com
// AprovadaPor preenchido) quanto pelo caminho com taxa paga (a partir de
// "aprovada_pendente_pagamento_taxa_inscricao", com AprovadaPor
// uuid.Nil — quem vincula é a confirmação de pagamento, não uma pessoa).
type SolicitacaoServicoExtraVinculadaEvent struct {
	BaseEvent
	AprovadaPor uuid.UUID
	UpdatedAt   time.Time
}

func (e *SolicitacaoServicoExtraVinculadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraVinculadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoServicoExtraReprovadaEvent struct {
	BaseEvent
	MotivoReprovacao string
	ReprovadaPor     uuid.UUID
	UpdatedAt        time.Time
}

func (e *SolicitacaoServicoExtraReprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraReprovadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent struct {
	BaseEvent
	MotivoCancelamento string
	CanceladaPor       string
	UpdatedAt          time.Time
}

func (e *SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent) GetPayload() interface{} {
	return e
}
func (e *SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

type SolicitacaoServicoExtraCanceladaEvent struct {
	BaseEvent
	MotivoCancelamento string
	CanceladaPor       string
	UpdatedAt          time.Time
}

func (e *SolicitacaoServicoExtraCanceladaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraCanceladaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ---------------------------------------------------------------------------
// Apply
// ---------------------------------------------------------------------------

func (s *SolicitacaoServicoExtra) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "SolicitacaoServicoExtraCriada":
		return s.applyCriada(event)
	case "SolicitacaoServicoExtraAprovadaPendentePagamento":
		return s.applyAprovadaPendentePagamento(event)
	case "SolicitacaoServicoExtraVinculada":
		return s.applyVinculada(event)
	case "SolicitacaoServicoExtraReprovada":
		return s.applyReprovada(event)
	case "SolicitacaoServicoExtraCanceladaAntesDaVinculacao":
		return s.applyCanceladaAntesDaVinculacao(event)
	case "SolicitacaoServicoExtraCancelada":
		return s.applyCancelada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido para SolicitacaoServicoExtra: %s", event.GetEventType())
	}
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (s *SolicitacaoServicoExtra) Criar(servicoExtraID uuid.UUID, codigoAcademia, codigoEstudante, documentoPath, documentoURL string) error {
	if servicoExtraID == uuid.Nil {
		return fmt.Errorf("servico_extra_id é obrigatório")
	}
	if strings.TrimSpace(codigoAcademia) == "" || strings.TrimSpace(codigoEstudante) == "" {
		return fmt.Errorf("codigo_academia e codigo_estudante são obrigatórios")
	}
	event := &SolicitacaoServicoExtraCriadaEvent{
		BaseEvent:       BaseEvent{EventType: "SolicitacaoServicoExtraCriada", AggregateID: s.ID},
		ServicoExtraID:  servicoExtraID,
		CodigoAcademia:  codigoAcademia,
		CodigoEstudante: codigoEstudante,
		DocumentoPath:   documentoPath,
		DocumentoURL:    documentoURL,
		CreatedAt:       time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// Aprovar aprova uma solicitação pendente. Quando o serviço não tem taxa de
// inscrição, vincula imediatamente. Quando tem, congela valor/métodos da
// taxa NO MOMENTO DA APROVAÇÃO (mudanças posteriores no ServicoExtra não
// alteram o que este estudante especificamente tem a pagar — mesmo
// princípio já usado em SolicitacaoMatricula.MarcarPendentePagamentoMatricula)
// e aguarda pagamento.
func (s *SolicitacaoServicoExtra) Aprovar(temTaxaInscricao bool, valorTaxa float64, metodosTaxa []string, aprovadaPor uuid.UUID) error {
	if s.Status != StatusInscricaoPendente {
		return fmt.Errorf("apenas solicitações pendentes podem ser aprovadas (status atual: %s)", s.Status)
	}
	if !temTaxaInscricao {
		event := &SolicitacaoServicoExtraVinculadaEvent{
			BaseEvent:   BaseEvent{EventType: "SolicitacaoServicoExtraVinculada", AggregateID: s.ID},
			AprovadaPor: aprovadaPor,
			UpdatedAt:   time.Now(),
		}
		s.RaiseEvent(event)
		return s.Apply(event)
	}
	if valorTaxa <= 0 || len(metodosTaxa) == 0 {
		return fmt.Errorf("valor e métodos de pagamento da taxa de inscrição são obrigatórios quando o serviço exige taxa")
	}
	event := &SolicitacaoServicoExtraAprovadaPendentePagamentoEvent{
		BaseEvent:                     BaseEvent{EventType: "SolicitacaoServicoExtraAprovadaPendentePagamento", AggregateID: s.ID},
		ValorTaxaInscricao:            valorTaxa,
		MetodosPagamentoTaxaInscricao: metodosTaxa,
		AprovadaPor:                   aprovadaPor,
		UpdatedAt:                     time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

func (s *SolicitacaoServicoExtra) Reprovar(motivo string, reprovadaPor uuid.UUID) error {
	if s.Status != StatusInscricaoPendente {
		return fmt.Errorf("apenas solicitações pendentes podem ser reprovadas (status atual: %s)", s.Status)
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo_reprovacao é obrigatório")
	}
	event := &SolicitacaoServicoExtraReprovadaEvent{
		BaseEvent:        BaseEvent{EventType: "SolicitacaoServicoExtraReprovada", AggregateID: s.ID},
		MotivoReprovacao: motivo,
		ReprovadaPor:     reprovadaPor,
		UpdatedAt:        time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// VincularAposPagamento efetiva o vínculo depois de o pagamento da taxa de
// inscrição ser confirmado pelo módulo financeiro (chamado a partir dos três
// pontos de confirmação descritos na seção 6.5: resposta síncrona, consulta
// e webhook — exatamente como já acontece para matrícula).
func (s *SolicitacaoServicoExtra) VincularAposPagamento() error {
	if s.Status != StatusInscricaoAprovadaPendentePagamentoTaxa {
		return fmt.Errorf("solicitação não está aguardando pagamento de taxa de inscrição (status atual: %s)", s.Status)
	}
	event := &SolicitacaoServicoExtraVinculadaEvent{
		BaseEvent: BaseEvent{EventType: "SolicitacaoServicoExtraVinculada", AggregateID: s.ID},
		UpdatedAt: time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// CancelarAntesDaVinculacao desiste de uma solicitação já aprovada mas ainda
// aguardando pagamento da taxa — nunca chegou a vincular.
func (s *SolicitacaoServicoExtra) CancelarAntesDaVinculacao(motivo, canceladaPor string) error {
	if s.Status != StatusInscricaoAprovadaPendentePagamentoTaxa {
		return fmt.Errorf("apenas solicitações aguardando pagamento de taxa podem ser canceladas neste estágio (status atual: %s)", s.Status)
	}
	if canceladaPor != "academia" && canceladaPor != "estudante" {
		return fmt.Errorf("cancelada_por deve ser 'academia' ou 'estudante'")
	}
	event := &SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent{
		BaseEvent:          BaseEvent{EventType: "SolicitacaoServicoExtraCanceladaAntesDaVinculacao", AggregateID: s.ID},
		MotivoCancelamento: motivo,
		CanceladaPor:       canceladaPor,
		UpdatedAt:          time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// Cancelar cancela uma inscrição já vinculada/ativa — "cancelar inscrição de
// um estudante" do pedido original, mais a extensão de autocancelamento
// (decisão de design 4).
func (s *SolicitacaoServicoExtra) Cancelar(motivo, canceladaPor string) error {
	if s.Status != StatusInscricaoVinculada {
		return fmt.Errorf("apenas inscrições vinculadas podem ser canceladas (status atual: %s)", s.Status)
	}
	if canceladaPor != "academia" && canceladaPor != "estudante" {
		return fmt.Errorf("cancelada_por deve ser 'academia' ou 'estudante'")
	}
	event := &SolicitacaoServicoExtraCanceladaEvent{
		BaseEvent:          BaseEvent{EventType: "SolicitacaoServicoExtraCancelada", AggregateID: s.ID},
		MotivoCancelamento: motivo,
		CanceladaPor:       canceladaPor,
		UpdatedAt:          time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// ---------------------------------------------------------------------------
// Apply handlers
// ---------------------------------------------------------------------------

func (s *SolicitacaoServicoExtra) applyCriada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraCriadaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.ServicoExtraID = p.ServicoExtraID
	s.CodigoAcademia = p.CodigoAcademia
	s.CodigoEstudante = p.CodigoEstudante
	s.DocumentoPath = p.DocumentoPath
	s.DocumentoURL = p.DocumentoURL
	s.Status = StatusInscricaoPendente
	s.CreatedAt = p.CreatedAt
	s.UpdatedAt = p.CreatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyAprovadaPendentePagamento(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraAprovadaPendentePagamentoEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoAprovadaPendentePagamentoTaxa
	s.ValorTaxaInscricao = p.ValorTaxaInscricao
	s.MetodosPagamentoTaxaInscricao = p.MetodosPagamentoTaxaInscricao
	s.AprovadaPor = p.AprovadaPor
	s.UpdatedAt = p.UpdatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyVinculada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraVinculadaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoVinculada
	if p.AprovadaPor != uuid.Nil {
		s.AprovadaPor = p.AprovadaPor
	}
	s.UpdatedAt = p.UpdatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyReprovada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraReprovadaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoReprovada
	s.MotivoReprovacao = p.MotivoReprovacao
	s.ReprovadaPor = p.ReprovadaPor
	s.UpdatedAt = p.UpdatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyCanceladaAntesDaVinculacao(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoCanceladaAntesDaVinculacao
	s.MotivoCancelamento = p.MotivoCancelamento
	s.CanceladaPor = p.CanceladaPor
	s.UpdatedAt = p.UpdatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyCancelada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraCanceladaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoCancelada
	s.MotivoCancelamento = p.MotivoCancelamento
	s.CanceladaPor = p.CanceladaPor
	s.UpdatedAt = p.UpdatedAt
	return nil
}
```

## 6. Integração com o módulo financeiro (`internal/finance`)

Esta seção altera arquivos existentes do módulo AppyPay. Siga a ordem exata — cada passo depende do anterior compilar.

### 6.1 Novo campo de metadado em `ChargeRequest` e `QRCodeRequest`

Em `internal/finance/appypay.go`, na struct `ChargeRequest` (linha 129), logo depois do campo `CodigoSolicitacao` (linha 144):

```go
CodigoInscricaoServico string `json:"codigo_inscricao_servico,omitempty"`
```

Repita exatamente igual na struct `QRCodeRequest` (linha 146), depois do seu próprio `CodigoSolicitacao` (linha 161).

Estes campos são metadados de auditoria do Spuri — **nunca são enviados à AppyPay** (mesmo comentário que já existe em `ChargeRequest` para `CodigoEstudante`/`CodigoSolicitacao`, linha 141).

### 6.2 Persistir o novo campo no payload gravado

Ainda em `appypay.go`, nos dois `return map[string]any{...}` que constroem o payload persistido em `financeiro_cobrancas` (linhas 1839 e 1846), adicione a chave:

```go
"codigo_inscricao_servico": in.CodigoInscricaoServico,
```

em ambos os literais (um é usado por `CreateCharge`, o outro por `CreateGPOQRCode`).

### 6.3 Categorização de origem (`CobrancaResumo`, `scanCobrancaResumo`, `origensClause`)

Em `internal/finance/appypay.go`:

1. Na struct `CobrancaResumo` (por volta da linha 237, junto a `CodigoEstudante`/`CodigoSolicitacao`), adicione:
   ```go
   CodigoInscricaoServico string `json:"codigo_inscricao_servico,omitempty"`
   ```
   Atualize o comentário de `Origem` (linhas 196-202) para mencionar a nova categoria `"servico_extra"`.

2. Em `scanCobrancaResumo` (a partir da linha 927), depois da linha que lê `dto.CodigoSolicitacao`, adicione:
   ```go
   dto.CodigoInscricaoServico, _ = payload["codigo_inscricao_servico"].(string)
   ```
   E ajuste o `switch` de derivação de `Origem` (a partir de `case dto.CodigoSolicitacao != "":`) para a **ordem exata** abaixo — a ordem importa, porque uma cobrança de taxa de inscrição de serviço extra também tem `codigo_estudante` preenchido e cairia erradamente em `"mensalidade"` se `CodigoInscricaoServico` não for checado antes:
   ```go
   switch {
   case dto.CodigoSolicitacao != "":
       dto.Origem = "matricula"
   case dto.CodigoInscricaoServico != "":
       dto.Origem = "servico_extra"
   case dto.CodigoEstudante != "":
       dto.Origem = "mensalidade"
   default:
       dto.Origem = "avulsa"
   }
   ```

3. Em `origensClause` (função dedicada, próxima ao topo de `appypay.go`, ~linha 897), adicione o caso `"servico_extra"` e ajuste `"mensalidade"`/`"avulsa"` para excluí-lo, preservando exclusão mútua com o switch acima:
   ```go
   case "matricula":
       clauses = append(clauses, "COALESCE(payload->>'codigo_solicitacao','') <> ''")
   case "servico_extra":
       clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_inscricao_servico','') <> '')")
   case "mensalidade":
       clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_inscricao_servico','') = '' AND COALESCE(payload->>'codigo_estudante','') <> '')")
   case "avulsa":
       clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_inscricao_servico','') = '' AND COALESCE(payload->>'codigo_estudante','') = '')")
   ```
   **Não pule este passo mesmo que pareça só filtro de listagem** — sem ele, `ListCobrancasEstudante`/`ListCobrancas` com filtro `origem=mensalidade` passam a incluir, incorretamente, as cobranças de taxa de inscrição de serviço extra (que têm `codigo_estudante` preenchido).

### 6.4 `gerarCobrancaInput` — novo campo

Em `internal/finance/cobranca_geracao.go`:

1. Adicione `CodigoInscricaoServico string` à struct `gerarCobrancaInput` (junto a `CodigoSolicitacao`).
2. Dentro de `gerarCobranca`, adicione `CodigoInscricaoServico: in.CodigoInscricaoServico,` em **ambos** os literais que constroem `QRCodeRequest{...}` e `ChargeRequest{...}`.
3. Atualize o comentário de topo do arquivo (que hoje diz "preencha apenas o(s) que fizer(em) sentido... CodigoEstudante para mensalidade, CodigoSolicitacao para matrícula") para incluir "CodigoInscricaoServico para taxa de inscrição de serviço extra".

### 6.5 Novo arquivo `internal/finance/servico_extra.go`

Crie este arquivo com as três funções que orquestram o pagamento da taxa de inscrição, mirror direto de `IniciarPagamentoMatricula` / `CodigoSolicitacaoDaCobranca` / `CancelarCobrancaMatriculaAberta` em `internal/finance/matricula.go` (linhas 215-284 — releia antes de escrever este arquivo):

```go
package finance

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/lib/pq"
)

type TaxaInscricaoServicoExtraPagamentoInput struct {
	SolicitacaoID   string `json:"-"` // preenchido pelo handler a partir do path/identidade, nunca do body do cliente
	MetodoPagamento string `json:"metodo_pagamento"`
	Telefone        string `json:"telefone,omitempty"`
}

type TaxaInscricaoServicoExtraPagamentoView struct {
	Charge QRCodeResult `json:"cobranca"`
}

// IniciarPagamentoTaxaInscricaoServicoExtra inicia a cobrança da taxa de
// inscrição de uma SolicitacaoServicoExtra já aprovada e aguardando
// pagamento. Mirror direto de IniciarPagamentoMatricula — mesma validação de
// estado, mesma checagem de cobrança aberta duplicada, mesmo uso de
// gerarCobranca.
func (s *Service) IniciarPagamentoTaxaInscricaoServicoExtra(ctx context.Context, in TaxaInscricaoServicoExtraPagamentoInput, ip string) (TaxaInscricaoServicoExtraPagamentoView, error) {
	in.SolicitacaoID = strings.TrimSpace(in.SolicitacaoID)
	in.MetodoPagamento = strings.ToUpper(strings.TrimSpace(in.MetodoPagamento))
	if in.SolicitacaoID == "" || in.MetodoPagamento == "" {
		return TaxaInscricaoServicoExtraPagamentoView{}, errors.New("solicitação e método de pagamento são obrigatórios")
	}
	var academia, status string
	var valor sql.NullFloat64
	var metodos []string
	err := s.client.DB().QueryRowContext(ctx, `SELECT codigo_academia,status,valor_taxa_inscricao::float8,metodos_pagamento_taxa_inscricao FROM projection_solicitacoes_servico_extra WHERE id=$1::uuid`, in.SolicitacaoID).
		Scan(&academia, &status, &valor, pq.Array(&metodos))
	if err != nil || status != "aprovada_pendente_pagamento_taxa_inscricao" || !valor.Valid {
		return TaxaInscricaoServicoExtraPagamentoView{}, errors.New("solicitação não disponível para pagamento de taxa de inscrição")
	}
	if !contains(metodos, in.MetodoPagamento) {
		return TaxaInscricaoServicoExtraPagamentoView{}, errors.New("método de pagamento não está habilitado para esta taxa de inscrição")
	}
	open, err := s.servicoExtraTemCobrancaAberta(ctx, in.SolicitacaoID)
	if err != nil {
		return TaxaInscricaoServicoExtraPagamentoView{}, err
	}
	if open {
		return TaxaInscricaoServicoExtraPagamentoView{}, errors.New("solicitação já possui cobrança de taxa de inscrição em aberto")
	}
	result, err := s.gerarCobranca(ctx, gerarCobrancaInput{
		CodigoAcademia:         academia,
		MetodoPagamento:        in.MetodoPagamento,
		Amount:                 valor.Float64,
		Description:            "Taxa de inscrição - serviço extra " + academia,
		MerchantTransactionID:  merchantID(),
		Telefone:               in.Telefone,
		CodigoInscricaoServico: in.SolicitacaoID,
	}, "solicitacao_servico_extra:"+in.SolicitacaoID, "solicitante", ip)
	if err != nil {
		return TaxaInscricaoServicoExtraPagamentoView{}, err
	}
	return TaxaInscricaoServicoExtraPagamentoView{Charge: result}, nil
}

func (s *Service) servicoExtraTemCobrancaAberta(ctx context.Context, solicitacaoID string) (bool, error) {
	var ok bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_cobrancas WHERE payload->>'codigo_inscricao_servico'=$1 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`))`, solicitacaoID).Scan(&ok)
	return ok, err
}

// CodigoInscricaoServicoExtraDaCobranca identifica a solicitação de serviço
// extra associada a uma cobrança, sem expor detalhes do estudante ao código
// que fala com a AppyPay. Mirror de CodigoSolicitacaoDaCobranca.
func (s *Service) CodigoInscricaoServicoExtraDaCobranca(ctx context.Context, identifier string) (string, error) {
	row, err := s.loadCharge(ctx, identifier)
	if err != nil {
		return "", err
	}
	codigo, _ := row.Payload["codigo_inscricao_servico"].(string)
	return strings.TrimSpace(codigo), nil
}

// CancelarCobrancaTaxaInscricaoServicoAberta cancela qualquer cobrança em
// aberto da taxa de inscrição antes de a solicitação ser cancelada
// (CancelarAntesDaVinculacao). Mirror de CancelarCobrancaMatriculaAberta.
func (s *Service) CancelarCobrancaTaxaInscricaoServicoAberta(ctx context.Context, solicitacaoID, motivo, actorID, actorType, ip string) error {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id::text,codigo_academia FROM financeiro_cobrancas WHERE payload->>'codigo_inscricao_servico'=$1 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)`, solicitacaoID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, academia string
		if err := rows.Scan(&id, &academia); err != nil {
			return err
		}
		if _, err := s.CancelCharge(ctx, ContextoAcademia, academia, id, motivo, actorID, actorType, ip); err != nil {
			return err
		}
	}
	return rows.Err()
}
```

**Confira antes de compilar:** o import `"github.com/lib/pq"` e a função `contains(...)` já existem e são usados em `matricula.go`/`mensalidade.go` no mesmo pacote — reaproveite-os, não duplique. Se `contains` for um helper não exportado noutro arquivo do mesmo pacote `finance`, ele já está disponível aqui sem novo import.

### 6.6 Três pontos de confirmação de pagamento (handlers) — os três precisam existir

O padrão já usado para matrícula (`internal/handlers/solicitacao_matricula_handlers.go` e `internal/handlers/financeiro_handlers.go`) confirma pagamento em **três** lugares independentes, porque a AppyPay pode confirmar de forma síncrona, por consulta posterior, ou só por webhook — os três precisam ficar coerentes:

1. **Resposta síncrona** do próprio `POST` de início de pagamento (seção 7.5): se `out.Charge.Status == "success"`, vincula imediatamente.
2. **Consulta manual** (`GET /financeiro/appypay/cobrancas/:id` — `ConsultarCobrancaAppyPay`, `internal/handlers/financeiro_handlers.go`, por volta da linha 280-360): quando o status consultado virar sucesso, o handler já verifica `CodigoSolicitacaoDaCobranca` e chama `efetivarVinculoMatriculaPaga`. Adicione, no mesmo handler, um `else if`/bloco equivalente checando `FinanceiroService.CodigoInscricaoServicoExtraDaCobranca(...)` e chamando a nova função `efetivarVinculoServicoExtraPago` (seção 7.6) quando não vazio.
3. **Webhook** (`ReceberWebhookAppyPay`, `internal/handlers/financeiro_handlers.go`, linhas 586-618): depois do bloco que já trata `CodigoSolicitacaoDaCobranca`/`efetivarVinculoMatriculaPaga`, adicione o equivalente:
   ```go
   if codigoInscricaoServico, err := FinanceiroService.CodigoInscricaoServicoExtraDaCobranca(c.Request.Context(), eventID); err == nil && codigoInscricaoServico != "" {
       if err := efetivarVinculoServicoExtraPago(c, codigoInscricaoServico); err != nil {
           c.Status(http.StatusInternalServerError)
           return
       }
   }
   ```
   Isto **não é `else if`** com o bloco de matrícula — mantenha os dois blocos independentes e sequenciais (uma cobrança só pertence a uma origem de cada vez, mas o código não precisa presumir isso para estar correto).

**Se qualquer um dos três pontos faltar, existe um caminho realista em produção onde a AppyPay confirma o pagamento mas o estudante nunca é vinculado ao serviço** — este é o bug mais caro que esta fase pode introduzir. Teste os três (seção 10).

## 7. Handlers

Arquivo novo: `internal/handlers/servico_extra_solicitacao_handlers.go`.

### 7.1 Helper de carregamento

```go
func loadSolicitacaoServicoExtra(c *gin.Context, id uuid.UUID) (*aggregates.SolicitacaoServicoExtra, bool) {
	agg, err := getRepository(c).WithContext(c.Request.Context()).Load(id, "SolicitacaoServicoExtra")
	if err != nil {
		utils.RespondWithNotFoundError(c, "solicitação de serviço extra")
		return nil, false
	}
	sol, ok := agg.(*aggregates.SolicitacaoServicoExtra)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return nil, false
	}
	return sol, true
}
```

Confirme o nome exato do método de carregamento por ID em `internal/db/repository.go` (`Load`, possivelmente com `.WithContext(...)` encadeado como em outros handlers deste pacote — copie a chamada exata usada por `loadSolicitacaoByCodigo` em `solicitacao_matricula_handlers.go`, adaptando só o segundo argumento e a busca por `id` UUID em vez de `codigo` string).

### 7.2 `SolicitarServicoExtra` (estudante)

Rota: `POST /estudante/servicos-extras/:id/solicitacao`, grupo `estudante` (`RequireEstudante`).

Fluxo (mirror de `CriarSolicitacaoEdicaoDadoEstudanteHandler`, `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go`, linhas 24-120 — releia antes de escrever):

1. `userID, _ := middleware.GetUserID(c)`; `est, err := getEstudanteProjection(c).GetByID(userID)`; validar `est != nil` e `est.CodigoAcademia != nil`.
2. Carregar o `ServicoExtra` pela projeção (`ServicoExtraProjection.GetByID(servicoID)`, Fase 1); validar que existe, está `ativo=true` e que `servico.CodigoAcademia == *est.CodigoAcademia` (**um estudante só pode solicitar serviços da academia em que está atualmente vinculado** — decisão implícita, consistente com o resto do sistema: `est.CodigoAcademia` é sempre a academia atual do estudante).
3. Se `servico.AnosAcademicosDisponiveis` não for vazio, validar que o ano acadêmico atual do estudante (`est.AnoAcademico`/campo equivalente já existente na projeção de estudante — confirme o nome exato do campo) está na lista; caso contrário, `403`/`400` com mensagem clara ("serviço não disponível para o seu ano acadêmico").
4. `c.Request.ParseMultipartForm(MaxPDFUploadBytes + 1024)`.
5. Reservar guard: `db.NewUniqueOperationGuard(getDbClient(c)).WithContext(ctx).Reserve("solicitacao_servico_extra:ativa", db.CanonicalGuardKey(servicoID.String(), est.CodigoEstudante), db.UniqueGuardOptions{UserID: userID.String(), UserType: "estudante"})`; em conflito, `409` "já existe solicitação ativa para este serviço"; `defer` liberar se não consumido (mesmo padrão da seção 3, linhas 49-67 do arquivo de referência).
6. Checar pendência ativa também pela projeção (`SolicitacaoServicoExtraProjection.ExisteAtiva(servicoID, est.CodigoEstudante)` — implemente este método na projeção da seção 8) — mesmo padrão de dupla checagem já usado (guard + projeção) antes de gastar um upload.
7. `fh, err := c.FormFile("documento")`: se `servico.DocumentoObrigatorio` e `err != nil` → erro de validação "documento é obrigatório para este serviço"; se não obrigatório e `err != nil` (`http.ErrMissingFile`), seguir sem documento; se fornecido (obrigatório ou não), validar com `readAndValidatePDF("documento", fh)`.
8. Se houver documento: `path := fmt.Sprintf("%s/estudantes/%s/servicos_extras/%s.pdf", servico.CodigoAcademia, est.CodigoEstudante, <novo uuid gerado para a solicitação>)`; `provider.Upload(...)`. Gere o UUID da solicitação **antes** de montar o path (`sol := aggregates.NewSolicitacaoServicoExtra()`; use `sol.GetID().String()` no path), para o path já nascer associado ao aggregate certo.
9. `sol.Criar(servico.ID, servico.CodigoAcademia, est.CodigoEstudante, stored.Path, stored.FileURL)`; em erro, `provider.Delete(stored.Path)` antes de responder (mesmo padrão de limpeza da referência).
10. `SaveWithAudit`; em erro, `provider.Delete(stored.Path)`.
11. `guard.Consume(sol.GetID())`.
12. Responder `201` com os campos do aggregate em memória (nunca reler a projeção — mesma regra da Fase 1).

### 7.3 `AprovarSolicitacaoServicoExtra` / `ReprovarSolicitacaoServicoExtra` (academia)

Rotas: `PUT /academia/servicos-extras/solicitacoes/:id/aprovar` e `.../reprovar`, grupo `academia`.

- Carregar `academiaDTO` (posse), carregar `sol` (seção 7.1), validar `sol.CodigoAcademia == academiaDTO.CodigoAcademia` (`403` caso contrário).
- **Aprovar:** carregar o `ServicoExtra` (`servico.ID == sol.ServicoExtraID`) da projeção para saber `TemTaxaInscricao`/`ValorTaxaInscricao`/`MetodosPagamentoTaxaInscricao` **atuais** (são estes que ficam congelados na solicitação, seção 5 — comentário do método `Aprovar`); chamar `sol.Aprovar(servico.TemTaxaInscricao, servico.ValorTaxaInscricao, servico.MetodosPagamentoTaxaInscricao, userID)`; `SaveWithAudit`; responder `200` com `status` final (`"vinculada"` ou `"aprovada_pendente_pagamento_taxa_inscricao"`, já disponível em `sol.Status` após `Apply`).
- **Reprovar:** `motivo_reprovacao` obrigatório no body; `sol.Reprovar(motivo, userID)`; `SaveWithAudit`; responder `200`.

### 7.4 Cancelamento — academia e estudante

Handler comum (chamado pelas duas rotas com o ator/tipo diferentes):

```go
func cancelarSolicitacaoServicoExtra(c *gin.Context, sol *aggregates.SolicitacaoServicoExtra, motivo, canceladaPor, actorID string) {
	var err error
	switch sol.Status {
	case aggregates.StatusInscricaoAprovadaPendentePagamentoTaxa:
		if err = FinanceiroService.CancelarCobrancaTaxaInscricaoServicoAberta(c.Request.Context(), sol.GetID().String(), motivo, actorID, canceladaPor, c.ClientIP()); err != nil {
			financeError(c, err)
			return
		}
		err = sol.CancelarAntesDaVinculacao(motivo, canceladaPor)
	case aggregates.StatusInscricaoVinculada:
		err = sol.Cancelar(motivo, canceladaPor)
	default:
		utils.RespondWithConflictError(c, fmt.Sprintf("solicitação em estado '%s' não pode ser cancelada", sol.Status))
		return
	}
	if err != nil {
		utils.RespondWithConflictError(c, err.Error())
		return
	}
	audit := db.AuditContext{UserID: actorID, UserType: canceladaPor, IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(sol, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "inscrição cancelada com sucesso", "status": sol.Status})
}
```

- `PUT /academia/servicos-extras/inscricoes/:id/cancelar`: valida posse (`sol.CodigoAcademia == academiaDTO.CodigoAcademia`), chama `cancelarSolicitacaoServicoExtra(c, sol, req.Motivo, "academia", academiaDTO.ID.String())`.
- `PUT /estudante/servicos-extras/minhas-inscricoes/:id/cancelar`: valida posse (`sol.CodigoEstudante == est.CodigoEstudante`), chama `cancelarSolicitacaoServicoExtra(c, sol, req.Motivo, "estudante", userID.String())`.

`motivo` é opcional em ambas (diferente da reprovação — cancelar uma inscrição já ativa não exige justificativa formal, mas registre o que vier, mesmo vazio).

### 7.5 `IniciarPagamentoTaxaInscricaoServicoExtra` (handler HTTP)

Rota: `POST /financeiro/servicos-extras/taxa-inscricao/pagamento`, grupo `protected` (autenticado, qualquer tipo — mesmo grupo de `POST /financeiro/mensalidades/pagamento`, `cmd/server/main.go` linha ~367), **não** o grupo `financeiro` restrito a academia/admin (linha 372) — quem paga a taxa é o estudante autenticado.

Mirror exato de `IniciarPagamentoMensalidades` (`internal/handlers/mensalidade_handlers.go:297-320`) quanto à forma de obter e forçar a identidade do estudante a partir do token (nunca confiar em um `codigo_estudante`/`solicitacao_id` que "prove" ser de outra pessoa — aqui a defesa é diferente: em vez de forçar um `codigo_estudante` no input, valide que a solicitação carregada por `id` **pertence** ao estudante autenticado):

```go
func IniciarPagamentoTaxaInscricaoServicoExtra(c *gin.Context) {
	var in finance.TaxaInscricaoServicoExtraPagamentoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok || typ != "estudante" {
		utils.RespondWithForbiddenError(c, "somente o estudante pode iniciar pagamento de taxa de inscrição de serviço extra")
		return
	}
	var solicitacaoIDStr string
	if err := c.ShouldBindQuery(&struct {
		ID *string `form:"solicitacao_id"`
	}{&solicitacaoIDStr}); err != nil {
		// ajuste conforme a convenção de binding já usada no repositório: solicitacao_id pode
		// vir como query param ou no body — escolha UMA forma e documente-a na Documentação da API.
	}
	solicitacaoID, err := uuid.Parse(c.Query("solicitacao_id"))
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("solicitacao_id inválido"))
		return
	}
	sol, ok := loadSolicitacaoServicoExtra(c, solicitacaoID)
	if !ok {
		return
	}
	var codigoEstudante string
	if err := getDBClient(c).DB().QueryRowContext(c.Request.Context(), `SELECT codigo_estudante FROM projection_estudantes WHERE id=$1`, id).Scan(&codigoEstudante); err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	if sol.CodigoEstudante != codigoEstudante {
		utils.RespondWithForbiddenError(c, "esta solicitação não pertence ao estudante autenticado")
		return
	}
	in.SolicitacaoID = solicitacaoID.String()
	out, err := FinanceiroService.IniciarPagamentoTaxaInscricaoServicoExtra(c.Request.Context(), in, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	if strings.EqualFold(out.Charge.Status, "success") {
		_ = efetivarVinculoServicoExtraPago(c, in.SolicitacaoID)
	}
	c.JSON(http.StatusCreated, out)
}
```

**Simplifique o trecho de exemplo acima** (deixei um rascunho de binding de `solicitacao_id` intencionalmente hesitante entre query/body para você decidir com base na convenção mais comum já usada em endpoints semelhantes deste repositório — confirme lendo 2-3 handlers de pagamento existentes e escolha UMA forma consistente; documente a escolha na atualização da `Documentação da API.md`, seção 9).

### 7.6 `efetivarVinculoServicoExtraPago`

```go
// efetivarVinculoServicoExtraPago é seguro em reentrega de webhook: a
// transição terminal (VincularAposPagamento) é verificada pelo próprio
// aggregate antes de aplicar qualquer coisa — chamar duas vezes para a
// mesma solicitação já vinculada retorna erro, que este helper ignora
// deliberadamente (idempotência), mirror do comportamento de
// efetivarVinculoMatriculaPaga para status já terminal.
func efetivarVinculoServicoExtraPago(c *gin.Context, solicitacaoIDStr string) error {
	solicitacaoID, err := uuid.Parse(solicitacaoIDStr)
	if err != nil {
		return err
	}
	agg, err := getRepository(c).WithContext(c.Request.Context()).Load(solicitacaoID, "SolicitacaoServicoExtra")
	if err != nil {
		return err
	}
	sol, ok := agg.(*aggregates.SolicitacaoServicoExtra)
	if !ok {
		return fmt.Errorf("tipo de aggregate inesperado")
	}
	if sol.Status != aggregates.StatusInscricaoAprovadaPendentePagamentoTaxa {
		return nil // idempotente: já vinculada (ou outro estado terminal) — nada a fazer
	}
	if err := sol.VincularAposPagamento(); err != nil {
		return err
	}
	return getRepository(c).WithContext(c.Request.Context()).SaveWithAudit(sol, db.AuditContext{UserID: "appypay:servico_extra", UserType: "sistema", IP: c.ClientIP()})
}
```

### 7.7 Listagens

- `GET /academia/servicos-extras/solicitacoes` (grupo `academiaRead`): lista solicitações de todos os serviços da academia autenticada, com filtro opcional por `status` e `servico_extra_id` via query string.
- `GET /academia/servicos-extras/solicitacoes/:id` (grupo `academiaRead`): detalhe de uma solicitação.
- `GET /estudante/servicos-extras/minhas-inscricoes` (grupo `estudante`): lista as próprias solicitações/inscrições do estudante autenticado, com filtro opcional por `status`.
- `GET /academia/documentos/servicos-extras/solicitacoes/:id/documento/download` (grupo `academiaRead`): download do documento anexado, mirror de `DownloadDocumentoSolicitacaoEdicaoAcademia`.
- `GET /estudante/servicos-extras/minhas-inscricoes/:id/documento/download` (grupo `estudante`): o próprio estudante baixa o documento que enviou, mirror de `DownloadDocumentoSolicitacaoEdicaoEstudante`.

Todas seguem o padrão de posse + leitura da projeção já estabelecido nas seções anteriores.

## 8. Projeção `SolicitacaoServicoExtraProjection`

Crie `internal/projections/solicitacao_servico_extra_projection.go`, mirror de `internal/projections/cursos_projection.go` (estrutura) e de como `SolicitacaoMatriculaProjection` trata os múltiplos eventos de uma máquina de estados (`internal/projections/` — encontre e leia o arquivo desta projeção antes de escrever a nova; o nome do arquivo pode não ser óbvio a partir do nome da classe, procure por `AggregateType != "SolicitacaoMatricula"` para achá-lo). `Name() string { return "solicitacoes_servico_extra" }`.

Adicione o método `ExisteAtiva(servicoExtraID uuid.UUID, codigoEstudante string) (bool, error)` usado pelo handler da seção 7.2, item 6:
```sql
SELECT EXISTS(
    SELECT 1 FROM projection_solicitacoes_servico_extra
    WHERE servico_extra_id = $1 AND codigo_estudante = $2
    AND status IN ('pendente','aprovada_pendente_pagamento_taxa_inscricao','vinculada')
)
```

Registre em `cmd/server/main.go`:
```go
projManager.RegisterProjection("solicitacoes_servico_extra", projections.NewSolicitacaoServicoExtraProjection(dbClient))
```

## 9. O que já foi validado (Claude/orquestrador) e o que falta a você (Codex)

**Já testado num PostgreSQL 16 real** (com a Fase 1 já aplicada por cima das 117 migrations pré-existentes — inclui a migration 117 da Tarefa 81/82; reconfirmei a sequência completa do zero depois que ela foi mergeada):
- A migration 119 completa aplica sem erro.
- O índice único parcial `ux_sol_servico_extra_ativa` foi testado com 4 passos: (1) primeira solicitação `pendente` de um estudante para um serviço → aceita; (2) segunda solicitação `pendente` do **mesmo** estudante para o **mesmo** serviço → **rejeitada** pelo índice, exatamente como esperado; (3) reprovar a primeira solicitação (`UPDATE status='reprovada'`); (4) nova solicitação do mesmo estudante para o mesmo serviço, agora que a anterior não está mais "ativa" → **aceita**. Resultado: **positivo**, comportamento correto confirmado.

**Não pôde ser validado neste ambiente** (mesmas limitações de rede da Tarefa 09 — sem acesso a `proxy.golang.org` para compilar o módulo completo):
- Compilação (`go build ./...`, `go vet ./...`) de todos os arquivos novos/alterados desta fase, em especial as alterações em `internal/finance` (seção 6), que são as de maior risco de erro de sintaxe/import por serem edições cirúrgicas em arquivos grandes já existentes.
- O teste de integração ponta a ponta do pagamento (abaixo) — requer Postgres real, que este ambiente não tem via Codex; requer também simular uma resposta da AppyPay, o que os testes de integração existentes já fazem com um servidor HTTP de teste local (veja `internal/finance/appypay_integration_test.go` para o padrão de mock do endpoint AppyPay) — **isto não depende de rede externa real, só de Postgres**, então é executável no seu ambiente assim que houver Postgres disponível (Docker/serviço local). Onde não houver Postgres disponível nem para isto, documente explicitamente no PR e pule apenas o(s) teste(s) marcados `RUN_POSTGRES_INTEGRATION=1`.

### 9.1 Testes unitários obrigatórios (aggregate)

`internal/domain/aggregates/solicitacao_servico_extra_test.go`:
- `Criar` → `pendente`.
- `Aprovar` sem taxa → `vinculada` diretamente, `AprovadaPor` preenchido.
- `Aprovar` com taxa (valor/métodos válidos) → `aprovada_pendente_pagamento_taxa_inscricao`, valores congelados no aggregate.
- `Aprovar` com taxa mas `valorTaxa<=0` → erro.
- `Aprovar` chamado duas vezes → segunda chamada erro ("apenas pendentes").
- `Reprovar` sem motivo → erro; com motivo → `reprovada`, `MotivoReprovacao` preenchido.
- `Reprovar` sobre solicitação já `vinculada` → erro.
- `VincularAposPagamento` chamado sem antes passar por `aprovada_pendente_pagamento_taxa_inscricao` → erro.
- `VincularAposPagamento` a partir do estado correto → `vinculada`.
- `CancelarAntesDaVinculacao` a partir de `pendente` (nunca aprovada) → erro.
- `CancelarAntesDaVinculacao` a partir de `aprovada_pendente_pagamento_taxa_inscricao` → `cancelada_antes_da_vinculacao`.
- `Cancelar` a partir de `vinculada` → `cancelada`.
- `Cancelar` a partir de `pendente` → erro.
- `Cancelar`/`CancelarAntesDaVinculacao` com `canceladaPor` inválido (nem "academia" nem "estudante") → erro.

### 9.2 Teste de integração obrigatório (requer Postgres real)

Cobrir o fluxo completo, com um servidor HTTP de teste simulando a AppyPay (mesma técnica de `appypay_integration_test.go`):
1. Academia com credencial de teste + um `ServicoExtra` com `tem_taxa_inscricao=true`.
2. Estudante solicita (`POST .../solicitacao`) sem documento (serviço não exige) → `201`, status `pendente`.
3. Tentar solicitar de novo para o mesmo serviço antes de qualquer decisão → `409` (guard) — depois, tentar diretamente via segunda goroutine/conexão simulando corrida, se praticável, para validar o índice único parcial também pela via HTTP (opcional, mas valioso).
4. Academia aprova (`PUT .../aprovar`) → status `aprovada_pendente_pagamento_taxa_inscricao`, valor/métodos da taxa presentes na resposta.
5. Estudante inicia pagamento (`POST /financeiro/servicos-extras/taxa-inscricao/pagamento`) com o mock da AppyPay respondendo status diferente de sucesso (ex.: "Pending") → solicitação continua `aprovada_pendente_pagamento_taxa_inscricao`.
6. Simular confirmação via **webhook** (não via resposta síncrona, para testar o caminho menos óbvio) → `POST /webhooks/appypay/{gpo|ref}` com o payload de sucesso e a assinatura esperada → consultar a solicitação e confirmar `status=vinculada`.
7. Repetir a entrega do mesmo webhook (reentrega) → sem erro, sem duplicar nenhum efeito (idempotência de `efetivarVinculoServicoExtraPago`).
8. Estudante tenta cancelar a própria inscrição já vinculada (`PUT .../minhas-inscricoes/:id/cancelar`) → `200`, status `cancelada`.
9. Repetir o teste completo mas com um serviço **sem** taxa de inscrição: aprovar deve vincular imediatamente, sem qualquer chamada ao financeiro.
10. Consultar `GET /financeiro/cobrancas/estudante/:codigo` com filtro `origem=servico_extra` (seção 6.3) e confirmar que só a cobrança da taxa aparece, e que ela **não** aparece também sob `origem=mensalidade`.

## 10. Atualização da documentação de API

Adicione à seção criada na Tarefa 09 (`## 20. Serviços Extras`) as subseções para todos os endpoints desta fase, no mesmo formato (`### 20.N`), incluindo o novo endpoint de pagamento em `## 19. Financeiro / AppyPay` (subseção nova, ex. `### 19.X Pagamento de taxa de inscrição de serviço extra`), documentando explicitamente a nova categoria `origem=servico_extra` nos endpoints de listagem de cobranças já existentes (`ListCobrancas`/`ListCobrancasEstudante`).

## 11. Checklist de aceite da Fase 2

- [ ] Migration 119 aplicada sem erro; índice único parcial testado (já validado — ver seção 9).
- [ ] `SolicitacaoServicoExtra` registrado na whitelist (aggregate + 6 eventos) e na factory.
- [ ] Aggregate com os 6 estados e todas as transições da seção 5; testes unitários passando.
- [ ] `internal/finance`: `ChargeRequest`/`QRCodeRequest`/`gerarCobrancaInput` com `CodigoInscricaoServico`; payload persistido inclui a chave; `CobrancaResumo`/`scanCobrancaResumo`/`origensClause` reconhecem `origem=servico_extra` com precedência correta sobre `mensalidade`.
- [ ] `internal/finance/servico_extra.go` com as 3 funções novas.
- [ ] Os **três** pontos de confirmação de pagamento (síncrono, consulta, webhook) chamam `efetivarVinculoServicoExtraPago`.
- [ ] Handlers de solicitar/aprovar/reprovar/cancelar (academia e estudante)/listar/download implementados com checagem de posse em toda mutação.
- [ ] Upload de documento opcional/obrigatório conforme `ServicoExtra.DocumentoObrigatorio`, com limpeza (`provider.Delete`) em qualquer falha após o upload.
- [ ] Guard + índice único parcial ambos testados (unit + integração) contra solicitação duplicada ativa.
- [ ] `SolicitacaoServicoExtraProjection` criada e **registrada em `main.go`**.
- [ ] Teste de integração ponta a ponta cobrindo aprovação sem taxa, aprovação com taxa + pagamento via webhook + idempotência, e cancelamento.
- [ ] `Documentação da API.md` atualizada, incluindo a nova categoria de origem nos endpoints financeiros já existentes.
- [ ] `go build ./...` e `go vet ./...` limpos no seu ambiente.
- [ ] Resultado reportado ao final: o que passou, o que falhou, o que não pôde ser testado no seu ambiente e por quê.
