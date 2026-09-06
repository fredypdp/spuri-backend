---
criado: 06-87-2026
origem: Fredy + Claude (orquestração)
status: pronto para execução
tipo: backend (spuri-backend)
depende_de: nenhuma (mas a Tarefa "Serviços Extras - Reestruturação de Rotas, Sub-tela de Criação e Cursos (Frontend)" depende desta)
---

# Tarefa 87 — Serviços Extras: disponibilidade por curso + anos combinados

### Documento de execução para o Codex (orquestrado e pré-testado pelo Claude)

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Eu (Claude) validei o que dava para validar no meu sandbox, com limitações reais que você precisa conhecer:

- **Banco de dados: validado de verdade.** Instalei PostgreSQL 16 no meu sandbox, apliquei as 128 migrations que já existem hoje no repositório (`001` a `120`, na mesma ordem alfabética que `internal/db/migrations.go` usa via `os.ReadDir`) numa base limpa — sem erro — e depois apliquei a migration 121 nova (seção 5 abaixo) por cima. Também inseri uma linha de teste em `projection_servicos_extras` combinando `anos_academicos_disponiveis = {6_ano_fundamental,7_ano_fundamental}` com `cursos_disponiveis = {<uuid>|2_ano_medio}` — o array `TEXT[]` guarda e devolve o formato `"<curso_id>|<ano_academico>"` exatamente como o `pq.Array()` do Go espera, e os `CHECK constraints` existentes (`chk_servico_extra_pago_campos`, `chk_servico_extra_taxa_campos`) continuam satisfeitos com a coluna nova presente.
- **Compilação Go: não consegui validar, e você provavelmente também não vai conseguir do jeito padrão.** `go.mod` exige `go 1.24.0` (toolchain `go1.24.12`). Instalei Go 1.22 via `apt` no meu sandbox, mas o `go build` tenta baixar automaticamente o toolchain 1.24 de `proxy.golang.org` e o meu proxy de rede bloqueia esse host (`403 Forbidden: Host not in allowlist`). `GOTOOLCHAIN=local` só troca o erro por "go.mod requires go >= 1.24.0 (running go 1.22.2)". Isto é a mesma limitação de rede já registrada na Tarefa 87 original do módulo de Serviços Extras (`docs/Tarefas feitas/83 - ... Fase 1 ....md`, seção 0) — não é novidade introduzida por esta tarefa, e o seu ambiente deve ter acesso ao toolchain correto (via `proxy.golang.org` ou um `GOPROXY` configurado) para compilar normalmente.
- **O que isso muda na prática para você:** rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...` (sequencial: `go test -p 1 ./...`, pela mesma razão de condição de corrida pré-existente já documentada nas tarefas anteriores) normalmente. Se aparecer erro de compilação em algo descrito abaixo, é desvio pontual de sintaxe/import a corrigir mantendo fielmente as decisões de design já tomadas — não é motivo para redesenhar nada.
- Referências de número de linha abaixo são precisas **no momento em que este documento foi escrito** (clone fresco de `main` de ambos os repositórios). Se algo já mudou por outra tarefa concorrente, localize sempre pelo conteúdo citado (a assinatura da função, o texto do comentário), não confie cegamente no número.

## 1. Prompt recomendado para executar esta tarefa

> Aplique exatamente o que está descrito neste documento (migration, aggregate, handlers, projeção, testes), na ordem das seções. Não replaneje nem redesenhe nada do que já está decidido — as decisões de design da seção 3 são definitivas. Ao final, rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test -p 1 ./...`, corrija qualquer erro, e preencha o checklist da seção 12.

## 2. Contexto e objetivo de negócio

O módulo de Serviços Extras (Tarefas 83-86, já em produção) permite que uma academia cadastre serviços adicionais (transporte, atividades extracurriculares, etc.) e restrinja opcionalmente a quais anos acadêmicos o serviço se aplica, via `anos_academicos_disponiveis []string` — uma lista solta de anos, sem qualquer vínculo com curso. Por decisão de design explícita da época (comentário em `validarAnosAcademicosServicoExtra`), essa lista **nunca** foi cruzada com cursos/turmas reais — nem na validação (só formato), nem em lugar nenhum: confirmei lendo `SolicitarServicoExtra` (`internal/handlers/servico_extra_solicitacao_handlers.go`) que o backend **não** verifica hoje se o ano do estudante bate com `anos_academicos_disponiveis` do serviço antes de aceitar a solicitação. O campo é, na prática atual, apenas informativo.

O pedido de produto agora é mais específico: uma academia com ensino médio/superior precisa poder disponibilizar um serviço extra apenas para estudantes de um ou mais **cursos** específicos (e, dentro de cada curso, apenas para determinados anos daquele curso) — porque um mesmo ano (ex. "2º ano médio") pode existir em vários cursos diferentes da mesma academia com populações de estudantes completamente diferentes. Ao mesmo tempo, uma academia mista (fundamental + médio) deve poder combinar, no mesmo serviço, anos fundamentais soltos (sem curso, porque fundamental nunca tem curso neste sistema) com anos de um curso específico do médio.

Esta tarefa:
1. Adiciona um novo campo `cursos_disponiveis` ao aggregate `ServicoExtra`, aditivo e retrocompatível com `anos_academicos_disponiveis`.
2. Passa a **de fato aplicar** a elegibilidade no momento da solicitação (`SolicitarServicoExtra`) — hoje isso não acontece para nenhum dos dois campos.

## 3. Decisões de design já tomadas (não repensar)

