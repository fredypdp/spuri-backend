---
criado: 2026-08-13 00:00
origem: solicitação do usuário
status: feito
---

# Permitir cadastro público de academia com ativação restrita a ADMIN ou FPP (feito)

## Prompt recomendado para executar a atualização

Implemente exatamente o código especificado neste documento: uma nova função `RegisterAcademiaPublica` em `internal/handlers/academia_handlers.go` e uma nova linha de rota pública em `cmd/server/main.go`. Não invente lógica adicional, não altere a assinatura de `RegisterAcademia` nem de `bindRegisterAcademiaRequest`, e não toque em nenhum arquivo relacionado ao fluxo de inscrição/matrícula de estudante numa academia (`estudante_handlers.go`, `solicitacao_matricula_handlers.go`, agregados/projeções de `Estudante`/`SolicitacaoMatricula`). Todo o código deste documento já foi lido e validado diretamente contra o repositório real (`fredypdp/spuri-backend`, branch `main`) e sintaticamente checado com `gofmt`. Aplique os diffs pelos pontos de inserção exatos indicados — não reescreva os arquivos inteiros. Ao final, rode `go build ./...`, `go vet ./...`, `gofmt -l .` e a suíte de testes (`go test ./...`), e só então abra o PR.

## Contexto (investigação já feita — não repetir)

Antes de escrever este documento, o repositório real foi clonado e lido integralmente nos pontos relevantes. Conclusões que já eliminam qualquer ambiguidade para o Codex:

1. **`Academia.Criar()` já força status `"inativo"` sempre.** Em `internal/domain/aggregates/academia.go`, `applyAcademiaCriada` define `Status: "inativo"` incondicionalmente, para qualquer academia criada — não há como o cliente sobrescrever isso, pois `RegisterAcademiaRequest` não tem campo de status. **Nenhuma mudança é necessária aqui.**
2. **Ativação/desativação já é restrita a `adm`/`fpp`.** As rotas `PUT /dominis/academia/:codigo/ativar` e `PUT /dominis/academia/:codigo/desativar` já existem, já chamam `handlers.AtivarAcademia`/`handlers.DesativarAcademia`, e já estão protegidas por `middleware.RequireAdm()`. Na hierarquia de roles do projeto (`internal/middleware/admin_auth_middleware.go`): `fpp` = nível 3, `adm` = nível 2, `gerente` = nível 1 — `RequireAdm()` exige nível ≥ 2, ou seja, exatamente `adm` ou `fpp`. **É literalmente a regra "ADMIN ou FPP" pedida — nenhuma mudança é necessária aqui.**
3. **Login de academia `"inativo"` já é bloqueado.** Em `internal/handlers/auth_handlers.go`, o handler `Login` já verifica o status da academia e retorna erro com a mensagem `"academia inativa. Entre em contato com o administrador."` quando o status não é `"ativo"`. **Nenhuma mudança é necessária aqui.**
4. **Existe precedente direto de rota pública para criação de entidade**: `router.POST("/solicitacao-matricula", handlers.CriarSolicitacaoMatricula)`, registrada solta em `router`, sem grupo, sem middleware.
5. **`RegisterAcademia` (fluxo atual, restrito a admin `fpp`) já usa exatamente as funções que a nova rota pública vai reaproveitar**: `bindRegisterAcademiaRequest`, `validarProvincia`, `generateCodigoAcademia`, `readAndValidatePDF`, `utils.ValidateSenha`, `services.GetDefaultPassword`. Nenhuma dessas funções precisa ser criada ou alterada.
6. **Senha padrão hoje é o próprio `codigo_academia`** (`services.GetDefaultPassword("academia", codigo) → codigo`). Isso funciona no fluxo admin porque o admin, presumivelmente, comunica essa convenção à escola por fora. No cadastro público não existe esse canal — por isso esta tarefa adiciona um campo opcional `senha` exclusivo do cadastro público (ver seção 2).

**Conclusão:** a tarefa é cirúrgica. Não é necessário criar arquivo novo, não é necessário alterar nenhum import existente em `academia_handlers.go` (a nova função usa apenas símbolos que o arquivo já importa), e não é necessário tocar em `AtivarAcademia`, `DesativarAcademia` ou `Login`.

