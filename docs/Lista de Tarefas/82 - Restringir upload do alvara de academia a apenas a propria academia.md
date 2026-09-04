---
criado: 04-09-2026 00:00
origem: Fredy + Claude (orquestração) — correção pós-implementação da Tarefa 81
status: pronto para execução
tipo: backend (spuri-backend)
---

# Tarefa 82 — Upload do alvará da academia deve ser exclusivo da própria academia

### Documento de execução para o Codex (orquestrado e pré-testado pelo Claude)

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Mudança pequena e isolada — um único arquivo Go. Ainda assim, validei com Go 1.24 real no meu sandbox (contornando o bloqueio de rede a `golang.org/x/*` do meu ambiente com `replace` directives no `go.mod` apontando para os mirrors em `github.com/golang/*`, revertidas depois — você não precisa disso, seu ambiente resolve os módulos normalmente):

- `go build ./...`, `go vet ./...`, `gofmt -l .`: limpos.
- `go test ./...` (sem integração Postgres): 100% verde, incluindo o pacote `internal/handlers` inteiro.

Não precisa de Postgres, Docker nem planejamento nenhum aqui — só aplicar o bloco da seção 2 e confirmar que compila/testa limpo.

## 1. Prompt recomendado para executar esta correção

> Aplique exatamente o bloco "localizar/substituir" da seção 2 deste documento. Depois rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...` na raiz do repositório e confirme que está tudo limpo. Ao final, siga o "Procedimento de conclusão" (seção 4).

## 2. Contexto

Depois de revisar a Tarefa 81 (NIF não-único) já implantada, o Fredy percebeu um problema **pré-existente**, não introduzido por aquela tarefa: o endpoint `POST /documentos/academias/{codigo_academia}/alvara/upload` permite que **qualquer admin** envie/substitua o alvará de **qualquer academia**. Isso está errado — "apenas a academia pode manipular seus dados". O admin pode continuar **consultando** o alvará (`GET .../download`) para fins de fiscalização; só a escrita (upload/substituição) precisa ficar restrita à própria academia.

A causa é que `UploadDocumentoAcademia` (`internal/handlers/documento_download_handlers.go`) reaproveita `canAccessAcademiaDocument`, a mesma função de autorização usada por `DownloadDocumentoAcademia` — e essa função concede acesso irrestrito a qualquer admin (`if userType == "admin" { return true }`), sem diferenciar leitura de escrita.

### 2.1 — `internal/handlers/documento_download_handlers.go`

**2.1.1 — Localizar este bloco exato** (dentro de `UploadDocumentoAcademia`):

```go
	if !canAccessAcademiaDocument(c, academia.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para enviar documento desta academia")
		return
	}
```

**Substituir por:**

```go
	if !canUploadAcademiaDocument(c, academia.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "apenas a própria academia pode enviar ou atualizar este documento")
		return
	}
```

**2.1.2 — Localizar este bloco exato** (logo após a definição de `canAccessAcademiaDocument`, antes de `canAccessSolicitacaoEdicaoDocument`):

```go
func canAccessAcademiaDocument(c *gin.Context, codigoAcademia string) bool {
	userType, _ := middleware.GetUserType(c)
	if userType == "admin" {
		return true
	}
	if userType != "academia" {
		return false
	}
	userID, _ := middleware.GetUserID(c)
	academia, _ := getAcademiaProjection(c).GetByID(userID)
	return academia != nil && academia.CodigoAcademia == codigoAcademia
}

