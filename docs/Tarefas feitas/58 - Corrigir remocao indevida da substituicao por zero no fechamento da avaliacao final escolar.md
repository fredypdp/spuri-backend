---
criado: 2026-08-21
origem: docs/Debbugs/Depurar correcao das falhas criticas no motor de avaliacao final escolar.md
status: feito
tipo: correcao_critica_pre_producao
---

# Corrigir remoção indevida da substituição por zero no fechamento da avaliação final escolar

## Prompt recomendado para o Codex

> Execute exatamente as alterações descritas neste documento, nesta ordem, sem alterar nenhuma decisão de design. Todas as decisões já foram tomadas e validadas (build, vet, gofmt e suíte de testes completa já rodaram 100% verdes com esta correção aplicada, em ambiente com Go 1.24 e PostgreSQL 16 reais). Sua tarefa é puramente mecânica: aplicar os 3 diffs exatos abaixo, rodar as validações da seção "Checklist de validação" e reportar o resultado. Não é necessário PostgreSQL, Docker nem `psql` para nenhuma parte desta tarefa — todas as validações rodam com `go build`, `go vet`, `gofmt` e `go test` puros. Não toque em nenhum arquivo ou lógica fora do escopo listado na seção "Fora de escopo".

---

## Contexto

A Tarefa 57 corrigiu 3 falhas críticas no motor de avaliação final escolar (Achados 1, 2 e 3). Uma depuração profunda pós-implementação (ver `docs/Debbugs/Depurar correcao das falhas criticas no motor de avaliacao final escolar.md`) confirmou que os Achados 1 e 3 foram corrigidos corretamente, mas a correção do **Achado 2** removeu, sem perceber, o mecanismo de substituição por zero de referências ausentes — mecanismo que a própria Tarefa 57 (seção 4.3) instruiu explicitamente a preservar. Isso introduziu uma regressão nova: se qualquer categoria referenciada pela fórmula de avaliação final (ex.: `nota_professor` de algum trimestre) nunca receber uma nota real, o fechamento do ano trava silenciosamente e **permanece travado para sempre**, sem nenhum evento automático capaz de reabri-lo — pior do que o bug original que a Tarefa 57 tentou resolver.

Esta tarefa aplica a correção mínima e cirúrgica: restaurar a chamada à substituição por zero (`substituirNotasAusentesPorZero`), agora aplicada a **todas** as matérias aplicáveis do laço (não mais restrita apenas à matéria que recebeu a nota-gatilho, que era o comportamento anterior à Tarefa 57). Isso preserva integralmente a correção do Achado 1 (quando disparar o gatilho) e do Achado 2 (avaliar o conjunto completo de matérias, não só uma) — apenas troca "erro rígido e permanente" por "substituição por zero com registro de auditoria", exatamente como o sistema já operava, de forma aprovada, antes da Tarefa 57 (só que agora aplicado corretamente a todas as matérias, e apenas no momento certo, graças ao Achado 1).

**Esta correção já foi implementada e validada por Claude** em um clone local, com os seguintes resultados, todos confirmados nesta mesma sessão de depuração:
- `go build ./...` — sem erros.
- `go vet ./...` — sem erros.
- `gofmt -l .` — sem divergências.
- `go test ./...` — suíte completa do módulo, 100% verde, incluindo um teste novo que reproduz o cenário exato do bug (referência de fórmula nunca lançada) e prova que o cálculo agora conclui corretamente em vez de travar.

O Codex não precisa validar decisões de design — apenas aplicar os diffs abaixo, que já são o resultado final e testado.

---

## Resumo executivo

| # | Arquivo | Tipo de mudança |
|---|---|---|
| 1 | `internal/handlers/avaliacao_final_handler.go` | Restaurar substituição por zero; remover código morto (`notasFormulaCompletas` e o catch de `"nota ausente"`) |
| 2 | `internal/handlers/avaliacao_final_formula_test.go` | Substituir teste que validava a função isolada por teste que valida o fluxo integrado |
| 3 | `Documentação da API.md` | Corrigir descrição que contradizia o comportamento real do código (seções 15.1.3 e 15.1.5) |