**Limitação desta validação:** não foi possível rodar `go build`/`go vet`/`go test` do módulo completo neste ambiente porque o sandbox de investigação não tem acesso de rede a `proxy.golang.org`. O código foi validado por leitura manual linha a linha contra os arquivos reais e por checagem sintática isolada com `gofmt` (sem erros). **É obrigatório que o Codex rode a suíte completa antes do PR** — ver seção "Critérios de aceite".

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Nova rota | `POST /academia/registo-publico`, sem autenticação | Qualquer academia se autocadastra sem precisar de admin |
| Status inicial | `"inativo"` (automático, já garantido por `Academia.Criar()`) | Nenhum código novo necessário para forçar isso |
| Ativação/desativação | Rotas já existentes, já restritas a `adm`/`fpp` | Nenhuma mudança |
| Fluxo admin (`RegisterAcademia`) | Inalterado | Continua exigindo role `fpp`, continua sem aceitar senha customizada |
| Senha no cadastro público | Campo opcional `senha` (multipart), validado com `utils.ValidateSenha` | Se omitido, cai no padrão atual (`codigo_academia`) |
| Documento `alvara` | Obrigatório, mesmas regras já usadas no fluxo admin | Sem alvará, sem cadastro — mesma exigência legal do fluxo atual |
| Duplicidade de NIF | Mesma checagem já usada em `RegisterAcademia` (`GetByNIF`) | Consistente com o fluxo admin; limitação de corrida pré-existente, não é escopo desta tarefa (ver "Fora de escopo") |
| Auditoria | `db.AuditContext{UserID: "publico", UserType: "publico", IP: ...}` | Rastreável no `spuri_ledger` como ator público, sem inventar um novo `user_type` usado em lógica de negócio (é só metadado livre, já confirmado que nada faz `switch`/`==` sobre esse campo) |

---

# 1. Nova função `RegisterAcademiaPublica`

## Objetivo

Adicionar uma função de handler pública, no mesmo arquivo de `RegisterAcademia`, reaproveitando toda a validação já existente, sem exigir admin autenticado.

## Ponto de inserção exato

Arquivo: `internal/handlers/academia_handlers.go`

Localizar o final da função `RegisterAcademia` — o trecho abaixo aparece **exatamente uma vez** no arquivo, imediatamente antes do comentário que introduz `AtualizarDadosAcademia` (conferido byte a byte contra o arquivo real):

```go
	log.Printf("Academia registada: %s (%s) por admin %s", req.Nome, codigoAcademia, userID)
	c.JSON(http.StatusCreated, gin.H{
		"message":         "academia registada com sucesso",
		"codigo_academia": codigoAcademia,
		"data": gin.H{
			"id":              academia.ID,
			"nome":            req.Nome,
			"nif":             req.NIF,
			"type":            req.Type,
			"provincia":       codigoProvincia,
			"codigo_academia": codigoAcademia,
		},
	})
}

// ============================================================================
// PUT /academia/dados
// ============================================================================
```

Inserir o novo código **entre o `}` que fecha `RegisterAcademia` e o comentário `// PUT /academia/dados`**, assim:

```go
	log.Printf("Academia registada: %s (%s) por admin %s", req.Nome, codigoAcademia, userID)
	c.JSON(http.StatusCreated, gin.H{
		"message":         "academia registada com sucesso",
		"codigo_academia": codigoAcademia,
		"data": gin.H{
			"id":              academia.ID,
			"nome":            req.Nome,
			"nif":             req.NIF,
			"type":            req.Type,
			"provincia":       codigoProvincia,
			"codigo_academia": codigoAcademia,
		},
	})
}

// ============================================================================
// POST /academia/registo-publico
// ============================================================================
//
// RegisterAcademiaPublica permite que uma academia se autocadastre na
// plataforma sem autenticação prévia. É uma variação pública do fluxo
// administrativo (RegisterAcademia) — usa exatamente as mesmas validações
// e o mesmo agregado, mas SEM exigir um admin executor autenticado.
//
// Diferença deliberada em relação a RegisterAcademia: como não há um admin
// para comunicar a senha padrão à academia fora de banda, o cadastro
// público aceita um campo opcional "senha" (multipart/form-data) definido
// pela própria academia. Se omitido, cai no mesmo padrão já usado pelo
// fluxo administrativo (services.GetDefaultPassword). RegisterAcademia
// (fluxo admin) permanece inalterado — continua sempre usando a senha
// padrão baseada no código, sem aceitar senha customizada.
//
// Segurança:
//   - academia.Criar() sempre inicia o status como "inativo" (ver
//     applyAcademiaCriada em internal/domain/aggregates/academia.go) —
//     este comportamento já é automático e não pode ser sobrescrito pelo
//     cliente, pois RegisterAcademiaRequest não possui campo de status.
//   - Apenas um admin com role "adm" ou "fpp" pode ativar a conta, através
//     das rotas já existentes PUT /dominis/academia/:codigo/ativar
//     (middleware.RequireAdm()) — nenhuma mudança necessária nessas rotas.
//   - Login com conta "inativo" já é bloqueado pelo handler Login
//     (internal/handlers/auth_handlers.go) — nenhuma mudança necessária.
func RegisterAcademiaPublica(c *gin.Context) {
	req, alvara, ok := bindRegisterAcademiaRequest(c)
	if !ok {
		return
	}

	if req.Nivel != "escola" && req.Nivel != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("nivel deve ser 'escola' ou 'superior'"))
		return
	}
	req.Type = strings.TrimSpace(strings.ToLower(req.Type))
	if req.Type != "public" && req.Type != "private" {
		utils.RespondWithValidationError(c, fmt.Errorf("type deve ser 'public' ou 'private'"))
		return
	}

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateEndereco(req.Endereco); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := utils.ValidateNIF(req.NIF); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if alvara == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("alvara é obrigatório"))
		return
	}
	alvaraPDF, err := readAndValidatePDF("alvara", alvara)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Senha customizada opcional — exclusiva do cadastro público. Lida
	// diretamente do multipart/form-data já parseado por
	// bindRegisterAcademiaRequest (c.PostForm não reprocessa o body).
	senhaCustomizada := strings.TrimSpace(c.PostForm("senha"))
	if senhaCustomizada != "" {
		if err := utils.ValidateSenha(senhaCustomizada); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	codigoProvincia, err := validarProvincia(req.Provincia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.NivelEscolar != nil {
		nivel := *req.NivelEscolar
		if nivel == "medio" && len(req.AnosAcademicos) > 0 {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"escolas de nivel_escolar 'medio' não devem definir anos_academicos",
			))
			return
		}
		if nivel == "fundamental" || nivel == "misto" {
			if len(req.AnosAcademicos) == 0 {
				utils.RespondWithValidationError(c, fmt.Errorf(
					"escolas de nivel_escolar '%s' devem definir anos_academicos "+
						"(ex: 1_ano_fundamental, 2_ano_fundamental, ...)",
					nivel,
				))
				return
			}
			if err := utils.ValidateAnosFundamental(req.AnosAcademicos); err != nil {
				utils.RespondWithValidationError(c, err)
				return
			}
		}
	}

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
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	senhaFinal := senhaCustomizada
	senhaEraCustomizada := senhaCustomizada != ""
	if senhaFinal == "" {
		senhaFinal = services.GetDefaultPassword("academia", codigoAcademia)
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(senhaFinal), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := aggregates.NewAcademia()
	if err := academia.Criar(
		req.Nivel,
		req.Type,
		req.Nome,
		req.NIF,
		codigoAcademia,
		string(hashedPassword),
		codigoProvincia,
		req.Endereco,
		req.Telefone,
		req.Email,
		req.Website,
		req.NivelEscolar,
		req.Cursos,
		req.AnosAcademicos,
		nil,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	audit := db.AuditContext{
		UserID:   "publico",
		UserType: "publico",
		IP:       c.ClientIP(),
	}
	provider := getStorageProvider(c)
	if provider == nil {
		p, _ := storage.NewStorageProvider()
		provider = p
	}
	dir := fmt.Sprintf("%s/Documentação formal", codigoAcademia)
	if provider == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("storage indisponível"))
		return
	}
	if err := provider.EnsureDir(dir); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if _, err := provider.Upload(fmt.Sprintf("%s/alvara_%s.pdf", dir, codigoAcademia), bytes.NewReader(alvaraPDF.data), alvaraPDF.size); err != nil {
		_ = provider.Delete(dir)
		utils.RespondWithInternalError(c, fmt.Errorf("falha no upload do alvara: %w", err))
		return
	}

	if err := repository.SaveWithAudit(academia, audit); err != nil {
		_ = provider.Delete(dir)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Academia auto-registada (cadastro público, pendente de ativação): %s (%s)", req.Nome, codigoAcademia)

	aviso := "guarde o código da academia: ele é o seu identificador de login. você definiu sua própria senha no cadastro."
	if !senhaEraCustomizada {
		aviso = "guarde o código da academia: ele é o identificador de login e também a senha inicial. altere a senha assim que a conta for ativada."
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "cadastro recebido com sucesso. a conta fica inativa até que um administrador (role adm ou fpp) a ative.",
		"codigo_academia": codigoAcademia,
		"data": gin.H{
			"id":              academia.ID,
			"nome":            req.Nome,
			"nif":             req.NIF,
			"type":            req.Type,
			"provincia":       codigoProvincia,
			"codigo_academia": codigoAcademia,
			"status":          "inativo",
		},
		"aviso": aviso,
	})
}

// ============================================================================
// PUT /academia/dados
// ============================================================================
```

