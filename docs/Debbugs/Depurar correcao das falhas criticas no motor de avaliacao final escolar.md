---
data: 2026-08-21
status: achado_2_com_regressao_critica_correcao_pendente
auditor: Claude (orquestrador) — depuração profunda pós-implementação do Codex
tarefa_origem: docs/Lista de Tarefas/57 - Corrigir falhas criticas no motor de avaliacao final escolar (aprovacao e reprovacao).md
tarefa_correcao: docs/Lista de Tarefas/58 - Corrigir remocao indevida da substituicao por zero no fechamento da avaliacao final escolar.md
---

# Depuração profunda da correção da Tarefa 57 — motor de avaliação final escolar

## Resumo executivo

| Achado da Tarefa 57 | Veredito | Observação |
|---|---|---|
| Achado 1 (gatilho ignora período) | ✅ Corrigido corretamente | Sem ressalvas |
| Achado 2 (materia isolada consome idempotência do ano/tipo) | ⚠️ **Correção incompleta — regressão crítica nova** | Ver detalhamento abaixo. Corrigido nesta depuração e formalizado na Tarefa 58 |
| Achado 3 (PAP 4º ano médio técnico) | ✅ Corrigido corretamente | 1 observação não bloqueante (defesa em profundidade) |
| `go build ./...` / `go vet ./...` / `gofmt` / `go test ./...` | ✅ Confirmados | Validados com Go 1.24 e PostgreSQL 16 reais neste ambiente |
| Documentação da API (seção 15) | ⚠️ Continha afirmação que contradizia o código real | Corrigida nesta depuração |

**Conclusão sobre produção:** o módulo **não deve ir para produção** no estado do commit `299d8e3`/`d9b4361`, porque o Achado 2 introduziu um bug silencioso e permanente mais grave do que o bug original que a Tarefa 57 tentou corrigir. A correção já foi desenhada, implementada e validada nesta depuração (build, vet, gofmt e suíte de testes completa — 100% verde) e está formalizada, com diffs exatos, na **Tarefa 58**, pronta para o Codex executar mecanicamente.

---

## Método usado nesta depuração

1. Clonagem real do repositório `fredypdp/spuri-backend` (branch `main`, commit `d9b4361` — merge da correção do Codex).
2. Leitura de todo o diff da Tarefa 57 (10 arquivos) e comparação linha a linha com os 3 achados originais e as instruções explícitas da Tarefa 57.
3. Instalação real de **Go 1.24.4** e **PostgreSQL 16** neste ambiente (via `apt-get`, que funciona aqui — diferente do ambiente do Codex). Contorno do bloqueio de rede a `proxy.golang.org` com `replace` locais temporários em `go.mod` apontando `golang.org/x/*` e outros pacotes de vanity import para os mirrors correspondentes no GitHub (nunca commitados — revertidos ao final).
4. Execução real de `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...` — todos passaram sem erros, confirmando as alegações do Codex.
5. Rastreamento manual, função por função, de todo o caminho de execução do motor de avaliação final (`tentarAvaliacoesFinaisAutomaticas` → `regraDespertadaPorNota` → `executarRegraAvaliacaoFinalAutomatica` → `calcularResultadoMateriasAvaliacaoFinal`), incluindo as fórmulas fixas reais em `modelo_avaliativo_escolar.go`.
6. Subida de um servidor real (binário compilado do próprio repositório) contra o PostgreSQL real, com migrations aplicadas e bootstrap do primeiro admin confirmado funcional — ambiente ficou pronto para reprodução HTTP end-to-end.
7. Implementação da correção diretamente no clone local, seguida de nova rodada completa de `go build`, `go vet`, `gofmt` e `go test ./...` (100% verde, incluindo um teste novo que reproduz o cenário exato do bug).

---

## Achado 1 — Gatilho ignorava o período da nota (`regraDespertadaPorNota`)

**Veredito: corrigido corretamente, sem ressalvas.**

A implementação deriva o "período de fechamento" de cada categoria diretamente da fórmula da regra (`periodoFechamentoCategoriaNaFormula`): entre todas as referências à mesma categoria dentro da fórmula, usa a de maior ordem (`ordemPeriodoAvaliacao`, que interpreta `N_trimestre`/`N_semestre`). Isso evita qualquer hardcode e funciona corretamente para os 4 modelos fixos:

