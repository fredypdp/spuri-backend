---
status: a fazer
---

# Tarefa 52 — Corrigir CORS bloqueando requisições PATCH em `/academia/notas-aluno/:id` e `/academia/faltas-aluno/:id`

## Prompt recomendado para Codex

```
Execute EXATAMENTE as instruções da Tarefa 51 (docs/Lista de Tarefas/51 - ...md), na ordem em
que aparecem: primeiro a Seção 1 (correção do CORS em cmd/server/main.go), depois a Seção 2
(criar o arquivo cmd/server/cors_test.go, conteúdo já pronto para copiar). NÃO invente nada além
do que está escrito, e NÃO toque em go.mod nem go.sum — os dois blocos de código já estão prontos
e validados. Depois de aplicar tudo, rode exatamente os comandos da seção "Comandos que você deve
rodar" e cole a saída completa de cada um. Você NÃO tem PostgreSQL disponível no seu ambiente
(isso é esperado e está documentado abaixo) — os testes de integração com banco serão pulados
(skip) automaticamente, e isso é normal, não é uma falha sua. NÃO tente instalar Postgres, Docker
ou usar apt — nada disso vai funcionar no seu ambiente e não é necessário para esta tarefa. NÃO
marque esta tarefa como concluída nem mova o arquivo para "Tarefas feitas" — quem faz essa
confirmação final é o Fredy (com a Claude), que já validou esta correção inteira, incluindo os
testes de integração com PostgreSQL real, antes de te passar esta tarefa. Ao final, apenas
reporte a saída dos comandos e pare.
```

## Contexto e diagnóstico

### O problema relatado

Ao tentar corrigir uma falta pela rota `PATCH /academia/faltas-aluno/{id}` a partir do
front-end (`spuripainel`, hospedado em `https://spuri-teste.vercel.app`), o navegador bloqueou a
requisição com este erro no console:

```
Access to fetch at 'https://spuri-backend-teste.onrender.com/academia/faltas-aluno/d239cf5f-aa27-455c-b2d2-88a8765c7c6c'
from origin 'https://spuri-teste.vercel.app' has been blocked by CORS policy: Method PATCH is not
allowed by Access-Control-Allow-Methods in preflight response.
```

Importante notar: um `curl` manual direto para essa mesma rota, com o mesmo método `PATCH` e o
mesmo token, **funciona normalmente** — porque `curl` não faz a etapa de "preflight" (uma
requisição `OPTIONS` que o navegador dispara automaticamente antes de qualquer `PATCH`/`PUT`/
`DELETE` com corpo JSON, para perguntar ao servidor quais métodos e cabeçalhos são permitidos).
Isso é exatamente por que esse bug passa despercebido em testes manuais via terminal/Postman e só
aparece de verdade no navegador, em produção.

### Causa raiz (já localizada e confirmada)

**Não há nenhum problema em `db/`, nos handlers, nos aggregates ou nas projections.** As rotas em
si estão corretamente registradas em `cmd/server/main.go`:

```go
academia.PATCH("/notas-aluno/:id", handlers.CorrigirNota)
academia.PATCH("/faltas-aluno/:id", handlers.CorrigirFalta)
```

O problema está isolado inteiramente no middleware de CORS, também em `cmd/server/main.go`,
função `corsMiddleware()`. Ele responde ao preflight `OPTIONS` com uma lista fixa de métodos
permitidos que **nunca incluiu `PATCH`**:

```go
// ANTES (linha ~597):
c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
```

Como essa string é mantida manualmente e separada da lista real de rotas, qualquer rota nova que
passe a usar um método ainda não presente nessa string quebra silenciosamente no navegador — sem
gerar nenhum erro do lado do servidor (o `PATCH` em si funcionaria perfeitamente se o preflight o
deixasse passar). Isso afeta **as duas rotas de correção**, exatamente como você suspeitou:

- `PATCH /academia/notas-aluno/:id` (correção de nota)
- `PATCH /academia/faltas-aluno/:id` (correção de falta)

Nenhuma outra rota do sistema usa `PATCH` hoje (confirmado por busca em todo o repositório), então
a correção abaixo resolve o problema para as duas rotas de uma vez, e também previne a recorrência
para qualquer rota `PATCH` futura.

### Já validado pela Claude, num sandbox com Go 1.24.12 e PostgreSQL 16 reais

Antes de te passar esta tarefa, eu (Claude) já:

1. Reproduzi o bug de verdade com um teste isolado que chama `setupRouter()` e simula o preflight
   `OPTIONS` exatamente como o navegador faz — confirmando `Access-Control-Allow-Methods` sem
   `PATCH` no comportamento atual do código.
2. Apliquei a correção da Seção 1 abaixo e confirmei que o mesmo teste passa a devolver `PATCH` no
   cabeçalho.
