---
criado: 2026-08-27
origem: conversa com Fredy (Claude como orquestrador — depuração e implementação com PostgreSQL 16 real em sandbox, Codex como executor)
status: pendente
tipo: nova_funcionalidade
depende_de: "70 (conceitualmente — 71 só faz sentido pleno depois de 70 estar aplicada, para que o Expired de uma referência realmente expirada seja refletido; não há dependência técnica de arquivo/linha entre as duas)"
---

# Permitir que a academia defina um dia-limite de pagamento de mensalidade, aplicado como prazo de expiração (dueDate) nas cobranças REF

## 0. Leia isto primeiro — sobre esta tarefa e sobre o seu ambiente (Codex)

Claude já implementou, testou e validou esta funcionalidade inteira com PostgreSQL 16 real: `go build ./...`, `go vet ./...`, `gofmt -l .` (limpo) e `go test ./...` — **suíte inteira do repositório**, com banco de dados limpo a cada execução — todos passando. Foi validada também depois de rebasear em cima do commit 598e142 (`Corrigir equivalencia do filtro estado=Failed...`, PR #578), mesclado ao `main` enquanto esta tarefa estava sendo escrita — sem nenhum conflito.

**Esta tarefa tem uma decisão de segurança embutida que o Codex NÃO deve remover nem contornar**: o envio real de `dueDate` para a AppyPay fica atrás de uma variável de ambiente desligada por padrão (`APPYPAY_REF_DUEDATE_ENABLED`), porque a hipótese de que `paymentInfo: {dueDate}` sozinho (sem `referenceNumber`/`nib`) é aceito pela AppyPay **não foi confirmada contra o ambiente de testes real da AppyPay** — só contra a documentação e contra os mocks de teste do próprio Spuri. Ver seção 2.4 para o racional completo. Não ligar essa flag em produção sem essa confirmação manual.

O seu ambiente (Codex) não tem `psql`/Docker/Postgres e bloqueia `apt`. A sua tarefa é aplicar os diffs da seção 3, na ordem em que aparecem, e depois rodar o checklist da seção 5.1.

## 1. Prompt recomendado para executar esta tarefa

> Execute exatamente as alterações descritas neste documento, nesta ordem, sobre o `main` atual do repositório. Todas as decisões de desenho já foram tomadas e validadas por Claude (implementação testada com `go build`, `go vet`, `gofmt` e a suíte inteira `go test ./...` contra PostgreSQL 16 real). Preste atenção especial à seção 2.4 (o interruptor de segurança `APPYPAY_REF_DUEDATE_ENABLED`) — ele deve permanecer desligado por padrão; não o ative nem o remova. Sua tarefa é mecânica: (1) aplicar os diffs/criar os arquivos da seção 3, na ordem em que aparecem; (2) rodar cada item do checklist da seção 5.1 e reportar o resultado; (3) seguir o "Procedimento de conclusão" (seção 7). Não toque em nenhum arquivo fora do escopo listado na seção 6.

---

## 2. Contexto e desenho

### 2.1 O pedido original

> "No caso da definição de data de expiração, permitir que as academias definam o dia limite para se pagar a mensalidade, esse dia será padrão em todo mês habilitado para pagar a mensalidade. Ex: definido dia 10, então dia 10 de todo mês elegível para pagar a mensalidade nessa academia deve ser o limite, e toda a cobrança gerada para cada um desses meses terá a data de expiração marcada para o dia 10 do mês+ano (calcular corretamente o ano civil do mês desse ano letivo) dessa mesma mensalidade."

Ou seja: **um único valor por academia** (não por ano letivo), que vira o `dueDate` do REF de cada cobrança de mensalidade, no mês/ano civil correto daquela mensalidade específica.

### 2.2 Por que REF, e por que a AppyPay já suporta isto

A matriz "Financial operations" da documentação da AppyPay mostra que **"Payment expiration"** só existe para REF (default de expiração: 72h, se `dueDate` não for enviado). GPO não tem esse conceito (é um fluxo síncrono de autorização no telefone do cliente, com timeout de ~90 segundos, não uma referência que "expira" num prazo configurável). Por isso esta tarefa só afeta cobranças REF de mensalidade — GPO/GPO_QR e matrícula continuam exatamente como estavam.

### 2.3 Onde isso se encaixa na arquitetura atual (pós-tarefa 67)

A tarefa 67 (`fix(finance): unificar geração de cobrança REF/GPO/GPO_QR num único ponto (gerarCobranca)`, já mesclada) criou `internal/finance/cobranca_geracao.go`, com uma única função `gerarCobranca(ctx, gerarCobrancaInput, ...)` chamada tanto por `IniciarPagamentoMensalidades` quanto por `IniciarPagamentoMatricula`. Isso torna esta tarefa mais simples: só é preciso adicionar **um campo opcional** a `gerarCobrancaInput`, que só `IniciarPagamentoMensalidades` preenche — matrícula nunca define um `dueDate`.

### 2.4 A descoberta que mudou o desenho: a validação "tudo ou nada" do REF, e por que o envio fica atrás de uma flag

`validateCharge` (em `appypay.go`) já exigia, antes desta tarefa, que **se** `paymentInfo` de uma cobrança REF não estivesse vazio, ele tivesse **os três campos completos**: `referenceNumber`, `dueDate` e `nib` (conta bancária). Essa regra reflete a documentação da AppyPay, que mostra dois fluxos mutuamente exclusivos para REF: "referência gerada pelo gateway" (paymentInfo vazio, é o que a Spuri sempre usou) ou "referência gerada pelo comerciante" (os três campos juntos).

**A Spuri não tem, em lugar nenhum do código, um `nib` configurado** — nem por academia, nem global. Implementar o fluxo completo ("referência gerada pelo comerciante") exigiria decidir se o NIB é uma conta única da Spuri ou uma por academia, e obter esse valor real — uma decisão de negócio que não pode ser tomada nesta tarefa.

**Decisão tomada**: em vez de bloquear a funcionalidade toda por causa disso, a validação foi relaxada para aceitar um caso adicional — `paymentInfo` do REF contendo **só** `dueDate` (sem `referenceNumber` nem `nib`), mantendo a regra "tudo ou nada" original para quem precisar do fluxo completo no futuro. Isso significa que a Spuri continua deixando a AppyPay gerar a referência (como sempre fez), só que agora pode sugerir um prazo de expiração customizado.

**Só que isto não está confirmado contra o ambiente real da AppyPay** — só contra a documentação (que não mostra explicitamente esse caso combinado) e contra os mocks de teste do próprio Spuri (que, claro, aceitam o que quer que a Spuri decida simular). Por isso, o envio de fato do campo `dueDate` para a AppyPay fica atrás de uma variável de ambiente desligada por padrão:

```go
func refDueDateEnabled() bool {
	return strings.TrimSpace(os.Getenv("APPYPAY_REF_DUEDATE_ENABLED")) == "1"
}
```

Enquanto desligada (o padrão, inclusive em produção até uma confirmação manual): a academia pode configurar o dia-limite normalmente, o valor é calculado e ficaria pronto para ser enviado — mas nunca chega a sair para a AppyPay. **Antes de ligar esta flag em produção**, alguém precisa confirmar manualmente, contra o ambiente de testes real da AppyPay (não só a documentação), que uma requisição `POST /charges` para REF com `paymentInfo: {"dueDate": "..."}` (sem `referenceNumber` nem `nib`) é aceita e resulta numa referência com o prazo de expiração customizado. Se a AppyPay rejeitar essa combinação, será necessário implementar o fluxo completo (gerar `referenceNumber` na Spuri e obter um `nib` real — provavelmente uma tarefa própria, maior, envolvendo decisão de negócio de Fredy).

### 2.5 O cálculo do ano civil correto (a parte "calcular corretamente" do pedido)

O sistema já tem, em `mensalidade.go`, a função `mesesAnoLetivo(anoLetivo, nivel)`, que resolve o ano civil de cada mês de um ano letivo (formato `"AAAA_AAAA"`, ex. `"2025_2026"`): meses de setembro a dezembro (ou outubro a dezembro, para nível superior) pertencem ao primeiro ano; meses de janeiro a julho pertencem ao segundo. Essa resolução **já está pronta e testada** em produção — em vez de duplicá-la, esta tarefa reaproveita `MensalidadeMesView.DataReferencia` (que já carrega o ano civil correto de cada mês pendente) para saber em que ano civil cair o dia-limite.

Nova função pura, sem depender de nenhuma consulta ao banco:

```go
func dataLimiteMensalidade(ano, mes, diaLimite int) time.Time {
	ultimoDoMes := time.Date(ano, time.Month(mes)+1, 0, 0, 0, 0, 0, diaLimiteZone).Day()
	dia := diaLimite
	if dia > ultimoDoMes {
		dia = ultimoDoMes
	}
	return time.Date(ano, time.Month(mes), dia, 23, 59, 59, 0, diaLimiteZone)
}
```

Faz o "clamp" para o último dia real do mês quando `dia_limite` (1-31) exceder o número de dias desse mês/ano específico (ex.: dia_limite=31 em fevereiro vira 28 ou 29, dependendo do ano ser bissexto) — nunca rola para o mês seguinte.

### 2.6 Fuso horário do `dueDate`

O exemplo da documentação da AppyPay para `dueDate` (`"2022-01-01T15:00:00"`) não tem offset de fuso horário. Não há confirmação em que fuso esse literal é interpretado pela AppyPay. Assumimos hora de Angola (WAT, UTC+1 fixo, sem horário de verão) por ser um provedor angolano — implementado como `time.FixedZone("WAT", 3600)` em vez de `time.LoadLocation("Africa/Luanda")`, para não depender da base `tzdata` estar instalada no ambiente de execução (o `Dockerfile` de produção já instala `tzdata`, mas o ambiente do Codex pode não ter). **Esta suposição também deve ser confirmada junto com a hipótese da seção 2.4**, contra o ambiente de testes real da AppyPay.

### 2.7 Cobrança que junta vários meses, e mensalidade em atraso

`IniciarPagamentoMensalidades` permite selecionar mais de um mês numa única cobrança. REF só aceita um `dueDate` por cobrança — usa-se o mês mais antigo entre os selecionados, comparado pela **data real** (não pela posição no ano letivo, já que os meses selecionados podem até pertencer a anos letivos diferentes), por ser o mais urgente.

Uma mensalidade em atraso (cujo mês de referência, e portanto cujo dia-limite, já passou) nunca deveria gerar um `dueDate` no passado — a AppyPay rejeitaria a criação da própria referência (erro 762, "Due Date can not be less than current dateTime"). Por isso `gerarCobranca` só define `dueDate` quando a data calculada ainda está no futuro no momento da criação; caso contrário, omite o campo e deixa a AppyPay aplicar seu próprio padrão de 72h — garantindo que pagar uma mensalidade atrasada continue funcionando normalmente.

---

## 3. Diffs e arquivos a criar, na ordem exata

### 3.1 Migration `migrations/112_financeiro_mensalidade_dia_limite.sql`

Verificar antes de aplicar que `112` ainda é o próximo número de migration disponível (rodar `ls migrations/ | sort -t_ -k1 -n | tail -3` — se já existir uma migration 112 de outra tarefa aplicada nesse meio tempo, renumerar este arquivo para o próximo número livre, mantendo o conteúdo idêntico).

Criar com exatamente este conteúdo:

```sql
-- Dia-limite de pagamento de mensalidade: permite que uma academia defina,
-- de uma vez só (sem depender de ano_letivo), o dia do mês (1-31) usado
-- como prazo de pagamento de TODA mensalidade elegível, em qualquer ano
-- letivo. Ex.: dia_limite=10 -> toda mensalidade tem como data-limite o dia
-- 10 do mês/ano civil daquela própria mensalidade (ver
-- Service.dataLimiteMensalidade e mesesAnoLetivo, em mensalidade.go, para
-- como o ano civil de cada mês do ano letivo é resolvido).
--
-- Mesmo padrão de event sourcing das demais configurações do módulo
-- financeiro (ver o comentário no topo de 109_financeiro_remocao_configuracoes.sql):
--   1. financeiro_mensalidade_dia_limite guarda cada fato "definido em X"
--      (nunca é reescrita nem perde linhas).
--   2. financeiro_mensalidade_dia_limite_remocoes guarda cada fato
--      "removido em X".
--   3. financeiro_mensalidade_dia_limite_atual resolve o valor vigente,
--      combinando a versão mais recente com a remoção mais recente. A
--      ausência de linha vigente faz o backend usar o padrão da AppyPay
--      para REF (expiração em 72h, sem dueDate explícito) — ver
--      Service.diaLimiteEfetivo.

CREATE TABLE financeiro_mensalidade_dia_limite (
    event_id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    codigo_academia TEXT NOT NULL,
    dia_limite SMALLINT NOT NULL CHECK (dia_limite BETWEEN 1 AND 31),
    definido_em TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_fin_mensalidade_dia_limite_lookup
    ON financeiro_mensalidade_dia_limite (codigo_academia, definido_em DESC);

CREATE TABLE financeiro_mensalidade_dia_limite_remocoes (
    event_id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    codigo_academia TEXT NOT NULL,
    removido_em TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_fin_mensalidade_dia_limite_remocoes_lookup
    ON financeiro_mensalidade_dia_limite_remocoes (codigo_academia, removido_em DESC);

CREATE VIEW financeiro_mensalidade_dia_limite_atual AS
SELECT c.codigo_academia, c.dia_limite, c.definido_em
FROM (
    SELECT DISTINCT ON (codigo_academia)
        codigo_academia, dia_limite, definido_em
    FROM financeiro_mensalidade_dia_limite
    ORDER BY codigo_academia, definido_em DESC, event_id DESC
) c
LEFT JOIN LATERAL (
    SELECT removido_em FROM financeiro_mensalidade_dia_limite_remocoes r
    WHERE r.codigo_academia = c.codigo_academia
      AND r.removido_em >= c.definido_em
    ORDER BY r.removido_em DESC LIMIT 1
) rm ON true
WHERE rm.removido_em IS NULL;

COMMENT ON TABLE financeiro_mensalidade_dia_limite IS
    'Fatos imutáveis de DiaLimiteCobrancaDefinido: dia do mês (1-31) usado como prazo de pagamento (dueDate do REF) para toda mensalidade elegível desta academia, em qualquer ano letivo. Um dia_limite maior que o número de dias do mês/ano civil resolvido é ajustado para o último dia real desse mês (ex.: 31 em fevereiro vira 28 ou 29) — ver Service.dataLimiteMensalidade.';
COMMENT ON TABLE financeiro_mensalidade_dia_limite_remocoes IS
    'Fatos imutáveis de DiaLimiteCobrancaRemovido. A ausência de linha vigente em financeiro_mensalidade_dia_limite_atual faz o backend voltar a omitir paymentInfo.dueDate nas cobranças REF de mensalidade, usando o padrão de expiração em 72h da própria AppyPay.';
COMMENT ON VIEW financeiro_mensalidade_dia_limite_atual IS
    'Dia-limite vigente por academia (ou nenhuma linha, se a última remoção for igual/posterior à última definição). Não versionado por ano_letivo: um único valor vale para todo ano letivo elegível a mensalidade nesta academia.';
```

### 3.2 Diff combinado (múltiplos arquivos): `internal/db/safe_queries.go`, `internal/domain/aggregates/financeiro.go`, `internal/projections/financeiro_projection.go`, `internal/finance/cobranca_geracao.go`, `internal/finance/mensalidade.go`, `internal/handlers/mensalidade_handlers.go`, `cmd/server/main.go`

Aplicar via `git apply` (o patch cobre vários arquivos de uma vez; se falhar por drift de linha em algum arquivo específico, aplicar manualmente só a parte daquele arquivo):

```diff
diff --git a/cmd/server/main.go b/cmd/server/main.go
index 5b902f6..77aeeb8 100644
--- a/cmd/server/main.go
+++ b/cmd/server/main.go
@@ -387,6 +387,8 @@ func setupRouter() *gin.Engine {
 			financeiro.DELETE("/mensalidades/configuracoes", handlers.RemoverConfiguracaoMensalidade)
 			financeiro.POST("/mensalidades/inicio-cobranca", handlers.DefinirMesInicioCobranca)
 			financeiro.DELETE("/mensalidades/inicio-cobranca", handlers.RemoverMesInicioCobranca)
+			financeiro.POST("/mensalidades/dia-limite-cobranca", handlers.DefinirDiaLimiteCobranca)
+			financeiro.DELETE("/mensalidades/dia-limite-cobranca", handlers.RemoverDiaLimiteCobranca)
 			financeiro.POST("/mensalidades/obrigacoes/anular", handlers.AnularObrigacoesMensalidade)
 			financeiro.POST("/mensalidades/obrigacoes/reativar", handlers.ReativarObrigacoesMensalidade)
 			financeiro.POST("/matriculas/configuracoes", handlers.ConfigurarMatricula)
diff --git a/internal/db/safe_queries.go b/internal/db/safe_queries.go
index 287d999..559810f 100644
--- a/internal/db/safe_queries.go
+++ b/internal/db/safe_queries.go
@@ -115,8 +115,13 @@ var validEventTypes = map[string]bool{
 	"MensalidadeConfiguracaoRemovida":                    true,
 	"MesInicioCobrancaDefinido":                          true,
 	"MesInicioCobrancaRemovido":                          true,
-	"ObrigacaoMensalidadeAnulada":                        true,
-	"ObrigacaoMensalidadeReativada":                      true,
+	// DiaLimiteCobrancaDefinido/Removido: dia-limite de pagamento de
+	// mensalidade (ver docs/Lista de Tarefas/69 - ...md), introduzido junto
+	// com paymentInfo.dueDate nas cobranças REF de mensalidade.
+	"DiaLimiteCobrancaDefinido":     true,
+	"DiaLimiteCobrancaRemovido":     true,
+	"ObrigacaoMensalidadeAnulada":   true,
+	"ObrigacaoMensalidadeReativada": true,
 	// MensalidadePaga is emitted by Phase 3. It is registered now so this
 	// projection can consume a real payment event without any compatibility
 	// path or inferred payment state.
diff --git a/internal/domain/aggregates/financeiro.go b/internal/domain/aggregates/financeiro.go
index 074d388..6e3fed9 100644
--- a/internal/domain/aggregates/financeiro.go
+++ b/internal/domain/aggregates/financeiro.go
@@ -22,6 +22,8 @@ const (
 	MensalidadeConfiguracaoRemovida        = "MensalidadeConfiguracaoRemovida"
 	MesInicioCobrancaDefinido              = "MesInicioCobrancaDefinido"
 	MesInicioCobrancaRemovido              = "MesInicioCobrancaRemovido"
+	DiaLimiteCobrancaDefinido              = "DiaLimiteCobrancaDefinido"
+	DiaLimiteCobrancaRemovido              = "DiaLimiteCobrancaRemovido"
 	ObrigacaoMensalidadeAnulada            = "ObrigacaoMensalidadeAnulada"
 	ObrigacaoMensalidadeReativada          = "ObrigacaoMensalidadeReativada"
 	MensalidadePaga                        = "MensalidadePaga"
diff --git a/internal/finance/cobranca_geracao.go b/internal/finance/cobranca_geracao.go
index a839dfa..24273a2 100644
--- a/internal/finance/cobranca_geracao.go
+++ b/internal/finance/cobranca_geracao.go
@@ -2,7 +2,9 @@ package finance
 
 import (
 	"context"
+	"os"
 	"strings"
+	"time"
 )
 
 // gerarCobrancaInput agrupa os parâmetros necessários para emitir uma nova
@@ -25,6 +27,17 @@ type gerarCobrancaInput struct {
 	CodigoEstudante       string
 	CodigoSolicitacao     string
 	Mensalidades          []MensalidadeSelecaoMes
+	// DataLimitePagamento, quando preenchido E ainda no futuro no momento
+	// da criação, vira paymentInfo.dueDate de uma cobrança REF (a AppyPay
+	// documenta "Payment expiration" só para REF — GPO/GPO_QR ignoram este
+	// campo). Uma data no passado (ex.: mensalidade em atraso, cujo
+	// dia-limite original já passou) é silenciosamente ignorada: a AppyPay
+	// rejeita um dueDate anterior ao instante atual (erro 762), então
+	// nesse caso é melhor deixar a AppyPay aplicar seu padrão de expiração
+	// em 72h do que impedir o pagamento de uma mensalidade atrasada.
+	// Apenas IniciarPagamentoMensalidades preenche este campo — matrícula
+	// (IniciarPagamentoMatricula) nunca define um dueDate.
+	DataLimitePagamento *time.Time
 }
 
 // gerarCobranca é a única função do módulo financeiro que decide, a partir
@@ -75,6 +88,17 @@ func (s *Service) gerarCobranca(ctx context.Context, in gerarCobrancaInput, acto
 	if in.MetodoPagamento == "GPO" {
 		info["phoneNumber"] = strings.TrimSpace(in.Telefone)
 	}
+	if in.MetodoPagamento == "REF" && in.DataLimitePagamento != nil && in.DataLimitePagamento.After(time.Now()) && refDueDateEnabled() {
+		// Formato exigido pela AppyPay para paymentInfo.dueDate: sem fuso
+		// horário explícito no literal (ex.: "2022-01-01T15:00:00" no
+		// exemplo da documentação) — interpretamos isso como hora local de
+		// Angola (ver diaLimiteZone em mensalidade.go) e formatamos nesse
+		// mesmo fuso, já que a AppyPay é um provedor angolano e não
+		// documenta explicitamente em qual fuso o literal é interpretado.
+		// Validar contra o ambiente de testes real da AppyPay antes de
+		// generalizar — ver a tarefa que introduziu este campo.
+		info["dueDate"] = in.DataLimitePagamento.In(diaLimiteZone).Format("2006-01-02T15:04:05")
+	}
 	charge, err := s.CreateCharge(ctx, ChargeRequest{
 		ContextoTipo:          ContextoAcademia,
 		CodigoAcademia:        in.CodigoAcademia,
@@ -93,3 +117,17 @@ func (s *Service) gerarCobranca(ctx context.Context, in gerarCobrancaInput, acto
 	}
 	return QRCodeResult{ChargeResult: charge}, nil
 }
+
+// refDueDateEnabled é um interruptor de segurança, desligado por padrão: só
+// depois de confirmar manualmente contra o ambiente de testes real da
+// AppyPay que paymentInfo.dueDate sozinho (sem referenceNumber nem nib) é
+// de facto aceite para REF é que faz sentido ligar
+// APPYPAY_REF_DUEDATE_ENABLED=1. Enquanto desligado, o dia-limite de
+// mensalidade continua configurável e calculado normalmente (ver
+// Service.diaLimiteEfetivo/dataLimiteMensalidade) — só o envio à AppyPay
+// fica inerte, para nunca arriscar quebrar a criação de cobranças REF de
+// mensalidade com uma suposição ainda não confirmada. Ver a tarefa que
+// introduziu este campo para o racional completo.
+func refDueDateEnabled() bool {
+	return strings.TrimSpace(os.Getenv("APPYPAY_REF_DUEDATE_ENABLED")) == "1"
+}
diff --git a/internal/finance/mensalidade.go b/internal/finance/mensalidade.go
index 460ccc5..6b168d9 100644
--- a/internal/finance/mensalidade.go
+++ b/internal/finance/mensalidade.go
@@ -76,6 +76,16 @@ type MesInicioCobrancaInput struct {
 	MesInicio      int    `json:"mes_inicio"`
 }
 
+// DiaLimiteCobrancaInput define o dia do mês (1-31) usado como prazo de
+// pagamento (dueDate do REF) de toda mensalidade elegível desta academia,
+// em qualquer ano letivo — ver Service.dataLimiteMensalidade. Ao contrário
+// de MesInicioCobrancaInput, não é versionado por ano_letivo: um único
+// valor vigente vale para todos os anos letivos desta academia.
+type DiaLimiteCobrancaInput struct {
+	CodigoAcademia string `json:"codigo_academia"`
+	DiaLimite      int    `json:"dia_limite"`
+}
+
 type ObrigacaoMensalidadeInput struct {
 	CodigoEstudante string `json:"codigo_estudante"`
 	CodigoAcademia  string `json:"codigo_academia"`
@@ -239,6 +249,48 @@ func (s *Service) RemoveMesInicioCobranca(ctx context.Context, codigoAcademia, a
 	return s.recordMensalidade(ctx, codigoAcademia, aggregates.MesInicioCobrancaRemovido, payload, actorID, actorType, ip)
 }
 
+// DefinirDiaLimiteCobranca define (ou redefine) o dia do mês usado como
+// prazo de pagamento de toda mensalidade elegível desta academia — ver
+// dataLimiteMensalidade para como esse dia é combinado com o ano civil de
+// cada mês do ano letivo, e diaLimiteEfetivo para como gerarCobranca lê o
+// valor vigente. Como financeiro_mensalidade_dia_limite_atual não é
+// versionada por ano_letivo, redefinir o valor afeta imediatamente toda
+// cobrança de mensalidade criada a partir de agora, em qualquer ano
+// letivo — cobranças já criadas antes da redefinição mantêm o dueDate que
+// já foi enviado à AppyPay no momento da criação (nunca é alterado
+// retroativamente).
+func (s *Service) DefinirDiaLimiteCobranca(ctx context.Context, in DiaLimiteCobrancaInput, actorID, actorType, ip string) error {
+	if err := s.validateDiaLimiteCobranca(ctx, &in); err != nil {
+		return err
+	}
+	return s.recordMensalidade(ctx, in.CodigoAcademia, aggregates.DiaLimiteCobrancaDefinido, map[string]any{"codigo_academia": in.CodigoAcademia, "dia_limite": in.DiaLimite}, actorID, actorType, ip)
+}
+
+// RemoveDiaLimiteCobranca remove a redefinição do dia-limite de pagamento
+// de mensalidade, fazendo o sistema voltar a omitir paymentInfo.dueDate nas
+// cobranças REF de mensalidade (a AppyPay passa a aplicar seu próprio
+// padrão de expiração em 72h). O evento DiaLimiteCobrancaDefinido original
+// permanece intacto no ledger.
+func (s *Service) RemoveDiaLimiteCobranca(ctx context.Context, codigoAcademia, actorID, actorType, ip string) error {
+	if s.client == nil {
+		return errors.New("serviço financeiro não inicializado")
+	}
+	codigoAcademia = strings.TrimSpace(codigoAcademia)
+	if codigoAcademia == "" {
+		return errors.New("academia é obrigatória")
+	}
+	var existe bool
+	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_mensalidade_dia_limite_atual WHERE codigo_academia=$1)`, codigoAcademia).Scan(&existe)
+	if err != nil {
+		return err
+	}
+	if !existe {
+		return fmt.Errorf("%w: nenhum dia-limite de cobrança definido para esta academia", ErrNotFound)
+	}
+	payload := map[string]any{"codigo_academia": codigoAcademia}
+	return s.recordMensalidade(ctx, codigoAcademia, aggregates.DiaLimiteCobrancaRemovido, payload, actorID, actorType, ip)
+}
+
 func (s *Service) AnularObrigacoesMensalidade(ctx context.Context, in ObrigacaoMensalidadeInput, actorID, actorType, ip string) error {
 	return s.alterarObrigacoesMensalidade(ctx, in, aggregates.ObrigacaoMensalidadeAnulada, actorID, actorType, ip)
 }
