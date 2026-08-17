---
criado: 2026-08-17
origem: auditoria da tarefa 47 ("Correção de dois problemas de backend do módulo de pagamentos"), feita por
  Claude (Anthropic) por leitura completa do commit `594e238` (branch main, PR #544) e comparação byte a byte
  contra o documento original de orientação, mais leitura do restante do backend relevante
  (internal/handlers/mensalidade_handlers.go, internal/middleware/auth.go, migrations/, "Documentação da
  API.md") e validação ao vivo da SQL nova contra um PostgreSQL 16 real (115 migrations aplicadas do zero no
  ambiente de orquestração).
status: pendente
depende_de: "47 - Correção de dois problemas de backend do módulo de pagamentos (listagem de cobranças e QR
  Code na resposta de pagamento).md" (já implementada e mesclada — commit 594e238, PR #544)
---

# Auditoria da tarefa 47 + endpoint de consulta do estudante + atualização da documentação da API

## Prompt recomendado para executar a tarefa

```
Leia por completo o arquivo "docs/Lista de Tarefas/48 - Auditoria da tarefa 47 + endpoint de consulta do
estudante + atualização da documentação da API.md". A seção "Resultado da auditoria" já confirma que a tarefa
47 foi implementada corretamente — não mexa em nada do que ela já entregou. O que falta está inteiramente
especificado na seção "Tarefa A" (rota nova para o estudante consultar o próprio histórico de pagamentos) e
na seção "Tarefa B" (atualizar "Documentação da API.md", que ficou sem menção às mudanças da tarefa 47 e sem
a rota nova desta tarefa). Todas as decisões de design já foram tomadas e estão documentadas com
justificativa — não é necessário planejar, investigar causa raiz ou decidir abordagem nenhuma. Toda a SQL
nova já foi validada ao vivo contra um PostgreSQL 16 real com as 115 migrations do projeto aplicadas (não é
uma suposição). Aplique as mudanças exatamente como especificado em cada "Localizar" / "Substituir por",
crie os 2 arquivos de teste exatamente como especificados, aplique as edições de documentação exatamente como
especificado, e então execute a seção "Checklist de aceitação (o que você consegue rodar)" ao final do
documento. Preste atenção especial à seção "Nota sobre validação de testes de integração" — ela explica por
que o seu ambiente não consegue confirmar sozinho que os testes de integração passam, e o que fazer a
respeito (reportar, não inventar um jeito de contornar). Se qualquer comando do checklist falhar, pare e
reporte o erro exato — não prossiga para o próximo item nem invente uma correção diferente da especificada
aqui sem antes reportar.
```

## Resultado da auditoria