1. **Nenhuma migração/reinterpretação de dados existentes.** `anos_academicos_disponiveis` continua exatamente como está, com o mesmo significado de sempre. Nenhuma linha existente é tocada por esta tarefa.
2. **Novo campo aditivo, não substituto.** `cursos_disponiveis []string`, cada item no formato `"<curso_id>|<ano_academico>"`. Combinável **simultaneamente** com `anos_academicos_disponiveis` no mesmo serviço — é assim que uma academia mista habilita "1-9 fundamental" (via `anos_academicos_disponiveis`) + "2º e 3º ano de um curso médio específico" (via `cursos_disponiveis`) ao mesmo tempo, exatamente como pedido. "Ambos vazios" continua significando "disponível para todos", preservando o comportamento atual.
3. **Semântica legada preservada.** Uma entrada `"_ano_medio"`/`"_ano_superior"` solta (sem "`|`") em `anos_academicos_disponiveis` — seja em registros já existentes, seja porque alguém decida enviar isso via API diretamente — continua válida e significa "disponível nesse ano em **qualquer** curso da academia" (o comportamento de sempre). Não é erro, não precisa ser convertida. O frontend (tarefa separada) só vai parar de **gerar** esse formato para médio/superior daqui em diante — vai gerar sempre `curso_id|ano` para esses dois níveis, mas o backend continua aceitando o formato solto por compatibilidade.
4. **Fundamental nunca é escopado a curso.** Este sistema não tem o conceito de curso para ensino fundamental (confirmado em `AppSidebar.tsx`: o item "Cursos" é removido do menu para academias com `nivel_escolar === "fundamental"`). Por isso `cursos_disponiveis` só aceita anos terminados em `_ano_medio` ou `_ano_superior` — nunca `_ano_fundamental`. Anos fundamentais soltos continuam indo em `anos_academicos_disponiveis`, como já é hoje.
5. **Separação aggregate/handler mantida.** O aggregate (`servico_extra.go`) só valida o **formato** de `cursos_disponiveis` (é um UUID válido + ano com sufixo certo) — nunca importa a projeção de Cursos, para não criar ciclo de import nem acoplar o domínio a infraestrutura. A checagem "do mundo real" (o curso existe, pertence à mesma academia, não está deletado, o ano faz parte dos anos do curso, o tipo do curso bate com o sufixo do ano) é feita pelo **handler**, exatamente pela mesma razão e no mesmo padrão que a checagem de credenciais AppyPay já é feita hoje (ver comentário acima de `Criar` em `servico_extra.go`).
6. **Elegibilidade passa a ser aplicada de verdade em `SolicitarServicoExtra`.** Hoje não é. Esta tarefa adiciona a checagem: se o serviço tem qualquer restrição (`anos_academicos_disponiveis` OU `cursos_disponiveis` não-vazios), o estudante só pode solicitar se o ano/curso atual dele bater com alguma das duas listas. Isto é uma mudança de comportamento deliberada e pedida — revisita parcialmente a "decisão de design 7" da Tarefa 87 original (que dizia que a lista era só informativa), porque agora faz sentido de negócio que ela restrinja de verdade.
7. **Cursos "inativo" são aceitos; "deletado" não.** Uma academia pode querer preparar um serviço para um curso pausado temporariamente. Só curso com `status = "deletado"` é rejeitado na seleção.
8. **Nenhum novo tipo de evento.** `CursosDisponiveis` é só mais um campo nos eventos `ServicoExtraCriado`/`ServicoExtraAtualizado` já existentes — não crie `ServicoExtraCursosAtualizado` nem nada parecido. `internal/db/safe_queries.go` já tem `"ServicoExtra"` em `validAggregateTypes` e os quatro tipos de evento (`ServicoExtraCriado/Atualizado/Desativado/Reativado`) em `validEventTypes` — **nenhuma alteração é necessária nesse arquivo** para esta tarefa.

## 4. Fora de escopo (não implementar)

- Qualquer alteração em `internal/finance/servico_extra.go` ou `cobranca_geracao.go` — nenhum dos dois lê `anos_academicos_disponiveis`/`cursos_disponiveis` hoje, e esta tarefa não muda isso.
- Migrar/reescrever `anos_academicos_disponiveis` de registros existentes.
- Qualquer tela de frontend — é a tarefa separada "Serviços Extras - Reestruturação de Rotas, Sub-tela de Criação e Cursos (Frontend)", que **depende** desta.
- Materializar `cursos_disponiveis` numa tabela auxiliar tipo `projection_regras_avaliacao_final_escopos` (migration 080). Esse padrão existe lá para suportar unicidade/consulta em massa entre muitas regras; aqui a checagem de elegibilidade é feita em memória, item a item, sobre no máximo algumas dezenas de serviços por academia — não precisa de tabela auxiliar nem trigger.
- Endpoint novo para "listar serviços elegíveis para o estudante atual" — a listagem pública (`GET /academia/servico/:codigo_academia/servicos-extras`) continua devolvendo todos os serviços ativos da academia, como hoje; a checagem de elegibilidade acontece só no momento da solicitação (seção 9). Se o frontend quiser filtrar visualmente o catálogo, ele já recebe `anos_academicos_disponiveis` e `cursos_disponiveis` no DTO e pode comparar com os dados do estudante autenticado — isso é decisão da tarefa de frontend, não desta.

## 5. Modelo de dados

### 5.1 Migration

Crie `migrations/121_servico_extra_cursos_disponiveis.sql`:

```sql
-- ============================================================================
-- MIGRATION 121 — Adiciona cursos_disponiveis a projection_servicos_extras
-- ============================================================================
--
-- CONTEXTO:
--   anos_academicos_disponiveis (migration 118) é uma lista solta de anos
--   acadêmicos, sem vínculo com curso — suficiente para o ensino fundamental,
--   que nunca tem cursos neste sistema (confirmado: AppSidebar.tsx remove o
--   item "Cursos" do menu para academias com nivel_escolar='fundamental').
--   Para médio e superior, o mesmo ano (ex.: "2_ano_medio") pode existir em
--   vários cursos diferentes da mesma academia, e agora um serviço extra
--   precisa poder se limitar a um ou mais cursos específicos.
--
--   Esta migration adiciona cursos_disponiveis, onde cada item usa o formato
--   "<curso_id>|<ano_academico>" — mesma técnica de codificação já usada em
--   projection_regras_avaliacao_final.anos_academicos (migration 080,
--   regras_avaliacao_final_escopos.sql) para o mesmo problema geral de
--   "ano dentro de um curso específico". Combinável simultaneamente com
--   anos_academicos_disponiveis: um serviço pode estar disponível para
--   "6_ano_fundamental" (solto) E para "<curso_id_medio>|2_ano_medio"
--   (escopado a um curso) ao mesmo tempo — ver decisão de design 2 no
--   documento da tarefa.
--
--   anos_academicos_disponiveis permanece EXATAMENTE como está — nenhuma
--   linha existente é migrada ou reinterpretada. Registros antigos com
--   "_ano_medio"/"_ano_superior" soltos ali continuam válidos e passam a
--   significar "disponível para esse ano em QUALQUER curso da academia"
--   (semântica legada — ver decisão de design 3).
--
--   Formato validado em Go (validarCursosDisponiveisServicoExtra, apenas
--   formato). Posse do curso pela mesma academia, correspondência de tipo
--   (medio/superior) e pertencimento do ano aos anos_academicos do curso são
--   validados no handler (validarPosseCursosDisponiveis) — mesma separação
--   aggregate/handler já usada para a checagem de credenciais AppyPay.
-- ============================================================================

BEGIN;

ALTER TABLE projection_servicos_extras
    ADD COLUMN cursos_disponiveis TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[];

COMMENT ON COLUMN projection_servicos_extras.cursos_disponiveis IS
    'Cada item no formato "<curso_id>|<ano_academico>" (ano_academico termina em _ano_medio ou _ano_superior — nunca _ano_fundamental). Lista vazia junto com anos_academicos_disponiveis vazio = disponível para todos os anos/cursos. Combinável com anos_academicos_disponiveis (migration 118). Adicionada na migration 121.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 121 — cursos_disponiveis adicionada a projection_servicos_extras';
END $$;
```

Já validei esta migration de ponta a ponta (seção 0) — pode aplicar sem medo de conflito com as 120 migrations anteriores.

### 5.2 Whitelist do ledger

Nada a fazer — ver decisão de design 8.

## 6. Aggregate `ServicoExtra` (`internal/domain/aggregates/servico_extra.go`)

