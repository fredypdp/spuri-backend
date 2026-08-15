---
criado: 2026-08-14 00:00
atualizado: 2026-08-15 00:00
origem: v2 — reauditoria completa contra o HEAD atual de fredypdp/spuri-backend (branch main), feita clonando o repositório e lendo o código linha a linha (não apenas o diff do commit cf0f2be). Substitui integralmente a v1 deste documento.
status: feito
prioridade: alta — o Problema C abaixo é uma regressão de dados em produção (faltas históricas somem de todas as consultas e não podem ser corrigidas), não apenas dívida de documentação/testes.
repositorio: fredypdp/spuri-backend (branch main)
---

# 37 — Corrigir pendências da Tarefa 34 (nota de contrato duplicada, testes de periodo ausentes e regressão crítica de leitura/correção de faltas históricas) — v2 FINAL (feito)

## Por que esta versão existe

A v1 deste documento foi escrita a partir de uma auditoria feita logo após o commit `cf0f2be` (implementação original da Tarefa 34). Desde então, dois commits adicionais foram mergeados em `main`:

- `9f8c73f` / merge `#532` — implementação da Tarefa 35 (vínculo de estudante a turma no cadastro). Não afeta faltas.
- `d11b1fe` / merge `#533` — **"fix: permite faltas historicas sem periodo"**. Este commit alterou `migrations/107_periodo_faltas.sql`, removendo a exigência de `NOT NULL` em `projection_faltas.periodo` e permitindo `NULL` para faltas legadas sem período determinístico. **Essa mudança não foi acompanhada de nenhum ajuste no código Go que lê essa coluna**, e isso introduziu uma regressão que não existia (e não podia ter sido vista) na auditoria original.

Esta v2 mantém os Problemas A e B da v1 (ainda válidos, verificados novamente linha a linha contra o HEAD atual) e adiciona o **Problema C**, que é o mais importante dos três e deve ser corrigido primeiro.

Esta versão é definitiva: todas as decisões de design já foram tomadas abaixo. Codex deve apenas executar — não há necessidade de investigar, escolher entre abordagens alternativas ou reler o histórico do git.

## Prompt recomendado para executar a atualização

Implemente exatamente os três itens da seção "Escopo obrigatório" abaixo, na ordem C → A → B (C primeiro porque A e B ficam mais simples de revisar depois que os testes novos de C existem como referência de padrão). **Não altere nenhum arquivo em `internal/domain/aggregates/` além da função `CorrigirFalta` em `estudante_falta.go` (item C.2, abaixo — mudança cirúrgica e explicitamente especificada)**, e não altere `internal/handlers/faltas_handlers.go`, `internal/handlers/registros_handlers.go` além dos pontos exatos indicados no item C.1, nem `migrations/107_periodo_faltas.sql` (a migração já está correta como está — o problema é que o código Go não foi atualizado para refletir a mudança que ela já fez). Ao final, rode `gofmt -l .` (sem saída), `go build ./...`, `go vet ./...` e `go test ./...`, e confirme cada item de "Critérios de aceite".

## Contexto

### Problema C — regressão crítica: faltas históricas somem das consultas e não podem ser corrigidas (NOVO — não estava na v1)

`migrations/107_periodo_faltas.sql` (estado atual, já como está no repositório):

```sql
-- 3. Registros legados que permanecerem sem período ficam explicitamente NULL,
--    com CHECK permitindo NULL apenas como dívida histórica. [...]
```

A coluna `projection_faltas.periodo` **aceita `NULL`** para faltas registradas antes da Tarefa 34 cujo tipo de matéria não permite backfill determinístico (fundamental/médio — só matérias tipo `superior` recebem backfill automático a partir de `projection_materias.periodo`).

**C.1 — Leitura silenciosamente quebrada.** Em `internal/projections/faltas_projection.go`, a struct `FaltaDTO` declara `Periodo string` (não `*string`), e a função `scanFaltas` (linha ~306) faz `rows.Scan(..., &f.Periodo, ...)` direto nesse campo. Escanear um `NULL` do Postgres para um `string` Go não-ponteiro retorna erro (`sql: Scan error [...] converting NULL to string is unsupported`). O código trata esse erro assim:

```go
if err := rows.Scan(...); err != nil {
    continue // descarta a linha inteira, sem log, sem erro propagado
}
```