Nenhum arquivo novo é criado nesta tarefa (exceto os documentos de conclusão, ao final). Nenhum arquivo é removido.

---

## 1. `internal/handlers/avaliacao_final_handler.go`

Esta alteração tem **duas** localizações independentes no mesmo arquivo. Aplique as duas.

### 1.1 — Remover o catch morto de `"nota ausente"` em `tentarAvaliacoesFinaisAutomaticas`

Localize o seguinte trecho **exato** (dentro da função `tentarAvaliacoesFinaisAutomaticas`, no laço `for _, regra := range regras`, logo após a chamada a `executarRegraAvaliacaoFinalAutomatica`):

**Bloco a localizar (antes):**
```go
			resultado, registrado, err := executarRegraAvaliacaoFinalAutomatica(
				c,
				estudante,
				estudanteDTO,
				codigoAcademia,
				anoLectivo,
				tipoEnsino,
				anoAcademicoAtual,
				regra,
				overlay,
			)
			if err != nil {
				if strings.Contains(err.Error(), "nota ausente") {
					continue
				}
				return resultados, err
			}
			if !registrado {
				continue
			}
```

**Substituir por (depois):**
```go
			resultado, registrado, err := executarRegraAvaliacaoFinalAutomatica(
				c,
				estudante,
				estudanteDTO,
				codigoAcademia,
				anoLectivo,
				tipoEnsino,
				anoAcademicoAtual,
				regra,
				overlay,
			)
			if err != nil {
				return resultados, err
			}
			if !registrado {
				continue
			}
```

**Motivo:** a partir da correção da seção 1.2 abaixo, `executarRegraAvaliacaoFinalAutomatica` (via `calcularResultadoMateriasAvaliacaoFinal`) nunca mais produz um erro contendo a substring `"nota ausente"` — essa mensagem de erro deixa de existir no código. O bloco `if strings.Contains(...)` fica morto (inalcançável) e deve ser removido para não confundir leitura futura do código.

**Atenção:** não remova o import `"strings"` do arquivo — ele continua sendo usado em outros pontos do mesmo arquivo (ex.: `strings.Contains(err.Error(), "já registrada")` mais abaixo, e outras normalizações de string). Se o `go build`/`go vet` acusar `"strings" imported and not used`, é sinal de que algo mais foi removido incorretamente — reverta e reaplique apenas o bloco acima.

### 1.2 — Restaurar a substituição por zero em `calcularResultadoMateriasAvaliacaoFinal`

Localize o seguinte trecho **exato** (dentro da função `calcularResultadoMateriasAvaliacaoFinal`, no laço `for _, materia := range materias`, logo após a aplicação do `overlay`):

**Bloco a localizar (antes):**
```go
		notasSubstituidasZero := []aggregates.NotaReferenciaAvaliacaoFinal{}
		if !notasFormulaCompletas(formulaExecucao, notasFormula) {
			return nil, 0, false, false, nil, fmt.Errorf("matéria %s: nota ausente para fechamento da avaliação final", materia.ID)
		}
		nota, err := calcularFormulaAvaliacao(formulaExecucao, notasFormula)
		if err != nil {
			return nil, 0, false, false, nil, fmt.Errorf("matéria %s: %w", materia.ID, err)
		}
```

**Substituir por (depois):**
```go
		notasSubstituidasZero, err := substituirNotasAusentesPorZero(formulaExecucao, notasFormula)
		if err != nil {
			return nil, 0, false, false, nil, fmt.Errorf("matéria %s: %w", materia.ID, err)
		}
		nota, err := calcularFormulaAvaliacao(formulaExecucao, notasFormula)
		if err != nil {
			return nil, 0, false, false, nil, fmt.Errorf("matéria %s: %w", materia.ID, err)
		}
```

**Atenção — variável `err` reaproveitada, não redeclarada:** a linha `notasSubstituidasZero, err := substituirNotasAusentesPorZero(...)` usa `:=` porque `notasSubstituidasZero` é uma variável nova neste escopo — mas `err` já existe no mesmo bloco (foi declarada poucas linhas acima, em `notasFormula, err := carregarNotasFormulaMateria(...)`). Isso é válido em Go (uma variável nova no lado esquerdo do `:=` é suficiente para reaproveitar as demais no mesmo escopo) e **não deve gerar erro de "redeclared"**. Se o `go vet`/`go build` acusar erro de redeclaração, confirme que este bloco está exatamente dentro do mesmo laço `for _, materia := range materias` e não foi movido para um escopo diferente.

