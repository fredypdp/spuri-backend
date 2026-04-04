---
modificado: 03-04-2026 19:07
criado: 07-03-2026 01:01
---
> Use este prompt como guia sempre que pedir "refatore o handler X", "simplifique o aggregate Y", "limpe a projeção Z" ou qualquer variação. O objetivo é: **código mais simples, mais legível, mais fácil de manter — sem perder segurança, auditabilidade nem comportamento funcional.**

---

## CONTEXTO FIXO (repetir em toda refatoração)

O projeto é **spuri-backend**, uma API em Go que usa **Event Sourcing + CQRS**. Toda mutação de estado deve obrigatoriamente passar pelo ledger (`spuri_ledger`) antes de atualizar qualquer projeção. O sistema deve permanecer: **Seguro · Auditável · Confiável · Legível**.

**Antes de qualquer refatoração:**

- Leia o arquivo completo que será refatorado — nunca assuma o conteúdo de memória
- Leia os arquivos relacionados (aggregate ↔ projection ↔ handler formam uma unidade)
- Mantenha consistência com tudo que não está sendo refatorado

---

## PRINCÍPIOS GERAIS DE REFATORAÇÃO

### O que DEVE ser feito

- Extrair funções auxiliares nomeadas para lógica repetida ou aninhada demais
- Remover comentários óbvios (`// retorna erro se erro != nil`) — manter apenas comentários que explicam _por quê_, não _o quê_
- Consolidar blocos de validação dispersos em funções únicas coesas
- Usar early return para reduzir aninhamento (substituir `else` após `return`)
- Nomear variáveis com clareza (`agg` → `estudante`, `p` → `payload`)
- Eliminar código morto: funções não chamadas, variáveis não usadas, imports desnecessários
- Agrupar logicamente: constantes juntas, tipos juntos, funções de comando juntas, apply handlers juntos
- Manter o arquivo compilável e testável após cada mudança

### O que NÃO pode ser feito

- **Nunca** remover o `Apply()` após `RaiseEvent()` — a mutação de estado em memória é obrigatória
- **Nunca** escrever direto em tabela de projeção sem passar pelo ledger
- **Nunca** silenciar erros de `json.Unmarshal` com `_ =` ou `log + continue` sem retornar erro
- **Nunca** omitir `AuditContext` (IP, userID, userType) em `SaveWithAudit`
- **Nunca** alterar o nome de um `EventType` string — isso quebra o ledger histórico e o rebuild
- **Nunca** remover campos de eventos já gravados no ledger — apenas adicionar como ponteiros (`*string`, `*uuid.UUID`) com valor nil-safe
- **Nunca** reordenar o switch de `Apply()` de forma que quebre o mapeamento evento → handler
- **Nunca** mover lógica de validação de negócio do aggregate para o handler
- **Nunca** alterar o schema do `spuri_ledger` — é imutável por design

---

## REGRAS DE ENTREGA

1. Retorne cada arquivo refatorado como um **artefact separado** — nunca cole código no corpo do chat
2. Retorne o **arquivo completo** — sem omissões, sem `// ... resto igual`, sem truncamento
3. Se um arquivo for **removido** pela refatoração, indique: `🗑️ REMOVER: caminho/arquivo.go — motivo`
4. Se uma **migration nova** for necessária (ex: renomear coluna de projeção), crie como artefact separado com nome sequencial correto
5. Não altere arquivos que não fazem parte do escopo pedido

---

## VALIDAÇÃO ANTES DE ENTREGAR

Para cada arquivo refatorado, responda internamente:

- [ ] O arquivo compila? (imports corretos, sem símbolo undefined, sem redeclaração)
- [ ] Todos os `EventType` string continuam idênticos aos originais?
- [ ] `Apply()` ainda é chamado após cada `RaiseEvent()`?
- [ ] `AuditContext` está preenchido em todos os `SaveWithAudit`?
- [ ] Nenhuma lógica de negócio foi movida do aggregate para o handler (ou vice-versa)?
- [ ] O rebuild da projeção ainda funcionaria após esta mudança?
- [ ] Nenhum comportamento observável foi alterado — apenas a forma, não a função?

