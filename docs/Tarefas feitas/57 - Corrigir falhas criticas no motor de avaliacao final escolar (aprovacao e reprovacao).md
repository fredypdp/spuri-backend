---
criado: 2026-08-20 00:00
origem: Depuração solicitada pelo usuário e executada por Claude (Anthropic) em sandbox com PostgreSQL real
status: feito
prioridade: crítica
---

# Corrigir falhas críticas no motor de avaliação final escolar (aprovação e reprovação)

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Você (Codex) está rodando num ambiente que **bloqueia `apt` (403 Forbidden)** e **não tem Docker nem `psql`**. Isso significa que você **não consegue subir um PostgreSQL real** para reproduzir os cenários end-to-end descritos aqui.

Isso já foi feito por outra IA (Claude/Anthropic), num sandbox com acesso root e `apt` liberado, que:

1. Clonou o repositório, instalou PostgreSQL 16 e Go 1.24 (via `apt-get`, sem Docker).
2. Compilou o binário real do servidor (`cmd/server`) e o rodou contra o Postgres real, com as 115 migrations aplicadas automaticamente no boot (`internal/db/migrations.go`).
3. Rodou `go test ./...` completo — **todos os testes existentes passam** (isso é importante: os bugs abaixo não são pegos pela suíte atual porque nenhum teste existente cobre múltiplas matérias na mesma avaliação final, nem o cenário do 4º ano médio técnico).
4. Dirigiu chamadas HTTP reais contra o servidor real (bootstrap de admin → registro/ativação de academia → criação de cursos liceu e técnico → criação de matérias → registro de estudantes → lançamento de notas) para reproduzir cenários de aprovação/reprovação normais, com exame e com recurso, do 1º ano fundamental ao 9º, e do 1º ao 3º ano médio (liceu) / 1º ao 4º ano médio (técnico).
5. Confirmou os 3 achados críticos abaixo **empiricamente**, com respostas HTTP reais coladas neste documento como evidência, e também **por leitura estática do código-fonte** (para cada achado, os dois métodos concordam).

**O que isso significa na prática para você:**

- **Não tente instalar Postgres, Docker ou rodar migrations.** Você não precisa disso para implementar as correções abaixo.
- **Trate os achados 1, 2 e 3 como fatos confirmados**, não como hipóteses a re-verificar do zero. As evidências (requests/responses reais) estão coladas nas seções correspondentes.
- **Você pode e deve validar suas correções com o que já funciona no seu ambiente**: `go build ./...`, `go vet ./...`, `go test ./...`, e **novos testes unitários Go que não dependem de banco de dados** (várias das funções envolvidas são puras — ver seção 7).
- Para qualquer mudança que só possa ser validada com um Postgres real rodando (por exemplo, reproduzir de novo o cenário HTTP completo depois da correção), **não tente simular isso**. Em vez disso, documente claramente no `docs/Debbugs/...md` desta tarefa (ver seção 9) exatamente quais passos ficam **pendentes de validação com banco real**, para que Claude (ou um humano com acesso a um Postgres) rode depois. Não afirme "validado" para algo que você não conseguiu rodar de fato.
- Ao final, gere um `docs/Tarefas feitas/...md` e um `docs/Debbugs/...md` seguindo exatamente o padrão já usado neste repositório (veja exemplos em `docs/Tarefas feitas/` e `docs/Debbugs/`, em especial os arquivos sobre `nota_despertadora` e sobre "padronização de avaliação final diante de notas ausentes ou alteradas" — este último é o audit anterior desta mesma área, e você vai precisar reconciliar as conclusões dele com os achados novos abaixo, porque elas **não se contradizem, mas o audit anterior não teve cobertura suficiente para pegar o achado 2**).

---

## 1. Prompt recomendado para executar esta correção

Corrija os três defeitos críticos documentados neste arquivo no motor de avaliação final escolar (`internal/handlers/avaliacao_final_handler.go`, `internal/handlers/avaliacao_final_regras.go`, `internal/handlers/modelo_avaliativo_escolar.go`, `internal/domain/aggregates/materia_disciplinar.go`), na ordem de prioridade em que estão descritos (Achado 1 → Achado 2 → Achado 3), sem introduzir aliases, wrappers de compatibilidade ou fallbacks temporários para o comportamento antigo (errado). Ao corrigir o Achado 1 e o Achado 2, tenha cuidado redobrado porque as mesmas funções (`tiposAvaliacaoFinalDespertadosPorCategoria`, `calcularResultadoMateriasAvaliacaoFinal`, `regraPodeExecutarAutomaticamente`) são compartilhadas entre o modelo escolar fixo (fundamental/médio, foco desta tarefa) e o modelo configurável do Superior — qualquer correção precisa preservar o comportamento correto já existente do Superior (que tem sua própria noção de período/semestre por matéria). Depois de corrigir, rode `go build ./...`, `go vet ./...` e `go test ./...`, escreva novos testes unitários (sem banco de dados) para os pontos descritos na seção 7, e gere a documentação de tarefa concluída e de depuração seguindo o padrão de `docs/Tarefas feitas/` e `docs/Debbugs/`, deixando claro na documentação de depuração quais passos ficam pendentes de validação com um Postgres real (que você não tem acesso). Atualize também `Documentação da API.md`, seção 15 (Avaliações Finais), para refletir o comportamento corrigido.

---