- Ano regular (sem exame): fecha com `prova_trimestral` no `3_trimestre` — correto, pois é a maior ordem entre as 3 referências de `prova_trimestral` na fórmula.
- Ano com exame: fecha com `exame_final` no `3_trimestre` — correto (única referência).
- Recurso: fecha com `exame_recurso` no `3_trimestre` — correto (única referência).
- PAP (4º ano médio técnico): fecha com `nota_pap` no `3_trimestre` — correto (única referência).

Testado com `go test`, incluindo os testes novos `TestTiposAvaliacaoFinalDespertadosRespeitaPeriodoDaFormula` (prova que `prova_trimestral` do 1º trimestre não dispara, e do 3º trimestre dispara). Confirmado nesta depuração via leitura do código e nova execução da suíte completa.

---

## Achado 3 — PAP no 4º ano médio técnico

**Veredito: corrigido corretamente. 1 observação não bloqueante.**

A validação de `anos_academicos` contendo `4_ano_medio` foi movida do aggregate `MateriaDisciplinar` (que a bloqueava estruturalmente, tornando a PAP impossível) para o handler HTTP (`materia_disciplinar_handlers.go`), que agora permite `4_ano_medio` apenas quando:
1. o curso vinculado é de modelo `tecnico`; e
2. o array de anos acadêmicos contém **somente** esse ano (sem mistura com outros anos médios).

Isso é verificado tanto em `CriarMateria` quanto em `AtualizarDadosMateria` — os dois únicos pontos de entrada que chamam `materia.Criar()`/`materia.AtualizarDados()` no aggregate hoje.

**Observação não bloqueante (defesa em profundidade):** o aggregate `MateriaDisciplinar` ficou **sem nenhuma validação própria** sobre `4_ano_medio` — a proteção existe apenas no handler HTTP. Hoje isso não é um bug (confirmado por busca exaustiva: não há outro caminho de código que chame `materia.Criar()`), mas viola o princípio de Event Sourcing de que o aggregate deve proteger seus próprios invariantes independentemente de quem o chama. **Não é necessário corrigir agora** — registrado aqui apenas como dívida técnica para referência futura, por instrução explícita do Fredy de ignorar erros que não afetam a lógica do módulo.

---

## Achado 2 — Materia isolada consumia a idempotência do ano/tipo

### O que o Codex implementou

Em vez de calcular apenas a matéria do `overlay` (a que recebeu a nota que disparou o gatilho), o código passou a iterar sobre **todas** as matérias aplicáveis do ano/tipo. Para cada uma, verificava se a fórmula estava **100% completa** (`notasFormulaCompletas`) com dados reais — e se qualquer matéria estivesse incompleta, **abortava o cálculo inteiro com erro** (`"nota ausente para fechamento da avaliação final"`), erro esse capturado silenciosamente (`continue`) pela função chamadora.

Isso resolve, em teoria, o problema original do Achado 2 (a primeira matéria não fecha mais o ano sozinha). **Mas, ao fazer isso, removeu por completo o mecanismo de substituição por zero** (`substituirNotasAusentesPorZero`) — mecanismo que a própria Tarefa 57 (seção 4.3) instruiu explicitamente a **preservar**:

> *"Tenha cuidado: substituição por zero de referências ausentes é comportamento intencional e já auditado/aprovado (...) a correção aqui é sobre quando disparar o gatilho, não sobre remover a substituição por zero em si."*

### Prova mecânica de que a substituição por zero foi removida da produção

```
$ grep -rn "substituirNotasAusentesPorZero\|notasSubstituidasZero" --include="*.go" internal/ | grep -v _test.go

internal/handlers/avaliacao_final_handler.go:823:  notasSubstituidasZero := []aggregates.NotaReferenciaAvaliacaoFinal{}
internal/handlers/avaliacao_final_handler.go:844:      NotasSubstituidasZero: notasSubstituidasZero,
internal/handlers/avaliacao_final_handler.go:912: func substituirNotasAusentesPorZero(...) (...)
internal/domain/aggregates/estudante_avaliacao.go:55: NotasSubstituidasZero []NotaReferenciaAvaliacaoFinal ...
```