@@ -316,6 +368,7 @@ func (s *Service) IniciarPagamentoMensalidades(ctx context.Context, in Mensalida
 		return MensalidadePagamentoView{}, errors.New("não há mensalidades pendentes nesta academia")
 	}
 	selected, total := map[string]bool{}, 0.0
+	var maisAntiga *MensalidadeMesView
 	for _, m := range in.Meses {
 		key := m.AnoLetivo + ":" + strconv.Itoa(m.Mes)
 		if selected[key] {
@@ -327,6 +380,19 @@ func (s *Service) IniciarPagamentoMensalidades(ctx context.Context, in Mensalida
 			if due.AnoLetivo == m.AnoLetivo && due.Mes == m.Mes {
 				total += due.Valor
 				found = true
+				// A cobrança pode reunir vários meses (ex.: pagar duas
+				// mensalidades atrasadas de uma vez), mas o REF só aceita
+				// um único dueDate por cobrança — usamos o prazo do mês
+				// mais antigo entre os selecionados (comparando a data
+				// real, não a posição no ano letivo, já que os meses
+				// selecionados podem até pertencer a anos letivos
+				// diferentes), por ser o mais urgente: nunca faz sentido
+				// dar mais prazo a um pagamento que também cobre um mês já
+				// mais atrasado.
+				if maisAntiga == nil || due.DataReferencia.Before(maisAntiga.DataReferencia) {
+					dueCopy := due
+					maisAntiga = &dueCopy
+				}
 				break
 			}
 		}
@@ -347,6 +413,13 @@ func (s *Service) IniciarPagamentoMensalidades(ctx context.Context, in Mensalida
 	}
 	total = roundAmount(total)
 	description := fmt.Sprintf("Propinas %s: %d mensalidade(s)", in.CodigoAcademia, len(in.Meses))
+	var dataLimite *time.Time
+	if dia, ok, err := s.diaLimiteEfetivo(ctx, in.CodigoAcademia); err != nil {
+		return MensalidadePagamentoView{}, err
+	} else if ok && maisAntiga != nil {
+		limite := dataLimiteMensalidade(maisAntiga.DataReferencia.Year(), int(maisAntiga.DataReferencia.Month()), dia)
+		dataLimite = &limite
+	}
 	result, err := s.gerarCobranca(ctx, gerarCobrancaInput{
 		CodigoAcademia:        in.CodigoAcademia,
 		MetodoPagamento:       in.MetodoPagamento,
@@ -356,6 +429,7 @@ func (s *Service) IniciarPagamentoMensalidades(ctx context.Context, in Mensalida
 		Telefone:              in.Telefone,
 		CodigoEstudante:       in.CodigoEstudante,
 		Mensalidades:          in.Meses,
+		DataLimitePagamento:   dataLimite,
 	}, actorID, actorType, ip)
 	if err != nil {
 		return MensalidadePagamentoView{}, err
@@ -541,6 +615,25 @@ func (s *Service) validateMesInicioCobranca(ctx context.Context, in *MesInicioCo
 	return nil
 }
 
+func (s *Service) validateDiaLimiteCobranca(ctx context.Context, in *DiaLimiteCobrancaInput) error {
+	in.CodigoAcademia = strings.TrimSpace(in.CodigoAcademia)
+	if in.CodigoAcademia == "" || in.DiaLimite < 1 || in.DiaLimite > 31 {
+		return errors.New("codigo_academia e dia_limite entre 1 e 31 são obrigatórios")
+	}
+	var typ string
+	err := s.client.DB().QueryRowContext(ctx, `SELECT type FROM projection_academias WHERE codigo_academia=$1`, in.CodigoAcademia).Scan(&typ)
+	if err == sql.ErrNoRows {
+		return fmt.Errorf("%w: academia", ErrNotFound)
+	}
+	if err != nil {
+		return err
+	}
+	if typ != "private" {
+		return errors.New("mensalidade só pode ser configurada por academia privada")
+	}
+	return nil
+}
+
 func (s *Service) recordMensalidade(ctx context.Context, codigoAcademia, event string, payload map[string]any, userID, userType, ip string) error {
 	if strings.TrimSpace(userID) == "" {
 		return errors.New("autor do evento financeiro é obrigatório")
@@ -667,6 +760,53 @@ func (s *Service) mesInicioEfetivo(ctx context.Context, academia, anoLetivo, niv
 	return mes, nil
 }
 
+// diaLimiteEfetivo devolve o dia-limite de pagamento configurado para a
+// academia (ok=true), ou ok=false quando a academia nunca configurou um
+// (ou removeu a configuração) — sinal para o chamador (gerarCobranca) não
+// enviar paymentInfo.dueDate, deixando a AppyPay aplicar seu próprio padrão
+// de expiração em 72h para REF.
+func (s *Service) diaLimiteEfetivo(ctx context.Context, academia string) (dia int, ok bool, err error) {
+	err = s.client.DB().QueryRowContext(ctx, `SELECT dia_limite FROM financeiro_mensalidade_dia_limite_atual WHERE codigo_academia=$1`, academia).Scan(&dia)
+	if err == sql.ErrNoRows {
+		return 0, false, nil
+	}
+	if err != nil {
+		return 0, false, err
+	}
+	if dia < 1 || dia > 31 {
+		return 0, false, errors.New("configuração de dia_limite inconsistente")
+	}
+	return dia, true, nil
+}
+
+// diaLimiteZone é o fuso horário fixo de Angola (WAT, UTC+1, sem horário de
+// verão) usado para calcular o fim do dia de uma data-limite de pagamento
+// de mensalidade. Um fuso fixo (em vez de time.LoadLocation("Africa/Luanda"))
+// evita depender da base tzdata estar instalada no ambiente de execução —
+// o Dockerfile de produção já instala tzdata, mas ambientes de teste (ex.:
+// Codex) podem não ter.
+var diaLimiteZone = time.FixedZone("WAT", 3600)
+
+// dataLimiteMensalidade calcula o fim do dia (23:59:59, hora de Angola) do
+// dia_limite configurado pela academia, no ano civil e mês fornecidos —
+// já resolvidos pelo chamador a partir do ano letivo (ver
+// MensalidadeMesView.DataReferencia, preenchido a partir de
+// mesesAnoLetivo). Quando dia_limite exceder o número de dias do mês/ano
+// civil (ex.: 31 em fevereiro), usa o último dia real desse mês em vez de
+// rolar para o mês seguinte — nunca gera uma data num mês diferente do
+// pedido.
+func dataLimiteMensalidade(ano, mes, diaLimite int) time.Time {
+	// O dia 0 do mês seguinte, no fuso escolhido, é sempre o último dia do
+	// mês pedido — uma forma robusta de descobrir quantos dias o mês tem
+	// sem tabela hardcoded nem tratamento manual de anos bissextos.
+	ultimoDoMes := time.Date(ano, time.Month(mes)+1, 0, 0, 0, 0, 0, diaLimiteZone).Day()
+	dia := diaLimite
+	if dia > ultimoDoMes {
+		dia = ultimoDoMes
+	}
+	return time.Date(ano, time.Month(mes), dia, 23, 59, 59, 0, diaLimiteZone)
+}
+
 // mesNaturalInicioAnoLetivo devolve o mês de calendário em que o ano letivo
 // começa para o nível informado. Fundamental e médio começam em setembro;
 // superior começa em outubro.
diff --git a/internal/handlers/mensalidade_handlers.go b/internal/handlers/mensalidade_handlers.go
index 564008f..c3ab56b 100644
--- a/internal/handlers/mensalidade_handlers.go
+++ b/internal/handlers/mensalidade_handlers.go
@@ -235,6 +235,57 @@ func RemoverMesInicioCobranca(c *gin.Context) {
 	c.Status(http.StatusNoContent)
 }
 
+func DefinirDiaLimiteCobranca(c *gin.Context) {
+	var in finance.DiaLimiteCobrancaInput
+	if err := c.ShouldBindJSON(&in); err != nil {
+		utils.RespondWithValidationError(c, errors.New("payload inválido"))
+		return
+	}
+	if !authorizeMensalidadeAcademia(c, &in.CodigoAcademia, true) {
+		utils.RespondWithForbiddenError(c, "sem permissão para definir o dia-limite de cobrança desta academia")
+		return
+	}
+	id, typ, _, ok := financeActor(c)
+	if !ok {
+		utils.RespondWithUnauthorizedError(c)
+		return
+	}
+	if err := FinanceiroService.DefinirDiaLimiteCobranca(c.Request.Context(), in, id.String(), typ, c.ClientIP()); err != nil {
+		financeError(c, err)
+		return
+	}
+	c.Status(http.StatusCreated)
+}
+
+// RemoverDiaLimiteCobrancaInput identifica a academia cujo dia-limite de
+// cobrança deve deixar de valer, voltando o sistema a omitir
+// paymentInfo.dueDate nas cobranças REF de mensalidade.
+type RemoverDiaLimiteCobrancaInput struct {
+	CodigoAcademia string `json:"codigo_academia"`
+}
+
+func RemoverDiaLimiteCobranca(c *gin.Context) {
+	var in RemoverDiaLimiteCobrancaInput
+	if err := c.ShouldBindJSON(&in); err != nil {
+		utils.RespondWithValidationError(c, errors.New("payload inválido"))
+		return
+	}
+	if !authorizeMensalidadeAcademia(c, &in.CodigoAcademia, true) {
+		utils.RespondWithForbiddenError(c, "sem permissão para remover o dia-limite de cobrança desta academia")
+		return
+	}
+	id, typ, _, ok := financeActor(c)
+	if !ok {
+		utils.RespondWithUnauthorizedError(c)
+		return
+	}
+	if err := FinanceiroService.RemoveDiaLimiteCobranca(c.Request.Context(), in.CodigoAcademia, id.String(), typ, c.ClientIP()); err != nil {
+		financeError(c, err)
+		return
+	}
+	c.Status(http.StatusNoContent)
+}
+
 func ConsultarMensalidadesEstudante(c *gin.Context) {
 	codigo := strings.TrimSpace(c.Param("codigo"))
 	var estudanteID string
diff --git a/internal/projections/financeiro_projection.go b/internal/projections/financeiro_projection.go
index 9137b65..82f88cf 100644
--- a/internal/projections/financeiro_projection.go
+++ b/internal/projections/financeiro_projection.go
@@ -161,6 +161,31 @@ func (p *FinanceiroProjection) Handle(e db.Event) error {
 		}
 		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_inicio_cobranca_remocoes (event_id,aggregate_id,codigo_academia,ano_letivo,removido_em) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.AnoLetivo, e.OccurredAt)
 		return err
+	case "DiaLimiteCobrancaDefinido":
+		var in struct {
+			CodigoAcademia string `json:"codigo_academia"`
+			DiaLimite      int    `json:"dia_limite"`
+		}
+		if err := json.Unmarshal(e.Payload, &in); err != nil {
+			return err
+		}
+		if in.CodigoAcademia == "" || in.DiaLimite < 1 || in.DiaLimite > 31 {
+			return fmt.Errorf("evento DiaLimiteCobrancaDefinido inválido")
+		}
+		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_dia_limite (event_id,aggregate_id,codigo_academia,dia_limite,definido_em) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.DiaLimite, e.OccurredAt)
+		return err
+	case "DiaLimiteCobrancaRemovido":
+		var in struct {
+			CodigoAcademia string `json:"codigo_academia"`
+		}
+		if err := json.Unmarshal(e.Payload, &in); err != nil {
+			return err
+		}
+		if in.CodigoAcademia == "" {
+			return fmt.Errorf("evento DiaLimiteCobrancaRemovido inválido")
+		}
+		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_dia_limite_remocoes (event_id,aggregate_id,codigo_academia,removido_em) VALUES ($1,$2,$3,$4) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, e.OccurredAt)
+		return err
 	case "ObrigacaoMensalidadeAnulada", "ObrigacaoMensalidadeReativada", "MensalidadePaga":
 		var in struct {
 			CodigoEstudante string `json:"codigo_estudante"`
```

### 3.3 Diff de `internal/finance/cobranca_geracao_integration_test.go`

```diff
diff --git a/internal/finance/cobranca_geracao_integration_test.go b/internal/finance/cobranca_geracao_integration_test.go
index c408ebd..49a1fb0 100644
--- a/internal/finance/cobranca_geracao_integration_test.go
+++ b/internal/finance/cobranca_geracao_integration_test.go
@@ -3,6 +3,7 @@ package finance
 import (
 	"context"
 	"encoding/json"
+	"errors"
 	"io"
 	"net/http"
 	"strings"
@@ -205,3 +206,240 @@ func TestIntegrationGerarCobrancaREFNaoEnviaPhoneNumber(t *testing.T) {
 		}
 	})
 }
+
+// TestIntegrationDiaLimiteCobrancaEnviaDueDateParaMesFuturo cobre o
+// cenário "em dia" pedido nesta tarefa: uma academia que definiu
+// dia_limite=15 tem, na cobrança REF da primeira mensalidade de um ano
+// letivo que ainda vai começar (Setembro/2026, com "agora" em
+// Agosto/2026), paymentInfo.dueDate enviado como "2026-09-15T23:59:59" —
+// dia 15 do MESMO mês/ano civil da mensalidade, não do dia em que a
+// cobrança foi criada.
+func TestIntegrationDiaLimiteCobrancaEnviaDueDateParaMesFuturo(t *testing.T) {
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	t.Setenv("APPYPAY_REF_DUEDATE_ENABLED", "1")
+	client := integrationClient(t)
+	service := NewService(client)
+	ctx := context.Background()
+
+	academia := mensalidadeCodigo()
+	estudante := "EST-DL-" + uuid.NewString()[:8]
+	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
+	seedMensalidadeTurma(t, client, academia, "T-DL", "2026_2027", estudante, nil)
+	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 1000, 7, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
+	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{REF}' WHERE codigo_academia=$1`, academia); err != nil {
+		t.Fatal(err)
+	}
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+	if err := service.DefinirDiaLimiteCobranca(ctx, DiaLimiteCobrancaInput{CodigoAcademia: academia, DiaLimite: 15}, "fpp-test", "admin", "127.0.0.1"); err != nil {
+		t.Fatalf("DefinirDiaLimiteCobranca falhou: %v", err)
+	}
+	transport := &capturingAppyPayTransport{}
+	service.SetHTTPClient(&http.Client{Transport: transport})
+
+	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(pendentes) == 0 {
+		t.Fatal("esperava pelo menos uma mensalidade pendente")
+	}
+	alvo := mensalidadePorMes(t, pendentes, academia, "2026_2027", 9)
+
+	_, err = service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
+		CodigoEstudante: estudante, CodigoAcademia: academia,
+		Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
+		MetodoPagamento: "REF",
+	}, estudante, "estudante", "127.0.0.1")
+	if err != nil {
+		t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
+	}
+	pi := transport.paymentInfo()
+	if pi == nil || pi["dueDate"] != "2026-09-15T23:59:59" {
+		t.Fatalf("esperava paymentInfo.dueDate=2026-09-15T23:59:59, obteve %#v", pi)
+	}
+}
+
+// TestIntegrationDiaLimiteCobrancaDesligadaPorPadraoMesmoComMesFuturo prova
+// o interruptor de segurança: sem APPYPAY_REF_DUEDATE_ENABLED=1, mesmo um
+// mês futuro com dia-limite configurado NUNCA envia dueDate à AppyPay. O
+// dia-limite continua sendo calculado e persistido normalmente
+// (diaLimiteEfetivo/dataLimiteMensalidade) — só o envio fica inerte, até a
+// hipótese de paymentInfo.dueDate sozinho ser confirmada manualmente contra
+// o ambiente de testes real da AppyPay. Ver refDueDateEnabled em
+// cobranca_geracao.go.
+func TestIntegrationDiaLimiteCobrancaDesligadaPorPadraoMesmoComMesFuturo(t *testing.T) {
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	// Deliberadamente NÃO define APPYPAY_REF_DUEDATE_ENABLED.
+	client := integrationClient(t)
+	service := NewService(client)
+	ctx := context.Background()
+
+	academia := mensalidadeCodigo()
+	estudante := "EST-DLOFF-" + uuid.NewString()[:8]
+	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
+	seedMensalidadeTurma(t, client, academia, "T-DLOFF", "2026_2027", estudante, nil)
+	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 1000, 7, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
+	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{REF}' WHERE codigo_academia=$1`, academia); err != nil {
+		t.Fatal(err)
+	}
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+	if err := service.DefinirDiaLimiteCobranca(ctx, DiaLimiteCobrancaInput{CodigoAcademia: academia, DiaLimite: 15}, "fpp-test", "admin", "127.0.0.1"); err != nil {
+		t.Fatalf("DefinirDiaLimiteCobranca falhou: %v", err)
+	}
+	transport := &capturingAppyPayTransport{}
+	service.SetHTTPClient(&http.Client{Transport: transport})
+
+	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
+	if err != nil {
+		t.Fatal(err)
+	}
+	alvo := mensalidadePorMes(t, pendentes, academia, "2026_2027", 9)
+	_, err = service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
+		CodigoEstudante: estudante, CodigoAcademia: academia,
+		Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
+		MetodoPagamento: "REF",
+	}, estudante, "estudante", "127.0.0.1")
+	if err != nil {
+		t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
+	}
+	if pi := transport.paymentInfo(); pi != nil {
+		if _, ok := pi["dueDate"]; ok {
+			t.Fatalf("com a flag desligada, dueDate nunca deveria ser enviado, obteve paymentInfo=%#v", pi)
+		}
+	}
+}
+
+// TestIntegrationDiaLimiteCobrancaOmiteDueDateParaMesAtrasado cobre o
+// cenário "em atraso": uma mensalidade cujo mês de referência já passou há
+// muito (logo, cujo dia_limite daquele mês também já passou) nunca deve
+// receber um dueDate no passado — a AppyPay rejeitaria a criação da
+// referência (erro 762). Omitir o campo deixa a AppyPay aplicar seu
+// próprio padrão de expiração em 72h, garantindo que um pagamento
+// atrasado continue possível.
+func TestIntegrationDiaLimiteCobrancaOmiteDueDateParaMesAtrasado(t *testing.T) {
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	client := integrationClient(t)
+	service := NewService(client)
+	ctx := context.Background()
+
+	academia := mensalidadeCodigo()
+	estudante := "EST-DLATR-" + uuid.NewString()[:8]
+	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
+	seedMensalidadeTurma(t, client, academia, "T-DLATR", "2025_2026", estudante, nil)
+	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
+	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{REF}' WHERE codigo_academia=$1`, academia); err != nil {
+		t.Fatal(err)
+	}
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+	if err := service.DefinirDiaLimiteCobranca(ctx, DiaLimiteCobrancaInput{CodigoAcademia: academia, DiaLimite: 10}, "fpp-test", "admin", "127.0.0.1"); err != nil {
+		t.Fatalf("DefinirDiaLimiteCobranca falhou: %v", err)
+	}
+	transport := &capturingAppyPayTransport{}
+	service.SetHTTPClient(&http.Client{Transport: transport})
+
+	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(pendentes) == 0 {
+		t.Fatal("esperava pelo menos uma mensalidade pendente")
+	}
+	// ano_letivo 2025_2026 já terminou por completo antes de "agora" (o
+	// mês mais tardio possível, Julho/2026, já passou) — todo mês
+	// pendente aqui está necessariamente em atraso.
+	alvo := pendentes[0]
+
+	_, err = service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
+		CodigoEstudante: estudante, CodigoAcademia: academia,
+		Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
+		MetodoPagamento: "REF",
+	}, estudante, "estudante", "127.0.0.1")
+	if err != nil {
+		t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
+	}
+	pi := transport.paymentInfo()
+	if _, ok := pi["dueDate"]; ok {
+		t.Fatalf("mensalidade em atraso não deveria enviar dueDate, obteve paymentInfo=%#v", pi)
+	}
+}
+
+// TestIntegrationDiaLimiteCobrancaMatriculaNuncaEnviaDueDate prova que
+// IniciarPagamentoMatricula, que nunca preenche
+// gerarCobrancaInput.DataLimitePagamento, continua sem enviar dueDate
+// mesmo quando a academia tem um dia_limite de MENSALIDADE configurado —
+// os dois fluxos são independentes.
+func TestIntegrationDiaLimiteCobrancaMatriculaNuncaEnviaDueDate(t *testing.T) {
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	client := integrationClient(t)
+	service := NewService(client)
+	ctx := context.Background()
+
+	academia := mensalidadeCodigo()
+	if err := service.DefinirDiaLimiteCobranca(ctx, DiaLimiteCobrancaInput{CodigoAcademia: academia, DiaLimite: 10}, "fpp-test", "admin", "127.0.0.1"); err == nil {
+		t.Fatal("esperava falha: academia ainda não existe em projection_academias")
+	}
+	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
+	if err := service.DefinirDiaLimiteCobranca(ctx, DiaLimiteCobrancaInput{CodigoAcademia: academia, DiaLimite: 10}, "fpp-test", "admin", "127.0.0.1"); err != nil {
+		t.Fatalf("DefinirDiaLimiteCobranca falhou: %v", err)
+	}
+	codigo := seedMatriculaPendente(t, client, academia, 750)
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+	transport := &capturingAppyPayTransport{}
+	service.SetHTTPClient(&http.Client{Transport: transport})
+
+	_, err := service.IniciarPagamentoMatricula(ctx, MatriculaPagamentoInput{
+		CodigoSolicitacao: codigo, MetodoPagamento: "REF",
+	}, "127.0.0.1")
+	if err != nil {
+		t.Fatalf("IniciarPagamentoMatricula falhou: %v", err)
+	}
+	pi := transport.paymentInfo()
+	if _, ok := pi["dueDate"]; ok {
+		t.Fatalf("matrícula nunca deveria enviar dueDate, obteve paymentInfo=%#v", pi)
+	}
+}
+
+// TestIntegrationDiaLimiteCobrancaDefinirRemoverCicloCompleto cobre o
+// ciclo de vida do comando administrativo em si (sem passar por
+// gerarCobranca): definir, redefinir (o valor mais recente vence),
+// remover, e um segundo remove sem nada vigente falhando com ErrNotFound.
+func TestIntegrationDiaLimiteCobrancaDefinirRemoverCicloCompleto(t *testing.T) {
+	client := integrationClient(t)
+	service := NewService(client)
+	ctx := context.Background()
+	academia := mensalidadeCodigo()
+	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
+
+	if err := service.DefinirDiaLimiteCobranca(ctx, DiaLimiteCobrancaInput{CodigoAcademia: academia, DiaLimite: 5}, "fpp-test", "admin", "127.0.0.1"); err != nil {
+		t.Fatalf("primeira definição falhou: %v", err)
+	}
+	dia, ok, err := service.diaLimiteEfetivo(ctx, academia)
+	if err != nil || !ok || dia != 5 {
+		t.Fatalf("diaLimiteEfetivo = %d,%t,%v — queria 5,true,nil", dia, ok, err)
+	}
+
+	if err := service.DefinirDiaLimiteCobranca(ctx, DiaLimiteCobrancaInput{CodigoAcademia: academia, DiaLimite: 20}, "fpp-test", "admin", "127.0.0.1"); err != nil {
+		t.Fatalf("redefinição falhou: %v", err)
+	}
+	if dia, ok, err = service.diaLimiteEfetivo(ctx, academia); err != nil || !ok || dia != 20 {
+		t.Fatalf("diaLimiteEfetivo após redefinição = %d,%t,%v — queria 20,true,nil", dia, ok, err)
+	}
+
+	if err := service.RemoveDiaLimiteCobranca(ctx, academia, "fpp-test", "admin", "127.0.0.1"); err != nil {
+		t.Fatalf("remove falhou: %v", err)
+	}
+	if _, ok, err = service.diaLimiteEfetivo(ctx, academia); err != nil || ok {
+		t.Fatalf("diaLimiteEfetivo após remove = ok=%t err=%v — queria ok=false", ok, err)
+	}
+
+	if err := service.RemoveDiaLimiteCobranca(ctx, academia, "fpp-test", "admin", "127.0.0.1"); !errors.Is(err, ErrNotFound) {
+		t.Fatalf("segundo remove sem nada vigente = %v, queria ErrNotFound", err)
+	}
+
+	if err := service.DefinirDiaLimiteCobranca(ctx, DiaLimiteCobrancaInput{CodigoAcademia: academia, DiaLimite: 32}, "fpp-test", "admin", "127.0.0.1"); err == nil {
+		t.Fatal("dia_limite=32 deveria ser rejeitado")
+	}
+	if err := service.DefinirDiaLimiteCobranca(ctx, DiaLimiteCobrancaInput{CodigoAcademia: academia, DiaLimite: 0}, "fpp-test", "admin", "127.0.0.1"); err == nil {
+		t.Fatal("dia_limite=0 deveria ser rejeitado")
+	}
+}
```

### 3.4 Novo arquivo `internal/finance/mensalidade_dia_limite_test.go`

Criar com exatamente este conteúdo:

```go
package finance

