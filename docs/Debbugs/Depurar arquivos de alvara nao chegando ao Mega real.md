---
data: 2026-08-25
status: corrigido_via_65_corrigir_fallback_local_silencioso_do_storage_mega_e_criar_migracao_de_recuperacao
auditor: Claude (orquestrador) — depuração com PostgreSQL 16 e Go 1.24 reais em sandbox, testes ao vivo do binário compilado, e simulação completa da tarefa de correção num clone novo do zero
tarefa_correcao: docs/Lista de Tarefas/65 - Corrigir fallback local silencioso do storage Mega e criar migracao de recuperacao.md
---

# Depuração — arquivos de alvará de "cadastros independentes" de academia não aparecem no Mega real

## Como o problema foi relatado

Fredy fez alguns cadastros de academia pelo fluxo público/independente (`POST /academia/registo-publico`) e reparou que os diretórios e arquivos de alvará correspondentes não apareciam na conta Mega real (via mega.nz), mas o próprio sistema continuava conseguindo retornar esses documentos normalmente para visualização e download do cliente. Pediu explicitamente que o armazenamento seja **apenas** no Mega — nenhum arquivo pode ser salvo em qualquer outro lugar.

## Investigação

O upload e o download de documentos da academia (`RegisterAcademia`/`RegisterAcademiaPublica` em `internal/handlers/academia_handlers.go`, e `DownloadDocumentoAcademia` em `internal/handlers/documento_download_handlers.go`) não têm dois caminhos possíveis de armazenamento — os dois sempre passam pelo mesmo `storage.StorageProvider`, resolvido na inicialização do processo (`initStorage()`, `cmd/server/main.go`) ou, quando ausente do contexto, resolvido de novo em cada handler via `storage.NewStorageProvider()`. Ou seja: se o upload "funcionou" e o download "funciona", os dois estão necessariamente falando com o mesmo backend — a pergunta certa não é "existe um bug fazendo o download ler de outro lugar" (não existe), é "qual backend esse provider resolveu ser, de fato, nesta instância, e o que acontece quando essa resolução falha?". A investigação encontrou dois problemas distintos, o segundo exposto ao corrigir o primeiro.

### Bug 1 — o fallback local ativava sozinho, por causa de `ENV`

`storage.NewStorageProvider()` (`internal/storage/storage.go`) decide o backend a partir de `STORAGE_PROVIDER`, mas a função que decide se cai para o fallback local (`MegaProvider{local: true}`, que salva tudo num diretório local em vez de falar com o Mega) tinha duas brechas:

```go
func useLocalMegaFallback() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "test") || strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER"))) == "local"
}
```

1. `ENV=test` ativava o fallback local mesmo com `STORAGE_PROVIDER=mega` definido explicitamente e credenciais Mega válidas configuradas.
2. `ENV=test` também ativava o fallback local quando `STORAGE_PROVIDER` estava completamente indefinido.

`ENV=test` é usado, para propósitos completamente diferentes, para colocar a AppyPay em modo sandbox (`internal/finance/appypay.go`) e para relaxar a exigência de `JWT_SECRET` em testes (`internal/middleware/auth.go`). Qualquer ambiente com `ENV=test` — por qualquer motivo, mesmo sem nenhuma relação com storage — desativava o Mega real para arquivos silenciosamente: sem erro, sem log, sem aviso. Dois agravantes tornavam isso praticamente indetectável: **`ProviderName()` sempre retornava `"mega"`**, mesmo em modo local (inclusive no endpoint de diagnóstico `GET /dominis/storage/quota`), e **não havia log de inicialização** dizendo qual backend tinha sido escolhido. O diretório do fallback local (`data/mega_storage` por padrão) também não tem volume persistente declarado no `Dockerfile` — numa plataforma como o Render, sem disco persistente anexado, é efêmero: os arquivos ali são perdidos no próximo redeploy/restart.

### Prova ao vivo do Bug 1 (binário compilado, PostgreSQL 16 real)

Compilei o binário do `main` original e subi com `STORAGE_PROVIDER=mega`, `ENV=test`, **sem** `MEGA_EMAIL`/`MEGA_PASSWORD` reais:

- O servidor subiu normalmente, sem nenhum erro ou aviso relacionado a storage nos logs.
- Chamei `POST /academia/registo-publico` com um PDF de alvará, exatamente como o cadastro independente de Fredy — resposta `201`, `codigo_academia: "LDA20261"`, sucesso.
- O arquivo apareceu em `data/mega_storage/LDA20261/Documentação formal/alvara_LDA20261.pdf` no disco local — nunca tentou falar com o Mega real (nenhuma linha de log de tentativa de login).

Isso reproduz exatamente o sintoma relatado. Com o binário corrigido, a mesma combinação de variáveis agora falha alto na inicialização:

```
[ERROR] Erro ao inicializar armazenamento: configuração de storage inválida: MEGA_EMAIL e MEGA_PASSWORD são obrigatórios quando STORAGE_PROVIDER=mega
```

### Correção do Bug 1 — regra única e definitiva

Fredy pediu explicitamente que armazenamento seja **só** no Mega, sem exceção. Por isso a correção não parou em "não deixar `ENV=test` sobrescrever `STORAGE_PROVIDER=mega` explícito" — foi além, fechando a segunda brecha também: `useLocalMegaFallback()` passou a ignorar `ENV` completamente, em qualquer situação:

```go
func useLocalMegaFallback() bool {
	return strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER"))) == "local"
}
```

Agora **`STORAGE_PROVIDER=local`, explícito, é o único jeito de ativar o fallback local** — nenhum valor de `ENV`, em nenhuma combinação, influencia essa decisão. Testei ao vivo essa segunda brecha também: subi o binário corrigido com `STORAGE_PROVIDER` completamente indefinido e `ENV=test` (a combinação que antes ainda escapava) — o servidor recusou subir, com o mesmo erro claro acima, confirmando que a segunda brecha também está fechada.

Complementando: `ProviderName()` passa a retornar `"mega-local"` (nunca mais `"mega"`) quando de fato está em modo local; `cmd/server/main.go` passa a logar, em toda inicialização, qual backend está ativo, com `[ALERTA]` se cair em modo local com `ENV=production`; e uma ferramenta de recuperação nova (`internal/storage/migration.go` + `POST /dominis/storage/migrar-local-para-mega`, role `fpp`) reenvia para o Mega real qualquer arquivo que tenha ficado só no fallback local — sem apagar nem sobrescrever nada, idempotente.

### Bug 2 — exposto pela correção do Bug 1: erro de storage ignorado silenciosamente

Ao aplicar a regra "só `STORAGE_PROVIDER=local` explícito ativa o fallback local" e rodar a suíte de testes completa (com PostgreSQL 16 real) para validar, um teste de integração pré-existente (`cmd/server/turma_vinculo_estudante_integration_test.go`, que cadastra um estudante com upload de BI e cédula) passou a falhar — não com um erro de storage, mas com **panic de nil pointer**.

Investigando a causa, encontrei um segundo bug, real e mais sério, que estava simplesmente mascarado até agora: **7 pontos do código**, em 3 arquivos (`internal/handlers/estudante_handlers.go`, duas ocorrências; `internal/handlers/academia_handlers.go`, três ocorrências idênticas; `internal/handlers/solicitacao_matricula_handlers.go`, uma ocorrência), chamam `storage.NewStorageProvider()` e **ignoram o erro retornado**:

```go
provider := getStorageProvider(c)
if provider == nil {
	p, _ := storage.NewStorageProvider() // erro descartado
	provider = p
}
```

Quando `NewStorageProvider()` falha, `provider` fica `nil`, e a primeira chamada de método sobre ele (`provider.EnsureDir(...)`, `provider.Upload(...)`) causa um `panic`. Isso nunca aparecia nos testes porque, antes da correção do Bug 1, `ENV=test` quase sempre fazia `NewStorageProvider()` "funcionar" (via fallback local) — o caminho de erro nunca era exercitado. **Sem esta segunda correção, qualquer falha real do Mega em produção** (credenciais erradas, API fora do ar, rede instável) faria cadastro de academia, cadastro de estudante, criação de solicitação de matrícula e conclusão de documentos pendentes **quebrarem com panic/500 genérico**, em vez de um erro HTTP claro.

