---
modificado: 2026-07-31 16:20
criado: 2026-07-31 16:20
origem: Depuração "AppyPay 1" + auditoria de segurança/arquitetura solicitada
---

# Auditoria e Plano de Melhoria — Módulo Financeiro (AppyPay)

## 1. Veredito imediato

**O sistema NÃO está pronto para ser disponibilizado a clientes com um provider de pagamento real.**

Foi confirmada uma falha crítica de autorização (qualquer admin, incluindo o role `gerente`, controla o kill-switch financeiro e as credenciais do contexto `spuri`) e uma limitação crítica de arquitetura (o estado financeiro vive em mapas em memória por processo, o que quebra idempotência, deduplicação de webhook e o próprio kill-switch assim que houver mais de uma instância do backend). Com `FakeProvider` (o único provider existente hoje) o dano prático é limitado porque não há dinheiro real em jogo, mas o desenho atual **não pode** receber um `Provider` HTTP real contra a AppyPay sem antes resolver os itens da secção 4.

A secção 7 lista os critérios de saída objectivos para poder liberar o módulo.

---

## 2. Âmbito desta auditoria

Verificação directa do código actual: `internal/finance/financeiro.go`, `internal/projections/financeiro_projection.go`, `internal/handlers/financeiro_handlers.go`, `internal/middleware/auth.go`, `internal/middleware/admin_auth_middleware.go`, `cmd/server/main.go`, migrations `097`/`098`, e os testes em `internal/finance/financeiro_test.go`.

Esta é a 4ª ronda de auditoria ao mesmo módulo. As três rondas anteriores já corrigiram 8 problemas (ver secção 3). Esta ronda parte do princípio de que esses 8 já estão resolvidos — confirmado por leitura directa do código — e procura problemas **novos**, incluindo problemas que as próprias correcções anteriores podem ter introduzido.

---

## 3. Estado confirmado — correcções anteriores validadas no código actual

| # | Problema (rondas 1–3) | Estado |
|---|---|---|
| 1 | Estudante contornava isolamento em `ListarCredenciais`/`ObterCredencial`/`TestarCredencial` | ✅ Corrigido (`podeAcessarCredencial`, nega por omissão) |
| 2 | `motivo` nunca chegava ao payload do evento | ✅ Corrigido (`payloadWithMotivo`, presente em todas as chamadas relevantes) |
| 3 | Ciphertext embutido na projecção pública `financeiro_credenciais_appypay` | ✅ Corrigido (`financeiro_segredos_appypay`, tags `json:"-"`) |
| 4 | Ausência de `decrypt()` | ✅ Implementado e testado |
| 5 | Chave de cifra validada por `GO_ENV` (nunca usada em mais lado nenhum) | ✅ Corrigido — agora usa `ENV`, com validação no arranque (`ValidateEncryptionConfig`) |
| 6 | Escritor duplo (`Service` + `FinanceiroProjection`) duplicava `Historico` | ✅ Corrigido — `Service` só escreve no cofre de segredos; as tabelas públicas `financeiro_*` só são escritas pela `FinanceiroProjection` assíncrona |
| 7 | Mutex global preso durante a chamada ao provider (bloqueava o kill-switch) | ✅ Corrigido — lock libertado antes de `provider.CriarCobranca`/`ConsultarCobranca` |
| 8 | `codigo_academia` nunca chegava ao contexto Gin (autoatendimento de academia não funcionava) | ✅ Corrigido — `AuthMiddleware` popula `codigo_academia` para `user_type=="academia"` |
| 9 | Corrida de idempotência: duas requisições concorrentes com a mesma `referencia_externa` podiam chamar o provider duas vezes | ✅ Corrigido — `s.idem[key]` é reservado **antes** de libertar o lock, ainda antes da chamada ao provider (`TestGerarCobrancaConcorrenteReservaIdempotenciaAntesDoProvider` confirma) |
| 10 | Academia conseguia reatribuir `contexto_tipo`/`codigo_academia` da própria credencial via `PUT` | ✅ Corrigido — `AtualizarCredencial` força `c.ContextoTipo`/`c.CodigoAcademia` = valores antigos quando `autorTipo=="academia"` (`TestAcademiaNaoReatribuiContextoAoAtualizarCredencial` confirma) |

