---
criado: 06-09-2026
origem: Fredy + Claude (orquestração)
status: concluída (integração PostgreSQL pendente no ambiente)
tipo: backend (spuri-backend)
depende_de: Tarefas 83-86 (Módulo de Serviços Extras) já implementadas
---

# Módulo de Serviços Extras — Categoria própria + Personalização tipada (Backend)

### Documento de execução para o Codex (orquestrado e pré-testado pelo Claude)

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Mesma limitação de rede já documentada nas tarefas anteriores deste módulo — não consigo rodar `go build`/`go test` completos no meu sandbox. Mas as duas partes mais arriscadas desta tarefa **foram testadas de verdade**, isoladamente:

- **A migration 122** (seção 2) foi aplicada com sucesso sobre as 121 migrations reais e atuais do repositório, numa base PostgreSQL 16 limpa. Testei также as regras de negócio da tabela nova: nome de categoria duplicado (case-insensitive) na mesma academia é rejeitado; o mesmo nome em academias diferentes é aceito; reaproveitar um nome depois de desativar a categoria antiga funciona.
- **Toda a lógica de validação de `detalhes_personalizados` tipado** (seção 4) foi extraída para um programa Go isolado (só bibioteca padrão, compila sem depender da rede bloqueada) e testada com 15 casos — os 4 exemplos válidos do pedido original (incluindo os dois exemplos de "Transporte" e "Natação" exatamente como especificados) e 11 casos inválidos (tipo inexistente, valor não bate com o tipo declarado para cada um dos 6 tipos, rótulo vazio, chave em formato errado). Todos os 15 se comportaram exatamente como esperado.

O que não pôde ser testado aqui: compilação do pacote inteiro depois de eu editar `servico_extra.go` (arquivo grande, já existente, com edições cirúrgicas — releia com atenção antes de considerar concluído) e os handlers relacionados. Rode `go build ./...`, `go vet ./...` e `gofmt -l .` no seu ambiente antes de finalizar.

## 1. Prompt recomendado para executar esta tarefa

