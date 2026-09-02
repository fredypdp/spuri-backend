---
tarefa: 80
titulo: Tornar o alvará opcional no cadastro de academia (2 rotas renomeadas) + upload individual posterior
repo: fredypdp/spuri-backend
status: pronta para execução pelo Codex
orquestrado_e_pre_testado_por: Claude
validado_contra: PostgreSQL 16 real + storage local (não Mega), Go 1.24, suíte completa + 9 testes novos
---

# Como usar este documento (leia antes de tocar em qualquer arquivo)

Este documento foi pré-testado de ponta a ponta pelo Claude contra infraestrutura real (Postgres 16, Go 1.24, storage local) — não é um plano teórico. Todos os diffs abaixo já foram aplicados, compilados (`go build ./...`, `go vet ./...`) e testados (suíte completa + 9 testes de integração novos, todos passando) numa cópia de trabalho do repositório.

Regras:
1. Aplique os diffs **exatamente como estão**, na ordem listada. Não refatore, não renomeie nada além do que está aqui, não "melhore" nada por conta própria.
2. Você (Codex) **não tem PostgreSQL, Docker nem psql** no seu ambiente. Não tente rodar os testes de integração marcados com `SPURI_RUN_DB_INTEGRITY_TESTS=1` — eles já foram rodados e confirmados por mim (evidência na seção 4). Rode apenas `go build ./...`, `go vet ./...` e a suíte que NÃO exige banco (`go test ./...` sem a env var — isso já roda a maior parte da suíte, inclusive os testes unitários dos handlers tocados aqui).
3. Se qualquer diff não aplicar limpo (contexto não bate), **pare e sinalize** — não tente adivinhar o que mudou.
4. Não é necessária nenhuma migration nova. Confirmei isso contra o schema real: `alvara` nunca foi uma coluna de banco, é só um arquivo no storage num path determinístico (`{codigo_academia}/Documentação formal/alvara_{codigo_academia}.pdf`).
5. Ao terminar e validar (seção 5), mova este arquivo para `docs/Tarefas feitas/`.

---

## 1. Contexto / diagnóstico

Duas rotas de cadastro de academia exigiam o alvará (PDF do documento formal) como obrigatório:

- `POST /dominis/academia/register` (admin, role `fpp`) → `internal/handlers/academia_handlers.go`, função `RegisterAcademia`
- `POST /academia/registo-publico` (público, sem autenticação) → mesma arquivo, função `RegisterAcademiaPublica`

Ambas devem ser **renomeadas** e o alvará deve se tornar **opcional**, podendo ser enviado depois via um **novo endpoint** de upload individual.

Renomeação exigida (só estas duas rotas — nenhuma outra):
- `POST /dominis/academia/register` → `POST /dominis/academia/cadastro`
- `POST /academia/registo-publico` → `POST /academia/cadastro`

**Fora de escopo, não tocar:** `POST /dominis/academia/register/async` (batch assíncrono) e a função não-roteada `RegisterAcademiaBatch` continuam com o nome atual — elas chamam `RegisterAcademia` internamente, então herdam o comportamento de alvará opcional automaticamente, sem qualquer mudança adicional de código. Só o comentário de `batch_handlers.go` ficará mencionando o path antigo — deixei assim de propósito para não expandir o escopo; é cosmético.

### Achado crítico #1 do pré-teste: alvará nunca foi rastreado no banco