import "testing"

// Estes testes cobrem dataLimiteMensalidade isoladamente (sem PostgreSQL):
// o cálculo do fim do dia de um dia-limite de mensalidade, incluindo o
// "clamp" para o último dia real do mês quando dia_limite exceder o número
// de dias desse mês/ano (ex.: 31 em fevereiro, com e sem ano bissexto).

func TestDataLimiteMensalidadeDiaDentroDoMes(t *testing.T) {
	got := dataLimiteMensalidade(2026, 9, 15)
	want := "2026-09-15T23:59:59"
	if got.In(diaLimiteZone).Format("2006-01-02T15:04:05") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
}

func TestDataLimiteMensalidadeClampFevereiroComum(t *testing.T) {
	// 2026 não é bissexto: fevereiro tem 28 dias.
	got := dataLimiteMensalidade(2026, 2, 31)
	want := "2026-02-28T23:59:59"
	if got.In(diaLimiteZone).Format("2006-01-02T15:04:05") != want {
		t.Fatalf("got %v, want %s (clamp para o último dia de fevereiro/2026)", got, want)
	}
}

func TestDataLimiteMensalidadeClampFevereiroBissexto(t *testing.T) {
	// 2028 é bissexto: fevereiro tem 29 dias.
	got := dataLimiteMensalidade(2028, 2, 31)
	want := "2028-02-29T23:59:59"
	if got.In(diaLimiteZone).Format("2006-01-02T15:04:05") != want {
		t.Fatalf("got %v, want %s (clamp para o último dia de fevereiro/2028, bissexto)", got, want)
	}
}