Se alguma resposta for **não**, corrija antes de entregar.

No chat, ao final, poste apenas:

```
✅ Refatoração concluída.
Arquivos modificados: X
Arquivos removidos: Y (listados acima)
Migrations novas: Z
Comportamento alterado: nenhum / [descrever se houver exceção justificada]
```

---

## INSTRUÇÕES POR TIPO DE ARQUIVO

---

### 🔷 AGGREGATE (`internal/domain/aggregates/`)

**Objetivo:** aggregate deve conter apenas lógica de domínio pura — sem acesso a banco, sem imports de `gin`, sem chamadas HTTP.

**Simplificações permitidas:**

**1. Funções de validação extraídas** Validações repetidas entre comandos (ex: verificar se estudante pertence à academia) devem virar funções privadas:

```go
// Antes: bloco repetido em RegistrarNota e AtualizarNota
if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
    return fmt.Errorf("estudante não pertence a esta academia")
}

// Depois: função privada
func (e *Estudante) validarPertenceAcademia(codigoAcademia string) error {
    if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
        return fmt.Errorf("estudante não pertence a esta academia")
    }
    return nil
}
```

**2. Apply handlers simplificados** O padrão marshal→unmarshal pode ser extraído para uma função genérica de desserialização, mas apenas se não comprometer a clareza. O padrão atual é correto e não deve ser forçado a mudar se já está claro.

**3. Remoção de comentários de scaffolding** Remover comentários como `// Etapa 4 deve preencher` se a etapa já foi implementada. Manter apenas comentários que explicam decisões de design não-óbvias.

**4. Constantes no lugar de strings literais repetidas**

```go
// Antes
if status != "inativo" && status != "em_andamento" && status != "finalizado" { ... }

// Depois
const (
    StatusInativo     = "inativo"
    StatusEmAndamento = "em_andamento"
    StatusFinalizado  = "finalizado"
)
var statusValidos = map[string]bool{StatusInativo: true, StatusEmAndamento: true, StatusFinalizado: true}
```

**5. Separação de arquivos por responsabilidade** Se um aggregate cresceu demais, manter a separação já existente (`estudante_notas.go`, `estudante_falta.go`, etc.) e considerar extrair novos arquivos para grupos coesos. Nunca fundir tudo num único arquivo.

**Regras invioláveis do aggregate:**

- Switch `Apply()` deve cobrir **todos** os eventos que o aggregate emite — sem evento órfão
- `RaiseEvent()` sempre antes de `Apply()` nos comandos
- `applyXxx()` sempre deserializa via marshal→unmarshal (não cast direto) para garantir que o rebuild funciona
- Erros de `json.Unmarshal` sempre retornados com `fmt.Errorf("applyXxx: unmarshal error: %w", err)`

---

### 🔶 HANDLER (`internal/handlers/`)

**Objetivo:** handler deve ser fino — extrair contexto, validar input, chamar aggregate, salvar, responder. Sem lógica de domínio embutida.

**Estrutura canônica de um handler de escrita:**

```go
func NomeHandler(c *gin.Context) {
    // 1. Extrair identidade
    userID, _ := middleware.GetUserID(c)

    // 2. Bind e validar input
    var req struct { ... }
    if err := c.ShouldBindJSON(&req); err != nil {
        utils.RespondWithValidationError(c, fmt.Errorf("..."))
        return
    }

    // 3. Carregar dados de suporte da projeção (se necessário)
    proj := getXProjection(c)
    dto, err := proj.GetByID(userID)
    if err != nil || dto == nil {
        utils.RespondWithNotFoundError(c, "entidade")
        return
    }

    // 4. Carregar aggregate
    repository := getRepository(c)
    agg, err := repository.Load(dto.ID, "TipoAggregate")
    if err != nil {
        utils.RespondWithInternalError(c, err)
        return
    }
    entidade := agg.(*aggregates.TipoAggregate)

    // 5. Executar comando de domínio
    if err := entidade.Comando(params...); err != nil {
        utils.RespondWithValidationError(c, err)
        return
    }

    // 6. Persistir com auditoria
    audit := db.AuditContext{
        UserID:   userID.String(),
        UserType: "tipo_usuario",
        IP:       c.ClientIP(),
    }
    if err := repository.SaveWithAudit(entidade, audit); err != nil {
        utils.RespondWithInternalError(c, err)
        return
    }

    // 7. Responder
    c.JSON(http.StatusOK, gin.H{"message": "operação realizada com sucesso"})
}
```