## 2. Contexto e metodologia da auditoria

O usuário pediu uma depuração profunda do sistema de avaliação final (aprovação/reprovação) do **nível escolar** (fundamental 1ª–9ª classe, médio liceu 1º–3º ano, médio técnico 1º–4º ano), cobrindo os três tipos de avaliação (`normal`, `exame`/`exame_final` e `recurso`/`exame_recurso`).

Passos executados por Claude no sandbox:

1. `git clone https://github.com/fredypdp/spuri-backend.git`
2. `apt-get install -y postgresql postgresql-contrib golang-1.24-go` (ambiente root, sem sudo necessário).
3. `service postgresql start`, criação do banco `spuri_test`.
4. Compilação do módulo com `GOPROXY=direct` e `replace` locais em `go.mod` para os pacotes `golang.org/x/*` (que usam vanity import e não estavam acessíveis via proxy no sandbox), **apenas localmente, nunca commitados**. Build final 100% limpo (`go build ./...` sem erros).
5. `go test ./...` → **todos os pacotes passam** (`spuri/internal/handlers`, `spuri/internal/domain/aggregates`, `spuri/internal/projections`, etc.).
6. Execução do binário real (`cmd/server`) com `.env` local apontando para o Postgres real; todas as 115 migrations aplicadas automaticamente no boot.
7. Sequência de chamadas HTTP reais (documentadas com request/response completos nas seções de cada achado):
   - `POST /bootstrap` → admin FPP
   - `POST /admin/definir-ano-letivo-geral` (`type=escolar`)
   - `POST /academia/registo-publico` (`nivel_escolar=misto`, `anos_academicos` = todos os 9 anos fundamentais)
   - `PUT /dominis/academia/:codigo/ativar`
   - `POST /login` (academia)
   - `POST /academia/definir-ano-letivo`
   - `POST /academia/curso` (`modelo=liceu` e `modelo=tecnico`) — confirmado que o backend fixa corretamente `anos_academicos=[1,2,3]_ano_medio` para liceu e `[1,2,3,4]_ano_medio` para técnico (`internal/domain/aggregates/curso.go`, função que aplica `ModeloCursoMedioLiceu`/`ModeloCursoMedioTecnico`) — **este ponto está correto, não precisa de correção**.
   - `POST /academia/materia` (fundamental e médio)
   - `POST /academia/estudante/register` (com PDFs mínimos válidos para satisfazer a validação de documentos)
   - Ajuste direto do `ano_escolar_fundamental`/`ano_escolar_medio` do estudante via SQL na tabela de projeção `projection_estudantes` **apenas como atalho de setup de teste** (para não precisar recriar toda a cadeia de matrícula/documentos a cada ano testado) — isso não afeta a validade dos achados, porque a lógica de avaliação final lê o ano atual do estudante da projeção da mesma forma, independentemente de como ele chegou lá.
   - `POST /academia/notas-aluno` repetidamente, variando categoria/período/matéria, observando o campo `avaliacoes_finais_automaticas` de cada resposta e o estado final via `GET /avaliacoes-estudante/:codigo` e `GET /consultar-estudante/:codigo`.

---

## 3. Resumo executivo

| # | Achado | Severidade | Escopo afetado | Confirmação |
|---|---|---|---|---|
| 1 | O gatilho da avaliação final (`nota_despertadora`) casa apenas por **categoria**, ignorando o **período**. Qualquer lançamento de `prova_trimestral` (1º, 2º ou 3º trimestre) dispara a avaliação final "de fechamento", preenchendo com zero os trimestres ainda não lançados. | **Crítica** | Todos os anos com fórmula "sem exame" (1º–5º, 7º–8º fundamental; 1º–2º médio) — ou seja, a **maioria** dos anos escolares. Também pode afetar anos "com exame" se `exame_final` for lançado com período diferente de `3_trimestre`. | Confirmado empiricamente (seção 4) e por leitura estática (`avaliacao_final_handler.go:436-449`). |
| 2 | O cálculo de avaliação final, quando disparado automaticamente (o que **sempre** acontece em produção), filtra o conjunto de matérias avaliadas para **apenas a matéria que recebeu a nota que disparou o gatilho**. Como a chave de idempotência do evento de avaliação final é por `(estudante, ano_letivo, nível, ano_academico, tipo)` — **sem matéria** — a primeira matéria cujo gatilho dispara cria o único registro de avaliação final daquele ano/tipo e **bloqueia permanentemente** a avaliação de todas as outras matérias do mesmo ano. A decisão de aprovação/reprovação do aluno no ano inteiro passa a depender de uma única matéria arbitrária (a que por acaso foi lançada primeiro), e as demais nunca são avaliadas nem entram na conta. | **Crítica** | Qualquer ano/curso com mais de uma matéria (ou seja, praticamente todos os casos reais). | Confirmado empiricamente (seção 5) e por leitura estática (`avaliacao_final_handler.go:750-751` + `avaliacao_final_projection.go:630-635` + `avaliacao_final_handler.go:481-526`). |
| 3 | É **estruturalmente impossível** lançar nota (`nota_pap`) ou avaliar um estudante no **4º ano médio técnico** (Prova de Aptidão Profissional), porque a validação de matérias proíbe qualquer matéria de ter `4_ano_medio` em `anos_academicos`, mas o lançamento de nota exige exatamente essa correspondência para aceitar a nota. | **Crítica** | Especificamente o 4º ano médio dos cursos técnicos (o ano final do curso). | Confirmado empiricamente com 3 tentativas de contorno diferentes, todas bloqueadas (seção 6) e por leitura estática (`materia_disciplinar.go:209` + `notas_handlers.go`, função `inferirAnoAcademicoParaNota`). |
| 4 (menor, não bloqueante) | O endpoint `POST /academia/avaliacao-final` (handler `RegistrarAvaliacaoFinal`) e seu lote (`RegistrarAvaliacaoFinalBatch`) **não estão registrados em nenhuma rota** em `cmd/server/main.go` — são código morto, confirmado também pela própria `Documentação da API.md` ("Não existe rota pública/registrada para executar avaliação final manualmente"). Esse caminho morto também tem seu próprio bug independente (chama `regraAvaliacaoFinalEscolarFixa(..., nil, "")` com `modeloCursoMedio` vazio, então nunca resolveria regra para `4_ano_medio` técnico mesmo se fosse usado), mas como é inatingível em produção, isso não afeta usuários reais. | Baixa (cosmética/dívida técnica) | Nenhum em produção (código não roteado) | Confirmado por leitura estática. |