Nenhuma destas correcções foi revertida ou enfraquecida no código actual.

---

## 4. Vulnerabilidades novas encontradas nesta ronda

Classificadas por **categoria de impacto** (Segurança, Arquitectura, Consistência, Funcionamento) e, dentro de cada categoria, por **criticidade**.

### 4.1 Segurança

#### 🔴 CRÍTICO — Qualquer admin, incluindo `gerente`, controla o kill-switch financeiro e as credenciais `spuri`

O grupo de rotas `/financeiro` em `cmd/server/main.go` usa apenas:

```go
financeiro := protected.Group("/financeiro")
financeiro.Use(middleware.RequireAcademiaOuAdmin())
```

`RequireAcademiaOuAdmin()` (`internal/middleware/auth.go`) só verifica se `user_type` é `"academia"` ou `"admin"` — **não** verifica o `role` do admin (`fpp`/`adm`/`gerente`) nem chama `RequireAdminRole`/`RequireFPP`/`RequireAdm`. Consequência: `c.Get("admin_role")` nunca é definido para estas rotas.

O handler (`internal/handlers/financeiro_handlers.go`) extrai apenas o tipo grosseiro:

```go
func user(c *gin.Context) (string, string, string) {
	id, _ := c.Get("user_id")
	t, _ := c.Get("user_type")   // sempre "admin", nunca "fpp"/"adm"/"gerente"
	cod := c.GetString("codigo_academia")
	return toS(id), toS(t), cod
}
```

E o serviço financeiro só distingue `autorTipo == "admin"` de tudo o resto:

```go
if autorTipo != "fpp" && autorTipo != "admin" {
    return CredencialAppyPay{}, errors.New("apenas FPP/ADMIN podem criar credenciais")
}
```

Como `autorTipo` vindo do handler **nunca é `"fpp"`** (esse valor é o `AdminRole`, não o `UserType`), esta condição é, na prática, "é uma conta admin?" — sem distinguir hierarquia. Isto significa que uma conta `gerente` (o nível mais baixo, documentado como "Consultas e ações básicas administrativas" na tabela de actores do `Documentação.md`) pode hoje:

- Criar credenciais AppyPay do contexto `spuri` (`POST /financeiro/appypay/credenciais`);
- Activar/desactivar qualquer credencial, incluindo `spuri` (`POST .../ativar`, `POST .../desativar`);
- **Alterar o kill-switch de pagamento** global, do contexto Spuri e de qualquer academia (`POST /financeiro/modalidade-pagamento`).

Isto contradiz directamente tanto o texto da tarefa 15 ("Apenas FPP/ADMIN podem criar, atualizar, ativar, desativar ou testar credenciais do contexto spuri") como o requisito de produto já registado para este módulo, de que o kill-switch deve ser controlável apenas por administradores com role FPP.

**Correcção recomendada:**
1. No router, aplicar `middleware.RequireFPP()` às rotas que só devem ser FPP: criação de credencial, activação/desactivação e alteração de modalidade. Manter `RequireAcademiaOuAdmin()` apenas nas rotas de leitura e na actualização de credencial (para permitir o autoatendimento de academia).
2. Propagar o `admin_role` real (não apenas `user_type`) até ao `Service`, e fazer o `Service` validar explicitamente `"fpp"` em vez de aceitar qualquer `"admin"`. Isto fecha a lacuna mesmo que uma futura rota volte a esquecer o middleware certo — a validação fica onde a lógica de negócio vive, não só no router.

---

#### 🟠 ALTO — Um dos dois escritores do ledger (`SQLLedger`) ignora completamente a whitelist de eventos/aggregates

`internal/finance/financeiro.go` define **duas** implementações de `LedgerWriter`:

- `RepositoryLedger` — usada em produção (`NewServiceWithClient`), passa pelo `AggregateRepository`/`aggregates.Financeiro`, que por sua vez passa por `EventStore.AppendTx` → `ValidateAggregateType`/`ValidateEventType` (a whitelist de `safe_queries.go`).
- `SQLLedger` — grava directamente com SQL cru:

```go
_, err = tx.ExecContext(ctx, `INSERT INTO spuri_ledger (
    event_id, aggregate_id, aggregate_type, event_type,
    event_version, payload, metadata, occurred_at
) VALUES ($1,$2,'Financeiro',$3,$4,$5,$6,$7)`, ...)
```

Este `INSERT` **não passa por `ValidateEventType`/`ValidateAggregateType`**. É exactamente o mecanismo que `Documentação.md` descreve como garantia central do sistema: *"Apenas eventos previamente autorizados podem ser gravados no ledger (safe_queries.go). Qualquer evento desconhecido é rejeitado antes de chegar ao banco."* — `SQLLedger` quebra essa garantia por construção.

Hoje, `SQLLedger` **não é seleccionado** pelo caminho real de produção (`main.go` usa `NewServiceWithClient` → `RepositoryLedger`), por isso não é explorável neste momento. Mas é seleccionado automaticamente por `NewServiceWithDB(db, provider)` sempre que alguém passar uma `*sqlx.DB` real sem indicar explicitamente um `ledger` — uma assinatura de função perfeitamente razoável de se chamar num futuro refactor ou teste de integração. É uma mina: quem a activar por engano perde, sem aviso, a validação central de segurança do event sourcing.

**Correcção recomendada:** eliminar `SQLLedger` e fazer `RepositoryLedger` ser o único `LedgerWriter` de produção; ou, no mínimo, fazer `SQLLedger.AppendFinanceEvent` chamar `db.ValidateEventType`/`db.ValidateAggregateType` antes do `INSERT`, tal como `EventStore` já faz.

---

#### 🟠 ALTO — `ProcessarWebhookFinanceiroBase` não valida autenticidade do webhook

A função base de processamento de webhook não recebe, nem verifica, nenhuma assinatura ou segredo:

```go
func (s *Service) ProcessarWebhookFinanceiroBase(ctx context.Context, eventID string, chargeID uuid.UUID, autorID string) (bool, error) {
```

A tarefa 15 (secção 5.7, requisito 1) exige explicitamente *"validar origem/autenticidade com os mecanismos disponíveis e com segredo local quando configurado"*. Hoje não existe nenhuma verificação de HMAC/assinatura contra `WebhookSecretEncrypted` em lado nenhum do módulo. A mitigação existente (nunca liquidar sem confirmar via `SincronizarStatusCobrancaFinanceiraBase`, que consulta o provider) reduz o dano de um webhook forjado, mas não impede o efeito colateral de forçar chamadas indevidas ao provider real, nem cumpre o requisito documentado.

**Correcção recomendada:** a função base deve receber o payload bruto e a assinatura recebida, calcular o HMAC esperado a partir do `webhook_secret` decifrado da credencial correspondente, e rejeitar antes de gravar `WebhookFinanceiroRecebido` se a assinatura não bater.

---

#### 🟡 MÉDIO — `sanitizeMap` não é recursivo; segredos em `Metadata` de cobrança podem chegar ao ledger imutável

```go
func sanitizeMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		if ContainsSensitive(k) {
			out[k+"_redacted"] = "***"
			continue
		}
		out[k] = v   // valores aninhados NÃO são inspeccionados
	}
	return out
}
```

`GerarCobrancaInput.Metadata` é um `map[string]string` totalmente controlado pelo chamador (hoje o próprio handler HTTP, no futuro qualquer domínio de negócio como propina/mensalidade). O seu conteúdo entra em `chargePayload(c)["metadata"]` sem qualquer inspecção — só a chave de topo `"metadata"` é verificada contra `ContainsSensitive`, nunca o interior do mapa. Se um chamador futuro colocar por engano algo sensível dentro de `Metadata` (ex.: um token de sessão, um NIF completo sem necessidade), isso fica gravado para sempre no ledger append-only, sem possibilidade de correcção.