### 6.1 Campo novo na struct

**Localizar:**
```go
	AnosAcademicosDisponiveis []string

	DocumentoObrigatorio bool
```
**Substituir por:**
```go
	AnosAcademicosDisponiveis []string
	CursosDisponiveis         []string

	DocumentoObrigatorio bool
```

### 6.2 `NewServicoExtra()`

**Localizar:**
```go
		MetodosPagamento:              []string{},
		MetodosPagamentoTaxaInscricao: []string{},
		AnosAcademicosDisponiveis:     []string{},
		DetalhesPersonalizados:        map[string]interface{}{},
```
**Substituir por:**
```go
		MetodosPagamento:              []string{},
		MetodosPagamentoTaxaInscricao: []string{},
		AnosAcademicosDisponiveis:     []string{},
		CursosDisponiveis:             []string{},
		DetalhesPersonalizados:        map[string]interface{}{},
```

### 6.3 Evento `ServicoExtraCriadoEvent`

**Localizar:**
```go
	MetodosPagamentoTaxaInscricao []string
	AnosAcademicosDisponiveis     []string
	DocumentoObrigatorio          bool
	DocumentoInstrucoes           string
	DetalhesPersonalizados        map[string]interface{}
	CriadoPor                     uuid.UUID
	CreatedAt                     time.Time
}
```
**Substituir por:**
```go
	MetodosPagamentoTaxaInscricao []string
	AnosAcademicosDisponiveis     []string
	CursosDisponiveis             []string
	DocumentoObrigatorio          bool
	DocumentoInstrucoes           string
	DetalhesPersonalizados        map[string]interface{}
	CriadoPor                     uuid.UUID
	CreatedAt                     time.Time
}
```

### 6.4 Evento `ServicoExtraAtualizadoEvent`

**Localizar:**
```go
	MetodosPagamentoTaxaInscricao *[]string
	AnosAcademicosDisponiveis     *[]string
	DocumentoObrigatorio          *bool
	DocumentoInstrucoes           *string
	DetalhesPersonalizados        map[string]interface{} // nil = não alterar; não-nil substitui o mapa inteiro
	AtualizadoPor                 uuid.UUID
	UpdatedAt                     time.Time
}
```
**Substituir por:**
```go
	MetodosPagamentoTaxaInscricao *[]string
	AnosAcademicosDisponiveis     *[]string
	CursosDisponiveis             *[]string
	DocumentoObrigatorio          *bool
	DocumentoInstrucoes           *string
	DetalhesPersonalizados        map[string]interface{} // nil = não alterar; não-nil substitui o mapa inteiro
	AtualizadoPor                 uuid.UUID
	UpdatedAt                     time.Time
}
```

### 6.5 `Criar` — assinatura, validação e evento

**Localizar (assinatura):**
```go
func (s *ServicoExtra) Criar(
	codigoAcademia, nome, descricao, categoria string,
	pago bool, preco float64, tipoCobranca string, metodosPagamento []string,
	temTaxaInscricao bool, valorTaxaInscricao float64, metodosPagamentoTaxaInscricao []string,
	anosAcademicosDisponiveis []string,
	documentoObrigatorio bool, documentoInstrucoes string,
	detalhesPersonalizados map[string]interface{},
	criadoPor uuid.UUID,
) error {
```
**Substituir por:**
```go
func (s *ServicoExtra) Criar(
	codigoAcademia, nome, descricao, categoria string,
	pago bool, preco float64, tipoCobranca string, metodosPagamento []string,
	temTaxaInscricao bool, valorTaxaInscricao float64, metodosPagamentoTaxaInscricao []string,
	anosAcademicosDisponiveis []string,
	cursosDisponiveis []string,
	documentoObrigatorio bool, documentoInstrucoes string,
	detalhesPersonalizados map[string]interface{},
	criadoPor uuid.UUID,
) error {
```

**Localizar:**
```go
	if err := validarAnosAcademicosServicoExtra(anosAcademicosDisponiveis); err != nil {
		return err
	}
	if !pago {
```
**Substituir por:**
```go
	if err := validarAnosAcademicosServicoExtra(anosAcademicosDisponiveis); err != nil {
		return err
	}
	if err := validarCursosDisponiveisServicoExtra(cursosDisponiveis); err != nil {
		return err
	}
	if !pago {
```

**Localizar:**
```go
		MetodosPagamentoTaxaInscricao: metodosPagamentoTaxaInscricao,
		AnosAcademicosDisponiveis:     anosAcademicosDisponiveis,
		DocumentoObrigatorio:          documentoObrigatorio,
		DocumentoInstrucoes:           strings.TrimSpace(documentoInstrucoes),
```
**Substituir por:**
```go
		MetodosPagamentoTaxaInscricao: metodosPagamentoTaxaInscricao,
		AnosAcademicosDisponiveis:     anosAcademicosDisponiveis,
		CursosDisponiveis:             cursosDisponiveis,
		DocumentoObrigatorio:          documentoObrigatorio,
		DocumentoInstrucoes:           strings.TrimSpace(documentoInstrucoes),
```
(esta é a montagem do `ServicoExtraCriadoEvent` dentro de `Criar` — há só uma ocorrência deste bloco exato no arquivo.)

### 6.6 `Atualizar` — assinatura, validação e evento

**Localizar (assinatura):**
```go
func (s *ServicoExtra) Atualizar(
	nome, descricao, categoria *string,
	pago *bool, preco *float64, tipoCobranca *string, metodosPagamento *[]string,
	temTaxaInscricao *bool, valorTaxaInscricao *float64, metodosPagamentoTaxaInscricao *[]string,
	anosAcademicosDisponiveis *[]string,
	documentoObrigatorio *bool, documentoInstrucoes *string,
	detalhesPersonalizados map[string]interface{},
	atualizadoPor uuid.UUID,
) error {
```
**Substituir por:**
```go
func (s *ServicoExtra) Atualizar(
	nome, descricao, categoria *string,
	pago *bool, preco *float64, tipoCobranca *string, metodosPagamento *[]string,
	temTaxaInscricao *bool, valorTaxaInscricao *float64, metodosPagamentoTaxaInscricao *[]string,
	anosAcademicosDisponiveis *[]string,
	cursosDisponiveis *[]string,
	documentoObrigatorio *bool, documentoInstrucoes *string,
	detalhesPersonalizados map[string]interface{},
	atualizadoPor uuid.UUID,
) error {
```

**Localizar:**
```go
	if anosAcademicosDisponiveis != nil {
		if err := validarAnosAcademicosServicoExtra(*anosAcademicosDisponiveis); err != nil {
			return err
		}
	}

	// Zera campos que deixaram de se aplicar,
```
**Substituir por:**
```go
	if anosAcademicosDisponiveis != nil {
		if err := validarAnosAcademicosServicoExtra(*anosAcademicosDisponiveis); err != nil {
			return err
		}
	}
	if cursosDisponiveis != nil {
		if err := validarCursosDisponiveisServicoExtra(*cursosDisponiveis); err != nil {
			return err
		}
	}

	// Zera campos que deixaram de se aplicar,
```