A tarefa 47 (arquivo `docs/Lista de Tarefas/47 - Correção de dois problemas de backend do módulo de
pagamentos (listagem de cobranças e QR Code na resposta de pagamento).md`, implementada no commit `594e238`,
mesclado via PR #544) foi conferida **linha a linha contra o diff real do commit** — não apenas contra o
relatório do Codex. Resultado:

- **Correção do QR Code (`MensalidadePagamentoView`/`MatriculaPagamentoView` → `QRCodeResult`)**: implementada
  exatamente como especificado, nos dois arquivos (`internal/finance/mensalidade.go`,
  `internal/finance/matricula.go`). Nada a corrigir.
- **Listagem de cobranças (`ListCobrancas`, `CobrancaResumo`, `ListarCobrancasAppyPay`,
  `GET /financeiro/cobrancas`)**: implementada exatamente como especificado, em
  `internal/finance/appypay.go`, `internal/handlers/financeiro_handlers.go` e `cmd/server/main.go`. A SQL
  gerada (filtros de contexto/academia/estado/origem, paginação, derivação de `origem`/`metodo_pagamento` a
  partir do payload) foi **reexecutada linha a linha contra um PostgreSQL 16 real** (115 migrations do
  projeto aplicadas do zero) com dados de teste equivalentes aos usados nos testes de integração do próprio
  Codex — os resultados batem exatamente com o esperado. Nada a corrigir aqui também.
- **Testes:** os 4 arquivos de teste novos previstos foram criados com o conteúdo exato especificado.

**O que ficou de fora (não fazia parte do escopo original da tarefa 47, mas você pediu agora):**

1. **Estudante não consegue consultar os próprios pagamentos.** `GET /financeiro/cobrancas` está registrada
   dentro do grupo `financeiro := protected.Group("/financeiro"); financeiro.Use(middleware.RequireAcademiaOuAdmin())`
   — um estudante autenticado recebe `403` nessa rota, sempre. Mesmo que a rota estivesse aberta para
   estudante, `ListCobrancas` só filtra por `contexto_tipo`/`codigo_academia`, nunca por `codigo_estudante` —
   não daria pra um estudante ver só os pagamentos dele. E mesmo com um filtro por `codigo_estudante`, a
   cobrança da **matrícula original** do estudante nunca teria aparecido: o payload dela só tem
   `codigo_solicitacao`, nunca `codigo_estudante` (a cobrança é anterior ao registo do estudante — ver
   `internal/finance/matricula.go`, função `IniciarPagamentoMatricula`). A Tarefa A abaixo resolve os três
   pontos.
2. **`Documentação da API.md` não foi tocada pela tarefa 47.** Confirmado por busca no arquivo inteiro: zero
   menções a `GET /financeiro/cobrancas`, `ListarCobrancasAppyPay`, `CobrancaResumo` ou ao novo comportamento
   de `qrCodeArr` nas respostas de pagamento de mensalidade/matrícula. A Tarefa B abaixo resolve isso, junto
   com a documentação da rota nova da Tarefa A.

Nenhuma outra parte do backend precisa de correção. Você **não precisa reler nem revalidar** o que a tarefa 47
já entregou — já foi validado nesta auditoria.

## Nota sobre validação de testes de integração

Isto é importante e não é um formalismo: **o relatório de testes da tarefa 47 é enganoso, e o seu ambiente vai
ter o mesmo problema nesta tarefa se você não prestar atenção.**

No relatório da tarefa 47, o Codex reportou `✅ go test -count=1 ./internal/finance/...` (sem
`RUN_POSTGRES_INTEGRATION=1`) como sucesso, junto com o loop de 5 execuções falhando por `psql: command not
found` / conexão recusada. **O `✅` sem `RUN_POSTGRES_INTEGRATION=1` não prova nada sobre os testes de
integração** — todo teste cujo nome começa com `TestIntegration` chama `integrationClient(t)` /
`integrationFinanceClient(t)`, que fazem `t.Skip(...)` imediatamente se `RUN_POSTGRES_INTEGRATION` não for
`"1"`. Ou seja: esse `go test` "verde" só executou os testes puramente unitários (ex:
`TestMensalidadePagamentoViewIncludesQRCodeArr`) e **pulou silenciosamente todos os testes de integração**,
que são os únicos que realmente provam que a correção funciona contra um banco de verdade.

Sabemos, por confirmação explícita sua, que o seu ambiente:
- bloqueia `apt` (403 Forbidden) — não é possível instalar PostgreSQL;
- não tem Docker;
- não tem `psql` nem qualquer outro cliente Postgres.

**O que fazer nesta tarefa, dado isso:**

1. Rode sempre com `RUN_POSTGRES_INTEGRATION=1` definido (mesmo sabendo que vai falhar por falta de
   conexão) — isso faz os testes de integração **falharem ruidosamente com erro de conexão** em vez de serem
   pulados silenciosamente. Um erro de conexão recusada é a confirmação correta de "não executei"; um `PASS`
   sem essa variável é uma confirmação **falsa** de "passou".
2. Rode `go build ./...`, `go vet ./...` e `gofmt -l .` normalmente — esses não dependem de banco e são
   validações reais.
3. Rode `go test ./internal/finance/... ./internal/handlers/...` **sem** `RUN_POSTGRES_INTEGRATION=1` só para
   confirmar que os testes puramente unitários passam (os que não chamam `integrationClient`/
   `integrationFinanceClient`) — isso é uma confirmação parcial real, não confunda com confirmação total.
4. No seu relatório final, **não escreva `✅` ao lado de nenhum teste `TestIntegration*`** a menos que você
   tenha, de fato, uma conexão Postgres funcionando e `RUN_POSTGRES_INTEGRATION=1` setado durante aquela
   execução específica. Se não tiver, escreva explicitamente "não validado neste ambiente — ver nota sobre
   validação de testes de integração" para cada um.
5. **Eu (o orquestrador) já validei a parte que mais importava** — a SQL nova desta tarefa (a query de
   `ListCobrancasEstudante`, incluindo o vínculo `codigo_estudante_gerado`) foi executada ao vivo contra
   PostgreSQL 16 real com o schema completo do projeto, com os mesmos dados que os testes de integração desta
   tarefa usam. Isso está documentado na Tarefa A abaixo, função por função. A parte que falta validar de
   verdade é só a integração completa (Go compilando, testes rodando ponta a ponta) — é isso que eu vou fazer
   depois que você aplicar o diff e me devolver o resultado.

---

## Escopo desta tarefa (e o que NÃO fazer)

**Arquivos a modificar:**
- `internal/finance/appypay.go`
- `internal/handlers/financeiro_handlers.go`
- `cmd/server/main.go`
- `Documentação da API.md`

**Arquivos novos a criar:**
- `internal/finance/cobrancas_estudante_integration_test.go`
- `internal/handlers/financeiro_cobrancas_estudante_handlers_test.go`

**Fora do escopo — não alterar:**
- Nada do que a tarefa 47 já entregou (`ListCobrancas`, `CobrancaResumo`, `ListarCobrancasAppyPay`,
  `GET /financeiro/cobrancas`, a correção de `qrCodeArr`, e os 4 arquivos de teste da tarefa 47) — está tudo
  correto, ver "Resultado da auditoria" acima. A única alteração autorizada num arquivo já tocado pela tarefa
  47 é a extração de `scanCobrancaResumo` descrita na Tarefa A.1 abaixo (refactor sem mudança de
  comportamento, para reaproveitar a mesma lógica de derivação de campos sem duplicar código).
- Nenhuma migration. `codigo_estudante_gerado` já existe em `projection_solicitacoes_matricula` desde a
  migration do módulo de matrícula — nenhuma coluna nova é necessária.
- `docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md` e
  `docs/Lista de Tarefas/47 - ....md` — não mexer no status desses arquivos nesta tarefa; isso é decisão
  minha depois de validar o resultado final.
- `docs/Lista de Tarefas/11 - Enriquecer documentação da API com significado de cada campo.md` — é uma
  iniciativa maior e separada, não relacionada a este escopo; não mexer.

---

## Decisões de design já tomadas

1. **Rota nova dedicada, fora do grupo `financeiro`:** `GET /financeiro/cobrancas/estudante/:codigo`,
   registrada diretamente em `protected` (não dentro do grupo `financeiro` que exige
   `RequireAcademiaOuAdmin()`), exatamente ao lado de
   `protected.GET("/financeiro/mensalidades/estudante/:codigo", handlers.ConsultarMensalidadesEstudante)`.
   Alternativa descartada: abrir o grupo `financeiro` inteiro (ou só a rota `GET /financeiro/cobrancas`) para
   estudantes — rejeitada porque todas as outras rotas daquele grupo (credenciais, criar cobrança, cancelar,
   configurar mensalidade/matrícula) são exclusivas de academia/admin por design, e `GET /financeiro/cobrancas`
   não tem filtro por estudante (ver decisão 2). Duas rotas com propósitos e regras de autorização diferentes
   — igual ao par já existente `GET /financeiro/mensalidades/estudante/:codigo` (estudante/academia/admin) vs.
   o resto do grupo `financeiro` (só academia/admin).

2. **Handler e método de serviço novos e dedicados** (`ConsultarCobrancasEstudante` /
   `ListCobrancasEstudante`), em vez de reaproveitar `ListarCobrancasAppyPay`/`ListCobrancas` com parâmetros
   extras. Motivo: `ListCobrancas` já está implementada, testada e em produção com uma assinatura de 6
   argumentos posicionais — mudar essa assinatura quebraria os 3 call-sites de teste já existentes da tarefa
   47 sem necessidade. Um estudante também não filtra por `contexto_tipo`/`codigo_academia` (quer o histórico
   inteiro, cruzando academias), então a forma de filtrar é estruturalmente diferente o suficiente para
   justificar um método próprio. A única coisa que os dois métodos compartilham (derivar `CobrancaResumo` a
   partir de uma linha de `financeiro_cobrancas`) é extraída para a função `scanCobrancaResumo`, reaproveitada
   pelos dois — ver Tarefa A.1.

3. **A cobrança de matrícula é resolvida via `codigo_estudante_gerado`.** `projection_solicitacoes_matricula`
   já grava esse vínculo quando a solicitação é aprovada (usado por `seedMatriculaPendente` nos testes de
   integração já existentes, e por toda a lógica de aprovação de matrícula). A query nova faz
   `payload->>'codigo_estudante' = $1 OR payload->>'codigo_solicitacao' IN (SELECT codigo_solicitacao FROM
   projection_solicitacoes_matricula WHERE codigo_estudante_gerado = $1)` — **validada ao vivo contra
   PostgreSQL 16 real** com uma solicitação aprovada + 3 cobranças (matrícula, mensalidade paga, mensalidade
   falhada) + 1 cobrança de outro estudante: devolveu exatamente as 3 cobranças certas do estudante certo, e
   nenhuma da outra pessoa. Filtro por `estado=Success` sobre esse mesmo conjunto devolveu exatamente 2
   (matrícula + mensalidade paga), excluindo a falhada — confirmando que o filtro de estado funciona igual ao
   de `ListCobrancas`.

4. **Sem filtro de estado por padrão = todos os estados.** É o pedido explícito: "estudantes possam consultar
   todos os pagamentos que já tiveram em todos os estados". A rota aceita `estado` (repetível) como filtro
   opcional, exatamente como `GET /financeiro/cobrancas`, mas por padrão não filtra nada — pendente, falhada,
   cancelada e paga aparecem todas.

5. **Escopo por ator, reaproveitando `academiaPossuiVinculoMensalidade` e `financeAdminAllowed` já
   existentes** (nenhuma função de autorização nova é necessária):
   - **Estudante:** só pode consultar o próprio código (`:codigo` tem que bater com o `codigo_estudante` do
     `user_id` do token). Vê o histórico inteiro, sem restrição de academia — "todos os pagamentos que já
     teve", incluindo academias anteriores caso tenha trocado de escola.
   - **Academia:** só pode consultar estudante com vínculo atual ou histórico com ela (mesmo critério já
     usado por `ConsultarMensalidadesEstudante`, função `academiaPossuiVinculoMensalidade` — vínculo direto ou
     por turma histórica). **E o resultado é restrito às cobranças da própria academia** (parâmetro
     `somenteAcademia` do serviço, o mesmo padrão já usado por `ListMensalidades`/`ConsultarMensalidadesEstudante`)
     — isto é deliberado: uma academia não deve ver os pagamentos que um estudante fez a uma *outra* academia,
     mesmo que o estudante tenha vínculo histórico com as duas. **Validado ao vivo**: com o mesmo estudante
     tendo cobranças em duas academias diferentes, a consulta com `somenteAcademia` restrita a uma delas
     devolveu só a cobrança daquela academia; sem restrição (caso do estudante/admin) devolveu as duas.
   - **Admin:** precisa da permissão `"fpp"` (`financeAdminAllowed`), igual a todas as outras rotas
     financeiras administrativas. Vê o histórico inteiro do estudante, sem restrição de academia.
   - Este é exatamente o mesmo desenho de três vias já usado por `ConsultarMensalidadesEstudante`
     (`internal/handlers/mensalidade_handlers.go`) — reaproveitado quase literalmente, incluindo a ordem das
     checagens (existência do estudante → tipo de ator → autorização específica do tipo).

6. **Formato da resposta idêntico ao de `GET /financeiro/cobrancas`** (`{"cobrancas": [...], "total": N,
   "total_geral": M, "limit": L, "offset": O}`, cada item um `CobrancaResumo`) — nenhum campo novo, nenhum
   formato novo. Isso significa que qualquer cliente que já sabe interpretar a resposta da tarefa 47 já sabe
   interpretar esta.

---

## Tarefa A — `GET /financeiro/cobrancas/estudante/:codigo`

### A.1 — `internal/finance/appypay.go`: import novo + refactor + método novo

**Localizar** (bloco de imports):

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
	"github.com/lib/pq"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)