## Escopo obrigatório

### 1.1 Imports

**Nenhum import novo é necessário.** Toda a função usa apenas símbolos que `academia_handlers.go` já importa hoje (`bytes`, `fmt`, `log`, `net/http`, `strings`, `gin`, `db`, `aggregates`, `services`, `storage`, `utils`, `bcrypt`) — confirmado por leitura direta do bloco de imports do arquivo real. Não adicionar, remover nem reordenar nenhuma linha do bloco `import (...)`.

### 1.2 Campo `senha` — regra de negócio

| Campo | Origem | Obrigatório | Regra |
| --- | --- | --- | --- |
| `senha` | `multipart/form-data`, novo | Não | Se enviado, validar com `utils.ValidateSenha` (mínimo 6, máximo 128 caracteres — já existente em `internal/utils/validation.go`, mesma função usada em `internal/handlers/profile_handlers.go` para troca de senha). Se ausente ou vazio, usar `services.GetDefaultPassword("academia", codigoAcademia)`, exatamente como o fluxo admin já faz hoje. |

`RegisterAcademia` (fluxo admin, `POST /dominis/academia/register`) **não deve ganhar esse campo** — continua exclusivamente com a senha padrão baseada no código, sem nenhuma alteração de código nessa função.

### 1.3 Por que não alterar `RegisterAcademiaRequest` nem `bindRegisterAcademiaRequest`

O campo `senha` é lido diretamente via `c.PostForm("senha")` dentro da própria `RegisterAcademiaPublica`, **depois** de chamar `bindRegisterAcademiaRequest`. Isso é seguro porque `bindRegisterAcademiaRequest` já chama `c.Request.ParseMultipartForm` antes, e `c.PostForm` lê do formulário multipart já parseado em memória — não há re-leitura do body HTTP. Essa abordagem evita qualquer alteração na struct/função compartilhada com `RegisterAcademia`, eliminando risco de regressão no fluxo admin.

---

# 2. Nova rota pública

## Objetivo

Expor `RegisterAcademiaPublica` como rota pública, sem qualquer middleware de autenticação.

## Ponto de inserção exato

Arquivo: `cmd/server/main.go`

Localizar o bloco abaixo, na seção de rotas públicas (aparece **exatamente uma vez** no arquivo):

```go
	router.POST("/login", middleware.LoginRateLimit(), handlers.Login)
	router.POST("/bootstrap", middleware.LoginRateLimit(), handlers.BootstrapAdminFPP)
	router.POST("/solicitacao-matricula", handlers.CriarSolicitacaoMatricula)
```

Substituir por:

```go
	router.POST("/login", middleware.LoginRateLimit(), handlers.Login)
	router.POST("/bootstrap", middleware.LoginRateLimit(), handlers.BootstrapAdminFPP)
	router.POST("/solicitacao-matricula", handlers.CriarSolicitacaoMatricula)
	router.POST("/academia/registo-publico", handlers.RegisterAcademiaPublica)
```

## Escopo obrigatório

- A rota deve ser registrada **diretamente em `router`**, sem middleware, exatamente como `/solicitacao-matricula` — não usar `AuthMiddleware()`, `OptionalAuthMiddleware()` nem qualquer `RequireX()`.
- Não alterar nenhuma outra linha de `main.go` além desta inserção.
- Não existe conflito de rota: os grupos `router.Group("/academia")` já existentes (`academiaShared`, `academiaAnoLetivoRead`, `academiaRead`, `academia`) não registram `POST /registo-publico` em nenhum lugar, e o único outro endpoint de cadastro de academia (`POST /dominis/academia/register`) vive sob um prefixo de grupo diferente (`/dominis`).

