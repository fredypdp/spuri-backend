---
criado: 2026-08-15 12:05
origem: Auditoria profunda do módulo financeiro pós-tarefa 37, conduzida por Claude (Anthropic) com ambiente real (Go 1.24 + PostgreSQL 16), executando build, vet e toda a suíte de testes de integração repetidamente contra banco limpo.
status: feito
depende_de: "37 - Correção pós-implementação da tarefa 33 (bug crítico de mensalidades e outros).md"
---

# Correção de bugs críticos de produção descobertos na auditoria pós-tarefa 37 (módulo financeiro)

## Prompt recomendado para executar a atualização

```
Leia por completo o arquivo "docs/Lista de Tarefas/38 - Correção de bugs críticos de produção descobertos na
auditoria pós-tarefa 37 (módulo financeiro).md". Ele contém 9 correções já totalmente especificadas e
validadas (diffs exatos, arquivo e trecho a substituir). Não é necessário planejar, investigar causa raiz ou
decidir a abordagem — tudo isso já foi feito e confirmado experimentalmente contra PostgreSQL real. Aplique as
9 correções na ordem em que aparecem, exatamente como especificado em cada "Substituir" / "Por", e então
execute a seção "Checklist de aceitação" ao final do documento, na ordem, sem pular nenhum passo. Se qualquer
comando do checklist falhar, pare e reporte o erro — não prossiga para o próximo item nem tente uma correção
diferente da especificada aqui sem antes reportar.
```

## Contexto

A tarefa 37 corrigiu 4 problemas identificados numa auditoria anterior (migration de `status VARCHAR(50)`, bug
de meses do ano letivo em `ListMensalidades`/`mesInicioEfetivo`, mock `appyPayMockTransport` no ramo `GET`, e
a ausência de um teste HTTP de integração cobrindo o fluxo completo do webhook). Todas as 4 correções da
tarefa 37 foram **revalidadas nesta auditoria e confirmadas corretas** — nenhuma delas precisa de ajuste.

Esta tarefa nasce de uma auditoria mais profunda, feita **depois** da tarefa 37, que foi além da leitura de
código: foi montado um ambiente real (Go 1.24 + PostgreSQL 16 local) e executados repetidamente, contra banco
limpo, `go build ./...`, `go vet ./...`, e toda a suíte `go test ... -run TestIntegration -v` dos pacotes
`internal/finance` e `internal/handlers` — os dois únicos pacotes do repositório com testes de integração
Postgres reais. Cada falha encontrada foi isolada em banco limpo, instrumentada com prints de depuração
temporários e, em dois casos, com `log_statement = 'all'` do PostgreSQL para capturar a query SQL exata que
falhava. As 9 correções abaixo foram então aplicadas nesse mesmo ambiente e a suíte completa foi executada
**8 vezes consecutivas contra banco recém-criado, com sucesso total em todas elas** (0 falhas), incluindo
`go test ./...` do repositório inteiro (todos os pacotes, não só finance/handlers).

**Nenhuma destas correções foi commitada ainda.** O ambiente onde foram validadas era um sandbox temporário;
o repositório real em `github.com/fredypdp/spuri-backend` continua com os bugs. É isso que o Codex deve
corrigir.

Três dos nove bugs são de **produção real** (afetam o comportamento do sistema quando implantado, não só os
testes) e são extremamente graves: juntos, eles tornam **impossível concluir o pagamento de qualquer taxa de
matrícula** e fazem com que **o sistema nunca registre uma mensalidade como paga mesmo quando o pagamento foi
processado com sucesso pela AppyPay**. Os outros seis são bugs em código de teste (alguns pré-existentes da
tarefa 33, outros introduzidos pela própria tarefa 37) que mascaravam os bugs de produção acima — cada
correção de produção, ao "destravar" um teste, expôs o próximo bug de teste na cadeia, até a suíte inteira
ficar verde.

---

# 1. [CRÍTICO — PRODUÇÃO] `pq.Array` ausente ao ler colunas `TEXT[]` — pagamento de matrícula sempre falha

## 1.1 Causa raiz confirmada

Quatro consultas SQL fazem `Scan` direto de uma coluna PostgreSQL `TEXT[]` para um `[]string` do Go, sem
envolver o destino em `pq.Array(...)`. O driver `lib/pq` **não** sabe converter automaticamente um array
Postgres para `*[]string` sem esse wrapper — o resultado é sempre um erro de scan, nunca um valor. Isso foi
confirmado ao vivo: instrumentando `IniciarPagamentoMatricula` com um print temporário do erro, obteve-se
exatamente:

```
sql: Scan error on column index 3, name "metodos_pagamento_matricula": unsupported Scan, storing driver.Value type []uint8 into type *[]string
```