```

**Substituir por:**

```go
import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
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
	"github.com/lib/pq"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)
```

**Localizar** (o corpo do laço de `ListCobrancas`, incluindo o fechamento da função — este trecho já existe
tal como está, implementado pela tarefa 47; só o laço interno muda, a query e a paginação continuam
idênticas):

```go
	defer rows.Close()
	out := []CobrancaResumo{}
	for rows.Next() {
		var dto CobrancaResumo
		var rawPayload []byte
		if err := rows.Scan(&dto.ID, &dto.ProviderChargeID, &dto.MerchantTransactionID, &dto.ContextoTipo, &dto.CodigoAcademia, &rawPayload, &dto.AtualizadoEm); err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(rawPayload, &payload); err != nil {
			return nil, err
		}
		dto.Status, _ = payload["status"].(string)
		dto.Valor, _ = payload["amount"].(float64)
		dto.Moeda, _ = payload["currency"].(string)
		dto.Descricao, _ = payload["description"].(string)
		dto.MetodoPagamento, _ = payload["payment_method"].(string)
		if qrType, ok := payload["qr_code_type"].(string); ok && qrType != "" {
			dto.MetodoPagamento = "GPO_QR"
		}
		dto.CodigoEstudante, _ = payload["codigo_estudante"].(string)
		dto.CodigoSolicitacao, _ = payload["codigo_solicitacao"].(string)
		switch {
		case dto.CodigoSolicitacao != "":
			dto.Origem = "matricula"
		case dto.CodigoEstudante != "":
			dto.Origem = "mensalidade"
		default:
			dto.Origem = "avulsa"
		}
		if mesesRaw, ok := payload["mensalidades"]; ok && mesesRaw != nil {
			if b, err := json.Marshal(mesesRaw); err == nil {
				var meses []MensalidadeSelecaoMes
				if json.Unmarshal(b, &meses) == nil {
					dto.Mensalidades = meses
				}
			}
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
}

func canCancelCharge(row chargeRow, academia, actorType string) bool {
```

**Substituir por:**

```go
	defer rows.Close()
	out := []CobrancaResumo{}
	for rows.Next() {
		dto, err := scanCobrancaResumo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
}

// scanCobrancaResumo lê uma linha de financeiro_cobrancas (id,
// provider_charge_id, merchant_transaction_id, contexto_tipo,
// codigo_academia, payload, updated_at, nesta ordem exata) e deriva os
// campos resumidos a partir do payload persistido. Compartilhado por
// ListCobrancas e ListCobrancasEstudante para não duplicar a lógica de
// derivação de origem/método/mensalidades — extraído durante a tarefa 48 sem
// nenhuma mudança de comportamento em relação ao que ListCobrancas já fazia
// inline desde a tarefa 47.
func scanCobrancaResumo(rows *sql.Rows) (CobrancaResumo, error) {
	var dto CobrancaResumo
	var rawPayload []byte
	if err := rows.Scan(&dto.ID, &dto.ProviderChargeID, &dto.MerchantTransactionID, &dto.ContextoTipo, &dto.CodigoAcademia, &rawPayload, &dto.AtualizadoEm); err != nil {
		return CobrancaResumo{}, err
	}
	var payload map[string]any
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return CobrancaResumo{}, err
	}
	dto.Status, _ = payload["status"].(string)
	dto.Valor, _ = payload["amount"].(float64)
	dto.Moeda, _ = payload["currency"].(string)
	dto.Descricao, _ = payload["description"].(string)
	dto.MetodoPagamento, _ = payload["payment_method"].(string)
	if qrType, ok := payload["qr_code_type"].(string); ok && qrType != "" {
		dto.MetodoPagamento = "GPO_QR"
	}
	dto.CodigoEstudante, _ = payload["codigo_estudante"].(string)
	dto.CodigoSolicitacao, _ = payload["codigo_solicitacao"].(string)
	switch {
	case dto.CodigoSolicitacao != "":
		dto.Origem = "matricula"
	case dto.CodigoEstudante != "":
		dto.Origem = "mensalidade"
	default:
		dto.Origem = "avulsa"
	}
	if mesesRaw, ok := payload["mensalidades"]; ok && mesesRaw != nil {
		if b, err := json.Marshal(mesesRaw); err == nil {
			var meses []MensalidadeSelecaoMes
			if json.Unmarshal(b, &meses) == nil {
				dto.Mensalidades = meses
			}
		}
	}
	return dto, nil
}

// ListCobrancasEstudante lista TODAS as cobranças de um estudante (qualquer
// origem, qualquer estado por padrão) — usada por
// GET /financeiro/cobrancas/estudante/:codigo, a consulta do próprio
// estudante (ou de uma academia/admin autorizados) ao histórico de
// pagamentos dele. Diferente de ListCobrancas (visão de academia/admin sobre
// cobranças recebidas, filtrada por contexto/academia), esta consulta é
// centrada no estudante: ele pode ter mensalidades e matrícula em mais de
// uma academia (histórico), e o próprio estudante ou um admin FPP devem ver
// tudo.
//
// somenteAcademia, quando não nil, restringe o resultado a essa academia —
// usado quando quem chama é uma academia (só pode ver os pagamentos que o
// estudante fez à própria academia, nunca o histórico completo dele noutras
// academias). Estudante e admin FPP chamam com somenteAcademia nil. Mesmo
// padrão já usado por ListMensalidades.
//
// Inclui a cobrança de matrícula do estudante mesmo que o payload dela não
// tenha codigo_estudante (só codigo_solicitacao) — resolvido via o vínculo
// já existente em projection_solicitacoes_matricula.codigo_estudante_gerado,
// preenchido quando a solicitação é aprovada.
func (s *Service) ListCobrancasEstudante(ctx context.Context, codigoEstudante string, somenteAcademia *string, estados []string, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	if codigoEstudante == "" {
		return nil, errors.New("código do estudante é obrigatório")
	}
	where := `WHERE (payload->>'codigo_estudante' = $1 OR payload->>'codigo_solicitacao' IN (SELECT codigo_solicitacao FROM projection_solicitacoes_matricula WHERE codigo_estudante_gerado = $1))`
	args := []any{codigoEstudante}
	i := 2
	if somenteAcademia != nil {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, *somenteAcademia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estados))
		i++
	}
	var total int
	if err := s.client.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM financeiro_cobrancas "+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`SELECT id, COALESCE(provider_charge_id,''), merchant_transaction_id, contexto_tipo, COALESCE(codigo_academia,''), payload, updated_at FROM financeiro_cobrancas %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, where, i, i+1)
	args = append(args, limit, offset)
	rows, err := s.client.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CobrancaResumo{}
	for rows.Next() {
		dto, err := scanCobrancaResumo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
}

