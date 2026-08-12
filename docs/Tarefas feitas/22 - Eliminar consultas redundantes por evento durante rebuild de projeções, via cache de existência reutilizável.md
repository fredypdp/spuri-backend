---
criado: 2026-08-10 00:00
origem: Teste de rebuild da projeção "estudantes" em staging (Render + NeonDB) + code review em github.com/fredypdp/spuri-backend — conversa com Claude
status: a fazer
---

# 22 — Eliminar consultas redundantes por evento durante rebuild de projeções, via cache de existência reutilizável (a fazer)

## Prompt recomendado para executar a atualização

Implemente exatamente as três mudanças descritas na seção "Escopo obrigatório" deste documento: (1) criar o tipo reutilizável `ExistenceCache` em `internal/projections/projection.go`; (2) em `internal/projections/estudante_projection.go`, parametrizar `handleEstudanteCriadoComVinculo` e `resolveCursoIDs` para aceitarem a função de checagem como parâmetro (preservando o comportamento e as mensagens de erro exatamente como estão hoje), e adaptar `Rebuild()` para construir dois `ExistenceCache` (academias, cursos) uma única vez no início e usá-los via um novo método `handleForRebuild`; (3) aplicar o mesmo padrão em `internal/projections/materias_projection.go` (`handleMateriaCriada`, um único cache de academias). NÃO altere `Handle()` em nenhum dos dois arquivos — ele deve continuar fazendo a checagem direta ao banco exatamente como hoje, sem cache, pois é o caminho usado pelo processamento ao vivo. NÃO altere `academiaExists`, `cursoExists`, `defaultRebuildOrder`, `ErrMateriaAcademiaNotProjected`, nenhuma mensagem de erro, nenhuma regra de negócio, nem qualquer projeção além dessas duas (o restante foi auditado nesta tarefa e não tem o mesmo padrão — ver seção "Fora de escopo"). O `ExistenceCache` deve ser uma variável local ao `Rebuild()`, nunca um campo de struct — isso é obrigatório para não introduzir concorrência com o processamento ao vivo, que roda em paralelo (ver "Contexto"). Ao final, rode `gofmt`, `go build ./...`, `go vet ./...` e `go test ./...`, e confirme cada item da seção "Critérios de aceite" antes de reportar a tarefa como concluída.

## Contexto

Um teste manual de rebuild da projeção "estudantes" em staging (1821 estudantes, ~2400+ eventos no total, contando eventos de ciclo de vida além da criação) levou a projeção a ficar reconstruindo por mais de 20 minutos contínuos, mantendo o NeonDB ativo o tempo todo — evidência: logs do Render mostrando uma linha `[DEBUG] [estudantes] Estudante X projetado` a cada ~500-600ms, e o painel Monitoring do NeonDB mostrando essa janela de compute ativo bem mais longa que as janelas normais (que, depois da Tarefa 21, já caíram para 5-7 minutos).

**Isto não é o mesmo problema das Tarefas 19/20/21.** Aquelas tarefas eliminaram atividade *falsa* — polling residual sem trabalho real por trás. Aqui o trabalho é genuíno (aplicar milhares de eventos reais numa reconstrução completa), e o NeonDB precisa mesmo ficar acordado durante esse tempo. A pergunta é: **esse trabalho real precisa demorar tanto?**

### Diagnóstico (código lido e confirmado)

Em `internal/projections/estudante_projection.go`, `handleEstudanteCriadoComVinculo` (chamado uma vez por estudante criado, e por `Rebuild()` para todo o histórico) faz, sequencialmente, **até 4 consultas ao banco por evento**:

1. `academiaExists(payload.CodigoAcademia)` — 1 `SELECT` em `projection_academias`.
2. `resolveCursoIDs(...)` → `cursoExists(...)` — até 2 `SELECT`s em `projection_cursos` (um para curso médio, um para curso superior, se aplicável).
3. O `INSERT ... ON CONFLICT DO UPDATE` final — 1 `Exec`.