**Localizar:**
```go
		MetodosPagamentoTaxaInscricao: metodosPagamentoTaxaInscricao,
		AnosAcademicosDisponiveis:     anosAcademicosDisponiveis,
		DocumentoObrigatorio:          documentoObrigatorio,
		DocumentoInstrucoes:           documentoInstrucoes,
```
**Substituir por:**
```go
		MetodosPagamentoTaxaInscricao: metodosPagamentoTaxaInscricao,
		AnosAcademicosDisponiveis:     anosAcademicosDisponiveis,
		CursosDisponiveis:             cursosDisponiveis,
		DocumentoObrigatorio:          documentoObrigatorio,
		DocumentoInstrucoes:           documentoInstrucoes,
```
(esta é a montagem do `ServicoExtraAtualizadoEvent` dentro de `Atualizar` — bloco único no arquivo; note que é diferente do bloco 6.5, que não tem ponteiros.)

### 6.7 `applyCriado`

**Localizar:**
```go
	s.AnosAcademicosDisponiveis = p.AnosAcademicosDisponiveis
	s.DocumentoObrigatorio = p.DocumentoObrigatorio
	s.DocumentoInstrucoes = p.DocumentoInstrucoes
	s.DetalhesPersonalizados = p.DetalhesPersonalizados
	s.Ativo = true
```
**Substituir por:**
```go
	s.AnosAcademicosDisponiveis = p.AnosAcademicosDisponiveis
	s.CursosDisponiveis = p.CursosDisponiveis
	s.DocumentoObrigatorio = p.DocumentoObrigatorio
	s.DocumentoInstrucoes = p.DocumentoInstrucoes
	s.DetalhesPersonalizados = p.DetalhesPersonalizados
	s.Ativo = true
```

### 6.8 `applyAtualizado`

**Localizar:**
```go
	if p.AnosAcademicosDisponiveis != nil {
		s.AnosAcademicosDisponiveis = *p.AnosAcademicosDisponiveis
	}
	if p.DocumentoObrigatorio != nil {
```
**Substituir por:**
```go
	if p.AnosAcademicosDisponiveis != nil {
		s.AnosAcademicosDisponiveis = *p.AnosAcademicosDisponiveis
	}
	if p.CursosDisponiveis != nil {
		s.CursosDisponiveis = *p.CursosDisponiveis
	}
	if p.DocumentoObrigatorio != nil {
```

### 6.9 Novo validador de formato

**Localizar (imediatamente após o fim de `validarAnosAcademicosServicoExtra`, último bloco do arquivo):**
```go
		default:
			return fmt.Errorf("formato de ano acadêmico inválido: %q", ano)
		}
	}
	return nil
}
```
**Substituir por:**
```go
		default:
			return fmt.Errorf("formato de ano acadêmico inválido: %q", ano)
		}
	}
	return nil
}

// validarCursosDisponiveisServicoExtra valida apenas o FORMATO de cada item —
// "<curso_id>|<ano_academico>", com curso_id sendo um UUID válido e
// ano_academico terminando em _ano_medio ou _ano_superior (nunca
// _ano_fundamental — o ensino fundamental não tem cursos neste sistema, ver
// decisão de design 4 no documento da tarefa). Lista vazia é válida. NÃO
// verifica se o curso existe, pertence à mesma academia, está ativo, ou se o
// ano faz parte dos anos_academicos do curso — essas checagens exigem acesso
// à projeção de Cursos e são feitas pelo HANDLER
// (validarPosseCursosDisponiveis em servico_extra_handlers.go), não pelo
// aggregate — mesma separação de responsabilidades já usada para a checagem
// de credenciais AppyPay (ver comentário acima de Criar).
func validarCursosDisponiveisServicoExtra(cursos []string) error {
	for _, item := range cursos {
		partes := strings.SplitN(item, "|", 2)
		if len(partes) != 2 || partes[0] == "" || partes[1] == "" {
			return fmt.Errorf("formato de curso_disponivel inválido: %q — esperado \"<curso_id>|<ano_academico>\"", item)
		}
		if _, err := uuid.Parse(partes[0]); err != nil {
			return fmt.Errorf("curso_id inválido em %q: %v", item, err)
		}
		ano := partes[1]
		switch {
		case strings.HasSuffix(ano, "_ano_medio"):
			if err := utils.ValidateAnoMedio(ano); err != nil {
				return err
			}
		case strings.HasSuffix(ano, "_ano_superior"):
			if err := utils.ValidateAnoSuperior(ano); err != nil {
				return err
			}
		default:
			return fmt.Errorf("ano_academico inválido em %q: deve terminar em _ano_medio ou _ano_superior (fundamental não usa curso)", item)
		}
	}
	return nil
}
```

### 6.10 Atualizar todas as chamadas existentes de `.Criar(`/`.Atualizar(` neste pacote

`internal/domain/aggregates/servico_extra_test.go` tem **6 chamadas** a `NewServicoExtra().Criar(...)`/`s.Criar(...)` e **1** chamada a `s.Atualizar(...)`, todas com argumentos posicionais na assinatura antiga (sem `cursosDisponiveis`). Adicione `nil` na nova posição (logo depois do argumento de `anosAcademicosDisponiveis`, antes de `documentoObrigatorio`/`false`) em cada uma das 6 chamadas de `Criar`, e `nil` na chamada de `Atualizar` (logo depois do argumento de `anosAcademicosDisponiveis`). `go build`/`go vet` vai apontar erro de contagem de argumentos em qualquer uma que você esquecer — trate isso como checklist, não como retrabalho.

Depois de corrigir as 7 chamadas existentes, **adicione** um novo teste `TestServicoExtraCursosDisponiveisValidation` no mesmo arquivo:

```go
func TestServicoExtraCursosDisponiveisValidation(t *testing.T) {
	id := uuid.New()
	cursoID := uuid.New().String()

	// válido: curso médio + ano médio
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil,
		[]string{cursoID + "|2_ano_medio"}, false, "", nil, id); e != nil {
		t.Fatalf("entrada válida rejeitada: %v", e)
	}
	// válido: combinando fundamental solto (anos_academicos_disponiveis) + curso (cursos_disponiveis)
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil,
		[]string{"6_ano_fundamental"}, []string{cursoID + "|3_ano_superior"}, false, "", nil, id); e != nil {
		t.Fatalf("combinação fundamental + curso rejeitada: %v", e)
	}
	// inválido: sem "|"
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil,
		[]string{cursoID + "-2_ano_medio"}, false, "", nil, id); e == nil {
		t.Fatal("formato sem separador '|' foi aceito")
	}
	// inválido: curso_id não é UUID
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil,
		[]string{"nao-e-uuid|2_ano_medio"}, false, "", nil, id); e == nil {
		t.Fatal("curso_id inválido foi aceito")
	}
	// inválido: fundamental não pode ser escopado a curso
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil,
		[]string{cursoID + "|6_ano_fundamental"}, false, "", nil, id); e == nil {
		t.Fatal("ano fundamental escopado a curso foi aceito")
	}
}
```