### Correção do Bug 2

Os 7 pontos passaram a tratar o erro corretamente — mesmo padrão já usado, corretamente, em `internal/handlers/documento_download_handlers.go`:

```go
provider := getStorageProvider(c)
if provider == nil {
	p, err := storage.NewStorageProvider()
	if err != nil {
		utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
		return
	}
	provider = p
}
```

O teste de integração que dependia do comportamento antigo passou a configurar `STORAGE_PROVIDER=local` explicitamente, do jeito que sempre deveria ter feito.

## Validação executada

- **Baseline**: `go build ./...`, `go vet ./...`, `gofmt -l`, e `go test ./...` completo (com `RUN_POSTGRES_INTEGRATION=1 SPURI_RUN_DB_INTEGRITY_TESTS=1`, PostgreSQL 16 real, banco recriado do zero) rodados no `main` original, sem nenhuma alteração — 100% verde, para ter certeza de que qualquer falha depois seria minha, não pré-existente.
- **Depois de cada correção** (Bug 1, depois Bug 1+2 juntos): mesma bateria completa — build, vet, gofmt, suíte de testes inteira com Postgres real e banco fresco — 100% verde nas duas rodadas, incluindo o teste que antes falhava com panic.
- **Testes ao vivo do binário compilado**: as duas combinações exatas que causavam os bugs (`STORAGE_PROVIDER=mega`+`ENV=test` sem credenciais; `STORAGE_PROVIDER` indefinido+`ENV=test`) foram reproduzidas contra o binário original (confirmando os bugs) e contra o binário corrigido (confirmando a correção — falha alta e clara em vez de fallback silencioso).
- **Simulação completa da tarefa de correção**: para garantir que o documento de tarefa (`docs/Lista de Tarefas/65 - ...md`) pudesse ser executado mecanicamente sem nenhuma ambiguidade, apliquei — à risca, copiando o texto do próprio documento — cada um dos 17 blocos de "localizar/substituir" e "arquivo novo" num clone novo e limpo do repositório, do zero. Todos os blocos foram encontrados e substituídos sem nenhum erro de correspondência, e a bateria completa (build, vet, gofmt, `go test ./...` com Postgres real) rodou 100% verde nesse clone simulado.

## Risco residual — ação recomendada antes de qualquer redeploy

Se algum ambiente do Fredy (provavelmente o de teste/staging no Render, a julgar pelo histórico recente de troubleshooting de deploy) estiver sem `STORAGE_PROVIDER=mega` explícito — indefinido, ou setado para `local` — os alvarás das academias cadastradas recentemente existem **apenas** no disco daquela instância específica; um redeploy os apaga de vez, sem aviso. Recomendo: aplicar esta correção o quanto antes, confirmar via log de inicialização (`[INFO]`/`[ALERTA] armazenamento de arquivos: ...`) qual modo cada ambiente está usando e — se algum ambiente ainda tiver `data/mega_storage` com arquivos de uma execução anterior no modo local — corrigir `STORAGE_PROVIDER=mega` com credenciais reais, subir de novo e chamar `POST /dominis/storage/migrar-local-para-mega` antes do próximo redeploy, para recuperar esses arquivos.

Depois desta correção, se o Mega ficar fora do ar ou mal configurado em produção, os quatro fluxos afetados (cadastro de academia, cadastro de estudante, solicitação de matrícula, conclusão de documentos pendentes) vão responder `503` com mensagem clara — esse é o efeito pretendido do Bug 2 corrigido, não uma regressão.

## O que o Codex precisa fazer

Tudo já implementado, testado e validado com PostgreSQL e Go reais, incluindo testes ao vivo do binário compilado e uma simulação completa da própria tarefa de correção num clone novo. Seguir `docs/Lista de Tarefas/65 - Corrigir fallback local silencioso do storage Mega e criar migracao de recuperacao.md` mecanicamente.
