---
criado: 04-09-2026
origem: Fredy + Claude (orquestração)
status: pronto para execução — depende das Tarefas 09 e 10 estarem concluídas
tipo: backend (spuri-backend)
depende_de: Tarefa 09 (Fase 1), Tarefa 10 (Fase 2)
---

# Tarefa 11 — Módulo de Serviços Extras — Fase 3: Cobrança recorrente mensal e preço único

### Documento de execução para o Codex (orquestrado e pré-testado pelo Claude)

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Mesma situação das Tarefas 09 e 10: sem `apt`/Docker/`psql` aqui, mas isto já foi validado por mim com PostgreSQL 16 real:

- A migration 120 desta fase (`ALTER TABLE` + tabela nova + índices, seção 4.1) aplica sem erro sobre as 117 migrations existentes + as migrations 118 e 119 das Tarefas 09/10.
- A lógica de precedência de estado (`anulada`/`reativada`/`paga` → `pendente`/`anulada`/`pago`) foi testada manualmente inserindo as sequências de eventos reais na tabela nova e conferindo, linha a linha, que a leitura na ordem `ORDER BY ocorrido_em, id` produz o resultado esperado pela função `precedenciaEstado` já existente (que esta fase reaproveita sem alterar) — ver seção 7.
- O índice único `ux_servico_extra_obrigacoes_event_id` (proteção de idempotência contra reprocessamento) foi testado: inserir o mesmo `event_id` duas vezes com `ON CONFLICT (event_id) DO NOTHING` resulta em exatamente 1 linha, não 2.

**Esta é a fase com maior risco de erro de compilação de todo o módulo** — edita `internal/finance/appypay.go`, `internal/finance/cobranca_geracao.go` E `internal/projections/financeiro_projection.go` (três arquivos grandes já existentes, dois deles já editados pela Tarefa 10), além de **substituir** uma função inteira criada na Tarefa 10 (`CodigoInscricaoServicoExtraDaCobranca` → `DadosServicoExtraDaCobranca`, seção 5.1) e atualizar os três pontos de confirmação de pagamento que a chamam. Não pude compilar (`go build ./...`) nem rodar `go test` neste ambiente pela mesma limitação de rede já explicada nas Tarefas 09/10 (`golang.org/x/*`, `google.golang.org/protobuf`, `go.opentelemetry.io/auto/sdk` bloqueados pelo proxy do sandbox antes mesmo de um redirecionamento `git insteadOf` conseguir agir). **Compile e rode os testes com atenção redobrada nesta fase especificamente** — é onde um erro de sintaxe tem mais chance de escapar por serem edições cirúrgicas somadas de duas tarefas.

Confirmei — comparando os arquivos antes e depois das Tarefas 81-82 — que nenhum dos arquivos do módulo financeiro foi alterado por elas; as referências de linha para `internal/finance/*.go` permanecem exatas. Para `cmd/server/main.go`/`internal/db/safe_queries.go`, valem as mesmas ressalvas de aproximação já feitas nas Tarefas 09/10.

Se for rodar a suíte completa: `FINANCE_ENCRYPTION_KEY` precisa estar definida (qualquer string, para teste) e use `go test -p 1 ./...` — nota herdada da Tarefa 81.

## 1. Prompt recomendado para executar esta tarefa