Isso significa: **toda falta histórica de matéria fundamental/médio sem período determinístico desaparece silenciosamente de `GetByID`, `GetByEstudante`, `GetByAcademia`, `GetByPeriodo` e `GetAll`** — ou seja, de todos os endpoints que leem faltas.

O mesmo padrão existe, de forma um pouco menos silenciosa, em `internal/handlers/registros_handlers.go`, na função que atende `GET /faltas` (a struct `FaltaRegistroResponse`, campo `Periodo string`, escaneado na query de `baseQuery` por volta da linha 253). Ali o erro é logado (`log.Printf("[WARN] ListarFaltas: erro ao ler linha: %v", err)`) mas a linha ainda é descartada da resposta — e pior, o `total` retornado ao cliente vem de uma `COUNT(*)` separada (`baseCountQuery`) que **não** sofre esse problema, então `total` fica maior que `len(faltas)` retornado, quebrando a paginação do cliente.

Note que essa mesma query já usa `COALESCE(m.nome, '')` para `materia_nome` — ou seja, o padrão correto para lidar com colunas que podem ser `NULL` já existe no arquivo, só não foi aplicado à coluna `periodo`.

**C.2 — Correção (`PATCH /academia/faltas-aluno/:id`) de faltas históricas falha mesmo quando a leitura funciona.** Esta é mais sutil e mais grave: mesmo depois de corrigir C.1, `CorrigirFalta` continuaria falhando para faltas antigas. O motivo:

- `internal/domain/aggregates/estudante_falta.go`, função `chaveFalta` (linha 60), monta uma chave de deduplicação em memória incluindo o campo `periodo`.
- `applyFaltasRegistradas` (linha 152) reconstrói essa chave ao repetir (replay) cada evento `FaltasRegistradas` do ledger, usando `ev.Periodo` — o campo `Periodo` do payload do evento, decodificado via JSON.
- Eventos gravados **antes** da Tarefa 34 nunca tiveram o campo `Periodo` no payload JSON. Ao serem decodificados na struct atual `FaltasRegistradasEvent` (que agora inclui `Periodo string`), o campo ausente vira `""` (zero value do Go) — isso é comportamento padrão e correto do `encoding/json`, não um bug em si.
- Ou seja: para **qualquer** falta registrada antes da Tarefa 34, `e.FaltasRegistradasPorChave` guarda a chave com `periodo=""`, independentemente do que a projeção (`projection_faltas.periodo`) tenha depois de aplicado o backfill da migração 107.
- `CorrigirFalta` (handler, `internal/handlers/faltas_handlers.go`, linha ~267) chama `estudante.CorrigirFalta(..., falta.Periodo, ...)` usando o `periodo` **da projeção** (que pode ser `"1_semestre"` para uma matéria superior com backfill, ou `""`/vazio após a correção do item C.1 para uma matéria fundamental/médio sem backfill).
- Para uma falta histórica de matéria **superior com backfill bem-sucedido**, isso causa mismatch: a chave em memória é `..._""_..." `, mas o handler pede a chave `..._"1_semestre"_...`. Resultado: `estudante.CorrigirFalta` retorna `"falta original não encontrada para correção"` — uma mensagem enganosa, já que a falta claramente existe (foi encontrada segundos antes por `getFaltasProjection(c).GetByID`).
- Para uma falta histórica sem backfill (fica com periodo vazio após a correção do item C.1), a chave já bate por coincidência (`""` == `""`), então **esse subcaso já funciona** depois de C.1 — mas não se pode confiar nisso continuar funcionando sem o fallback abaixo, e o subcaso "matéria superior com backfill" continua quebrado sem ele.

**Nenhum teste hoje cobre esse cenário** porque os testes existentes de agregado (`estudante_registros_correcao_test.go`) sempre chamam `RegistrarFalta` e `CorrigirFalta` na mesma instância em memória, com o mesmo `periodo` nas duas chamadas — nunca simulam "carregar do ledger um evento antigo sem o campo".

### Problema A — nota de contrato "vazou" para 11 endpoints não relacionados (confirmado, sem mudanças desde a v1)

