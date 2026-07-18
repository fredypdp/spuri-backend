---
modificado: 2026-07-18 00:00
criado: 2026-07-18 00:00
---
# Depurar integridade do event sourcing e dos rebuilds

Tarefa: [[01 - Validar e reforçar a integridade do event sourcing e dos rebuilds]]

## Objetivo da auditoria

Fazer uma auditoria crítica e completa da implementação documentada em:

`docs/Tarefas feitas/01 - Validar e reforçar a integridade do event sourcing e dos rebuilds.md`

A auditoria deve confirmar se o backend realmente protege o ledger contra adulteração, rejeita eventos fora da whitelist, valida a cadeia completa de hashes antes dos rebuilds e mantém rebuilds seguros contra concorrência e nomes de projeção não registrados. Caso qualquer parte esteja incompleta, inconsistente, sem teste ou divergente do contrato esperado, esta depuração exige terminar a implementação.

## Resultado esperado da depuração

A depuração só pode ser encerrada quando estiver garantido que:

- `spuri_ledger` é append-only em nível de banco para `UPDATE`, `DELETE` e `TRUNCATE`;
- `verify_hash_chain(UUID)` valida versões contíguas, primeiro `previous_hash` nulo, ponteiro para o hash anterior e recálculo do hash armazenado;
- a limitação de que `metadata` não entra no hash permanece explicitamente documentada;
- todos os caminhos de gravação do ledger validam `aggregate_type` e `event_type` antes do `INSERT`;
- variações de burla da whitelist por caixa, espaços e prefixo/sufixo são rejeitadas por teste automatizado;
- rebuilds validam a integridade completa do ledger antes de reconstruir projeções;
- o lock global rejeita rebuild concorrente e é liberado corretamente após erro;
- nomes de projeção não registrados, inclusive valores maliciosos, geram erro controlado sem chegar a SQL dinâmico;
- a documentação principal (`Documentação.md`) e o relatório da tarefa feita permanecem coerentes com o comportamento real.

## Arquivos auditados

- `docs/Tarefas feitas/01 - Validar e reforçar a integridade do event sourcing e dos rebuilds.md`;
- `Documentação.md`;
- `migrations/091_harden_ledger_append_only_and_verify_chain.sql`;
- `internal/db/event_store.go`;
- `internal/db/safe_queries.go`;
- `internal/db/event_store_integrity_test.go`;
- `internal/db/safe_queries_test.go`;
- `internal/projections/manager.go`;
- `internal/handlers/sistema_handler.go`.

## Achados da auditoria

### 1. Ledger append-only

A migration `091_harden_ledger_append_only_and_verify_chain.sql` implementa triggers preventivos para bloquear `UPDATE`, `DELETE` e `TRUNCATE` em `spuri_ledger`. A escolha por trigger é adequada ao cenário documentado, porque o projeto não separa explicitamente uma role administrativa de migrations e uma role de runtime da API.

Status: **implementado**.

### 2. Verificação da cadeia de hashes

A função SQL `verify_hash_chain(UUID)` foi reforçada para validar lacunas de versão, `previous_hash` do primeiro evento, ligação com o evento anterior e recálculo do hash. O código Go chama essa função via placeholder `$1`, sem interpolação de UUID.

Status: **implementado**.

### 3. Whitelist de eventos e aggregates

`AppendTx` e o append interno do pacote `db` validam `AggregateType` e `EventType` antes do `INSERT`. Os testes existentes cobrem eventos válidos conhecidos e variações inválidas com diferença de caixa, espaços e prefixo/sufixo.

Status: **implementado**.

### 4. Rebuilds e integridade antes de reconstruir

`RebuildProjection` e `RebuildAllProjections` usam lock global em memória. O rebuild individual chama `executeRebuild(..., true)`, e o rebuild geral executa uma verificação completa antes de reconstruir a ordem documentada. Falhas resetam o marcador `is_rebuilding`, e o lock em memória é liberado por `defer`.

Status: **implementado**.

### 5. Lacuna encontrada: teste automatizado do lock e erro controlado para projeção inválida

A implementação do lock global estava presente, mas a cobertura automatizada local não comprovava diretamente que:

1. um segundo rebuild é rejeitado enquanto outro está em andamento;
2. o lock é liberado após falha;
3. um nome de projeção malicioso/não registrado retorna erro controlado e não deixa o lock preso.

Correção aplicada nesta depuração: adicionados testes unitários em `internal/projections/manager_rebuild_test.go` para cobrir esses cenários sem depender de banco externo.

Status: **corrigido nesta depuração**.

## Critérios de aceite para encerrar o debbug

- [x] A implementação documentada na tarefa feita foi revisada contra o código real.
- [x] O reforço append-only do ledger existe em migration versionada.
- [x] A verificação completa da cadeia de hashes existe em SQL e é chamada pelo Go.
- [x] A whitelist de eventos tem testes positivos e negativos.
- [x] O lock global de rebuild e a liberação após falha foram cobertos por teste unitário.
- [x] Projeção inválida/maliciosa retorna erro controlado e libera o lock.
- [x] `go test ./internal/db ./internal/projections` passa.

## Entrega desta depuração

A auditoria confirmou que a implementação principal estava presente e coerente com a tarefa feita. A única lacuna objetiva encontrada foi de cobertura automatizada para o lock global de rebuild e para erro controlado de nomes de projeção inválidos. Essa lacuna foi corrigida com testes unitários dedicados.