3. Escrevi e validei o teste de regressão da Seção 2 (falha sem a correção, passa com ela).
4. Rodei `go build ./...`, `go vet ./...` e a suíte completa `go test ./...` — incluindo os testes
   de integração que exigem `SPURI_RUN_DB_INTEGRITY_TESTS=1` contra um PostgreSQL 16 real que
   instalei no sandbox — e confirmei que a mudança **não quebra nada** no restante do sistema.

Ou seja: a correção abaixo já está validada de ponta a ponta, inclusive contra banco real. Sua
parte é só aplicar exatamente o que está escrito e confirmar que builda e passa nos testes que não
dependem de Postgres (que é a limitação conhecida do seu ambiente).

### Não é necessário mexer no front-end (`spuripainel`)

`Access-Control-Allow-Methods` é um cabeçalho controlado inteiramente pelo servidor. Uma vez
corrigido no backend, nenhuma mudança é necessária no front-end para este bug especificamente.

---

## Seção 1 — Correção obrigatória (CORS `Access-Control-Allow-Methods`)

Arquivo: `cmd/server/main.go`, dentro da função `corsMiddleware()`.

```go
// ANTES:
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

// DEPOIS:
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
```

É a única mudança de produção necessária: adicionar `PATCH` à lista, mantendo a ordem e o
formato (vírgula + espaço) idênticos ao restante da string.

---

## Seção 2 — Teste de regressão automatizado (sem depender de Postgres)

Crie o arquivo novo `cmd/server/cors_test.go` com exatamente este conteúdo:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCORSPreflightAllowsAllRegisteredHTTPMethods garante que o preflight
// OPTIONS tratado por corsMiddleware sempre libera, em Access-Control-Allow-Methods,
// todo método HTTP que exista em pelo menos uma rota registrada em setupRouter().
//
// Este teste existe porque Access-Control-Allow-Methods em corsMiddleware
// (cmd/server/main.go) é uma string fixa, mantida manualmente, separada da
// lista real de rotas. Quando uma rota nova passa a usar um método HTTP que
// ainda não constava nessa string (foi o caso de PATCH em
// /academia/notas-aluno/:id e /academia/faltas-aluno/:id), a chamada direta
// via curl/Postman continua funcionando normalmente — só o navegador, que faz
// preflight OPTIONS antes de métodos não "simples", passa a bloquear a
// requisição real com o erro:
//
//	"Method PATCH is not allowed by Access-Control-Allow-Methods in preflight response"
//
// Isso faz o bug ficar invisível em testes manuais de terminal e só aparecer
// em produção, no navegador. Ao invés de fixar apenas "PATCH" no teste, ele
// itera dinamicamente sobre router.Routes(), então qualquer método novo
// adicionado no futuro (ex.: um DELETE ou HEAD em alguma rota nova) que não
// for espelhado em Access-Control-Allow-Methods derruba este teste
// automaticamente, sem precisar lembrar de atualizá-lo.
func TestCORSPreflightAllowsAllRegisteredHTTPMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	registeredMethods := map[string]bool{}
	for _, route := range router.Routes() {
		registeredMethods[route.Method] = true
	}
	if len(registeredMethods) == 0 {
		t.Fatal("nenhuma rota registrada em setupRouter(); não é possível validar o CORS")
	}

	req := httptest.NewRequest(http.MethodOptions, "/academia/faltas-aluno/00000000-0000-0000-0000-000000000000", nil)
	req.Header.Set("Origin", "https://spuri-teste.vercel.app")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("esperado 204 no preflight OPTIONS, recebeu %d (body=%s)", w.Code, w.Body.String())
	}

	allowMethods := w.Header().Get("Access-Control-Allow-Methods")
	if allowMethods == "" {
		t.Fatal("Access-Control-Allow-Methods ausente na resposta de preflight")
	}

	for method := range registeredMethods {
		if method == http.MethodOptions {
			continue
		}
		if !strings.Contains(allowMethods, method) {
			t.Errorf(
				"método %s está registrado em pelo menos uma rota mas ausente em Access-Control-Allow-Methods (%q); "+
					"navegadores bloquearão o preflight para esse método em produção",
				method, allowMethods,
			)
		}
	}
}