Reconferido linha a linha contra o HEAD atual: **12 ocorrências exatas** da frase `**Nota de contrato:** esta é uma mudança breaking; \`POST /academia/faltas-aluno\` e \`POST /academia/faltas-aluno/async\` rejeitam itens sem \`periodo\`.` em `Documentação da API.md`. As seções afetadas (indevidamente) continuam sendo exatamente as 11 já listadas na v1:

1. `PUT /academia/materia/ativar/async`
2. `PUT /academia/materia/desativar/async`
3. `PUT /academia/materia/dados/async`
4. `DELETE /academia/materia/async`
5. `POST /academia/turma/async`
6. `POST /academia/turma/estudante/async`
7. `PUT /academia/turma/ativar/async`
8. `PUT /academia/turma/desativar/async`
9. `PUT /academia/turma/dados/async`
10. `DELETE /academia/turma/async`
11. `DELETE /academia/turma/estudante/async`

A 12ª ocorrência (correta, a manter) está dentro da seção `### \`POST /academia/faltas-aluno/async\``, próxima ao final do arquivo.

### Problema B — testes obrigatórios da seção 5 da Tarefa 34 ainda não implementados (confirmado, sem mudanças desde a v1)

Reconferido: `internal/domain/aggregates/estudante_registros_correcao_test.go` tem só 4 funções de teste (`TestCorrigirNotaPreservaEventoOriginal`, `TestCorrigirFaltaExigeMotivo`, `TestRegistrarECorrigirFaltaRespeitamTetoDoAggregate`, `TestRegistrarNotaRespeitaTetoDoAggregate`) — nenhuma delas é um dos 13 cenários exigidos. `cmd/server/notas_faltas_correcao_integration_test.go` tem 3 funções, idem, nenhuma cobre os 13 cenários.

Uma correção em relação à v1: a mensagem de erro citada no cenário 5 foi **reverificada contra o código atual e está correta como estava documentada** — `internal/handlers/faltas_handlers.go` (linha ~131) e `internal/handlers/notas_handlers.go` (linha ~128) usam ambos exatamente `"periodo '%s' invalido para a materia '%s'. Periodo definido: '%s'"` para o caso em que a matéria é tipo `superior`, tem `Periodo` fixo definido, e o período enviado não bate.

## Escopo obrigatório

### C. Corrigir a regressão de faltas históricas (fazer primeiro)

#### C.1 — `COALESCE` nas leituras de `periodo`

Em `internal/projections/faltas_projection.go`, adicione `COALESCE(f.periodo, '')` no lugar de `f.periodo` nas 5 queries a seguir (mantendo a struct `FaltaDTO.Periodo string` como está, sem virar ponteiro — é a mudança de menor risco, e já é o padrão usado no mesmo arquivo para `materia_nome`):

- `GetByID` (linha ~217)
- `GetByEstudante` (linha ~237)
- `GetByAcademia` (linha ~254)
- `GetByPeriodo` (linha ~271)
- `GetAll` (linha ~290)

Em cada uma, a linha:
```sql
f.periodo, f.data, f.materia_disciplinar_id, m.nome, f.quantidade, [...]
```
vira:
```sql
COALESCE(f.periodo, ''), f.data, f.materia_disciplinar_id, m.nome, f.quantidade, [...]
```
(as 5 ocorrências são literalmente idênticas — pode fazer um find-and-replace de `f.periodo, f.data, f.materia_disciplinar_id, m.nome` para `COALESCE(f.periodo, ''), f.data, f.materia_disciplinar_id, m.nome` no arquivo inteiro, já que essa substring só aparece nessas 5 queries).

Em `internal/handlers/registros_handlers.go`, na função que monta `baseQuery` para `GET /faltas` (por volta da linha 215-219), a linha:
```sql
f.periodo, f.data, f.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
```
vira:
```sql
COALESCE(f.periodo, ''), f.data, f.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
```

Depois dessa mudança, `scanFaltas` e o loop de scan de `ListarFaltas` nunca mais recebem `NULL` na coluna `periodo`, então o `continue` silencioso deixa de ser acionado por esse motivo (o `continue` genérico pode continuar existindo para outros erros de scan verdadeiramente inesperados — não é preciso removê-lo, só deixar de ser exercitado pela causa que identificamos).

#### C.2 — Fallback de chave legada em `CorrigirFalta`

Em `internal/domain/aggregates/estudante_falta.go`, dentro de `func (e *Estudante) CorrigirFalta(...)` (linha ~122), substitua:

```go
chave := chaveFalta(e.CodigoEstudante, codigoAcademia, anoLectivo, periodo, data, materiaID)
if e.FaltasRegistradasPorChave == nil || !e.FaltasRegistradasPorChave[chave] {
    return fmt.Errorf("falta original não encontrada para correção")
}
```

por:

```go
chave := chaveFalta(e.CodigoEstudante, codigoAcademia, anoLectivo, periodo, data, materiaID)
encontrada := e.FaltasRegistradasPorChave != nil && e.FaltasRegistradasPorChave[chave]
if !encontrada && periodo != "" {
    // Compatibilidade com faltas registradas antes da Tarefa 34: o evento
    // FaltasRegistradas gravado no ledger não tinha o campo Periodo, então,
    // ao ser repetido (replay), a chave em memória foi indexada com
    // periodo="" — mesmo que a projeção (via migration 107_periodo_faltas.sql)
    // tenha preenchido retroativamente um periodo determinístico (matérias
    // tipo superior). Sem este fallback, a correção dessas faltas falha com
    // "falta original não encontrada" mesmo quando a falta existe.
    chaveLegado := chaveFalta(e.CodigoEstudante, codigoAcademia, anoLectivo, "", data, materiaID)
    encontrada = e.FaltasRegistradasPorChave != nil && e.FaltasRegistradasPorChave[chaveLegado]
}
if !encontrada {
    return fmt.Errorf("falta original não encontrada para correção")
}
```

Não mude mais nada nesse arquivo — `RegistrarFalta` não precisa desse fallback (só lida com registros novos, sempre com `periodo` validado e não-vazio).

#### C.3 — Testes que comprovam a correção (numeração 14 e 15, continuando a lista do Problema B)

Adicione estes dois testes à mesma leva de testes do item B (mesmos arquivos/padrões descritos abaixo):

**14. (agregado, sem banco)** Simular exatamente o cenário de replay legado: crie um `*aggregates.Estudante` vazio (`aggregates.NewEstudante()` + `CriarComVinculo(...)`), aplique manualmente um evento `FaltasRegistradasEvent` **sem** passar pelo `RaiseEvent`/`Apply` normal de `RegistrarFalta` — construa a struct do evento diretamente com `Periodo: ""` (simulando o payload de um evento antigo) e chame `estudante.Apply(event)` para popular `FaltasRegistradasPorChave` como um replay real faria. Em seguida, chame `CorrigirFalta` passando um `periodo` não-vazio (ex.: `"1_semestre"`, simulando o valor vindo da projeção já com backfill) para os mesmos `codigoAcademia/anoLectivo/data/materiaID` do evento simulado, e confirme que a correção **é aceita** (sem erro) — isso comprova o fallback do item C.2. Nome sugerido: `TestCorrigirFaltaAceitaChaveLegadaSemPeriodo`.

**15. (integração HTTP + banco real)** Estenda `setupRegistrosCorrecaoIntegration` (ou crie uma variação local no mesmo arquivo de teste) para, além do fluxo atual, inserir diretamente no banco de teste — via SQL bruto, imitando um dado pré-Tarefa-34 — uma falta em `projection_faltas` com `periodo = NULL`, associada a um evento `FaltasRegistradas` também gravado sem o campo `periodo` no ledger (ajuste o payload JSON manualmente na tabela do ledger para remover a chave `"Periodo"`, ou insira o evento com um marshal que a omita). Depois: (a) confirme via `GET /faltas-estudante/:codigo` que essa falta **aparece** na resposta com `"periodo": ""` (prova de C.1); (b) confirme via `PATCH /academia/faltas-aluno/:id` que a correção dessa falta **funciona** (prova de C.2, ponta a ponta). Nome sugerido: `TestHTTPIntegrationFaltaHistoricaSemPeriodoEhListavelECorrigivel`.

### A. `Documentação da API.md`

Rode este script Python a partir da raiz do repositório (ele localiza as seções pelo cabeçalho, não por número de linha, então funciona mesmo que o arquivo tenha mudado ligeiramente desde esta auditoria):

