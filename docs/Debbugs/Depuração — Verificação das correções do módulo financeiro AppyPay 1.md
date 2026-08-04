---
modificado: 2026-07-31 4:04
criado: 2026-07-31 4:04
---
> **Nota de arquivo (rollback):** o módulo financeiro/AppyPay descrito e auditado
> neste relatório foi removido por completo do código em `2026-08-03`
> (ver `docs/Lista de Tarefas/17 - Remover completamente o modulo financeiro
> AppyPay (rollback total).md`). Este documento é mantido apenas como
> referência histórica das vulnerabilidades encontradas, para orientar uma
> futura reimplementação mais robusta a partir da tarefa 15.

# Depuração — Verificação das correções do módulo financeiro AppyPay 1

## Objetivo

Verificar, item a item e direto no código atual (`internal/finance/financeiro.go`, `internal/projections/financeiro_projection.go`, `internal/handlers/financeiro_handlers.go`, `cmd/server/main.go`, migrations `097`/`098`), se as correções descritas em `docs/Debbugs/Correções — Módulo base de gestão financeira com AppyPay.md` foram de fato implementadas como descrito, e procurar ativamente por vulnerabilidades novas ou remanescentes — inclusive as que a própria correção pode ter introduzido.

## Resultado geral

Os **4 dos 6 itens** reivindicados na correção foram **confirmados como corrigidos** no código. Porém a auditoria encontrou **uma vulnerabilidade crítica não relacionada às correções anteriores** (chave de criptografia com fallback previsível) e **uma regressão real introduzida pela própria correção do item 4/replay** (duplicação de histórico auditável). Ambas passaram despercebidas porque a suíte de testes atual nunca exercita o par `Service` + `FinanceiroProjection` real contra Postgres simultaneamente.

---

## ✅ Correções confirmadas como resolvidas

|#|Achado original|Status|
|---|---|---|
|1|Estudante contornava isolamento em `ListarCredenciais`/`ObterCredencial`/`TestarCredencial`|**Resolvido.** As três funções agora usam `podeAcessarCredencial` (nega por padrão), e o grupo `/financeiro` em `main.go` ganhou `middleware.RequireAcademiaOuAdmin()` como defesa em profundidade. Teste `TestEstudanteNaoAcessaCredenciaisFinanceiras` cobre o caso.|
|3|`motivo` nunca chegava ao payload do evento|**Resolvido.** `record()` agora extrai `payload["motivo"]` e grava em `Metadata`; todas as chamadas identificadas (`AlterarStatusCredencial`, `AlterarModalidade`, `CancelarCobrancaFinanceiraBase`, `ReembolsarCobrancaFinanceiraBase`, `ReverterCobrancaFinanceiraBase`) passam `motivo` explicitamente via `payloadWithMotivo` ou literal no map.|
|4|Ciphertext embutido na projeção pública `financeiro_credenciais_appypay`|**Resolvido.** `ClientSecretEncrypted`, `WebhookSecretEncrypted`, `APIKey`, `APIKeyEncrypted` agora têm `json:"-"`; o ciphertext vai para `financeiro_segredos_appypay` via `projectCredencialSegredos`/`loadCredencialSegredos`. Confirmado que um `Rebuild()` da projeção não apaga o cofre (tabelas diferentes), então o ciphertext sobrevive a um rebuild administrativo.|
|5|Ausência de `decrypt()`|**Resolvido** como utilitário (existe, simétrico a `encrypt()`, testado em `TestEncryptDecryptSegredoFinanceiro`). Continua sem uso em runtime real, o que é esperado — não há `Provider` HTTP real ainda.|

---

## 🔴 CRÍTICO (novo) — Chave de criptografia cai num valor previsível em produção por checar a variável de ambiente errada

`encrypt()` e `decrypt()` (`internal/finance/financeiro.go`) só exigem `FINANCE_ENCRYPTION_KEY` quando:

```go
keyMaterial := os.Getenv("FINANCE_ENCRYPTION_KEY")
if keyMaterial == "" && strings.EqualFold(os.Getenv("GO_ENV"), "production") {
    return "", errors.New("FINANCE_ENCRYPTION_KEY obrigatório em produção")
}
h := sha256.Sum256([]byte(keyMaterial + "spuri-finance-default-key"))
```

O problema: **todo o resto do sistema usa a variável `ENV`**, nunca `GO_ENV`:

```go
// cmd/server/main.go
if os.Getenv("ENV") != "production" { ... godotenv.Load() ... }
if os.Getenv("ENV") == "production" { gin.SetMode(gin.ReleaseMode) }

// internal/middleware/auth.go
env := os.Getenv("ENV")
if secret == "" {
    if env == "production" {
        log.Fatalf("[FATAL] JWT_SECRET não configurado em produção...")
```

`GO_ENV` não aparece em nenhum outro arquivo do projeto — só nessas duas funções. Um operador que siga a própria convenção do projeto (definir `ENV=production`, que já é o que ativa `gin.ReleaseMode` e exige `JWT_SECRET`) **nunca** vai pensar em definir também `GO_ENV=production`. Resultado prático: se `FINANCE_ENCRYPTION_KEY` não estiver configurada, o guard de produção **nunca dispara**, e a chave de cifra vira:

```go
sha256.Sum256([]byte("" + "spuri-finance-default-key"))
```

— ou seja, uma chave **fixa, derivada de uma string em texto claro presente no próprio código-fonte**. Qualquer pessoa com acesso ao repositório (ou que reconstrua essa string óbvia) consegue decifrar `client_secret`, `webhook_secret` e `apiKey` de qualquer credencial gravada em `financeiro_segredos_appypay`, bastando ter acesso de leitura ao banco (dump, backup vazado, réplica mal protegida, etc.). Isso não é explorável remotamente via API, mas **anula por completo** a defesa em profundidade que a criptografia deveria oferecer contra exatamente esse cenário — que é justamente o motivo de existir o item de segurança "criptografia de dados sensíveis" nas tarefas 15/16. É um controle de segurança que parece existir, mas cujo gatilho de aplicação está desligado por um erro de digitação em nome de variável.

**Correção sugerida:** trocar `os.Getenv("GO_ENV")` por `os.Getenv("ENV")` nas duas funções (`encrypt`/`decrypt`), e — mais importante — **remover o fallback silencioso por completo**: `keyMaterial == ""` deveria falhar sempre, independentemente de `ENV`, ou pelo menos logar um `WARNING` alto e visível quando cair no valor padrão fora de produção. Também recomendo validar `FINANCE_ENCRYPTION_KEY` no boot (`initDB()`/`main()`), não só no primeiro uso de `encrypt`/`decrypt`, para falhar cedo em vez de silenciosamente.

---

## 🟠 ALTO (novo/regressão) — Histórico auditável é gravado em duplicidade por causa do escritor duplo que não foi eliminado

A correção do item 2 (convergência do `FinanceiroProjection` sem sobrescrever) foi feita corretamente **isoladamente** — `projectCredencial`/`projectCobranca`/`projectModalidade` em `internal/projections/financeiro_projection.go` agora leem o registro existente antes de aplicar o evento e fazem `append` no `Historico`:

```go
c := finance.CredencialAppyPay{}
var existing []byte
if err := p.client.DB().QueryRow(`SELECT payload FROM financeiro_credenciais_appypay WHERE id=$1`, event.AggregateID).Scan(&existing); err == nil {
    _ = json.Unmarshal(existing, &c)
}
...
c.Historico = append(c.Historico, eventoFinanceiro(event, payload, "", c.CodigoAcademia))
```

O problema é que a recomendação original ("eliminar um dos dois escritores") **não foi seguida** — o `Service` em `internal/finance/financeiro.go` continua escrevendo diretamente e de forma síncrona nas mesmas tabelas, com o `Historico` já construído em memória:

```go
// CriarCredencial
ev, err := s.record(ctx, c.ID, "CredenciaisAppyPayCadastradas", credentialPayload(c), autorID, autorTipo, "http")
...
c.Historico = append(c.Historico, ev)   // 1ª entrada, em memória
if err := s.projectCredencial(ctx, c); err != nil { ... }   // grava no Postgres — SÍNCRONO
```

Em produção (`NewServiceWithClient`), o ledger real (`RepositoryLedger`) grava o evento em `spuri_ledger`, e o `projManager` — que roda continuamente via `go projManager.StartProcessing()` (`initProjections()`, `main.go`) — processa esse **mesmo evento** e chama `FinanceiroProjection.projectCredencial(event)`, que faz **outro** `append` sobre o registro que o `Service` já tinha escrito com a entrada correta:

```
Service (síncrono)        →  DB: Historico = [ev]
FinanceiroProjection (assíncrono, mesmo evento) → DB: Historico = [ev, ev']   ← duplicado
```