func canCancelCharge(row chargeRow, academia, actorType string) bool {
```

### A.2 — `internal/handlers/financeiro_handlers.go`: import novo + handler novo

**Localizar** (bloco de imports):

```go
package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)
```

**Substituir por:**

```go
package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)
```

**Localizar** (fim de `ListarCobrancasAppyPay`, logo antes do comentário de `CancelarCobrancaAppyPay` — este
trecho já existe tal como está, implementado pela tarefa 47; não muda, só serve de âncora para a inserção):

```go
	c.JSON(http.StatusOK, gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset})
}

// CancelarCobrancaAppyPay intentionally does not use authorizeFinanceScope:
```

**Substituir por:**

```go
	c.JSON(http.StatusOK, gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset})
}

// ConsultarCobrancasEstudante lista TODAS as cobranças (mensalidade,
// matrícula ou avulsa) já associadas a um estudante, em qualquer estado —
// diferente de ListarCobrancasAppyPay (academia/admin, dentro do próprio
// contexto), esta rota é acessível ao próprio estudante para consultar o seu
// histórico completo de pagamentos, exatamente como ConsultarMensalidadesEstudante
// já faz para as obrigações de mensalidade (mesmo desenho de autorização em
// três vias: estudante só o próprio código, academia só com vínculo e
// restrita à própria academia, admin com permissão "fpp").
func ConsultarCobrancasEstudante(c *gin.Context) {
	codigo := strings.TrimSpace(c.Param("codigo"))
	var estudanteID string
	err := getDBClient(c).DB().QueryRowContext(c.Request.Context(), `SELECT id::text FROM projection_estudantes WHERE codigo_estudante=$1`, codigo).Scan(&estudanteID)
	if err == sql.ErrNoRows {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	actorID, typ, own, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	var somenteAcademia *string
	switch typ {
	case "estudante":
		if actorID.String() != estudanteID {
			utils.RespondWithForbiddenError(c, "você só pode consultar os seus próprios pagamentos")
			return
		}
	case "academia":
		if !academiaPossuiVinculoMensalidade(c, codigo, own) {
			utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
			return
		}
		somenteAcademia = &own
	case "admin":
		if !financeAdminAllowed(c) {
			utils.RespondWithForbiddenError(c, "sem permissão financeira FPP")
			return
		}
	default:
		utils.RespondWithForbiddenError(c, "sem permissão para consultar pagamentos")
		return
	}
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, c.QueryArray("estado"), limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset})
}

// CancelarCobrancaAppyPay intentionally does not use authorizeFinanceScope:
```

(`getDBClient`, `academiaPossuiVinculoMensalidade`, `financeActor`, `financeAdminAllowed` e
`parseBoundedInt` já existem no pacote `handlers` — `getDBClient` e `academiaPossuiVinculoMensalidade` em
`internal/handlers/mensalidade_handlers.go`, `financeActor`/`financeAdminAllowed` no início deste mesmo
arquivo, `parseBoundedInt` em `internal/handlers/solicitacao_matricula_handlers.go`. Nenhum import adicional
além de `database/sql` é necessário.)

### A.3 — `cmd/server/main.go`: rota nova

**Localizar:**

```go
		// Consulta de mensalidades também é acessível ao próprio estudante; as
		// demais ações financeiras ficam no grupo academia/admin abaixo.
		protected.GET("/financeiro/mensalidades/estudante/:codigo", handlers.ConsultarMensalidadesEstudante)
		protected.POST("/financeiro/mensalidades/pagamento", handlers.IniciarPagamentoMensalidades)
```

**Substituir por:**

```go
		// Consulta de mensalidades também é acessível ao próprio estudante; as
		// demais ações financeiras ficam no grupo academia/admin abaixo.
		protected.GET("/financeiro/mensalidades/estudante/:codigo", handlers.ConsultarMensalidadesEstudante)
		protected.POST("/financeiro/mensalidades/pagamento", handlers.IniciarPagamentoMensalidades)
		// Idem para o histórico de cobranças do estudante (mensalidade, matrícula
		// e avulsas, em qualquer estado) — ver tarefa 48.
		protected.GET("/financeiro/cobrancas/estudante/:codigo", handlers.ConsultarCobrancasEstudante)
