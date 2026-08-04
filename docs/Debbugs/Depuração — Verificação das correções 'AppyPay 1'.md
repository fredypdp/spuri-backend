---
modificado: 2026-07-31 15:03
criado: 2026-07-31 15:03
---
> **Nota de arquivo (rollback):** o módulo financeiro/AppyPay descrito e auditado
> neste relatório foi removido por completo do código em `2026-08-03`
> (ver `docs/Lista de Tarefas/17 - Remover completamente o modulo financeiro
> AppyPay (rollback total).md`). Este documento é mantido apenas como
> referência histórica das vulnerabilidades encontradas, para orientar uma
> futura reimplementação mais robusta a partir da tarefa 15.

# Depuração — Verificação das correções "AppyPay 1"

## Objetivo

Verificar, direto no código (`internal/finance/financeiro.go`, `internal/projections/financeiro_projection.go`, `internal/handlers/financeiro_handlers.go`, `internal/middleware/auth.go`, `cmd/server/main.go`), se as 4 correções descritas em `docs/Debbugs/Correções executadas — AppyPay 1.md` foram implementadas como descrito, e procurar ativamente por vulnerabilidades remanescentes ou **introduzidas pelas próprias correções** — o padrão que os dois rounds anteriores já mostraram ser produtivo neste módulo.

## Resultado geral

As **4 correções foram confirmadas como implementadas** no código. Porém, duas delas — item 3 (liberação do mutex) e item 4 (`codigo_academia` no contexto) — **abriram, cada uma, uma vulnerabilidade nova e concreta** que só existe _por causa_ da correção ter sido feita corretamente em isolamento, sem tratar um efeito colateral direto. Isso é consistente com o padrão dos rounds anteriores: a suíte de testes atual nunca simula concorrência real nem verifica que campos de "propriedade" (`contexto_tipo`, `codigo_academia`) não podem ser reescritos por um ator de tenant.

---

## ✅ Correções confirmadas como implementadas

### 1. Chave de criptografia validada pela variável correta

```go
func financeEncryptionKeyMaterial() (string, error) {
	keyMaterial := os.Getenv("FINANCE_ENCRYPTION_KEY")
	if keyMaterial == "" && strings.EqualFold(os.Getenv("ENV"), "production") {
		return "", errors.New("FINANCE_ENCRYPTION_KEY obrigatório em produção")
	}
	return keyMaterial, nil
}
```

`encrypt()`/`decrypt()` agora centralizam nessa função, e `buildCredential()` propaga o erro em vez de ignorá-lo. `initDB()` chama `finance.ValidateEncryptionConfig()` antes de conectar/migrar. Nenhuma referência a `GO_ENV` sobrou no código. **Confirmado.**

_Residual (baixa prioridade, não é regressão):_ o fallback para a chave derivada da string fixa `"spuri-finance-default-key"` continua ativo sempre que `ENV` não for exatamente `"production"` — inclusive em um eventual ambiente de staging real (ex.: a candidata Neon mencionada no histórico do projeto) que processe dados sensíveis. Vale considerar exigir a chave sempre que `FINANCE_ENCRYPTION_KEY` estiver ausente, independente do valor de `ENV`.

### 2. Escritor duplo removido do fluxo síncrono do `Service`

Confirmado por leitura completa: `CriarCredencial`, `AtualizarCredencial`, `TestarCredencial`, `AlterarStatusCredencial` agora chamam apenas `s.projectCredencialSegredos(ctx, c)` (grava só o cofre `financeiro_segredos_appypay`), **não** mais `s.projectCredencial` (que escrevia a projeção pública inteira). O mesmo vale para `GerarCobrancaFinanceiraBase`/`SincronizarStatusCobrancaFinanceiraBase`/`CancelarCobrancaFinanceiraBase` (nenhuma chama `s.projectCobranca`) e `AlterarModalidade` (não chama `s.projectModalidade`). A única gravação nas tabelas públicas `financeiro_credenciais_appypay`/`financeiro_cobrancas`/`financeiro_modalidade_pagamento` agora vem do `FinanceiroProjection` assíncrono (`internal/projections/financeiro_projection.go`), que já fazia o padrão _load→merge→append_ corrigido no round anterior. **Confirmado — resolve a duplicação de `Historico` relatada antes.**