> Implemente exatamente as três mudanças descritas neste documento (categoria como entidade própria, `categoria_servico_id` no lugar de `categoria`, `detalhes_personalizados` tipado), na ordem das seções. As decisões de design já estão tomadas — não replaneje. Preste atenção especial à seção 3 (edições em `servico_extra.go`, um arquivo grande e já existente) e à seção 6 (não esqueça a whitelist do ledger — é a causa mais comum de erro 500 silencioso neste repositório). Ao final, rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...`, corrija qualquer erro, e preencha o checklist da seção 9.

## 2. Contexto e decisões de design

O dono do produto pediu duas mudanças na personalização de `ServicoExtra`:

1. **Categoria vira uma entidade com CRUD próprio**, e `ServicoExtra.categoria` (texto livre) é substituído por `ServicoExtra.categoria_servico_id` (referência a essa nova entidade).
2. **`detalhes_personalizados` deixa de ser um JSON livre sem forma** e passa a ter uma estrutura previsível: cada chave mapeia para um objeto com `rotulo` (rótulo de exibição), `valor` e `tipo` — sendo `tipo` um valor de um conjunto fechado definido pelo backend, para permitir validação real e um formulário guiado no frontend.

Decisões já tomadas:

1. **`CategoriaServico` é uma entidade própria da academia**, seguindo exatamente o mesmo padrão de event sourcing de `Curso`/`ServicoExtra`: aggregate com `Criar`/`Renomear`/`Desativar`/`Reativar`, sem hard delete (uma categoria desativada não pode mais ser escolhida em novos serviços, mas serviços que já a usam continuam funcionando normalmente — mesma filosofia de preservar histórico já usada em todo o resto do sistema).
2. **Nome de categoria é único por academia, ignorando maiúsculas/minúsculas, enquanto ativa** — evita "Transporte" e "transporte" coexistindo como categorias diferentes por descuido. Testado (seção 0).
3. **Sem FK física no banco entre `projection_servicos_extras.categoria_servico_id` e `projection_categorias_servico.id`.** Este é um schema derivado de projeções de event sourcing, não um schema relacional tradicional — o mesmo já vale para `cursos_disponiveis`↔`Curso` desde a migration 121. A posse/existência/status da categoria é validada no HANDLER (mirror exato de `validarPosseCursosDisponiveis`, seção 3.3), não por constraint de banco.
4. **`categoria_servico_id` é opcional** (um serviço pode não ter categoria) e, quando informado, precisa pertencer à mesma academia e estar `ativo=true` no momento da criação/edição do serviço (mas continua valendo depois que a categoria for desativada — mesma regra de "congelar no momento do uso" já usada em outras partes do sistema).
5. **A coluna antiga `categoria` (texto) é descartada, não migrada.** Não havia nenhuma tela expondo um seletor estruturado de categoria até agora (era um campo de texto livre no formulário) — o risco de perda de dados relevantes é baixo. Se isto for uma preocupação real em algum ambiente, pare antes da migration 122 e exporte os valores atuais de `categoria` primeiro; a migration como está **não** preserva esses valores.
6. **`detalhes_personalizados` tem um conjunto fechado de 6 tipos**: `texto`, `numero`, `booleano`, `data`, `hora`, `lista_texto`. O pedido original citava string/boolean/Date/lista de strings — adicionei `numero` (praticamente inevitável para atributos como capacidade, distância, etc.) e separei `hora` de `data`, porque o próprio exemplo do pedido ("06:30" para horário de saída) é uma hora do dia, não uma data de calendário — tratá-los como o mesmo tipo obrigaria o frontend a usar um seletor de data inteiro para capturar só um horário. Fechado deliberadamente em 6 tipos: o objetivo é um formulário previsível no frontend (6 widgets possíveis), não um JSON Schema genérico.
7. **Chave de cada detalhe personalizado é validada por formato** (minúsculas, números e `_`, começando por letra, até 50 caracteres — mesmo estilo `snake_case` usado no resto do sistema), e o **valor é validado contra o tipo declarado** (ex.: `tipo=numero` exige que `valor` seja numérico; `tipo=data` exige uma string no formato `AAAA-MM-DD`; `tipo=hora` exige `HH:MM`; `tipo=lista_texto` exige um array onde todo elemento é string). Limite de 30 entradas por serviço, para não virar um vetor de payloads arbitrariamente grandes.
8. **Nenhuma migração automática dos dados que já existem em `detalhes_personalizados` livre** — pelo mesmo motivo do item 5 (a feature nunca teve UI, uso real é baixo/nulo). Se um serviço já tiver algo salvo no formato antigo, o `Atualizar` vai rejeitá-lo na primeira tentativa de alterar esse campo, com uma mensagem de validação clara — não um crash.

## 3. Nova entidade `CategoriaServico`

### 3.1 Migration — **já testada** (seção 0)

Crie `migrations/122_categorias_servico.sql`:

```sql
BEGIN;

CREATE TABLE IF NOT EXISTS projection_categorias_servico (
    id UUID PRIMARY KEY,
    codigo_academia VARCHAR(50) NOT NULL,
    nome VARCHAR(100) NOT NULL,
    ativo BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID
);
CREATE INDEX IF NOT EXISTS idx_categorias_servico_academia ON projection_categorias_servico(codigo_academia, ativo);
CREATE UNIQUE INDEX IF NOT EXISTS ux_categorias_servico_nome_ativo
    ON projection_categorias_servico (codigo_academia, lower(nome))
    WHERE ativo = true;

ALTER TABLE projection_servicos_extras ADD COLUMN categoria_servico_id UUID NULL;
ALTER TABLE projection_servicos_extras DROP COLUMN categoria;

COMMIT;
```

### 3.2 Aggregate — `internal/domain/aggregates/categoria_servico.go` (arquivo novo)

```go
package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CategoriaServico é uma etiqueta organizacional que a academia define para
// agrupar os seus ServicoExtra (ex.: "Transporte", "Dança", "Reforço
// Escolar"). Existe como entidade própria — não texto livre — para garantir
// um catálogo consistente: sem isto, duas grafias do "mesmo" rótulo
// ("Transporte"/"transporte") virariam categorias distintas.
type CategoriaServico struct {
	BaseAggregate

	CodigoAcademia string
	Nome           string
	Ativo          bool
	CriadoPor      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewCategoriaServico() *CategoriaServico {
	return &CategoriaServico{
		BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}},
		Ativo:         true,
	}
}

