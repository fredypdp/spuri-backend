---
modificado: 04-03-2026 17:05
criado: 04-03-2026 17:05
---
> Executar em **4 chamadas separadas**, uma por etapa. Cada etapa gera seu próprio `.md`.

---

## CONTEXTO FIXO (repetir em todas as etapas)

O projeto é **spuri-backend**, uma API em Go que usa **Event Sourcing + CQRS**.  
Toda mutação de estado deve obrigatoriamente passar pelo ledger (`spuri_ledger`) antes de atualizar qualquer projeção.  
O sistema deve ser: **Seguro · Auditável · Confiável · Sólido**.

Critérios de auditoria:
- Nenhum `UPDATE`/`INSERT` direto em tabelas de projeção que bypass o ledger
- Todo evento tem `Apply()` correspondente no aggregate
- Todo evento gravado no ledger tem handler correspondente na projection
- Hash chain do ledger é gerada e validada corretamente
- Payloads de eventos são completos, sem campos críticos omitidos
- Erros de `json.Unmarshal` nunca são silenciados
- Validações de entrada existem antes de qualquer comando de domínio
- Não há vazamento de dados sensíveis (senhas, tokens) em respostas HTTP
- Rebuild de projeção reproduz estado idêntico ao estado atual
- Permissões e autorizações são verificadas antes de cada operação

---

## ETAPA 1 — `/domain` (Aggregates + Models)

**Arquivos a analisar:**
- `internal/domain/aggregates/aggregate.go`
- `internal/domain/aggregates/admin.go`
- `internal/domain/aggregates/academia.go`
- `internal/domain/aggregates/academia_categorias_nota.go` *(se existir)*
- `internal/domain/aggregates/estudante.go`
- `internal/domain/aggregates/estudante_falta.go`
- `internal/domain/aggregates/estudante_notas.go`
- `internal/domain/aggregates/estudante_avaliacao.go`
- `internal/domain/aggregates/estudante_aprovacao.go`
- `internal/domain/aggregates/sistema_config.go` *(se existir)*
- Todos os demais arquivos em `internal/domain/`

**O que auditar:**
1. Cada aggregate tem `Apply()` com switch cobrindo **todos** os eventos que ele pode emitir — sem evento órfão
2. Todos os `applyXxx()` propagam erros de `json.Unmarshal` (sem `_ =` silencioso)
3. Comandos validam estado antes de `RaiseEvent()` (ex: não emite evento se já está no estado alvo)
4. `BaseAggregate.RaiseEvent()` incrementa versão corretamente
5. Nenhum comando mutua estado diretamente — toda mutação passa por `Apply()`
6. Eventos contêm todos os campos necessários para rebuild fiel
7. Structs de evento têm campos suficientes para auditoria (who/when/why)
8. `GetType()` de cada aggregate corresponde exatamente ao `aggregate_type` usado no ledger

**Saída:** arquivo `auditoria-etapa1-domain.md`  
**Formato do arquivo:** apenas erros encontrados, organizados por aggregate. Para cada erro: arquivo, função/evento afetado, descrição do problema e impacto. Sem introduções, sem conclusões, sem elogios.

---

## ETAPA 2 — `/db`

**Arquivos a analisar:**
- `internal/db/` — todos os arquivos (repository, safe_queries, event_store, client, etc.)
- `migrations/` — todos os arquivos `.sql`

**O que auditar:**
1. `SaveWithAudit` e `Save` gravam no ledger **antes** de qualquer outro efeito colateral
2. Hash chain: o trigger `auto_generate_ledger_hash` usa o hash do evento anterior correto (ORDER BY `event_version DESC`, não `id DESC`)
3. A constraint `UNIQUE(aggregate_id, event_version)` está presente e não pode ser contornada
4. `prevent_ledger_modification` bloqueia `UPDATE` e `DELETE` no ledger — validar se cobre todos os casos
5. `safe_queries.go`: whitelist de `event_type` está sincronizada com todos os eventos que os aggregates emitem
6. Queries parametrizadas em todo lugar — sem concatenação de string em SQL
7. Transações: `Save` é atômico (ledger + projeção na mesma transação, ou falha total)
8. Migrations: sem `ALTER TABLE` que viole imutabilidade do ledger; colunas novas têm `DEFAULT` seguro
9. `Load()`: reconstrói aggregate na ordem correta de versão (ORDER BY `event_version ASC`)
10. Nenhum evento é perdido silenciosamente durante o `Load()` (erro de `Apply()` é propagado)

**Saída:** arquivo `auditoria-etapa2-db.md`  
**Formato:** igual à Etapa 1.

---

## ETAPA 3 — `/projections`

**Arquivos a analisar:**
- `internal/projections/` — todos os arquivos

**O que auditar:**
1. Cada projection tem handler para **todos** os `event_type` que o aggregate correspondente pode emitir
2. Nenhum handler faz `UPDATE`/`INSERT` direto sem ter recebido o evento do ledger
3. `Rebuild()` limpa a projeção **e** reseta o checkpoint — sem estado residual
4. Após rebuild, o estado final é idêntico ao que seria gerado evento a evento
5. Campos calculados (ex: totais, status derivados) são recalculados corretamente no rebuild
6. Erros de parse de payload são propagados (não `log + continue`)
7. `projection_checkpoints` é atualizado atomicamente com o processamento do evento
8. Eventos desconhecidos: comportamento está documentado e é seguro (ignorar vs. erro)
9. Campos sensíveis (senha_hash) são gravados na projeção apenas quando necessário e nunca expostos em queries de leitura pública
10. Consistência entre o schema SQL da projeção e os campos que os handlers escrevem

**Saída:** arquivo `auditoria-etapa3-projections.md`  
**Formato:** igual à Etapa 1.

---

## ETAPA 4 — `/handlers`

**Arquivos a analisar:**
- `internal/handlers/` — todos os arquivos
- `internal/middleware/` — todos os arquivos
- Arquivo de rotas (`routes.go` ou equivalente)

**O que auditar:**
1. **Cascata por rota:** para cada rota, seguir o fluxo completo:  
   `rota → middleware de auth → validação de input → comando no aggregate → Save → resposta`
2. Nenhuma rota mutua projeção diretamente (bypass do event sourcing)
3. Autenticação e autorização verificadas antes de qualquer lógica de negócio
4. Dados sensíveis (senha_hash, tokens) nunca retornados no body de resposta HTTP
5. Validações de input com `binding:"required"` e verificações de negócio antes do comando
6. Erros do `repository.Save()` são sempre tratados e retornam status HTTP correto
7. `AuditContext` (IP, userID, userType) é preenchido em todas as operações de escrita
8. Rotas administrativas protegidas por verificação de role (não apenas autenticação)
9. Idempotência: operações que deveriam ser idempotentes o são
10. Race conditions: operações de leitura-modificação-escrita são protegidas

**Saída:** arquivo `auditoria-etapa4-handlers.md`  
**Formato:** igual às etapas anteriores. Organizar por rota.

---

## INSTRUÇÃO FINAL (repetir em todas as etapas)

> Analise **todos** os arquivos da etapa sem exceção.  
> Não corrija nada — apenas identifique e descreva os erros.  
> O resultado deve ir **apenas** para o arquivo `.md` da etapa.  
> No chat, poste somente: `✅ Auditoria da Etapa X concluída — ver [nome-do-arquivo.md]`  
> Não adicione introduções, conclusões, elogios ou observações positivas no `.md` — apenas erros.