_Nota de arquitetura (não é vulnerabilidade):_ como consequência, em uma implantação com múltiplas instâncias do backend, `s.creds`/`s.charges`/`s.modalidade` em memória de uma instância só refletem escritas feitas _naquela mesma instância_ — outra instância só vê a mudança após reiniciar (`loadPersisted`). Aceitável para deployment single-instance (coerente com o pool de conexões restrito do Aiven), mas vale documentar como limitação conhecida caso o backend seja escalado horizontalmente.

### 3. Mutex liberado durante a chamada externa ao provider

Confirmado: `GerarCobrancaFinanceiraBase` agora libera `s.mu` antes de `s.provider.CriarCobranca(...)` e reabre o lock só para persistir o resultado. Sem deadlock, sem double-unlock. **Confirmado — mas ver achado crítico abaixo, que é uma consequência direta dessa mudança.**

### 4. `codigo_academia` populado no contexto Gin

Confirmado em `internal/middleware/auth.go`:

```go
if userType == "academia" {
    query = `SELECT status, codigo_academia FROM projection_academias WHERE id::text = $1`
    err := client.DB().QueryRow(query, userID.String()).Scan(&status, &codigoAcademia)
    if codigoAcademia.Valid {
        c.Set("codigo_academia", codigoAcademia.String)
    }
    ...
```

`user(c)` em `financeiro_handlers.go` agora recebe um `codigo_academia` real, destravando o autoatendimento de academia descrito na `Documentação.md`. **Confirmado — mas ver achado alto abaixo, que só se tornou explorável porque essa correção fez o caminho de "academia edita a própria credencial" passar a funcionar de verdade.**

---

## 🔴 CRÍTICO (regressão introduzida pela correção #3) — Janela de corrida quebra a idempotência de cobrança

A liberação do mutex antes da chamada ao provider foi feita corretamente do ponto de vista de deadlock/travamento, mas **a chave de idempotência só é gravada em `s.idem`/`s.charges` no fim da função**, depois que o provedor já respondeu:

```go
s.mu.Lock()
...
key := string(in.ContextoTipo) + ":" + in.CodigoAcademia + ":" + in.ReferenciaExterna
if id, ok := s.idem[key]; ok {          // ← única verificação de idempotência
    ch := s.charges[id]
    s.mu.Unlock()
    return ch, nil
}
...
ev, err := s.record(ctx, ch.ID, "CobrancaFinanceiraCriada", chargePayload(ch), autorID, "sistema", "http")
...
ch.Historico = append(ch.Historico, ev)
s.mu.Unlock()                            // ← lock solto aqui

pid, pstatus, providerErr := s.provider.CriarCobranca(ctx, cred, ch, app)  // chamada externa, sem lock

s.mu.Lock()
defer s.mu.Unlock()
...
s.charges[ch.ID] = ch
s.idem[key] = ch.ID                      // ← só reservado AQUI, no final
return ch, providerErr
```

Entre o `s.mu.Unlock()` e a chamada ao provedor, **nada reserva a chave `key`**. Se uma segunda requisição com a mesma `referencia_externa` chegar nessa janela — cenário extremamente comum em pagamentos: cliente que reenvia após timeout, duplo clique em "pagar", retry automático do frontend — ela também vai encontrar `s.idem[key]` vazio, passar pela checagem de idempotência, gerar um **segundo evento `CobrancaFinanceiraCriada`** e **chamar o provedor de pagamento uma segunda vez para a mesma cobrança lógica**. Isso é exatamente o cenário que o critério de aceite #9 da tarefa 15 e o item 6 da seção 7 ("controlo de concorrência para evitar cobranças duplicadas") exigem que seja impossível — e antes da correção #3, o mutex global (mesmo sendo ruim para disponibilidade) impedia acidentalmente essa corrida ao serializar toda a função.