```

### A.4 — Teste novo (integração, nível `Service`): `internal/finance/cobrancas_estudante_integration_test.go`

As duas queries deste teste (com e sem `somenteAcademia`) foram **validadas ao vivo contra PostgreSQL 16
real** (115 migrations aplicadas do zero) durante esta auditoria, com o mesmo desenho de dados usado aqui.

Criar o arquivo com exatamente este conteúdo:

```go
package finance

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestIntegrationListCobrancasEstudanteIncluiMensalidadeEMatricula cobre o
// requisito de que um estudante deve conseguir consultar TODOS os pagamentos
// que já teve, em qualquer estado — incluindo a cobrança da matrícula
// original (associada por codigo_solicitacao, não por codigo_estudante,
// porque é anterior ao registo do estudante) e cobranças de mensalidade em
// qualquer estado (não só "pago").
func TestIntegrationListCobrancasEstudanteIncluiMensalidadeEMatricula(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	codigoSolicitacao := seedMatriculaPendente(t, client, academia, 750)
	codigoEstudante := "EST" + codigoSolicitacao[3:7] // mesmo cálculo de seedMatriculaPendente

	insert := func(status, estudante, solicitacao string) {
		payload := map[string]any{"status": status, "amount": 500.0, "currency": "AOA", "description": "teste", "payment_method": "REF"}
		if estudante != "" {
			payload["codigo_estudante"] = estudante
		}
		if solicitacao != "" {
			payload["codigo_solicitacao"] = solicitacao
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), integrationMerchant("EST"), academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert("Success", "", codigoSolicitacao) // cobrança da matrícula original (sem codigo_estudante)
	insert("Success", codigoEstudante, "")   // mensalidade paga
	insert("Failed", codigoEstudante, "")    // mensalidade falhada — deve aparecer também (todos os estados)
	insert("Success", "OUTRO12", "")         // de outro estudante — não deve aparecer

	res, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 {
		t.Fatalf("esperava 3 cobranças do estudante (1 matrícula + 2 mensalidade), obteve %d: %#v", res.Total, res.Cobrancas)
	}
	var temMatricula, temFalhada bool
	for _, cobranca := range res.Cobrancas {
		if cobranca.Origem == "matricula" && cobranca.CodigoSolicitacao == codigoSolicitacao {
			temMatricula = true
		}
		if cobranca.Status == "Failed" {
			temFalhada = true
		}
	}
	if !temMatricula {
		t.Fatalf("cobrança de matrícula original não apareceu na listagem: %#v", res.Cobrancas)
	}
	if !temFalhada {
		t.Fatalf("cobrança falhada não apareceu (listagem deveria incluir todos os estados por padrão): %#v", res.Cobrancas)
	}

	pagas, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, []string{"Success"}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if pagas.Total != 2 {
		t.Fatalf("filtro por estado=Success deveria devolver 2 cobranças (matrícula + mensalidade paga), obteve %d", pagas.Total)
	}
}

// TestIntegrationListCobrancasEstudanteSomenteAcademiaIsolaOutraAcademia
// cobre o isolamento por academia: quando somenteAcademia é passado (caso da
// academia autenticada, via ConsultarCobrancasEstudante), cobranças que o
// mesmo estudante fez a OUTRA academia não devem aparecer.
func TestIntegrationListCobrancasEstudanteSomenteAcademiaIsolaOutraAcademia(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academiaA := mensalidadeCodigo()
	academiaB := mensalidadeCodigo()
	estudante := "EST" + uuid.NewString()[:4]

	insert := func(academia string) {
		payload := map[string]any{"status": "Success", "amount": 500.0, "currency": "AOA", "description": "teste", "payment_method": "REF", "codigo_estudante": estudante}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), integrationMerchant("ISO"), academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert(academiaA)
	insert(academiaB)

	semRestricao, err := service.ListCobrancasEstudante(ctx, estudante, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if semRestricao.Total != 2 {
		t.Fatalf("sem somenteAcademia deveria ver as 2 cobranças (histórico completo), obteve %d", semRestricao.Total)
	}

	comRestricao, err := service.ListCobrancasEstudante(ctx, estudante, &academiaA, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comRestricao.Total != 1 || comRestricao.Cobrancas[0].CodigoAcademia != academiaA {
		t.Fatalf("com somenteAcademia=%s deveria ver só a cobrança dessa academia, obteve %#v", academiaA, comRestricao.Cobrancas)
	}
}
```

### A.5 — Teste novo (integração, nível handler HTTP): `internal/handlers/financeiro_cobrancas_estudante_handlers_test.go`

O INSERT mínimo em `projection_estudantes` usado no helper `seedEstudanteParaCobrancas` abaixo, e as duas
queries de vínculo academia↔estudante (com e sem vínculo), foram **validados ao vivo contra o mesmo
PostgreSQL 16 real**, incluindo as colunas obrigatórias exatas (`id`, `nome`, `senha_hash`, `telefone`,
`created_at` — as demais têm default) e a checagem `EXISTS` já usada por `academiaPossuiVinculoMensalidade`.

Criar o arquivo com exatamente este conteúdo:

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
)

// seedEstudanteParaCobrancas cria a linha mínima válida de
// projection_estudantes necessária para ConsultarCobrancasEstudante
// encontrar o estudante pelo código e devolve o id gerado (usado como
// user_id do ator "estudante" nos testes deste arquivo).
func seedEstudanteParaCobrancas(t *testing.T, client *db.Client, codigoEstudante, academia string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := client.DB().Exec(`INSERT INTO projection_estudantes (id,nome,codigo_estudante,senha_hash,telefone,codigo_academia,status,created_at) VALUES ($1,'Estudante de teste',$2,'hash',$3,$4,'ativo',CURRENT_TIMESTAMP)`,
		id, codigoEstudante, "923"+codigoEstudante, academia); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestIntegrationConsultarCobrancasEstudanteEstudanteVeTodosOsEstados cobre
// o requisito principal desta tarefa: o próprio estudante consulta o
// histórico completo, sem filtro de estado por padrão.
func TestIntegrationConsultarCobrancasEstudanteEstudanteVeTodosOsEstados(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "COBEST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	codigoEstudante := "ESTCOB1"
	estudanteID := seedEstudanteParaCobrancas(t, client, codigoEstudante, academia)

	insert := func(status string) {
		payload := map[string]any{"status": status, "amount": 300.0, "currency": "AOA", "description": "teste", "payment_method": "REF", "codigo_estudante": codigoEstudante}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		merchant := "COB" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), merchant, academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert("Success")
	insert("Failed")

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+codigoEstudante, nil)
	ctx.Params = gin.Params{{Key: "codigo", Value: codigoEstudante}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", estudanteID)
	ctx.Set("user_type", "estudante")

	ConsultarCobrancasEstudante(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("estudante consultando o próprio histórico = %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		TotalGeral int `json:"total_geral"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 2 {
		t.Fatalf("estudante deveria ver as 2 cobranças (todos os estados), obteve %d: %s", body.TotalGeral, recorder.Body.String())
	}
}

// TestIntegrationConsultarCobrancasEstudanteRejeitaOutroEstudante garante
// que um estudante não consegue consultar o histórico de outro, mesmo
// sabendo o código dele.
func TestIntegrationConsultarCobrancasEstudanteRejeitaOutroEstudante(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "COBEST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	codigoEstudante := "ESTCOB2"
	seedEstudanteParaCobrancas(t, client, codigoEstudante, academia)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+codigoEstudante, nil)
	ctx.Params = gin.Params{{Key: "codigo", Value: codigoEstudante}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", uuid.New()) // outro estudante, não o dono do código
	ctx.Set("user_type", "estudante")

	ConsultarCobrancasEstudante(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("outro estudante recebeu %d, queria 403: %s", recorder.Code, recorder.Body.String())
	}
}

// TestIntegrationConsultarCobrancasEstudanteAcademiaSemVinculoEProibida
// garante que uma academia sem vínculo (atual ou histórico) com o estudante
// recebe 403, mesmo sabendo o código dele.
func TestIntegrationConsultarCobrancasEstudanteAcademiaSemVinculoEProibida(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academiaDona := "COBEST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	outraAcademia := "COBEST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	codigoEstudante := "ESTCOB3"
	seedEstudanteParaCobrancas(t, client, codigoEstudante, academiaDona)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+codigoEstudante, nil)
	ctx.Params = gin.Params{{Key: "codigo", Value: codigoEstudante}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", uuid.New())
	ctx.Set("user_type", "academia")
	ctx.Set("codigo_academia", outraAcademia)

	ConsultarCobrancasEstudante(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("academia sem vínculo recebeu %d, queria 403: %s", recorder.Code, recorder.Body.String())
	}
}
```

---

## Tarefa B — Atualizar `Documentação da API.md`

O arquivo já tem uma seção 19 ("Financeiro / AppyPay") detalhada e no mesmo formato usado em todo o resto do
documento (tabela de rotas + subseções `####` por rota, com "Escopo da rota", "Proteção", tabelas de
campos, exemplo de resposta e "Regras de negócio"). Esta tarefa insere 2 subseções novas nesse mesmo formato
e corrige 2 lacunas de conteúdo já existentes (a resposta de pagamento não documentava `qrCodeArr`). Como a
inserção fica no meio da seção 19, é necessário renumerar as subseções seguintes — inclusive corrige uma
duplicação de numeração (`19.8`) que já existia no arquivo antes desta tarefa, por acidente.

Aplique os itens B.1 a B.8 **nesta ordem** (os textos de busca são todos únicos no arquivo — confirmado por
busca antes de escrever esta tarefa).

### B.1 — Regra geral do escopo financeiro

**Localizar:**

```
- Todas as rotas `/financeiro/*` exigem autenticação. As rotas de administração aceitam somente `academia` ou admin FPP; a consulta de mensalidades de um estudante também pode ser feita pelo próprio estudante autenticado.
```

**Substituir por:**

```
- Todas as rotas `/financeiro/*` exigem autenticação. As rotas de administração aceitam somente `academia` ou admin FPP; a consulta de mensalidades e a consulta do histórico de cobranças (seção 19.8) de um estudante também podem ser feitas pelo próprio estudante autenticado.
```

### B.2 — Tabela de rotas: duas linhas novas

**Localizar:**

```
| `GET` | `/financeiro/appypay/cobrancas/:id` | Consulta cobrança por id AppyPay ou `merchantTransactionId`. |
| `POST` | `/financeiro/appypay/cobrancas/:id/cancelar` | Cancela localmente uma cobrança pendente do próprio contexto. |
```

**Substituir por:**

```
| `GET` | `/financeiro/appypay/cobrancas/:id` | Consulta cobrança por id AppyPay ou `merchantTransactionId`. |
| `GET` | `/financeiro/cobrancas` | Lista cobranças (mensalidade, matrícula, avulsa) do contexto autorizado, com filtros e paginação. |
| `GET` | `/financeiro/cobrancas/estudante/:codigo` | Lista todas as cobranças de um estudante, em qualquer estado — acessível ao próprio estudante. |
| `POST` | `/financeiro/appypay/cobrancas/:id/cancelar` | Cancela localmente uma cobrança pendente do próprio contexto. |
```

### B.3 — Duas subseções novas (19.7 e 19.8), inseridas depois de 19.6

**Localizar:**

```
- Se a consulta detectar `Success` da AppyPay depois de a cobrança ter sido cancelada localmente, grava `CobrancaAppyPayConflitoPosCancelamento` e preserva o status local `cancelada`, para reconciliação manual por admin FPP.

#### 19.7 Mensalidades/propinas e pagamento pelo estudante
```

**Substituir por:**

```
- Se a consulta detectar `Success` da AppyPay depois de a cobrança ter sido cancelada localmente, grava `CobrancaAppyPayConflitoPosCancelamento` e preserva o status local `cancelada`, para reconciliação manual por admin FPP.

#### 19.7 GET /financeiro/cobrancas

**Escopo da rota:** lista cobranças (mensalidade, matrícula ou avulsa) do contexto autorizado, com paginação e filtros por estado e origem — visão de academia/admin sobre pagamentos recebidos. Para o estudante consultar o próprio histórico de pagamentos, use 19.8.

**Proteção:** autenticado + academia do próprio contexto ou admin FPP. Estudantes recebem `403` nesta rota.

**Query params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---|---|---|
| `contexto_tipo` | string | Não | Contexto financeiro consultado. Para academia autenticada é forçado para `academia`. |
| `codigo_academia` | string | Não | Academia dona das cobranças. Para academia autenticada é forçado para o código do token. |
| `estado` | string, repetível | Não | Filtra pelo texto exato (case-sensitive) persistido em `status` — mistura estados internos (`solicitada`, `criada`, `cancelada`, `falhada`) e estados crus da AppyPay (`Success`, `Pending`, `Failed`, etc). Repita o parâmetro para casar mais de um valor (`?estado=Success&estado=Pending`). |
| `tipo` | string, repetível | Não | Filtra por origem: `matricula`, `mensalidade` ou `avulsa`. |
| `limit` | inteiro | Não | Itens por página. Padrão 50, mínimo 1, máximo 1000. |
| `offset` | inteiro | Não | Deslocamento de paginação. Padrão 0. |

**Response 200:**

```json
{
  "cobrancas": [
    {
      "id": "4d2bbf53-c8c0-4c9a-a3f4-5a0f0cf988d1",
      "provider_charge_id": "APPYPAY-987654",
      "merchant_transaction_id": "P2608LDA000001",
      "contexto_tipo": "academia",
      "codigo_academia": "ACA001",
      "origem": "mensalidade",
      "status": "Success",
      "valor": 1000.00,
      "moeda": "AOA",
      "descricao": "Mensalidade outubro/2025",
      "metodo_pagamento": "GPO",
      "codigo_estudante": "EST0001",
      "mensalidades": [{"ano_letivo": "2025_2026", "mes": 10}],
      "atualizado_em": "2026-08-08T12:30:00Z"
    }
  ],
  "total": 1,
  "total_geral": 1,
  "limit": 50,
  "offset": 0
}
```

**Regras de negócio:**

- Não devolve `payment_info`, `response` (resposta crua da AppyPay) nem `qrCodeArr`; para o detalhe completo de uma cobrança específica, use 19.6.
- `origem` é derivada do payload persistido, nunca gravada separadamente: `matricula` quando a cobrança tem `codigo_solicitacao`, `mensalidade` quando tem `codigo_estudante` (e não tem `codigo_solicitacao`), `avulsa` nos demais casos.
- Ordenação sempre por `updated_at DESC` (atividade mais recente primeiro); não há campo de data de criação separado nesta listagem.
- `total` é o número de itens nesta página; `total_geral` é o total real que casa com os filtros aplicados.

#### 19.8 GET /financeiro/cobrancas/estudante/:codigo

**Escopo da rota:** lista TODAS as cobranças que um estudante já teve, em qualquer estado, academia ou origem — incluindo a cobrança da matrícula original, mesmo que ela tenha sido paga antes de o estudante existir como tal. É a visão do próprio estudante sobre o seu histórico de pagamentos. Para a visão de academia/admin sobre cobranças recebidas, use 19.7.

**Proteção:** o próprio estudante (`:codigo` deve ser o código do token), academia à qual o estudante pertence ou pertenceu (mesmo vínculo histórico de `GET /financeiro/mensalidades/estudante/:codigo`), ou admin FPP.

**Query params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---|---|---|
| `estado` | string, repetível | Não | Mesmo filtro de 19.7. Sem filtro, devolve todos os estados. |
| `limit` | inteiro | Não | Itens por página. Padrão 50, mínimo 1, máximo 1000. |
| `offset` | inteiro | Não | Deslocamento de paginação. Padrão 0. |

**Response 200:** mesma estrutura de 19.7.

**Regras de negócio:**

- Diferente de 19.7, esta consulta não aceita `contexto_tipo` nem `codigo_academia`: um estudante pode ter mensalidades e matrícula em mais de uma academia (histórico), e o histórico mostra tudo — exceto quando quem consulta é uma academia, caso em que o resultado é restrito às cobranças feitas a essa academia especificamente (uma academia nunca vê pagamentos que o estudante fez a outra academia, mesmo com vínculo histórico com as duas).
- A cobrança de matrícula é resolvida pelo vínculo `codigo_estudante_gerado`, já gravado em `projection_solicitacoes_matricula` quando a solicitação é aprovada — o payload da cobrança de matrícula em si nunca grava `codigo_estudante`, porque a cobrança é anterior ao registo do estudante.
- Sem filtro de `estado`, a listagem inclui cobranças pendentes, falhadas e canceladas, não só as pagas — intencional: o objetivo é o estudante conseguir ver tudo que já teve, não só os pagamentos concluídos.

#### 19.9 Mensalidades/propinas e pagamento pelo estudante
```

### B.4 — Nota sobre `qrCodeArr` na resposta de pagamento de mensalidade

**Localizar:**

```
A seleção deve conter o mês pendente mais antigo daquela academia; meses adicionais podem ser quaisquer outros pendentes da mesma academia. Pagos, anulados, duplicados ou meses já cobertos por cobrança aberta são recusados antes da chamada AppyPay. O valor é a soma dos preços históricos, arredondada pela regra financeira única, e a resposta traz uma única `cobranca` e os meses associados. Ao receber `Success` por consulta ou webhook, `MensalidadesCobrancaConfirmada` grava os pagamentos de todos os meses da cobrança numa única transação projetada; repetições são idempotentes.
```

**Substituir por:**

```
A seleção deve conter o mês pendente mais antigo daquela academia; meses adicionais podem ser quaisquer outros pendentes da mesma academia. Pagos, anulados, duplicados ou meses já cobertos por cobrança aberta são recusados antes da chamada AppyPay. O valor é a soma dos preços históricos, arredondada pela regra financeira única, e a resposta traz uma única `cobranca` e os meses associados. Quando `metodo_pagamento` é `GPO_QR`, `cobranca` inclui `qrCodeArr` em base64 (mesmo campo documentado em 19.5); nos demais métodos, o campo fica ausente. Ao receber `Success` por consulta ou webhook, `MensalidadesCobrancaConfirmada` grava os pagamentos de todos os meses da cobrança numa única transação projetada; repetições são idempotentes.
```

### B.5 — Nota sobre `qrCodeArr` na resposta de pagamento de matrícula

**Localizar:**

```
As rotas públicas, limitadas por IP, são `GET /solicitacao-matricula/busca` (exige ao menos dois campos exatos entre telefone, telefone do encarregado, e-mail e BIs e só devolve dados de reconhecimento), `GET /solicitacao-matricula/:codigo/status` (estado e, se pendente, valor/métodos) e `POST /solicitacao-matricula/:codigo/pagamento-matricula` (método e telefone opcional GPO). O valor cobrado é sempre o congelado na aprovação e só há uma cobrança aberta por solicitação.
```

**Substituir por:**

```
As rotas públicas, limitadas por IP, são `GET /solicitacao-matricula/busca` (exige ao menos dois campos exatos entre telefone, telefone do encarregado, e-mail e BIs e só devolve dados de reconhecimento), `GET /solicitacao-matricula/:codigo/status` (estado e, se pendente, valor/métodos) e `POST /solicitacao-matricula/:codigo/pagamento-matricula` (método e telefone opcional GPO). O valor cobrado é sempre o congelado na aprovação e só há uma cobrança aberta por solicitação. A resposta traz `cobranca` no mesmo formato do módulo financeiro (seção 19): quando `metodo_pagamento` é `GPO_QR`, inclui `qrCodeArr` em base64 (ver 19.5); nos demais métodos, o campo fica ausente.
```

### B.6 — Renumeração das subseções restantes (7 substituições, uma por linha)

Cada bloco abaixo é uma substituição isolada de uma única linha (cabeçalho `####`). Aplique as 7, em
qualquer ordem, depois de aplicar B.3 (que já renomeou a antiga 19.7 para 19.9 dentro do próprio bloco
inserido — não repita essa).

**Localizar:**
```
#### 19.8 POST /financeiro/appypay/cobrancas/:id/cancelar
```
**Substituir por:**
```
#### 19.10 POST /financeiro/appypay/cobrancas/:id/cancelar
```

**Localizar:**
```
#### 19.8 POST /webhooks/appypay/gpo
```
**Substituir por:**
```
#### 19.11 POST /webhooks/appypay/gpo
```

**Localizar:**
```
#### 19.9 POST /webhooks/appypay/ref
```
**Substituir por:**
```
#### 19.12 POST /webhooks/appypay/ref
```

**Localizar:**
```
#### 19.10 GET /financeiro/appypay/credenciais/:id/webhook-secret
```
**Substituir por:**
```
#### 19.13 GET /financeiro/appypay/credenciais/:id/webhook-secret
```

**Localizar:**
```
#### 19.11 POST /financeiro/appypay/credenciais/:id/webhook-secret/rotacionar
```
**Substituir por:**
```
#### 19.14 POST /financeiro/appypay/credenciais/:id/webhook-secret/rotacionar
```

(As duas primeiras substituições deste item corrigem, como efeito colateral, uma duplicação de numeração
`19.8` que já existia no arquivo antes desta tarefa — não é um erro introduzido por você, mas fica corrigido
de qualquer forma.)

### B.7 — Conferência final da numeração

Depois de aplicar B.1 a B.6, rode:

```
grep -n "^#### 19\." "Documentação da API.md"
```

Deve mostrar, em ordem, exatamente: `19.1`, `19.2`, `19.3`, `19.4`, `19.5`, `19.6`, `19.7`, `19.8`, `19.9`,
`19.10`, `19.11`, `19.12`, `19.13`, `19.14` — cada um aparecendo uma única vez. Se algum número faltar, se
repetir, ou vier fora de ordem, pare e reporte antes de prosseguir.

### B.8 — Índice do documento

Verifique se o índice no início de `Documentação da API.md` lista subseções individuais de outras seções
numeradas (ex.: `19.1`, `19.2`, ...) além da entrada única `19. [Financeiro / AppyPay](#19-financeiro--appypay)`.
Se listar (confira uma seção como `## 18.` ou `## 20.` para comparar o padrão), adicione entradas equivalentes
para `19.7` e `19.8` seguindo exatamente o mesmo formato das entradas irmãs já existentes, e ajuste os números
das entradas que forem renumeradas por B.6. Se o índice só tiver a entrada única de nível de seção (era o caso
confirmado nesta auditoria — busca por `19.` nas primeiras 60 linhas do arquivo encontrou só a linha 46, a
entrada única `19. [Financeiro / AppyPay]`), não é necessário nenhum ajuste aqui.

---

## Ordem de execução recomendada

1. Aplicar A.1 (`internal/finance/appypay.go`: import, refactor de `ListCobrancas`, `scanCobrancaResumo`,
   `ListCobrancasEstudante`).
2. Aplicar A.2 (`internal/handlers/financeiro_handlers.go`: import, `ConsultarCobrancasEstudante`).
3. Aplicar A.3 (`cmd/server/main.go`: rota nova).
4. Criar A.4 (`internal/finance/cobrancas_estudante_integration_test.go`) e A.5
   (`internal/handlers/financeiro_cobrancas_estudante_handlers_test.go`).
5. Rodar:
   ```
   gofmt -w internal/finance/appypay.go internal/handlers/financeiro_handlers.go cmd/server/main.go \
     internal/finance/cobrancas_estudante_integration_test.go \
     internal/handlers/financeiro_cobrancas_estudante_handlers_test.go
   ```
6. Aplicar B.1 a B.6, na ordem, em `Documentação da API.md`. Conferir com B.7. Verificar B.8.
7. Executar a checklist abaixo.

---

## Checklist de aceitação (o que você consegue rodar)

Execute cada item na ordem. Se algo falhar de um jeito que **não seja** a ausência de `psql`/conexão Postgres
já prevista na "Nota sobre validação de testes de integração", pare e reporte o erro exato.

1. **Build e vet limpos:**
   ```
   go build ./...
   go vet ./...
   ```
   Ambos devem terminar sem nenhuma saída de erro.

2. **`gofmt` não encontra nada pendente:**
   ```
   gofmt -l .
   ```
   Deve devolver vazio.

3. **Testes puramente unitários (sem `RUN_POSTGRES_INTEGRATION`) — confirmação parcial real:**
   ```
   go test ./internal/finance/... ./internal/handlers/...
   ```
   Deve terminar `ok` (os `TestIntegration*` são pulados — isso é esperado e não é uma confirmação total, ver
   a nota no início do documento).

4. **Testes com `RUN_POSTGRES_INTEGRATION=1` — tentativa, esperando falha de conexão, não pulo silencioso:**
   ```
   RUN_POSTGRES_INTEGRATION=1 DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
     DB_NAME=spuri_test DB_SSLMODE=disable APPYPAY_RESOURCE=integration-resource \
     FINANCE_ENCRYPTION_KEY=01234567890123456789012345678901 ENV=test \
     go test ./internal/finance/... ./internal/handlers/... -run TestIntegration -v
   ```
   Reporte exatamente o que aconteceu: se falhar por erro de conexão (`connection refused` ou similar), isso
   é esperado no seu ambiente — reporte isso explicitamente, não como um `FAIL` genérico. Se, por algum
   motivo, uma conexão Postgres estiver disponível no seu ambiente para esta tarefa e os testes rodarem de
   verdade, reporte o resultado real (`PASS`/`FAIL` por teste) — nesse caso trate como uma validação completa
   de verdade.

5. **Conferência da renumeração da documentação:**
   ```
   grep -n "^#### 19\." "Documentação da API.md"
   ```
   Confirme a lista exata descrita em B.7.

6. **Diff final — confirmar que apenas os arquivos esperados foram alterados:**
   ```
   git diff --stat
   git status --short
   ```
   Deve mostrar exatamente estes arquivos modificados, e nenhum outro:
   - `internal/finance/appypay.go`
   - `internal/handlers/financeiro_handlers.go`
   - `cmd/server/main.go`
   - `Documentação da API.md`

   E exatamente estes arquivos novos:
   - `internal/finance/cobrancas_estudante_integration_test.go`
   - `internal/handlers/financeiro_cobrancas_estudante_handlers_test.go`

No seu relatório final, inclua o resultado literal dos itens 1 a 6 (sem marcar `TestIntegration*` como `✅`
a menos que tenha rodado de verdade contra um Postgres real, conforme a nota no início do documento) e o
commit criado. Eu vou pegar esse commit, rodar a suíte de integração completa contra o PostgreSQL real que já
tenho preparado no meu ambiente (mesmo schema, mesmas 115 migrations), e reportar a você se está tudo certo
ou se algo precisa de mais uma rodada.