```python
path = "Documentação da API.md"
with open(path, "r", encoding="utf-8") as f:
    content = f.read()

nota = '**Nota de contrato:** esta é uma mudança breaking; `POST /academia/faltas-aluno` e `POST /academia/faltas-aluno/async` rejeitam itens sem `periodo`.'

secoes_indevidas = [
    "PUT /academia/materia/ativar/async",
    "PUT /academia/materia/desativar/async",
    "PUT /academia/materia/dados/async",
    "DELETE /academia/materia/async",
    "POST /academia/turma/async",
    "POST /academia/turma/estudante/async",
    "PUT /academia/turma/ativar/async",
    "PUT /academia/turma/desativar/async",
    "PUT /academia/turma/dados/async",
    "DELETE /academia/turma/async",
    "DELETE /academia/turma/estudante/async",
]

lines = content.split("\n")
header_positions = [i for i, l in enumerate(lines) if l.startswith("### ")]

def section_range(header_text):
    for idx, i in enumerate(header_positions):
        if f"`{header_text}`" in lines[i]:
            start = i
            end = header_positions[idx + 1] if idx + 1 < len(header_positions) else len(lines)
            return start, end
    raise ValueError(f"header não encontrado: {header_text}")

removed = 0
for h in secoes_indevidas:
    start, end = section_range(h)
    achou = False
    for i in range(start, end):
        if lines[i].strip() == nota:
            del lines[i]
            if i < len(lines) and lines[i].strip() == "":
                del lines[i]
            removed += 1
            achou = True
            break
    if not achou:
        raise ValueError(f"nota não encontrada na seção: {h}")

assert removed == 11, f"esperado remover 11, removi {removed}"

with open(path, "w", encoding="utf-8") as f:
    f.write("\n".join(lines))

print("OK, removidas", removed, "ocorrências indevidas.")
```

Depois de rodar, confirme com:
```bash
grep -n "Nota de contrato" "Documentação da API.md"
```
Isso deve retornar **exatamente uma linha**, dentro da seção `### \`POST /academia/faltas-aluno/async\``. Se retornar mais de uma ou nenhuma, o script não deve ser considerado concluído — revise manualmente antes de prosseguir.

### B. Testes da seção 5 da Tarefa 34 (13 testes, numeração original preservada para rastreabilidade)

Padrões de arquivo (iguais à v1, reconfirmados):
- Testes de agregado (sem banco): `internal/domain/aggregates/estudante_registros_correcao_test.go`, reaproveitando `estudanteParaRegistro()` já existente no pacote.
- Testes HTTP + banco real: `cmd/server/notas_faltas_correcao_integration_test.go`, com o guard `if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" { t.Skip(...) }` e reaproveitando/estendendo `setupRegistrosCorrecaoIntegration` (não duplique a criação de academia/estudante/matéria — o fixture atual já cria uma academia `fundamental` com um estudante e uma matéria `fundamental`; para os testes 4 e 5, que exigem tipo `superior`, você precisará inserir adicionalmente um curso e uma matéria `superior`, ver abaixo).

1. **Registrar falta tipo escolar com `periodo="1_trimestre"` — sucesso.** Teste de agregado. Chame `estudante.RegistrarFalta(...)` com `periodo="1_trimestre"` e `periodosValidos=aggregates.PeriodosEscolar`, confirme `err == nil`.

2. **Registrar falta tipo escolar com `periodo` fora do conjunto fixo (ex.: `"4_trimestre"`) — `400`.** Teste HTTP de integração: `POST /academia/faltas-aluno` com `periodo: "4_trimestre"`, confirme status `400`.

3. **Registrar falta tipo escolar sem `periodo` — `400`, mensagem listando o campo como obrigatório.** Teste HTTP: omita `periodo` no corpo do `POST /academia/faltas-aluno`, confirme `400` e que a mensagem de erro menciona `periodo` (o binding `binding:"required"` do campo já deve gerar isso — confirme a mensagem exata rodando o teste antes de fixar o `assert`, em vez de adivinhar a string).

4. **Registrar falta tipo superior com `periodo` igual ao período fixo já configurado na matéria — sucesso.** Requer fixture adicional: insira em `projection_cursos` um curso `type='superior'`, `periodos` (JSONB) contendo `["1_semestre","2_semestre"]`, associado à mesma `codigo_academia` do fixture; insira em `projection_materias` uma matéria `type='superior'`, `curso_id` apontando para esse curso, `periodo='1_semestre'`. Teste HTTP: `POST /academia/faltas-aluno` com essa `materia_disciplinar_id`, `periodo: "1_semestre"`, tipo inferido como superior — confirme `201`/sucesso.