func TestDataLimiteMensalidadeClampMesDe30Dias(t *testing.T) {
	got := dataLimiteMensalidade(2026, 9, 31) // setembro tem 30 dias
	want := "2026-09-30T23:59:59"
	if got.In(diaLimiteZone).Format("2006-01-02T15:04:05") != want {
		t.Fatalf("got %v, want %s (clamp para o último dia de setembro)", got, want)
	}
}

func TestDataLimiteMensalidadeDezembroParaJaneiro(t *testing.T) {
	// Garante que o cálculo do "último dia do mês" (mes+1, dia 0) não
	// vaza para o ano seguinte quando mes=12.
	got := dataLimiteMensalidade(2026, 12, 31)
	want := "2026-12-31T23:59:59"
	if got.In(diaLimiteZone).Format("2006-01-02T15:04:05") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
}

func TestDataLimiteMensalidadeNuncaMudaDeMes(t *testing.T) {
	// Nenhuma combinação de (mes, dia_limite=1..31) pode produzir uma data
	// fora do mês pedido — só o clamp para dentro do mesmo mês é permitido.
	for mes := 1; mes <= 12; mes++ {
		for dia := 1; dia <= 31; dia++ {
			got := dataLimiteMensalidade(2026, mes, dia)
			if int(got.Month()) != mes {
				t.Fatalf("dataLimiteMensalidade(2026, %d, %d) = %v, mês mudou para %d", mes, dia, got, int(got.Month()))
			}
			if got.Year() != 2026 {
				t.Fatalf("dataLimiteMensalidade(2026, %d, %d) = %v, ano mudou para %d", mes, dia, got, got.Year())
			}
		}
	}
}
```

---

## 4. Resumo do que cada peça faz (para revisão, não para re-decidir nada)

1. **Migration 112**: cria `financeiro_mensalidade_dia_limite` (fatos), `financeiro_mensalidade_dia_limite_remocoes` (remoções) e a view `financeiro_mensalidade_dia_limite_atual` — mesmo padrão de event sourcing já usado por `financeiro_mensalidade_inicio_cobranca` (ver migration 109), mas **sem** dimensão de ano letivo: um único valor vale por academia.
2. **`aggregates.DiaLimiteCobrancaDefinido`/`Removido`**: dois novos tipos de evento, e a whitelist central em `internal/db/safe_queries.go` atualizada para aceitá-los (sem isso, `record`/`recordMensalidade` rejeitaria o evento com "tipo de evento inválido").
3. **`internal/projections/financeiro_projection.go`**: dois novos `case` no handler de projeção, inserindo nas tabelas da migration 112 — mesmo padrão exato do `case "MesInicioCobrancaDefinido"`/`"MesInicioCobrancaRemovido"` logo acima.
4. **`mensalidade.go`**:
   - `DiaLimiteCobrancaInput{CodigoAcademia, DiaLimite}`.
   - `Service.DefinirDiaLimiteCobranca`/`RemoveDiaLimiteCobranca`/`validateDiaLimiteCobranca` — mesmo padrão de `DefinirMesInicioCobranca`/`RemoveMesInicioCobranca` (exige que a academia exista e seja `type='private'`; rejeita `dia_limite` fora de 1-31).
   - `Service.diaLimiteEfetivo(ctx, academia) (dia int, ok bool, err error)` — `ok=false` quando a academia nunca configurou (ou removeu), sinal para `gerarCobranca` não enviar `dueDate`.
   - `dataLimiteMensalidade`/`diaLimiteZone` — ver seção 2.5/2.6.
   - `IniciarPagamentoMensalidades`: passa a rastrear o mês mais antigo (por data real) entre os selecionados e, se a academia tiver `dia_limite` configurado, calcula `dataLimite` e a passa em `gerarCobrancaInput.DataLimitePagamento`.
5. **`cobranca_geracao.go`**:
   - Novo campo `DataLimitePagamento *time.Time` em `gerarCobrancaInput`, documentado como "só REF, só preenchido por mensalidade, nunca por matrícula".
   - `gerarCobranca` só define `info["dueDate"]` quando: método é REF **e** a data ainda está no futuro **e** `refDueDateEnabled()` está ligada (ver seção 2.4).
   - `refDueDateEnabled()` — o interruptor de segurança.
6. **`internal/finance/appypay.go`** (não incluído no diff desta tarefa — já parte do diff da tarefa **70**, que deve ser aplicada em conjunto ou antes): `validateCharge` relaxada para aceitar `paymentInfo` do REF contendo só `dueDate`. **Atenção**: como a tarefa 70 também mexe em `appypay.go`, se 70 ainda não tiver sido aplicada quando esta tarefa (71) for executada, aplicar manualmente este trecho extra em `validateCharge` antes de prosseguir:

```go
	if strings.HasPrefix(m, "REF") && len(in.PaymentInfo) > 0 {
		// Duas formas válidas de paymentInfo para REF:
		//  1. Só dueDate (o caso introduzido na tarefa 71): a referência
		//     continua gerada pelo gateway (AppyPay escolhe
		//     referenceNumber), só o prazo de expiração é customizado —
		//     ver gerarCobranca em cobranca_geracao.go e o comentário sobre
		//     a hipótese ainda não confirmada contra o ambiente real da
		//     AppyPay.
		//  2. Os três campos completos (referenceNumber+dueDate+nib): a
		//     forma "referência gerada pelo comerciante" documentada pela
		//     AppyPay. Nenhum chamador atual usa esta forma — mantida por
		//     integridade caso um chamador futuro precise dela.
		_, hasDueDateOnly := in.PaymentInfo["dueDate"].(string)
		if hasDueDateOnly && len(in.PaymentInfo) == 1 {
			// válido: só dueDate.
		} else {
			for _, k := range []string{"referenceNumber", "dueDate", "nib"} {
				value, ok := in.PaymentInfo[k].(string)
				if !ok || strings.TrimSpace(value) == "" {
					return fmt.Errorf("REF com paymentInfo exige %s (ou apenas dueDate sozinho)", k)
				}
			}
		}
	}