Como esse erro nunca é `nil`, o código trata SEMPRE como "solicitação não disponível para pagamento de
matrícula" — mesmo quando a solicitação está perfeitamente válida. **Isso significa que, hoje, em produção,
nenhuma taxa de matrícula pode ser paga, em nenhuma academia, nunca.** O mesmo padrão quebra `ResolveMatriculaConfiguracao`,
que é chamado durante a aprovação de uma solicitação (`internal/handlers/solicitacao_matricula_handlers.go`)
sempre que a academia tem uma taxa de matrícula configurada — nesse caso, a aprovação inteira falha com HTTP 500.

O padrão correto (`pq.Array(&campo)`) já é usado com sucesso em várias outras partes do próprio código, por
exemplo em `internal/projections/solicitacao_matricula_projection.go` e em `internal/finance/mensalidade.go`
na função `metodosPagamentoMensalidade` (que usa `unnest()` no SQL, uma alternativa igualmente válida). Isso
confirma que a ausência de `pq.Array` nestes 4 pontos é uma omissão, não uma escolha deliberada.

## 1.2 Correção — `internal/finance/matricula.go`

**Passo 1 — adicionar o import.** Localizar o bloco de imports no topo do arquivo:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)
```

Substituir por:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)
```

**Passo 2 — função `ListMatriculaConfiguracoes`.** Localizar:

```go
		if err := rows.Scan(&v.Nivel, &v.AnoAcademico, &curso, &v.Valor, &v.MetodosPagamento, &v.VigenteEm); err != nil {
```

Substituir por:

```go
		if err := rows.Scan(&v.Nivel, &v.AnoAcademico, &curso, &v.Valor, pq.Array(&v.MetodosPagamento), &v.VigenteEm); err != nil {
```

**Passo 3 — função `ResolveMatriculaConfiguracao`.** Localizar:

```go
	err := s.client.DB().QueryRowContext(ctx, `SELECT valor::float8,metodos_pagamento,vigente_em,curso_id FROM financeiro_matricula_configuracoes WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM NULLIF($4,'')::uuid ORDER BY vigente_em DESC,event_id DESC LIMIT 1`, strings.TrimSpace(academia), strings.ToLower(strings.TrimSpace(nivel)), strings.TrimSpace(ano), optionalString(curso)).Scan(&v.Valor, &v.MetodosPagamento, &v.VigenteEm, &raw)
```

Substituir por (a única mudança é `&v.MetodosPagamento` → `pq.Array(&v.MetodosPagamento)`):

```go
	err := s.client.DB().QueryRowContext(ctx, `SELECT valor::float8,metodos_pagamento,vigente_em,curso_id FROM financeiro_matricula_configuracoes WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM NULLIF($4,'')::uuid ORDER BY vigente_em DESC,event_id DESC LIMIT 1`, strings.TrimSpace(academia), strings.ToLower(strings.TrimSpace(nivel)), strings.TrimSpace(ano), optionalString(curso)).Scan(&v.Valor, pq.Array(&v.MetodosPagamento), &v.VigenteEm, &raw)
```

**Passo 4 — função `IniciarPagamentoMatricula`.** Localizar:

```go
	err := s.client.DB().QueryRowContext(ctx, `SELECT codigo_academia,status,valor_matricula::float8,metodos_pagamento_matricula FROM projection_solicitacoes_matricula WHERE codigo_solicitacao=$1`, in.CodigoSolicitacao).Scan(&academia, &status, &valor, &metodos)
```

Substituir por (a única mudança é `&metodos` → `pq.Array(&metodos)`):

```go
	err := s.client.DB().QueryRowContext(ctx, `SELECT codigo_academia,status,valor_matricula::float8,metodos_pagamento_matricula FROM projection_solicitacoes_matricula WHERE codigo_solicitacao=$1`, in.CodigoSolicitacao).Scan(&academia, &status, &valor, pq.Array(&metodos))
```

## 1.3 Correção — `internal/finance/mensalidade.go`

**Passo 1 — adicionar o import.** Localizar:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)
```

Substituir por:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)
```

**Passo 2 — função `ListMensalidadeConfiguracoes`.** Localizar:

```go
		if err := rows.Scan(&v.Nivel, &v.AnoAcademico, &curso, &v.Valor, &v.MesFimCobranca, &v.MetodosPagamento, &v.VigenteEm); err != nil {
```

Substituir por:

```go
		if err := rows.Scan(&v.Nivel, &v.AnoAcademico, &curso, &v.Valor, &v.MesFimCobranca, pq.Array(&v.MetodosPagamento), &v.VigenteEm); err != nil {
```

## 1.4 Não fazer

Não alterar `metodosPagamentoMensalidade` (usa `unnest()`, já está correto) nem nenhuma outra função de
`mensalidade.go`/`matricula.go` além das 4 listadas acima. Foi feita uma varredura sistemática de todo o
módulo financeiro comparando cada `Scan`/`QueryRow` contra o schema real das colunas `TEXT[]` em
`migrations/104_financeiro_mensalidades.sql`, `migrations/105_financeiro_mensalidades_pagamento_estudante.sql`
e `migrations/106_financeiro_matricula.sql` — estes são os únicos 4 pontos afetados.

