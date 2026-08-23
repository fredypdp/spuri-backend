# Manual do Módulo de Avaliação Final — Ensino Superior (Spuri)

> Baseado em análise directa do código-fonte de `fredypdp/spuri-backend` (branch `main`): agregados (`internal/domain/aggregates`), projeções (`internal/projections`), handlers (`internal/handlers`), migrações (`migrations/`) e `Documentação da API.md`. Todas as referências de ficheiro/função citadas neste manual foram confirmadas no código, não assumidas.
>
> Este manual cobre **apenas o ensino superior**. O modelo avaliativo de escolas (fundamental/médio) é fixo do sistema (`regraAvaliacaoFinalEscolarFixa`, em `internal/handlers/modelo_avaliativo_escolar.go`) e não é configurável pela academia — é mencionado só como contraste, quando ajuda a entender uma decisão de desenho do superior.

---

## Índice

1. [Como funciona](#1-como-funciona)
2. [Como configurar — passo a passo](#2-como-configurar--passo-a-passo)
   - [2.1 Configuração da fórmula](#21-configuração-da-fórmula-regra-de-avaliação-final)
   - [2.2 Configuração das categorias de nota](#22-configuração-das-categorias-de-nota)
   - [2.3 Matérias pendentes/em atraso](#23-matérias-pendentesem-atraso)
3. [Recomendações de simplificação e versatilidade](#3-recomendações-de-simplificação-e-versatilidade)

---

## 1. Como funciona

### 1.1 Arquitectura em uma frase

O módulo é **Event Sourcing/CQRS puro**: um comando no agregado `Estudante` (`RegistrarAvaliacaoFinal`, em `internal/domain/aggregates/estudante_avaliacao.go`) gera um evento imutável (`AvaliacaoFinalEscolar` ou `AvaliacaoFinalSuperior`) que é gravado no ledger append-only (`spuri_ledger`) e depois projectado para tabelas de leitura (`projection_avaliacao_final`, `projection_materias_pendentes`) pelo `AvaliacaoFinalProjection` (`internal/projections/avaliacao_final_projection.go`). Nada na avaliação final é calculado directamente contra o banco de leitura — o cálculo acontece nos *handlers*, e só o resultado final vira evento.

### 1.2 Não existe botão "avaliar agora" — é 100% orientado a evento de nota

Este é o ponto mais importante para qualquer pessoa a configurar o sistema: **a avaliação final do superior não é disparada manualmente**. Ela é despoletada automaticamente sempre que uma nota é lançada (`POST /academia/notas-aluno`, handler `RegistrarNota` em `internal/handlers/notas_handlers.go`) ou corrigida (`PATCH /academia/notas-aluno/:id`, handler `CorrigirNota`). Ambos chamam, no fim, `tentarAvaliacoesFinaisAutomaticas` (`internal/handlers/avaliacao_final_handler.go:296`).

> **Nota de auditoria de código:** existe um handler `RegistrarAvaliacaoFinal` (`avaliacao_final_handler.go:25`) que regista avaliação final "manualmente", mais um `RegistrarAvaliacaoFinalBatch` e `RegistrarAvaliacaoFinalBatchAsync`. Verificámos `cmd/server/main.go` linha a linha: **nenhuma destas três rotas está registada no router**, e o `JobType` associado (`registrar_avaliacao_final_batch`) não tem *worker* que o processe. Ou seja, este caminho é código morto do ponto de vista da API pública — confirmado também pela própria documentação oficial ("Não existe rota pública/registrada para executar avaliação final manualmente", `Documentação da API.md`, secção "Execução automática da avaliação final"). Trate a secção [3.1](#31-remover-ou-religar-o-caminho-manual-morto) como leitura obrigatória antes de decidir manter esse código.

### 1.3 O fluxo completo, passo a passo

1. A academia lança uma nota (`POST /academia/notas-aluno`) para um estudante, matéria e categoria específicas.
2. O backend infere o nível de ensino do estudante (`inferirTipoEnsinoDoEstudante`, `avaliacao_final_handler.go:915`): superior tem prioridade se o estudante tiver `curso_superior_id`, `ano_superior` ou `status_superior="em_andamento"`.
3. Para superior, o `semestre_atual` do estudante é convertido no período `"[n]_semestre"` (`periodoSuperiorAtual`, `avaliacao_final_handler.go:1541`) e validado contra `curso.periodos`.
4. O backend busca as regras activas aplicáveis àquele nível/escopo (`listarRegrasAvaliacaoFinalAplicaveis`, `avaliacao_final_regras.go:953`). Para superior, isto lê `projection_regras_avaliacao_final` filtrando por `codigo_academia`, `nivel='superior'`, `status='ativo'`.
5. A cadeia de regras precisa ter **exactamente uma raiz** (regra sem `aplica_se_reprovado_em_type`) — `validarCadeiaAvaliacaoFinalAplicavel`, `avaliacao_final_handler.go:741`. Se houver zero ou mais de uma raiz aplicável, a operação falha com erro (a nota em si é gravada; só a avaliação automática é abortada).
6. O disparo só acontece se a categoria da nota lançada for exactamente a `nota_despertadora` da regra raiz (`regraDespertadaPorNota`, `avaliacao_final_handler.go:454`) — **e** corresponder ao período de fecho dessa categoria dentro da fórmula (`periodoFechamentoCategoriaNaFormula`). Isto existe para impedir que um lançamento de nota "no meio do semestre" feche a avaliação antes da hora.
7. Uma vez disparada, o backend resolve **todas** as matérias aplicáveis àquele semestre/curso (`materiasAplicaveisAvaliacaoFinal`, `avaliacao_final_handler.go:724`) e calcula uma `nota_final` **por matéria**, não uma média global do estudante (`calcularResultadoMateriasAvaliacaoFinal`, `avaliacao_final_handler.go:766`).
8. Qualquer referência da fórmula sem nota lançada, em qualquer matéria aplicável, é substituída por `0` nesse momento (`substituirNotasAusentesPorZero`) — e fica registada em `resultados_materias[].notas_substituidas_zero` para auditoria. Isto é intencional: evita que o semestre fique pendente indefinidamente por uma nota nunca lançada.
9. O evento `AvaliacaoFinalSuperior` é gravado com *snapshot* completo: fórmula usada, regra usada, resultado por matéria, progressão de semestre — nada é recalculado a partir do estado actual em consultas futuras.
10. A projecção grava a linha em `projection_avaliacao_final` e, se houver pendência aprovada, grava também em `projection_materias_pendentes` (`registrarPendenciasGeradas`, `avaliacao_final_projection.go:166`).

### 1.4 Cadeia de regras: raiz + descendentes

Uma "cadeia" de avaliação final é composta por:

- **Uma regra raiz** — sem `aplica_se_reprovado_em_type`. É a primeira tentativa (ex.: `type="avaliacao_final"`).
- **Zero ou mais regras descendentes** — cada uma com `aplica_se_reprovado_em_type` a apontar para o `type` anterior (ex.: `avaliacao_final_com_recurso` depende de `avaliacao_final`).

Regras de negócio validadas no código (`avaliacao_final_regras.go`):

| Regra | Onde está validada |
|---|---|
| Só pode haver **uma raiz activa** por academia/nível (superior não tem escopo por ano) | `validarRaizUnicaRegraAvaliacaoFinal` |
| Uma descendente não pode apontar para si mesma, nem criar ciclo | `validarDependenciaRegraAvaliacaoFinal` |
| `nota_despertadora` só é aceite na raiz; descendentes são rejeitadas se enviarem o campo | `validarNotaDespertadoraRegraAvaliacaoFinal` |
| Uma descendente só executa depois de reprovação confirmada no `type` de que depende | `regraPodeExecutarAutomaticamente` |
| Ao inactivar uma regra, todas as dependentes directas/indirectas são inactivadas em cascata | `idsCadeiaDependenteRegraAvaliacaoFinal`, chamado em `DeletarRegraAvaliacaoFinal` |

Execução da cadeia em runtime (`tentarAvaliacoesFinaisAutomaticas`): o motor tenta repetidamente todas as regras "despertadas" até nenhuma progredir (`for { avancou := false; ... }`), permitindo que uma raiz reprovada desperte imediatamente a descendente seguinte no mesmo lançamento de nota, sem esperar por um novo request.

### 1.5 Cálculo por matéria — o núcleo do motor

Ao contrário do fluxo escolar (que também é por matéria, mas com fórmula fixa), o superior é **inteiramente por matéria e configurável**:

- Para cada matéria aplicável, a fórmula da regra é reescrita substituindo o período implícito pelo período real da matéria (`preencherPeriodoFormulaSuperior`) — por isso, na fórmula de uma regra superior, escreve-se `[categoria]` e nunca `[categoria,periodo]`.
- As notas são carregadas filtradas por `materia_disciplinar_id` (`carregarNotasFormulaMateria`), nunca uma massa única de notas do estudante.
- `nota_final` da matéria = resultado da fórmula. `aprovado` = `nota_final >= nota_minima_aprovacao`.
- A `nota_final` do evento de avaliação final (aquela que aparece em `projection_avaliacao_final.nota_final`) é a **média aritmética simples** dos resultados de todas as matérias avaliadas (`soma / float64(len(resultados))`, `avaliacao_final_handler.go:866`) — não é ponderada por créditos/carga horária, porque o sistema actualmente não modela créditos.

### 1.6 Aprovação, reprovação e aprovação com pendência

```
aprovado_geral := true (assume-se aprovado)
para cada matéria:
    se nota_final < nota_minima_aprovacao:
        aprovado_geral := false
        reprovadas++
        se materia.pendencia_permitida:
            candidatas_a_pendencia.append(materia)

se NÃO aprovado_geral E regra.limite_materias_pendentes != nil:
    se reprovadas <= limite_materias_pendentes  E  reprovadas == len(candidatas_a_pendencia):
        aprovado_geral := true
        aprovado_com_pendencia := true
        gera pendência para cada matéria candidata
```

Ponto-chave, muitas vezes mal-entendido: a aprovação com pendência **só acontece se todas as matérias reprovadas** permitirem pendência (`materia.pendencia_permitida = true`). Basta **uma única matéria reprovada sem `pendencia_permitida`** para que a aprovação com pendência seja recusada e o estudante fique reprovado (mesmo que o número total de reprovações esteja dentro do limite). Isto está implementado em `calcularResultadoMateriasAvaliacaoFinal`, `avaliacao_final_handler.go:854`:

```go
if !aprovado && tipoEnsino != "fundamental" && regra.LimiteMateriasPendentes != nil &&
   reprovadas <= *regra.LimiteMateriasPendentes && reprovadas == len(reprovadasPendenciaveis) {
```

### 1.7 Progressão semestral

A progressão do superior **não** usa "ano académico" como fundamental/médio — usa `semestre_atual` (inteiro) no estudante:

- Aprovação num semestre intermédio: `semestre_atual++`, e `ano_superior = ceil(semestre_atual / 2)` é recalculado (`calcularProximoSemestreCurso`, `avaliacao_final_handler.go:1569`).
- Aprovação no último semestre do curso: `status_superior` passa a `"finalizado"`, `semestre_atual` não avança mais.
- Reprovação (sem pendência): `semestre_atual`, `ano_superior` e `status_superior` **não mudam**. O estudante repete o mesmo semestre.
- O cliente (frontend) **nunca envia** `proximo_ano_academico`/`proximo_semestre_atual`: é sempre calculado no backend a partir do estado actual do estudante e da lista `curso.periodos`.

### 1.8 Idempotência e auditoria

Cada avaliação final é idempotente pela chave `(codigo_estudante, codigo_academia, ano_lectivo, tipo_ensino, ano_academico_atual, type)` — reforçada tanto no agregado (`chaveAvaliacaoNivel`, `estudante_avaliacao.go:88`, guardada em `Estudante.AvaliacoesPorAno`) como na projecção (índice único `uq_avaliacao_final_estudante_ano_tipo`, migração `070_avaliacao_final_regras_formula.sql`). Isto permite que `1_semestre` e `2_semestre` do mesmo ano lectivo não se bloqueiem mutuamente — cada semestre é uma chave distinta.

Cada evento grava um *snapshot* completo (fórmula usada, regra usada, resultados por matéria) para que alterações futuras na regra/matéria/nota **não reescrevam silenciosamente** decisões já tomadas (`Documentação da API.md`, secção 15.1.10).

### 1.9 Onde cada peça vive no código

| Peça | Ficheiro |
|---|---|
| Comando/evento `RegistrarAvaliacaoFinal` no agregado `Estudante` | `internal/domain/aggregates/estudante_avaliacao.go` |
| Motor de disparo automático e cálculo por matéria | `internal/handlers/avaliacao_final_handler.go` |
| CRUD de regras + parser/validador de fórmula | `internal/handlers/avaliacao_final_regras.go` |
| CRUD de categorias de nota | `internal/handlers/notas_handlers.go` |
| Agregado `MateriaDisciplinar` (`pendencia_permitida`, `pendencia_nivel_conclusao`) | `internal/domain/aggregates/materia_disciplinar.go` |
| Agregado `Curso` (períodos/semestres do superior) | `internal/domain/aggregates/curso.go` |
| Projecção de leitura + criação de `projection_materias_pendentes` | `internal/projections/avaliacao_final_projection.go` |
| Whitelist de eventos permitidos no ledger | `internal/db/safe_queries.go` |
| Rotas HTTP registadas | `cmd/server/main.go` |

---

## 2. Como configurar — passo a passo

Toda a configuração abaixo assume uma **academia autenticada com `nivel="superior"`**. Escolas recebem erro de validação em todas estas rotas — o backend força isso explicitamente (`preencherValidarNivelRegraAcademia`, `avaliacao_final_regras.go:245`).

A ordem importa porque cada passo valida contra o estado criado no passo anterior:

```
Curso (períodos)  →  Matérias (por período)  →  Categorias de nota  →  Regra de avaliação final (fórmula)
```

### Passo 0 — Pré-requisito: curso com períodos (semestres)

Antes de tudo, o curso superior precisa de `periodos` definidos. A academia **não** envia `anos_academicos` para curso superior — é proibido (`prepararDadosCursoPorTipo`, `internal/handlers/cursos_handlers.go:91`, retorna erro se `anos_academicos` for enviado). Em vez disso, envia-se apenas a **quantidade de semestres**:

```json
POST /academia/cursos
{
  "nome": "Licenciatura em Engenharia Informática",
  "type": "superior",
  "periodos": 8
}
```

O backend deriva automaticamente (`derivarCursoSuperior`, `cursos_handlers.go:165`):

- `periodos = ["1_semestre", "2_semestre", ..., "8_semestre"]`
- `anos_academicos = ["1_ano_superior", "2_ano_superior", "3_ano_superior", "4_ano_superior"]` (fórmula: `ceil(total_periodos / 2)`)

**Regra de negócio:** um curso com número ímpar de semestres (ex.: 7) arredonda para cima — 7 semestres geram 4 "anos superiores", com o último ano tendo apenas 1 semestre. Isto é aritmética pura em `derivarCursoSuperior`, não configurável por academia.

### 2.1 Configuração da fórmula (regra de avaliação final)

#### 2.1.1 Ordem de dependências dentro deste passo

Uma regra de avaliação final depende de três coisas já existirem:

1. **Matérias superiores activas**, cada uma com `periodo` definido (ver 2.1.2 abaixo).
2. **Categorias de nota** já criadas (ver secção 2.2) — a fórmula só aceita categorias que já existam e estejam activas na academia.
3. Nenhuma regra raiz activa a mais — só pode existir **uma raiz activa** por academia superior.

#### 2.1.2 Sub-passo: criar as matérias do curso

```json
POST /academia/materia
{
  "nome": "Cálculo I",
  "type": "superior",
  "curso_id": "<uuid do curso>",
  "periodo": "1_semestre",
  "anos_academicos": ["1_ano_superior"],
  "pendencia_permitida": true
}
```

Campos e onde são validados (`internal/handlers/materia_disciplinar_handlers.go`):

| Campo | Obrigatório para superior | Regra de negócio |
|---|---|---|
| `periodo` | Sim | Deve pertencer a `curso.periodos` (linha 133). É o campo que **realmente** governa em que semestre a matéria é avaliada. |
| `anos_academicos` | Sim | Não pode ser vazio (`validarAnosAcademicosMateria`), mas **não é validado contra `curso.anos_academicos`** — ver [3.4](#34-unificar-o-escopo-duplo-das-matérias-superiores). Só é usado para (a) inferir o "ano académico" ao lançar notas e (b) casar com `anos_academicos` da categoria de nota. |
| `pendencia_permitida` | Opcional, default `true` para superior | Só existe para `type="superior"` — escolar/médio rejeitam o campo (`materia_disciplinar.go:172`). |
| `pendencia_nivel_conclusao` | Opcional | Deve ser um `"N_semestre"` pertencente a `curso.periodos` (`validarPendenciaNivelConclusao`). **Atenção:** ver secção 2.3 — este campo é aceite e guardado, mas actualmente não tem efeito nenhum na avaliação (nenhum código o lê fora da validação de criação/edição). |

> **Armadilha comum:** como `anos_academicos` da matéria não é cruzado com `curso.anos_academicos`, é possível criar uma matéria de "Cálculo I" (`periodo="1_semestre"`) com `anos_academicos=["2_ano_superior"]` por engano. Isto não bloqueia nada na criação, mas pode impedir o lançamento de notas mais tarde se nenhuma categoria de nota tiver `"2_ano_superior"` na sua lista de anos — o erro que aparece é indirecto ("categoria X não configurada para o ano Y"), não aponta para a matéria. Mantenha `anos_academicos` da matéria sempre coerente com `ceil(periodo/2)`.

#### 2.1.3 Sub-passo: escrever a fórmula

A fórmula é uma expressão textual, interpretada por um *parser* próprio (`formulaParser`, `avaliacao_final_regras.go:1046`) — **não há `eval`, não há JavaScript, não há template dinâmico.** Isto é uma escolha de segurança deliberada: uma fórmula maliciosa não consegue executar código, só aritmética sobre notas já existentes.

**Gramática para o nível superior** (diferente de fundamental/médio):

| Nível | Formato da referência | Porquê |
|---|---|---|
| Fundamental / Médio | `[categoria,periodo]` | Precisa do trimestre explícito porque uma matéria vive em vários trimestres simultaneamente. |
| **Superior** | `[categoria]` — **sem período** | O período é inferido automaticamente a partir do `periodo` da própria matéria avaliada (`preencherPeriodoFormulaSuperior`). Enviar `[categoria,periodo]` numa regra superior é **rejeitado** (`validarPeriodosFormulaPorNivel`, linha 1150: `"formula de nivel superior deve referenciar apenas [categoria]"`). |

Operadores: `+ - * /`. Precedência normal (`*`/`/` antes de `+`/`-`). Parênteses aceites. Constantes numéricas ≥ 0 (constantes negativas são rejeitadas — `validarASTFormula`). Divisão por zero é bloqueada tanto na validação (quando o divisor é uma constante literal `0`) como na execução (quando o divisor calculado dá zero). Limite de 1000 caracteres por fórmula (`maxFormulaAvaliacaoLen`).

Exemplos válidos para superior:

```
([prova_parcelar_1]+[prova_parcelar_2])/2
([prova]*0.4)+([trabalho]*0.3)+([participacao]*0.3)
[exame_final]
```

Exemplos inválidos (e o porquê):

```
[prova,1_semestre]        # período explícito não é aceite no superior
[categoria_inexistente]   # categoria tem de existir e estar activa na academia
[prova]/0                 # divisão por zero
```

#### 2.1.4 Sub-passo: criar a regra raiz

```json
POST /academia/avaliacao-final/regras
{
  "type": "avaliacao_final",
  "nome": "Avaliação final do semestre",
  "nivel": "superior",
  "nota_minima_aprovacao": 10,
  "formula": "([prova_parcelar_1]+[prova_parcelar_2])/2",
  "limite_materias_pendentes": 2,
  "nota_despertadora": "prova_parcelar_2"
}
```

Regras de negócio validadas, em ordem de execução no handler `CriarRegraAvaliacaoFinal` (`avaliacao_final_regras.go:121`):

1. `nivel` é forçado para `"superior"` pelo backend com base na academia autenticada — não é o cliente que decide (`preencherValidarNivelRegraAcademia`).
2. `anos_academicos` é **rejeitado** para superior (`validarCamposPorNivelRegraAvaliacaoFinal`) — o escopo é sempre "toda a academia superior"; a segmentação por semestre acontece via `periodo` da matéria, não da regra.
3. `limite_materias_pendentes` é **obrigatório** para superior e deve ser `>= 0`.
4. `categorias_envolvidas` é extraído automaticamente da fórmula — se você enviar o campo manualmente, tem de bater certo com o que o *parser* extraiu, senão é erro.
5. Cada categoria referenciada tem de estar activa na academia (ver secção 2.2). Como regras superiores nunca têm `anos_academicos`, a validação (`validarCategoriasRegraAvaliacaoFinal`) trata **todas** as categorias activas da academia como disponíveis — não há filtragem por ano aqui (mais detalhe em [2.2.3](#223-um-detalhe-que-confunde-anos_academicos-da-categoria-não-filtra-nada-no-superior)).
6. `nota_despertadora`, se enviado, tem de ser o `codigo` exacto de uma categoria activa da academia. Se omitido, a regra fica válida mas **nunca dispara sozinha** — só é útil como alvo de `aplica_se_reprovado_em_type` de outra regra, ou para execução futura caso o disparo automático seja acionado por outro caminho.
7. Só pode existir **uma raiz activa** por academia superior (`validarRaizUnicaRegraAvaliacaoFinal`) — tentar criar uma segunda raiz falha com `"já existe avaliação final raiz ativa"`.

#### 2.1.5 Sub-passo opcional: criar regras descendentes (recuperação/recurso/exame)

```json
POST /academia/avaliacao-final/regras
{
  "type": "avaliacao_final_com_recurso",
  "nome": "Recurso do semestre",
  "nivel": "superior",
  "nota_minima_aprovacao": 10,
  "formula": "[exame_recurso]",
  "limite_materias_pendentes": 2,
  "aplica_se_reprovado_em_type": "avaliacao_final",
  "materias_aplicaveis": [
    { "curso_id": "<uuid>", "ano_academico": "1_semestre", "materias": ["<uuid materia>"] }
  ]
}
```

Pontos de atenção específicos de descendentes:

- `nota_despertadora` é **rejeitado** numa descendente — descendentes são acionadas exclusivamente por reprovação na regra ancestral (`validarNotaDespertadoraRegraAvaliacaoFinal`).
- `materias_aplicaveis` é o mecanismo para restringir a descendente só às matérias que reprovaram na etapa anterior. Repare que o campo chama-se `ano_academico` no contrato JSON, mas para `nivel="superior"` o valor esperado é na verdade um **período/semestre** (ex.: `"1_semestre"`), porque `validarMateriaAplicavelEscopo` (linha 634) faz o *match* contra `materia.periodo`, não contra `materia.anos_academicos`. Isto está documentado explicitamente na `Documentação da API.md` ("com ano derivado dos semestres"), mas é fácil de errar se você generalizar mentalmente a partir do contrato de fundamental/médio.
- Não é obrigatório restringir por `materias_aplicaveis`: se omitido, a descendente recalcula todas as matérias aplicáveis ao semestre, mas o motor só efectivamente recalcula as que já estavam reprovadas na etapa anterior quando a regra é `Fixed` (escolar); para regras **configuráveis do superior**, `regra.Fixed` é sempre `false`, então este filtro automático por reprovação anterior (linhas 773–798 de `avaliacao_final_handler.go`) **não se aplica** — é por isso que, na prática, você deve sempre usar `materias_aplicaveis` numa descendente superior para não recalcular matérias já aprovadas.

> **Isto é uma pegadinha real do código, não uma opinião:** o bloco `if regra.Fixed && regra.AplicaSeReprovadoEmType != nil { ... filtra só matérias reprovadas ... }` em `calcularResultadoMateriasAvaliacaoFinal` só executa quando `regra.Fixed == true`. Regras criadas pela academia via `POST /academia/avaliacao-final/regras` nunca têm `Fixed=true` (esse campo só é preenchido nas regras fixas do sistema escolar, em `regraAvaliacaoFinalEscolarFixa`). Portanto, **uma descendente superior sem `materias_aplicaveis` recalcula todas as matérias do semestre outra vez**, incluindo as que já tinham sido aprovadas na raiz. Configure `materias_aplicaveis` sempre que criar uma descendente superior, apontando exactamente para as matérias reprovadas que você quer que a etapa de recurso cubra.

#### 2.1.6 Editar e remover regras

- `PUT /academia/avaliacao-final/regras/:id` só permite alterar `nome`, `descricao`, `nota_minima_aprovacao`, `formula` (e `nota_despertadora`, só em raiz). **Não é possível mudar `nivel`, `type` ou `aplica_se_reprovado_em_type`** — para mudar o desenho da cadeia, crie uma nova regra.
- `DELETE /academia/avaliacao-final/regras/:id` faz *soft delete* (`status='inativo'`) e **inactiva em cascata** todas as dependentes directas/indirectas, para não deixar uma descendente órfã apontando para um `type` morto.

### 2.2 Configuração das categorias de nota

#### 2.2.1 Por que categorias existem antes da fórmula

Uma categoria de nota é o vocabulário que a fórmula usa (`[prova_parcelar_1]`, `[trabalho]`, etc.). Só existe para o superior — escolas usam um catálogo fixo do sistema e não podem criar/editar/remover categorias (`CriarCategoriaNota`, `internal/handlers/notas_handlers.go:440`, rejeita academias `nivel != "superior"`).

```json
POST /academia/categorias-nota
{
  "codigo": "prova_parcelar_1",
  "nome": "Prova Parcelar 1",
  "descricao": "Primeira prova parcelar do semestre",
  "anos_academicos": ["1_ano_superior", "2_ano_superior"]
}
```

#### 2.2.2 Regras de negócio

| Regra | Onde |
|---|---|
| `codigo` é normalizado: minúsculas, espaços viram `_`, só letras/números/`_` são aceites | `normalizarCodigoCategoriaNota`, `academia_categorias_nota.go:114` |
| `codigo` tem de ser único **entre as categorias activas** da mesma academia (verificado tanto no estado do agregado quanto na projecção, para cobrir *replay*/condição de corrida parcial) | `AdicionarCategoriaNota`, linhas 45–54 |
| `anos_academicos` é obrigatório e não pode conter valores vazios | `AdicionarCategoriaNota`, linhas 37–44 |
| Remoção é *soft delete* — emite `CategoriaNotaRemovida`; a categoria some das consultas activas mas o `codigo` histórico permanece em notas já lançadas | `RemoverCategoriaNota` |

#### 2.2.3 Um detalhe que confunde: `anos_academicos` da categoria não filtra nada no superior (no contexto de regras)

Isto é uma inconsistência real de comportamento entre dois pontos do código que vale a pena entender antes de configurar:

- **Ao lançar uma nota**, o backend *filtra* as categorias disponíveis pelo "ano académico" inferido do estudante/matéria (`carregarCategoriasDisponiveisParaAno`, `notas_handlers.go:770`) — aqui, `anos_academicos` da categoria **importa** e é comparado por igualdade de string.
- **Ao validar a fórmula de uma regra de avaliação final superior**, a função `validarCategoriasRegraAvaliacaoFinal` (`avaliacao_final_regras.go:1053`) recebe `anosAcademicos` da própria regra — que para superior é **sempre vazio**, porque regras superiores não aceitam esse campo (secção 2.1.4, ponto 2). Quando `len(anos) == 0`, o código cai neste ramo:

  ```go
  for _, cat := range categoriasAcademia {
      if len(anos) == 0 {
          disponiveis[cat.Codigo] = true   // TODAS as categorias contam como disponíveis
          continue
      }
      ...
  }
  ```

  Ou seja, **para efeitos de validação de fórmula de regra superior, `anos_academicos` da categoria é ignorado por completo** — qualquer categoria activa da academia pode entrar em qualquer fórmula superior, independentemente do que está declarado em `anos_academicos`.

**Implicação prática:** `anos_academicos` de uma categoria de nota superior só tem efeito real no momento de *lançar a nota* (que categorias aparecem disponíveis para aquele ano/semestre do estudante), não no momento de *configurar a fórmula*. Se você criar uma categoria com `anos_academicos: ["1_ano_superior"]` mas usá-la numa fórmula que deveria valer para todos os anos, a fórmula é aceite sem aviso — o problema só aparece semestres depois, quando um estudante do 3º ano tenta lançar nota nessa categoria e recebe erro porque ela "não está configurada para aquele ano". Trate `anos_academicos` da categoria como um campo que só a equipa que lança notas vê o efeito — não como um filtro de fórmula.

### 2.3 Matérias pendentes/em atraso

Esta é a área do módulo com a maior distância entre "o que o esquema de dados promete" e "o que o código realmente executa". Vale a pena a instituição de ensino superior configurar isto sabendo exactamente onde está o limite actual do sistema.

#### 2.3.1 O que existe e funciona hoje

1. **Configuração por matéria** (`pendencia_permitida`, `pendencia_nivel_conclusao`) — funciona, é validada e persistida (secção 2.1.2).
2. **Geração de pendência na avaliação final** — funciona: quando a aprovação com pendência acontece (secção 1.6), o backend grava uma linha em `projection_materias_pendentes` por matéria pendente, com protecção contra duplicidade (`uq_materia_pendente_aberta_escopo`, migração `079_avaliacao_final_nivel_materias_pendentes.sql`) — não é possível abrir duas pendências abertas para a mesma matéria/estudante/escopo.
3. **`limite_materias_pendentes`** é aplicado correctamente como tecto de quantas matérias podem gerar pendência numa única avaliação (secção 1.6).

#### 2.3.2 O que existe no esquema mas **não tem código a usá-lo**

Confirmado por busca exaustiva no repositório (`grep` por `projection_materias_pendentes`, `pendencia_nivel_conclusao`, `baixada_por_event_id` em todo `internal/` e `cmd/`):

- **Não existe nenhum endpoint para listar as matérias pendentes de um estudante.** A tabela `projection_materias_pendentes` só recebe `INSERT` (na avaliação final) e `DELETE` (no *rebuild* de projecção). Nenhum handler faz `SELECT` nela fora do *rebuild*. Uma instituição não tem, hoje, forma de perguntar ao sistema "que matérias este estudante tem pendentes?" via API.
- **Não existe fluxo de "baixa" (regularização) de pendência.** A coluna `baixada_por_event_id` existe na tabela desde a migração 079 (pensada precisamente para isto), mas nenhum código Go escreve nela. Quando um estudante refaz e passa a matéria pendente, **a pendência aberta permanece aberta para sempre** na projecção — não há evento nem handler que a feche.
- **`pendencia_nivel_conclusao` é validado na criação/edição da matéria, mas nunca lido depois disso.** Buscámos todas as ocorrências do campo fora dos ficheiros de validação de matéria (`materia_disciplinar_handlers.go`, `materia_disciplinar.go`, `materias_projection.go`) e não há nenhuma leitura dele em `avaliacao_final_handler.go` ou em qualquer lógica de conclusão de curso. Ou seja: **configurar `pendencia_nivel_conclusao` numa matéria não bloqueia nada hoje** — nem a conclusão do curso, nem a progressão de semestre. É um valor guardado, não um comportamento.

A própria `Documentação da API.md` reconhece esta lacuna nas secções 15.1.11 e 15.1.12, nomeando-a explicitamente como "limitação atual" — a análise de código aqui apenas confirma, linha por linha, que a documentação está correcta e que não há nenhum caminho escondido que implemente isto por fora.

#### 2.3.3 Como configurar hoje, com esta limitação em mente

Passo a passo, honesto quanto ao que cada campo realmente faz:

1. **Ao criar a matéria**, decida `pendencia_permitida`:
   - `true` (padrão para superior) → esta matéria pode ficar pendente se o estudante reprovar nela e o total de reprovações couber em `limite_materias_pendentes`.
   - `false` → reprovar nesta matéria **nunca** vira pendência; qualquer reprovação aqui reprova o semestre inteiro, mesmo que o `limite_materias_pendentes` da regra tivesse espaço de sobra. É o interruptor certo para matérias que a instituição considera "bloqueantes" (ex.: matérias de segurança/estágio obrigatório).
2. **Defina `limite_materias_pendentes`** na regra raiz pensando no cenário realista mais permissivo que a instituição aceita — como não há hoje mecanismo de baixa nem de bloqueio por `pendencia_nivel_conclusao`, um limite generosamente alto acumula pendências que **não são automaticamente cobradas de volta** em nenhum momento do fluxo actual.
3. **Não confie em `pendencia_nivel_conclusao` como controlo funcional ainda.** Preencha-o se quiser manter o dado para quando a regularização for implementada (ver secção 3.3), mas comunique à equipa académica que, por enquanto, ele é só metadado.
4. **Acompanhe pendências manualmente**, por agora, directamente na base de dados (`SELECT * FROM projection_materias_pendentes WHERE codigo_academia = $1 AND pendente = TRUE`) ou peça à equipa de desenvolvimento uma rota de consulta antes de depender operacionalmente deste mecanismo — ver recomendação [3.2](#32-fechar-o-ciclo-de-pendências-leitura--baixa).

---

## 3. Recomendações de simplificação e versatilidade

Cada recomendação abaixo está ancorada em código confirmado nesta análise — não é uma sugestão genérica de boas práticas. Estão ordenadas por impacto/urgência, não por esforço.

### 3.1 Remover ou religar o caminho manual morto

**Achado:** `RegistrarAvaliacaoFinal` (~260 linhas), `RegistrarAvaliacaoFinalBatch`, `RegistrarAvaliacaoFinalBatchAsync` e `jobs.JobTypeRegistrarAvaliacaoFinalBatch` não têm nenhuma rota registada em `cmd/server/main.go` nem *worker* que processe o `JobType`. É código morto do ponto de vista de qualquer cliente HTTP.

**Porquê importa para simplicidade:** é a maior função single-purpose do módulo (`avaliacao_final_handler.go:25`–`283`) e, por estar solta, cria risco de dois motores de avaliação divergentes no futuro — repare que este caminho manual calcula a nota com uma fórmula agregada sobre *todas* as notas do estudante (`carregarNotasFormula`, sem filtrar por matéria), enquanto o caminho automático real calcula por matéria (`carregarNotasFormulaMateria`). São semânticas diferentes para o mesmo conceito. Se algum dia esta rota for religada por engano (ex.: um "quick fix" que expõe `POST /academia/avaliacao-final` sem revisar a implementação), o resultado da avaliação vai divergir silenciosamente do motor automático.

**Recomendação concreta:** decidir entre (a) remover as ~300 linhas associadas (handler, batch, async batch, job type, rota comentada) se não há intenção de reactivar avaliação manual; ou (b) se a instituição quer mesmo um botão manual de "forçar avaliação", reescrever esse caminho para reutilizar `calcularResultadoMateriasAvaliacaoFinal` (o motor real, por matéria) em vez de manter uma segunda implementação paralela.

### 3.2 Fechar o ciclo de pendências (leitura + baixa)

**Achado:** secção [2.3.2](#232-o-que-existe-no-esquema-mas-não-tem-código-a-usá-lo) — escrita sem leitura, e sem regularização.

**Recomendação concreta, em duas partes que podem ser entregues separadamente:**

1. **Leitura (baixo risco, alto valor imediato):** adicionar `GET /academia/materias-pendentes` e `GET /estudante/materias-pendentes`, reaproveitando o índice já existente `idx_materias_pendentes_consulta (codigo_academia, codigo_estudante, nivel, pendente, curso_id)` — o índice já foi desenhado para este acesso, só falta o *handler*. Isto sozinho já resolve a lacuna operacional mais urgente sem tocar em lógica de negócio.
2. **Baixa (mais desenho necessário):** um evento novo (ex.: `MateriaPendenteBaixada`) emitido quando uma reavaliação daquela matéria específica aprova o estudante, escrevendo `baixada_por_event_id` e `pendente=FALSE`. Isto pode ser modelado como uma variante dirigida da avaliação por matéria já existente (`calcularResultadoMateriasAvaliacaoFinal` já sabe calcular o resultado de uma matéria isolada) — não precisa de um motor novo, precisa de um comando que, ao aprovar, feche a pendência correspondente em vez de (ou além de) gravar uma nova linha de avaliação final.

### 3.3 Decidir o destino de `pendencia_nivel_conclusao`: implementar o bloqueio ou remover o campo

**Achado:** campo validado e persistido, sem nenhuma leitura funcional (secção 2.3.2).

Um campo de configuração que a interface deixa a academia preencher, mas que não altera nenhum comportamento, é pior do que não ter o campo — cria uma falsa sensação de controlo. Há duas saídas igualmente válidas, e ambas são mais simples do que o estado actual (que combina "meio implementado" com "documentado como limitação"):

- **Implementar:** ao concluir o curso (evento de fim do último semestre com `aprovado=true`), verificar se existem pendências abertas cujo `pendencia_nivel_conclusao` seja `<=` o semestre de conclusão e, se houver, bloquear a finalização (`status_superior` permanece `"em_andamento"` com um motivo explícito, no mesmo padrão já usado para `motivo_progressao` no fundamental). É a mesma técnica que o código já usa para "aprovado mas sem oferta do próximo ano" — reaproveitar o padrão existente é mais barato do que inventar um novo.
- **Remover:** se a instituição não pretende implementar isto a curto prazo, remover o campo (migração + validações + DTO) reduz a superfície de configuração para exactamente o que o sistema faz hoje, evitando que uma academia configure algo que não tem efeito.

### 3.4 Unificar o escopo duplo das matérias superiores

**Achado:** secção 2.1.2 — uma matéria superior carrega simultaneamente `periodo` (semestre, o que **realmente** rege a avaliação) e `anos_academicos` (ano superior, usado só para notas/categorias), sem validação cruzada entre os dois nem contra `curso.anos_academicos`.

**Recomendação concreta:** já que `curso.anos_academicos` é sempre derivado deterministicamente de `curso.periodos` (`ceil(periodo_index/2)`), o backend pode **derivar `materia.anos_academicos` automaticamente a partir do `periodo` informado**, em vez de pedir à academia para preencher os dois campos manualmente e mantê-los coerentes por convenção. Isto elimina uma classe inteira de erro de configuração silenciosa (a "armadilha comum" descrita em 2.1.2) sem remover nenhuma funcionalidade — o campo passa a ser calculado, não digitado.

### 3.5 Proteger a criação de regras contra condição de corrida

**Achado:** `CriarRegraAvaliacaoFinal` valida unicidade de raiz (`validarRaizUnicaRegraAvaliacaoFinal`) e de `type` (`validarUnicidadeRegraAvaliacaoFinal`) com `SELECT`s de leitura antes do `INSERT`, sem usar o mecanismo `UniqueOperationGuard` que já existe no próprio repositório (`internal/db/unique_operation_guard.go`) e é usado noutros fluxos de escrita. A migração `071_regras_avaliacao_final_por_ano.sql` até documenta explicitamente que a restrição de unicidade a nível de banco foi removida em favor de validação a nível de aplicação, por causa do `JSONB` de `anos_academicos` — o que torna a janela de corrida ainda mais relevante para o superior (que hoje não usa `anos_academicos`, mas herda a mesma função de validação).

**Impacto se isto acontecer:** duas raízes activas simultâneas para a mesma academia superior fazem `validarCadeiaAvaliacaoFinalAplicavel` falhar em **toda** avaliação automática futura daquela academia (erro "mais de uma regra raiz de avaliação final encontrada"), até alguém detectar e inactivar uma delas manualmente — ou seja, o motor de avaliação inteiro pára silenciosamente para essa academia.

**Recomendação concreta:** envolver a validação + `INSERT` de `CriarRegraAvaliacaoFinal` numa reserva do `UniqueOperationGuard` com chave `("regra_avaliacao_final_raiz", codigo_academia+":superior")`, liberando a reserva após o commit. É reutilização de infraestrutura já existente, não uma peça nova.

### 3.6 Consolidar `validarFormulaAvaliacao` duplicada

**Achado:** `validarFormulaAvaliacao` (`avaliacao_final_regras.go:1163`) é uma cópia quase idêntica de `validarFormulaAvaliacaoPorNivel(nivel="fundamental", ...)` e **não é chamada por nenhum código de produção** — só pelos próprios testes (`avaliacao_final_formula_test.go`, `avaliacao_final_regras_test.go`).

**Recomendação concreta:** apagar `validarFormulaAvaliacao` e reescrever as chamadas de teste para `validarFormulaAvaliacaoPorNivel("fundamental", formula, categorias)`. Remove ~30 linhas de lógica duplicada sem perder cobertura de teste nenhuma.

### 3.7 Avisar (ou impedir) a remoção de categorias referenciadas por regras activas

**Achado:** `RemoverCategoriaNota` (`academia_categorias_nota.go:71`) não verifica se a categoria está referenciada em `formula` ou `nota_despertadora` de alguma regra activa antes de a remover. O efeito não é um erro imediato: como a categoria some da lista de categorias disponíveis para lançamento de notas, nenhuma nota nova pode ser lançada com aquele código, e a próxima vez que a fórmula for calculada, a referência é silenciosamente preenchida com `0` (mecanismo de "notas ausentes" da secção 1.3, passo 8) — arrastando a média de todos os estudantes para baixo sem aviso.

**Recomendação concreta:** antes de remover, consultar `projection_regras_avaliacao_final WHERE status='ativo' AND (categorias_envolvidas @> '["<codigo>"]' OR nota_despertadora = '<codigo>')`; se houver resultado, recusar a remoção com uma mensagem explícita ("categoria em uso pela regra X; edite ou inactive a regra primeiro"), ou pelo menos devolver essa lista como aviso no corpo da resposta para decisão consciente da academia.

### 3.8 Sincronizar a documentação sobre correcção de notas

**Achado, fora do escopo de avaliação final mas com impacto directo nela:** `Documentação da API.md`, secção 15.1.15, afirma que não existe endpoint para editar/corrigir notas já registadas. O código actual **tem** esse endpoint (`PATCH /academia/notas-aluno/:id`, handler `CorrigirNota`, `notas_handlers.go:257`), que já chama correctamente `tentarAvaliacoesFinaisAutomaticas` para recalcular a avaliação afectada. Isto não é um problema de comportamento — a implementação parece coerente com a regra que a própria secção 15.1.15 propõe para o futuro — é só a documentação que ficou desactualizada em relação ao código.

**Recomendação concreta:** ao tocar de novo neste módulo, actualizar a secção 15.1.15 para descrever o comportamento real de `CorrigirNota` (evento `NotaCorrigida`, já presente na whitelist do ledger em `internal/db/safe_queries.go`), evitando que a próxima pessoa (humana ou agente) a ler a documentação assuma incorrectamente que precisa de implementar este fluxo do zero.

---

## Resumo executivo das recomendações

| # | Recomendação | Tipo | Esforço relativo |
|---|---|---|---|
| 3.1 | Remover ou religar `RegistrarAvaliacaoFinal`/batch/async mortos | Simplificação (remoção) | Baixo |
| 3.2 | Endpoint de leitura de matérias pendentes + evento de baixa | Funcionalidade em falta | Médio |
| 3.3 | Implementar bloqueio por `pendencia_nivel_conclusao` ou remover o campo | Coerência configuração↔comportamento | Médio (implementar) / Baixo (remover) |
| 3.4 | Derivar `materia.anos_academicos` a partir de `periodo` | Simplificação (elimina classe de erro) | Baixo |
| 3.5 | `UniqueOperationGuard` na criação de regras | Robustez (corrige risco de corrida) | Baixo |
| 3.6 | Apagar `validarFormulaAvaliacao` duplicada | Simplificação (remoção) | Muito baixo |
| 3.7 | Bloquear/avisar remoção de categoria em uso | Robustez | Baixo |
| 3.8 | Actualizar documentação sobre `CorrigirNota` | Documentação | Muito baixo |

Em conjunto, 3.1 + 3.4 + 3.6 removem código sem remover nenhuma funcionalidade real hoje disponível — são as três primeiras candidatas naturais a um único task document para o Codex, por serem independentes entre si e não exigirem nenhuma decisão de produto prévia (ao contrário de 3.2 e 3.3, que exigem que decida, primeiro, se quer mesmo o fluxo de regularização de pendências como funcionalidade de curto prazo).