func canAccessSolicitacaoEdicaoDocument(c *gin.Context, codigoEstudante, codigoAcademia string) bool {
```

**Substituir por:**

```go
func canAccessAcademiaDocument(c *gin.Context, codigoAcademia string) bool {
	userType, _ := middleware.GetUserType(c)
	if userType == "admin" {
		return true
	}
	if userType != "academia" {
		return false
	}
	userID, _ := middleware.GetUserID(c)
	academia, _ := getAcademiaProjection(c).GetByID(userID)
	return academia != nil && academia.CodigoAcademia == codigoAcademia
}

// canUploadAcademiaDocument, diferente de canAccessAcademiaDocument, NÃO
// concede acesso a admin: enviar/substituir um documento formal da academia
// (ex.: alvará) é uma escrita nos dados da própria academia, exclusiva dela
// — admin pode consultar (canAccessAcademiaDocument), mas não alterar.
func canUploadAcademiaDocument(c *gin.Context, codigoAcademia string) bool {
	userType, _ := middleware.GetUserType(c)
	if userType != "academia" {
		return false
	}
	userID, _ := middleware.GetUserID(c)
	academia, _ := getAcademiaProjection(c).GetByID(userID)
	return academia != nil && academia.CodigoAcademia == codigoAcademia
}

func canAccessSolicitacaoEdicaoDocument(c *gin.Context, codigoEstudante, codigoAcademia string) bool {
```

Não mexa em `DownloadDocumentoAcademia`/`streamDocumentoAcademiaPorCodigo` — eles continuam usando `canAccessAcademiaDocument` sem alteração, então admin continua podendo **visualizar** o alvará normalmente.

### 2.2 — `Documentação da API.md`

**Localizar este bloco exato** (dentro de `### POST /documentos/academias/{codigo_academia}/alvara/upload`):

```
**Proteção**: autenticado. Permitido para admin ou para a própria academia dona do `codigo_academia`. Estudantes e academias de outro código recebem `403 Forbidden`.

**Request**: `multipart/form-data`.
```

**Substituir por:**

```
**Proteção**: autenticado. Permitido **apenas para a própria academia dona** do `codigo_academia` — diferente do endpoint de download, admin não pode enviar/substituir este documento (só consultar). Estudantes, academias de outro código e qualquer admin recebem `403 Forbidden`.

**Request**: `multipart/form-data`.
```

Não toque no bloco equivalente de `### GET /documentos/academias/{codigo_academia}/alvara/download` — a proteção desse endpoint (admin ou a própria academia) continua correta e não muda.

## 3. Checklist de validação

- [ ] `go build ./...`, `go vet ./...`, `gofmt -l .` limpos.
- [ ] `go test ./...` (sem integração) 100% verde.
- [ ] `grep -n "canAccessAcademiaDocument" internal/handlers/documento_download_handlers.go` mostra a função ainda usada em `streamDocumentoAcademiaPorCodigo` (download) — não removida.

## Critérios de aceite

1. Uma academia autenticada consegue enviar/atualizar seu próprio alvará normalmente (sem regressão).
2. Um admin autenticado tentando `POST /documentos/academias/{qualquer_codigo}/alvara/upload` recebe `403`, mesmo sendo `fpp`.
3. Um admin autenticado continua conseguindo `GET /documentos/academias/{qualquer_codigo}/alvara/download` normalmente (sem regressão).
4. Uma academia tentando enviar o alvará de **outra** academia continua recebendo `403` (comportamento já existente, preservado).

## 4. Procedimento de conclusão

Depois de tudo validado, mova este arquivo para `docs/Tarefas feitas/`, renomeando para o padrão já usado nesse diretório (ex.: `82 - Restringir upload do alvara de academia a apenas a propria academia.md`).

**Coordenação de deploy**: a tarefa irmã do frontend (`Tarefa - Corrigir localizacao das personalizacoes de academia e criar pagina de solicitacoes de NIF (Frontend).md`, no repositório `spuripainel`) remove o botão de upload do lado do admin. Não há dependência estrita de ordem entre elas — mesmo que o frontend suba primeiro (botão já não aparece) ou o backend suba primeiro (endpoint já rejeita, e o front antigo mostraria um erro genérico até o deploy do front acompanhar) — mas o ideal é subir as duas próximas uma da outra.