func (c *CategoriaServico) GetType() string { return "CategoriaServico" }

type CategoriaServicoCriadaEvent struct {
	BaseEvent
	CodigoAcademia string
	Nome           string
	CriadoPor      uuid.UUID
	CreatedAt      time.Time
}

func (e *CategoriaServicoCriadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaServicoCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type CategoriaServicoRenomeadaEvent struct {
	BaseEvent
	Nome          string
	AtualizadoPor uuid.UUID
	UpdatedAt     time.Time
}

func (e *CategoriaServicoRenomeadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaServicoRenomeadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type CategoriaServicoDesativadaEvent struct {
	BaseEvent
	DesativadoPor uuid.UUID
	UpdatedAt     time.Time
}

func (e *CategoriaServicoDesativadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaServicoDesativadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type CategoriaServicoReativadaEvent struct {
	BaseEvent
	ReativadoPor uuid.UUID
	UpdatedAt    time.Time
}

func (e *CategoriaServicoReativadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaServicoReativadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

func (c *CategoriaServico) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "CategoriaServicoCriada":
		return c.applyCriada(event)
	case "CategoriaServicoRenomeada":
		return c.applyRenomeada(event)
	case "CategoriaServicoDesativada":
		c.Ativo = false
		return nil
	case "CategoriaServicoReativada":
		c.Ativo = true
		return nil
	default:
		return fmt.Errorf("tipo de evento desconhecido para CategoriaServico: %s", event.GetEventType())
	}
}

func (c *CategoriaServico) Criar(codigoAcademia, nome string, criadoPor uuid.UUID) error {
	if strings.TrimSpace(codigoAcademia) == "" {
		return fmt.Errorf("codigo_academia é obrigatório")
	}
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if len(nome) > 100 {
		return fmt.Errorf("nome deve ter no máximo 100 caracteres")
	}
	event := &CategoriaServicoCriadaEvent{
		BaseEvent:      BaseEvent{EventType: "CategoriaServicoCriada", AggregateID: c.ID},
		CodigoAcademia: codigoAcademia,
		Nome:           nome,
		CriadoPor:      criadoPor,
		CreatedAt:      time.Now(),
	}
	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *CategoriaServico) Renomear(nome string, atualizadoPor uuid.UUID) error {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if len(nome) > 100 {
		return fmt.Errorf("nome deve ter no máximo 100 caracteres")
	}
	event := &CategoriaServicoRenomeadaEvent{
		BaseEvent:     BaseEvent{EventType: "CategoriaServicoRenomeada", AggregateID: c.ID},
		Nome:          nome,
		AtualizadoPor: atualizadoPor,
		UpdatedAt:     time.Now(),
	}
	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *CategoriaServico) Desativar(desativadoPor uuid.UUID) error {
	if !c.Ativo {
		return fmt.Errorf("categoria já está inativa")
	}
	event := &CategoriaServicoDesativadaEvent{
		BaseEvent:     BaseEvent{EventType: "CategoriaServicoDesativada", AggregateID: c.ID},
		DesativadoPor: desativadoPor,
		UpdatedAt:     time.Now(),
	}
	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *CategoriaServico) Reativar(reativadoPor uuid.UUID) error {
	if c.Ativo {
		return fmt.Errorf("categoria já está ativa")
	}
	event := &CategoriaServicoReativadaEvent{
		BaseEvent:    BaseEvent{EventType: "CategoriaServicoReativada", AggregateID: c.ID},
		ReativadoPor: reativadoPor,
		UpdatedAt:    time.Now(),
	}
	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *CategoriaServico) applyCriada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p CategoriaServicoCriadaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	c.CodigoAcademia = p.CodigoAcademia
	c.Nome = p.Nome
	c.Ativo = true
	c.CriadoPor = p.CriadoPor
	c.CreatedAt = p.CreatedAt
	c.UpdatedAt = p.CreatedAt
	return nil
}

func (c *CategoriaServico) applyRenomeada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p CategoriaServicoRenomeadaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	c.Nome = p.Nome
	c.UpdatedAt = p.UpdatedAt
	return nil
}
```

### 3.3 Handlers — `internal/handlers/categoria_servico_handlers.go` (arquivo novo)

Mirror exato do padrão de `servico_extra_handlers.go` (mais simples, sem campos financeiros). Implemente:

- `CriarCategoriaServico` — `POST /academia/categorias-servico`, payload `{"nome": "..."}`. Use o mesmo padrão de binding com tags `json` explícitas e `DisallowUnknownFields` — **não repita o bug já corrigido em `servicoExtraPayload`** (falta de tags `json` fazendo campos de mais de uma palavra ficarem sempre vazios); aqui só há um campo (`nome`), risco baixo, mas inclua a tag mesmo assim por consistência: `Nome string \`json:"nome"\``.
- `AtualizarCategoriaServico` — `PUT /academia/categorias-servico/:id`, mesmo payload, chama `.Renomear(...)`.
- `DesativarCategoriaServico` / `ReativarCategoriaServico` — `PUT /academia/categorias-servico/:id/desativar` e `.../reativar`.
- `ListarCategoriasServico` — `GET /academia/categorias-servico` (grupo `academiaRead`), lê da projeção, filtra por `codigo_academia` do ator autenticado.
- Todas as mutações verificam posse (`categoria.CodigoAcademia == codigo`) antes de agir, e todas respondem a partir do aggregate em memória (`gin.H{"data": gin.H{"id": ..., "nome": ..., "ativo": ..., ...}}` — snake_case explícito, **não** `gin.H{"data": c}` com o aggregate cru; mesmo bug de serialização já corrigido em `servico_extra_handlers.go` — não reintroduza).