5. **Registrar falta tipo superior com `periodo` diferente do período fixo da matéria — `400`, mensagem `"periodo '%s' invalido para a materia '%s'. Periodo definido: '%s'"`.** Mesmo fixture do teste 4, mas envie `periodo: "2_semestre"` (diferente do `"1_semestre"` fixado na matéria). Confirme `400` e a mensagem exata (reconfirmada nesta auditoria contra `internal/handlers/faltas_handlers.go` linha ~131).

6. **`PATCH /academia/faltas-aluno/:id` enviando `periodo` no corpo — rejeitado.** Teste HTTP: registre uma falta normalmente, depois tente `PATCH` incluindo `"periodo": "2_trimestre"` no corpo. Confirme que é rejeitado (o guard é `rejeitarCamposLegadosSumarioFaltas(c, "periodo")`, primeira linha de `CorrigirFalta` em `faltas_handlers.go` — confirme o status code exato rodando o handler, ele já está implementado e correto, o teste só precisa comprovar o comportamento existente).

7. **`PATCH /academia/faltas-aluno/:id` sem enviar `periodo` — sucesso, `periodo` do registro permanece o do lançamento original.** Registre uma falta com `periodo="1_trimestre"`, corrija quantidade/observação via `PATCH` sem tocar em `periodo`, confirme via `GET` subsequente que `periodo` continua `"1_trimestre"`.

8. **`GET /faltas` com filtro `?periodo=1_trimestre` — retorna somente faltas cujo próprio registro tem esse período.** Registre duas faltas do mesmo estudante em matérias/dias diferentes com períodos diferentes (`1_trimestre` e `2_trimestre`), confirme que o filtro retorna só a correta.

9. **`GET /faltas-estudante/:codigo?periodo=2_semestre` — mesmo comportamento acima, para o endpoint por estudante.** Análogo ao 8, usando `GetFaltasEstudante`.

10. **Rebuild da projeção `faltas` (`FaltasProjection.Rebuild()`) preserva `periodo` corretamente.** Registre faltas com período, chame `projections.NewFaltasProjection(client).Rebuild()`, confirme via `GetByID`/query direta que `periodo` continua correto após o rebuild.

11. **Duas faltas do mesmo estudante/matéria/data com `periodo` diferente — ambas aceitas (constraint `uq_falta_unica` agora inclui `periodo`, ver `migrations/107_periodo_faltas.sql` linha 50-52).** Registre a mesma falta (estudante, academia, data, matéria) duas vezes, uma com `periodo="1_trimestre"` e outra com `periodo="2_trimestre"`. Confirme que **ambas são aceitas** (`201` nas duas) — documente esse comportamento de borda no comentário do teste, já que antes da Tarefa 34 isso teria sido rejeitado como duplicata.

12. **Migração `107_periodo_faltas.sql` aplicada sobre uma base com faltas pré-existentes sem `periodo`.** ⚠️ **Este cenário mudou desde a v1** — a migração **não aborta mais** se restarem faltas sem período determinístico; ela deixa `periodo = NULL` para esses casos (ver Contexto, Problema C, acima). O teste deve confirmar: (a) uma falta pré-existente de matéria `tipo=superior` com `projection_materias.periodo` definido recebe o backfill determinístico correto; (b) uma falta pré-existente de matéria `tipo=fundamental`/`medio` (sem fonte determinística) fica com `periodo = NULL` no banco após a migração, **sem** a migração falhar/abortar; (c) depois da correção do item C.1 deste documento, essa falta com `periodo = NULL` no banco aparece normalmente via `GetByID`/`GetByEstudante` com `periodo=""` na resposta, em vez de desaparecer.

13. **Teste de regressão confirmando que `FaltasProjection.GetByPeriodo` (a função de intervalo de datas, não renomeada) continua funcionando sem alteração de comportamento, apenas retornando `periodo` a mais em cada `FaltaDTO`.** Chame `GetByPeriodo(codigoEstudante, anoLectivo, dataInicio, dataFim)` como já era chamado antes da Tarefa 34, confirme que o filtro por intervalo de datas continua correto e que o `FaltaDTO` retornado agora inclui `periodo` preenchido.

## Fora de escopo