O próprio backend já tem o mecanismo certo para isso, usado em outros fluxos de criação idempotente (`internal/db/unique_operation_guard.go`, tabela `unique_operation_guards` da migration 096, e o padrão de `codigo_academia_reservas`/`codigo_estudante_reservas`), mas o módulo financeiro não o reaproveita.

**Correção sugerida:** reservar a chave de idempotência **antes** de soltar o lock, usando `UniqueOperationGuard.Reserve("cobranca_financeira", key, ...)` — devolvendo `ErrUniqueOperationInProgress` (ou aguardando/retornando a cobrança já existente) para o segundo request, e chamando `.Consume()`/`.Release()` conforme o resultado da chamada ao provedor. Alternativa mais simples: gravar um placeholder em `s.idem[key]` (com um status "reservando") **antes** de soltar o lock, e resolvê-lo depois.

---

## 🟠 ALTO (exposto pela correção #4) — Academia reescreve `contexto_tipo`/`codigo_academia` da própria credencial

Com `codigo_academia` agora populado no contexto, `AtualizarCredencial` passou a funcionar de verdade para academias — e revelou que a checagem de permissão valida a credencial **antiga**, mas o objeto **novo** gravado vem inteiramente do corpo da requisição, sem qualquer trava sobre `ContextoTipo`/`CodigoAcademia`:

```go
func (s *Service) AtualizarCredencial(ctx context.Context, id uuid.UUID, in CredencialInput, autorID, autorTipo, codAcad string) (CredencialAppyPay, error) {
	...
	old, ok := s.creds[id]
	...
	if autorTipo != "fpp" && autorTipo != "admin" &&
	   !(autorTipo == "academia" && old.ContextoTipo == ContextoAcademia && old.CodigoAcademia == codAcad) {
		return old, errors.New("sem permissão")     // ← valida contra OLD
	}
	c, err := buildCredential(in)                    // ← constrói a partir do BODY, incluindo in.ContextoTipo/in.CodigoAcademia
	if err != nil {
		return c, err
	}
	c.ID = id
	c.CreatedAt = old.CreatedAt
	c.Version = old.Version + 1
	...
	s.creds[id] = c                                   // ← grava com o NOVO contexto/academia, nunca revalidado
	return maskCred(c), nil
}
```

Ou seja: uma academia `ACA001` autenticada, autorizada apenas porque a credencial `id` **já era dela** (`old.CodigoAcademia == "ACA001"`), pode enviar no corpo de `PUT /financeiro/appypay/credenciais/:id` um payload com `"contexto_tipo": "spuri"` ou `"codigo_academia": "ACA002"`. A checagem de permissão passa (compara contra o registro antigo), mas o registro é reescrito com o novo dono/contexto — quebrando o isolamento por instituição exigido explicitamente pela tarefa ("uma cobrança de estudante deve validar que o estudante pertence à academia do contexto autenticado" / "nenhuma credencial de uma academia pode ser usada para cobrar estudante de outra academia").

A gravação sempre volta `Status: StatusPendenteValidacao` (dentro de `buildCredential`), então não há ativação automática — a exploração financeira direta exige um FPP/ADMIN ativar depois. Mas isso é justamente o vetor perigoso: um FPP revisando credenciais "pendentes" e vendo `codigo_academia="ACA002"` pode confiar na aparência do dado e ativá-la, ativando na prática uma credencial cujo `client_secret`/`apiKey` foram fornecidos por `ACA001` — permitindo a `ACA001` interceptar/receber cobranças destinadas a `ACA002`. Mesmo sem chegar a esse ponto, é uma violação de integridade de dados por tenant no nível de escrita, não só de leitura (que era o único ponto testado por `TestIsolamentoAcademiasEIdempotencia`).