Adicione também o helper de validação usado pelo `ServicoExtra` (seção 3.4):

```go
// validarCategoriaServico confirma que a categoria existe, pertence à
// academia e está ativa. Mirror de validarPosseCursosDisponiveis.
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
```

### 3.4 Projeção — `internal/projections/categoria_servico_projection.go` (arquivo novo)

Mirror exato de `internal/projections/servico_extra_projection.go` (mais simples — sem colunas array/JSONB). `Name() string { return "categorias_servico" }`, `GetByID`, `GetByAcademia(codigo string, ativosOnly bool)`, `Handle` para os 4 eventos, `Rebuild`.

### 3.5 Registro (whitelist, factory, projeções, rotas)

Em `internal/db/safe_queries.go`:
```go
// validAggregateTypes
"CategoriaServico": true,
// validEventTypes
"CategoriaServicoCriada":     true,
"CategoriaServicoRenomeada":  true,
"CategoriaServicoDesativada": true,
"CategoriaServicoReativada":  true,
```

Em `internal/domain/aggregates/aggregate.go`, `DefaultAggregateFactory.Create`:
```go
case "CategoriaServico":
    return NewCategoriaServico(), nil
```

Em `cmd/server/main.go`:
```go
projManager.RegisterProjection("categorias_servico", projections.NewCategoriaServicoProjection(dbClient))
// grupo academiaRead:
academiaRead.GET("/categorias-servico", handlers.ListarCategoriasServico)
// grupo academia:
academia.POST("/categorias-servico", handlers.CriarCategoriaServico)
academia.PUT("/categorias-servico/:id", handlers.AtualizarCategoriaServico)
academia.PUT("/categorias-servico/:id/desativar", handlers.DesativarCategoriaServico)
academia.PUT("/categorias-servico/:id/reativar", handlers.ReativarCategoriaServico)
```

**Não esqueça a whitelist** — é a causa mais comum de erro 500 silencioso neste repositório (já aconteceu duas vezes: `NotaCorrigida`/`FaltaCorrigida` e, na Tarefa 81/82, `SolicitacaoAlteracaoNIFAcademia`). Compila e roda, mas toda escrita falha.

## 4. `ServicoExtra.categoria` → `categoria_servico_id`

Em `internal/domain/aggregates/servico_extra.go`:

1. Troque `Categoria string` (linha 34) por `CategoriaServicoID *uuid.UUID`.
2. Em `ServicoExtraCriadoEvent` e `ServicoExtraAtualizadoEvent`, troque `Categoria string`/`Categoria *string` por `CategoriaServicoID *uuid.UUID` (em `Atualizado`, o ponteiro já cobre "não alterar" quando `nil` — **não precisa de ponteiro-de-ponteiro**: enviar um `*uuid.UUID` nil no evento de atualização já significa "não mudou"; para "remover a categoria explicitamente", o handler distingue via `informado["categoria_servico_id"]`, seção 5).
3. Em `Criar`, troque o parâmetro `categoria string` por `categoriaServicoID *uuid.UUID`; remova qualquer `strings.TrimSpace(categoria)` associado; a validação de posse/existência/status (seção 3.3) **não entra aqui** — é feita no handler antes de chamar `Criar`/`Atualizar`, pelo mesmo motivo já documentado no topo do arquivo (aggregate não importa outras projeções para evitar ciclo).
4. Em `applyCriado`/`applyAtualizado`, troque as atribuições de `s.Categoria` por `s.CategoriaServicoID`.
5. Em `Atualizar`, o parâmetro correspondente também vira `categoriaServicoID *uuid.UUID`, propagado sem transformação para o evento (a semântica de ponteiro já é a certa: nil = não alterar).

## 5. Handler `servico_extra_handlers.go` — payload e validação

1. Em `servicoExtraPayload`, troque `Categoria string \`json:"categoria"\`` por `CategoriaServicoID *uuid.UUID \`json:"categoria_servico_id"\``.
2. Em `bindServicoExtraPayload`, troque `"categoria"` por `"categoria_servico_id"` no mapa `allowed`.
3. Em `CriarServicoExtra`, depois da checagem de credenciais e antes de `validarPosseCursosDisponiveis`, adicione:
   ```go
   if e := validarCategoriaServico(c, codigo, r.CategoriaServicoID); e != nil {
       utils.RespondWithValidationError(c, e)
       return
   }
   ```
   E troque `r.Categoria` por `r.CategoriaServicoID` na chamada a `s.Criar(...)`.
4. Em `AtualizarServicoExtra`, mesma checagem de `validarCategoriaServico` quando `r.informado["categoria_servico_id"]` for verdadeiro, e troque o argumento correspondente na chamada a `.Atualizar(...)`.
5. Em `servicoExtraToJSON` (o helper que serializa o aggregate em memória com chaves snake_case — não confunda com a projeção), troque a chave `"categoria"` por `"categoria_servico_id": s.CategoriaServicoID` (serializa como string UUID ou `null`, igual a qualquer outro ponteiro).

## 6. `detalhes_personalizados` tipado

### 6.1 Novo tipo em `servico_extra.go`

