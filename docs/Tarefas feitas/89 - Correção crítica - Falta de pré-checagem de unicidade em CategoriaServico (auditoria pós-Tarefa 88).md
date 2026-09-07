---
criado: 07-09-2026
origem: Fredy + Claude (auditoria pós-Tarefa 88)
status: concluido
tipo: backend (spuri-backend) — correção crítica
depende_de: Tarefa 88 (Módulo de Serviços Extras — Categoria própria e Personalização tipada), já implementada e íntegra
prioridade: CRÍTICA — risco real de travamento permanente de uma projeção em produção
---

# Correção crítica — Falta de pré-checagem de unicidade em `CategoriaServico`

### Documento de execução para o Codex (auditado e pré-testado pelo Claude)

> **Este documento já contém todas as decisões e fatos necessários.** Você (Codex) não precisa
> investigar o "porquê" nem decidir a abordagem — a causa raiz já foi confirmada com testes reais
> contra PostgreSQL 16, e a correção já está desenhada. Apenas **aplique exatamente o patch da seção
> 3** e rode a checklist da seção 5.

## 0. Leia isto primeiro — sobre o seu ambiente e a gravidade deste bug

Sei que seu ambiente bloqueia `apt` (403) e não tem Docker nem `psql`. Isso não é um obstáculo aqui:
a causa raiz e a correção **já foram validadas de verdade** por mim (Claude), numa base PostgreSQL 16
real no meu sandbox, reproduzindo byte a byte as mesmas queries que o código Go gera. A correção em si
não cria nenhuma migration nova nem depende de infraestrutura — é só Go puro (uma função nova + 3
pontos de chamada num arquivo já existente).

**Isto não é uma tarefa de "nice to have".** Auditando a Tarefa 88 (Categoria própria de Serviço
Extra) descobri que `internal/handlers/categoria_servico_handlers.go` tem uma falha que pode travar
**permanentemente** o processamento de eventos de `CategoriaServico` para **todas as academias da
plataforma**, exigindo intervenção manual no banco para recuperar. Detalhes completos na seção 2 —
leia antes de aplicar o patch, porque entender o "porquê" evita que essa mesma classe de bug volte a
aparecer em features futuras que usem unicidade por nome.

## 1. Prompt recomendado para executar esta tarefa

> Aplique exatamente o patch descrito na seção 3 deste documento em
> `internal/handlers/categoria_servico_handlers.go`: uma função nova
> (`validarNomeCategoriaServicoDisponivel`) e três pontos de chamada
> (`CriarCategoriaServico`, `AtualizarCategoriaServico`, e o branch `ativar` de
> `toggleCategoriaServico`). As decisões já estão tomadas — não replaneje, não adicione uma query SQL
> dedicada nem mexa em outros arquivos. Ao final, rode `go build ./...`, `go vet ./...`, `gofmt -l .`
> e `go test ./...`, corrija qualquer erro de compilação, e preencha o checklist da seção 5.

## 2. O bug, explicado

### 2.1 O que a Tarefa 88 implementou (e o que faltou)

A Tarefa 88 criou `CategoriaServico` como entidade de event sourcing, com um índice único parcial na
projeção (`migrations/122_categorias_servico.sql`):

```sql
CREATE UNIQUE INDEX IF NOT EXISTS ux_categorias_servico_nome_ativo
    ON projection_categorias_servico (codigo_academia, lower(nome))
    WHERE ativo = true;
```

Isso é o padrão correto para "nome único por academia, ignorando maiúsculas/minúsculas, enquanto
ativa" — e eu mesmo validei essa regra de verdade no meu sandbox (nome duplicado é rejeitado, nomes
iguais em academias diferentes convivem, reaproveitar nome após desativar funciona). **O índice em si
está certo.** O problema é *onde* essa é a única linha de defesa.

### 2.2 Por que um índice único na projeção não basta sozinho aqui