> Confirme que as Tarefas 09 e 10 já estão implementadas e mergeadas antes de começar. Aplique exatamente o que está descrito neste documento, na ordem das seções — preste atenção especial à seção 5.1, que substitui uma função criada na Tarefa 10 e exige atualizar os três pontos de confirmação de pagamento que a chamam. Não replaneje nada do que já está decidido. Ao final, rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...`, corrija qualquer erro, e preencha o checklist da seção 10.

> **Pré-requisito obrigatório:** Tarefas 09 e 10 implementadas, mergeadas e com testes passando. Este documento assume que `ServicoExtra`, `SolicitacaoServicoExtra` e toda a integração AppyPay da Fase 2 (seção 6 daquele documento) já existem exatamente como especificado.
>
> Leia `docs/Tarefas feitas/tarefa-codex-backend-spuri-backend.md` e este documento por completo antes de escrever qualquer código.

## 2. Objetivo desta fase

Cobrar, ao longo do tempo, o **preço do serviço em si** (`ServicoExtra.Preco`/`TipoCobranca`, campo independente da taxa de inscrição já resolvida na Fase 2 — ver decisão de design 4 da Tarefa 09):

- **`tipo_cobranca = "mensal"`**: uma cobrança recorrente, mês a mês, enquanto a inscrição estiver `vinculada`.
- **`tipo_cobranca = "unico"`**: uma única cobrança, devida uma vez, a qualquer momento depois de vinculado (o estudante escolhe quando pagar, não é cobrado automaticamente).

**Esta fase não mexe na máquina de estados de `SolicitacaoServicoExtra`.** A vinculação/cancelamento já são resolvidos pelas Fases 1-2; esta fase só adiciona cobranças **sobre** uma inscrição já `vinculada`, de forma inteiramente aditiva. Nenhum evento novo é adicionado ao aggregate `SolicitacaoServicoExtra`; os eventos novos desta fase pertencem ao aggregate `Financeiro` já existente (mesmo aggregate type usado por toda a mensalidade/matrícula/credenciais — ver `internal/domain/aggregates/financeiro.go`).

## 3. Decisões de design já tomadas

1. **Modelo de pendência por evento imutável, não por linha física pré-criada por mês** — mirror exato do mecanismo já usado para mensalidade de matrícula regular. Em vez de inserir uma linha "devido" para cada mês de cada inscrição (o que explodiria em volume e exigiria um job de cron para manter em dia), o estado de cada mês é **derivado** a partir do histórico de eventos imutáveis (`anulada`, `reativada`, `paga`) já registrados para aquele mês, usando a mesmíssima função de precedência já existente e testada:
   ```go
   // internal/finance/mensalidade.go:721-739 — reaproveitar sem alterar
   func precedenciaEstado(eventos []string) string {
       state := EstadoPendente
       for _, typ := range eventos {
           switch typ {
           case "anulada":
               if state != EstadoPago { state = EstadoAnulado }
           case "reativada":
               if state == EstadoAnulado { state = EstadoPendente }
           case "paga":
               state = EstadoPago
           }
       }
       return state
   }
   ```
   **Não reescreva esta função — chame-a diretamente** (ela já está em `internal/finance`, pacote onde você vai adicionar o código desta fase). Isto já foi testado manualmente (seção 7) com as três sequências relevantes (anulada→reativada→paga, paga→anulada, e o caso trivial sem eventos) e o comportamento de precedência é exatamente o desejado: um pagamento real nunca é desfeito por uma anulação posterior.

2. **Sem "ano letivo" nem calendário escolar.** Diferente da mensalidade de matrícula (que segue o calendário do ano letivo, `mesesAnoLetivo`, `mes_inicio_cobranca` configurado pela academia), um serviço extra é um vínculo individual — cada estudante entra num mês diferente. Os meses devidos vão de **`vinculada_em`** (novo campo — item 3 abaixo) até o mês atual (se `status="vinculada"`) ou até o mês em que a inscrição foi cancelada, **inclusive** (se `status="cancelada"` — cancelar a meio do mês ainda deve aquele mês inteiro; sem pro-rata, mesma filosofia de "sem reprecificação retroativa" já estabelecida na Fase 1).

3. **Nova coluna `vinculada_em` em `projection_solicitacoes_servico_extra`.** O campo `updated_at` do aggregate já é sobrescrito a cada transição de estado subsequente (ex.: ao cancelar), por isso não serve como o início do período de cobrança. Adicione:
   - No aggregate `SolicitacaoServicoExtra` (`internal/domain/aggregates/solicitacao_servico_extra.go`, editado na Fase 2): um novo campo `VinculadaEm time.Time`, setado **apenas** dentro de `applyVinculada` (nunca sobrescrito depois):
     ```go
     func (s *SolicitacaoServicoExtra) applyVinculada(event DomainEvent) error {
         // ... (código já existente da Fase 2, sem alteração) ...
         s.Status = StatusInscricaoVinculada
         if p.AprovadaPor != uuid.Nil {
             s.AprovadaPor = p.AprovadaPor
         }
         s.UpdatedAt = p.UpdatedAt
         if s.VinculadaEm.IsZero() {
             s.VinculadaEm = p.UpdatedAt
         }
         return nil
     }
     ```
     (`p.UpdatedAt` já existe no payload de `SolicitacaoServicoExtraVinculadaEvent` — nenhuma mudança na struct do evento é necessária, só a leitura de mais um campo dela no `apply`.)
   - Nova migration `120_servico_extra_obrigacoes.sql` (seção 4) adicionando a coluna à projeção e atualizando `SolicitacaoServicoExtraProjection` (Fase 2) para persistir `vinculada_em` no `handleSolicitacaoServicoExtraVinculada` (grave-o **apenas quando a coluna ainda está `NULL`**, com `COALESCE(vinculada_em, $X)` ou lógica equivalente no `UPDATE`, para o caso de o mesmo evento ser reprocessado num rebuild de projeção — idempotência).

4. **Cobrança gerada sob demanda pelo estudante, nunca por job automático** — mesmo modelo *pull* já usado por mensalidade (`IniciarPagamentoMensalidades`: o sistema não empurra cobranças, o estudante pede para pagar um mês específico quando quiser). Isto evita depender de um scheduler/cron (que este repositório não usa para isto) e mantém o comportamento consistente com o resto do módulo financeiro.

5. **Reaproveitamento total do transporte de cobrança já existente** (`gerarCobranca`, `financeiro_cobrancas`, `CancelCharge`, os três pontos de confirmação). A única coisa nova no transporte é **discriminar**, dentro do metadado já adicionado na Fase 2 (`codigo_inscricao_servico`), **qual tipo de lançamento** aquela cobrança específica representa — porque agora uma mesma `SolicitacaoServicoExtra` pode ter cobranças de três naturezas diferentes ao longo do tempo: a taxa de inscrição (Fase 2), uma mensalidade de um mês específico, ou o preço único. Ver seção 5.

6. **Eventos desta fase pertencem ao aggregate `Financeiro`, não a um novo aggregate.** Mirror exato de como `ObrigacaoMensalidadeAnulada`/`ObrigacaoMensalidadeReativada`/`MensalidadePaga` já funcionam (`internal/domain/aggregates/financeiro.go`, `internal/finance/mensalidade.go:549-559` — `recordMensalidade`). Novos eventos: `ObrigacaoServicoExtraAnulada`, `ObrigacaoServicoExtraReativada`, `ServicoExtraLancamentoPago`. Vão para a whitelist de `validEventTypes` (não para `validAggregateTypes` — `"Financeiro"` já está lá).

## 4. Modelo de dados

### 4.1 Migration — **já testada** manualmente (seção 7)

Crie `migrations/120_servico_extra_obrigacoes.sql`:

```sql
ALTER TABLE projection_solicitacoes_servico_extra ADD COLUMN IF NOT EXISTS vinculada_em TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS financeiro_servico_extra_obrigacoes_eventos (
    id BIGSERIAL PRIMARY KEY,
    event_id UUID NOT NULL,
    solicitacao_id UUID NOT NULL,
    tipo_lancamento VARCHAR(20) NOT NULL CHECK (tipo_lancamento IN ('mensalidade','preco_unico')),
    ano INTEGER,
    mes INTEGER CHECK (mes IS NULL OR (mes >= 1 AND mes <= 12)),
    tipo VARCHAR(10) NOT NULL CHECK (tipo IN ('anulada','reativada','paga')),
    ocorrido_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_servico_extra_obrigacoes_busca ON financeiro_servico_extra_obrigacoes_eventos(solicitacao_id, tipo_lancamento, ano, mes);
CREATE UNIQUE INDEX IF NOT EXISTS ux_servico_extra_obrigacoes_event_id ON financeiro_servico_extra_obrigacoes_eventos(event_id);
```

Para `tipo_lancamento = 'preco_unico'`, `ano`/`mes` ficam `NULL` — é um lançamento único, sem mês associado; a constraint de `mes` (`mes IS NULL OR mes BETWEEN 1 AND 12`) já aceita isso sem necessidade de sentinela artificial.

### 4.2 Whitelist do ledger

Em `internal/db/safe_queries.go`, `validEventTypes`, adicione:
```go
"ObrigacaoServicoExtraAnulada":   true,
"ObrigacaoServicoExtraReativada": true,
"ServicoExtraLancamentoPago":     true,
```
**Não** adicione novo `aggregate_type` — estes eventos são gravados com `aggregate_type="Financeiro"`, que já está na whitelist desde sempre.

### 4.3 `internal/domain/aggregates/financeiro.go`

Adicione as três novas constantes de nome de evento junto às já existentes (linhas 17-27):
```go
ObrigacaoServicoExtraAnulada   = "ObrigacaoServicoExtraAnulada"
ObrigacaoServicoExtraReativada = "ObrigacaoServicoExtraReativada"
ServicoExtraLancamentoPago     = "ServicoExtraLancamentoPago"
```

## 5. Metadado adicional de cobrança: discriminar o tipo de lançamento

Na Fase 2 (seção 6.1), `ChargeRequest`/`QRCodeRequest`/`gerarCobrancaInput` ganharam `CodigoInscricaoServico`. Esta fase adiciona, nos três mesmos locais, mais três campos:

```go
TipoLancamentoServicoExtra string `json:"tipo_lancamento_servico_extra,omitempty"` // "taxa_inscricao" | "mensalidade" | "preco_unico"
MesReferencia              *int   `json:"mes_referencia,omitempty"`                // apenas quando "mensalidade"
AnoReferencia              *int   `json:"ano_referencia,omitempty"`                // apenas quando "mensalidade"
```

**Retrocompatibilidade com a Fase 2:** ao criar a cobrança da taxa de inscrição (`IniciarPagamentoTaxaInscricaoServicoExtra`, já escrita na Fase 2), preencha agora também `TipoLancamentoServicoExtra: "taxa_inscricao"` na chamada a `gerarCobranca` — isto não quebra nada que já existe, só adiciona um metadado antes ausente.

Replique nos dois literais `map[string]any{...}` do payload persistido (`appypay.go`, mesmas duas linhas alteradas na Fase 2, seção 6.2) as três novas chaves: `"tipo_lancamento_servico_extra"`, `"mes_referencia"`, `"ano_referencia"`.

### 5.1 Substituir `CodigoInscricaoServicoExtraDaCobranca` por uma versão que também devolve o discriminador

A Fase 2 criou `FinanceiroService.CodigoInscricaoServicoExtraDaCobranca(ctx, identifier) (string, error)`, usada nos três pontos de confirmação. Troque-a por:

```go
// DadosServicoExtraDaCobranca substitui CodigoInscricaoServicoExtraDaCobranca
// (Fase 2): além do código da solicitação, agora também devolve QUAL
// lançamento aquela cobrança representa, para o chamador decidir se deve
// vincular a inscrição (taxa_inscricao) ou apenas marcar um lançamento como
// pago (mensalidade/preco_unico).
func (s *Service) DadosServicoExtraDaCobranca(ctx context.Context, identifier string) (codigoInscricao, tipoLancamento string, mes, ano int, err error) {
	row, err := s.loadCharge(ctx, identifier)
	if err != nil {
		return "", "", 0, 0, err
	}
	codigoInscricao, _ = row.Payload["codigo_inscricao_servico"].(string)
	tipoLancamento, _ = row.Payload["tipo_lancamento_servico_extra"].(string)
	if m, ok := row.Payload["mes_referencia"].(float64); ok {
		mes = int(m)
	}
	if a, ok := row.Payload["ano_referencia"].(float64); ok {
		ano = int(a)
	}
	return strings.TrimSpace(codigoInscricao), tipoLancamento, mes, ano, nil
}
```

**Atualize os três pontos de confirmação já escritos na Fase 2** (seção 6.6 daquele documento: resposta síncrona do handler, `ConsultarCobrancaAppyPay`, `ReceberWebhookAppyPay`) para usar esta nova função em vez da antiga, com o despacho:

```go
codigoInscricao, tipoLancamento, mes, ano, err := FinanceiroService.DadosServicoExtraDaCobranca(ctx, identifier)
if err == nil && codigoInscricao != "" {
	switch tipoLancamento {
	case "taxa_inscricao":
		_ = efetivarVinculoServicoExtraPago(c, codigoInscricao) // já existente, Fase 2 — sem alteração
	case "mensalidade", "preco_unico":
		_ = FinanceiroService.ConfirmarLancamentoServicoExtraPago(ctx, codigoInscricao, tipoLancamento, ano, mes, actorID, actorType, ip) // novo, seção 6.3
	}
}
```

Não deixe as duas versões (antiga e nova função) coexistindo — remova `CodigoInscricaoServicoExtraDaCobranca` por completo, já que `DadosServicoExtraDaCobranca` a substitui integralmente.

## 6. `internal/finance/servico_extra.go` (arquivo criado na Fase 2) — funções novas

### 6.1 `recordServicoExtraObrigacao` — mirror de `recordMensalidade`

```go
func (s *Service) recordServicoExtraObrigacao(ctx context.Context, solicitacaoID, event string, payload map[string]any, userID, userType, ip string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("autor do evento financeiro é obrigatório")
	}
	agg := aggregates.NewFinanceiroWithID(servicoExtraObrigacaoAggregateID(solicitacaoID))
	agg.Registrar(event, payload)
	if err := s.repository.WithContext(ctx).SaveWithAudit(agg, db.AuditContext{UserID: userID, UserType: userType, IP: ip}); err != nil {
		return err
	}
	return s.projection.ApplyLatestForAggregate(agg.ID)
}