É auditável — o evento `CredenciaisAppyPayAtualizadas` grava `user_type=academia` no metadata e o novo `contexto_tipo`/`codigo_academia` no payload, então um FPP atento consegue detectar via ledger. Mas detecção não substitui prevenção aqui.

**Correção sugerida:** em `AtualizarCredencial`, quando `autorTipo == "academia"`, forçar `c.ContextoTipo = old.ContextoTipo` e `c.CodigoAcademia = old.CodigoAcademia` após `buildCredential(in)` (ignorando o que veio no body para esses dois campos), ou rejeitar com `400` se `in.ContextoTipo`/`in.CodigoAcademia` divergirem do registro antigo quando o ator não for fpp/admin.

---

## Itens de rounds anteriores — sem mudança

- Reembolso/reversão/reconciliação continuam só com o evento "solicitado"/"executado" inicial, sem função de conclusão.
- Validação "estudante pertence à academia" em `GerarCobrancaFinanceiraBase` continua condicionada a `metadata["codigo_academia_estudante"]` ser informado pelo chamador — não há consulta estrutural a `projection_estudantes`.
- Não existe `Provider` HTTP real contra a AppyPay; `FakeProvider` continua sendo o único usado em runtime.

---

## Checklist atualizado

- [x] `FINANCE_ENCRYPTION_KEY` validada por `ENV` (não mais `GO_ENV`), com falha antecipada no boot
- [x] Escritor único das projeções públicas financeiras (`FinanceiroProjection` assíncrona)
- [x] Mutex global não trava mais durante chamada externa ao provider
- [ ] ❌ **Nova regressão:** janela de corrida permite duplo envio de cobrança com a mesma `referencia_externa` durante a chamada ao provider
- [x] `codigo_academia` populado no contexto — autoatendimento de academia funcional
- [ ] ❌ **Nova vulnerabilidade exposta:** `AtualizarCredencial` permite academia reatribuir `contexto_tipo`/`codigo_academia` da própria credencial para outro tenant/escopo
- [ ] ⚠️ Fallback de chave de criptografia para valor derivado de string fixa ainda ativo fora de `ENV=production` (residual, baixa prioridade)

## Recomendação de ordem de correção

1. Reservar a chave de idempotência (`UniqueOperationGuard` ou equivalente) **antes** de soltar `s.mu` em `GerarCobrancaFinanceiraBase` — é o item de maior risco financeiro direto.
2. Travar `ContextoTipo`/`CodigoAcademia` contra o valor antigo em `AtualizarCredencial` quando o ator é `academia`.
3. Adicionar testes de concorrência (duas goroutines chamando `GerarCobrancaFinanceiraBase` com a mesma `referencia_externa` simultaneamente, com um `Provider` fake que introduz latência artificial) e teste de reescrita de tenant (`academia` tentando `PUT` com `codigo_academia` de outra academia sobre credencial própria).
4. Opcional: eliminar de vez o fallback de chave de criptografia fora de produção, exigindo `FINANCE_ENCRYPTION_KEY` sempre.

## Comandos de validação sugeridos

```bash
go test ./internal/finance/... -race -run TestIsolamento -v   # -race ajuda a expor a corrida de idempotência
```

Teste manual recomendado para a corrida: dois `go test` concorrentes (ou `t.Run` em paralelo com `sync.WaitGroup`) chamando `GerarCobrancaFinanceiraBase` com o mesmo `ReferenciaExterna` contra um `Provider` fake com `time.Sleep` artificial — hoje deve produzir dois eventos `CobrancaFinanceiraCriada` distintos para a mesma referência.

Teste manual para o segundo achado: autenticar como `academia` dona de uma credencial, chamar `PUT /financeiro/appypay/credenciais/:id` com `codigo_academia` de outra academia no body, e confirmar em `financeiro_credenciais_appypay`/`spuri_ledger` que o registro foi reatribuído.