Isso bate com precisão com os ~500-600ms observados entre cada linha do log (3-4 idas e vindas sequenciais × latência de rede até o NeonDB). Em `internal/projections/materias_projection.go`, `handleMateriaCriada` tem o mesmo padrão, com 1 checagem (`academiaExists`) antes do `INSERT`.

**Essas checagens não são acidente — são a correção deliberada de um bug crítico documentado no próprio código** (`FIX BUG #2` em `estudante_projection.go`, `ErrMateriaAcademiaNotProjected` em `materias_projection.go`): como o Projection Manager processa cada projeção de forma independente, durante o **processamento ao vivo** um evento `EstudanteCriadoComVinculo` pode legitimamente chegar antes do evento `AcademiaCriada` correspondente ter sido projetado. Sem a checagem, o `INSERT` quebraria (ou pior, inseriria um `codigo_academia` órfão), e a correção existente lida com isso corretamente: retorna erro temporário (estudantes) ou um erro sentinel que o `Rebuild()` sabe ignorar (`materias`), fazendo o evento ser reprocessado no próximo ciclo.

**A pergunta que valida a otimização: essa condição de corrida é possível durante um rebuild especificamente?** Não, por dois motivos confirmados no código:

- `defaultRebuildOrder` (`manager.go`) lista `academias` e `cursos` **antes** de `estudantes` e `materias`, e `RebuildAllProjections` executa cada projeção **sequencialmente**, nunca em paralelo (`for _, name := range orderedRebuildProjectionNames(snapshot) { ...executeRebuild... }`). Quando "estudantes" ou "materias" começam a reconstruir, "academias" e "cursos" já estão 100% reconstruídos.
- Um `RebuildProjection("estudantes")` isolado (não "all") não toca nas outras projeções — elas permanecem como estavam, presumivelmente já corretas.

Ou seja: durante rebuild, a checagem está praticamente sempre a retornar verdadeiro, gastando round-trips que o cenário não precisa. Durante o processamento ao vivo, ela continua absolutamente necessária.

### Por que um cache com fallback, e não simplesmente remover a checagem durante rebuild

Remover a checagem por completo durante rebuild seria arriscado: depende inteiramente da suposição de que nada mais escreve no banco durante a janela do rebuild, o que não é garantido pelo código (`beginRebuild`/`endRebuild`, em `manager.go`, usa um mutex que impede **rebuilds concorrentes entre si**, mas não pausa o loop de processamento ao vivo — `processNewEvents()` continua rodando na mesma instância de projeção em paralelo, se uma escrita real acontecer durante o rebuild). Um cache com fallback resolve isso sem depender dessa suposição: ele resolve a maioria esmagadora das checagens a partir de um snapshot em memória (populado por **uma única consulta em lote** no início do rebuild), e qualquer chave que não estiver no snapshot cai automaticamente para a mesma checagem direta ao banco usada hoje — preservando exatamente a mesma garantia de correção, inclusive nesse cenário de escrita concorrente durante o rebuild.

**Importante para quem for implementar:** o cache deve ser uma variável local dentro de `Rebuild()`, nunca um campo da struct da projeção. A struct `EstudanteProjection`/`MateriasProjection` hoje não tem nenhum estado mutável compartilhado (só `client *db.Client`, que já é seguro para uso concorrente) — é exatamente isso que torna seguro `Handle()` ser chamado ao mesmo tempo pelo rebuild e pelo processamento ao vivo, hoje. Adicionar um campo mutável quebraria essa garantia. Manter o cache como variável local elimina o problema por construção: nada é compartilhado entre goroutines.

## Objetivo

Reduzir drasticamente o tempo de wall-clock que uma reconstrução de projeção mantém o NeonDB ativo, sem enfraquecer nenhuma proteção de correção do Event Sourcing:

- Trocar os round-trips repetidos de checagem de existência por evento, durante rebuild, por um cache em memória populado uma única vez por rebuild.
- Preservar 100% o comportamento de `Handle()` no processamento ao vivo — nenhuma mudança de correção, mensagens de erro, ou timing de retry nesse caminho.
- Preservar 100% o comportamento observável do `Rebuild()` em caso de cache miss (fallback para a checagem real, idêntica à de hoje).
- Entregar o cache como um tipo reutilizável (`ExistenceCache`), documentado o suficiente para que uma projeção futura com o mesmo padrão (checagem de existência de uma entidade pré-requisito de outra projeção, durante rebuild) possa adotá-lo com poucas linhas, sem duplicar a lógica de cache.