### 1.3 — Remover a função `notasFormulaCompletas` (código morto)

Logo após o fechamento da função `calcularResultadoMateriasAvaliacaoFinal` (que termina com `return resultados, soma / float64(len(resultados)), aprovado, aprovadoComPendencia, pendencias, nil` seguido de `}`), existe a seguinte função, que passa a ser **código morto** (zero chamadas em produção após as seções 1.1 e 1.2 acima):

**Bloco a remover (por completo):**
```go
func notasFormulaCompletas(formula string, notas map[string]map[string][]float64) bool {
	refs, err := referenciasFormulaAvaliacao(formula)
	if err != nil {
		return false
	}
	for _, ref := range refs {
		if len(notas[ref.Categoria][ref.Periodo]) == 0 {
			return false
		}
	}
	return true
}

```

Remova esse bloco inteiro (incluindo a linha em branco final antes da próxima função, `func referenciasFormulaAvaliacao(...)`, que deve permanecer intacta imediatamente após a remoção).

**Não remova** `referenciasFormulaAvaliacao` nem `substituirNotasAusentesPorZero` — ambas continuam em uso (a primeira é usada por `substituirNotasAusentesPorZero` e por `periodoFechamentoCategoriaNaFormula`; a segunda passa a ser chamada pela seção 1.2 acima).

---

## 2. `internal/handlers/avaliacao_final_formula_test.go`

Localize o seguinte teste, no final do arquivo:

**Bloco a localizar (antes):**
```go
func TestNotasFormulaCompletasNaoRemoveSubstituicaoPorZero(t *testing.T) {
	notas := map[string]map[string][]float64{"nota_professor": {"1_trimestre": {8}}}
	if notasFormulaCompletas("([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre])/2", notas) {
		t.Fatal("fórmula incompleta não deve estar pronta para fechamento automático")
	}
	faltantes, err := substituirNotasAusentesPorZero("([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre])/2", notas)
	if err != nil {
		t.Fatalf("substituirNotasAusentesPorZero retornou erro: %v", err)
	}
	if len(faltantes) != 1 || faltantes[0].Categoria != "prova_trimestral" || faltantes[0].Periodo != "1_trimestre" {
		t.Fatalf("faltantes inesperados: %#v", faltantes)
	}
}
```

**Substituir por (depois):**
```go
func TestSubstituirNotasAusentesPorZeroContinuaAtivaAposFechamentoPorPeriodo(t *testing.T) {
	formula := "([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre]+[nota_professor,2_trimestre]+[prova_trimestral,2_trimestre]+[nota_professor,3_trimestre]+[prova_trimestral,3_trimestre])/6"
	notas := map[string]map[string][]float64{
		"prova_trimestral": {
			"1_trimestre": {7},
			"2_trimestre": {8},
			"3_trimestre": {9},
		},
		// nota_professor nunca foi lançada em nenhum trimestre (cenário real: professor esqueceu).
	}

	faltantes, err := substituirNotasAusentesPorZero(formula, notas)
	if err != nil {
		t.Fatalf("substituirNotasAusentesPorZero retornou erro: %v", err)
	}
	if len(faltantes) != 3 {
		t.Fatalf("esperava 3 referências substituídas por zero (nota_professor x3), obteve: %#v", faltantes)
	}

	nota, err := calcularFormulaAvaliacao(formula, notas)
	if err != nil {
		t.Fatalf("cálculo da fórmula não deveria falhar mesmo com nota_professor ausente: %v", err)
	}
	esperado := (0.0 + 7 + 0.0 + 8 + 0.0 + 9) / 6
	if nota != esperado {
		t.Fatalf("nota final inesperada: got=%v want=%v", nota, esperado)
	}
}
```