**Estrutura canônica de um handler de leitura:**

```go
func NomeQueryHandler(c *gin.Context) {
    userID, _ := middleware.GetUserID(c)
    proj := getXProjection(c)

    resultado, err := proj.GetByID(userID)
    if err != nil || resultado == nil {
        utils.RespondWithNotFoundError(c, "entidade")
        return
    }

    c.JSON(http.StatusOK, resultado)
}
```

**Simplificações permitidas:**

**1. Extração de helpers de validação de input** Blocos de validação de request body que aparecem em mais de um handler viram função:

```go
// Extrair para helpers.go ou arquivo de validação dedicado
func validarReqNota(req *ReqNota) error {
    if req.Nota < 0 || req.Nota > 20 {
        return fmt.Errorf("nota deve estar entre 0 e 20")
    }
    // ...
    return nil
}
```

**2. Extração de helpers de resposta** Padrões repetidos de construção de `gin.H{}` podem virar funções:

```go
func respondEstudante(c *gin.Context, e *projections.EstudanteDTO) {
    c.JSON(http.StatusOK, gin.H{
        "codigo_estudante": e.CodigoEstudante,
        "nome":             e.Nome,
        // campos padrão...
    })
}
```

**3. Handlers muito longos (>80 linhas)** Dividir em funções privadas nomeadas pela responsabilidade:

```go
func RegistrarAvaliacaoFinal(c *gin.Context) {
    academia, estudante, err := carregarEntidadesPrincipais(c)
    if err != nil { return }

    if err := validarNotasParaAvaliacao(c, estudante, academia); err != nil {
        utils.RespondWithValidationError(c, err)
        return
    }

    if err := executarAvaliacaoFinal(c, academia, estudante, req); err != nil { return }
    removerDeTurmas(c, estudante.CodigoEstudante, academia.CodigoAcademia)
    c.JSON(http.StatusOK, gin.H{"message": "avaliação registrada"})
}
```

**Regras invioláveis do handler:**

- `AuditContext` com `UserID`, `UserType` e `IP: c.ClientIP()` em todo `SaveWithAudit`
- Nunca retornar `senha_hash`, tokens ou dados sensíveis no body de resposta
- Validações de autorização (pertence à academia, role correto) sempre antes do comando
- Erros do `repository.Save/SaveWithAudit` sempre tratados com `utils.RespondWithInternalError`

---

### 🟣 BATCH HANDLERS (`internal/handlers/batch_handlers.go`)

**Objetivo:** cada função batch é um invólucro fino que divide o array de entrada em chunks, chama `newFakeContext` e delega ao handler individual correspondente.

**Regras específicas de batch:**

**1. `newFakeContext` propaga exclusivamente as chaves listadas em `batch_context.go`:**

```
user_id, user_type, admin_role, dbClient, repository, projManager, request_id
```

Nunca copiar valores sensíveis adicionais para o contexto falso.

**2. Nenhum batch handler contém lógica de negócio** — a validação ocorre dentro do handler individual chamado via `newFakeContext`.

**3. `validarTamanhoBatch(n, max)` chamado antes de qualquer processamento** — body vazio e arrays acima do limite retornam 400 imediatamente.

**4. Códigos HTTP de retorno são determinados por `batchHTTPStatus(results)`:**

- Todos ok → `200 OK`
- Parcialmente ok → `207 Multi-Status`
- Todos falharam → `422 Unprocessable Entity`

**5. Limites de lote por domínio:**

