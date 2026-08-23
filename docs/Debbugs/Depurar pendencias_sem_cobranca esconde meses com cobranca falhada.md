---
data: 2026-08-23
status: causa_raiz_confirmada_correcao_pronta_para_execucao
auditor: Claude (orquestrador) — depuração com PostgreSQL 16 real em sandbox
tarefa_correcao: docs/Lista de Tarefas/63 - Listar meses com cobranca falhada em pendencias_sem_cobranca.md
relacionado_a: docs/Tarefas feitas/62 - Corrigir N+1 de PendenciasSemCobranca em GET financeiro-cobrancas com ano_letivo.md
---

# Depuração — `pendências_sem_cobranca` some meses que têm uma cobrança FALHADA, não só os pagos

## Como o problema foi relatado

Depois da tarefa 62 (correção do N+1), Fredy testou `GET /financeiro/cobrancas?...&ano_letivo=2020_2021&mes=9...` no ambiente de teste (`spuri-backend-teste.onrender.com`) e reparou que `pendencias_sem_cobranca` voltava `[]` para setembro, enquanto outros meses da mesma academia voltavam normalmente. A resposta trazia:

```json
{
  "cobrancas": [{
    "status": "falhada",
    "codigo_estudante": "SJS1125",
    "mensalidades": [
      { "ano_letivo": "2020_2021", "mes": 1 },
      { "ano_letivo": "2020_2021", "mes": 9 }
    ]
  }],
  "pendencias_sem_cobranca": [],
  "total": 1
}
```

Ou seja: setembro **tinha, sim**, uma cobrança — uma tentativa (GPO_QR) que cobria janeiro e setembro juntos, numa única transação, que **falhou**. Reproduzi esse cenário exato com PostgreSQL real (ver seção 2) e confirmei: não é um bug de escopo (a exclusão continua sendo por estudante, não vaza para outros estudantes do mesmo mês) — é o critério de exclusão em si que está errado. Perguntei a Fredy se a intenção era essa (uma tentativa falhada contar como "já tem cobrança", escondendo o mês) ou se `pendências_sem_cobranca` deveria listar qualquer mês ainda não **pago**, falha ou não. Resposta: **"exatamente isso, é suposto listar tudo."**

## Causa raiz

`PendenciasSemCobranca` (e `PendenciasSemCobrancaEstudante`, o mesmo caminho para um único estudante) excluíam um mês de duas formas:

1. `Estado != EstadoPendente` — a fonte correta: vem de `financeiro_mensalidade_obrigacoes_eventos`, que só grava um evento `"paga"` quando a AppyPay **confirma** o pagamento (via webhook — ver o `case` de confirmação em `internal/projections/financeiro_projection.go`, linha ~207) ou um evento `"anulada"`/`"reativada"` quando a academia anula/reativa manualmente a obrigação (`AnularObrigacoesMensalidade`/`ReativarObrigacoesMensalidade`, `internal/finance/mensalidade.go`). Esta fonte está correta e não muda.
2. `cobrancasExistentesMensalidade` — uma segunda verificação, mais ampla e errada: qualquer linha em `financeiro_mensalidade_cobrancas` já excluía o mês. Essa tabela é escrita a **cada evento do ciclo de vida** de uma tentativa de cobrança — não só quando dá certo. Confirmei em `internal/projections/financeiro_projection.go` (linha 231): o mesmo `upsertMensalidadeCobrancas` roda para `CobrancaAppyPaySolicitada`, `CobrancaAppyPayCriada`, `CobrancaAppyPayFalhou`, `CobrancaAppyPayConsultada`, `CobrancaAppyPayCancelada`, `QRCodeAppyPaySolicitado`, `QRCodeAppyPayGerado`, `QRCodeAppyPayFalhou` — ou seja, uma tentativa que nem chegou a sair do provedor já grava essa linha, para sempre (`ON CONFLICT DO NOTHING`, nunca é removida).

O critério 2 é o problema: ele confunde "já houve uma tentativa" com "já está resolvido". Uma cobrança falhada não resolve nada — o estudante continua devendo — mas o mês desaparecia de toda visão agregada da academia mesmo assim.

## 2. Reprodução com PostgreSQL real

Recriei o cenário exato do relato: uma academia com 2 estudantes no mesmo `ano_letivo`, um deles (equivalente ao `SJS1125`) com uma cobrança falhada cobrindo 2 meses (jan+set) numa única transação, o outro sem nenhuma tentativa:

```
pendencias_sem_cobranca para mes=9: 1 resultado(s)
  -> estudante=ESTREPRO2 mes=9 estado=pendente
```

Isso confirmou: o estudante com a tentativa falhada foi corretamente excluído (pelo critério 2, o errado) e o outro, sem tentativa nenhuma, apareceu — ou seja, **não é** um bug de vazamento entre estudantes (não esconde a academia inteira), é exatamente o critério 2 fazendo o que foi escrito para fazer: esconder qualquer mês com pelo menos uma tentativa registrada, mesmo falhada. Isso bate 100% com o relatado.

## 3. Correção

Removido o critério 2 (`cobrancasExistentesMensalidade`) das duas funções que o usavam. `financeiro_mensalidade_cobrancas` continua existindo e sendo escrita normalmente — ela permanece a fonte usada por `chargeIDsEscopoMensalidade` para vincular cobranças de mensalidade ao escopo na **listagem normal** de cobranças (`ListCobrancas`), o que é um propósito diferente (mostrar a cobrança que já existe, ligada ao estudante certo) e não muda.

**Arquivos alterados** (sobre o estado atual do `main`, que já tem a tarefa 62 aplicada):
1. `internal/finance/mensalidade_pendencias.go` — remove a função `cobrancasExistentesMensalidade` (fica sem nenhum chamador) e as duas chamadas a ela, em `PendenciasSemCobranca` e `PendenciasSemCobrancaEstudante`. `PendenciasSemCobrancaEstudante` fica mais simples: como `ListMensalidades` já devolve `Estado` corretamente calculado por mês, a função passa a só filtrar por `Estado == EstadoPendente`, sem nenhuma consulta adicional.
2. `internal/finance/mensalidade_pendencias_integration_test.go` — o teste que verificava o comportamento antigo (`TestIntegrationPendenciasSemCobrancaExcluiQuandoJaExisteTentativa`) tinha a expectativa **invertida** em relação à decisão de produto; foi reescrito com o nome e a expectativa corretos, e 3 testes novos garantem que os dois critérios de exclusão que continuam válidos (pago, anulado — e que reativar volta a listar) não regridem, mais 1 teste espelhando a mesma verificação no caminho por estudante único.

Nenhuma outra função foi tocada: `escopoMensalidadeEstudantes`, `chargeIDsEscopoMensalidade`, `estadosObrigacaoBatch` (tarefa 62), `ListMensalidades`, `estadoObrigacao`, `precedenciaEstado`, `mesInicioEfetivo`, `resolveConfiguracao` — todas continuam exatamente como estavam.

## 4. Validação executada (PostgreSQL 16 e Go 1.24 reais)

- Clonei `main` de novo, do zero (`fredypdp/spuri-backend`), e confirmei que já tem a tarefa 62 aplicada (`mensalidade_pendencias_batch.go` presente, byte-a-byte idêntico ao que validei na tarefa 62; `docs/Tarefas feitas/62 - ...md` presente com `status: concluido`).
- Apliquei os 2 arquivos desta tarefa **sobre esse clone limpo** (não sobre o meu ambiente de trabalho, que tinha histórico acumulado de outras investigações).
- `go build ./...`, `go vet ./...`, `gofmt -l` nos 2 arquivos — todos limpos.
- `go test ./internal/finance/...` com PostgreSQL 16 real (`RUN_POSTGRES_INTEGRATION=1`): os 11 testes de `PendenciasSemCobranca`/`ListCobrancas` passam, incluindo os 4 novos:
  - `TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma` — mês nunca tentado E mês com tentativa falhada aparecem os dois.
  - `TestIntegrationPendenciasSemCobrancaExcluiMesesPagos` — mês com evento `"paga"` continua excluído.
  - `TestIntegrationPendenciasSemCobrancaExcluiMesesAnuladosEIncluiReativados` — mês anulado continua excluído; reativado volta a aparecer.
  - `TestIntegrationPendenciasSemCobrancaEstudanteIncluiMesComTentativaFalhada` — mesma verificação no caminho por estudante único.
- As mesmas 9 falhas pré-existentes e não relacionadas (`FINANCE_ENCRYPTION_KEY` ausente neste sandbox) continuam aparecendo, sem nenhuma nova.

## 5. O que o Codex precisa fazer

Tudo já implementado, testado e validado com PostgreSQL real. Seguir `docs/Lista de Tarefas/63 - Listar meses com cobranca falhada em pendencias_sem_cobranca.md` mecanicamente.