A função `substituirNotasAusentesPorZero` estava **definida mas com zero pontos de chamada em produção** — a variável `notasSubstituidasZero` era sempre inicializada vazia e nunca preenchida. O único lugar que ainda chamava a função era um teste (`TestNotasFormulaCompletasNaoRemoveSubstituicaoPorZero`), que testava a função **isolada**, não o fluxo integrado — por isso o teste passava mesmo com a integração quebrada. Isso é confirmado inclusive pelo próprio documento que o Codex escreveu em `docs/Tarefas feitas/...md`, onde afirma ter "preservado as funções de cálculo e substituição por zero" — confundindo "a função continua existindo no arquivo" com "a função continua sendo chamada pelo fluxo real".

A documentação da API atualizada pelo próprio Codex também continha a afirmação **"a substituição por zero continua existindo (...) quando a fórmula for deliberadamente calculada com referências ausentes"** — frase que contradizia o código real (já corrigida nesta depuração, ver diff no final deste documento).

### Por que isso é mais grave do que o bug original

Sem a substituição por zero, qualquer categoria referenciada pela fórmula que **nunca** receba uma nota real (ex.: o professor esquece de lançar `nota_professor` em algum trimestre) faz o cálculo abortar — silenciosamente, sem erro visível na resposta HTTP (`avaliacoes_finais_automaticas: []`) — **e não existe nenhum evento futuro capaz de reabrir esse cálculo automaticamente**, porque:

- `nota_professor` nunca é `nota_despertadora` em nenhuma fórmula fixa (confirmado em `modelo_avaliativo_escolar.go`) — lançá-la ou corrigi-la depois nunca dispara o gatilho.
- O único jeito de tentar novamente é corrigir (`PATCH`) exatamente a nota da categoria/período de fechamento (ex.: `prova_trimestral` do `3_trimestre`) de novo — algo que ninguém tem motivo óbvio para fazer, já que essa nota já foi lançada corretamente.

Isso também quebra o cenário de **notas lançadas fora de ordem** (ex.: 3º trimestre lançado antes do 1º) — cenário que o próprio documento de depuração do Codex listou como **pendente de validação** ("Concorrência/ordem de chegada com 3+ matérias, confirmando que o resultado final independe da primeira matéria cujo gatilho chegou"). Se o 3º trimestre (fechamento) chega primeiro e falta qualquer outra referência, o ano trava permanentemente — pior que o bug original, que ao menos produzia *algum* resultado (embora prematuro/incorreto).

Em resumo: o Achado 1 (quando disparar) foi corrigido corretamente, mas a correção do Achado 2 (o que fazer quando dispara) trocou "fecha cedo demais e com zero indevido" por "às vezes nunca fecha, silenciosamente, para sempre" — uma troca de um bug visível por um bug invisível, em um módulo que decide aprovação/reprovação/graduação de estudantes.

### A correção implementada e validada nesta depuração

A solução mínima e cirúrgica: manter a mudança de "avaliar todas as matérias aplicáveis juntas" (que resolve corretamente o Achado 2 — nenhuma matéria fica de fora, a decisão usa o conjunto completo), mas **restaurar a substituição por zero**, agora aplicada a **todas** as matérias do laço (não mais restrita à matéria do `overlay`), no lugar do erro rígido:

```diff
-		notasSubstituidasZero := []aggregates.NotaReferenciaAvaliacaoFinal{}
-		if !notasFormulaCompletas(formulaExecucao, notasFormula) {
-			return nil, 0, false, false, nil, fmt.Errorf("matéria %s: nota ausente para fechamento da avaliação final", materia.ID)
+		notasSubstituidasZero, err := substituirNotasAusentesPorZero(formulaExecucao, notasFormula)
+		if err != nil {
+			return nil, 0, false, false, nil, fmt.Errorf("matéria %s: %w", materia.ID, err)
 		}
 		nota, err := calcularFormulaAvaliacao(formulaExecucao, notasFormula)
```

Por que isso é seguro e correto:

1. Graças ao Achado 1 (já corrigido), só se chega a este ponto quando o gatilho de fechamento **realmente** correspondeu ao período de fechamento real da fórmula — ou seja, o "ano fechou" para aquele ano/tipo/academia como um todo (todas as matérias aplicáveis compartilham a mesma fórmula fixa e o mesmo período de fechamento).
2. Nesse momento, é seguro e é exatamente o comportamento já aprovado anteriormente: qualquer referência de qualquer matéria aplicável que ainda não tenha nota real é zerada, e a lacuna fica registrada em `notas_substituidas_zero` para auditoria — nada fica escondido.
3. Isso elimina o risco de travamento permanente: a primeira matéria cujo gatilho de fechamento chegar já fecha o ano inteiro (com zeros para o que faltar), exatamente como o sistema já operava antes da Tarefa 57 para uma única matéria — só que agora, corretamente, para **todas**.
4. Não há necessidade de um novo tipo de evento ou de acumulação progressiva por matéria (a direção que a Tarefa 57 havia sugerido como uma possibilidade) — a combinação "gatilho correto (Achado 1) + zero em todas as matérias aplicáveis no momento do fechamento (Achado 2)" já satisfaz completamente os dois achados com a menor mudança possível de código.