**Motivo:** o teste anterior chamava `notasFormulaCompletas`, função removida na seção 1.3. O teste novo prova o comportamento correto de ponta a ponta: uma fórmula com uma categoria inteira (`nota_professor`) nunca lançada em nenhum dos 3 trimestres ainda assim é calculada com sucesso, com as 3 referências ausentes substituídas por zero — reproduzindo exatamente o cenário real que travava permanentemente antes desta correção.

---

## 3. `Documentação da API.md`

Duas correções pontuais na seção 15, para que a documentação volte a refletir o comportamento real do código.

### 3.1 — Seção 15.1.3

**Localize o parágrafo exato (antes):**
```
A execução automática só dispara quando a nota recém-lançada corresponde à categoria despertadora **e ao período de fechamento** dessa categoria na fórmula da regra. No escolar fixo, isso impede que `prova_trimestral` do 1º ou 2º trimestre feche indevidamente anos sem exame: o fechamento regular ocorre com `prova_trimestral` do `3_trimestre`; anos com exame fecham com `exame_final` do `3_trimestre`; recurso fecha com `exame_recurso` do `3_trimestre`; e o 4º ano médio técnico fecha com `nota_pap` do `3_trimestre`. Antes de registrar a avaliação automática, o backend também exige que todas as matérias aplicáveis tenham as referências da fórmula preenchidas; assim a primeira matéria lançada não consome sozinha a idempotência do ano/tipo. A substituição por zero continua existindo para execuções de fechamento quando a fórmula for deliberadamente calculada com referências ausentes, e as lacunas são registradas em `resultados_materias.notas_substituidas_zero`. A fórmula sempre lê notas do ano letivo atual, da mesma academia, do mesmo estudante, da matéria avaliada e de categorias extraídas da própria fórmula.
```

**Substituir por (depois):**
```
A execução automática só dispara quando a nota recém-lançada corresponde à categoria despertadora **e ao período de fechamento** dessa categoria na fórmula da regra. No escolar fixo, isso impede que `prova_trimestral` do 1º ou 2º trimestre feche indevidamente anos sem exame: o fechamento regular ocorre com `prova_trimestral` do `3_trimestre`; anos com exame fecham com `exame_final` do `3_trimestre`; recurso fecha com `exame_recurso` do `3_trimestre`; e o 4º ano médio técnico fecha com `nota_pap` do `3_trimestre`. Assim que esse gatilho de fechamento dispara para qualquer matéria aplicável, o backend calcula **todas** as matérias aplicáveis daquele ano/tipo em um único evento idempotente — não apenas a matéria que recebeu a nota-gatilho. Qualquer referência da fórmula que ainda não tenha nota registrada, em qualquer matéria aplicável, é calculada como `0` naquele momento (a substituição por zero não fica restrita à matéria despertadora); as lacunas preenchidas são registradas em `resultados_materias.notas_substituidas_zero`. Isso significa que a matéria/momento cujo lançamento efetivamente dispara o fechamento do ano determina, na prática, quais lacunas de outras matérias ainda pendentes são zeradas — comportamento intencional para evitar que o ano fique indefinidamente pendente por uma nota nunca lançada. A fórmula sempre lê notas do ano letivo atual, da mesma academia, do mesmo estudante, da matéria avaliada e de categorias extraídas da própria fórmula.
```

### 3.2 — Seção 15.1.5 (observações sobre anos sem exame)

**Localize a linha exata (antes):**
```
- Nos anos sem exame, somente a `prova_trimestral` do 3º trimestre é período de fechamento; lançamentos de `prova_trimestral` no 1º ou 2º trimestre não disparam avaliação final automática. O registro automático só ocorre quando todas as matérias aplicáveis possuem os dados exigidos pela fórmula da etapa.
```

**Substituir por (depois):**
```
- Nos anos sem exame, somente a `prova_trimestral` do 3º trimestre é período de fechamento; lançamentos de `prova_trimestral` no 1º ou 2º trimestre não disparam avaliação final automática. Quando esse gatilho dispara para qualquer matéria aplicável, o backend fecha o conjunto inteiro de matérias aplicáveis naquele mesmo momento, zerando (`notas_substituidas_zero`) qualquer referência de fórmula ainda sem nota lançada em qualquer uma delas.
```

---