---

## 4. Achado 1 — Gatilho da avaliação final ignora o período

### 4.1 Causa raiz exata

Em `internal/handlers/avaliacao_final_handler.go`, a função que decide se uma nota recém-lançada deve disparar avaliação final:

```go
func tiposAvaliacaoFinalDespertadosPorCategoria(regras []regraAvaliacaoFinalDTO, raiz *regraAvaliacaoFinalDTO, categoriaAlterada string) map[string]bool {
	categoriaAlterada = strings.TrimSpace(categoriaAlterada)
	tiposDisparados := map[string]bool{}
	if raiz != nil && raiz.NotaDespertadora != nil && categoriaAlterada == strings.TrimSpace(*raiz.NotaDespertadora) {
		tiposDisparados[raiz.Type] = true
		return tiposDisparados
	}
	for _, regra := range regras {
		if regra.AplicaSeReprovadoEmType != nil && regra.NotaDespertadora != nil && categoriaAlterada == strings.TrimSpace(*regra.NotaDespertadora) {
			tiposDisparados[regra.Type] = true
		}
	}
	return tiposDisparados
}
```

Repare que a comparação é **só** `categoriaAlterada == *regra.NotaDespertadora` — nunca compara o **período** da nota que acabou de ser lançada. Para os anos "sem exame" (1º–5º, 7º–8º fundamental; 1º–2º médio), a fórmula fixa (`internal/handlers/modelo_avaliativo_escolar.go`) usa `nota_despertadora = "prova_trimestral"`, categoria que é legitimamente lançada **três vezes por ano** (uma por trimestre). Ou seja: o primeiro lançamento de `prova_trimestral` do **1º trimestre** já dispara o cálculo "final", usando zero para os trimestres 2 e 3 que ainda nem aconteceram.

### 4.2 Evidência empírica

Criada uma academia mista, um curso liceu e um técnico (ambos com `anos_academicos` corretos: liceu `[1,2,3]_ano_medio`, técnico `[1,2,3,4]_ano_medio` — isso está correto). Criadas duas matérias em `5_ano_fundamental` ("Matematica" e "Portugues"). Estudante `RMY7124` promovido para `5_ano_fundamental`. Sequência (com atraso de 0.4s entre chamadas para eliminar qualquer suspeita de condição de corrida):

```
POST /academia/notas-aluno {materia: Matematica, periodo: 1_trimestre, categoria: nota_professor, nota: 8}
  -> 201, avaliacoes_finais_automaticas: null   (esperado — não é a categoria-gatilho)

POST /academia/notas-aluno {materia: Matematica, periodo: 1_trimestre, categoria: prova_trimestral, nota: 8}
  -> 201, avaliacoes_finais_automaticas: [{
       "aprovado": false,
       "nota_final": 2.6666666666666665,
       "nota_minima_aprovacao": 5,
       "resultados_materias": [{
         "materia_id": "...Matematica...",
         "nota_final": 2.6666666666666665,
         "aprovado": false,
         "notas_substituidas_zero": [
           {"categoria":"nota_professor","periodo":"2_trimestre"},
           {"categoria":"prova_trimestral","periodo":"2_trimestre"},
           {"categoria":"nota_professor","periodo":"3_trimestre"},
           {"categoria":"prova_trimestral","periodo":"3_trimestre"}
         ]
       }],
       "type": "normal"
     }]
```

Isso aconteceu **no 1º trimestre**, muito antes do ano letivo terminar. O aluno tinha nota 8 (de 10) nos dois lançamentos reais que existiam até então — e mesmo assim foi avaliado e reprovado, porque os outros 4 valores da fórmula (nota_professor e prova_trimestral dos trimestres 2 e 3, que ainda não existem) foram tratados como zero.

Continuando o teste: os lançamentos reais e corretos dos trimestres 2 e 3 de Matemática (todos com nota 8) foram feitos depois, e a resposta de cada um foi `avaliacoes_finais_automaticas: []` — ou seja, **foram ignorados**, porque a avaliação daquele ano/tipo já "existe" (ver Achado 2). O registro final e definitivo do aluno nesse ano/matéria/tipo continua sendo o de nota 2.67, calculado com dados incompletos do 1º trimestre.