```

Isto substitui o bloco existente (antes desta tarefa):

```go
	if strings.HasPrefix(m, "REF") && len(in.PaymentInfo) > 0 {
		for _, k := range []string{"referenceNumber", "dueDate", "nib"} {
			value, ok := in.PaymentInfo[k].(string)
			if !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf("REF com paymentInfo exige %s", k)
			}
		}
	}
```

7. **`internal/handlers/mensalidade_handlers.go`** + **`cmd/server/main.go`**: dois novos handlers HTTP (`DefinirDiaLimiteCobranca`/`RemoverDiaLimiteCobranca`) e duas novas rotas (`POST`/`DELETE /financeiro/mensalidades/dia-limite-cobranca`) — mesmo padrão de `inicio-cobranca`. **Não há rota GET dedicada** para consultar o valor vigente — mesmo padrão de `inicio-cobranca`, que também não tem.
8. **Testes de integração novos** (`cobranca_geracao_integration_test.go`): cobrem (a) o caso feliz — mês futuro recebe `dueDate` correto quando a flag está ligada; (b) mês em atraso nunca recebe `dueDate`; (c) matrícula nunca recebe `dueDate` mesmo com a academia tendo configurado um; (d) o ciclo completo definir/redefinir/remover/rejeitar valores inválidos; (e) a flag desligada por padrão nunca envia `dueDate`, mesmo para um mês futuro válido.
9. **`mensalidade_dia_limite_test.go`** (novo, testes unitários puros sem Postgres): cobre `dataLimiteMensalidade` — dia dentro do mês, clamp em fevereiro comum, clamp em fevereiro bissexto, clamp em mês de 30 dias, dezembro não vaza para o ano seguinte, e uma checagem exaustiva (todos os meses × todos os dias 1-31) de que o mês/ano do resultado nunca muda em relação ao pedido.

---

## 5. Validação já executada por Claude (com PostgreSQL 16 real)

```
$ go build ./...                    # sem erros
$ go vet ./...                      # sem avisos
$ gofmt -l .                        # sem arquivos com formatação incorreta
$ go test ./... -count=1            # com banco de dados recriado do zero antes de cada execução