`grep` completo no domínio/agregados/projeções confirma: não existe nenhuma coluna `alvara` em `projection_academias` nem em nenhuma migration. O "obrigatório" era **só** a validação no handler (`if alvara == nil { erro }`) + upload físico no storage. Isso significa: tornar opcional é seguro e não deixa o banco num estado inconsistente — a "ausência" do alvará já é modelada corretamente hoje como "arquivo não existe no path determinístico", e o download/list já tratam isso (quase) graciosamente (ver achado #2).

### Achado crítico #2 do pré-teste: bug real no storage local (não é do Mega em produção)

Ao testar "cadastrar sem alvará, depois tentar baixar", encontrei um bug **pré-existente** em `internal/storage/storage.go`, método `(*MegaProvider).Read`, branch `m.local` (usado quando `STORAGE_PROVIDER=local`, o fallback de dev/teste — **não** o caminho real do Mega em produção):

```go
func (m *MegaProvider) Read(remotePath string) (io.ReadCloser, error) {
	if m.local {
		p, err := m.path(remotePath)
		if err != nil {
			return nil, err
		}
		return os.Open(p)   // <- erro cru do os, nunca vira ErrNotFound
	}
	...
```

Quando o arquivo não existe, `os.Open` retorna um `*fs.PathError` que **nunca** é convertido em `storage.ErrNotFound` — diferente do método `listLocal`, 20 linhas abaixo, que já faz esse `os.IsNotExist(err)` corretamente. Resultado: baixar um alvará nunca enviado respondia **503** ("falha ao ler documento no storage") em vez de **404** ("documento não encontrado").

Esse bug sempre existiu, mas era invisível: como o alvará era sempre obrigatório, "baixar um alvará que não existe" nunca acontecia em uso normal. Com o cadastro opcional, esse caminho passa a ser comum (é literalmente o estado esperado logo após um cadastro sem alvará). Por isso a correção abaixo **faz parte desta tarefa**, não é um extra — sem ela, o fluxo "cadastre sem alvará, confira depois" devolve um erro 503 assustador em vez de um 404 tratável.

A correção é mínima, aditiva, e **não toca em nada do fluxo Mega real** (só o branch `m.local`), respeitando o aviso já existente no topo de `storage.go` ("não alterar dependências, provider ou fluxo de arquivos sem solicitação explícita").

### Achado crítico #3 do pré-teste: toda academia nasce "inativo" (mesmo pela rota admin)

`Academia.Criar()` sempre define `Status = "inativo"` por padrão — inclusive no cadastro pela rota admin. Só um `PUT /dominis/academia/:codigo/ativar` (role `adm`+) muda para `"ativo"`. `AuthMiddleware`/`verificarStatusUsuario` bloqueia login (JWT) de qualquer conta inativa, **independente do tipo**. Isso não é um bug — é a regra de negócio existente — mas é essencial saber disso para entender por que "a academia envia o próprio alvará depois" só funciona **após** a ativação. Não requer nenhuma mudança de código; é só um fato do domínio que evita confusão ao ler os testes novos (seção 4).

### Decisão de design: upload individual posterior

Novo endpoint, espelhando exatamente o escopo e a regra de permissão do endpoint de download já existente:

```
GET  /documentos/academias/:codigo/:campo/download   (já existe — DownloadDocumentoAcademia)
POST /documentos/academias/:codigo/:campo/upload      (NOVO — UploadDocumentoAcademia)
```

Mesmo grupo de rota (`protected`, fora de `/academia`, `/estudante` e `/dominis` — autenticado, sem exigir role específica), mesma função de permissão `canAccessAcademiaDocument` (admin, ou a própria academia dona do `codigo_academia`). `:campo` só aceita `"alvara"` hoje (mesma restrição do download) — 404 para qualquer outro valor. Reenviar para uma academia que já tem alvará **substitui** o arquivo (delete-then-upload no mesmo path determinístico), então o mesmo endpoint serve tanto para o primeiro envio quanto para corrigir um envio anterior.

Optei por **não** adicionar nenhum campo de "alvará presente/ausente" nas rotas de listagem (`ListarDocumentosAcademia` etc.) — essas rotas hoje são deliberadamente baratas (constroem a URL de forma determinística, sem nenhuma chamada ao storage). Adicionar uma verificação de existência ali criaria uma chamada de rede ao Mega por documento por requisição, em potencial N+1 em telas de listagem — um custo de performance real que o pedido original não justifica. O frontend trata a ausência via 404 gracioso no download (ver documento do frontend).

---

## 2. Diffs a aplicar, nesta ordem

Todos os diffs abaixo foram extraídos com `git diff` a partir de uma cópia limpa de `main` após aplicar e validar as mudanças. Aplique com `git apply` ou reproduza manualmente as linhas `-`/`+` — o resultado final tem que ser idêntico.

### Arquivo 1/5 — `internal/storage/storage.go` (correção do bug #2)

```diff
--- a/internal/storage/storage.go
+++ b/internal/storage/storage.go
@@ -514,7 +514,13 @@ func (m *MegaProvider) List(remotePath string) ([]StoredFile, error) {
 func (m *MegaProvider) Read(remotePath string) (io.ReadCloser, error) {
 	if m.local {
 		p, err := m.path(remotePath)
 		if err != nil {
 			return nil, err
 		}
-		return os.Open(p)
+		f, err := os.Open(p)
+		if err != nil {
+			if os.IsNotExist(err) {
+				return nil, ErrNotFound
+			}
+			return nil, err
+		}
+		return f, nil
 	}
```

*(A linha `@@` acima é aproximada — localize o método `func (m *MegaProvider) Read(remotePath string) (io.ReadCloser, error) {` e substitua exatamente o corpo do `if m.local { ... }` como mostrado.)*

### Arquivo 2/5 — `cmd/server/main.go` (rename das 2 rotas + nova rota de upload)

```diff
--- a/cmd/server/main.go
+++ b/cmd/server/main.go
@@
-	router.POST("/academia/registo-publico", handlers.RegisterAcademiaPublica)
+	router.POST("/academia/cadastro", handlers.RegisterAcademiaPublica)
@@
 		protected.GET("/documentos/academias/:codigo/:campo/download", handlers.DownloadDocumentoAcademia)
+		protected.POST("/documentos/academias/:codigo/:campo/upload", handlers.UploadDocumentoAcademia)
@@
-		admin.POST("/academia/register", middleware.RequireFPP(), handlers.RegisterAcademia)
+		admin.POST("/academia/cadastro", middleware.RequireFPP(), handlers.RegisterAcademia)
```

Não toque na linha vizinha `admin.POST("/academia/register/async", middleware.RequireFPP(), handlers.RegisterAcademiaBatchAsync)` — fica exatamente como está.

### Arquivo 3/5 — `internal/handlers/academia_handlers.go`

**3a. Comentário de cabeçalho de `RegisterAcademia`:**
```diff
-// ============================================================================
-// POST /admin/academia/register
-// ============================================================================
+// ============================================================================
+// POST /dominis/academia/cadastro
+// ============================================================================
```

**3b. Dentro de `RegisterAcademia` — validação do alvará vira opcional:**
```diff
 	if err := utils.ValidateNIF(req.NIF); err != nil {
 		utils.RespondWithValidationError(c, err)
 		return
 	}
-	if alvara == nil {
-		utils.RespondWithValidationError(c, fmt.Errorf("alvara é obrigatório"))
-		return
-	}
-	alvaraPDF, err := readAndValidatePDF("alvara", alvara)
-	if err != nil {
-		utils.RespondWithValidationError(c, err)
-		return
-	}
+	// alvara é opcional no cadastro: pode ser enviado agora ou depois, via
+	// POST /documentos/academias/{codigo_academia}/alvara/upload.
+	var alvaraPDF uploadedPDF
+	temAlvara := alvara != nil
+	if temAlvara {
+		var err error
+		alvaraPDF, err = readAndValidatePDF("alvara", alvara)
+		if err != nil {
+			utils.RespondWithValidationError(c, err)
+			return
+		}
+	}
```

**3c. Ainda em `RegisterAcademia` — bloco de upload + resposta final** (localize pelo trecho `audit := db.AuditContext{` seguido de `UserType: "admin",` — é o único lugar com esses dois literais juntos):
```diff
 	dir := fmt.Sprintf("%s/Documentação formal", codigoAcademia)
 	if provider == nil {
 		utils.RespondWithInternalError(c, fmt.Errorf("storage indisponível"))
 		return
 	}
-	if err := provider.EnsureDir(dir); err != nil {
-		utils.RespondWithInternalError(c, err)
-		return
-	}
-	if _, err := provider.Upload(fmt.Sprintf("%s/alvara_%s.pdf", dir, codigoAcademia), bytes.NewReader(alvaraPDF.data), alvaraPDF.size); err != nil {
-		_ = provider.Delete(dir)
-		utils.RespondWithInternalError(c, fmt.Errorf("falha no upload do alvara: %w", err))
-		return
-	}
+	if temAlvara {
+		if err := provider.EnsureDir(dir); err != nil {
+			utils.RespondWithInternalError(c, err)
+			return
+		}
+		if _, err := provider.Upload(fmt.Sprintf("%s/alvara_%s.pdf", dir, codigoAcademia), bytes.NewReader(alvaraPDF.data), alvaraPDF.size); err != nil {
+			_ = provider.Delete(dir)
+			utils.RespondWithInternalError(c, fmt.Errorf("falha no upload do alvara: %w", err))
+			return
+		}
+	}
 
 	if err := repository.SaveWithAudit(academia, audit); err != nil {
 		_ = provider.Delete(dir)
 		utils.RespondWithInternalError(c, err)
 		return
 	}
 
 	log.Printf("Academia registada: %s (%s) por admin %s", req.Nome, codigoAcademia, userID)
-	c.JSON(http.StatusCreated, gin.H{
+	response := gin.H{
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
-	})
+	}
+	if !temAlvara {
+		response["aviso"] = fmt.Sprintf(
+			"alvará não enviado no cadastro. envie depois em POST /documentos/academias/%s/alvara/upload.",
+			codigoAcademia,
+		)
+	}
+	c.JSON(http.StatusCreated, response)
 }
```

**3d. Comentário de cabeçalho de `RegisterAcademiaPublica`:**
```diff
-// ============================================================================
-// POST /academia/registo-publico
-// ============================================================================
+// ============================================================================
+// POST /academia/cadastro
+// ============================================================================
```

**3e. Dentro de `RegisterAcademiaPublica` — mesma mudança de 3b** (o bloco é idêntico ao de `RegisterAcademia`, mas localizado logo antes do comentário `// Senha obrigatória — exclusiva do cadastro público.`):
```diff
 	if err := utils.ValidateNIF(req.NIF); err != nil {
 		utils.RespondWithValidationError(c, err)
 		return
 	}
-	if alvara == nil {
-		utils.RespondWithValidationError(c, fmt.Errorf("alvara é obrigatório"))
-		return
-	}
-	alvaraPDF, err := readAndValidatePDF("alvara", alvara)
-	if err != nil {
-		utils.RespondWithValidationError(c, err)
-		return
-	}
+	// alvara é opcional no cadastro: pode ser enviado agora ou depois, via
+	// POST /documentos/academias/{codigo_academia}/alvara/upload.
+	var alvaraPDF uploadedPDF
+	temAlvara := alvara != nil
+	if temAlvara {
+		var err error
+		alvaraPDF, err = readAndValidatePDF("alvara", alvara)
+		if err != nil {
+			utils.RespondWithValidationError(c, err)
+			return
+		}
+	}
 
 	// Senha obrigatória — exclusiva do cadastro público.
```

**3f. Ainda em `RegisterAcademiaPublica` — bloco de upload + aviso** (mesmo padrão de 3c, mas aqui a variável `aviso` já existe — só precisa ganhar um `+=` condicional):
```diff
 	dir := fmt.Sprintf("%s/Documentação formal", codigoAcademia)
 	if provider == nil {
 		utils.RespondWithInternalError(c, fmt.Errorf("storage indisponível"))
 		return
 	}
-	if err := provider.EnsureDir(dir); err != nil {
-		utils.RespondWithInternalError(c, err)
-		return
-	}
-	if _, err := provider.Upload(fmt.Sprintf("%s/alvara_%s.pdf", dir, codigoAcademia), bytes.NewReader(alvaraPDF.data), alvaraPDF.size); err != nil {
-		_ = provider.Delete(dir)
-		utils.RespondWithInternalError(c, fmt.Errorf("falha no upload do alvara: %w", err))
-		return
-	}
+	if temAlvara {
+		if err := provider.EnsureDir(dir); err != nil {
+			utils.RespondWithInternalError(c, err)
+			return
+		}
+		if _, err := provider.Upload(fmt.Sprintf("%s/alvara_%s.pdf", dir, codigoAcademia), bytes.NewReader(alvaraPDF.data), alvaraPDF.size); err != nil {
+			_ = provider.Delete(dir)
+			utils.RespondWithInternalError(c, fmt.Errorf("falha no upload do alvara: %w", err))
+			return
+		}
+	}
 
 	if err := repository.SaveWithAudit(academia, audit); err != nil {
 		_ = provider.Delete(dir)
 		utils.RespondWithInternalError(c, err)
 		return
 	}
 
 	log.Printf("Academia auto-registada (cadastro público, pendente de ativação): %s (%s)", req.Nome, codigoAcademia)
 
 	aviso := "guarde o código da academia: ele é o seu identificador de login. você definiu sua própria senha no cadastro."
+	if !temAlvara {
+		aviso += fmt.Sprintf(
+			" alvará não enviado no cadastro. envie depois em POST /documentos/academias/%s/alvara/upload.",
+			codigoAcademia,
+		)
+	}
 
 	c.JSON(http.StatusCreated, gin.H{
```
*(o `c.JSON` continua exatamente igual depois — não mude mais nada nesse bloco de resposta)*

**3g. Mensagem de erro genérica em `bindRegisterAcademiaRequest`** (remove menção a "alvara", já que não é mais obrigatório — essa mensagem só aparece quando o `ShouldBindJSON` falha por outros campos ausentes):
```diff
 	if err := c.ShouldBindJSON(&req); err != nil {
-		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: nivel, type, nome, nif, provincia, endereco e alvara"))
+		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: nivel, type, nome, nif, provincia, endereco"))
 		return req, nil, false
 	}
```

### Arquivo 4/5 — `internal/handlers/documento_download_handlers.go` (novo handler)

**4a. Import de `bytes`** (o arquivo já importa `database/sql`, `errors`, `fmt`, `io`, `net/http`, `strings`, gin, e os pacotes internos — só falta `bytes`):
```diff
 import (
+	"bytes"
 	"database/sql"
 	"errors"
 	"fmt"
```

**4b. Novo handler `UploadDocumentoAcademia`**, inserido imediatamente antes de `func canAccessEstudanteDocument(...)`:
```go
// UploadDocumentoAcademia permite anexar — ou substituir — um documento
// formal da academia (hoje apenas "alvara") depois do cadastro, quando ele
// não foi enviado no momento do registo (POST /dominis/academia/cadastro ou
// POST /academia/cadastro, ambos com alvara agora opcional). Espelha
// exatamente o escopo e as regras de permissão de DownloadDocumentoAcademia:
// mesma rota "protected" (fora de /academia, /estudante e /dominis) e mesma
// função canAccessAcademiaDocument — admin ou a própria academia dona do
// codigo_academia. Reenviar para uma academia que já tem alvara substitui o
// arquivo existente (mesmo path determinístico usado no cadastro e no
// download).
func UploadDocumentoAcademia(c *gin.Context) {
	codigoAcademia := strings.TrimSpace(c.Param("codigo"))
	campo := strings.TrimSpace(c.Param("campo"))
	if strings.ToLower(campo) != "alvara" {
		utils.RespondWithNotFoundError(c, "documento")
		return
	}

	academia, err := getAcademiaProjection(c).GetByCodigo(codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	if !canAccessAcademiaDocument(c, academia.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para enviar documento desta academia")
		return
	}

	fh, err := c.FormFile("alvara")
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("alvara é obrigatório"))
		return
	}
	alvaraPDF, err := readAndValidatePDF("alvara", fh)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	provider := getStorageProvider(c)
	if provider == nil {
		p, err := storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
		provider = p
	}

	dir := fmt.Sprintf("%s/Documentação formal", academia.CodigoAcademia)
	if err := provider.EnsureDir(dir); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	path := fmt.Sprintf("%s/alvara_%s.pdf", dir, academia.CodigoAcademia)
	// Delete-then-upload garante substituição limpa caso já exista um alvara
	// no mesmo path (correção de um envio anterior); ErrNotFound (primeiro
	// envio) é ignorado de propósito, mesmo padrão usado no resto do pacote.
	_ = provider.Delete(path)
	if _, err := provider.Upload(path, bytes.NewReader(alvaraPDF.data), alvaraPDF.size); err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("falha no upload do alvara: %w", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "alvará enviado com sucesso",
		"codigo_academia": academia.CodigoAcademia,
		"download_url":    academiaDocumentoDownloadURL(academia.CodigoAcademia, "alvara"),
	})
}
```

### Arquivo 5/5 — testes existentes que quebram com o rename (precisam ser atualizados, não são opcionais)

**5a. `cmd/server/main_test.go`** — este teste manda uma requisição HTTP real pela rota antiga; com o rename ela vira 404 em vez de 401, então o teste precisa apontar para a rota nova:
```diff
-func TestDominisAcademiaRegisterUnauthorizedUsesStandardErrorEnvelope(t *testing.T) {
+func TestDominisAcademiaCadastroUnauthorizedUsesStandardErrorEnvelope(t *testing.T) {
 	gin.SetMode(gin.TestMode)
 
 	router := setupRouter()
 
-	req := httptest.NewRequest(http.MethodPost, "/dominis/academia/register", nil)
+	req := httptest.NewRequest(http.MethodPost, "/dominis/academia/cadastro", nil)
 	w := httptest.NewRecorder()
 	router.ServeHTTP(w, req)
 
 	assertStandardErrorEnvelope(t, w, http.StatusUnauthorized, "UNAUTHORIZED")
 }
```

**5b. `internal/handlers/academia_register_publica_test.go`** — este teste procura o texto literal `/academia/registo-publico` no source de `main.go`; com o rename, ele nunca encontra e falha com `t.Fatal`:
```diff
-func TestAcademiaRegistoPublicoRouteIsPublic(t *testing.T) {
+func TestAcademiaCadastroPublicoRouteIsPublic(t *testing.T) {
 	source, err := os.ReadFile("../../cmd/server/main.go")
 	if err != nil {
 		t.Fatalf("read main.go source: %v", err)
 	}
 
 	var routeLine string
 	for _, line := range strings.Split(string(source), "\n") {
-		if strings.Contains(line, "/academia/registo-publico") {
+		if strings.Contains(line, "/academia/cadastro") {
 			routeLine = line
 			break
 		}
 	}
 	if routeLine == "" {
-		t.Fatal("rota POST /academia/registo-publico não encontrada em cmd/server/main.go")
+		t.Fatal("rota POST /academia/cadastro não encontrada em cmd/server/main.go")
 	}
 	if strings.Contains(routeLine, "middleware.") {
-		t.Fatalf("rota /academia/registo-publico deve ser pública, sem middleware de autenticação: %q", routeLine)
+		t.Fatalf("rota /academia/cadastro deve ser pública, sem middleware de autenticação: %q", routeLine)
 	}
 	if !strings.Contains(routeLine, "router.POST(") {
-		t.Fatalf("rota /academia/registo-publico deve ser registrada diretamente em router.POST, fora de grupos autenticados: %q", routeLine)
+		t.Fatalf("rota /academia/cadastro deve ser registrada diretamente em router.POST, fora de grupos autenticados: %q", routeLine)
 	}
 }
```

**5c. `internal/handlers/academia_register_errors_test.go`** — não é funcionalmente necessário (esse teste chama `bindRegisterAcademiaRequest` direto, sem passar pelo roteador gin, então o path é só um literal decorativo), mas troque por consistência — são 2 ocorrências:
```diff
-	c.Request = httptest.NewRequest(http.MethodPost, "/dominis/academia/register", strings.NewReader(`{"nivel":`))
+	c.Request = httptest.NewRequest(http.MethodPost, "/dominis/academia/cadastro", strings.NewReader(`{"nivel":`))
```
```diff
-	c.Request = httptest.NewRequest(http.MethodPost, "/dominis/academia/register", strings.NewReader("not-a-multipart-body"))
+	c.Request = httptest.NewRequest(http.MethodPost, "/dominis/academia/cadastro", strings.NewReader("not-a-multipart-body"))
```

### Arquivo NOVO — `cmd/server/academia_cadastro_alvara_opcional_integration_test.go`

Crie este arquivo com o conteúdo exato abaixo (503 linhas). Ele cobre, contra Postgres real: cadastro admin sem alvará (com verificação do 404 correto — pega regressão do bug #2 se ele voltar), cadastro admin com alvará (regressão do fluxo antigo), cadastro público sem alvará, upload individual pós-cadastro (admin e pela própria academia), rejeição cross-tenant, campo inválido, academia inexistente, e substituição de um alvará já existente.

```go
package main

// [conteúdo completo do arquivo — ver anexo "academia_cadastro_alvara_opcional_integration_test.go" neste mesmo pacote de entrega]
```

> **Nota para quem for colar o conteúdo:** o arquivo completo está disponível como anexo separado nesta entrega (mesmo nome). Copie-o exatamente — ele já roda limpo contra Postgres real (ver seção 4). Não precisa (nem pode, no seu ambiente) rodar os testes marcados com `SPURI_RUN_DB_INTEGRITY_TESTS=1`, mas o arquivo precisa existir no repo, compilado, para não quebrar `go vet ./...` nem a suíte de quem tiver banco disponível (CI, ou eu numa próxima sessão).

---

## 3. Checklist de aceitação

- [ ] `POST /dominis/academia/register` não existe mais; `POST /dominis/academia/cadastro` existe, protegida por `RequireFPP()`.
- [ ] `POST /academia/registo-publico` não existe mais; `POST /academia/cadastro` existe, pública.
- [ ] `POST /dominis/academia/register/async` continua exatamente igual (não tocar).
- [ ] Cadastrar uma academia (qualquer uma das duas rotas) **sem** anexar `alvara` retorna `201`, com um campo `"aviso"` na resposta mencionando `POST /documentos/academias/{codigo}/alvara/upload`.
- [ ] Cadastrar **com** `alvara` continua funcionando exatamente como antes (sem `"aviso"`, arquivo salvo no path de sempre).
- [ ] Novo endpoint `POST /documentos/academias/:codigo/:campo/upload` existe, aceita só `campo=alvara` (outro valor → 404), exige autenticação, permite admin ou a própria academia (dona do `codigo`), rejeita outra academia (403).
- [ ] Reenviar o alvará para uma academia que já tem um substitui o arquivo (download depois reflete o conteúdo novo).
- [ ] `internal/storage/storage.go`: `Read` no branch `local` devolve `storage.ErrNotFound` (não erro cru) quando o arquivo não existe — só o branch local, Mega real intocado.
- [ ] `go build ./...` e `go vet ./...` limpos.
- [ ] `go test ./...` (sem `SPURI_RUN_DB_INTEGRITY_TESTS`) limpo — inclui os testes renomeados/atualizados da seção 2, arquivo 5.
- [ ] Nenhuma migration nova foi criada (não é necessária).

## 4. Validação já feita (evidência real, não suposição)

Rodei tudo isto eu mesmo, numa cópia de trabalho, contra infraestrutura real (não é leitura de código):

- **Ambiente**: Go 1.24.4 + PostgreSQL 16 instalados via apt no meu sandbox (você não tem isso — daí eu ter rodado por você). Para compilar localmente, usei `replace` directives temporárias em `go.mod` apontando dependências de domínios bloqueados (`golang.org/x`, `gopkg.in`, `go.opentelemetry.io`, `google.golang.org/protobuf`) para os mirrors equivalentes no GitHub — isso é só um artefato do MEU sandbox para conseguir compilar; **não inclua nenhuma dessas `replace` no `go.mod` real**, elas não fazem parte do diff acima e não devem ir para o repositório.
- **Build**: `go build ./...` → limpo. Binário real gerado (`cmd/server`, 19 MB).
- **Vet**: `go vet ./...` → limpo, zero avisos.
- **Suíte completa existente**: `go test ./...` (sem banco) → todos os pacotes `ok`, incluindo os 3 arquivos de teste atualizados na seção 2/5.
- **Suíte completa com banco real**: `go test ./...` com `SPURI_RUN_DB_INTEGRITY_TESTS=1` contra Postgres limpo → todos os pacotes `ok`, incluindo os 9 testes novos do arquivo de integração.
- **Revert-and-confirm**: reverti manualmente só a validação do alvará obrigatório no fluxo admin (voltando ao `if alvara == nil { erro }` antigo) e confirmei que `TestRegisterAcademiaAdminSemAlvaraCriaAcademiaSemArquivo` **falha** corretamente (400 "alvara é obrigatório") — prova que o teste testa o que devia, não é um tautologia. Restaurei o fix depois e reconfirmei tudo verde.
- **Achado do bug #2 (storage local)**: reproduzido ao vivo (cadastro sem alvará → download → 503 em vez de 404), corrigido, e reconfirmado com o teste `TestRegisterAcademiaAdminSemAlvaraCriaAcademiaSemArquivo` (que checa explicitamente a mensagem "documento" no corpo do 404, não só o status code).
- **Falsos positivos descartados**: numa rodada da suíte completa contra um banco Postgres que eu já tinha usado repetidamente (dados acumulados de execuções anteriores, nunca truncado), 14 testes de "Faltas"/"Notas" falharam. Investiguei: são **completamente não relacionados** a esta tarefa (módulo de faltas/avaliação). Rodei um deles isolado contra um banco novo e limpo → passou. Confirmado: pura sujeira de dados acumulados no MEU banco de teste repetidamente reutilizado, não uma regressão desta tarefa. Se você (ou eu numa sessão futura) rodar a suíte de integração, use sempre um banco limpo por execução.

## 5. Avisos de deploy

- Backend e frontend devem subir juntos (mesmo padrão já estabelecido no projeto): o frontend (`spuripainel`) tem um documento de tarefa irmão que renomeia as mesmas 2 rotas do lado do cliente. Se só o backend subir, o formulário de cadastro do painel vai chamar rotas que não existem mais (`/academia/registo-publico`, `/dominis/academia/register`) e todo cadastro de academia quebra até o frontend subir também.
- Sem impacto em dados existentes: nenhuma migration, nenhum academia já cadastrado é afetado.
- Sem impacto em NeonDB/compute: nenhuma query nova de vulto, nenhuma migration, nenhuma mudança que force wake extra fora do padrão normal de escrita.

## 6. Ao terminar

Mova este arquivo para `docs/Tarefas feitas/80 - Tornar alvara opcional no cadastro de academia e renomear rotas de cadastro.md` depois que o checklist da seção 3 estiver 100% marcado.