## Escopo obrigatório

### 1. Criar `ExistenceCache` reutilizável — `internal/projections/projection.go`

#### Correção esperada

Adicionar ao final do arquivo:

```go
// ============================================================================
// Cache de existência para uso exclusivo durante Rebuild()
// ============================================================================

// ExistenceCache acelera checagens repetidas de "esta chave já existe numa
// tabela de outra projeção" durante o REBUILD de uma projeção, evitando uma
// consulta ao banco por evento. NÃO deve ser usado no caminho de
// processamento ao vivo (Handle chamado a partir do processamento normal do
// Manager) — lá a verificação direta ao banco continua obrigatória, porque
// projeções diferentes processam de forma independente e a entidade
// pré-requisito pode legitimamente ainda não existir (condição de corrida
// documentada, por exemplo, como FIX BUG #2 em estudante_projection.go).
//
// Uso seguro durante rebuild: a ordem de reconstrução (defaultRebuildOrder,
// em manager.go) garante que projeções pré-requisito já estão totalmente
// reconstruídas antes de uma projeção dependente começar, e um rebuild de
// uma única projeção via RebuildProjection não altera as demais. Um snapshot
// inicial das chaves válidas é, portanto, seguro na esmagadora maioria dos
// casos. Para não depender inteiramente dessa suposição — por exemplo, se
// uma escrita ao vivo legítima criar uma nova academia exatamente durante a
// janela do rebuild — toda consulta que não encontra a chave no snapshot cai
// automaticamente para a mesma checagem direta ao banco usada hoje. O
// comportamento observável nunca muda; só o número de consultas no caso
// comum (chave já existente) é que cai.
//
// Cada instância deve viver apenas durante uma única chamada a Rebuild() —
// nunca deve ser guardada como campo de uma struct de projeção. Isso evita
// introduzir estado mutável compartilhado entre a goroutine do rebuild e a
// goroutine de processamento ao vivo, que podem rodar concorrentemente sobre
// a mesma instância de projeção.
type ExistenceCache struct {
	mu       sync.Mutex
	known    map[string]struct{}
	fallback func(key string) (bool, error)
}

// NewExistenceCache cria um cache pré-populado com as chaves de seed (ex.:
// todos os codigo_academia já presentes em projection_academias no início do
// rebuild) e uma função de fallback — deve ser exatamente a mesma checagem
// direta ao banco já usada hoje (ex.: o método academiaExists existente),
// para qualquer chave que não estiver no seed.
func NewExistenceCache(seed []string, fallback func(key string) (bool, error)) *ExistenceCache {
	known := make(map[string]struct{}, len(seed))
	for _, k := range seed {
		known[k] = struct{}{}
	}
	return &ExistenceCache{known: known, fallback: fallback}
}

// Exists devolve exatamente o mesmo resultado que a checagem direta ao banco
// devolveria, resolvendo a partir do snapshot em memória sempre que possível.
func (c *ExistenceCache) Exists(key string) (bool, error) {
	c.mu.Lock()
	_, ok := c.known[key]
	c.mu.Unlock()
	if ok {
		return true, nil
	}

	exists, err := c.fallback(key)
	if err != nil {
		return false, err
	}
	if exists {
		c.mu.Lock()
		c.known[key] = struct{}{}
		c.mu.Unlock()
	}
	return exists, nil
}
```

Adicionar `"sync"` aos imports de `projection.go`, se ainda não estiver presente.

#### Cuidados

- Não tornar `ExistenceCache` um campo de `BaseProjection` nem exigir que as projeções o embutam — deve funcionar como um tipo independente, usável por qualquer arquivo do pacote `projections` via variável local.
- Não adicionar nenhum mecanismo de expiração/TTL — o cache vive só pela duração de um `Rebuild()`, não precisa disso.

---

### 2. `internal/projections/estudante_projection.go`