### 4.3 Correção recomendada

O gatilho deve disparar **apenas quando a nota lançada corresponde ao ponto de fechamento real da fórmula daquela matéria/regra** — não a qualquer ocorrência da categoria. Duas abordagens possíveis (escolha uma e documente a decisão no PR, como é costume neste repositório — ver exemplos em `docs/Lista de Tarefas/09 - ...md`):

**Opção A (mais simples, específica ao escolar fixo):** adicionar a noção explícita de "período de fechamento" à regra/categoria despertadora. Para o modelo escolar fixo, todas as regras hoje fecham no `3_trimestre` (verificar em `internal/handlers/modelo_avaliativo_escolar.go` — todas as fórmulas fixas usam `3_trimestre` como último período). Adicionar essa checagem explícita em `tiposAvaliacaoFinalDespertadosPorCategoria` (ou no ponto de chamada, em `notas_handlers.go`), exigindo que `periodoAlterado` também seja igual ao período de fechamento antes de disparar. Isso exige passar o período da nota lançada para dentro dessa função (hoje ela só recebe a categoria) e possivelmente estender `regraAvaliacaoFinalDTO`/`NotaDespertadora` para incluir esse período (ex.: `NotaDespertadoraPeriodo *string`), preenchido nas regras fixas.

**Opção B (mais geral, cobre também o Superior configurável):** em vez de fixar um período literal, derivar dinamicamente se a nota lançada é "a última peça de dados que faltava": antes de disparar o cálculo, verificar se **todas** as combinações de categoria/período exigidas pela fórmula daquela matéria (exceto a que acabou de ser lançada) já têm alguma nota real registrada no banco. Só disparar (e só então aplicar substituição por zero) quando isso for verdade. Essa abordagem é mais robusta porque não depende de hardcodar "3_trimestre" e funciona igualmente bem se a fórmula do Superior usar semestres diferentes. Tenha cuidado: **substituição por zero de referências ausentes é comportamento intencional e já auditado/aprovado** (ver `docs/Debbugs/Depurar padronizacao de avaliacao final diante de notas ausentes ou alteradas.md`) — a correção aqui é sobre **quando** disparar o gatilho, não sobre remover a substituição por zero em si.

Qualquer que seja a opção escolhida, **ela também deve corrigir o Achado 2 de forma coerente** — idealmente as duas correções devem ser pensadas juntas, porque ambas giram em torno da mesma pergunta: "quando exatamente uma avaliação final de um ano/tipo deve ser considerada completa e fechada?".

**Atenção ao Superior:** `tiposAvaliacaoFinalDespertadosPorCategoria` e `calcularResultadoMateriasAvaliacaoFinal` são usadas tanto pelo modelo escolar fixo quanto pelo modelo configurável do Superior (`tipoEnsino == "superior"`, que usa `preencherPeriodoFormulaSuperior` e `materia.Periodo`). Confirme que a correção não quebra o Superior — lá, cada matéria já tem um período fixo (`materia.Periodo`), então o conceito de "período de fechamento por matéria" já existe implicitamente ali; verifique se a Opção B naturalmente já cobre esse caso (parece que sim, mas valide com os testes existentes em `internal/handlers/avaliacao_final_regras_test.go` e `avaliacao_final_formula_test.go`).

---

## 5. Achado 2 — Avaliação final trava na primeira matéria e ignora as demais

### 5.1 Causa raiz exata

`internal/handlers/avaliacao_final_handler.go`, função `calcularResultadoMateriasAvaliacaoFinal` (por volta da linha 750):

```go
for _, materia := range materias {
    if overlay != nil && overlay.MateriaID != "" && overlay.MateriaID != materia.ID.String() {
        continue
    }
    ...
}
```

`overlay` é a nota recém-lançada que causou a tentativa de avaliação automática. As **duas únicas chamadas existentes** em produção para `tentarAvaliacoesFinaisAutomaticas` (em `internal/handlers/notas_handlers.go`, dentro de `RegistrarNota` e de `CorrigirNota`) **sempre** passam esse overlay preenchido com o `materia_disciplinar_id` da nota que acabou de ser lançada/corrigida. Isso significa que, na prática, **este `continue` sempre executa para todas as matérias exceto uma**, e o resultado (`resultados`) de `calcularResultadoMateriasAvaliacaoFinal` contém **no máximo 1 matéria**, mesmo quando `materias` (a lista de matérias aplicáveis àquele ano) tem várias.

Isso, por si só, seria administrável **se** o sistema acumulasse progressivamente os resultados de cada matéria à medida que cada uma recebe seu próprio gatilho, e só fechasse a decisão final quando todas as matérias aplicáveis tivessem contribuído. **Não é isso que acontece.** A trava está em `internal/projections/avaliacao_final_projection.go`, função `ExistsByEstudanteAnoLetivoNivelType` (linha ~630), cuja chave é `(codigo_estudante, codigo_academia, ano_lectivo, tipo_ensino, ano_academico_atual, type)` — **sem matéria**. E em `internal/handlers/avaliacao_final_handler.go`, função `regraPodeExecutarAutomaticamente` (linha 481):

```go
jaAvaliado, err := avaliacaoProj.ExistsByEstudanteAnoLetivoNivelType(...)
...
if jaAvaliado {
    return false, true, nil   // podeExecutar=false, encerrar=true
}
```