---

# 3. Testes obrigatórios

## Objetivo

Seguir exatamente o padrão de teste já usado no arquivo `internal/handlers/academia_register_contact_test.go` (leitura do código-fonte como texto e checagem de trechos esperados) — este projeto usa esse padrão precisamente porque os testes deste pacote não dependem de banco de dados real. Não criar um padrão de teste diferente para esta tarefa.

## Ponto de inserção exato

Criar um **novo arquivo**: `internal/handlers/academia_register_publica_test.go`.

Confirmar antes de criar que nenhuma das funções abaixo já existe em outro arquivo de teste do pacote `handlers` (evita `redeclared`) — já verificado nesta investigação: não existe.

```go
package handlers

import (
	"os"
	"strings"
	"testing"
)

func extractFunctionSource(t *testing.T, source, funcSignature string) string {
	t.Helper()
	start := strings.Index(source, funcSignature)
	if start == -1 {
		t.Fatalf("função não encontrada no arquivo: %s", funcSignature)
	}
	rest := source[start:]
	end := strings.Index(rest[1:], "\nfunc ")
	if end == -1 {
		return rest
	}
	return rest[:end+1]
}

func TestRegisterAcademiaPublicaPassesContactFieldsToAggregate(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademiaPublica(")

	const expectedCall = "\t\treq.Endereco,\n\t\treq.Telefone,\n\t\treq.Email,\n\t\treq.Website,"
	if !strings.Contains(fn, expectedCall) {
		t.Fatal("RegisterAcademiaPublica deve repassar req.Telefone e req.Email para Academia.Criar, igual ao RegisterAcademia")
	}
}

func TestRegisterAcademiaPublicaDoesNotRequireAdminAuth(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademiaPublica(")

	forbidden := []string{"getAdminProjection(", "middleware.GetUserID(", "executorAdmin"}
	for _, term := range forbidden {
		if strings.Contains(fn, term) {
			t.Fatalf("RegisterAcademiaPublica não deve depender de autenticação/admin — encontrado %q", term)
		}
	}
}

func TestRegisterAcademiaPublicaForcesNilCriadoPor(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademiaPublica(")

	if !strings.Contains(fn, "\t\treq.AnosAcademicos,\n\t\tnil,") {
		t.Fatal("RegisterAcademiaPublica deve chamar academia.Criar com criadoPor=nil (cadastro público sem admin executor)")
	}
}

func TestRegisterAcademiaPublicaAllowsCustomPasswordWithFallback(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademiaPublica(")

	mustContain := []string{
		`c.PostForm("senha")`,
		"utils.ValidateSenha(senhaCustomizada)",
		`services.GetDefaultPassword("academia", codigoAcademia)`,
	}
	for _, term := range mustContain {
		if !strings.Contains(fn, term) {
			t.Fatalf("RegisterAcademiaPublica deve aceitar senha customizada opcional com fallback para o padrão — não encontrado: %q", term)
		}
	}
}

func TestRegisterAcademiaDoesNotAcceptCustomPassword(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademia(")

	if strings.Contains(fn, `c.PostForm("senha")`) {
		t.Fatal("RegisterAcademia (fluxo admin) não deve aceitar senha customizada — comportamento deve continuar exclusivo do cadastro público")
	}
}

func TestAcademiaRegistoPublicoRouteIsPublic(t *testing.T) {
	source, err := os.ReadFile("../../cmd/server/main.go")
	if err != nil {
		t.Fatalf("read main.go source: %v", err)
	}

	var routeLine string
	for _, line := range strings.Split(string(source), "\n") {
		if strings.Contains(line, "/academia/registo-publico") {
			routeLine = line
			break
		}
	}
	if routeLine == "" {
		t.Fatal("rota POST /academia/registo-publico não encontrada em cmd/server/main.go")
	}
	if strings.Contains(routeLine, "middleware.") {
		t.Fatalf("rota /academia/registo-publico deve ser pública, sem middleware de autenticação: %q", routeLine)
	}
	if !strings.Contains(routeLine, "router.POST(") {
		t.Fatalf("rota /academia/registo-publico deve ser registrada diretamente em router.POST, fora de grupos autenticados: %q", routeLine)
	}
}
```