---

# 2. [CRÍTICO — PRODUÇÃO] `ON CONFLICT` sem constraint correspondente — anular/reativar mensalidade e confirmação de pagamento via webhook sempre falham

## 2.1 Causa raiz confirmada

A migration `migrations/105_financeiro_mensalidades_pagamento_estudante.sql` trocou a chave primária da
tabela `financeiro_mensalidade_obrigacoes_eventos` de `event_id` sozinho para uma chave composta
`(event_id, codigo_estudante, codigo_academia, ano_letivo, mes)`, e criou também um índice único parcial
`uq_fin_mensalidade_pagamento_por_cobranca` em `(charge_id, codigo_estudante, codigo_academia, ano_letivo, mes)
WHERE charge_id IS NOT NULL`. O `internal/projections/financeiro_projection.go`, porém, nunca foi atualizado
para refletir essa mudança e continua usando as constraints antigas em dois `INSERT ... ON CONFLICT`.

Isso foi confirmado ligando `log_statement = 'all'` no PostgreSQL e capturando o erro exato do servidor:

```
ERROR:  there is no unique or exclusion constraint matching the ON CONFLICT specification
```

Reproduzido e corrigido manualmente via `psql` antes de aplicar a correção no código, confirmando que:

- `ON CONFLICT (event_id) DO NOTHING` não corresponde a nenhuma constraint (a PK agora é composta) —
  corrigir para `ON CONFLICT (event_id, codigo_estudante, codigo_academia, ano_letivo, mes) DO NOTHING`.
- `ON CONFLICT (charge_id, codigo_estudante, codigo_academia, ano_letivo, mes) DO NOTHING` não corresponde a
  nenhuma constraint porque a constraint real (`uq_fin_mensalidade_pagamento_por_cobranca`) é um índice único
  **parcial** (`WHERE charge_id IS NOT NULL`), e o PostgreSQL exige que o predicado apareça explicitamente no
  próprio `ON CONFLICT` para poder inferir um índice parcial como árbitro — corrigir adicionando
  `WHERE charge_id IS NOT NULL` à cláusula.

**Impacto em produção:** todo `AnularObrigacoesMensalidade` e `ReativarObrigacoesMensalidade` falha sempre
(o primeiro INSERT já falha). Mais grave ainda: a confirmação de pagamento de mensalidade via webhook
(evento `MensalidadesCobrancaConfirmada`) também falha sempre — ou seja, **um estudante pode pagar a
mensalidade com sucesso pela AppyPay e o sistema nunca vai registrar esse pagamento em
`financeiro_mensalidade_obrigacoes_eventos`**, a tabela de onde `estadoObrigacao`/`ListMensalidades` leem se
um mês está pago. O aluno pagaria de verdade e o sistema continuaria mostrando o mês como pendente
indefinidamente.

Foi feita uma varredura de **todos** os `ON CONFLICT` do módulo financeiro (`internal/projections/financeiro_projection.go`
e `internal/finance/appypay.go`) comparando cada um contra a constraint real correspondente no banco
(`\d <tabela>` via psql, com todas as 115 migrations aplicadas). Apenas estes dois estão quebrados — todos os
outros (`financeiro_matricula_configuracoes`, `financeiro_mensalidade_configuracoes`,
`financeiro_mensalidade_inicio_cobranca`, `financeiro_credenciais_appypay`, `financeiro_cobrancas`,
`financeiro_webhooks_recebidos`, `financeiro_segredos_appypay`, `financeiro_cobrancas_reservas`) continuam
corretos e não devem ser alterados.

## 2.2 Correção — `internal/projections/financeiro_projection.go`

**Primeira ocorrência** (dentro do `case "ObrigacaoMensalidadeAnulada", "ObrigacaoMensalidadeReativada", "MensalidadePaga":`).
Localizar:

```go
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,motivo,ocorrido_em) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoEstudante, in.CodigoAcademia, in.AnoLetivo, in.Mes, tipo, in.Motivo, e.OccurredAt)
```

Substituir por:

```go
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,motivo,ocorrido_em) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9) ON CONFLICT (event_id, codigo_estudante, codigo_academia, ano_letivo, mes) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoEstudante, in.CodigoAcademia, in.AnoLetivo, in.Mes, tipo, in.Motivo, e.OccurredAt)
```

**Segunda ocorrência** (dentro do `case "MensalidadesCobrancaConfirmada":`). Localizar:

```go
			if _, err = tx.Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,charge_id,ocorrido_em) VALUES ($1,$2,$3,$4,$5,$6,'paga',$7,$8) ON CONFLICT (charge_id, codigo_estudante, codigo_academia, ano_letivo, mes) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoEstudante, in.CodigoAcademia, mes.AnoLetivo, mes.Mes, chargeID, e.OccurredAt); err != nil {
```

Substituir por:

```go
			if _, err = tx.Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,charge_id,ocorrido_em) VALUES ($1,$2,$3,$4,$5,$6,'paga',$7,$8) ON CONFLICT (charge_id, codigo_estudante, codigo_academia, ano_letivo, mes) WHERE charge_id IS NOT NULL DO NOTHING`, e.EventID, e.AggregateID, in.CodigoEstudante, in.CodigoAcademia, mes.AnoLetivo, mes.Mes, chargeID, e.OccurredAt); err != nil {
```

---

# 3. [CRÍTICO — PRODUÇÃO] Dois tipos de evento do fluxo de matrícula ausentes da whitelist `validEventTypes`

## 3.1 Causa raiz confirmada

`internal/db/safe_queries.go` mantém um mapa `validEventTypes` que é a whitelist central de tipos de evento
permitidos a serem gravados no ledger (`spuri_ledger`). Qualquer tentativa de gravar um evento cujo
`EventType` não esteja neste mapa falha com `"tipo de evento inválido: <nome>"`.

O agregado `SolicitacaoMatricula` (`internal/domain/aggregates/solicitacao_matricula.go`) emite 6 tipos de
evento distintos: `SolicitacaoMatriculaCriada`, `SolicitacaoMatriculaAprovada`, `SolicitacaoMatriculaReprovada`,
`SolicitacaoMatriculaCancelada`, `SolicitacaoMatriculaAprovadaPendentePagamento` e
`SolicitacaoMatriculaVinculada`. Comparando essa lista com `validEventTypes`, **os dois últimos estão
ausentes**. Isso foi confirmado ao vivo, instrumentando temporariamente o handler do webhook: mesmo depois de
corrigir os bugs 1 e 2 acima, a aprovação de uma solicitação com taxa de matrícula configurada continuava
falhando, agora com:

```
erro ao salvar evento: tipo de evento inválido: SolicitacaoMatriculaAprovadaPendentePagamento
```

E, corrigido esse, o passo seguinte do mesmo fluxo (confirmação do pagamento via webhook, que efetiva o
vínculo do estudante) falhava com:

```
erro ao salvar evento: tipo de evento inválido: SolicitacaoMatriculaVinculada
```

**Impacto em produção:** mesmo depois de corrigidos os bugs 1 e 2, a aprovação de uma solicitação de matrícula
numa academia com taxa de matrícula configurada **continuaria falhando** ao tentar gravar o evento no ledger
— e, mesmo que isso fosse contornado manualmente, a etapa final do fluxo (confirmar o pagamento e vincular o
estudante após o webhook de sucesso da AppyPay) **também falharia**. Ou seja: este é o bug mais profundo dos
três — estava escondido atrás do bug 1, e só foi descoberto depois de corrigi-lo.

`internal/projections/solicitacao_matricula_projection.go` já trata corretamente ambos os `case` para estes
dois tipos de evento (`handleAprovadaPendentePagamento` e `handleVinculada`) — o problema é exclusivamente a
ausência na whitelist de `internal/db/safe_queries.go`.

## 3.2 Correção — `internal/db/safe_queries.go`

Localizar o bloco (seção `// ── Solicitação de Matrícula`):

```go
	"SolicitacaoMatriculaCriada":                         true,
	"SolicitacaoMatriculaAprovada":                       true,
	"SolicitacaoMatriculaReprovada":                      true,
	"SolicitacaoMatriculaCancelada":                      true,
```

Substituir por:

```go
	"SolicitacaoMatriculaCriada":                         true,
	"SolicitacaoMatriculaAprovada":                       true,
	"SolicitacaoMatriculaAprovadaPendentePagamento":      true,
	"SolicitacaoMatriculaReprovada":                      true,
	"SolicitacaoMatriculaCancelada":                      true,
	"SolicitacaoMatriculaVinculada":                      true,
```

(As duas linhas novas podem ficar em qualquer posição dentro do bloco — a ordem não importa para o
funcionamento, apenas para legibilidade. O importante é que as duas chaves `"SolicitacaoMatriculaAprovadaPendentePagamento": true`
e `"SolicitacaoMatriculaVinculada": true` passem a existir no mapa.)

## 3.3 Nota lateral — fora de escopo desta tarefa

Durante a varredura sistemática de todos os tipos de evento emitidos por `internal/domain/aggregates/*.go`
contra `validEventTypes`, foram encontrados **mais dois** tipos de evento ausentes da whitelist, mas que **não
pertencem ao módulo financeiro**: `FaltaCorrigida` e `NotaCorrigida` (do módulo de notas/faltas). Não foram
investigados nem corrigidos aqui — ficam fora do escopo desta tarefa, que é especificamente o módulo
financeiro. Recomenda-se abrir uma tarefa separada para investigar o impacto real desses dois antes de
corrigi-los (não foi confirmado se são alcançáveis em produção ou se são código morto).

---

# 4. [Teste] NIF gerado a partir de hex de UUID viola `check_academia_nif_10_digits`

## 4.1 Causa raiz confirmada

`seedAcademiaParaMatriculaWebhook`, em `internal/handlers/financeiro_handlers_integration_test.go` (adicionado
pela tarefa 37, seção 4), gera o NIF com `strings.ReplaceAll(uuid.NewString(), "-", "")[:10]` — os primeiros
10 caracteres hexadecimais de um UUID, que podem conter `a`-`f`. A constraint
`check_academia_nif_10_digits` (migration 080) exige exatamente 10 dígitos numéricos
(`nif ~ '^[0-9]{10}$'`). Reproduzido 3 vezes consecutivas em banco limpo, com falha nas 3 (a chance de um UUID
aleatório ter 10 caracteres hexadecimais consecutivos todos numéricos é praticamente nula — a probabilidade
teórica de qualquer substring aceitável é da ordem de `10^-2` a `10^-3`, e na prática nunca ocorreu). Ou seja:
o teste que a tarefa 37 adicionou para validar o fluxo do webhook **nunca passou de verdade** — o item do
checklist da tarefa 37 que dizia "novo teste HTTP passa" não foi validado contra banco real antes de ser
marcado como concluído.

O padrão correto já existe no próprio repositório, em `internal/finance/mensalidade_integration_test.go`,
função `seedMensalidadeAcademia`: filtra apenas os dígitos de um UUID com `strings.Map`, em vez de pegar os
primeiros N caracteres hexadecimais brutos. Isso garante, com probabilidade praticamente 1, pelo menos 10
dígitos entre os 32 caracteres hexadecimais de um UUID (esperado ~20 dígitos).

## 4.2 Correção — `internal/handlers/financeiro_handlers_integration_test.go`

Localizar, dentro de `seedAcademiaParaMatriculaWebhook`:

```go
	_, err := client.DB().Exec(`INSERT INTO projection_academias
		(id,nivel,nome,nif,codigo_academia,senha_hash,provincia,endereco,nivel_escolar,status,cursos,anos_academicos,type,ano_letivo,created_at)
		VALUES ($1,'escola','Academia webhook',$2,$3,'hash','LUA','endereco','fundamental','ativo','[]'::jsonb,'["1_ano_fundamental"]'::jsonb,'private','2026_2027',CURRENT_TIMESTAMP)`,
		uuid.New(), strings.ReplaceAll(uuid.NewString(), "-", "")[:10], codigo)
```

Substituir por:

```go
	_, err := client.DB().Exec(`INSERT INTO projection_academias
		(id,nivel,nome,nif,codigo_academia,senha_hash,provincia,endereco,nivel_escolar,status,cursos,anos_academicos,type,ano_letivo,created_at)
		VALUES ($1,'escola','Academia webhook',$2,$3,'hash','LUA','endereco','fundamental','ativo','[]'::jsonb,'["1_ano_fundamental"]'::jsonb,'private','2026_2027',CURRENT_TIMESTAMP)`,
		uuid.New(), strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, uuid.NewString())[:10], codigo)
```

---

# 5. [Teste] Telefone gerado a partir de hex de UUID viola formato de 9 dígitos

## 5.1 Causa raiz confirmada

`seedSolicitacaoMatriculaPendenteComLedger`, no mesmo arquivo (também adicionado pela tarefa 37, seção 4),
constrói `telefone := "244" + codigo[3:]` e `telefoneResp := "923" + codigo[3:]`, onde `codigo[3:]` são 8
caracteres hexadecimais brutos de um UUID. Isso produz strings de 11 caracteres, que:

1. Nunca têm o comprimento certo — `utils.ValidatePhone` (`internal/utils/validation.go`) exige exatamente 9
   dígitos (`^[0-9]{9}$`), sem prefixo de país; e
2. Frequentemente contêm letras `a`-`f`.

Reproduzido ao vivo com o erro exato do log da aplicação:

```
📱 [ValidatePhone] Validando telefone: 2448693ed3c
❌ [ValidatePhone] Formato inválido: 2448693ed3c
```

Este helper é usado **apenas** por `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` (o teste
novo da tarefa 37) — não é um bug de produção, é outro defeito no próprio teste novo.

## 5.2 Correção — `internal/handlers/financeiro_handlers_integration_test.go`

**Passo 1.** Adicionar esta função auxiliar nova, logo depois de `seedAcademiaParaMatriculaWebhook` e antes de
`seedSolicitacaoMatriculaPendenteComLedger`:

```go
func geraDigitos(n int) string {
	digitos := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, uuid.NewString())
	for len(digitos) < n {
		digitos += "0"
	}
	return digitos[:n]
}
```

**Passo 2.** Dentro de `seedSolicitacaoMatriculaPendenteComLedger`, localizar:

```go
	telefone := "244" + codigo[3:]
	telefoneResp := "923" + codigo[3:]
```

Substituir por:

```go
	telefone := geraDigitos(9)
	telefoneResp := geraDigitos(9)
```

---

# 6. [Teste] `SolicitacoesSemelhantes` nulo viola `NOT NULL` em `solicitacoes_semelhantes`

## 6.1 Causa raiz confirmada

Ainda em `seedSolicitacaoMatriculaPendenteComLedger`, a chamada a `sol.Criar(...)` passa `nil` como último
argumento (`semelhantes []string`). O handler da projeção
(`internal/projections/solicitacao_matricula_projection.go`, função `handleCriada`) grava esse campo com
`pq.Array(payload.SolicitacoesSemelhantes)` — e `pq.Array` de um slice `nil` produz `NULL`, o que viola a
constraint `NOT NULL` da coluna `solicitacoes_semelhantes`. Confirmado ao vivo:

```
pq: null value in column "solicitacoes_semelhantes" of relation "projection_solicitacoes_matricula" violates not-null constraint
```

Isto **não é um bug de produção**: o handler HTTP real de criação de solicitação
(`internal/handlers/solicitacao_matricula_handlers.go`) sempre obtém `semelhantes` a partir de
`FindSemelhantesPendentes(...)`, que inicializa `out := []string{}` e portanto nunca retorna `nil` — só o
seed de teste passava `nil` diretamente.

## 6.2 Correção — `internal/handlers/financeiro_handlers_integration_test.go`

Localizar, dentro de `seedSolicitacaoMatriculaPendenteComLedger`:

```go
	if err := sol.Criar(codigo, academia, "Estudante Webhook", "feminino", time.Date(2017, 1, 2, 0, 0, 0, 0, time.UTC), &email, &telefone, &telefoneResp, &bi, &biResp, &ano, nil, nil, nil, nil, docs, nil); err != nil {
```

Substituir por (apenas o último argumento muda, de `nil` para `[]string{}`):

```go
	if err := sol.Criar(codigo, academia, "Estudante Webhook", "feminino", time.Date(2017, 1, 2, 0, 0, 0, 0, time.UTC), &email, &telefone, &telefoneResp, &bi, &biResp, &ano, nil, nil, nil, nil, docs, []string{}); err != nil {
```

---

# 7. [Teste] `eventID` do webhook divergente de `payload["id"]` — condição nunca satisfeita

## 7.1 Causa raiz confirmada

Em `internal/finance/appypay_integration_test.go`, `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito`
chama:

```go
service.AcceptWebhook(context.Background(), "REF", "evt-"+uuid.NewString(), WebhookOwner{...}, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Success"})
```

Ou seja, passa um `eventID` sintético (`"evt-"+uuid...`) diferente do valor em `payload["id"]`. Isso não
reflete o fluxo real: no handler HTTP de produção (`internal/handlers/financeiro_handlers.go`, função
`ReceberWebhookAppyPay`), o `eventID` passado a `AcceptWebhook` é **sempre** extraído do próprio payload via
`webhookID(payload)` (que lê `payload["id"]` primeiro) — nunca é um valor independente. Dentro de
`AcceptWebhook` (`internal/finance/appypay.go`), a cobrança é localizada com
`s.loadCharge(ctx, eventID)`, que busca por `id::text=$1 OR provider_charge_id=$1 OR merchant_transaction_id=$1`.
Como o teste usa um `eventID` que não corresponde a nada na tabela `financeiro_cobrancas`, `loadCharge` nunca
encontra a cobrança, e todo o bloco que registraria o evento `CobrancaAppyPayConflitoPosCancelamento` é
silenciosamente pulado — daí a asserção final do teste (`conflitos != 1`) falhar sempre.

Confirmado comparando com `TestIntegrationAcceptWebhookIsIdempotent` (mesmo arquivo), que já segue o padrão
correto: `payload := map[string]any{"id": eventID, ...}` — o mesmo valor em ambos os lugares.

Isto é exclusivamente um bug de teste (o código de produção `AcceptWebhook`/`ReceberWebhookAppyPay` está
correto e não deve ser alterado).

## 7.2 Correção — `internal/finance/appypay_integration_test.go`

Localizar, dentro de `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito`:

```go
	accepted, err := service.AcceptWebhook(context.Background(), "REF", "evt-"+uuid.NewString(), WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Success"})
```

Substituir por (apenas o terceiro argumento muda):

```go
	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Success"})
```

---

# 8. [Teste, pré-existente da tarefa 33] Reuso do parâmetro `$2` em dois contextos de tipo — `inconsistent types deduced`

## 8.1 Causa raiz confirmada

`seedSolicitacaoMatriculaParaBusca`, em `internal/handlers/financeiro_handlers_integration_test.go` (este
helper já existia antes da tarefa 37 — é usado por `TestIntegrationBuscaPublicaMatriculaExigeDoisCamposENaoExibePagamento`),
usa `$2` (o valor de `codigo`) tanto diretamente na coluna `codigo_solicitacao` (`VARCHAR(11)`) quanto dentro
de duas concatenações SQL, `'BI-' || $2` e `'BI-RESP-' || $2`, para preencher `bilhete_identidade`
(`VARCHAR(50)`). O protocolo estendido do PostgreSQL exige que cada parâmetro posicional tenha um único tipo
inferido de forma consistente em toda a query; usar o mesmo `$2` em dois contextos de tipo/coluna diferentes
faz a inferência falhar. Confirmado ao vivo:

```
pq: inconsistent types deduced for parameter $2
```

Isto é 100% determinístico (não depende de dados aleatórios) e não afeta código de produção — é uma consulta
SQL exclusiva do teste.

## 8.2 Correção — `internal/handlers/financeiro_handlers_integration_test.go`

Localizar, dentro de `seedSolicitacaoMatriculaParaBusca`:

```go
	_, err := client.DB().Exec(`INSERT INTO projection_solicitacoes_matricula
		(id,codigo_solicitacao,codigo_academia,nome,genero,data_nascimento,email,telefone,telefone_encarregado,bilhete_identidade,bilhete_identidade_encarregado,ano_escolar_fundamental,status,documentos,codigo_estudante_gerado,valor_matricula,metodos_pagamento_matricula,created_at,updated_at)
		VALUES ($1,$2,'ACA-BUSCA','Estudante buscável','feminino','2012-01-02',$3,$4,'923000000','BI-' || $2,'BI-RESP-' || $2,'6_ano_fundamental','aprovada_pendente_pagamento_matricula','{}'::jsonb,'EST0001',1250,ARRAY['REF'],CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, uuid.New(), codigo, email, telefone)
```

Substituir por (os valores `'BI-' || $2` e `'BI-RESP-' || $2` são pré-computados em Go e passados como novos
parâmetros `$5` e `$6`, em vez de reutilizar `$2` dentro do SQL):

```go
	_, err := client.DB().Exec(`INSERT INTO projection_solicitacoes_matricula
		(id,codigo_solicitacao,codigo_academia,nome,genero,data_nascimento,email,telefone,telefone_encarregado,bilhete_identidade,bilhete_identidade_encarregado,ano_escolar_fundamental,status,documentos,codigo_estudante_gerado,valor_matricula,metodos_pagamento_matricula,created_at,updated_at)
		VALUES ($1,$2,'ACA-BUSCA','Estudante buscável','feminino','2012-01-02',$3,$4,'923000000',$5,$6,'6_ano_fundamental','aprovada_pendente_pagamento_matricula','{}'::jsonb,'EST0001',1250,ARRAY['REF'],CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, uuid.New(), codigo, email, telefone, "BI-"+codigo, "BI-RESP-"+codigo)
```

---

# 9. [Teste] Contador `nextID` do mock reinicia por instância — colisão de `provider_charge_id` entre testes irmãos

## 9.1 Causa raiz confirmada

`appyPayMockTransport`, em `internal/finance/appypay_integration_test.go`, gera IDs de cobrança fictícios com
um contador de instância (`t.nextID++`, começando em 0). Cada função de teste cria sua **própria** instância
do mock (`&appyPayMockTransport{status: "..."}`), então o contador de cada uma recomeça em 1. Como todos os
testes de integração do pacote `internal/finance` compartilham o **mesmo** banco PostgreSQL dentro de uma
única execução de `go test` (o banco não é recriado entre funções de teste), duas funções de teste diferentes
que cada uma cria sua "primeira" cobrança acabam gerando o mesmo `provider_charge_id`
(`"provider-charge-1"`), violando a constraint única `ux_financeiro_cobrancas_provider_id`.

Isto ficou mascarado enquanto os bugs 1 e 3 bloqueavam `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito`
antes de ele conseguir criar qualquer cobrança real. Ao corrigir os bugs 1 e 3, esse teste passou a de fato
criar uma cobrança, e sua colisão com `TestIntegrationCancelChargeAndLateSuccessConflict` (que roda a seguir,
por ordem alfabética de declaração no arquivo) passou a se manifestar de forma determinística.

## 9.2 Correção — `internal/finance/appypay_integration_test.go`

**Passo 1.** Localizar a definição do struct:

```go
type appyPayMockTransport struct {
	status string
	mu     sync.Mutex
	nextID int
}
```

Substituir por:

```go
type appyPayMockTransport struct {
	status string
}
```

**Passo 2.** Localizar o método:

```go
func (t *appyPayMockTransport) providerID(kind string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	return fmt.Sprintf("provider-%s-%d", kind, t.nextID)
}
```

Substituir por:

```go
func (t *appyPayMockTransport) providerID(kind string) string {
	return fmt.Sprintf("provider-%s-%s", kind, uuid.NewString())
}
```

**Passo 3.** Como `sync.Mutex` deixa de ser usado em qualquer lugar do arquivo, remover o import `"sync"` do
bloco de imports no topo do arquivo (ele ficará sem uso e `go vet`/`go build` vai rejeitar o arquivo se ele
permanecer). Localizar:

```go
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
```

Substituir por:

```go
	"net/http"
	"os"
	"strings"
	"testing"
```

(Confirme antes de remover que nenhum outro trecho do arquivo usa `sync` — nesta auditoria, `sync` só era
usado pelo campo `mu` que está sendo removido no Passo 1.)

---

# Ordem de execução recomendada

Aplique as correções na ordem 1 → 9 listada acima. Essa é a mesma ordem em que os bugs foram descobertos
nesta auditoria (cada correção destrava a descoberta da seguinte), e seguir essa ordem faz o diagnóstico de
qualquer desvio inesperado ser mais simples de rastrear. Depois de aplicar todas as 9, rode a íntegra do
checklist abaixo de uma vez.

---

# Checklist de aceitação

Execute cada comando abaixo, na ordem, contra um PostgreSQL real acessível (local ou de teste — nunca contra
produção). Se qualquer um falhar, pare e reporte o erro exato antes de prosseguir.

1. **Build e vet limpos:**
   ```
   go build ./...
   go vet ./...
   ```
   Ambos devem terminar sem nenhuma saída de erro (exit code 0).

2. **Suíte de integração do pacote `finance`, 5 vezes seguidas, cada uma contra banco recém-criado**
   (recrie o banco de teste — `DROP DATABASE` + `CREATE DATABASE`, ou equivalente — antes de cada uma das 5
   execuções; use as variáveis de ambiente que o projeto já espera, por exemplo `RUN_POSTGRES_INTEGRATION=1`,
   `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE=disable`,
   `APPYPAY_RESOURCE=integration-resource`, `FINANCE_ENCRYPTION_KEY` com pelo menos 32 caracteres, `ENV=test`):
   ```
   go test -count=1 ./internal/finance/... -run TestIntegration -v
   ```
   As 5 execuções devem terminar com `ok` e **zero** `--- FAIL`. Preste atenção especial a estes três testes,
   que foram os que falhavam antes desta correção:
   - `TestIntegrationMatriculaPagamentoFixaValorImpedeDuplicidadeECancelaEmCascata`
   - `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito`
   - `TestIntegrationCancelChargeAndLateSuccessConflict`
   - `TestIntegrationMensalidadeAnularEReativar`

3. **Suíte de integração do pacote `handlers`, 5 vezes seguidas, cada uma contra banco recém-criado** (mesmas
   variáveis de ambiente do passo 2):
   ```
   go test -count=1 ./internal/handlers/... -run TestIntegration -v
   ```
   As 5 execuções devem terminar com `ok` e **zero** `--- FAIL`. Preste atenção especial a:
   - `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula`
   - `TestIntegrationBuscaPublicaMatriculaExigeDoisCamposENaoExibePagamento`

4. **Suíte de testes unitários do repositório inteiro** (sem `RUN_POSTGRES_INTEGRATION`, para confirmar que
   nada fora do escopo desta tarefa foi quebrado — em especial `internal/db`, que teve
   `safe_queries.go` alterado, um arquivo compartilhado por todo o projeto):
   ```
   go test ./...
   ```
   Todos os pacotes devem terminar com `ok` (os pacotes sem arquivos de teste aparecem como `[no test files]`,
   o que é esperado e não é uma falha).

5. **Confirmação manual do diff.** Rode `git diff --stat` e confirme que exatamente estes 6 arquivos de
   produção/teste foram alterados (não deve haver nenhum outro arquivo fora desta lista, e nenhuma mudança em
   `go.mod`/`go.sum`):
   - `internal/db/safe_queries.go`
   - `internal/finance/appypay_integration_test.go`
   - `internal/finance/matricula.go`
   - `internal/finance/mensalidade.go`
   - `internal/handlers/financeiro_handlers_integration_test.go`
   - `internal/projections/financeiro_projection.go`

Se todos os 5 itens passarem, a tarefa está concluída. Mova este arquivo de `docs/Lista de Tarefas/` para
`docs/Tarefas feitas/`, atualize o front-matter (`status: feito`) e registre no início do arquivo, em uma
frase, a data e confirmação de que os 5 itens do checklist foram executados com sucesso.