## Fora de escopo (não altere)

- Qualquer lógica do Achado 1 (`regraDespertadaPorNota`, `periodoFechamentoCategoriaNaFormula`, `ordemPeriodoAvaliacao`, `tiposAvaliacaoFinalDespertadosPorCategoria`) — já está correta, não precisa de nenhuma mudança.
- Qualquer lógica do Achado 3 (validação de `4_ano_medio`/PAP em `materia_disciplinar.go` e `materia_disciplinar_handlers.go`) — já está correta, não precisa de nenhuma mudança.
- A validação do aggregate `MateriaDisciplinar` não ter mais checagem própria de `4_ano_medio` (observação registrada como dívida técnica não bloqueante na depuração) — **não corrigir nesta tarefa**, fora de escopo.
- Qualquer arquivo não listado nas seções 1, 2 e 3 acima.
- Qualquer refatoração adicional, renomeação de variáveis, ou "melhoria" não explicitamente pedida neste documento.
- Não crie nenhum mecanismo novo de acumulação progressiva por matéria, nem novo tipo de evento — a correção pedida é apenas a restauração da chamada a `substituirNotasAusentesPorZero`, exatamente como especificado na seção 1.2.

---

## Checklist de validação (Codex deve executar e reportar o resultado de cada item)

Nenhum destes comandos requer PostgreSQL, Docker ou `psql`:

1. `go build ./...` — deve terminar sem erros.
2. `go vet ./...` — deve terminar sem erros.
3. `gofmt -l .` — deve retornar vazio (nenhum arquivo fora do padrão).
4. `go test ./...` — deve passar 100%, sem nenhuma regressão em nenhum pacote (não apenas `internal/handlers`).
5. `go test ./internal/handlers/... -run TestSubstituirNotasAusentesPorZeroContinuaAtivaAposFechamentoPorPeriodo -v` — deve passar e a saída deve mostrar o teste executando com sucesso.
6. Busca de confirmação de que não sobrou nenhuma referência a `notasFormulaCompletas` no repositório: `grep -rn "notasFormulaCompletas" --include="*.go" .` deve retornar **vazio** (nenhuma ocorrência).

Se qualquer um desses itens falhar, **não prossiga** — reporte o erro exato ao invés de tentar corrigir com uma solução diferente da especificada neste documento.

---

## Critérios de aceite

- [ ] Os 2 blocos da seção 1.1 e 1.2 aplicados exatamente como especificado em `internal/handlers/avaliacao_final_handler.go`.
- [ ] A função `notasFormulaCompletas` removida por completo (seção 1.3).
- [ ] O teste da seção 2 substituído exatamente como especificado.
- [ ] As duas correções de texto da seção 3 aplicadas em `Documentação da API.md`.
- [ ] Todos os 6 itens do checklist de validação executados e reportados com sucesso.
- [ ] Nenhum arquivo fora do escopo desta tarefa foi alterado.
- [ ] `git diff --stat` mostra alterações **apenas** nos 3 arquivos listados no resumo executivo (mais os documentos de conclusão da seção seguinte).

---

## Procedimento de conclusão

1. Após todos os critérios de aceite acima estarem satisfeitos, mover este arquivo de `docs/Lista de Tarefas/` para `docs/Tarefas feitas/`, atualizando o frontmatter: `status: concluido` e adicionando `concluido: <data de hoje>`.
2. Atualizar o frontmatter de `docs/Debbugs/Depurar correcao das falhas criticas no motor de avaliacao final escolar.md`, campo `status`, de `achado_2_com_regressao_critica_correcao_pendente` para `achado_2_corrigido_via_tarefa_58`.
3. Criar um commit único contendo todas as alterações, com mensagem: `Corrigir remoção indevida da substituição por zero no fechamento da avaliação final escolar`.
4. Reportar ao Fredy: resultado de cada item do checklist de validação, e a lista de arquivos alterados (`git diff --stat` do commit).

**Não é necessário** nenhuma validação adicional com PostgreSQL real — a Claude já validou esta correção especificamente com PostgreSQL real e servidor real em execução, além da suíte de testes completa. Esta tarefa é de execução mecânica de um resultado já testado e aprovado.