| Endpoint                                       | Limite | Razão                                                     |
| ---------------------------------------------- | ------ | --------------------------------------------------------- |
| `POST /dominis/academia/register/batch`        | **50** | bcrypt por academia ≈ 300ms → 50×300ms = 17,5s de timeout |
| `PUT /dominis/academia/ativar/batch`           | 50     | Sem bcrypt, apenas UPDATE de status                       |
| `PUT /dominis/academia/desativar/batch`        | 50     | Sem bcrypt                                                |
| `POST /academia/estudante/register/batch`      | 100    | bcrypt por estudante mas custo distribuído                |
| `POST /academia/notas-aluno/batch`             | 200    | Operação leve                                             |
| `PUT /academia/atualizar-nota/batch`           | 200    | Operação leve                                             |
| `DELETE /academia/nota/batch`                  | 200    | Operação leve                                             |
| `POST /academia/faltas-aluno/batch`            | 200    | Operação leve                                             |
| `PUT /academia/atualizar-falta/batch`          | 200    | Operação leve                                             |
| `DELETE /academia/falta/batch`                 | 200    | Operação leve                                             |
| `POST /academia/avaliacao-final/batch`         | 100    | Cada avaliação remove estudante de turmas                 |
| `PUT /academia/estudante/status-escolar/batch` | 100    | Operação moderada                                         |
| `POST /academia/curso/batch`                   | 50     | Operação moderada                                         |
| `PUT /academia/curso/ativar/batch`             | 50     | Operação moderada                                         |
| `PUT /academia/curso/desativar/batch`          | 50     | Operação moderada                                         |
| `DELETE /academia/curso/batch`                 | 50     | Cascata de matérias e turmas                              |
| `POST /academia/materia/batch`                 | 100    | Operação leve                                             |
| `PUT /academia/materia/ativar/batch`           | 100    | Operação leve                                             |
| `PUT /academia/materia/desativar/batch`        | 100    | Operação leve                                             |
| `DELETE /academia/materia/batch`               | 100    | Operação leve                                             |
| `POST /academia/turma/batch`                   | 50     | Operação moderada                                         |
| `PUT /academia/turma/ativar/batch`             | 50     | Operação moderada                                         |
| `PUT /academia/turma/desativar/batch`          | 50     | Operação moderada                                         |
| `DELETE /academia/turma/batch`                 | 50     | Operação moderada                                         |
| `POST /academia/turma/estudante/batch`         | 100    | Operação leve                                             |
| `DELETE /academia/turma/estudante/batch`       | 100    | Operação leve                                             |

> ⚠️ **IMPORTANTE:** O limite de `POST /dominis/academia/register/batch` é **5, não 50**. Cada academia exige `bcrypt.GenerateFromPassword` (custo 10 ≈ 300ms de CPU) + `generateCodigoAcademia` (consulta ao ledger para unicidade de código). Com 50 academias em série o request levava 17,5s+ → timeout no Railway → 422 em todos os itens. O limite de 5 foi determinado empiricamente: 5 × 350ms ≈ 1,75s, dentro do timeout de 120s. O limite no handler Go (`validarTamanhoBatch`) deve ser 5 para este endpoint.

**6. Todos os endpoints batch do grupo `/dominis` (admin) estão registados em `main.go`:**

```go
admin.POST("/academia/register/batch",         handlers.RegisterAcademiaBatch)
admin.PUT("/academia/ativar/batch",   middleware.RequireAdm(), handlers.AtivarAcademiaBatch)
admin.PUT("/academia/desativar/batch", middleware.RequireAdm(), handlers.DesativarAcademiaBatch)
```

**7. Formato de resposta de cada item:**

```json
{ "index": 0, "sucesso": true,  "dados": { ... } }
{ "index": 1, "sucesso": false, "erro":  "mensagem de erro" }
```

A extracção de `dados` pelo cliente deve lidar com a estrutura aninhada:

```
dados.data.codigo_academia  →  ou  →  dados.codigo_academia
```

(O handler `RegisterAcademia` retorna `"data": {"id": ..., "codigo_academia": ...}` dentro do response body.)

---

### 🟢 PROJECTION (`internal/projections/`)

**Objetivo:** projeção é um read model — recebe eventos do ledger e mantém uma visão de leitura otimizada.

**Estrutura canônica de um handler de evento na projeção:**