Este arquivo já foi validado sintaticamente com `gofmt -e` (parse OK) e `gofmt -l` (sem diferenças de formatação) neste ambiente de investigação.

## Testes de integração recomendados (se a suíte do projeto já tiver base de integração com Postgres real)

Caso o projeto já possua testes de integração HTTP+DB para `RegisterAcademia` (verificar antes de assumir), replicar os mesmos cenários para `RegisterAcademiaPublica`:

1. Cadastro público completo e válido (com `alvara` PDF) → `201`, `status` retornado como `"inativo"`.
2. Cadastro público sem `alvara` → `400`.
3. Cadastro público com `nif` duplicado → `409`.
4. Cadastro público sem campo `senha` → sucesso, e a senha efetivamente gravada (hash) corresponde a `services.GetDefaultPassword("academia", codigoAcademia)`.
5. Cadastro público com `senha` de 3 caracteres → `400` (abaixo do mínimo de `utils.ValidateSenha`).
6. Cadastro público com `senha` válida (ex.: 8 caracteres) → sucesso, e login subsequente com essa senha funciona **somente depois** que um admin `adm`/`fpp` ativar a conta (login antes disso deve retornar o erro de "academia inativa" já existente).
7. `PUT /dominis/academia/:codigo/ativar` chamado por um admin `gerente` (não `adm`/`fpp`) sobre uma academia criada pelo cadastro público → `403` (comportamento já existente, apenas confirmando que nada quebrou).
8. `POST /dominis/academia/register` (fluxo admin) continua funcionando sem enviar `senha` e ignora `senha` caso seja enviada por engano (não existe leitura desse campo nessa função).

---

# 4. Atualização obrigatória da documentação

Arquivo: `Documentação da API.md`, seção **6. Academias**, imediatamente após o bloco `### POST /dominis/academia/register` (antes de `### PUT /dominis/academia/:codigo/ativar`).

Adicionar:

```markdown
### POST /academia/registo-publico

Permite que uma academia se autocadastre na plataforma **sem autenticação prévia**, via `multipart/form-data`. Usa exatamente as mesmas regras de validação de `POST /dominis/academia/register` (`nif` obrigatório, único, 10 dígitos; `alvara` obrigatório, PDF válido até 10MB, armazenado em `{codigo_academia}/Documentação formal/`). A academia é sempre criada com status `inativo` — apenas um admin com role `adm` ou `fpp` pode ativá-la, via `PUT /dominis/academia/:codigo/ativar`. Login antes da ativação retorna erro de "academia inativa".

**Proteção**: nenhuma (rota pública)

**Diferença em relação ao cadastro por admin**: aceita um campo opcional `senha` (string, 6–128 caracteres). Se enviado, essa senha é definida como a senha de acesso da academia. Se omitido, a senha inicial é o próprio `codigo_academia`, como no fluxo administrativo.

**Request:**

```json
{
  "nivel": "escola",
  "type": "public",
  "nome": "Escola Primária Ngola Kiluanje",
  "nif": "0012345678",
  "alvara": "@./alvara.pdf;type=application/pdf",
  "provincia": "luanda",
  "endereco": "Rua Direita, 123",
  "telefone": "+244923000000",
  "email": "escola@exemplo.ao",
  "website": "https://escola.ao",
  "nivel_escolar": "fundamental",
  "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental", "9_ano_fundamental"],
  "cursos": [],
  "senha": "minhaSenhaSegura123"
}
```

**Response 201:**

```json
{
  "message": "cadastro recebido com sucesso. a conta fica inativa até que um administrador (role adm ou fpp) a ative.",
  "codigo_academia": "LDA20261",
  "data": {
    "id": "uuid",
    "nome": "string",
    "nif": "0012345678",
    "type": "public",
    "provincia": "LDA",
    "codigo_academia": "LDA20261",
    "status": "inativo"
  },
  "aviso": "guarde o código da academia: ele é o seu identificador de login. você definiu sua própria senha no cadastro."
}
```

**Erros:**

- `400` — `nivel` inválido, `type` inválido, `nif` ausente/inválido, `alvara` ausente/não PDF/acima de 10MB, campos obrigatórios ausentes, `anos_academicos` inválidos, ou `senha` fora do intervalo de 6–128 caracteres
- `409` — `nif` já cadastrado em outra academia

---
```