#### Correção esperada

**a) Parametrizar `handleEstudanteCriadoComVinculo`** — extrair o corpo atual (linhas 160–317) para uma nova função `applyEstudanteCriadoComVinculo`, idêntica em tudo exceto que recebe as duas funções de checagem como parâmetros, e fazer `handleEstudanteCriadoComVinculo` delegar a ela usando as checagens diretas de sempre:

```go
func (p *EstudanteProjection) handleEstudanteCriadoComVinculo(event db.Event) error {
	return p.applyEstudanteCriadoComVinculo(event, p.academiaExists, p.cursoExistsChecker())
}

func (p *EstudanteProjection) applyEstudanteCriadoComVinculo(
	event db.Event,
	checkAcademiaExists func(string) (bool, error),
	checkCursoExists func(string) (bool, error),
) error {
	// Corpo idêntico ao atual de handleEstudanteCriadoComVinculo, com apenas
	// duas substituições:
	//   academiaExists, err := p.academiaExists(payload.CodigoAcademia)
	// vira:
	//   academiaExists, err := checkAcademiaExists(payload.CodigoAcademia)
	//
	//   resolvedCursoMedio, resolvedCursoSuperior := p.resolveCursoIDs(
	//       cursoMedioIDStr, cursoSuperiorIDStr, event.EventID,
	//   )
	// vira:
	//   resolvedCursoMedio, resolvedCursoSuperior := p.resolveCursoIDsWithChecker(
	//       cursoMedioIDStr, cursoSuperiorIDStr, event.EventID, checkCursoExists,
	//   )
	//
	// Nenhuma outra linha muda — mesmo parsing de payload, mesmas mensagens
	// de erro, mesmo INSERT, mesmo log final.
}
```

`cursoExistsChecker()` é só um adaptador trivial para o caminho ao vivo (mantém `resolveCursoIDs` existente funcionando sem mudar sua assinatura pública):

```go
func (p *EstudanteProjection) cursoExistsChecker() func(string) (bool, error) {
	return p.cursoExists
}
```

**b) Parametrizar `resolveCursoIDs`** — mesma extração, preservando a função original como wrapper fino:

```go
func (p *EstudanteProjection) resolveCursoIDs(
	cursoMedioID *string,
	cursoSuperiorID *string,
	eventID uuid.UUID,
) (*string, *string) {
	return p.resolveCursoIDsWithChecker(cursoMedioID, cursoSuperiorID, eventID, p.cursoExists)
}

func (p *EstudanteProjection) resolveCursoIDsWithChecker(
	cursoMedioID *string,
	cursoSuperiorID *string,
	eventID uuid.UUID,
	checkCursoExists func(string) (bool, error),
) (*string, *string) {
	// Corpo idêntico ao atual de resolveCursoIDs, trocando as duas chamadas
	// a p.cursoExists(...) por checkCursoExists(...).
}
```

**c) Adaptar `Rebuild()`** para construir os caches uma única vez e rotear apenas o tipo de evento que precisa deles:

```go
func (p *EstudanteProjection) Rebuild() error {
	log.Printf("[DEBUG] [estudantes] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar projection_estudantes: %w", err)
	}

	academiaCache, err := p.newAcademiaExistenceCache()
	if err != nil {
		return fmt.Errorf("erro ao preparar cache de academias para rebuild: %w", err)
	}
	cursoCache, err := p.newCursoExistenceCache()
	if err != nil {
		return fmt.Errorf("erro ao preparar cache de cursos para rebuild: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Estudante'
		ORDER BY id ASC
	`)
	if err != nil {
		return fmt.Errorf("erro ao buscar eventos para rebuild: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var event db.Event
		var prevHash sql.NullString
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		); err != nil {
			return fmt.Errorf("erro ao escanear evento %d: %w", count, err)
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		if err := p.handleForRebuild(event, academiaCache, cursoCache); err != nil {
			return fmt.Errorf("erro ao processar evento %d (type=%s): %w", event.ID, event.EventType, err)
		}
		count++
	}

	log.Printf("[DEBUG] [estudantes] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

// handleForRebuild despacha um evento durante Rebuild(). Para o único tipo de
// evento que consulta outra projeção por evento (EstudanteCriadoComVinculo),
// usa os caches em memória em vez da checagem direta ao banco. Todos os
// outros tipos de evento continuam por Handle(), sem nenhuma mudança de
// comportamento em relação a hoje.
func (p *EstudanteProjection) handleForRebuild(event db.Event, academiaCache, cursoCache *ExistenceCache) error {
	if event.AggregateType == "Estudante" && event.EventType == "EstudanteCriadoComVinculo" {
		return p.applyEstudanteCriadoComVinculo(event, academiaCache.Exists, cursoCache.Exists)
	}
	return p.Handle(event)
}

func (p *EstudanteProjection) newAcademiaExistenceCache() (*ExistenceCache, error) {
	rows, err := p.client.DB().Query(`SELECT codigo_academia FROM projection_academias`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seed []string
	for rows.Next() {
		var codigo string
		if err := rows.Scan(&codigo); err != nil {
			return nil, err
		}
		seed = append(seed, codigo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewExistenceCache(seed, p.academiaExists), nil
}

func (p *EstudanteProjection) newCursoExistenceCache() (*ExistenceCache, error) {
	rows, err := p.client.DB().Query(`SELECT id::text FROM projection_cursos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seed []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		seed = append(seed, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewExistenceCache(seed, p.cursoExists), nil
}
```

#### Cuidados

- Não alterar `academiaExists`, `cursoExists`, `clear`, `Handle`, `Name`, `GetLastProcessedEventID`, `UpdateCheckpoint`, nem qualquer handler de outro tipo de evento (`handleFundamentalRetomado`, `handleDadosPessoaisAtualizados`, etc.).
- `applyEstudanteCriadoComVinculo` e `resolveCursoIDsWithChecker` devem conter exatamente a mesma lógica, mesmas mensagens de erro (incluindo as que citam `payload.CodigoAcademia`, `event.ID`, etc.) e mesmos comentários `FIX BUG #1-#4` das funções originais — só os dois call sites de checagem mudam.
- `handleForRebuild` deve comparar `event.AggregateType` **e** `event.EventType` antes de rotear para o caminho com cache — qualquer outro tipo de evento (incluindo outros eventos do aggregate `Estudante`) deve cair em `p.Handle(event)` inalterado.
- Confirmar, depois da mudança, que não há nenhum outro call site de `p.academiaExists` ou `p.cursoExists` além dos já identificados: `rg -n "p\.academiaExists\(|p\.cursoExists\(" internal/projections/estudante_projection.go`.

---

### 3. `internal/projections/materias_projection.go`

#### Correção esperada

Mesmo padrão, com um único cache (academias). **a)** Parametrizar `handleMateriaCriada`:

```go
func (p *MateriasProjection) handleMateriaCriada(event db.Event) error {
	return p.applyMateriaCriada(event, p.academiaExists)
}

func (p *MateriasProjection) applyMateriaCriada(
	event db.Event,
	checkAcademiaExists func(string) (bool, error),
) error {
	// Corpo idêntico ao atual de handleMateriaCriada, trocando apenas:
	//   academiaExists, err := p.academiaExists(payload.CodigoAcademia)
	// por:
	//   academiaExists, err := checkAcademiaExists(payload.CodigoAcademia)
}
```

**b)** Adaptar `Rebuild()`:

```go
func (p *MateriasProjection) Rebuild() error {
	log.Printf("[DEBUG] [materias] Rebuild iniciado")
	if _, err := p.client.DB().Exec(`TRUNCATE TABLE projection_materias CASCADE`); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	academiaCache, err := p.newAcademiaExistenceCache()
	if err != nil {
		return fmt.Errorf("erro ao preparar cache de academias para rebuild: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'MateriaDisciplinar'
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	skipped := 0
	for rows.Next() {
		var event db.Event
		var prevHash sql.NullString
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		); err != nil {
			return err
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		if err := p.handleForRebuild(event, academiaCache); err != nil {
			if errors.Is(err, ErrMateriaAcademiaNotProjected) {
				skipped++
				log.Printf("[WARN] [materias] evento %d ignorado no rebuild: %v", event.ID, err)
				continue
			}
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}
	log.Printf("[DEBUG] [materias] Rebuild concluído: %d eventos processados, %d ignorados por academia ausente", count, skipped)
	return rows.Err()
}

func (p *MateriasProjection) handleForRebuild(event db.Event, academiaCache *ExistenceCache) error {
	if event.AggregateType == "MateriaDisciplinar" && event.EventType == "MateriaCriada" {
		return p.applyMateriaCriada(event, academiaCache.Exists)
	}
	return p.Handle(event)
}

func (p *MateriasProjection) newAcademiaExistenceCache() (*ExistenceCache, error) {
	rows, err := p.client.DB().Query(`SELECT codigo_academia FROM projection_academias`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seed []string
	for rows.Next() {
		var codigo string
		if err := rows.Scan(&codigo); err != nil {
			return nil, err
		}
		seed = append(seed, codigo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewExistenceCache(seed, p.academiaExists), nil
}
```

#### Cuidados

- Preservar exatamente o `skipped`/`ErrMateriaAcademiaNotProjected` já existente — o cache não muda esse comportamento, só a origem do resultado "não existe".
- Não alterar `academiaExists`, `Handle`, `handleStatusChange`, `handleMateriaDadosAtualizados`, `handleMateriaPeriodoDefinido`, `handleMateriaDeletada`.
- Confirmar, depois da mudança: `rg -n "p\.academiaExists\(" internal/projections/materias_projection.go` só deve aparecer dentro de `handleMateriaCriada` original (agora removido/substituído) e na nova função `newAcademiaExistenceCache`.

## Fora de escopo

Não fazer nesta tarefa:

- **Envolver o rebuild inteiro (ou em lotes) numa transação (`*sql.Tx`)** para reduzir o custo de commit por `INSERT`/`UPDATE`. Essa é uma otimização real e complementar, mas de categoria diferente e mais arriscada: exigiria que cada `p.client.DB().Exec(...)` dentro dos handlers aceitasse um `db.Queryer`/`*sql.Tx` em vez do pool direto (mudança em muito mais call sites), e uma transação muito longa (milhares de statements) tem implicações próprias de duração de lock e uso de memória no lado do NeonDB que merecem avaliação própria. Considerar como tarefa futura separada, só se a otimização desta tarefa não for suficiente.
- **Consolidar o boilerplate comum de `Rebuild()`** (limpar → consultar ledger por `aggregate_type` → laço → `Handle`) das 13 projeções num único helper genérico em `BaseProjection`. É uma limpeza de código legítima, mas amplia o raio de mudança desta tarefa para todas as projeções sem necessidade — o problema de desempenho identificado está concentrado em só duas. Pode virar uma tarefa de refatoração própria no futuro.
- **Alterar `avaliacao_final_projection.go`, `solicitacao_edicao_dado_estudante_projection.go`, `turmas_projection.go`, `admin_projection.go`.** Todos os quatro foram auditados nesta investigação e usam `SELECT EXISTS`/`QueryRow` por motivos diferentes do padrão corrigido aqui: `avaliacao_final` e `solicitacao_edicao_dado_estudante` expõem métodos de checagem usados por **handlers HTTP de negócio** (ex.: `ExistsByEstudanteAnoLetivoNivelType`, `ExistePendente`), não pelo próprio `Rebuild()`; `turmas_projection.go` usa `NOT EXISTS` dentro de uma única query CTE de alocação de turma, não uma checagem por evento; `admin_projection.go` faz uma checagem de unicidade de e-mail **dentro da própria tabela** (`handleAdminDadosAtualizados`), um invariante de negócio, não uma dependência cronológica entre projeções, e com volume de eventos tipicamente baixo (poucos admins por academia).
- Alterar `Handle()`, `processProjection`, `processEventTransactional`, `wakeCh`, `defaultRebuildOrder`, `beginRebuild`/`endRebuild`, ou qualquer parte do Manager além do necessário.
- Alterar mensagens de erro, códigos de erro sentinela (`ErrMateriaAcademiaNotProjected`), ou qualquer regra de negócio.
- Adicionar métricas, observabilidade ou logging além do já existente nos trechos alterados.

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual.
2. Adicionar `ExistenceCache` e `NewExistenceCache` a `internal/projections/projection.go`, com o import `"sync"`.
3. Rodar `gofmt` e `go build ./internal/projections/...`.
4. Editar `internal/projections/estudante_projection.go`: extrair `applyEstudanteCriadoComVinculo` e `resolveCursoIDsWithChecker`, ajustar os wrappers `handleEstudanteCriadoComVinculo`/`resolveCursoIDs`/`cursoExistsChecker`, e adaptar `Rebuild()` com `handleForRebuild`, `newAcademiaExistenceCache`, `newCursoExistenceCache`.
5. Rodar `gofmt` e `go build ./internal/projections/...`.
6. Editar `internal/projections/materias_projection.go`: mesma extração para `applyMateriaCriada`, e adaptar `Rebuild()` com `handleForRebuild`, `newAcademiaExistenceCache`.
7. Rodar `gofmt` e `go build ./...`.
8. Rodar `rg -n "p\.academiaExists\(|p\.cursoExists\(" internal/projections/estudante_projection.go` e `rg -n "p\.academiaExists\(" internal/projections/materias_projection.go` e confirmar que os únicos call sites diretos restantes são dentro das novas funções `newXExistenceCache` e nos wrappers do caminho ao vivo (`handleEstudanteCriadoComVinculo`, `cursoExistsChecker`, `handleMateriaCriada`).
9. Rodar `go vet ./...` e `go test ./...`.
10. Revisar o diff completo e confirmar que nenhum arquivo fora de `internal/projections/projection.go`, `internal/projections/estudante_projection.go` e `internal/projections/materias_projection.go` foi alterado.

## Critérios de aceite

- [ ] `internal/projections/projection.go` contém `ExistenceCache`, `NewExistenceCache` e o método `Exists`, com o comportamento de cache-hit e fallback-em-miss descrito nesta tarefa.
- [ ] `internal/projections/estudante_projection.go`: `handleEstudanteCriadoComVinculo` delega para `applyEstudanteCriadoComVinculo`, que aceita `checkAcademiaExists`/`checkCursoExists` como parâmetros.
- [ ] `internal/projections/estudante_projection.go`: `resolveCursoIDs` delega para `resolveCursoIDsWithChecker`, que aceita `checkCursoExists` como parâmetro.
- [ ] `internal/projections/estudante_projection.go`: `Rebuild()` constrói `academiaCache`/`cursoCache` uma única vez, antes do laço de eventos, e usa `handleForRebuild` para cada evento.
- [ ] `internal/projections/estudante_projection.go`: `handleForRebuild` só desvia para o caminho com cache quando `event.AggregateType == "Estudante"` e `event.EventType == "EstudanteCriadoComVinculo"`; todo o resto passa por `p.Handle(event)` inalterado.
- [ ] `internal/projections/materias_projection.go`: mesmo padrão aplicado a `handleMateriaCriada`/`applyMateriaCriada` e `Rebuild()`/`handleForRebuild`, preservando o `skipped`/`ErrMateriaAcademiaNotProjected` existente.
- [ ] `Handle()` em ambos os arquivos permanece byte-a-byte idêntico ao estado atual (nenhuma mudança no caminho de processamento ao vivo).
- [ ] Nenhum outro arquivo além dos três listados no escopo foi alterado.
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `go test ./...` passa, ou qualquer falha é documentada com causa comprovadamente não relacionada a esta tarefa.

## Checks manuais recomendados após deploy em staging

### 1. Confirmar redução real do tempo de rebuild

1. Anotar o tempo de início e fim de um `RebuildProjection("estudantes")` em staging (mesmo volume de dados do teste que originou esta tarefa, se possível).
2. Comparar com a duração observada antes desta tarefa (~20+ minutos para ~2400 eventos). Esperado: redução substancial, já que a maioria dos eventos de criação passa a resolver as checagens em memória.
3. Confirmar no painel Monitoring do NeonDB que a janela de compute ativo correspondente encolheu proporcionalmente.

### 2. Confirmar que o resultado do rebuild é idêntico ao de antes

1. Antes de aplicar esta tarefa, gerar um dump/contagem de `projection_estudantes` e `projection_materias` após um rebuild completo (ex.: `SELECT COUNT(*), codigo_academia FROM projection_estudantes GROUP BY codigo_academia`, mesma coisa para materias).
2. Depois de aplicar esta tarefa, rodar o rebuild novamente e comparar — os números devem ser idênticos, já que nenhuma lógica de negócio mudou.

### 3. Confirmar que o processamento ao vivo continua sem regressão

1. Com o backend em staging e sem nenhum rebuild em andamento, criar um estudante novo via API normalmente.
2. Confirmar que ele é projetado corretamente e sem atraso perceptível — este caminho usa `Handle()` diretamente, que não foi alterado.

### 4. (Opcional, se houver tempo) Simular o cenário de cache miss

1. Iniciar um `RebuildProjection("estudantes")` em staging.
2. Enquanto ele ainda está em andamento, criar uma academia nova e um estudante vinculado a ela via API (escrita ao vivo concorrente).
3. Confirmar nos logs que o estudante criado durante a janela do rebuild é processado corretamente (via fallback do cache, já que a academia nova não estava no snapshot inicial) — não deve haver erro nem inconsistência.

## Riscos e mitigação

### Risco: escrita concorrente ao vivo criando uma academia/curso novo durante a janela do rebuild, ausente do snapshot inicial

- Mitigado por construção: `ExistenceCache.Exists` cai automaticamente para a mesma checagem direta ao banco usada hoje sempre que a chave não está no snapshot. O comportamento observável (incluindo o retry automático em `estudantes` e o skip em `materias`) nunca muda — só o número de consultas no caso comum é que cai.

### Risco: crescimento de memória do cache para uma base muito grande de academias/cursos

- `projection_academias` e `projection_cursos` são tabelas pequenas por natureza (dezenas a poucas centenas de linhas, não milhares) — o footprint de memória de um `map[string]struct{}` com esse volume é desprezível frente ao resto do processo.

### Risco: nova concorrência entre a goroutine de rebuild e a goroutine de processamento ao vivo

- `ExistenceCache` é sempre uma variável local, criada e usada inteiramente dentro de uma única chamada a `Rebuild()` — nunca um campo de struct. Nenhum estado mutável novo é compartilhado entre goroutines; a garantia de que a struct da projeção permanece stateless (hoje só tem `client *db.Client`, já seguro para uso concorrente) continua valendo depois desta tarefa.

### Risco: `handleForRebuild` divergir silenciosamente de `Handle()` no futuro, se alguém adicionar um novo tipo de evento com checagem de existência sem atualizar os dois lugares

- Mitigação: o comentário em `handleForRebuild`, em ambos os arquivos, deixa explícito que qualquer novo tipo de evento cai em `p.Handle(event)` por padrão — só o(s) tipo(s) explicitamente listado(s) usa(m) o caminho com cache. Se uma tarefa futura adicionar um novo handler com o mesmo padrão de dependência cruzada entre projeções, deve seguir a mesma receita (extrair `applyX`, adicionar o cache correspondente, adicionar o `if` em `handleForRebuild`) — documentado aqui para reutilização.

## Observações finais

Esta tarefa é ortogonal às Tarefas 19-21: aquelas eliminaram atividade de fundo sem trabalho real por trás (polling residual); esta reduz o tempo necessário para um trabalho real e legítimo (rebuild completo de uma projeção), sem alterar nenhuma garantia de correção do Event Sourcing — a ordem de rebuild já estabelecida (`defaultRebuildOrder`) e o fallback do cache são o que tornam essa otimização segura. O padrão (`ExistenceCache` + `applyX`/`handleForRebuild` como wrappers finos) foi desenhado para ser reaproveitável por qualquer projeção futura com a mesma necessidade, sem forçar mudança nas 11 projeções que hoje não têm esse padrão.