Ou seja: assim que a **primeira** matéria de um ano cria o registro de avaliação final `type=normal` (ou `type=exame_recurso`, etc.), **qualquer** matéria seguinte daquele mesmo ano/tipo — mesmo que reprove catastroficamente — é descartada silenciosamente (`avaliacoes_finais_automaticas: []`), porque `jaAvaliado=true` faz o loop encerrar antes mesmo de chegar em `calcularResultadoMateriasAvaliacaoFinal`.

Isso vale igualmente para `RegistrarNota` e para `CorrigirNota` (`PATCH /academia/notas-aluno/:id`) — **não existe correção de nota, nem re-lançamento, capaz de destravar esse estado**, mesmo que a documentação da API diga que uma correção "pode disparar o recálculo de avaliação final" (isso só é verdade se aquele `type` específico ainda não tiver sido avaliado).

### 5.2 Nota sobre o audit anterior desta área

`docs/Debbugs/Depurar padronizacao de avaliacao final diante de notas ausentes ou alteradas.md` já confirmou (corretamente) que a substituição por zero é restrita à matéria que recebeu o próprio gatilho, e concluiu: *"outras matérias do mesmo estudante continuam aguardando seus próprios lançamentos/gatilhos e não recebem zeros por consequência indireta"*. Essa frase estava correta sobre o **cálculo de zero**, mas implicitamente presumia que essas outras matérias **eventualmente seriam avaliadas** quando seu próprio gatilho chegasse. O audit anterior não testou esse cenário (duas ou mais matérias no mesmo ano, cada uma recebendo seu próprio gatilho em momentos diferentes) — e é exatamente aí que mora o bug: a promessa de "aguardando seus próprios lançamentos" nunca se cumpre, porque a chave de idempotência do evento já foi consumida pela primeira matéria. Ambas as conclusões (a do audit anterior e a deste documento) são compatíveis; este documento apenas cobre um cenário que o anterior não cobriu.

### 5.3 Evidência empírica

Continuando o teste da seção 4 (estudante `RMY7124`, 5º ano fundamental, matérias Matemática e Português): depois que Matemática disparou (indevidamente, por causa do Achado 1) a avaliação com nota 2.67/reprovado usando só o 1º trimestre, os lançamentos seguintes — incluindo os 2º e 3º trimestres **corretos e completos** de Matemática (nota 8 em tudo) **e todos os 6 lançamentos completos de Português** (nota 2 em tudo, uma reprovação real e óbvia) — retornaram `avaliacoes_finais_automaticas: []` em cada chamada. O estado final consultado via `GET /avaliacoes-estudante/RMY7124`:

```json
{
  "total": 1,
  "avaliacoes": [{
    "type": "normal",
    "aprovado": false,
    "nota_final": 2.67,
    "resultados_materias": [
      {"materia_id": "...Matematica...", "nota_final": 2.6666666666666665, "aprovado": false}
    ]
  }]
}
```

Note que **Português nunca aparece em `resultados_materias`**, apesar de ter 6 notas lançadas, todas reprovando com nota 2/10. A decisão "reprovado" do aluno, embora coincidentemente correta neste caso específico, foi tomada **sem considerar Português de forma alguma** — e o inverso (aluno aprovado erroneamente porque a primeira matéria lançada passou, mesmo reprovando de verdade em outra matéria nunca avaliada) é igualmente possível e, dado que a maioria dos anos tem várias matérias, é o cenário mais provável no uso real do sistema.

### 5.4 Correção recomendada

Esta é uma mudança de maior porte que o Achado 1 e provavelmente vai exigir alterar o formato do evento/projeção de avaliação final para suportar acumulação progressiva de resultados por matéria dentro do mesmo `(estudante, ano_letivo, nível, ano_academico, type)`. Direção recomendada:

1. **Idempotência por matéria, não por ano inteiro.** Ao decidir se uma matéria específica pode ser avaliada, verificar se **aquela matéria específica** já tem resultado registrado dentro da avaliação em curso daquele `(estudante, ano_letivo, nível, ano_academico, type)` — não apenas se o "ano/tipo" como um todo já tem algum evento.
2. **Acumular, não sobrescrever.** Quando uma nova matéria dispara seu próprio gatilho (respeitando a correção do Achado 1 — só dispara no fechamento real daquela matéria), o resultado dela deve ser **adicionado** a `resultados_materias` da avaliação em curso daquele ano/tipo, não bloqueado.
3. **Só fechar/decidir quando completo.** O campo `aprovado` geral do evento, o cálculo de `nota_final` (média), e principalmente o **efeito colateral de progressão de ano** (`proximo_ano_academico`, atualização de `ano_escolar_fundamental`/`ano_escolar_medio` do estudante) só devem ser calculados e aplicados quando `len(resultados_materias) == len(materiasAplicaveis)` para aquele ano/curso — ou seja, quando **todas** as matérias aplicáveis já contribuíram com seu resultado.
4. Isso provavelmente implica: (a) um novo campo/mecanismo no aggregate `Estudante`/evento de avaliação final para representar um estado "avaliação em progresso, aguardando N de M matérias", e (b) mover a checagem de aprovação/reprovação e a chamada de progressão de ano para só executar no momento em que a última matéria pendente é avaliada.
5. Avalie se faz sentido introduzir um evento novo e distinto (ex.: `AvaliacaoFinalMateriaRegistrada`) versus reaproveitar o evento atual de forma mutável/append. Documente a decisão tomada explicitamente no PR e na documentação de tarefa concluída, como é costume neste repositório.