Não é necessário atualizar `.env.example` — nenhuma variável de ambiente nova é usada por esta tarefa.

---

# Fora de escopo

- Corrigir a condição de corrida entre a checagem `GetByNIF` e a gravação do evento (tanto no fluxo admin quanto no público) — é uma característica pré-existente do `RegisterAcademia` atual, não uma regressão introduzida por esta tarefa. Se Fredy quiser proteção atômica real (ex.: reservando o `nif` via `unique_operation_guards`, como já é feito em outras tarefas concluídas deste projeto), isso deve ser uma tarefa dedicada e explicitamente solicitada.
- Rate limiting/CAPTCHA na nova rota pública — a rota-irmã mais próxima (`/solicitacao-matricula`) também não tem rate limiting hoje; manter paridade. Proteção contra abuso desta rota específica pode ser uma tarefa futura dedicada.
- Notificação por email/SMS para admins avisando de um novo cadastro público pendente de ativação, ou para a academia avisando que foi ativada/desativada — não solicitado, e o módulo de SMS ainda não está integrado ao código (ver notas gerais do projeto).
- Qualquer tela ou fluxo no frontend (`spuripainel`) — esta tarefa cobre apenas o backend (`spuri-backend`). Se necessário, deve ser orquestrada como uma tarefa separada específica para o frontend, com o mesmo nível de investigação.
- Qualquer alteração em `AtivarAcademia`, `DesativarAcademia`, `Login`, `bindRegisterAcademiaRequest` ou `RegisterAcademiaRequest` — todos permanecem exatamente como estão.
- Qualquer alteração em arquivos do fluxo de inscrição/matrícula de estudante numa academia (`estudante_handlers.go`, `solicitacao_matricula_handlers.go`, agregados de `Estudante`/`SolicitacaoMatricula`, projeções correspondentes) — esta tarefa não toca nenhum desses arquivos.
- Permitir que a academia recupere/redefina a senha depois do cadastro caso a esqueça antes de ativar — o fluxo de recuperação de senha já existente (por email) continua sendo o caminho, sem mudanças aqui.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `RegisterAcademiaPublica` existir em `internal/handlers/academia_handlers.go`, inserida exatamente no ponto indicado na seção 1, sem nenhuma linha de import alterada;
2. `POST /academia/registo-publico` estiver registrada em `cmd/server/main.go`, sem middleware, exatamente no ponto indicado na seção 2;
3. uma academia criada por essa rota nascer sempre com status `inativo` (comportamento automático de `Academia.Criar()`, sem código adicional);
4. `PUT /dominis/academia/:codigo/ativar` e `PUT /dominis/academia/:codigo/desativar` continuarem funcionando sem nenhuma alteração de código, restritos a `adm`/`fpp`;
5. login de uma academia criada pela nova rota, antes de ser ativada, continuar retornando o erro já existente de "academia inativa", sem nenhuma alteração em `Login`;
6. o campo opcional `senha` funcionar conforme a seção 1.2 — validado com `utils.ValidateSenha` quando enviado, com fallback para `services.GetDefaultPassword` quando omitido — e `RegisterAcademia` (fluxo admin) **não** ganhar esse campo;
7. `internal/handlers/academia_register_publica_test.go` existir com todos os testes da seção 3, e todos passarem;
8. `Documentação da API.md` estar atualizada conforme a seção 4;
9. `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...` rodarem limpos, sem erros, sem `redeclared`, sem `undefined`, sem arquivos mal formatados — **esta checagem é obrigatória e não foi possível ser feita no ambiente de investigação usado para escrever este documento** (ver "Limitação desta validação", no início);
10. nenhum arquivo do fluxo de inscrição/matrícula de estudante numa academia ter sido alterado ou removido;
11. `RegisterAcademia`, `bindRegisterAcademiaRequest` e `RegisterAcademiaRequest` permanecerem byte-a-byte idênticos ao estado atual do repositório, exceto pela adição do novo bloco de código (nenhuma linha existente removida ou modificada nesses três símbolos).

## Arquivos a remover

**Nenhum.** Esta tarefa não deprecia nem substitui nenhum arquivo ou função existente — apenas adiciona.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Permitir cadastro público de academia com ativação restrita a ADMIN ou FPP (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