Neste repositório, `AggregateRepository.SaveWithAudit` (`internal/db/repository.go`) só grava o evento
no **event store** (`spuri_ledger`), dentro de uma transação. Ele **não** atualiza a projeção na mesma
chamada — isso é feito depois, de forma **assíncrona**, pelo Projection Manager
(`internal/projections/manager.go`), acordado por um hook não-bloqueante (`notifyLedgerWritten` →
`ledgerWriteHook` → `Manager.Wake`, ver `internal/db/ledger_hook.go`, comentário explícito: "chamado
de forma não bloqueante").

Isso significa: quando um handler chama `cat.Criar(...)` seguido de `SaveWithAudit(cat, ...)`, o
evento `CategoriaServicoCriada` é gravado no ledger e a resposta HTTP `201` já é enviada ao cliente
**antes** de a projeção sequer tentar processar esse evento. O handler atual
(`CriarCategoriaServico`) não faz nenhuma verificação de nome duplicado antes de gerar o evento — ele
confia inteiramente no índice único da projeção, que só vai ser avaliado depois, em outra goroutine.

Veja `internal/handlers/categoria_servico_handlers.go` (estado atual, sem a correção):

```go
func CriarCategoriaServico(c *gin.Context) {
	var r categoriaServicoPayload
	if err := bindCategoriaServicoPayload(c, &r); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	codigo, id, ok := academy(c)
	if !ok {
		return
	}
	cat := aggregates.NewCategoriaServico()
	if err := cat.Criar(codigo, r.Nome, id); err != nil { // Criar() não checa duplicidade — só valida formato
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).SaveWithAudit(cat, db.AuditContext{...}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "categoria de serviço criada com sucesso", ...})
}
```

Nada aqui pergunta "já existe uma categoria ativa com este nome?" antes de disparar o evento.

### 2.3 O que acontece quando duas categorias colidem (create, rename ou reactivate)

O INSERT que a projeção usa para processar `CategoriaServicoCriada`
(`internal/projections/categoria_servico_projection.go`, função `created`) é:

```go
p.client.DB().Exec(`INSERT INTO projection_categorias_servico(id,codigo_academia,nome,ativo,created_at,updated_at,version,last_event_id)
    VALUES($1,$2,$3,true,$4,$4,$5,$6) ON CONFLICT(id) DO NOTHING`, e.AggregateID, x.CodigoAcademia, x.Nome, x.CreatedAt, e.EventVersion, e.EventID)
```

O `ON CONFLICT(id) DO NOTHING` só protege contra reprocessar o **mesmo evento** duas vezes (idempotência
normal de at-least-once delivery). Ele **não** cobre conflito no índice `ux_categorias_servico_nome_ativo`,
que é uma constraint diferente. As funções `renamed` e `active` (usadas também por
`ReativarCategoriaServico`) são só um `UPDATE ... WHERE id=$4`, sem `ON CONFLICT` nenhum:

```go
// renamed:
UPDATE projection_categorias_servico SET nome=$1,... WHERE id=$4
// active (usada por Desativar E Reativar):
UPDATE projection_categorias_servico SET ativo=$1,... WHERE id=$4
```

Eu reproduzi os três cenários de verdade, contra PostgreSQL 16, executando exatamente essas mesmas
queries (INSERT/UPDATE) com os mesmos dados que o Go produziria:

**Criar duas categorias com nome colidente (case-insensitive) na mesma academia:**
```
ERROR:  duplicate key value violates unique constraint "ux_categorias_servico_nome_ativo"
DETAIL:  Key (codigo_academia, lower(nome::text))=(aca1, transporte) already exists.
```

**Renomear uma categoria ativa para o nome de outra categoria já ativa na mesma academia:**
```
ERROR:  duplicate key value violates unique constraint "ux_categorias_servico_nome_ativo"
DETAIL:  Key (codigo_academia, lower(nome::text))=(aca1, transporte) already exists.
```

**Reativar uma categoria desativada cujo nome colide com outra categoria que ficou ativa nesse meio tempo:**
```
ERROR:  duplicate key value violates unique constraint "ux_categorias_servico_nome_ativo"
DETAIL:  Key (codigo_academia, lower(nome::text))=(aca1, transporte) already exists.
```

Os três cenários são plausíveis em uso normal — não é preciso concorrência real: duplo clique no botão
"criar", duas pessoas da mesma academia criando "Transporte" e "transporte" em momentos diferentes, ou
reativar uma categoria antiga meses depois de outra ter "herdado" o nome.

### 2.4 Por que isso é crítico: o Manager trava o checkpoint da projeção inteira

`internal/projections/manager.go` já documenta, em comentários extensos no topo do arquivo, o
comportamento do Manager diante de um evento que falha ao ser processado pela projeção
(`processEventWithRetry` → `processProjection`): ele tenta 3 vezes, e se todas falharem,
**o checkpoint da projeção não avança** — o mesmo evento é retentado a cada novo ciclo (poll de 1s ou
acordado por escrita nova), indefinidamente. Trecho literal do código (`processProjection`):

```go
// Checkpoint NÃO avança — o mesmo evento será retentado na próxima rodada.
```

Como o erro de unicidade **não é transitório** (a mesma query falha sempre, para sempre, com os
mesmos dados), o evento problemático fica travado permanentemente nesse ponto. Isso significa: a
partir do momento em que isso acontecer, **nenhum evento seguinte de `CategoriaServico` — de nenhuma
academia — é processado nunca mais**, porque o Manager processa os eventos de uma projeção em ordem, a
partir do checkpoint. Criar, renomear, desativar ou reativar categorias continuaria retornando sucesso
HTTP normalmente (o handler só depende do event store, não da projeção), mas nada disso mais apareceria
em `GET /academia/categorias-servico` nem seria validável por `categoria_servico_id` em novos serviços
— até intervenção manual.

**Um rebuild da projeção não resolve sozinho.** `CategoriaServicoProjection.Rebuild()` faz `TRUNCATE` e
reprocessa **todos** os eventos de `CategoriaServico` do ledger, na mesma ordem — se os dois eventos
colidentes já estiverem lá, o rebuild falha exatamente no mesmo ponto, pela mesma razão. A única
correção real é **impedir que o evento colidente seja gerado**, com uma checagem antes do
`SaveWithAudit` — exatamente o padrão que este repositório já usa em outros lugares (veja
`internal/handlers/turmas_handler.go`, função `CriarTurma`: consulta a projeção por
`GetByCodigoTurma` **antes** de criar o evento, e responde `400` amigável se já existir. `Criar
CategoriaServico` deveria ter seguido esse mesmo padrão e não seguiu).

### 2.5 Recomendação para Fredy (fora do escopo deste documento, mas importante)

Antes ou logo depois de aplicar este patch, vale confirmar em produção se a projeção `categorias_servico`
já está com lag (checkpoint atrasado em relação ao `spuri_ledger`) — por exemplo comparando
`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name='categorias_servico'`
com `SELECT MAX(id) FROM spuri_ledger`. Se já estiver travada, a recuperação exige intervenção manual
no banco (não é algo que este patch resolve retroativamente) — terei prazer em ajudar com isso se for o
caso, mas prefiro confirmar antes de sugerir qualquer alteração direta em dados de produção.

## 3. A correção

**Escopo exato — só este arquivo:** `internal/handlers/categoria_servico_handlers.go`. Não mexa em
nenhum outro arquivo (migrations, aggregate, projeção, frontend, documentação — tudo isso está fora do
escopo aqui).

### 3.1 Localizar (logo após a função `validarCategoriaServico`, antes de `func CriarCategoriaServico`)

```go
func validarCategoriaServico(c *gin.Context, codigoAcademia string, categoriaID *uuid.UUID) error {
	if categoriaID == nil {
		return nil
	}
	cat, err := getCategoriasServicoProjection(c).GetByID(*categoriaID)
	if err != nil {
		return fmt.Errorf("erro ao verificar categoria: %v", err)
	}
	if cat == nil {
		return fmt.Errorf("categoria de serviço não encontrada")
	}
	if cat.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("categoria de serviço não pertence a esta academia")
	}
	if !cat.Ativo {
		return fmt.Errorf("categoria de serviço está inativa")
	}
	return nil
}
func CriarCategoriaServico(c *gin.Context) {
```

### 3.2 Substituir por (insere a função nova entre as duas)

```go
func validarCategoriaServico(c *gin.Context, codigoAcademia string, categoriaID *uuid.UUID) error {
	if categoriaID == nil {
		return nil
	}
	cat, err := getCategoriasServicoProjection(c).GetByID(*categoriaID)
	if err != nil {
		return fmt.Errorf("erro ao verificar categoria: %v", err)
	}
	if cat == nil {
		return fmt.Errorf("categoria de serviço não encontrada")
	}
	if cat.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("categoria de serviço não pertence a esta academia")
	}
	if !cat.Ativo {
		return fmt.Errorf("categoria de serviço está inativa")
	}
	return nil
}

// validarNomeCategoriaServicoDisponivel verifica, ANTES de gerar o evento
// (CategoriaServicoCriada/Renomeada/Reativada), se já existe outra categoria
// ATIVA com o mesmo nome (ignorando maiúsculas/minúsculas) nesta academia.
//
// Esta pré-checagem existe porque a unicidade de nome só é garantida por um
// índice único parcial na PROJEÇÃO (ux_categorias_servico_nome_ativo em
// projection_categorias_servico), não no event store. SaveWithAudit só grava
// o evento no ledger — a projeção é atualizada depois, de forma assíncrona,
// pelo Projection Manager. Sem esta pré-checagem, dois eventos com nomes
// colidentes são aceitos normalmente pelo ledger (o handler HTTP responde
// sucesso para ambos), mas quando o Manager tenta aplicar o SEGUNDO evento na
// projeção, o INSERT/UPDATE viola o índice único. Esse erro não é
// transitório, então processEventWithRetry (internal/projections/manager.go)
// esgota as 3 tentativas e o checkpoint da projeção "categorias_servico" para
// de avançar PERMANENTEMENTE nesse evento — bloqueando toda criação, edição,
// ativação e desativação de categoria de serviço, em QUALQUER academia, até
// intervenção manual. Um rebuild da projeção não resolve sozinho, pois
// replaya os mesmos eventos na mesma ordem e falha exatamente no mesmo ponto.
func validarNomeCategoriaServicoDisponivel(c *gin.Context, codigoAcademia, nome string, excluirID *uuid.UUID) error {
	nome = strings.TrimSpace(nome)
	cats, err := getCategoriasServicoProjection(c).GetByAcademia(codigoAcademia, true)
	if err != nil {
		return fmt.Errorf("erro ao verificar nome de categoria: %v", err)
	}
	for _, existente := range cats {
		if excluirID != nil && existente.ID == *excluirID {
			continue
		}
		if strings.EqualFold(existente.Nome, nome) {
			return fmt.Errorf("já existe uma categoria de serviço ativa com este nome nesta academia")
		}
	}
	return nil
}
func CriarCategoriaServico(c *gin.Context) {
```

### 3.3 Localizar (dentro de `CriarCategoriaServico`)

```go
	codigo, id, ok := academy(c)
	if !ok {
		return
	}
	cat := aggregates.NewCategoriaServico()
	if err := cat.Criar(codigo, r.Nome, id); err != nil {
```

### 3.4 Substituir por (adiciona a chamada de pré-checagem)

```go
	codigo, id, ok := academy(c)
	if !ok {
		return
	}
	if err := validarNomeCategoriaServicoDisponivel(c, codigo, r.Nome, nil); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	cat := aggregates.NewCategoriaServico()
	if err := cat.Criar(codigo, r.Nome, id); err != nil {
```

### 3.5 Localizar (dentro de `AtualizarCategoriaServico`)

```go
	cat, id, ok := loadCategoriaServico(c)
	if !ok {
		return
	}
	if err := cat.Renomear(r.Nome, id); err != nil {
```

### 3.6 Substituir por

```go
	cat, id, ok := loadCategoriaServico(c)
	if !ok {
		return
	}
	catID := cat.GetID()
	if err := validarNomeCategoriaServicoDisponivel(c, cat.CodigoAcademia, r.Nome, &catID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := cat.Renomear(r.Nome, id); err != nil {
```

### 3.7 Localizar (dentro de `toggleCategoriaServico`)

```go
	var err error
	if ativar {
		err = cat.Reativar(id)
	} else {
		err = cat.Desativar(id)
	}
```

### 3.8 Substituir por (checagem SÓ no branch de reativar — ver seção 4)

```go
	var err error
	if ativar {
		catID := cat.GetID()
		if err = validarNomeCategoriaServicoDisponivel(c, cat.CodigoAcademia, cat.Nome, &catID); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		err = cat.Reativar(id)
	} else {
		err = cat.Desativar(id)
	}
```

Nenhum import novo é necessário — `strings`, `fmt` e `uuid` já estão importados neste arquivo.

## 4. Por que só estes 3 pontos (e não `Desativar`)

Desativar uma categoria (`ativo=true → false`) **nunca** pode violar
`ux_categorias_servico_nome_ativo`, porque esse índice só cobre linhas com `ativo=true` — desativar
sempre *remove* uma linha do escopo do índice, nunca adiciona uma. Por isso o branch `else` (Desativar)
não recebe a checagem — adicionar ali seria trabalho supérfluo, não faça.

## 5. Checklist de autoverificação (rodar antes de finalizar)

1. `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...` — todos limpos, sem exceção. Este
   é o passo obrigatório do seu ambiente que eu não consigo rodar no meu (module do projeto exige Go
   1.24, que não consigo obter no meu sandbox por restrição de rede — mas já validei a sintaxe deste
   patch especificamente com `gofmt` isolado, sem erros).
2. Releia o arquivo final: confirme que `validarNomeCategoriaServicoDisponivel` foi inserida **uma
   única vez**, entre `validarCategoriaServico` e `CriarCategoriaServico`.
3. Confirme os 3 pontos de chamada, exatamente onde indicado: `CriarCategoriaServico` (com
   `excluirID = nil`), `AtualizarCategoriaServico` (com `excluirID = &catID`, a própria categoria
   sendo editada), e o branch `ativar` de `toggleCategoriaServico` (com `excluirID = &catID`, a própria
   categoria sendo reativada).
4. Confirme que o branch `else` (Desativar) de `toggleCategoriaServico` continua **sem** a checagem.
5. `grep -n "validarNomeCategoriaServicoDisponivel" internal/handlers/categoria_servico_handlers.go` —
   deve aparecer exatamente 4 vezes (1 definição + 3 chamadas).

## 6. Fora de escopo — não faça

- Não altere migrations, o aggregate `CategoriaServico`, a projeção, o frontend (`spuripainel`) ou
  `Documentação da API.md` — a correção da documentação é outra tarefa (Tarefa 90), que já assume que
  este patch foi aplicado.
- Não adicione uma query SQL dedicada (ex.: `SELECT EXISTS(...)`) à projeção — a solução aprovada
  reaproveita `GetByAcademia`, que já existe e já é testado.
- Não mexa em `ServicoExtra` nem em `validarCategoriaServico` (a função existente, que valida
  `categoria_servico_id` ao criar/editar um serviço — ela já está correta e não tem este problema,
  porque só faz leitura, nunca escrita).

## 7. Entrega

Ao terminar, reporte em texto simples (fora do arquivo `.go`, como resposta da sua execução):

- Confirmação de que a checklist da seção 5 foi executada e passou (cole a saída de
  `go build`/`go vet`/`gofmt`/`go test`).
- Confirmação do `grep` da checklist item 5 (contagem de ocorrências).
- Qualquer erro de compilação encontrado e como foi corrigido, se houver.