**Atenção ao Superior:** confirme que o Superior não depende do comportamento atual (uma matéria = um evento de avaliação final fechado imediatamente) de forma que quebraria com essa mudança. Pelo já documentado em `docs/Tarefas feitas/Documentar modelo de avaliacao final por materia e regra de avaliacao final.md`, o Superior já opera com o conceito de pendência por matéria (`pendencia_permitida`) e parece ter sido desenhado para múltiplas matérias/períodos desde o início — vale conferir se o Superior **já sofre exatamente do mesmo bug** (matéria única trava o ano) ou se ele tem algum mecanismo que escapa disso que possa ser reaproveitado para o escolar. Isso não foi testado nesta auditoria porque o escopo pedido pelo usuário foi apenas o nível escolar (fundamental e médio), mas psicologicamente é muito provável que o Superior tenha o mesmo problema, já que usa a mesma função `calcularResultadoMateriasAvaliacaoFinal`. Se confirmar que sim, resolva os dois de uma vez, já que a correção é na mesma função.

---

## 6. Achado 3 — 4º ano médio técnico (PAP) é estruturalmente inalcançável

### 6.1 Causa raiz exata (duas regras de negócio que se contradizem)

**Regra A** — `internal/domain/aggregates/materia_disciplinar.go`, função de validação de anos acadêmicos de matéria (linha ~209): proíbe **qualquer** matéria de conter `"4_ano_medio"` em `anos_academicos`. Confirmado pela migration `094_materias_medio_multiplos_anos_sem_quarto` e pela documentação: *"Matérias do médio podem informar múltiplos anos acadêmicos no mesmo array, desde que nenhum item seja `4_ano_medio`."*

**Regra B** — `internal/handlers/notas_handlers.go`, função `inferirAnoAcademicoParaNota`: para aceitar o lançamento de uma nota, exige que o ano acadêmico atual do estudante esteja **literalmente contido** em `materiaDTO.AnosAcademicos`.

Como nenhuma matéria pode ter `"4_ano_medio"` (Regra A), nenhuma matéria jamais vai satisfazer a Regra B para um estudante que está em `4_ano_medio`. Resultado: **nenhuma nota pode ser lançada** para um estudante de curso técnico no seu último ano, incluindo a nota `nota_pap` (Prova de Aptidão Profissional) — que é justamente a única categoria de nota fixa definida para esse ano (`internal/handlers/modelo_avaliativo_escolar.go`, função `categoriasEscolaresFixasParaAno`, caso especial `4_ano_medio` + `tecnico` → `categoriasEscolaresPAP`).

O sistema **sabe** que `nota_pap` deveria existir para `4_ano_medio` — `GET /academia/categorias-nota` retorna corretamente:

```json
{"codigo": "nota_pap", "nome": "Prova de Aptidão Profissional", "anos_academicos": ["4_ano_medio"], "fixed": true, "readonly": true}
```

— mas não há absolutamente nenhuma forma de efetivamente lançar essa nota.

### 6.2 Evidência empírica (3 tentativas de contorno, todas bloqueadas)

```
1) POST /academia/materia {type: medio, curso_id: <tecnico>, anos_academicos: ["4_ano_medio"]}
   -> 400 "não é permitido criar ou atualizar matérias para o 4º ano do ensino médio"

2) Criada uma matéria com anos_academicos: ["3_ano_medio"] no mesmo curso técnico (para tentar contornar).
   Estudante XTN8698 promovido para 4_ano_medio (via SQL, só para teste).
   POST /academia/notas-aluno {materia: <criada acima>, periodo: 3_trimestre, categoria: nota_pap, nota: 15}
   -> 400 "o estudante está no ano acadêmico médio '4_ano_medio', que não faz parte da
       matéria 'Projeto Final Tecnico' (anos permitidos: [3_ano_medio])"

3) GET /academia/categorias-nota confirma nota_pap como categoria fixa válida para
   anos_academicos: ["4_ano_medio"], mas isso é apenas metadado — não muda o resultado de (1) e (2).
```

### 6.3 Correção recomendada

Este é o achado com maior grau de decisão de produto envolvida (por que a Regra A foi criada assim?). Provavelmente a intenção da migration 094 era impedir que **disciplinas comuns** (Matemática, Português etc.) continuassem existindo no 4º ano técnico, já que esse ano é dedicado à PAP — mas isso acabou bloqueando também a própria PAP, que precisa de algum "ancoradouro" de matéria para ser lançada, dado que `POST /academia/notas-aluno` sempre exige `materia_disciplinar_id`. Opções, da mais para a menos invasiva:

**Opção A (recomendada):** permitir explicitamente que uma matéria seja criada com `anos_academicos: ["4_ano_medio"]` **apenas quando vinculada a um curso técnico** (`curso.Modelo == "tecnico"`) **e apenas para uso com a categoria fixa `nota_pap`** (ou seja, ainda impedir que vire uma disciplina comum recorrente com `nota_professor`/`prova_trimestral`/etc.). Isso preserva a intenção original (bloquear disciplinas comuns no 4º ano) e resolve o problema (dá à PAP um lugar legítimo para existir). Avalie se faz sentido a academia criar essa matéria manualmente (como qualquer outra) ou se o sistema deveria criá-la automaticamente ao ativar um curso técnico (mais amigável, evita erro operacional da academia esquecer de criar essa matéria e travar a avaliação de todos os alunos do 4º ano).