ok  	spuri/cmd/server
ok  	spuri/internal/db
ok  	spuri/internal/domain/aggregates
ok  	spuri/internal/finance
ok  	spuri/internal/handlers
ok  	spuri/internal/middleware
ok  	spuri/internal/projections
ok  	spuri/internal/services
ok  	spuri/internal/storage
ok  	spuri/internal/utils
```

Validado especificamente, entre outros:

```
$ go test ./internal/finance/... -run "TestIntegrationDiaLimite" -v
--- PASS: TestIntegrationDiaLimiteCobrancaEnviaDueDateParaMesFuturo
--- PASS: TestIntegrationDiaLimiteCobrancaOmiteDueDateParaMesAtrasado
--- PASS: TestIntegrationDiaLimiteCobrancaMatriculaNuncaEnviaDueDate
--- PASS: TestIntegrationDiaLimiteCobrancaDefinirRemoverCicloCompleto
--- PASS: TestIntegrationDiaLimiteCobrancaDesligadaPorPadraoMesmoComMesFuturo
```

### 5.1 O que o Codex deve rodar (sem Postgres)

```
go build ./...
go vet ./...
gofmt -l .
go test ./internal/finance/... -run "TestDataLimiteMensalidade" -v
```

Estes últimos (`TestDataLimiteMensalidade*`) não dependem de Postgres. Se o ambiente do Codex tiver Postgres disponível, rodar também `go test ./internal/finance/... -run "TestIntegrationDiaLimite" -v` com as variáveis de ambiente descritas na tarefa 70, seção 5.1 — devem passar todos os 5 testes listados acima.

---

## 6. Fora de escopo

- **Implementar o fluxo "referência gerada pelo comerciante" completo** (gerar `referenceNumber` na Spuri, obter e configurar um `nib` real) — depende de uma decisão de negócio (NIB único da Spuri vs. um por academia) que só Fredy pode tomar; se a hipótese da seção 2.4 (dueDate sozinho) se confirmar como inválida contra a AppyPay real, este será o próximo passo, como uma tarefa própria.
- **Ligar `APPYPAY_REF_DUEDATE_ENABLED=1` em produção** — só depois da confirmação manual descrita na seção 2.4.
- **Endpoint GET para consultar o dia-limite vigente de uma academia** — não existe também para `inicio-cobranca` (mesmo precedente); pode ser adicionado depois se fizer falta.
- **Qualquer mudança em GPO/GPO_QR/matrícula** — este campo nunca se aplica a esses fluxos.
- **Frontend** — coberto pela tarefa **72**, documento separado, no repositório `spuripainel`.
- **Atualizar `Documentação da API.md`** — como o envio real do `dueDate` está desligado por padrão e a rota nova (`dia-limite-cobranca`) é de baixo risco, esta tarefa não exige atualização obrigatória da documentação da API antes de ser considerada concluída, mas é recomendável documentar as duas novas rotas (`POST`/`DELETE /financeiro/mensalidades/dia-limite-cobranca`) no mesmo padrão de `inicio-cobranca` assim que possível.

## 7. Critérios de aceite

1. Migration 112 (ou renumerada, se necessário) aplicada e a view `financeiro_mensalidade_dia_limite_atual` funcionando.
2. Todos os diffs/arquivos da seção 3 aplicados exatamente como descritos, incluindo o trecho de `validateCharge` da seção 4 item 6 (verificar se a tarefa 70 já trouxe isso antes de duplicar).
3. `go build ./...`, `go vet ./...` e `gofmt -l .` limpos.
4. Todos os testes do checklist 5.1 passando.
5. Se Postgres estiver disponível: os 5 testes `TestIntegrationDiaLimite*` da seção 5 passando, mais a suíte inteira sem regressão.
6. Confirmado que `APPYPAY_REF_DUEDATE_ENABLED` permanece desligado por padrão (nenhum valor default diferente de vazio/`"0"` em nenhum lugar do código ou de configuração de deploy).
7. Nenhum arquivo fora da lista da seção 3 (mais o ajuste de `appypay.go` do item 6 da seção 4, se aplicável) alterado.

### Procedimento de conclusão

Ao finalizar:

1. Atualizar o título interno deste documento para `# Permitir que a academia defina um dia-limite de pagamento de mensalidade, aplicado como prazo de expiração (dueDate) nas cobranças REF (feito)`;
2. Alterar o front matter para `status: feito` e adicionar `concluido: <data>`;
3. Mover este arquivo para `docs/Tarefas feitas/`;
4. Registrar explicitamente, no PR ou no commit, que `APPYPAY_REF_DUEDATE_ENABLED` continua desligado por padrão e que a confirmação manual contra a AppyPay real (seção 2.4) ainda está pendente — para não ser esquecida.
