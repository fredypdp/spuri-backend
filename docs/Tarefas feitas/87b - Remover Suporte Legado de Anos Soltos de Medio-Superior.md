---
criado: 06-87-2026
origem: Fredy + Claude (orquestração)
status: pronto para execução
tipo: backend (spuri-backend)
depende_de: Tarefa 87 — Serviços Extras: disponibilidade por curso + anos combinados (já implementada)
---

# Tarefa 87b — Remover suporte legado de anos soltos de médio/superior em Serviços Extras

### Documento de execução para o Codex

## 0. Contexto

A Tarefa 87 preservou, por decisão de design 3, a possibilidade de `anos_academicos_disponiveis` conter entradas soltas `_ano_medio`/`_ano_superior` (sem curso associado) — comportamento herdado de antes da Tarefa 87, quando esse campo não tinha cruzamento com curso nenhum. Fredy confirmou que **não existe nenhum registro no banco usando esse formato solto de médio/superior** — então esse suporte de compatibilidade deixou de fazer sentido e deve ser removido: daqui para frente, `anos_academicos_disponiveis` só aceita anos de ensino fundamental; médio e superior são **sempre** obrigatoriamente escopados a um curso via `cursos_disponiveis`.

Conferi o código atual (já implementado pela Tarefa 87) antes de escrever isto — os trechos abaixo batem exatamente com o que está no repositório agora.

## 1. Prompt recomendado

> Aplique exatamente as mudanças descritas, na ordem das seções. Ao final, rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test -p 1 ./...`, corrija qualquer erro, e confirme o checklist da seção 5.

## 2. Decisão de design

Sem migração de dados (nada cadastrado usando o formato antigo, conforme Fredy confirmou) e sem mudança de schema — isto é puramente aperto de validação e remoção de branch morto no Go. `cursos_disponiveis` e sua validação de posse/consistência (`validarPosseCursosDisponiveis`) não mudam em nada.

## 3. `internal/domain/aggregates/servico_extra.go`

**Localizar:**
```go
// validarAnosAcademicosServicoExtra valida apenas o FORMATO de cada ano
// informado, despachando para o validador correto pelo sufixo. Lista vazia
// é válida e significa "disponível para todos os anos" — não validar como
// erro. Deliberadamente NÃO cruza com cursos/turmas reais da academia (ver
// decisão de design 7 no documento da tarefa).
func validarAnosAcademicosServicoExtra(anos []string) error {
	for _, ano := range anos {
		switch {
		case strings.HasSuffix(ano, "_ano_fundamental"):
			if err := utils.ValidateAnoFundamental(ano); err != nil {
				return err
			}
		case strings.HasSuffix(ano, "_ano_medio"):
			if err := utils.ValidateAnoMedio(ano); err != nil {
				return err
			}
		case strings.HasSuffix(ano, "_ano_superior"):
			if err := utils.ValidateAnoSuperior(ano); err != nil {
				return err
			}
		default:
			return fmt.Errorf("formato de ano acadêmico inválido: %q", ano)
		}
	}
	return nil
}
```
**Substituir por:**
```go
// validarAnosAcademicosServicoExtra valida apenas o FORMATO de cada ano
// informado. Lista vazia é válida e significa "disponível para todos os anos
// fundamentais". Só aceita anos de ensino fundamental — médio e superior são
// sempre escopados a um curso específico via cursos_disponiveis (ver
// validarCursosDisponiveisServicoExtra), nunca soltos aqui. (Anos soltos de
// médio/superior eram aceitos por compatibilidade até a Tarefa 87b; removido
// porque nenhum registro existente usava esse formato.)
func validarAnosAcademicosServicoExtra(anos []string) error {
	for _, ano := range anos {
		if !strings.HasSuffix(ano, "_ano_fundamental") {
			return fmt.Errorf("anos_academicos_disponiveis só aceita anos do ensino fundamental (%q inválido) — para médio/superior use cursos_disponiveis", ano)
		}
		if err := utils.ValidateAnoFundamental(ano); err != nil {
			return err
		}
	}
	return nil
}
```

## 4. `internal/handlers/servico_extra_handlers.go`

**Localizar:**
```go
// estudanteElegivelServicoExtra cruza as restrições do serviço com o ano e
// curso atuais do estudante. Anos legados sem curso continuam elegíveis.
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
**Substituir por:**
```go
// estudanteElegivelServicoExtra cruza as restrições do serviço com o ano e
// curso atuais do estudante. anos_academicos_disponiveis só cobre
// fundamental (sem curso); médio e superior são verificados sempre via
// cursos_disponiveis — não há mais fallback de ano solto para esses dois.
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
	if est.CursoMedioID != nil && est.AnoEscolarMedio != nil && contains(serv.CursosDisponiveis, *est.CursoMedioID+"|"+*est.AnoEscolarMedio) {
		return true
	}
	if est.CursoSuperiorID != nil && est.AnoSuperior != nil && contains(serv.CursosDisponiveis, *est.CursoSuperiorID+"|"+*est.AnoSuperior) {
		return true
	}
	return false
}
```