Isso se repete para **credenciais, cobranças e modalidade** — os três aggregates seguem exatamente o mesmo padrão (`Service.projectCredencial`, `Service.projectCobranca`, `Service.projectModalidade` todos fazem `INSERT ... ON CONFLICT DO UPDATE` com o objeto completo, e os três equivalentes em `FinanceiroProjection` fazem `append` sobre o mesmo evento processado de forma assíncrona).

**Por que não foi pego pelos testes:** todos os testes de replay (`TestReplayReconstróiModalidadeEIdempotencia`, `TestReplayPreservaHistoricoEMotivo`) usam `NewServiceWithDBAndLedger(nil, nil, l)` — **sem banco real** (`db=nil`). Nesse caminho, `Service.project*` faz early-return (`if s.db == nil { return nil }`) e o replay usa `Service.RebuildProjections`/`applyLedgerProjection` (o path em memória do próprio `finance.go`), **nunca** o `FinanceiroProjection` de `internal/projections`. Ou seja: não existe hoje nenhum teste que ligue os dois escritores reais ao mesmo tempo contra Postgres — exatamente o cenário que reproduz o bug.

**Impacto:** o objetivo central da tarefa 16 ("histórico financeiro imutável e **auditável**") fica comprometido — um auditor consultando `Historico` (via `GET /financeiro/appypay/credenciais/:id`, por exemplo, servido a partir do cache em memória do `Service`, que por sinal fica temporariamente "limpo" porque só reflete a escrita síncrona) veria uma coisa, e uma consulta direta a `financeiro_credenciais_appypay.payload` no banco, ou o estado pós-reinício (`loadPersisted` recarrega do banco, já duplicado), veria outra, com entradas repetidas. Isso é exatamente o tipo de inconsistência que um sistema de auditoria financeira não pode ter.

**Correção sugerida:** escolher **um** escritor. O caminho mais alinhado ao resto do backend (onde nenhuma outra entidade tem um "Service" escrevendo direto nas tabelas de projeção — só `academias`, `estudantes`, etc. via projeções puras) é remover `Service.projectCredencial`/`projectCobranca`/`projectModalidade` como escritores de banco e deixar o `Service` apenas ler (`loadPersisted`) e delegar toda escrita ao `FinanceiroProjection`/`projManager`. Isso exige que respostas HTTP síncronas (ex.: `POST /financeiro/appypay/credenciais`) aceitem que a leitura imediata pode vir do cache em memória do próprio processo (`s.creds[id]=c`, que já está correto) em vez de round-trip ao banco.

---

## 🟡 MÉDIO (novo) — Mutex global do `Service` fica preso durante chamada ao provider externo, inclusive bloqueando o kill-switch

`GerarCobrancaFinanceiraBase` segura `s.mu.Lock()` durante **toda** a chamada ao provider, inclusive a rede:

```go
func (s *Service) GerarCobrancaFinanceiraBase(...) (CobrancaFinanceira, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	...
	pid, pstatus, err := s.provider.CriarCobranca(ctx, cred, ch, app)   // chamada externa, dentro do lock
	...
```

Esse `s.mu` é o **mesmo mutex** usado por `ListarCredenciais`, `ObterCredencial`, `AlterarStatusCredencial` e `AlterarModalidade` — inclusive a própria função de ativar/desativar a modalidade de pagamento (o "kill-switch" mencionado na tarefa 15, seção "O payment module inclui um kill-switch controlável só por FPP"). Enquanto `FakeProvider` responde instantaneamente, isso é invisível nos testes; mas assim que um `Provider` real (HTTP contra a AppyPay) for implementado, uma chamada lenta ou travada ao provider **bloqueia globalmente** todas as demais operações financeiras do processo, incluindo exatamente a ação que um FPP precisaria tomar para desativar a modalidade durante uma instabilidade do provider. Isso é uma condição de indisponibilidade auto-infligida (não é uma falha de autorização, mas é um risco operacional/DoS real que vale corrigir antes de plugar o provider de verdade).

**Correção sugerida:** não segurar `s.mu` durante a chamada de rede — copiar o estado necessário, soltar o lock, chamar o provider, e reabrir o lock só para persistir o resultado (padrão já usado, aliás, em `SincronizarStatusCobrancaFinanceiraBase`, que solta o lock antes de chamar `s.provider.ConsultarCobranca`).

---

## 🟡 MÉDIO — `codigo_academia` nunca é populado no contexto Gin (bloqueia acesso legítimo, não é uma falha de segurança em si)