```go
// DetalhePersonalizado é um único item de personalização livre de um
// ServicoExtra: um rótulo de exibição, um valor, e o tipo desse valor — de
// um conjunto FECHADO de 6 tipos (ver tiposPersonalizadosValidos). Fechado
// deliberadamente: o objetivo é um formulário previsível no frontend (6
// widgets possíveis: texto, número, sim/não, data, hora, lista de textos),
// não um JSON Schema genérico.
type DetalhePersonalizado struct {
	Rotulo string      `json:"rotulo"`
	Valor  interface{} `json:"valor"`
	Tipo   string      `json:"tipo"`
}

var tiposPersonalizadosValidos = map[string]bool{
	"texto": true, "numero": true, "booleano": true, "data": true, "hora": true, "lista_texto": true,
}

const maxDetalhesPersonalizados = 30

var chaveDetalhePersonalizadoRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// validarDetalhesPersonalizados já foi testada isoladamente com 15 casos
// (ver seção 0 deste documento) — não altere a lógica, só adapte imports se
// necessário.
func validarDetalhesPersonalizados(m map[string]DetalhePersonalizado) error {
	if len(m) > maxDetalhesPersonalizados {
		return fmt.Errorf("no máximo %d detalhes personalizados são permitidos", maxDetalhesPersonalizados)
	}
	for chave, d := range m {
		if !chaveDetalhePersonalizadoRegex.MatchString(chave) {
			return fmt.Errorf("chave de detalhe personalizado inválida: %q (use letras minúsculas, números e _, começando por letra, até 50 caracteres)", chave)
		}
		if strings.TrimSpace(d.Rotulo) == "" {
			return fmt.Errorf("detalhe personalizado %q precisa de um rótulo", chave)
		}
		if len(d.Rotulo) > 100 {
			return fmt.Errorf("rótulo do detalhe personalizado %q deve ter no máximo 100 caracteres", chave)
		}
		if !tiposPersonalizadosValidos[d.Tipo] {
			return fmt.Errorf("tipo de detalhe personalizado %q inválido para %q: use texto, numero, booleano, data, hora ou lista_texto", d.Tipo, chave)
		}
		if err := validarValorPersonalizado(chave, d.Tipo, d.Valor); err != nil {
			return err
		}
	}
	return nil
}

func validarValorPersonalizado(chave, tipo string, valor interface{}) error {
	switch tipo {
	case "texto":
		if _, ok := valor.(string); !ok {
			return fmt.Errorf("valor de %q deve ser texto", chave)
		}
	case "numero":
		if _, ok := valor.(float64); !ok {
			return fmt.Errorf("valor de %q deve ser numérico", chave)
		}
	case "booleano":
		if _, ok := valor.(bool); !ok {
			return fmt.Errorf("valor de %q deve ser verdadeiro ou falso", chave)
		}
	case "data":
		s, ok := valor.(string)
		if !ok {
			return fmt.Errorf("valor de %q deve ser uma data no formato AAAA-MM-DD", chave)
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return fmt.Errorf("valor de %q não é uma data válida (use AAAA-MM-DD)", chave)
		}
	case "hora":
		s, ok := valor.(string)
		if !ok {
			return fmt.Errorf("valor de %q deve ser uma hora no formato HH:MM", chave)
		}
		if _, err := time.Parse("15:04", s); err != nil {
			return fmt.Errorf("valor de %q não é uma hora válida (use HH:MM)", chave)
		}
	case "lista_texto":
		lst, ok := valor.([]interface{})
		if !ok {
			return fmt.Errorf("valor de %q deve ser uma lista de textos", chave)
		}
		for _, item := range lst {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("todos os itens de %q devem ser texto", chave)
			}
		}
	}
	return nil
}
```

Adicione `"regexp"` e `"time"` aos imports de `servico_extra.go` (confira se `"time"` já não está lá — provavelmente já está, por causa de `CreatedAt`/`UpdatedAt`).

### 6.2 Troca do tipo do campo

Em todo o arquivo `servico_extra.go`, troque `DetalhesPersonalizados map[string]interface{}` por `DetalhesPersonalizados map[string]DetalhePersonalizado` — na struct `ServicoExtra`, em `ServicoExtraCriadoEvent`, em `ServicoExtraAtualizadoEvent`, e na inicialização em `NewServicoExtra()` (`map[string]DetalhePersonalizado{}`).

### 6.3 Chamar a validação

Em `Criar`, logo depois de `if detalhesPersonalizados == nil { detalhesPersonalizados = map[string]DetalhePersonalizado{} }`, adicione:
```go
if err := validarDetalhesPersonalizados(detalhesPersonalizados); err != nil {
    return err
}
```
Em `Atualizar`, quando `detalhesPersonalizados != nil` (ou seja, o cliente enviou o campo), valide da mesma forma antes de montar o evento.

### 6.4 Handler — payload

Em `servicoExtraPayload` (`servico_extra_handlers.go`), troque `DetalhesPersonalizados map[string]interface{} \`json:"detalhes_personalizados"\`` por `DetalhesPersonalizados map[string]aggregates.DetalhePersonalizado \`json:"detalhes_personalizados"\``. Nenhuma outra mudança é necessária no binding — a tag `json` já existe e está correta (confirmado no teste isolado da seção 0, que reproduz exatamente esta struct).