```go
func (p *XProjection) handleEventoXxx(event db.Event) error {
    var payload struct {
        Campo1 string    `json:"Campo1"`
        Campo2 time.Time `json:"Campo2"`
    }
    if err := json.Unmarshal(event.Payload, &payload); err != nil {
        return fmt.Errorf("handleEventoXxx: unmarshal: %w", err)
    }

    _, err := p.client.DB().Exec(`
        UPDATE projection_x
        SET campo1 = $1, updated_at = $2, version = version + 1
        WHERE id = $3
    `, payload.Campo1, payload.Campo2, event.AggregateID)
    return err
}
```

**Simplificações permitidas:**

**1. Extração de função de scan** Se o mesmo scan de `sql.Rows` aparece em múltiplos métodos de query, extrair para função privada:

```go
func scanXRow(row *sql.Row) (*XDTO, error) { ... }
func scanXRows(rows *sql.Rows) ([]*XDTO, error) { ... }
```

**2. Consolidar queries similares** Métodos `GetByID`, `GetByCodigo`, `GetByAcademia` que diferem apenas no WHERE podem compartilhar uma query base:

```go
func (p *XProjection) queryWhere(where string, args ...interface{}) ([]*XDTO, error) {
    rows, err := p.client.DB().Query(`SELECT `+xColumns+` FROM projection_x WHERE `+where, args...)
    // ...
}
```

**3. Remover log.Printf de debug em produção** `[DEBUG]` pode virar nível de log condicional ou ser removido. Manter apenas logs de erro e avisos relevantes.

**4. `Rebuild()` padronizado** O Rebuild segue sempre o mesmo padrão — se a projeção tiver Rebuild verbose, simplificar para o padrão canônico:

```go
func (p *XProjection) Rebuild() error {
    if _, err := p.client.DB().Exec(`TRUNCATE TABLE projection_x CASCADE`); err != nil {
        return fmt.Errorf("rebuild: truncate: %w", err)
    }
    return p.replayFromLedger("TipoAggregate")
}

func (p *XProjection) replayFromLedger(aggregateType string) error {
    rows, err := p.client.DB().Query(`
        SELECT `+ledgerColumns+`
        FROM spuri_ledger WHERE aggregate_type = $1 ORDER BY id ASC
    `, aggregateType)
    // ... scan e Handle padrão
}
```

**Regras invioláveis da projeção:**

- `Handle()` deve ter case para **todos** os eventos que o aggregate correspondente emite
- Eventos desconhecidos: retornar `nil` (ignorar silenciosamente é aceitável para eventos legados) — documentar o comportamento
- Erros de `json.Unmarshal` sempre retornados — nunca `log + continue`
- `Rebuild()` deve truncar a tabela antes de replay — sem estado residual
- `projection_checkpoints` atualizado junto com o processamento do evento

---

### 🟡 DB / REPOSITORY (`internal/db/`)

**Objetivo:** camada de persistência — gravação no ledger, reconstituição de aggregate, whitelist de eventos.

**Simplificações permitidas:**

**1. `safe_queries.go` — apenas manutenção de whitelist** Nenhuma lógica deve ser adicionada aqui. Se houver comentários desatualizados (ex: `FIX-WL-xx` já resolvidos), podem ser removidos ou consolidados num único comentário de contexto no topo.

**2. Queries longas com SQL multiline** SQL deve usar backtick com indentação clara:

```go
rows, err := db.Query(`
    SELECT id, event_id, aggregate_id, aggregate_type,
           event_type, event_version, payload, metadata,
           occurred_at, recorded_at, ledger_hash, previous_hash
    FROM spuri_ledger
    WHERE aggregate_id = $1
    ORDER BY event_version ASC
`, aggregateID)
```

**3. Funções de scan repetidas** Se o scan do ledger aparece em mais de um lugar (`Load`, `Rebuild`, `VerificarIntegridade`), extrair para `scanLedgerRow(rows)`.

**Regras invioláveis do db:**