- Qualquer alteração em `internal/domain/aggregates/estudante_falta.go` além da mudança pontual especificada no item C.2.
- Qualquer alteração em `internal/handlers/faltas_handlers.go` e `internal/handlers/registros_handlers.go` além dos pontos exatos do item C.1 (adicionar `COALESCE`).
- Qualquer alteração em `internal/projections/faltas_projection.go` além dos 5 pontos exatos do item C.1.
- Qualquer alteração em `migrations/107_periodo_faltas.sql` — a migração já está correta; o problema era só no código Go que a consome.
- Qualquer alteração em `Documentação da API.md` fora da remoção das 11 ocorrências indevidas listadas na seção A.
- Renomear ou alterar a assinatura de `FaltasProjection.GetByPeriodo`.
- Investigar por que a duplicação da nota de contrato ocorreu originalmente, ou por que o commit `d11b1fe` não incluiu a atualização do código Go — não é necessário para a correção.
- Qualquer mudança relacionada à Tarefa 35 (vínculo de estudante a turma) — isso é tratado em documento separado (`38 - Corrigir pendencias da Tarefa 35...`).

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual de `main`.
2. Aplicar a correção C.1 (`COALESCE`) nas 6 queries indicadas.
3. Aplicar a correção C.2 (fallback de chave legada) em `CorrigirFalta`.
4. Implementar os testes 14 e 15 (item C.3) e confirmar que passam — eles são a prova de que C.1/C.2 funcionam.
5. Aplicar a correção A em `Documentação da API.md` via o script Python fornecido; confirmar com `grep -n "Nota de contrato"` que sobra exatamente uma ocorrência.
6. Implementar os 13 testes da seção B (1 a 13), distribuindo entre testes de agregado e de integração HTTP conforme indicado em cada um.
7. Rodar `gofmt -l .` (sem saída) e `go build ./...`.
8. Rodar `go vet ./...`.
9. Rodar `go test ./...` e, para os testes de integração, `SPURI_RUN_DB_INTEGRITY_TESTS=1 go test ./... -run <padrão relevante>` contra uma base PostgreSQL isolada de testes.
10. Revisar o diff completo: só devem aparecer mudanças em `Documentação da API.md`, arquivos `*_test.go`, e as 3 mudanças cirúrgicas de código de produção especificadas em C.1/C.2 (`internal/projections/faltas_projection.go`, `internal/handlers/registros_handlers.go`, `internal/domain/aggregates/estudante_falta.go`).

## Critérios de aceite

- [ ] `Documentação da API.md` contém exatamente uma ocorrência da nota de contrato sobre `periodo` obrigatório em faltas, na seção `POST /academia/faltas-aluno/async`.
- [ ] As 11 seções listadas no Contexto (Problema A) não contêm mais essa nota.
- [ ] As 5 queries de `internal/projections/faltas_projection.go` e a query de `internal/handlers/registros_handlers.go` usam `COALESCE(f.periodo, '')`.
- [ ] `CorrigirFalta` em `estudante_falta.go` tem o fallback de chave legada (item C.2).
- [ ] Os 15 testes (13 da seção B + 14 e 15 de C.3) estão implementados, com nomes claros o suficiente para identificar qual cenário cada um cobre, e todos passam.
- [ ] Nenhum arquivo de código de produção foi alterado além dos 3 pontos exatos especificados (`internal/projections/faltas_projection.go`, `internal/handlers/registros_handlers.go`, `internal/domain/aggregates/estudante_falta.go`); nenhuma mudança em `migrations/...`.
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `go test ./...` passa; testes de integração que dependem de `SPURI_RUN_DB_INTEGRITY_TESTS=1` foram executados pelo menos uma vez contra uma base real e confirmados passando (documentar no PR).

## Procedimento de conclusão

Ao finalizar a implementação:

1. Atualizar o título interno deste arquivo para `# 37 — Corrigir pendências da Tarefa 34 (nota de contrato duplicada, testes de periodo ausentes e regressão crítica de leitura/correção de faltas históricas) — v2 FINAL (feito)`;
2. Alterar o front matter para `status: feito`;
3. Mover este arquivo para `docs/Tarefas feitas/`, substituindo/removendo a v1 se ela ainda estiver em `docs/Lista de Tarefas/` ou `docs/Tarefas feitas/`.