## 5. Teste novo (`internal/domain/aggregates/servico_extra_test.go`)

Nenhum teste existente hoje passa um ano de médio/superior solto em `anos_academicos_disponiveis` para `ServicoExtra` — a mudança acima não deveria quebrar nada. Adicione uma verificação confirmando a rejeição, dentro de `TestServicoExtraCursosDisponiveisValidation`:

**Localizar:**
```go
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil, []string{cursoID + "|6_ano_fundamental"}, false, "", nil, id); e == nil {
		t.Fatal("ano fundamental escopado a curso foi aceito")
	}
}
```
**Substituir por:**
```go
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil, []string{cursoID + "|6_ano_fundamental"}, false, "", nil, id); e == nil {
		t.Fatal("ano fundamental escopado a curso foi aceito")
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, []string{"2_ano_medio"}, nil, false, "", nil, id); e == nil {
		t.Fatal("ano médio solto em anos_academicos_disponiveis foi aceito (suporte legado deveria ter sido removido)")
	}
}
```

## 6. Documentação da API (`Documentação da API.md`, seção 20.1)

**Localizar:**
```
**Regras de negócio:** campos financeiros são obrigatórios apenas quando a respectiva cobrança estiver ativa; `anos_academicos_disponiveis` e `cursos_disponiveis` vazios (ambos) disponibilizam o serviço para todos os anos/cursos. `anos_academicos_disponiveis` aceita anos soltos (`N_ano_fundamental`/`N_ano_medio`/`N_ano_superior`) sem vínculo com curso — para fundamental é o único formato possível; para médio/superior um ano solto vale para qualquer curso da academia, por compatibilidade. `cursos_disponiveis` restringe a um curso específico: cada item é `"<curso_id>|<ano_academico>"`, com ano médio ou superior. O curso precisa pertencer à mesma academia, não estar deletado, ter o tipo correspondente ao ano e conter o ano entre seus anos acadêmicos. As duas listas são combináveis. Em `POST /estudante/servicos-extras/:id/solicitacao`, se o serviço tiver restrições, o estudante só consegue se inscrever quando seu ano/curso atual corresponder a uma das listas; caso contrário recebe `403`.
```
**Substituir por:**
```
**Regras de negócio:** campos financeiros são obrigatórios apenas quando a respectiva cobrança estiver ativa; `anos_academicos_disponiveis` e `cursos_disponiveis` vazios (ambos) disponibilizam o serviço para todos os anos/cursos. `anos_academicos_disponiveis` só aceita anos de ensino fundamental (`N_ano_fundamental`), sem vínculo com curso — o ensino fundamental não tem cursos neste sistema. `cursos_disponiveis` restringe a um curso específico: cada item é `"<curso_id>|<ano_academico>"`, com ano médio ou superior — médio e superior são sempre escopados a um curso, nunca soltos. O curso precisa pertencer à mesma academia, não estar deletado, ter o tipo correspondente ao ano e conter o ano entre seus anos acadêmicos. As duas listas são combináveis (ex.: fundamental solto + um curso médio específico). Em `POST /estudante/servicos-extras/:id/solicitacao`, se o serviço tiver restrições, o estudante só consegue se inscrever quando seu ano/curso atual corresponder a uma das listas; caso contrário recebe `403`.
```

## 7. Checklist de aceite

- [ ] `anos_academicos_disponiveis` rejeita qualquer entrada que não termine em `_ano_fundamental`.
- [ ] `estudanteElegivelServicoExtra` não verifica mais `AnoEscolarMedio`/`AnoSuperior` soltos contra `AnosAcademicosDisponiveis` — só contra `CursosDisponiveis` (via curso).
- [ ] Novo caso de teste da seção 5 passa.
- [ ] Nenhuma mudança em `cursos_disponiveis`, `validarCursosDisponiveisServicoExtra` ou `validarPosseCursosDisponiveis`.
- [ ] `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test -p 1 ./...` sem erros.
- [ ] Documentação da API atualizada (seção 6).