func servicoExtraObrigacaoAggregateID(solicitacaoID string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("spuri:servico_extra_obrigacao:"+strings.ToLower(strings.TrimSpace(solicitacaoID))))
}
```

Isto grava no `spuri_ledger` (auditável, imutável) **e** projeta sincronamente (`ApplyLatestForAggregate`) para a tabela nova ficar consistente já no fim da chamada — sem depender do `projManager` assíncrono, exatamente como `mensalidade.go` já faz.

### 6.2 Adicione o `case` na projeção financeira

Em `internal/projections/financeiro_projection.go`, no `switch` de `Handle` (perto da linha 169, mesma vizinhança do `case "ObrigacaoMensalidadeAnulada", ...`), adicione:

```go
case "ObrigacaoServicoExtraAnulada", "ObrigacaoServicoExtraReativada", "ServicoExtraLancamentoPago":
	var in struct {
		SolicitacaoID  string `json:"solicitacao_id"`
		TipoLancamento string `json:"tipo_lancamento"`
		Ano            *int   `json:"ano"`
		Mes            *int   `json:"mes"`
		Motivo         string `json:"motivo"`
	}
	if err := json.Unmarshal(e.Payload, &in); err != nil {
		return err
	}
	if in.SolicitacaoID == "" || (in.TipoLancamento != "mensalidade" && in.TipoLancamento != "preco_unico") {
		return fmt.Errorf("evento de obrigação de serviço extra inválido")
	}
	tipo := map[string]string{"ObrigacaoServicoExtraAnulada": "anulada", "ObrigacaoServicoExtraReativada": "reativada", "ServicoExtraLancamentoPago": "paga"}[e.EventType]
	_, err := p.client.DB().Exec(`INSERT INTO financeiro_servico_extra_obrigacoes_eventos (event_id,solicitacao_id,tipo_lancamento,ano,mes,tipo,ocorrido_em) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (event_id) DO NOTHING`, e.EventID, in.SolicitacaoID, in.TipoLancamento, in.Ano, in.Mes, tipo, e.OccurredAt)
	return err