Em `servicoExtraToJSON`, a chave `"detalhes_personalizados": s.DetalhesPersonalizados` continua igual — o novo tipo serializa naturalmente para o formato `{"chave": {"rotulo": ..., "valor": ..., "tipo": ...}}`.

## 7. Projeção `ServicoExtraProjection` — DTO e persistência

Em `internal/projections/servico_extra_projection.go`:
1. `ServicoExtraDTO`: troque `Categoria string \`json:"categoria,omitempty"\`` por `CategoriaServicoID *uuid.UUID \`json:"categoria_servico_id,omitempty"\``.
2. `DetalhesPersonalizados`: troque o tipo Go de `map[string]interface{}` para `map[string]aggregates.DetalhePersonalizado` (mesmo tipo do aggregate — a coluna no banco continua `JSONB`, só a interpretação em Go muda).
3. Ajuste o `Handle`/`Rebuild` (INSERT/UPDATE e SELECT) para gravar/ler `categoria_servico_id` em vez de `categoria`, e serializar `DetalhesPersonalizados` via `json.Marshal`/`json.Unmarshal` como já era feito (o tipo do valor Go muda, a serialização para `JSONB` não).

## 8. Testes obrigatórios

### 8.1 `internal/domain/aggregates/categoria_servico_test.go` (novo)
- `Criar` com nome vazio → erro; com nome válido → sucesso, `Ativo=true`.
- `Renomear` → sucesso; com nome vazio → erro.
- `Desativar`/`Reativar` → sucesso e erro de estado duplicado (mesmo padrão dos testes já existentes de `ServicoExtra`).

### 8.2 `internal/domain/aggregates/servico_extra_test.go` — adicionar
- `Criar` com `detalhes_personalizados` usando os **dois exemplos exatos do pedido original** ("Transporte" com `rota`/`ponto_de_embarque`/`horario_de_saida` tipo `hora`; "Natação" com `piscina`/`exige_saber_nadar`/`equipamento_incluido` tipo `lista_texto`) → sucesso.
- Um caso para cada um dos 6 tipos com valor **incompatível** (ex.: `tipo=numero` com `valor="dez"`) → erro.
- Um caso com mais de 30 entradas em `detalhes_personalizados` → erro.
- Um caso com chave em formato inválido (maiúscula, espaço, começando por número) → erro.
- Um caso com `categoria_servico_id` — como o aggregate não valida posse (isso é no handler), basta confirmar que o campo é aceito e persistido no `Apply` sem erro quando não-nil.

### 8.3 Teste de integração (requer Postgres — `RUN_POSTGRES_INTEGRATION=1`)
- Criar categoria "Transporte"; tentar criar "transporte" na mesma academia → rejeitado pelo banco (`ux_categorias_servico_nome_ativo`); em academia diferente → aceito.
- Criar um `ServicoExtra` com `categoria_servico_id` de uma categoria de **outra** academia → rejeitado pelo handler (`validarCategoriaServico`).
- Desativar a categoria depois de já usá-la num serviço → o serviço continua consultável normalmente; tentar usá-la num **novo** serviço → rejeitado.

## 9. Checklist de aceite

- [ ] Migration 122 aplicada sem erro; unicidade de nome por academia testada (já validado — seção 0).
- [ ] `CategoriaServico` registrado na whitelist (aggregate + 4 eventos) e na factory.
- [x] CRUD de categoria completo, com checagem de posse e resposta snake_case (nunca o aggregate cru).
- [x] `ServicoExtra.categoria_servico_id` substitui `categoria` em toda a cadeia: aggregate, eventos, handler, payload, projeção, resposta.
- [x] `validarCategoriaServico` chamada em `CriarServicoExtra` e `AtualizarServicoExtra` (quando informado).
- [x] `detalhes_personalizados` tipado (`rotulo`/`valor`/`tipo`), com os 6 tipos e a validação de valor-contra-tipo — já testada isoladamente (seção 0), só precisa ser transcrita fielmente.
- [ ] Os dois exemplos do pedido original ("Transporte", "Natação") funcionam ponta a ponta.
- [x] `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...` limpos no seu ambiente.
- [ ] Resultado reportado ao final: o que passou, o que falhou, o que não pôde ser testado no seu ambiente e por quê.