**Opção B (mais invasiva):** eliminar a exigência de `materia_disciplinar_id` para a categoria `nota_pap` especificamente, tratando-a como vinculada diretamente ao curso, sem depender de uma matéria disciplinar. Exige mudanças mais amplas no contrato de `POST /academia/notas-aluno` e em `materiasAplicaveisAvaliacaoFinal`.

Qualquer que seja a opção, valide que o fluxo completo funciona de ponta a ponta pelo menos no nível de leitura de código: criar/permitir a matéria → `inferirAnoAcademicoParaNota` aceita a nota → `materiasAplicaveisAvaliacaoFinal` encontra a matéria para `anoAcademicoAtual="4_ano_medio"` → `calcularResultadoMateriasAvaliacaoFinal` consegue calcular `nota_pap` contra a fórmula fixa de `4_ano_medio` técnico → aprovação/reprovação e conclusão do curso técnico funcionam. Esse último passo (conclusão do curso, `calcularProximoAnoCurso` retornando `nil` por já ser o último ano do array `[1,2,3,4]_ano_medio`) já parece correto por leitura de código, só precisa deixar de ser inatingível.

---

## 7. O que você (Codex) PODE validar sem banco de dados

As seguintes funções são **puras** (não tocam banco de dados) e já são testadas hoje sem precisar de Postgres (ver `internal/handlers/avaliacao_final_regras_test.go`, `avaliacao_final_formula_test.go`, `modelo_avaliativo_escolar_test.go`, todos passam hoje com `go test ./...` sem nenhuma conexão de banco):

- `tiposAvaliacaoFinalDespertadosPorCategoria` — escreva casos novos cobrindo: (a) categoria certa + período errado NÃO deve disparar (hoje dispara — este é o bug do Achado 1); (b) categoria certa + período de fechamento correto DEVE disparar; (c) comportamento inalterado para o Superior.
- `substituirNotasAusentesPorZero` e `calcularFormulaAvaliacao` — confirme que a correção do Achado 1 não alterou o comportamento de substituição por zero em si (que deve continuar existindo, só que disparado no momento certo).
- `regraAvaliacaoFinalEscolarFixa` e `categoriasEscolaresFixasParaAno` — para o Achado 3, depois de decidir a Opção A/B, adicione um teste confirmando que a regra fixa de `4_ano_medio` + técnico continua resolvendo corretamente `categoriasEscolaresPAP`.
- `calcularProximoAnoFundamental` e `calcularProximoAnoCurso` — já parecem corretas (sequência 1→9 fundamental sem transição automática para médio; sequência do curso vinda de `curso.AnosAcademicos`, que já é fixado corretamente como `[1,2,3]_ano_medio` para liceu e `[1,2,3,4]_ano_medio` para técnico), mas **não têm teste dedicado hoje** — adicione testes cobrindo liceu (termina em `3_ano_medio` → `nil`) e técnico (termina em `4_ano_medio` → `nil`), já que isso é diretamente relevante para "aprovação" no sentido de progressão de ano.

Para o Achado 2, a lógica central que precisa mudar (`calcularResultadoMateriasAvaliacaoFinal`, `regraPodeExecutarAutomaticamente`, `ExistsByEstudanteAnoLetivoNivelType`) depende de banco de dados real para ser testada de ponta a ponta (idempotência entre chamadas HTTP sucessivas). Escreva o máximo de teste unitário que der para isolar sem banco (ex.: a lógica de "quando uma avaliação está completa" pode provavelmente ser extraída para uma função pura que recebe `len(resultados_materias já registrados)` e `len(materiasAplicaveis)` e decide se fecha ou não — isso SIM dá para testar sem banco), mas documente claramente que o teste de ponta a ponta com 2+ matérias reais via HTTP/Postgres **fica pendente de validação** por quem tiver acesso a um banco real (Claude ou um humano).

---

## 8. Cenários de teste que devem ser cobertos (para quando houver acesso a banco real)

Esta lista é para quem for validar com Postgres real depois da sua correção (Claude ou um humano). Você não precisa executá-la, mas deve deixá-la pronta/documentada como checklist de validação pendente:

- **Fundamental "sem exame" (1º,2º,3º,4º,5º,7º,8º ano):** 2+ matérias no mesmo ano; uma aprova em todos os trimestres, outra reprova; nota exatamente no limite mínimo (5 para 1º-6º, 10 para 7º-9º/médio) deve aprovar (`>=`); nota um decimal abaixo do limite deve reprovar; lançar os trimestres fora de ordem (3º antes do 1º) não deve mudar o resultado final nem dar zero indevido nos já lançados depois.
- **Fundamental "com exame" (6º e 9º ano):** reprovação no normal libera exame; aprovação no exame aprova o aluno e avança o ano; reprovação no exame libera recurso; aprovação no recurso aprova e avança; reprovação no recurso mantém reprovado e não avança. Repetir com 2+ matérias, garantindo que cada matéria segue sua própria cadeia normal→exame→recurso de forma independente e que o resultado geral do aluno no ano só fecha quando todas as matérias concluírem sua própria cadeia.
- **Médio liceu (1º,2º "sem exame"; 3º "com exame/recurso"):** mesmos cenários acima, adaptados à escala 0-20/mínimo 10. Confirmar que liceu nunca aceita/gera `4_ano_medio`.
- **Médio técnico (1º,2º "sem exame"; 3º "com exame/recurso"; 4º = PAP):** mesmos cenários dos anos 1-3, mais o cenário do Achado 3 corrigido: PAP lançável, calculável, aprovando/reprovando e concluindo o curso corretamente ao aprovar.
- **Correção de nota (`PATCH /academia/notas-aluno/:id`) depois que a avaliação daquele tipo já existe:** confirmar que, após a correção do Achado 2, uma matéria ainda não avaliada consegue ser avaliada normalmente mesmo que outra matéria do mesmo ano já tenha sido avaliada antes (isso é exatamente o que está quebrado hoje).
- **Concorrência/ordem de chegada:** lançar notas de 3+ matérias em ordens diferentes (a que dispara primeiro varia) e confirmar que o resultado final do aluno é sempre o mesmo, independentemente de qual matéria disparou seu gatilho primeiro.