Nenhum middleware do backend (`auth.go`, `academia_middleware.go`, `admin_auth_middleware.go`) faz `c.Set("codigo_academia", ...)`. Isso significa que `cod := c.GetString("codigo_academia")` em `internal/handlers/financeiro_handlers.go` é **sempre string vazia**, mesmo para uma academia autenticada. Como `AtualizarCredencial`/`ListarCredenciais`/`ObterCredencial`/`TestarCredencial` comparam `c.CodigoAcademia == codAcad` (nunca `== ""` na prática), **nenhuma academia consegue de fato gerir a própria credencial** hoje, apesar de a `Documentação.md` descrever esse fluxo de autoatendimento como suportado. Não é uma vulnerabilidade (nega em vez de permitir indevidamente), mas é um efeito colateral que vale registrar já que foi descoberto nesta mesma revisão de segurança — sem isso, a única forma de gerir credenciais na prática é via FPP/ADMIN.

**Correção sugerida:** popular `codigo_academia` no contexto dentro de `AuthMiddleware`/`ValidarStatusAcademia` (consultando `projection_academias` pelo `user_id` quando `user_type == "academia"`), ou então resolver o código a partir do banco diretamente dentro do handler financeiro, como já é feito em outros lugares do sistema (ex.: `financiero_handlers.go` já tem o padrão `user(c)` que poderia ser estendido).

---

## Itens do round anterior que continuam em aberto (sem regressão, apenas não endereçados)

- Reembolso, reversão e reconciliação continuam só com o evento "solicitado"/"executado" inicial, sem função-irmã de conclusão real — consistente com o que a própria correção já não prometia resolver.
- Validação de "estudante pertence à academia" em `GerarCobrancaFinanceiraBase` continua opcional, dependente de `metadata["codigo_academia_estudante"]` ser informado pelo chamador, sem consulta estrutural a `projection_estudantes`.

---

## Checklist atualizado

- [x] Estudante não acessa/lista/testa credenciais financeiras (router + service)
- [x] `motivo` persistido no payload de todos os eventos que o recebem
- [x] Segredos cifrados fora da projeção pública, em tabela dedicada
- [x] `decrypt()` implementado e testado
- [ ] ❌ **Chave de criptografia de produção depende de `GO_ENV`, variável nunca usada em nenhum outro lugar do sistema** — fallback inseguro efetivamente sempre ativo em deployments que seguem a convenção `ENV` do próprio projeto
- [ ] ❌ **Histórico duplicado em produção** — escritor duplo (Service síncrono + FinanceiroProjection assíncrono) não foi eliminado, só um dos dois foi corrigido para não sobrescrever
- [ ] ⚠️ Mutex global do `Service` pode travar o kill-switch de modalidade durante indisponibilidade do provider
- [ ] ⚠️ `codigo_academia` nunca chega ao handler financeiro — autoatendimento de academia efetivamente não funciona

## Recomendação de ordem de correção

1. Trocar `GO_ENV` → `ENV` em `encrypt()`/`decrypt()` e adicionar validação de `FINANCE_ENCRYPTION_KEY` no boot do servidor (achado crítico, baixo esforço, alto risco se ignorado).
2. Remover a escrita direta do `Service` nas tabelas `financeiro_*`, deixando `FinanceiroProjection` como único escritor de projeção (resolve a duplicação de histórico).
3. Soltar `s.mu` antes de chamadas externas ao provider em `GerarCobrancaFinanceiraBase` (mesmo padrão já usado em `SincronizarStatusCobrancaFinanceiraBase`).
4. Popular `codigo_academia` no contexto de autenticação para destravar o autoatendimento de academia.

## Comandos de validação sugeridos

- `go test ./internal/finance/... -v` (deve continuar passando — nenhuma correção sugerida aqui quebra os testes existentes)
- Teste de integração novo recomendado: subir `FinanceiroProjection` real contra Postgres, criar uma credencial via `Service`, aguardar o `projManager` processar o evento, e então fazer `SELECT payload->'Historico' FROM financeiro_credenciais_appypay` — hoje deve mostrar 2 entradas em vez de 1, confirmando o achado 2.
- `FINANCE_ENCRYPTION_KEY= ENV=production go run ./cmd/server` (sem definir `GO_ENV`) e então inspecionar `financeiro_segredos_appypay.ciphertext` — hoje é decifrável com a chave fixa do código-fonte, confirmando o achado crítico.