```

(A migration da seção 4.1 já inclui `ux_servico_extra_obrigacoes_event_id`, um índice único em `event_id` — por isso o `INSERT` acima deve usar `ON CONFLICT (event_id) DO NOTHING`, e não um `ON CONFLICT DO NOTHING` genérico. Isto é a mesma proteção de idempotência contra reprocessamento duplicado já usada na tabela irmã de mensalidade.)

### 6.3 Estado de uma obrigação, pendências, pagamento e anulação/reativação

```go
// estadoObrigacaoServicoExtra é o mirror direto de estadoObrigacao
// (mensalidade.go:697-716), trocando a chave de busca (estudante+academia+
// ano_letivo+mes) por (solicitacao_id+tipo_lancamento+ano+mes) e reutilizando
// a MESMA função precedenciaEstado, sem alterá-la.
func (s *Service) estadoObrigacaoServicoExtra(ctx context.Context, solicitacaoID, tipoLancamento string, ano, mes int) (string, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT tipo FROM financeiro_servico_extra_obrigacoes_eventos WHERE solicitacao_id=$1 AND tipo_lancamento=$2 AND ano IS NOT DISTINCT FROM NULLIF($3,0) AND mes IS NOT DISTINCT FROM NULLIF($4,0) ORDER BY ocorrido_em, id`, solicitacaoID, tipoLancamento, ano, mes)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var eventos []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			return "", err
		}
		eventos = append(eventos, typ)
	}
	return precedenciaEstado(eventos), rows.Err()
}

type ServicoExtraPendenciaView struct {
	TipoLancamento string  `json:"tipo_lancamento"`
	Ano            int     `json:"ano,omitempty"`
	Mes            int     `json:"mes,omitempty"`
	Estado         string  `json:"estado"` // "pendente" | "anulada" | "pago"
	Valor          float64 `json:"valor"`
}

// PendenciasServicoExtra lista, para uma inscrição vinculada, todos os
// lançamentos devidos até agora. Para tipo_cobranca="mensal", enumera cada
// mês de VinculadaEm até o mês atual (ou até o mês do cancelamento, se
// cancelada) — sem pro-rata, sem calendário de ano letivo (decisão de
// design 2). Para tipo_cobranca="unico", devolve um único item.
func (s *Service) PendenciasServicoExtra(ctx context.Context, solicitacaoID string, tipoCobranca string, preco float64, vinculadaEm time.Time, fimPeriodo time.Time) ([]ServicoExtraPendenciaView, error) {
	var out []ServicoExtraPendenciaView
	if tipoCobranca == "unico" {
		estado, err := s.estadoObrigacaoServicoExtra(ctx, solicitacaoID, "preco_unico", 0, 0)
		if err != nil {
			return nil, err
		}
		return []ServicoExtraPendenciaView{{TipoLancamento: "preco_unico", Estado: estado, Valor: preco}}, nil
	}
	cursor := time.Date(vinculadaEm.Year(), vinculadaEm.Month(), 1, 0, 0, 0, 0, time.UTC)
	limite := time.Date(fimPeriodo.Year(), fimPeriodo.Month(), 1, 0, 0, 0, 0, time.UTC)
	for !cursor.After(limite) {
		estado, err := s.estadoObrigacaoServicoExtra(ctx, solicitacaoID, "mensalidade", cursor.Year(), int(cursor.Month()))
		if err != nil {
			return nil, err
		}
		out = append(out, ServicoExtraPendenciaView{TipoLancamento: "mensalidade", Ano: cursor.Year(), Mes: int(cursor.Month()), Estado: estado, Valor: preco})
		cursor = cursor.AddDate(0, 1, 0)
	}
	return out, nil
}
```

**Chame `PendenciasServicoExtra` sempre a partir do HANDLER**, passando `fimPeriodo = time.Now()` quando `sol.Status == "vinculada"`, ou `fimPeriodo = sol.UpdatedAt` quando `sol.Status == "cancelada"` (é exatamente o `UpdatedAt` da transição `Cancelar`, que não é sobrescrito depois — diferente de `VinculadaEm`, este campo já existe desde a Fase 2 e já para de mudar assim que o status se torna terminal). Para qualquer outro status (`pendente`, `aprovada_pendente_pagamento_taxa_inscricao`, `reprovada`, `cancelada_antes_da_vinculacao`), não há pendências de preço do serviço — devolva lista vazia sem chamar esta função.

```go
type ServicoExtraObrigacaoPagamentoInput struct {
	SolicitacaoID   string `json:"-"`
	TipoLancamento  string `json:"tipo_lancamento"` // "mensalidade" | "preco_unico"
	Ano             int    `json:"ano,omitempty"`
	Mes             int    `json:"mes,omitempty"`
	MetodoPagamento string `json:"metodo_pagamento"`
	Telefone        string `json:"telefone,omitempty"`
}

// IniciarPagamentoServicoExtraObrigacao inicia a cobrança de um lançamento
// específico (um mês ou o preço único). Mirror de
// IniciarPagamentoTaxaInscricaoServicoExtra (Fase 2) / IniciarPagamentoMatricula,
// mesma forma de validar estado antes de gerar a cobrança.
func (s *Service) IniciarPagamentoServicoExtraObrigacao(ctx context.Context, in ServicoExtraObrigacaoPagamentoInput, codigoAcademia string, preco float64, metodosPagamento []string, ip string) (QRCodeResult, error) {
	in.MetodoPagamento = strings.ToUpper(strings.TrimSpace(in.MetodoPagamento))
	if in.TipoLancamento != "mensalidade" && in.TipoLancamento != "preco_unico" {
		return QRCodeResult{}, errors.New("tipo_lancamento deve ser 'mensalidade' ou 'preco_unico'")
	}
	if in.TipoLancamento == "mensalidade" && (in.Mes < 1 || in.Mes > 12 || in.Ano < 2000) {
		return QRCodeResult{}, errors.New("ano e mes válidos são obrigatórios para mensalidade")
	}
	if !contains(metodosPagamento, in.MetodoPagamento) {
		return QRCodeResult{}, errors.New("método de pagamento não está habilitado para este serviço")
	}
	estado, err := s.estadoObrigacaoServicoExtra(ctx, in.SolicitacaoID, in.TipoLancamento, in.Ano, in.Mes)
	if err != nil {
		return QRCodeResult{}, err
	}
	if estado == EstadoPago {
		return QRCodeResult{}, errors.New("este lançamento já está pago")
	}
	if estado == EstadoAnulado {
		return QRCodeResult{}, errors.New("este lançamento foi anulado pela academia")
	}
	open, err := s.servicoExtraObrigacaoTemCobrancaAberta(ctx, in.SolicitacaoID, in.TipoLancamento, in.Ano, in.Mes)
	if err != nil {
		return QRCodeResult{}, err
	}
	if open {
		return QRCodeResult{}, errors.New("já existe cobrança em aberto para este lançamento")
	}
	var mesRef, anoRef *int
	desc := "Serviço extra " + codigoAcademia
	if in.TipoLancamento == "mensalidade" {
		mesRef, anoRef = &in.Mes, &in.Ano
		desc = fmt.Sprintf("Serviço extra %s - %02d/%d", codigoAcademia, in.Mes, in.Ano)
	}
	return s.gerarCobranca(ctx, gerarCobrancaInput{
		CodigoAcademia:             codigoAcademia,
		MetodoPagamento:            in.MetodoPagamento,
		Amount:                     preco,
		Description:                desc,
		MerchantTransactionID:      merchantID(),
		Telefone:                   in.Telefone,
		CodigoInscricaoServico:     in.SolicitacaoID,
		TipoLancamentoServicoExtra: in.TipoLancamento,
		MesReferencia:              mesRef,
		AnoReferencia:              anoRef,
	}, "solicitacao_servico_extra:"+in.SolicitacaoID, "solicitante", ip)
}

func (s *Service) servicoExtraObrigacaoTemCobrancaAberta(ctx context.Context, solicitacaoID, tipoLancamento string, ano, mes int) (bool, error) {
	var ok bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_cobrancas WHERE payload->>'codigo_inscricao_servico'=$1 AND payload->>'tipo_lancamento_servico_extra'=$2 AND COALESCE((payload->>'ano_referencia')::int,0)=$3 AND COALESCE((payload->>'mes_referencia')::int,0)=$4 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`))`, solicitacaoID, tipoLancamento, ano, mes).Scan(&ok)
	return ok, err
}

// ConfirmarLancamentoServicoExtraPago grava o evento "paga" para o
// lançamento identificado pela cobrança confirmada. Chamado pelos três
// pontos de confirmação (seção 5.1). Idempotente por natureza: gravar
// "paga" duas vezes não muda o resultado de precedenciaEstado.
func (s *Service) ConfirmarLancamentoServicoExtraPago(ctx context.Context, solicitacaoID, tipoLancamento string, ano, mes int, actorID, actorType, ip string) error {
	payload := map[string]any{"solicitacao_id": solicitacaoID, "tipo_lancamento": tipoLancamento}
	if tipoLancamento == "mensalidade" {
		payload["ano"] = ano
		payload["mes"] = mes
	}
	return s.recordServicoExtraObrigacao(ctx, solicitacaoID, aggregates.ServicoExtraLancamentoPago, payload, actorID, actorType, ip)
}

// AnularObrigacaoServicoExtra / ReativarObrigacaoServicoExtra: ações da
// academia, mirror de AnularObrigacoesMensalidade/ReativarObrigacoesMensalidade
// (mensalidade.go:244-250), mas para um único lançamento por vez (a
// academia decide inscrição por inscrição, não em lote por ano/curso, já
// que não existe aqui o conceito de turma inteira sujeita à mesma regra).
func (s *Service) AnularObrigacaoServicoExtra(ctx context.Context, solicitacaoID, tipoLancamento string, ano, mes int, motivo, actorID, actorType, ip string) error {
	estado, err := s.estadoObrigacaoServicoExtra(ctx, solicitacaoID, tipoLancamento, ano, mes)
	if err != nil {
		return err
	}
	if estado == EstadoPago {
		return errors.New("não é possível anular um lançamento já pago")
	}
	payload := map[string]any{"solicitacao_id": solicitacaoID, "tipo_lancamento": tipoLancamento, "motivo": strings.TrimSpace(motivo)}
	if tipoLancamento == "mensalidade" {
		payload["ano"], payload["mes"] = ano, mes
	}
	if err := s.recordServicoExtraObrigacao(ctx, solicitacaoID, aggregates.ObrigacaoServicoExtraAnulada, payload, actorID, actorType, ip); err != nil {
		return err
	}
	return s.servicoExtraObrigacaoCancelarCobrancasAbertas(ctx, solicitacaoID, tipoLancamento, ano, mes, actorID, actorType, ip)
}

func (s *Service) ReativarObrigacaoServicoExtra(ctx context.Context, solicitacaoID, tipoLancamento string, ano, mes int, motivo, actorID, actorType, ip string) error {
	estado, err := s.estadoObrigacaoServicoExtra(ctx, solicitacaoID, tipoLancamento, ano, mes)
	if err != nil {
		return err
	}
	if estado != EstadoAnulado {
		return errors.New("só é possível reativar um lançamento anulado e não pago")
	}
	payload := map[string]any{"solicitacao_id": solicitacaoID, "tipo_lancamento": tipoLancamento, "motivo": strings.TrimSpace(motivo)}
	if tipoLancamento == "mensalidade" {
		payload["ano"], payload["mes"] = ano, mes
	}
	return s.recordServicoExtraObrigacao(ctx, solicitacaoID, aggregates.ObrigacaoServicoExtraReativada, payload, actorID, actorType, ip)
}

func (s *Service) servicoExtraObrigacaoCancelarCobrancasAbertas(ctx context.Context, solicitacaoID, tipoLancamento string, ano, mes int, actorID, actorType, ip string) error {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id::text,codigo_academia FROM financeiro_cobrancas WHERE payload->>'codigo_inscricao_servico'=$1 AND payload->>'tipo_lancamento_servico_extra'=$2 AND COALESCE((payload->>'ano_referencia')::int,0)=$3 AND COALESCE((payload->>'mes_referencia')::int,0)=$4 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)`, solicitacaoID, tipoLancamento, ano, mes)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, academia string
		if err := rows.Scan(&id, &academia); err != nil {
			return err
		}
		if _, err := s.CancelCharge(ctx, ContextoAcademia, academia, id, "obrigação de serviço extra anulada pela academia", actorID, actorType, ip); err != nil {
			return err
		}
	}
	return rows.Err()
}
```

**Confira `EstadoPendente`/`EstadoAnulado`/`EstadoPago` (constantes já existentes em `mensalidade.go`)** — reaproveite-as tal como estão, não crie constantes paralelas.

## 7. O que já foi validado (Claude/orquestrador) e o que falta a você (Codex)

**Já testado num PostgreSQL 16 real** (com as Fases 1 e 2 já aplicadas):
- A migration 120 completa (`ALTER TABLE` + `CREATE TABLE` + índice) aplica sem erro.
- A constraint `mes BETWEEN 1 AND 12` rejeita corretamente um mês inválido (`13`) — testado.
- Uma linha com `tipo_lancamento='preco_unico'` e `ano`/`mes` `NULL` é aceita pela constraint — testado.
- A sequência de eventos **anulada → reativada → paga** gravada na tabela, quando lida na mesma ordem (`ORDER BY ocorrido_em, id`) e processada por `precedenciaEstado`, resulta em `"pago"` — comportamento correto confirmado manualmente linha a linha.
- A sequência **paga → anulada** resulta em `"pago"` (o pagamento real prevalece sobre uma anulação posterior) — confirmado.

Resultado: **positivo** em todos os casos. Você não precisa reexecutar esta validação de schema/precedência; ela está correta. Ainda assim, escreva os testes automatizados da seção 7.1 para regressão.

**Não pôde ser validado neste ambiente** (mesma limitação de rede das fases anteriores):
- Compilação completa (`go build ./...`, `go vet ./...`) — esta fase faz edições cirúrgicas em `internal/finance/appypay.go`, `internal/finance/cobranca_geracao.go` e `internal/projections/financeiro_projection.go` (todos arquivos grandes já existentes) além do arquivo novo — é o ponto de maior risco de erro de sintaxe/import desta fase inteira. Revise com atenção redobrada antes de considerar concluído.
- Teste de integração ponta a ponta (seção 7.2) — requer Postgres real; se disponível no seu ambiente, execute; caso contrário, documente e pule apenas o(s) teste(s) marcados `RUN_POSTGRES_INTEGRATION=1`.

### 7.1 Testes unitários obrigatórios

`internal/finance/servico_extra_obrigacao_test.go` (sem necessidade de Postgres — teste `precedenciaEstado` isoladamente, já é testável em memória):
- As três sequências acima (anulada→reativada→paga = pago; paga→anulada = pago; sem eventos = pendente) como teste de tabela, reaproveitando `precedenciaEstado` diretamente.
- `PendenciasServicoExtra` com `tipo_cobranca="unico"` devolve exatamente 1 item.
- `PendenciasServicoExtra` com `tipo_cobranca="mensal"`, `vinculadaEm` em Janeiro e `fimPeriodo` em Março do mesmo ano, devolve exatamente 3 itens (Jan, Fev, Mar), nesta ordem.
- `PendenciasServicoExtra` com `vinculadaEm` e `fimPeriodo` no mesmo mês devolve exatamente 1 item.

### 7.2 Teste de integração obrigatório (requer Postgres real + mock HTTP da AppyPay)

1. Serviço `tipo_cobranca="mensal"`, inscrição vinculada há 3 meses (ajuste `vinculada_em` manualmente via SQL no teste para simular o tempo passado, já que não há máquina do tempo).
2. `GET .../pendencias` → 3 itens, todos `"pendente"`.
3. Pagar o mês 1 via webhook simulado (mesma técnica da Fase 2) → `GET .../pendencias` novamente → mês 1 `"pago"`, meses 2 e 3 `"pendente"`.
4. Academia anula o mês 2 (`POST .../obrigacoes/anular`) → mês 2 vira `"anulada"`; tentar pagar o mês 2 → erro.
5. Academia reativa o mês 2 → volta a `"pendente"`; agora pagável de novo.
6. Cancelar a inscrição (mês corrente = mês 4) → `GET .../pendencias` não deve mais incluir o mês 4 nem além, mesmo que o tempo "avance" depois do cancelamento (teste isto ajustando `fimPeriodo` no handler para refletir `sol.UpdatedAt`, não `time.Now()`, quando `status="cancelada"`).
7. Repita o fluxo com um serviço `tipo_cobranca="unico"`: 1 pendência, pagável uma vez, `"pago"` depois de confirmado, erro ao tentar pagar de novo.
8. Confirme, consultando `financeiro_cobrancas` diretamente, que as cobranças desta fase carregam `tipo_lancamento_servico_extra` e (quando aplicável) `mes_referencia`/`ano_referencia` corretos, e que `origem` (seção 6.3 da Fase 2) continua `"servico_extra"` para todas elas.

## 8. Handlers e rotas

| Método | Rota | Grupo | Descrição |
|---|---|---|---|
| `GET` | `/estudante/servicos-extras/minhas-inscricoes/:id/pendencias` | `estudante` | Lista pendências da própria inscrição (posse verificada) |
| `GET` | `/academia/servicos-extras/inscricoes/:id/pendencias` | `academiaRead` | Idem, visão da academia |
| `POST` | `/financeiro/servicos-extras/obrigacao/pagamento` | `protected` | Estudante inicia pagamento de um lançamento (`solicitacao_id`, `tipo_lancamento`, `ano`/`mes` se aplicável, `metodo_pagamento`) — mesmo cuidado de identidade forçada da Fase 2, seção 7.5 |
| `POST` | `/financeiro/servicos-extras/obrigacao/anular` | `financeiro` (academia/admin) | `AnularObrigacaoServicoExtra` |
| `POST` | `/financeiro/servicos-extras/obrigacao/reativar` | `financeiro` (academia/admin) | `ReativarObrigacaoServicoExtra` |

Siga exatamente o padrão de handler já estabelecido nas Fases 1 e 2 (posse do recurso antes de qualquer ação; resposta montada a partir de dados já resolvidos, nunca com leitura assíncrona de projeção logo após escrita — aqui isto é ainda mais simples porque, como observado na seção 6.1, a escrita desta fase já é síncrona via `ApplyLatestForAggregate`, mas mantenha o hábito mesmo assim por consistência).

## 9. Atualização da documentação de API

Adicione as 5 rotas da seção 8 na seção `## 20. Serviços Extras` (ou `## 19. Financeiro / AppyPay`, para as três rotas sob `/financeiro/...`, seguindo o mesmo critério de organização já usado para as rotas de pagamento de matrícula/mensalidade). Documente explicitamente, no texto da seção financeira, que uma `SolicitacaoServicoExtra` pode agora ter cobranças de três naturezas (`taxa_inscricao`, `mensalidade`, `preco_unico`), todas sob `origem=servico_extra`.