(ajuste a posição exata dos parênteses/vírgulas se a assinatura final ficar ligeiramente diferente do que descrevi — o importante é a cobertura dos 5 casos, não a formatação literal.)

## 7. Handlers (`internal/handlers/servico_extra_handlers.go`)

### 7.1 Import novo

**Localizar:**
```go
import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/utils"
	"strings"
)
```
**Substituir por:**
```go
import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"
	"strings"
)
```

### 7.2 Payload

**Localizar:**
```go
	MetodosPagamentoTaxaInscricao []string               `json:"metodos_pagamento_taxa_inscricao"`
	AnosAcademicosDisponiveis     []string               `json:"anos_academicos_disponiveis"`
	DocumentoObrigatorio          bool                   `json:"documento_obrigatorio"`
	DocumentoInstrucoes           string                 `json:"documento_instrucoes"`
	DetalhesPersonalizados        map[string]interface{} `json:"detalhes_personalizados"`
	informado                     map[string]bool
}
```
**Substituir por:**
```go
	MetodosPagamentoTaxaInscricao []string               `json:"metodos_pagamento_taxa_inscricao"`
	AnosAcademicosDisponiveis     []string               `json:"anos_academicos_disponiveis"`
	CursosDisponiveis             []string               `json:"cursos_disponiveis"`
	DocumentoObrigatorio          bool                   `json:"documento_obrigatorio"`
	DocumentoInstrucoes           string                 `json:"documento_instrucoes"`
	DetalhesPersonalizados        map[string]interface{} `json:"detalhes_personalizados"`
	informado                     map[string]bool
}
```

### 7.3 Whitelist de campos aceitos

**Localizar:**
```go
	allowed := map[string]bool{"nome": true, "descricao": true, "categoria": true, "pago": true, "preco": true, "tipo_cobranca": true, "metodos_pagamento": true, "tem_taxa_inscricao": true, "valor_taxa_inscricao": true, "metodos_pagamento_taxa_inscricao": true, "anos_academicos_disponiveis": true, "documento_obrigatorio": true, "documento_instrucoes": true, "detalhes_personalizados": true}
```
**Substituir por:**
```go
	allowed := map[string]bool{"nome": true, "descricao": true, "categoria": true, "pago": true, "preco": true, "tipo_cobranca": true, "metodos_pagamento": true, "tem_taxa_inscricao": true, "valor_taxa_inscricao": true, "metodos_pagamento_taxa_inscricao": true, "anos_academicos_disponiveis": true, "cursos_disponiveis": true, "documento_obrigatorio": true, "documento_instrucoes": true, "detalhes_personalizados": true}
```

### 7.4 Serialização de resposta

**Localizar:**
```go
		"anos_academicos_disponiveis":      s.AnosAcademicosDisponiveis,
		"documento_obrigatorio":            s.DocumentoObrigatorio,
```
**Substituir por:**
```go
		"anos_academicos_disponiveis":      s.AnosAcademicosDisponiveis,
		"cursos_disponiveis":               s.CursosDisponiveis,
		"documento_obrigatorio":            s.DocumentoObrigatorio,
```

### 7.5 Nova função de checagem de posse/consistência

Adicione esta função no mesmo arquivo (por exemplo, logo depois de `academy()`):

```go
// validarPosseCursosDisponiveis verifica, para cada item bem-formado de
// cursos_disponiveis, que o curso existe, pertence à mesma academia, não
// está deletado, que o tipo do curso (medio/superior) corresponde ao sufixo
// do ano, e que o ano faz parte de curso.AnosAcademicos. Esta é a checagem
// "do mundo real" que o aggregate deliberadamente não faz (ver comentário
// acima de validarCursosDisponiveisServicoExtra em servico_extra.go) — mesma
// separação já usada para a checagem de credenciais AppyPay em
// CriarServicoExtra/AtualizarServicoExtra. Itens malformados (sem "|", UUID
// inválido) são ignorados aqui — já foram rejeitados pelo aggregate antes
// deste ponto ser alcançado.
func validarPosseCursosDisponiveis(c *gin.Context, codigoAcademia string, cursosDisponiveis []string) error {
	cursosProj := getCursosProjection(c)
	cache := map[uuid.UUID]*projections.CursoDTO{}
	for _, item := range cursosDisponiveis {
		partes := strings.SplitN(item, "|", 2)
		if len(partes) != 2 {
			continue
		}
		cursoID, err := uuid.Parse(partes[0])
		if err != nil {
			continue
		}
		curso, ok := cache[cursoID]
		if !ok {
			curso, err = cursosProj.GetByID(cursoID)
			if err != nil {
				return fmt.Errorf("erro ao verificar curso %s: %v", cursoID, err)
			}
			cache[cursoID] = curso
		}
		if curso == nil {
			return fmt.Errorf("curso %s não encontrado", cursoID)
		}
		if curso.CodigoAcademia != codigoAcademia {
			return fmt.Errorf("curso %s não pertence a esta academia", cursoID)
		}
		if curso.Status == "deletado" {
			return fmt.Errorf("curso %s foi removido e não pode ser usado em serviços extras", cursoID)
		}
		ano := partes[1]
		tipoEsperado := "medio"
		if strings.HasSuffix(ano, "_ano_superior") {
			tipoEsperado = "superior"
		}
		if curso.Type != tipoEsperado {
			return fmt.Errorf("ano %q não corresponde ao tipo do curso %s (%s)", ano, cursoID, curso.Type)
		}
		anoValido := false
		for _, a := range curso.AnosAcademicos {
			if a == ano {
				anoValido = true
				break
			}
		}
		if !anoValido {
			return fmt.Errorf("ano %q não faz parte dos anos acadêmicos do curso %s", ano, cursoID)
		}
	}
	return nil
}

// estudanteElegivelServicoExtra verifica se o estudante pode se inscrever no
// serviço, cruzando o ano/curso atual do estudante com as restrições do
// serviço. O CHAMADOR já deve garantir que o serviço tem alguma restrição
// (len(AnosAcademicosDisponiveis)>0 || len(CursosDisponiveis)>0) antes de
// chamar esta função — listas vazias sempre significam "disponível para
// todos" e não devem cair aqui. Compatibilidade com dados anteriores a esta
// funcionalidade: um ano "_ano_medio"/"_ano_superior" solto em
// AnosAcademicosDisponiveis (sem curso associado) continua significando
// "disponível nesse ano em qualquer curso da academia" (decisão de design 3).
func estudanteElegivelServicoExtra(serv *projections.ServicoExtraDTO, est *projections.EstudanteDTO) bool {
	contains := func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	if est.AnoEscolar != nil && contains(serv.AnosAcademicosDisponiveis, *est.AnoEscolar) {
		return true
	}
	if est.AnoEscolarMedio != nil && contains(serv.AnosAcademicosDisponiveis, *est.AnoEscolarMedio) {
		return true
	}
	if est.AnoSuperior != nil && contains(serv.AnosAcademicosDisponiveis, *est.AnoSuperior) {
		return true
	}
	if est.CursoMedioID != nil && est.AnoEscolarMedio != nil && contains(serv.CursosDisponiveis, *est.CursoMedioID+"|"+*est.AnoEscolarMedio) {
		return true
	}
	if est.CursoSuperiorID != nil && est.AnoSuperior != nil && contains(serv.CursosDisponiveis, *est.CursoSuperiorID+"|"+*est.AnoSuperior) {
		return true
	}
	return false
}
```