---

## 9. Documentação a atualizar

- `Documentação da API.md`, seção 15 (Avaliações Finais): descrever o comportamento corrigido do gatilho (agora sensível a período/fechamento) e do fechamento por matéria (agora progressivo, só decide aprovação/progressão quando todas as matérias aplicáveis contribuíram). Se a Opção A do Achado 3 for adotada, documentar como a matéria da PAP passa a ser criada/gerenciada.
- Criar `docs/Tarefas feitas/Corrigir falhas criticas no motor de avaliacao final escolar (aprovacao e reprovacao).md`, seguindo o padrão de `docs/Tarefas feitas/Corrigir regra de avaliacao final automatica por materia e pendencias.md`, descrevendo o que foi de fato implementado, as decisões tomadas para os pontos em aberto (Opção A/B de cada achado) e os testes adicionados.
- Criar `docs/Debbugs/Depurar correcao das falhas criticas no motor de avaliacao final escolar.md` documentando: o que foi corrigido, o que foi testado sem banco (liste os testes), e **a lista explícita e destacada do que fica pendente de validação com Postgres real** (a checklist da seção 8 deste documento), para que a próxima pessoa/IA com acesso a banco saiba exatamente o que rodar.

---

## 10. O que NÃO fazer

- Não criar aliases, flags de compatibilidade ou comportamento condicional para manter o bug antigo "por segurança" — os três achados são bugs, não mudanças de contrato que precisem de transição gradual.
- Não presumir que o Achado 2 pode ser corrigido só ajustando o Achado 1 — são independentes: mesmo com o gatilho disparando no período certo, se duas matérias fecham no mesmo instante/próximas, a trava de idempotência por ano (sem matéria) ainda vai deixar só uma delas valer.
- Não alterar o comportamento do Superior sem entender primeiro se ele compartilha os mesmos bugs (ver seção 5.4) — se compartilhar, corrija junto usando a mesma mudança; se não compartilhar, não regrida o que já funciona lá.
- Não declarar a tarefa como "testada e validada" para partes que você não conseguiu rodar contra um banco real — seja explícito sobre o que ficou pendente, conforme pedido pelo usuário original desta auditoria.

---

## Anexo — evidências brutas adicionais (para referência)

### A. Cadeia normal→recurso funcionando corretamente quando há apenas 1 matéria no ano (6º ano fundamental)

Isso confirma que a lógica de dependência entre tipos de regra (`AplicaSeReprovadoEmType`) e a progressão de ano em si **estão corretas** — o problema é exclusivamente o gatilho (Achado 1) e a limitação a uma matéria (Achado 2), não a cadeia normal→exame→recurso em si nem o avanço de ano:

```
Notas: T1=4/4, T2=4/4, T3 prof=2, T3 exame_final=0
  -> media = ((4+4)/2 + (4+4)/2 + (2+0)/2) / 3 = (4+4+1)/3 = 3.0  -> reprovado no normal (correto)
  -> estudante permanece em 6_ano_fundamental (correto, não avançou)

Lançado exame_recurso = 8 (dentro da escala 0-10 do 6º ano)
  -> POST retorna 201, avaliacoes_finais_automaticas com type=exame_recurso, aprovado=true, nota_final=8
  -> GET /consultar-estudante confirma: ano_escolar_fundamental agora é "7_ano_fundamental" (avançou corretamente)
```

### B. Confirmação de que o modelo de curso médio (liceu vs técnico) está correto

```
POST /academia/curso {nome: "Ciencias Fisicas e Biologicas", modelo: "liceu"}
  -> anos_academicos retornado: ["1_ano_medio", "2_ano_medio", "3_ano_medio"]

POST /academia/curso {nome: "Informatica", modelo: "tecnico"}
  -> anos_academicos retornado: ["1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"]
```

Isso está de acordo com o que o usuário descreveu (liceu até o 3º ano médio, técnico até o 4º) e não precisa de nenhuma correção.

### C. Confirmação de que a comparação de aprovação é inclusiva no limite (`>=`)

Em `internal/handlers/avaliacao_final_handler.go`: `aprovada := nota >= regra.NotaMinimaAprovacao`. Isso foi confirmado incidentalmente no teste do Anexo A, onde uma nota final de exatamente `5.0` (igual ao mínimo de aprovação do 6º ano fundamental) resultou em `aprovado: true`. Este comportamento está correto e não precisa de correção.