## 10. Checklist de aceite da Fase 3

- [ ] Migration 120 aplicada sem erro; constraints e precedência de estado testados (já validado — ver seção 7).
- [ ] `vinculada_em` persistido corretamente no aggregate e na projeção, nunca sobrescrito após o primeiro `vinculada`.
- [ ] 3 novos eventos na whitelist (`validEventTypes`), aggregate type continua sendo `"Financeiro"`.
- [ ] `ChargeRequest`/`QRCodeRequest`/`gerarCobrancaInput`/payload persistido com os 3 novos campos; `DadosServicoExtraDaCobranca` substitui `CodigoInscricaoServicoExtraDaCobranca` da Fase 2 nos três pontos de confirmação.
- [ ] `FinanceiroProjection.Handle` com o novo `case`, escrevendo em `financeiro_servico_extra_obrigacoes_eventos` com proteção de idempotência por `event_id`.
- [ ] `PendenciasServicoExtra`, `IniciarPagamentoServicoExtraObrigacao`, `ConfirmarLancamentoServicoExtraPago`, `AnularObrigacaoServicoExtra`, `ReativarObrigacaoServicoExtra` implementados e testados.
- [ ] Cancelamento de uma inscrição para de gerar pendências a partir do mês seguinte ao da transição para `cancelada` (testado).
- [ ] `Documentação da API.md` atualizada.
- [ ] `go build ./...` e `go vet ./...` limpos no seu ambiente — atenção redobrada aqui (arquivos grandes editados cirurgicamente).
- [ ] Resultado reportado ao final: o que passou, o que falhou, o que não pôde ser testado no seu ambiente e por quê.