### 7.6 `CriarServicoExtra` — chamar a checagem e passar o novo campo

**Localizar:**
```go
	s := aggregates.NewServicoExtra()
	if e := s.Criar(codigo, r.Nome, r.Descricao, r.Categoria, r.Pago, r.Preco, r.TipoCobranca, r.MetodosPagamento, r.TemTaxaInscricao, r.ValorTaxaInscricao, r.MetodosPagamentoTaxaInscricao, r.AnosAcademicosDisponiveis, r.DocumentoObrigatorio, r.DocumentoInstrucoes, r.DetalhesPersonalizados, id); e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
```
**Substituir por:**
```go
	if e := validarPosseCursosDisponiveis(c, codigo, r.CursosDisponiveis); e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	s := aggregates.NewServicoExtra()
	if e := s.Criar(codigo, r.Nome, r.Descricao, r.Categoria, r.Pago, r.Preco, r.TipoCobranca, r.MetodosPagamento, r.TemTaxaInscricao, r.ValorTaxaInscricao, r.MetodosPagamentoTaxaInscricao, r.AnosAcademicosDisponiveis, r.CursosDisponiveis, r.DocumentoObrigatorio, r.DocumentoInstrucoes, r.DetalhesPersonalizados, id); e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
```

### 7.7 `AtualizarServicoExtra` — mesma coisa, só quando o campo foi informado

**Localizar:**
```go
	var detalhes map[string]interface{}
	if r.informado["detalhes_personalizados"] {
		detalhes = r.DetalhesPersonalizados
	}
	e := s.Atualizar(cond(r, "nome", r.Nome), cond(r, "descricao", r.Descricao), cond(r, "categoria", r.Categoria), cond(r, "pago", r.Pago), cond(r, "preco", r.Preco), cond(r, "tipo_cobranca", r.TipoCobranca), cond(r, "metodos_pagamento", r.MetodosPagamento), cond(r, "tem_taxa_inscricao", r.TemTaxaInscricao), cond(r, "valor_taxa_inscricao", r.ValorTaxaInscricao), cond(r, "metodos_pagamento_taxa_inscricao", r.MetodosPagamentoTaxaInscricao), cond(r, "anos_academicos_disponiveis", r.AnosAcademicosDisponiveis), cond(r, "documento_obrigatorio", r.DocumentoObrigatorio), cond(r, "documento_instrucoes", r.DocumentoInstrucoes), detalhes, id)
```
**Substituir por:**
```go
	if r.informado["cursos_disponiveis"] {
		if e := validarPosseCursosDisponiveis(c, s.CodigoAcademia, r.CursosDisponiveis); e != nil {
			utils.RespondWithValidationError(c, e)
			return
		}
	}
	var detalhes map[string]interface{}
	if r.informado["detalhes_personalizados"] {
		detalhes = r.DetalhesPersonalizados
	}
	e := s.Atualizar(cond(r, "nome", r.Nome), cond(r, "descricao", r.Descricao), cond(r, "categoria", r.Categoria), cond(r, "pago", r.Pago), cond(r, "preco", r.Preco), cond(r, "tipo_cobranca", r.TipoCobranca), cond(r, "metodos_pagamento", r.MetodosPagamento), cond(r, "tem_taxa_inscricao", r.TemTaxaInscricao), cond(r, "valor_taxa_inscricao", r.ValorTaxaInscricao), cond(r, "metodos_pagamento_taxa_inscricao", r.MetodosPagamentoTaxaInscricao), cond(r, "anos_academicos_disponiveis", r.AnosAcademicosDisponiveis), cond(r, "cursos_disponiveis", r.CursosDisponiveis), cond(r, "documento_obrigatorio", r.DocumentoObrigatorio), cond(r, "documento_instrucoes", r.DocumentoInstrucoes), detalhes, id)
```

## 8. Projeção (`internal/projections/servico_extra_projection.go`)

### 8.1 DTO

**Localizar:**
```go
	AnosAcademicosDisponiveis     []string               `json:"anos_academicos_disponiveis"`
	DocumentoObrigatorio          bool                   `json:"documento_obrigatorio"`
```
**Substituir por:**
```go
	AnosAcademicosDisponiveis     []string               `json:"anos_academicos_disponiveis"`
	CursosDisponiveis             []string               `json:"cursos_disponiveis"`
	DocumentoObrigatorio          bool                   `json:"documento_obrigatorio"`
```

### 8.2 `created()` — struct de payload e INSERT

**Localizar:**
```go
		MetodosPagamentoTaxaInscricao, AnosAcademicosDisponiveis []string
		DocumentoObrigatorio                                     bool
```
**Substituir por:**
```go
		MetodosPagamentoTaxaInscricao, AnosAcademicosDisponiveis, CursosDisponiveis []string
		DocumentoObrigatorio                                                       bool
```

**Localizar:**
```go
	_, err := p.client.DB().Exec(`INSERT INTO projection_servicos_extras(id,codigo_academia,nome,descricao,categoria,pago,preco,tipo_cobranca,metodos_pagamento,tem_taxa_inscricao,valor_taxa_inscricao,metodos_pagamento_taxa_inscricao,anos_academicos_disponiveis,documento_obrigatorio,documento_instrucoes,detalhes_personalizados,ativo,criado_por,created_at,updated_at,version,last_event_id) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,NULLIF($15,''),$16,true,$17,$18,$18,$19,$20) ON CONFLICT(id) DO NOTHING`, e.AggregateID, x.CodigoAcademia, x.Nome, x.Descricao, x.Categoria, x.Pago, preco, tipo, pq.Array(x.MetodosPagamento), x.TemTaxaInscricao, taxa, pq.Array(x.MetodosPagamentoTaxaInscricao), pq.Array(x.AnosAcademicosDisponiveis), x.DocumentoObrigatorio, x.DocumentoInstrucoes, string(d), x.CriadoPor, x.CreatedAt, e.EventVersion, e.EventID)
```
**Substituir por:**
```go
	_, err := p.client.DB().Exec(`INSERT INTO projection_servicos_extras(id,codigo_academia,nome,descricao,categoria,pago,preco,tipo_cobranca,metodos_pagamento,tem_taxa_inscricao,valor_taxa_inscricao,metodos_pagamento_taxa_inscricao,anos_academicos_disponiveis,cursos_disponiveis,documento_obrigatorio,documento_instrucoes,detalhes_personalizados,ativo,criado_por,created_at,updated_at,version,last_event_id) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,NULLIF($16,''),$17,true,$18,$19,$19,$20,$21) ON CONFLICT(id) DO NOTHING`, e.AggregateID, x.CodigoAcademia, x.Nome, x.Descricao, x.Categoria, x.Pago, preco, tipo, pq.Array(x.MetodosPagamento), x.TemTaxaInscricao, taxa, pq.Array(x.MetodosPagamentoTaxaInscricao), pq.Array(x.AnosAcademicosDisponiveis), pq.Array(x.CursosDisponiveis), x.DocumentoObrigatorio, x.DocumentoInstrucoes, string(d), x.CriadoPor, x.CreatedAt, e.EventVersion, e.EventID)
```