// TestCORSPreflightFaltasAlunoAndNotasAlunoAllowPatch cobre especificamente os
// dois endpoints que geraram o bug em produção (correção de nota e de falta),
// reproduzindo o preflight real feito pelo front-end (mesmo Origin usado pelo
// spuripainel de teste), com asserção explícita de que PATCH está liberado.
func TestCORSPreflightFaltasAlunoAndNotasAlunoAllowPatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	paths := []string{
		"/academia/notas-aluno/00000000-0000-0000-0000-000000000000",
		"/academia/faltas-aluno/00000000-0000-0000-0000-000000000000",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodOptions, path, nil)
		req.Header.Set("Origin", "https://spuri-teste.vercel.app")
		req.Header.Set("Access-Control-Request-Method", "PATCH")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("%s: esperado 204 no preflight OPTIONS, recebeu %d", path, w.Code)
		}

		allowMethods := w.Header().Get("Access-Control-Allow-Methods")
		if !strings.Contains(allowMethods, "PATCH") {
			t.Fatalf("%s: PATCH ausente em Access-Control-Allow-Methods (%q)", path, allowMethods)
		}

		allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "https://spuri-teste.vercel.app" {
			t.Fatalf("%s: Access-Control-Allow-Origin inesperado (%q)", path, allowOrigin)
		}
	}
}
```

Estes dois testes **não precisam de PostgreSQL** — `setupRouter()` monta o router e o middleware
de CORS sem tocar no banco para este caminho de código, exatamente como os demais testes já
existentes em `cmd/server/main_test.go` (que também chamam `setupRouter()` sem banco).

---

## Comandos que você deve rodar (sem Postgres — Codex)

Nesta ordem, colando a saída completa de cada um:

```bash
go build ./...
go vet ./...
go test ./cmd/server/... -run TestCORSPreflight -v
go test ./...
```

O último comando (`go test ./...`) vai naturalmente **pular (skip)** os testes de integração que
exigem `SPURI_RUN_DB_INTEGRITY_TESTS=1` com PostgreSQL — isso é esperado no seu ambiente e não deve
ser tratado como falha. Não defina essa variável, não tente instalar Postgres/Docker via `apt`
(vai dar `403 Forbidden`, é uma limitação conhecida do seu ambiente) — essa parte já foi validada
pela Claude, como descrito acima.

**Resultado esperado:** `go build` e `go vet` sem nenhuma saída (sucesso silencioso); os dois
testes novos de CORS em `PASS`; e `go test ./...` sem nenhuma falha **nova** introduzida por esta
mudança (alguns pacotes vão mostrar testes pulados/`skip`, o que é normal).

### Observação sobre 3 falhas pré-existentes e não relacionadas

Ao validar a suíte completa contra Postgres real, a Claude encontrou 3 falhas em testes que **já
existiam antes desta tarefa e não têm nenhuma relação com CORS/PATCH** (ficaram confirmadas mesmo
isolando cada teste individualmente):

1. `internal/finance/qrcode_regression_integration_test.go` →
   `TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr`: falha com `APPYPAY_RESOURCE não
   configurada` porque esse teste específico não chama `t.Setenv("APPYPAY_RESOURCE", ...)` como os
   demais testes irmãos do pacote fazem.
2. `cmd/server/turma_vinculo_estudante_integration_test.go` →
   `TestTurmaVinculo11AdicionarEstudanteATurmaRotaManualPreservaStatusERegraDuplicidade`: falha com
   `bi_encarregado deve ser PDF`, indicando que o arquivo fake usado como fixture de teste não
   satisfaz mais a validação de PDF.
3. `internal/db/event_store_integrity_test.go` e `internal/db/repository_concurrency_test.go`:
   falham com `erro ao ler diretório de migrations 'migrations': no such file or directory` — ao
   contrário dos testes de integração em `cmd/server`, esses dois não fazem `os.Chdir("../..")`
   antes de rodar as migrations, então o caminho relativo `"migrations"` nunca resolve quando o
   teste roda a partir do diretório `internal/db`.

**Nenhuma dessas 3 falhas é do escopo desta tarefa** — são bugs de teste pré-existentes, em
pacotes que esta correção de CORS não toca. Se elas aparecerem no seu `go test ./...` (fora do
`SPURI_RUN_DB_INTEGRITY_TESTS=1`, a 1 e a 3 serão puladas por padrão; a 2 pode aparecer se você
rodar a suíte completa de `cmd/server`), não tente corrigi-las agora. Apenas confirme que nenhuma
falha *nova* apareceu além dessas já conhecidas.

---

## O que você NÃO deve fazer

- Não altere `go.mod` nem `go.sum`.
- Não tente instalar PostgreSQL, Docker ou usar `apt` — não vai funcionar no seu ambiente.
- Não defina `SPURI_RUN_DB_INTEGRITY_TESTS=1` nem `RUN_POSTGRES_INTEGRATION=1`.
- Não tente corrigir as 3 falhas pré-existentes listadas acima — são de outra tarefa.
- Não toque no repositório `spuripainel` (front-end) — este bug é 100% resolvido no backend.
- Não marque esta tarefa como concluída nem mova este arquivo para "Tarefas feitas".

## Commit sugerido

```
fix(cors): liberar PATCH em Access-Control-Allow-Methods

Access-Control-Allow-Methods nunca incluiu PATCH, quebrando o preflight
do navegador para PATCH /academia/notas-aluno/:id e
PATCH /academia/faltas-aluno/:id em produção (curl/Postman não sofriam,
por não fazerem preflight). Adiciona teste de regressão que verifica
automaticamente, para qualquer rota registrada, que seu método HTTP
está espelhado em Access-Control-Allow-Methods.
```