- `Save` e `SaveWithAudit` gravam no ledger **antes** de qualquer efeito colateral
- `Load` reconstrói aggregate em ordem `ORDER BY event_version ASC`
- Erros de `Apply()` durante `Load` são propagados — nunca silenciados
- Todas as queries usam `$1`, `$2`... — nunca concatenação de string SQL
- A whitelist de `safe_queries.go` deve estar sincronizada com todos os eventos que os aggregates emitem

---

### 🔵 MIDDLEWARE (`internal/middleware/`)

**Objetivo:** extrair identidade do JWT, verificar permissões, bloquear acesso indevido.

**Simplificações permitidas:**

**1. Funções de extração de contexto consolidadas** Se `GetUserID`, `GetUserType`, `GetUserRole` fazem lógica repetida de extração do claim do JWT, consolidar num único `extractClaims(c) (Claims, error)` privado.

**2. Middlewares de role** Se `RequireAcademia`, `RequireAdmin`, `RequireEstudante` têm estrutura idêntica variando apenas o tipo, extrair para:

```go
func RequireUserType(userType string) gin.HandlerFunc {
    return func(c *gin.Context) {
        t, _ := GetUserType(c)
        if t != userType {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
            return
        }
        c.Next()
    }
}
```

**Regras invioláveis do middleware:**

- Nunca pular verificação de tipo de usuário em rota protegida
- `c.Abort()` sempre chamado antes de retornar erro — nunca apenas `c.JSON + return` sem Abort

---

## INSTRUÇÕES POR DOMÍNIO DE NEGÓCIO

Estas instruções se aplicam independentemente do tipo de arquivo sendo refatorado. Use quando o contexto do pedido envolver uma dessas entidades.

---

### Admin

- A hierarquia `fpp > adm > gerente` nunca deve ser simplificada para uma comparação de string direta — manter o mapa `adminHierarchy` com valores inteiros
- `ValidatePermission` deve continuar verificando nível estritamente superior (não igual)
- O bootstrap (`POST /bootstrap`) com advisory lock não deve ser simplificado — a proteção contra race condition é intencional

---

### Academia

- `CategoriasNota` no aggregate é necessário para detectar duplicatas sem depender da projeção — não remover
- `NivelEscolar` e `AnosAcademicos` têm regras cruzadas (`medio` não deve ter `anos_academicos`) — manter a validação no handler e no aggregate
- Academias iniciam `status = inativo` — nunca alterar o default
- `generateCodigoAcademia` consulta o **ledger** (não a projeção) para garantir unicidade de código mesmo em cadastros simultâneos — não alterar esta lógica

---

### Estudante

- Os três status independentes (`StatusEscolarFundamental`, `StatusEscolarMedio`, `StatusSuperior`) representam trajetórias paralelas — não consolidar num único campo
- `AprovacoesPorAno` (mapa de chave `tipoEnsino_anoLectivo_nivelAtual`) é o mecanismo de idempotência de aprovação — não remover
- A distinção entre `EstudanteCriado` (auto-cadastro) e `EstudanteCriadoComVinculo` (pela academia) é intencional — não unificar os eventos
- `BilheteIdentidade` e `BilheteIdentidadeResp` são mutuamente exclusivos de igualdade — manter a validação

---

### Notas

- Nota deve estar entre 0 e 20 — validação obrigatória no aggregate, não apenas no handler
- `AtualizarNota` exige `observacao` não vazia — regra de negócio intencional, não remover
- A separação entre `tipo = "escolar"` e `tipo = "superior"` define quais categorias são aceitas — manter a lógica de `validarCategoria`
- `RegistradoPor` e `AtualizadoPor` nos eventos são campos de auditoria — sempre preencher com o `userID` do contexto

---

### Faltas

- Falta pertence a uma `MateriaDisciplinarID` — sempre validar que a matéria existe e pertence à academia
- `Quantidade` deve ser positivo — validar no aggregate

---

### Aprovação de Ano

- A chave de idempotência `tipoEnsino + "_" + anoLectivo + "_" + nivelAtual` deve ser mantida exatamente neste formato para ser compatível com dados já gravados no ledger
- `ProximoNivel` é obrigatório quando `Aprovado = true`, exceto quando é o último ano do ciclo — a validação de "último ano" é feita pelo aggregate via lista de níveis