### 8.3 `updated()` — mapa de campos

**Localizar:**
```go
	fields := map[string]string{"Nome": "nome", "Descricao": "descricao", "Categoria": "categoria", "Pago": "pago", "Preco": "preco", "TipoCobranca": "tipo_cobranca", "MetodosPagamento": "metodos_pagamento", "TemTaxaInscricao": "tem_taxa_inscricao", "ValorTaxaInscricao": "valor_taxa_inscricao", "MetodosPagamentoTaxaInscricao": "metodos_pagamento_taxa_inscricao", "AnosAcademicosDisponiveis": "anos_academicos_disponiveis", "DocumentoObrigatorio": "documento_obrigatorio", "DocumentoInstrucoes": "documento_instrucoes", "DetalhesPersonalizados": "detalhes_personalizados"}
```
**Substituir por:**
```go
	fields := map[string]string{"Nome": "nome", "Descricao": "descricao", "Categoria": "categoria", "Pago": "pago", "Preco": "preco", "TipoCobranca": "tipo_cobranca", "MetodosPagamento": "metodos_pagamento", "TemTaxaInscricao": "tem_taxa_inscricao", "ValorTaxaInscricao": "valor_taxa_inscricao", "MetodosPagamentoTaxaInscricao": "metodos_pagamento_taxa_inscricao", "AnosAcademicosDisponiveis": "anos_academicos_disponiveis", "CursosDisponiveis": "cursos_disponiveis", "DocumentoObrigatorio": "documento_obrigatorio", "DocumentoInstrucoes": "documento_instrucoes", "DetalhesPersonalizados": "detalhes_personalizados"}
```

### 8.4 `scan()` e `servicoCols`

**Localizar:**
```go
	err := row.Scan(&d.ID, &d.CodigoAcademia, &d.Nome, &d.Descricao, &d.Categoria, &d.Pago, &d.Preco, &d.TipoCobranca, pq.Array(&d.MetodosPagamento), &d.TemTaxaInscricao, &d.ValorTaxaInscricao, pq.Array(&d.MetodosPagamentoTaxaInscricao), pq.Array(&d.AnosAcademicosDisponiveis), &d.DocumentoObrigatorio, &d.DocumentoInstrucoes, &detalhes, &d.Ativo, &d.CreatedAt, &d.UpdatedAt)
```
**Substituir por:**
```go
	err := row.Scan(&d.ID, &d.CodigoAcademia, &d.Nome, &d.Descricao, &d.Categoria, &d.Pago, &d.Preco, &d.TipoCobranca, pq.Array(&d.MetodosPagamento), &d.TemTaxaInscricao, &d.ValorTaxaInscricao, pq.Array(&d.MetodosPagamentoTaxaInscricao), pq.Array(&d.AnosAcademicosDisponiveis), pq.Array(&d.CursosDisponiveis), &d.DocumentoObrigatorio, &d.DocumentoInstrucoes, &detalhes, &d.Ativo, &d.CreatedAt, &d.UpdatedAt)
```

**Localizar:**
```go
const servicoCols = `id,codigo_academia,nome,COALESCE(descricao,''),COALESCE(categoria,''),pago,preco,tipo_cobranca,metodos_pagamento,tem_taxa_inscricao,valor_taxa_inscricao,metodos_pagamento_taxa_inscricao,anos_academicos_disponiveis,documento_obrigatorio,COALESCE(documento_instrucoes,''),detalhes_personalizados,ativo,created_at,updated_at`
```
**Substituir por:**
```go
const servicoCols = `id,codigo_academia,nome,COALESCE(descricao,''),COALESCE(categoria,''),pago,preco,tipo_cobranca,metodos_pagamento,tem_taxa_inscricao,valor_taxa_inscricao,metodos_pagamento_taxa_inscricao,anos_academicos_disponiveis,cursos_disponiveis,documento_obrigatorio,COALESCE(documento_instrucoes,''),detalhes_personalizados,ativo,created_at,updated_at`
```

## 9. Aplicar a elegibilidade em `SolicitarServicoExtra` (`internal/handlers/servico_extra_solicitacao_handlers.go`)

**Localizar:**
```go
	serv, e := getServicosExtrasProjection(c).GetByID(sid)
	if e != nil || serv == nil || !serv.Ativo || serv.CodigoAcademia != *est.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "serviço extra indisponível")
		return
	}
	active, e := getSolicitacoesServicoExtraProjection(c).ExisteAtiva(sid, est.CodigoEstudante)
```
**Substituir por:**
```go
	serv, e := getServicosExtrasProjection(c).GetByID(sid)
	if e != nil || serv == nil || !serv.Ativo || serv.CodigoAcademia != *est.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "serviço extra indisponível")
		return
	}
	if len(serv.AnosAcademicosDisponiveis) > 0 || len(serv.CursosDisponiveis) > 0 {
		if !estudanteElegivelServicoExtra(serv, est) {
			utils.RespondWithForbiddenError(c, "este serviço não está disponível para o seu ano/curso atual")
			return
		}
	}
	active, e := getSolicitacoesServicoExtraProjection(c).ExisteAtiva(sid, est.CodigoEstudante)
```

`estudanteElegivelServicoExtra` já foi definida na seção 7.5, no mesmo pacote `handlers` — não precisa duplicar.

## 10. O que já foi validado (Claude/orquestrador) e o que falta validar (Codex)

### 10.1 Já validado
- Migration 121 aplicada com sucesso sobre as 128 migrations reais existentes, em base limpa (seção 0).
- Round-trip do formato `"<curso_id>|<ano_academico>"` em `cursos_disponiveis TEXT[]`, inserido e lido de volta corretamente via SQL puro (equivalente ao que `pq.Array()` faz em Go).
- Os dois `CHECK constraints` existentes na tabela continuam satisfeitos com a coluna nova presente.
- Conferi manualmente, lendo o código-fonte atual: `SolicitarServicoExtra` hoje **não** faz nenhuma checagem de ano/curso — a mudança da seção 9 é estritamente aditiva a um caminho hoje sem essa validação, não pode quebrar nenhum teste existente que dependa da ausência dela (não deveria haver teste que dependa disso, mas confira).