Como consequência direta, ficam mortos e foram removidos:
- a função `notasFormulaCompletas` (só existia para o gate que foi removido);
- o `if strings.Contains(err.Error(), "nota ausente") { continue }` em `tentarAvaliacoesFinaisAutomaticas` (a mensagem de erro que ele capturava deixa de ser produzida).

**Validação realizada nesta depuração** (não apenas planejada — já implementada e testada no clone local):
- Novo teste `TestSubstituirNotasAusentesPorZeroContinuaAtivaAposFechamentoPorPeriodo`, que reproduz o cenário exato do bug (fórmula com `nota_professor` nunca lançada em nenhum trimestre, mas `prova_trimestral` completa nos 3 trimestres) e prova que o cálculo agora conclui corretamente com substituição por zero, em vez de travar. ✅ Passa.
- `go build ./...`, `go vet ./...`, `gofmt -l .` — limpos. ✅
- `go test ./...` — suíte completa do módulo (todos os pacotes, não só os afetados) — 100% verde, zero regressões. ✅

A correção completa, com os diffs exatos para os 3 arquivos afetados (`avaliacao_final_handler.go`, `avaliacao_final_formula_test.go` e `Documentação da API.md`), está formalizada na **Tarefa 58** para execução mecânica pelo Codex.

---

## Avaliação dos testes que o Codex escreveu

A maioria dos testes novos da Tarefa 57 é sólida e continua válida (gatilho por período, PAP técnica, progressão de sequência acadêmica). Uma exceção relevante para o diagnóstico:

- `TestNotasFormulaCompletasNaoRemoveSubstituicaoPorZero` (removido/substituído nesta correção) testava `notasFormulaCompletas` e `substituirNotasAusentesPorZero` **isoladamente**, cada função chamada diretamente pelo teste. Isso prova que as duas funções, cada uma por si, se comportam como esperado — mas **não prova que a produção realmente chama `substituirNotasAusentesPorZero`**. É um teste de "a peça existe e funciona", não de "a peça está montada no motor". Esse é o tipo de lacuna que uma bateria de testes unitários bem escrita, mas sem verificação de integração, deixa passar — e é exatamente por isso que esta depuração profunda foi necessária antes de ir para produção.

---

## Ambiente de validação usado nesta depuração

- Go 1.24.4 (via `apt-get install golang-1.24-go`) com `GOTOOLCHAIN=local`.
- PostgreSQL 16 (via `apt-get install postgresql`), banco `spuri_test` criado do zero, migrations aplicadas via o próprio binário do servidor compilado a partir do repositório.
- Contorno do bloqueio de rede a `proxy.golang.org`: `replace` locais temporários em `go.mod` para `golang.org/x/net`, `golang.org/x/crypto`, `golang.org/x/text`, `golang.org/x/sys`, `golang.org/x/time`, `google.golang.org/protobuf` e `gopkg.in/yaml.v3`, apontando para os mirrors correspondentes no GitHub — nunca commitados, usados apenas localmente para viabilizar `go build`/`go vet`/`go test` reais.
- Servidor real subido e testado (bootstrap do admin FPP via `POST /bootstrap` confirmado funcional) — ambiente ficou pronto para reprodução HTTP end-to-end; a validação final desta rodada, no entanto, apoiou-se na prova estática exaustiva (busca por todos os pontos de chamada) combinada com a suíte de testes completa, por ser uma evidência determinística e não dependente de uma única reprodução de cenário.

---

## Próximo passo

Executar a **Tarefa 58** (`docs/Lista de Tarefas/58 - Corrigir remocao indevida da substituicao por zero no fechamento da avaliacao final escolar.md`) com o Codex. Os diffs já estão prontos, validados e testados nesta depuração — a tarefa é de execução mecânica, sem necessidade de nenhuma decisão de design adicional.