**Correcção recomendada:** tornar `sanitizeMap` recursivo (percorrer `map[string]any` e `map[string]string` aninhados), ou, mais seguro ainda, definir uma lista explícita de chaves permitidas em `Metadata` de cobrança em vez de aceitar qualquer coisa.

---

#### 🟡 MÉDIO — Funções "base" transaccionais não verificam autorização nem posse do recurso

`GerarCobrancaFinanceiraBase`, `CancelarCobrancaFinanceiraBase`, `ReembolsarCobrancaFinanceiraBase` e `ReverterCobrancaFinanceiraBase` recebem apenas um `autor`/`autorID` (string, validada só quanto a não-vazia) — nenhuma recebe `autorTipo`, nenhuma verifica que o autor tem direito sobre o `contexto_tipo`/`codigo_academia` da cobrança em causa. Ao contrário de `CriarCredencial`/`AlterarStatusCredencial`/`AlterarModalidade` (que **têm** RBAC interno), estas quatro funções dependem inteiramente de quem as chamar no futuro se lembrar de validar isto antes. Dado que a tarefa 15 pede explicitamente que estas funções sejam *"reutilizáveis"* por handlers de domínio futuros (propina, mensalidade, etc.), a ausência de uma verificação mínima aqui é um convite a que o próximo handler introduza um bypass de isolamento por instituição sem perceber.

**Correcção recomendada:** acrescentar `autorTipo string` e, quando aplicável, `codigoAcademiaAutor string` a estas quatro assinaturas, e validar contra `c.ContextoTipo`/`c.CodigoAcademia` da cobrança, seguindo exactamente o padrão já usado em `podeAcessarCredencial`.

---

#### 🟢 BAIXO (residual) — Chave de cifra sem rotação real apesar do esquema a suportar

`financeiro_segredos_appypay` tem `key_id VARCHAR(128) NOT NULL DEFAULT 'FINANCE_ENCRYPTION_KEY'`, mas o código nunca lê nem grava um `key_id` diferente — há uma única chave derivada de `FINANCE_ENCRYPTION_KEY + "spuri-finance-default-key"` para sempre. Rodar a chave hoje implicaria não conseguir decifrar segredos antigos. Isto não é explorável remotamente, mas é uma lacuna directa face ao que a tarefa 16 pede ("registar key_id para suportar rotação").

---

### 4.2 Arquitectura

#### 🔴 CRÍTICO — Estado financeiro vive em mapas por processo; o módulo não sobrevive a mais de uma instância do backend

`Service` guarda `creds`, `charges`, `idem`, `webhooks` e `modalidade` como mapas Go protegidos por `sync.Mutex` — estruturas **exclusivas de um processo**. Com mais de uma instância do backend a correr (cenário real assim que houver necessidade de escalar horizontalmente, ou mesmo só para alta disponibilidade em deploy com rolling restart):