### 10.2 Testes unitários obrigatórios (aggregate)
- As 7 chamadas existentes ajustadas (seção 6.10) continuam passando.
- `TestServicoExtraCursosDisponiveisValidation` (seção 6.10) — 5 casos.

### 10.3 Teste de integração recomendado (requer Postgres real)
Se o seu ambiente tiver `RUN_POSTGRES_INTEGRATION=1` disponível (mesma convenção das demais suítes de integração do repositório, ex. `internal/handlers/financeiro_handlers_integration_test.go`), adicione um teste cobrindo:
1. `CriarServicoExtra` com `cursos_disponiveis` referenciando um curso de **outra** academia → rejeitado.
2. `CriarServicoExtra` com `cursos_disponiveis` referenciando um ano que não está em `curso.AnosAcademicos` → rejeitado.
3. `CriarServicoExtra` com `cursos_disponiveis` referenciando um curso `type="medio"` mas ano terminando em `_ano_superior` (ou vice-versa) → rejeitado.
4. `SolicitarServicoExtra`: estudante com `curso_medio_id`/`ano_escolar_medio` batendo com uma entrada de `cursos_disponiveis` → aceito. Estudante com ano certo mas curso errado → rejeitado (403). Estudante fundamental batendo com `anos_academicos_disponiveis` → aceito, mesmo com `cursos_disponiveis` não-vazio no mesmo serviço (a combinação da decisão de design 2).

Se não tiver Postgres disponível no seu ambiente, pule este teste e documente isso no relatório final — não invente mock de banco para contornar (mesma orientação já herdada de tarefas anteriores).

## 11. Atualização da documentação de API

Em `Documentação da API.md`, seção `## 20. Serviços Extras`:

**Localizar:**
```
### 20.1 Criar serviço extra
**Proteção:** academia autenticada e ativa. `POST /academia/servicos-extras`.

**Request body:** `nome` (obrigatório), `descricao`, `categoria`, `pago`, `preco`, `tipo_cobranca` (`unico` ou `mensal`), `metodos_pagamento` (`GPO`, `REF`, `GPO_QR`), `tem_taxa_inscricao`, `valor_taxa_inscricao`, `metodos_pagamento_taxa_inscricao`, `anos_academicos_disponiveis`, `documento_obrigatorio`, `documento_instrucoes` e `detalhes_personalizados`.

**Regras de negócio:** campos financeiros são obrigatórios apenas quando a respectiva cobrança estiver ativa; lista de anos vazia disponibiliza o serviço para todos os anos.
```
**Substituir por:**
```
### 20.1 Criar serviço extra
**Proteção:** academia autenticada e ativa. `POST /academia/servicos-extras`.

**Request body:** `nome` (obrigatório), `descricao`, `categoria`, `pago`, `preco`, `tipo_cobranca` (`unico` ou `mensal`), `metodos_pagamento` (`GPO`, `REF`, `GPO_QR`), `tem_taxa_inscricao`, `valor_taxa_inscricao`, `metodos_pagamento_taxa_inscricao`, `anos_academicos_disponiveis`, `cursos_disponiveis`, `documento_obrigatorio`, `documento_instrucoes` e `detalhes_personalizados`.

**Regras de negócio:** campos financeiros são obrigatórios apenas quando a respectiva cobrança estiver ativa; `anos_academicos_disponiveis` e `cursos_disponiveis` vazios (ambos) disponibilizam o serviço para todos os anos/cursos. `anos_academicos_disponiveis` aceita anos soltos (`N_ano_fundamental`/`N_ano_medio`/`N_ano_superior`) sem vínculo com curso — para fundamental é o único formato possível, já que este sistema não tem cursos de fundamental; para médio/superior um ano solto aqui vale para qualquer curso da academia (compatibilidade com serviços criados antes desta funcionalidade). `cursos_disponiveis` restringe a um curso específico: cada item é `"<curso_id>|<ano_academico>"`, com `ano_academico` terminando em `_ano_medio` ou `_ano_superior` — o curso precisa pertencer à mesma academia, não estar deletado, ter o tipo (`medio`/`superior`) correspondente ao sufixo do ano, e o ano precisa fazer parte dos anos acadêmicos do curso. As duas listas são combináveis no mesmo serviço (ex.: fundamental solto + um curso médio específico). A partir desta funcionalidade, `POST /estudante/servicos-extras/:id/solicitacao` passa a **aplicar de fato** essa elegibilidade: se o serviço tiver qualquer restrição, o estudante só consegue se inscrever se o ano/curso atual dele bater com uma das duas listas — caso contrário recebe `403`.
```

**Localizar:**
```
### 20.2 Atualizar serviço extra
**Proteção:** academia proprietária autenticada e ativa. `PUT /academia/servicos-extras/:id`. Aceita os mesmos campos da criação de forma parcial.
```
**Substituir por:**
```
### 20.2 Atualizar serviço extra
**Proteção:** academia proprietária autenticada e ativa. `PUT /academia/servicos-extras/:id`. Aceita os mesmos campos da criação (incluindo `cursos_disponiveis`) de forma parcial.
```

## 12. Checklist de aceite

- [ ] Migration 121 criada e aplicada sem erro (já validado por mim — só confirme no seu ambiente também).
- [ ] `ServicoExtra.CursosDisponiveis` presente no aggregate, nos dois eventos, em `Criar`/`Atualizar`/`applyCriado`/`applyAtualizado`.
- [ ] `validarCursosDisponiveisServicoExtra` (formato) implementado e coberto pelos 5 casos de teste da seção 6.10.
- [ ] `validarPosseCursosDisponiveis` (posse + consistência com o curso real) chamado em `CriarServicoExtra` e em `AtualizarServicoExtra` (só quando informado).
- [ ] `estudanteElegivelServicoExtra` aplicado em `SolicitarServicoExtra`, só quando o serviço tem alguma restrição.
- [ ] Projeção (`created`, `updated`, `scan`, `servicoCols`) e handler (`servicoExtraPayload`, `allowed`, `servicoExtraToJSON`) todos com `cursos_disponiveis`.
- [ ] Nenhuma alteração em `internal/db/safe_queries.go` (não é necessária — decisão de design 8).
- [ ] Nenhuma linha existente de `anos_academicos_disponiveis` foi migrada ou reinterpretada.
- [ ] `go build ./...`, `go vet ./...`, `gofmt -l .` sem erros.
- [ ] `go test -p 1 ./...` sem falhas (ou falhas documentadas, se algum teste de integração precisar de Postgres indisponível no seu ambiente).
- [ ] Documentação da API atualizada (seção 11).

## Procedimento de conclusão

Ao terminar, rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test -p 1 ./...`, corrija qualquer erro, e relate: o que passou, o que não pôde ser testado (ex. teste de integração de Postgres se `RUN_POSTGRES_INTEGRATION` não estava disponível) e por quê. Confirme explicitamente que as 7 chamadas existentes de `.Criar(`/`.Atualizar(` no pacote `aggregates` foram todas ajustadas para a nova assinatura — é o ponto mais fácil de esquecer uma ocorrência nesta tarefa.