---

### Avaliação Final

- A validação de notas antes de registrar avaliação final (`validarNotasParaAprovacao`) é complexa por necessidade — o domínio exige verificar matérias × períodos × categoria. Pode ser refatorada em sub-funções, mas a lógica não pode ser removida
- A remoção automática do estudante de todas as turmas é efeito colateral obrigatório da avaliação final — nunca omitir
- `observacao` preenchida serve como override de validação ("forçar aprovação") — manter este comportamento

---

### Turma

- `Estudantes` é um array de `CodigoEstudante` (string) — não é FK para `projection_estudantes`
- `EstudanteAdicionadoNaTurma` é alias legado de `EstudanteAdicionadoATurma` — ao refatorar, manter ambos no switch do Apply e da projeção para compatibilidade com ledger histórico

---

### Curso e Matéria

- `Type` do Curso é imutável após criação — qualquer refatoração que permita mudar o type é um bug
- `AnosAcademicos` de matéria `medio/superior` aceita exatamente 1 item; `fundamental` aceita 1 a 9 — manter a função `ValidateAnosMateria` separada para cada caso
- `Periodos` é obrigatório apenas para cursos `superior` — não validar para `medio`

---

### SistemaConfig

- O UUID do SistemaConfig é determinístico (`uuid.NewSHA1(uuid.NameSpaceDNS, []byte("sistema_config.spuri.ao"))`) — nunca gerar aleatório
- É um singleton: sempre tenta carregar o aggregate existente antes de criar novo

---

### Operações em Lote (Batch)

- Ausência de atomicidade entre itens de um batch é **proposital** — documentar para o cliente
- Cada item é processado com `newFakeContext` que clona o request HTTP sem body e propaga apenas as chaves de contexto necessárias
- O limite de `POST /dominis/academia/register/batch` é **5** por causa do custo do bcrypt — nunca aumentar sem benchmarkar o tempo de processamento
- Clientes que consomem a API de batch devem extrair `dados.data.codigo_academia` OU `dados.codigo_academia` (a estrutura tem dois níveis possíveis dependendo do handler)

---

## CHECKLIST DE REVISÃO FINAL

Antes de entregar qualquer refatoração, percorra este checklist:

**Segurança e auditabilidade:**

- [ ] Nenhum dado sensível (senha_hash, token) exposto em resposta HTTP
- [ ] Todo `SaveWithAudit` tem `IP: c.ClientIP()` (não string vazia)
- [ ] Whitelist de eventos em `safe_queries.go` ainda cobre todos os eventos
- [ ] Nenhum `UPDATE`/`INSERT` direto em tabela de projeção (bypass do ledger)

**Corretude de Event Sourcing:**

- [ ] Todos os `EventType` string são idênticos aos originais
- [ ] `Apply()` chamado após `RaiseEvent()` em todos os comandos
- [ ] `applyXxx()` deserializa via unmarshal — nunca cast direto do payload
- [ ] Rebuild da projeção ainda reproduziria o estado atual corretamente

**Qualidade do código:**

- [ ] Arquivo compila sem erros (imports corretos, sem símbolo undefined)
- [ ] Nenhum `log.Printf("[DEBUG]` deixado em caminhos de produção críticos
- [ ] Nenhum código comentado deixado para trás (`// if err := ...`)
- [ ] Funções extraídas têm nomes que explicam o que fazem, não como fazem
- [ ] Early return usado onde reduz aninhamento sem perder clareza

**Compatibilidade:**

- [ ] Nenhuma interface pública alterada (assinaturas de funções exportadas)
- [ ] Nenhuma coluna de projeção removida sem migration correspondente
- [ ] Nenhum campo de evento removido (apenas adicionado como ponteiro nil-safe)

**Batch:**

- [ ] O limite de `RegisterAcademiaBatch` está em 5 (não 50) em `validarTamanhoBatch`
- [ ] Os três endpoints batch de admin (`register`, `ativar`, `desativar`) estão registados em `main.go`
- [ ] `extractResult` em `batch_context.go` extrai correctamente `dados` e `erro` de cada item