- **Idempotência quebra entre instâncias.** A correcção da secção 3 (#9) só serializa dentro do mesmo processo. Duas requisições com a mesma `referencia_externa`, balanceadas para duas instâncias diferentes, passam ambas na verificação `s.idem[key]` (cada uma vê o seu próprio mapa vazio) e **ambas** chamam o provider — o cenário exacto que a correcção #9 resolveu, mas só localmente.
- **Deduplicação de webhook quebra entre instâncias**, pelo mesmo motivo — `s.webhooks[eventID]` é local.
- **O kill-switch (`AlterarModalidade`) não se propaga.** Um FPP desactiva a modalidade numa instância; as outras instâncias continuam a aceitar cobranças com `s.modalidade` desactualizada em memória até reiniciarem.
- **Desactivar uma credencial não se propaga.** Uma credencial marcada `inativo` numa instância continua `ativo` na memória de outra até reiniciar — uma academia poderia continuar a cobrar através da instância "desactualizada".

Isto vai directamente contra o requisito explícito do utilizador ("escalável horizontalmente") e contra a própria natureza de um kill-switch, que existe precisamente para cortar acesso **imediatamente** em todas as instâncias.

**Correcção recomendada:**
1. Para **idempotência de cobrança**: reutilizar o padrão já existente no resto do sistema (`internal/db/unique_operation_guard.go`, `unique_operation_guards`) — reservar a chave numa tabela com `UNIQUE` antes de chamar o provider, em vez de num mapa em memória.
2. Para **deduplicação de webhook**: já existe `financeiro_webhooks_recebidos` com `event_id PRIMARY KEY` — basta que a verificação de duplicado seja sempre feita por `INSERT ... ON CONFLICT DO NOTHING` contra a base de dados (devolvendo se inseriu ou não), nunca só contra o mapa local.
3. Para **modalidade e estado de credenciais** (dados lidos com muita frequência e que precisam de ser imediatamente consistentes por serem um kill-switch): ou deixar de usar cache em memória para estas duas leituras específicas (ir sempre à base de dados, que já é rápida e tem pool dedicado), ou implementar invalidação activa entre instâncias (ex.: `LISTEN`/`NOTIFY` do PostgreSQL, já que o projecto está em Postgres/Aiven/Neon).

---

#### 🟠 ALTO — Janela de corrida entre reserva em memória e projecção assíncrona permite duplicar cobrança num reinício do processo

Consequência directa da correcção #6 (escritor único) combinada com a correcção #9 (reserva antes do unlock): `s.idem[key]` é reservado **em memória**, de imediato, mas a tabela durável `financeiro_cobrancas` (com `idempotency_key UNIQUE`) só é escrita mais tarde, de forma assíncrona, quando `FinanceiroProjection` processar o evento `CobrancaFinanceiraCriada` a partir do ledger. Se o processo cair depois do evento estar gravado no ledger (durável) mas antes da projecção o processar, o reinício recarrega `s.idem` a partir de `financeiro_cobrancas` (`loadPersisted`) — que ainda não tem a entrada, porque a projecção ainda não apanhou o evento. Nessa janela, uma nova tentativa com a mesma `referencia_externa` passa a verificação de idempotência outra vez.

**Correcção recomendada:** a mesma da secção anterior (reserva via tabela `UNIQUE`, não em memória) resolve este problema ao mesmo tempo que resolve o problema de escalabilidade horizontal — são a mesma causa raiz.

---

#### 🟡 MÉDIO — Dois mecanismos de rebuild independentes, um deles permanentemente quebrado

`Service.RebuildProjections(ctx)` chama `s.ledger.LoadFinanceEvents(ctx)`. Quando o `ledger` em uso é `RepositoryLedger` (o caminho de produção via `NewServiceWithClient`), essa chamada **devolve sempre erro**:

```go
func (l RepositoryLedger) LoadFinanceEvents(ctx context.Context) ([]LedgerEvent, error) {
	return nil, errors.New("RepositoryLedger não expõe replay; use FinanceiroProjection para rebuild canônico")
}
```

Ou seja, `Service.RebuildProjections()` está permanentemente quebrado em produção — felizmente, não é chamado por nenhuma rota HTTP hoje (o rebuild real acontece via `POST /dominis/projections/rebuild/financeiro`, que usa `FinanceiroProjection.Rebuild()`, um caminho completamente separado que funciona bem). Ter dois mecanismos de "reconstruir o mesmo estado", um funcional e outro que falha sempre com uma mensagem a apontar para o outro, é confuso para quem for dar manutenção — e é exactamente o tipo de código que engana tanto humanos como IA numa refactorização (parece uma função pública válida, mas está morta).

**Correcção recomendada:** remover `Service.RebuildProjections`/`applyLedgerProjection`/`SQLLedger.LoadFinanceEvents` do caminho de produção, ou documentar claramente com um comentário `// DEPRECATED` à cabeça de cada uma, apontando para `FinanceiroProjection.Rebuild` como único caminho suportado. Idealmente, remover mesmo — menos código, menos ambiguidade.

---

#### 🟡 MÉDIO — Duas fontes de verdade para o mesmo estado, sem indicador de atraso entre elas

Leituras HTTP (`GET /financeiro/appypay/credenciais`, etc.) respondem a partir do cache em memória do `Service` (`s.creds`), que reflecte imediatamente as escritas feitas *nessa instância*. As tabelas `financeiro_*` reflectem o que a `FinanceiroProjection` já processou, de forma assíncrona. Nada no código expõe até que ponto estas duas vistas podem estar desalinhadas (não há `updated_at`/checkpoint comparável exposto ao chamador). Combinado com o ponto anterior sobre múltiplas instâncias, isto é uma fonte de bugs difíceis de reproduzir ("porque é que a academia B não vê a credencial que a academia A acabou de criar, se ambas apontam ao mesmo backend, mas em instâncias diferentes?").

---

### 4.3 Consistência

#### 🟡 MÉDIO — Cobrança pode ficar presa em `pendente` para sempre se a segunda escrita no ledger falhar depois do provider já ter respondido

Em `GerarCobrancaFinanceiraBase`, depois de chamar o provider:

```go
ev, recErr := s.record(ctx, ch.ID, "CobrancaFinanceiraEnviadaAoProvider", chargePayload(ch), autorID, "sistema", "provider")
if recErr != nil {
    return CobrancaFinanceira{}, recErr   // <- ch.Status (já actualizado localmente) NUNCA chega a s.charges
}
ev.Metadata = map[string]any{"provider_status": pstatus}
ch.Historico = append(ch.Historico, ev)
s.charges[ch.ID] = ch      // só aqui a actualização é persistida no mapa
s.idem[key] = ch.ID
return ch, providerErr
```

Se `s.record(...)` para o segundo evento falhar (ex.: falha transitória de ligação à base de dados mesmo depois do provider já ter aceite a cobrança), a função devolve erro **sem nunca gravar** o novo `ch.Status`/`ch.ProviderChargeID` em `s.charges[ch.ID]`. A cobrança fica presa em `CobrancaPendente` no estado partilhado, mesmo que o provider já tenha processado (ou recusado) o pedido de verdade. Como `s.idem[key]` já tinha sido reservado **antes** desta chamada, qualquer nova tentativa com a mesma `referencia_externa` vai devolver directamente esta cobrança "presa", sem nunca voltar a tentar sincronizar com o provider.

**Correcção recomendada:** persistir `s.charges[ch.ID] = ch` com o novo `Status` **antes** de tentar gravar o segundo evento no ledger (ou seja, actualizar o estado local primeiro, e tratar a falha do `s.record` como algo a reconciliar depois, não como motivo para nunca reflectir o resultado real do provider). Alternativa mais robusta: registar aqui uma falha explícita e accionável (ex.: marcar `CobrancaEnviada` com uma flag `sincronizacao_pendente=true`) em vez de devolver erro silencioso.

---

#### 🟡 MÉDIO — `internal/handlers/financeiro_handlers.go` não usa o envelope de erro padrão do sistema

Todo o resto da API usa `utils.RespondWithError`/`RespondWithErrorData`, produzindo sempre `{error, message, request_id, details?}` (contrato documentado em `Documentação.md`, secção "Envelope de Erro"). O módulo financeiro devolve erros crus:

```go
c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
```

Sem `message`, sem `request_id`, sem o tipo padronizado de erro (`VALIDATION_ERROR`/`FORBIDDEN`/etc.). Isto quebra o contrato documentado da API para exactamente este módulo, obriga o cliente/frontend a tratar as respostas financeiras de forma diferente de todas as outras, e — porque `err.Error()` é devolvido directamente — corre o risco de, no futuro, expor mensagens internas não pensadas para serem lidas por um cliente externo.

**Correcção recomendada:** substituir todas as respostas de erro em `financeiro_handlers.go` por `utils.RespondWithError(c, status, mensagem_segura, err)`.

---

#### 🟢 BAIXO — Crescimento sem limite dos mapas `idem`/`webhooks`

Nenhuma das duas estruturas em memória (nem as tabelas `financeiro_cobrancas`/`financeiro_webhooks_recebidos` que as espelham) tem política de expiração/arquivo. Não é urgente, mas junta-se à recomendação de mover a idempotência para a base de dados (secção 4.2) — nessa altura, convém já pensar num índice/partição por data para não crescer para sempre sem plano.

---

### 4.4 Funcionamento

#### 🟡 MÉDIO — Reembolso, reversão e reconciliação só têm a "primeira metade" do fluxo

`ReembolsarCobrancaFinanceiraBase`/`ReverterCobrancaFinanceiraBase` só emitem o evento "solicitado" (`ReembolsoFinanceiroSolicitado`/`ReversaoFinanceiraSolicitada`); os eventos de conclusão (`ReembolsoFinanceiroStatusAtualizado`/`ReversaoFinanceiraStatusAtualizado`) estão na whitelist mas nunca são emitidos por nenhuma função. `ReconciliarFinanceiroBase` grava um único evento fixo (`ReconciliacaoFinanceiraExecutada`) sem nenhuma lógica real de comparação contra o provider. Isto é esperado dado que ainda não existe um `Provider` real, mas é importante deixar explícito: **estes três fluxos estão apenas parcialmente implementados** e não devem ser tratados como prontos a usar por um handler de domínio futuro sem revisão.

---

#### 🟡 MÉDIO — Isolamento estudante↔academia continua opcional, não estrutural

Ainda sem alteração desde a ronda anterior: em `GerarCobrancaFinanceiraBase`, a validação de que um estudante pertence à academia que está a cobrar só corre se `in.Metadata["codigo_academia_estudante"]` for explicitamente preenchido pelo chamador — não há consulta real a `projection_estudantes`. Um futuro handler de domínio que esqueça de preencher este campo do metadata perde silenciosamente esta protecção.

---

## 5. Recomendações de melhoria estrutural

### 5.1 Segurança
- Aplicar `RequireFPP()` a todas as operações de escrita sensíveis do módulo financeiro (criação/activação/desactivação de credencial, alteração de modalidade), e propagar o `admin_role` real (não só `user_type`) até ao `Service`.
- Fazer `sanitizeMap` recursivo, ou trocar `Metadata` de cobrança por uma allow-list explícita de chaves.
- Assinar/validar webhooks (HMAC contra `webhook_secret`) antes de qualquer processamento, mesmo na função base.
- Dar às quatro funções "base" sem RBAC (`GerarCobrancaFinanceiraBase`, `CancelarCobrancaFinanceiraBase`, `ReembolsarCobrancaFinanceiraBase`, `ReverterCobrancaFinanceiraBase`) uma verificação mínima de posse (`autorTipo`/`codigo_academia` vs. `contexto_tipo`/`codigo_academia` da cobrança), para que sejam seguras por omissão para quem as reutilizar.
- Eliminar `SQLLedger` (ou fazê-lo passar pela whitelist), para não deixar um caminho de escrita ao ledger que ignora a validação central do sistema.
- Suportar `key_id` de verdade em `financeiro_segredos_appypay`, com rotação testável.

### 5.2 Auditabilidade
- Um único escritor de estado por aggregate (já corrigido para as tabelas públicas — falta aplicar o mesmo princípio ao rebuild, eliminando o caminho morto de `Service.RebuildProjections`).
- Garantir que todo o campo que aparece em `Historico`/`EventoFinanceiro` (autor, motivo, contexto) é reconstruído de forma idêntica a partir do replay do ledger e a partir da escrita síncrona original — hoje isso já é verdade para os campos principais; validar com um teste de integração que compara os dois caminhos byte a byte.
- Acrescentar um teste que suba `FinanceiroProjection` real contra Postgres (não só o caminho em memória `NewServiceWithDBAndLedger(nil, nil, l)`) — é o único jeito de apanhar problemas como os desta e da ronda anterior antes de produção.

### 5.3 Escalabilidade horizontal
- Mover idempotência de cobrança e deduplicação de webhook de mapas em memória para tabelas com `UNIQUE` (reaproveitar `unique_operation_guards`, já existente no projecto).
- Não confiar em cache em memória para decisões de autorização de alto impacto (modalidade activa, status de credencial) — ler sempre da base de dados nesses dois pontos específicos, ou implementar invalidação activa entre instâncias.
- Documentar explicitamente (no próprio `Documentação.md`) que, enquanto o estado financeiro depender de cache em memória por processo, o backend financeiro **deve correr numa única instância** — para que ninguém escale horizontalmente por engano antes das correcções acima.

### 5.4 Legibilidade e manutenção por IA
- `internal/finance/financeiro.go` tem mais de 700 linhas com múltiplas responsabilidades (CRUD de credenciais, cobranças, modalidade, webhooks, reconciliação, cifra, e duas implementações de `LedgerWriter`). Recomenda-se dividir em ficheiros por responsabilidade, seguindo o padrão já usado no resto do repositório (`internal/domain/aggregates/academia_categorias_nota.go`, `estudante_avaliacao.go`, etc. — um ficheiro por sub-domínio dentro do mesmo pacote): por exemplo `credenciais.go`, `cobrancas.go`, `modalidade.go`, `webhooks.go`, `crypto.go`, `ledger.go`.
- Structs como `CredencialAppyPay`/`CobrancaFinanceira` têm vários campos na mesma linha separados por vírgulas (`AuthBaseURL, APIBaseURL, WebAPIBaseURL, ClientID string`). Isto compila bem mas dificulta diffs, comentários por campo, e leitura rápida — tanto para humanos como para uma IA a tentar perceber o que mudou num PR. Recomenda-se um campo por linha, como o resto do código-base já faz (ver `internal/domain/aggregates/academia.go`, `estudante.go`).
- Faltam comentários `// FIX X-NN: ...` explicando *porquê* de decisões não óbvias — o resto do repositório usa este padrão de forma consistente e é o que torna mais fácil para uma IA perceber a intenção antes de alterar algo (ex.: por que é que `s.idem` é reservado antes do unlock; por que é que `sanitizeMap` existe; por que é que há dois `LedgerWriter`).
- Remover código morto assim que confirmado morto (`SQLLedger`, `Service.RebuildProjections` quando usado com `RepositoryLedger`) em vez de o deixar "só por precaução" — código morto num módulo de segurança é um risco, não uma rede de segurança.

---

## 6. Checklist de bloqueio para produção com provider real

Estes itens têm de estar resolvidos antes de ligar um `Provider` HTTP real contra a AppyPay com dinheiro/dados reais:

- [ ] RBAC do módulo financeiro restringido a FPP nas operações sensíveis (secção 4.1, crítico)
- [ ] Idempotência de cobrança e deduplicação de webhook movidas para a base de dados (secção 4.2, crítico)
- [ ] `SQLLedger` removido ou corrigido para respeitar a whitelist (secção 4.1, alto)
- [ ] Validação de assinatura de webhook implementada (secção 4.1, alto)
- [ ] Janela de corrida crash-restart eliminada (mesma correcção da idempotência em BD)
- [ ] Bug de cobrança presa em `pendente` corrigido (secção 4.3)
- [ ] Envelope de erro padrão aplicado ao módulo financeiro (secção 4.3)

Itens recomendados mas **não bloqueantes** para uma primeira operação controlada (podem ser feitos em paralelo/depois): dividir o ficheiro por responsabilidade, `sanitizeMap` recursivo, RBAC mínimo nas funções base, rotação de chave, conclusão dos fluxos de reembolso/reversão/reconciliação, validação estrutural de isolamento estudante-academia.

---

## 7. Veredito final

**Estado actual:** seguro para continuar desenvolvimento interno com `FakeProvider`, numa única instância. **Não seguro** para expor a clientes com dinheiro real, nem para escalar horizontalmente, no estado actual.

**Aviso de prontidão:** o módulo estará pronto para clientes quando todos os itens da checklist da secção 6 estiverem resolvidos e cobertos por teste automatizado equivalente ao já existente para as correcções das rondas anteriores (nomeadamente: um teste com `FinanceiroProjection` real contra Postgres, e um teste de RBAC que confirme que um admin `gerente` é rejeitado nas operações sensíveis). Nessa altura, recomendo uma quinta ronda de verificação, focada exclusivamente em confirmar esses testes — não é preciso repetir a auditoria inteira se nada mais tiver mudado entretanto.
