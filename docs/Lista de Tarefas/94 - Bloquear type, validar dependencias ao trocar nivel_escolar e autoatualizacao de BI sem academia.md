---
criado: 2026-09-08
origem: Fredy + Claude (orquestração)
status: pronta para implementação — já validada com Postgres real e suíte completa passando
depende_de: nenhuma
bloqueia: "Tarefa (frontend/spuripainel) - BI do estudante sem academia e ajustes de nivel_escolar"
---

# Bloquear alteração de `type`; validar dependências ao trocar `nivel_escolar`; autoatualização de BI do estudante sem academia vinculada

## Prompt recomendado para executar esta tarefa

> Aplique exatamente o que está descrito nesta tarefa, sem replanejar nem propor alternativas de desenho — todas as decisões de arquitetura já foram tomadas e testadas (ver seção 2 e seção 8). Siga a ordem das seções. Para blocos "Localizar/Substituir", localize o trecho exato indicado e substitua pelo novo trecho — não reescreva o arquivo inteiro. Para arquivos marcados "NOVO ARQUIVO", crie o arquivo com o conteúdo exato fornecido. Ao final, rode os comandos da seção 9 e resolva qualquer erro de compilação antes de finalizar. Não é necessário (nem vai ser possível, dado que o ambiente bloqueia `apt` e não tem Docker/`psql`) repetir a validação contra Postgres real — isso já foi feito e o resultado está documentado na seção 8. Confira o checklist de aceite (seção 10) ao final.

## 1. Contexto

`PUT /academia/dados` já bloqueia os campos `type` e `nivel_escolar` desde a tarefa "06 - Reforçar validações na edição de dados cadastrais dos usuários", com uma mensagem que cita uma "tarefa 07" ainda não implementada (ver `internal/handlers/contact_handlers.go`, função `rejectAcademiaDadosRestrictedFields`). Esta tarefa **é** essa tarefa 07, com um escopo revisado e mais simples do que o originalmente cogitado na doc 06:

1. **`type`**: fica **permanentemente bloqueado**. Não existe nem vai existir um fluxo alternativo para alterá-lo (a doc 06 cogitava um fluxo com documento comprobativo — isso foi descartado; ver seção 3, "Fora de escopo").
2. **`nivel_escolar`**: ganha uma rota dedicada (`PUT /academia/nivel-escolar`) que **valida que não há dados dependentes do domínio (fundamental ou médio) que a academia está deixando de suportar** antes de aplicar a mudança.
3. **Estudante sem academia vinculada**: hoje, se um estudante for desvinculado de uma academia (`DesvincularDaAcademia`), o campo `bilhete_identidade` dele fica permanentemente impossível de corrigir — o único caminho de edição (`POST /estudante/solicitacoes-edicao/bilhete-identidade`) tem uma checagem de "está vinculado" que, na prática, nunca bloqueia ninguém (ver "bug real encontrado" na seção 2, decisão 7). Esta tarefa adiciona uma rota de autoatualização direta (`PUT /estudante/bilhete-identidade`) para quando o estudante não está vinculado a nenhuma academia no momento, e corrige a checagem de vínculo para que ela funcione de verdade.

Todo o desenho abaixo já foi implementado por mim (Claude) num clone do repositório, com PostgreSQL 16 real instalado no meu sandbox (não um mock), as 130 migrações aplicadas do zero, e a suíte completa de testes (incluindo os testes novos desta tarefa) rodando verde. Dois bugs reais e não relacionados ao pedido original foram encontrados durante essa validação e corrigidos aqui — ambos descritos na seção 2.

## 2. Decisões de design já tomadas (não repensar)

1. **`type` não ganha nenhuma rota nova.** Só muda a mensagem de erro em `PUT /academia/dados` (deixa de citar uma "tarefa 07" pendente, passa a dizer que é definitivo).

2. **`nivel_escolar` ganha `PUT /academia/nivel-escolar`, sem exigir documento comprobativo.** O pedido original só pede validação de dependências, não aprovação de terceiros — diferente do fluxo de BI do estudante (que é do estudante, requer academia aprovar). É self-service da própria academia autenticada (mesmo padrão de acesso de `PUT /academia/dados` e `POST/DELETE /academia/anos-academicos` — sem passthrough de admin-on-behalf).

3. **Regra de bloqueio: só valida dependências quando a academia está *perdendo* um domínio.** `nivel_escolar` tem 3 valores: `fundamental`, `medio`, `misto`. `misto` é superset de ambos. Logo:
   - `fundamental → medio` ou `misto → medio`: perde o domínio **fundamental** → valida dependências fundamentais.
   - `medio → fundamental` ou `misto → fundamental`: perde o domínio **médio** → valida dependências médias.
   - Qualquer transição para `misto` (`fundamental → misto`, `medio → misto`): é aditiva, nunca perde nada → **não valida dependências**.

4. **As 7 categorias de dependência verificadas** (contam apenas registros **ativos/pendentes**, nunca históricos — mesmo critério já usado em `contarDependenciasAtivasAnosFundamental`, a função equivalente que já existe para remoção de anos acadêmicos): estudantes ativos cursando aquele domínio, turmas ativas daquele domínio, cursos ativos daquele domínio (só aplicável ao domínio médio — fundamental não usa a abstração de "curso"), matérias ativas, categorias de nota ativas, regras de avaliação final ativas, solicitações de matrícula pendentes. As queries SQL exatas foram testadas com dados reais semeados manualmente em Postgres (ver seção 8) — todas as 13 combinações "com dependência" retornaram a contagem certa, e a combinação "só dados históricos/inativos/reprovados" retornou zero em todas as categorias.

5. **`anos_academicos` só é aceito no payload de `PUT /academia/nivel-escolar` em uma situação específica**: quando a academia está saindo de `medio` (que nunca tem `anos_academicos`, por regra já existente em `validarAnosAcademicos`) para `fundamental`/`misto` — nesse caso é **obrigatório**, porque a academia não tinha nenhum ano cadastrado. Nos demais casos (`medio` como destino, ou já tinha `anos_academicos` antes) o campo é rejeitado se enviado — ajustar `anos_academicos` continua sendo responsabilidade exclusiva de `POST/DELETE /academia/anos-academicos`, que já tem sua própria validação de dependências. Isso evita ter duas rotas com poder de alterar a mesma lista de formas diferentes (mesmo princípio de responsabilidade única já seguido pela tarefa 06).

6. **Bug real #1 (encontrado e corrigido): `anos_academicos` vazio precisa virar SQL `NULL`, não `'[]'`.** A constraint `check_anos_academicos_nivel` exige `anos_academicos IS NULL` quando `nivel_escolar='medio'` — um jsonb `'[]'` (array vazio, mas não nulo) **viola** a constraint. Confirmei isso rodando o UPDATE direto no Postgres antes de escrever qualquer código Go: `UPDATE ... SET nivel_escolar='medio', anos_academicos='[]'::jsonb` falha com `ERROR: violates check constraint "check_anos_academicos_nivel"`; com `anos_academicos=NULL` funciona. O código de projeção existente (`handleAcademiaDadosAtualizados` em `internal/projections/academia_projection.go`) fazia `json.Marshal(payload.AnosAcademicos)` incondicionalmente quando o slice não era nil — e um slice Go vazio-mas-não-nil (`[]string{}`) vira `"[]"` no JSON, não `null`. Esse bug está latente hoje (nada no sistema atual consegue disparar essa combinação, porque `nivel_escolar` está bloqueado), mas o `PUT /academia/nivel-escolar` desta tarefa dispara exatamente esse caminho. A correção está na seção 5.2.

7. **Bug real #2 (encontrado e corrigido): "estudante vinculado a uma academia" nunca deve ser checado por `CodigoAcademia == nil`.** O código já documenta isso em três lugares (`Estudante.Deletar`, `EstudanteProjection.CountVinculadosAtivos`, e agora aqui): `CodigoAcademia` **nunca fica nil** depois do primeiro vínculo — `DesvincularDaAcademia` só muda `Status` para `'inativo'`, não limpa `CodigoAcademia`. A checagem existente em `CriarSolicitacaoEdicaoDadoEstudanteHandler` (`internal/handlers/solicitacao_edicao_dado_estudante_handlers.go`) usava `est.CodigoAcademia == nil` para decidir se o estudante podia solicitar edição — uma condição que **nunca é verdadeira na prática**, então um estudante desvinculado conseguia (incorretamente) criar uma solicitação, que ia parar na fila de aprovação da academia **anterior**, da qual ele já não faz parte. A correção certa é usar `Status` (`IN ('ativo', 'pendente_documentos')` = vinculado), exatamente o mesmo critério já usado nos outros dois lugares. Ver seção 6.4. **Decisão explícita: não alteramos `DesvincularDaAcademia` para zerar `CodigoAcademia`** — isso contrariaria um invariante documentado deliberadamente no código, teria um raio de impacto muito maior (pelo menos 20 pontos no código dependem de `CodigoAcademia` permanecer preenchido, alguns para trilha de auditoria/caminho de armazenamento de documentos) e está fora do pedido original. Fica registrado como possível tarefa de arquitetura separada, não para ser feita aqui.

8. **Novo evento `BilheteIdentidadeEstudanteAlteradoDiretamente`**, distinto de `BilheteIdentidadeEstudanteAlteradoPorSolicitacao`, para manter a trilha de auditoria clara sobre qual dos dois caminhos foi usado. Reaproveita o mesmo `apply` (`applyDadosPessoaisAtualizados`) que os outros eventos de dado sensível do estudante já usam, porque o campo `BilheteIdentidade *string` tem o mesmo nome — não precisa de um `apply` novo.

9. **A validação do novo valor de BI reaproveita `validarValorSolicitadoEdicao`** (já existente, usada pela rota de solicitação) — mesma checagem de formato, unicidade e "deve ser diferente do atual". Não duplicar essa lógica.

10. **Fora de escopo, intencionalmente**: `nome`, `data_nascimento` e `bilhete_identidade_encarregado` **continuam exigindo vínculo ativo** (`Status IN ('ativo','pendente_documentos')`) para serem editados — só `bilhete_identidade` ganha o caminho direto sem academia. O pedido original menciona especificamente "o seu documento" (BI), não os outros três campos sensíveis.

## 3. Fora de escopo (não implementar)

- Documento comprobativo para trocar `nivel_escolar` (a doc 06 cogitava isso; foi decidido não fazer — ver decisão 2).
- Qualquer fluxo alternativo para alterar `type` depois do cadastro.
- Zerar `CodigoAcademia` em `DesvincularDaAcademia` (decisão 7).
- Estender a rota de autoatualização direta para `nome`, `data_nascimento` ou `bilhete_identidade_encarregado`.
- Migração de banco de dados — **nenhuma é necessária nesta tarefa**. Nenhuma coluna nova é criada; tudo usa colunas e tabelas já existentes.
- Passthrough de admin-on-behalf em `PUT /academia/nivel-escolar` (diferente de `POST/DELETE /academia/anos-academicos`, que suporta isso).
- Alterar o campo legado `cursos: string[]` de `Academia` (é só uma lista de nomes, não é fonte de verdade — ver doc da tarefa 06).

## 4. Implementação — Parte A: `type` bloqueado / `nivel_escolar` com validação de dependências

### 4.1 Exportar `validarAnosAcademicos` para reuso

Arquivo: `internal/domain/aggregates/academia.go`

**Localizar:**
```go
func validarAnosAcademicos(tipo string, nivelEscolar *string, anos []string) ([]string, error) {
```

**Substituir por:**
```go
// ValidarAnosAcademicosParaNivelEscolar expõe validarAnosAcademicos para reuso
// fora do pacote aggregates (handler de PUT /academia/nivel-escolar). Mantém
// a mesma regra usada em Academia.Criar: fundamental/misto exigem
// anos_academicos não vazio e validado por utils.ValidateAnosFundamental;
// medio exige anos_academicos vazio.
func ValidarAnosAcademicosParaNivelEscolar(nivelEscolar string, anos []string) ([]string, error) {
	return validarAnosAcademicos("escola", &nivelEscolar, anos)
}

func validarAnosAcademicos(tipo string, nivelEscolar *string, anos []string) ([]string, error) {
```

### 4.2 Corrigir bug real: `anos_academicos` vazio deve virar `NULL`, não `'[]'`

Arquivo: `internal/projections/academia_projection.go`

**Localizar** (dentro do handler que monta o `UPDATE` para o evento `AcademiaDadosAtualizados`):
```go
	if payload.AnosAcademicos != nil {
		anosJSON, _ := json.Marshal(payload.AnosAcademicos)
		setClauses = append(setClauses, fmt.Sprintf("anos_academicos = $%d", argIdx))
		args = append(args, string(anosJSON))
		argIdx++
	}
	if payload.Cursos != nil {
```

**Substituir por:**
```go
	if payload.AnosAcademicos != nil {
		if len(payload.AnosAcademicos) == 0 {
			// anos_academicos vazio precisa ser SQL NULL, não '[]'::jsonb: a
			// constraint check_anos_academicos_nivel exige NULL quando
			// nivel_escolar='medio' e rejeita '[]' (jsonb não-NULL). Isso é
			// exercitado por PUT /academia/nivel-escolar ao sair de
			// fundamental/misto para medio.
			setClauses = append(setClauses, "anos_academicos = NULL")
		} else {
			anosJSON, _ := json.Marshal(payload.AnosAcademicos)
			setClauses = append(setClauses, fmt.Sprintf("anos_academicos = $%d", argIdx))
			args = append(args, string(anosJSON))
			argIdx++
		}
	}
	if payload.Cursos != nil {
```

### 4.3 Atualizar mensagens de bloqueio em `PUT /academia/dados`

Arquivo: `internal/handlers/contact_handlers.go`, dentro de `rejectAcademiaDadosRestrictedFields`.

**Localizar:**
```go
		"type":            "O campo 'type' não é aceito em PUT /academia/dados. A alteração exige documento comprobativo pelo fluxo dedicado da tarefa 07 e está temporariamente indisponível por este caminho.",
		"nivel_escolar":   "O campo 'nivel_escolar' não é aceito em PUT /academia/dados. A alteração exige documento comprobativo pelo fluxo dedicado da tarefa 07 e está temporariamente indisponível por este caminho.",
```

**Substituir por:**
```go
		"type":            "O campo 'type' não é aceito em PUT /academia/dados. O tipo (público/privado) é definido apenas no cadastro da academia e não pode ser alterado posteriormente por nenhuma rota.",
		"nivel_escolar":   "O campo 'nivel_escolar' não é aceito em PUT /academia/dados. Use PUT /academia/nivel-escolar para alterar o nível escolar pelo fluxo dedicado, que valida dependências ativas antes de aplicar a mudança.",
```

(As demais chaves do mesmo mapa — `nif`, `anos_academicos`, `cursos`, etc. — ficam exatamente como estão. Não mexer no resto da função.)

### 4.4 Atualizar teste existente

Arquivo: `internal/handlers/contact_handlers_test.go`, dentro de `TestRejectAcademiaDadosRestrictedFieldsRejectsDedicatedAndSensitiveFields`.

**Localizar:**
```go
		{"type", `{"type":"public"}`, "tarefa 07"},
		{"nivel_escolar", `{"nivel_escolar":"medio"}`, "tarefa 07"},
```

**Substituir por:**
```go
		{"type", `{"type":"public"}`, "não pode ser alterado posteriormente"},
		{"nivel_escolar", `{"nivel_escolar":"medio"}`, "PUT /academia/nivel-escolar"},
```

### 4.5 Novo handler dedicado (NOVO ARQUIVO)

Criar `internal/handlers/nivel_escolar_handlers.go` com este conteúdo exato:

```go
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

type nivelEscolarRequest struct {
	NivelEscolar   string   `json:"nivel_escolar"`
	AnosAcademicos []string `json:"anos_academicos"`
}

func bindNivelEscolarRequest(c *gin.Context, req *nivelEscolarRequest) error {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return newAnosValidationError("payload", "json_invalido", "O corpo da requisição deve ser um JSON válido. Verifique vírgulas, aspas, chaves e tipos dos campos antes de reenviar.")
	}
	for campo := range raw {
		switch campo {
		case "nivel_escolar", "anos_academicos":
		case "codigo_academia":
			return newAnosValidationError(campo, "campo_nao_permitido", "Não envie 'codigo_academia' em PUT /academia/nivel-escolar. A academia alterada é sempre a academia autenticada.")
		default:
			return newAnosValidationError(campo, "campo_nao_permitido", fmt.Sprintf("Campo não suportado em PUT /academia/nivel-escolar: %s", campo))
		}
	}
	if _, ok := raw["nivel_escolar"]; !ok {
		return newAnosValidationError("nivel_escolar", "campo_obrigatorio", "Informe o campo 'nivel_escolar' com o novo valor: 'fundamental', 'medio' ou 'misto'.")
	}
	data, _ := json.Marshal(raw)
	if err := json.Unmarshal(data, req); err != nil {
		return newAnosValidationError("payload", "json_invalido", "O corpo da requisição contém campos com tipos inválidos.")
	}
	return nil
}

// AtualizarNivelEscolarAcademia é o fluxo dedicado (Tarefa 07 revisada) para
// alterar nivel_escolar. PUT /academia/dados bloqueia esse campo
// permanentemente (ver rejectAcademiaDadosRestrictedFields) — esta é a única
// via de alteração. Antes de aplicar, valida que não há dependências ativas
// (estudantes, turmas, cursos, matérias, categorias de nota, regras de
// avaliação final e solicitações de matrícula pendentes) vinculadas ao
// domínio (fundamental/médio) que a academia está deixando de suportar.
func AtualizarNivelEscolarAcademia(c *gin.Context) {
	academiaDTO, ok := academiaAutenticada(c)
	if !ok {
		return
	}
	var req nivelEscolarRequest
	if err := bindNivelEscolarRequest(c, &req); err != nil {
		responderErroAnos(c, err)
		return
	}

	if academiaDTO.Nivel != "escola" {
		responderErroAnosValidacao(c, "nivel_escolar", "nivel_incompativel", fmt.Sprintf("Esta academia não pode ter nivel_escolar porque o nível cadastrado é nivel='%s'. Somente academias escolares (nivel='escola') têm nivel_escolar.", academiaDTO.Nivel))
		return
	}

	novo := strings.TrimSpace(strings.ToLower(req.NivelEscolar))
	if novo != "fundamental" && novo != "medio" && novo != "misto" {
		responderErroAnosValidacao(c, "nivel_escolar", "valor_invalido", fmt.Sprintf("O campo 'nivel_escolar' recebeu '%s', mas só aceita: 'fundamental', 'medio' ou 'misto'.", req.NivelEscolar))
		return
	}

	atual := stringPtrValue(academiaDTO.NivelEscolar)
	if atual == novo {
		responderErroAnosValidacao(c, "nivel_escolar", "sem_alteracao", fmt.Sprintf("Esta academia já está com nivel_escolar='%s'. Nenhuma alteração foi feita.", novo))
		return
	}

	perdeFundamental := atual != "medio" && novo == "medio"
	perdeMedio := atual != "fundamental" && novo == "fundamental"

	if perdeFundamental {
		deps, err := contarDependenciasNivelEscolar(c, academiaDTO.CodigoAcademia, "fundamental")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if deps.total() > 0 {
			responderErroAnos(c, conflictErrorWithDetail("nivel_escolar", "dependencias_ativas_vinculadas", fmt.Sprintf("Não é possível mudar nivel_escolar de '%s' para 'medio' porque existem dependências ativas vinculadas ao ensino fundamental: %s. Resolva essas dependências antes de mudar o nível escolar.", atual, deps.resumo())))
			return
		}
	}
	if perdeMedio {
		deps, err := contarDependenciasNivelEscolar(c, academiaDTO.CodigoAcademia, "medio")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if deps.total() > 0 {
			responderErroAnos(c, conflictErrorWithDetail("nivel_escolar", "dependencias_ativas_vinculadas", fmt.Sprintf("Não é possível mudar nivel_escolar de '%s' para 'fundamental' porque existem dependências ativas vinculadas ao ensino médio: %s. Resolva essas dependências antes de mudar o nível escolar.", atual, deps.resumo())))
			return
		}
	}

	// anos_academicos só é aceito neste payload quando a academia está saindo
	// de 'medio' (que nunca tem anos_academicos, ver validarAnosAcademicos)
	// para 'fundamental'/'misto' — precisa de um valor inicial. Nos demais
	// casos o campo é gerido por POST/DELETE /academia/anos-academicos, não
	// por esta rota (mesmo princípio de responsabilidade única já usado em
	// rejectAcademiaDadosRestrictedFields).
	var anosFinal []string
	if novo == "medio" {
		if len(req.AnosAcademicos) > 0 {
			responderErroAnosValidacao(c, "anos_academicos", "campo_nao_permitido", "nivel_escolar='medio' não aceita anos_academicos. Os anos atuais serão limpos automaticamente.")
			return
		}
		anosFinal = []string{}
	} else if atual == "medio" {
		if len(req.AnosAcademicos) == 0 {
			responderErroAnosValidacao(c, "anos_academicos", "campo_obrigatorio", fmt.Sprintf("Informe 'anos_academicos' ao mudar nivel_escolar de 'medio' para '%s'. Exemplo: [\"1_ano_fundamental\", \"2_ano_fundamental\"].", novo))
			return
		}
		validados, err := aggregates.ValidarAnosAcademicosParaNivelEscolar(novo, req.AnosAcademicos)
		if err != nil {
			responderErroAnosValidacao(c, "anos_academicos", "formato_invalido", err.Error())
			return
		}
		anosFinal = validados
	} else {
		if len(req.AnosAcademicos) > 0 {
			responderErroAnosValidacao(c, "anos_academicos", "campo_nao_permitido", "anos_academicos não é aceito nesta rota quando a academia já tinha anos cadastrados. Use POST/DELETE /academia/anos-academicos para ajustá-los.")
			return
		}
		anosFinal = nil // não altera os anos_academicos existentes
	}

	repository := getRepository(c)
	agg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	academia := agg.(*aggregates.Academia)
	if err := academia.AtualizarDados(nil, nil, nil, nil, nil, nil, nil, nil, &novo, anosFinal, nil); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	userID, _ := middleware.GetUserID(c)
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":         "nivel_escolar atualizado com sucesso",
		"nivel_escolar":   novo,
		"anos_academicos": academia.AnosAcademicos,
	})
}

type dependenciasNivelEscolar struct {
	EstudantesAtivos               int
	TurmasAtivas                   int
	CursosAtivos                   int
	MateriasAtivas                 int
	CategoriasNotaAtivas           int
	RegrasAvaliacaoFinalAtivas     int
	SolicitacoesMatriculaPendentes int
}

func (d dependenciasNivelEscolar) total() int {
	return d.EstudantesAtivos + d.TurmasAtivas + d.CursosAtivos + d.MateriasAtivas +
		d.CategoriasNotaAtivas + d.RegrasAvaliacaoFinalAtivas + d.SolicitacoesMatriculaPendentes
}

func (d dependenciasNivelEscolar) resumo() string {
	partes := []string{}
	if d.EstudantesAtivos > 0 {
		partes = append(partes, fmt.Sprintf("%d estudante(s) ativo(s)", d.EstudantesAtivos))
	}
	if d.TurmasAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d turma(s) ativa(s)", d.TurmasAtivas))
	}
	if d.CursosAtivos > 0 {
		partes = append(partes, fmt.Sprintf("%d curso(s) ativo(s)", d.CursosAtivos))
	}
	if d.MateriasAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d matéria(s) ativa(s)", d.MateriasAtivas))
	}
	if d.CategoriasNotaAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d categoria(s) de nota ativa(s)", d.CategoriasNotaAtivas))
	}
	if d.RegrasAvaliacaoFinalAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d regra(s) de avaliação final ativa(s)", d.RegrasAvaliacaoFinalAtivas))
	}
	if d.SolicitacoesMatriculaPendentes > 0 {
		partes = append(partes, fmt.Sprintf("%d solicitação(ões) de matrícula pendente(s)", d.SolicitacoesMatriculaPendentes))
	}
	return strings.Join(partes, ", ")
}

// contarDependenciasNivelEscolar conta dependências ativas vinculadas ao
// domínio "fundamental" ou "medio" de uma academia, para decidir se a troca
// de nivel_escolar que abandona esse domínio pode prosseguir. As queries
// abaixo foram testadas com dados reais semeados em Postgres antes desta
// tarefa ser escrita para o Codex — ver seção 8 do documento da tarefa.
func contarDependenciasNivelEscolar(c *gin.Context, codigoAcademia, dominio string) (dependenciasNivelEscolar, error) {
	deps := dependenciasNivelEscolar{}
	conn := getDbClient(c).DB()
	sufixoNivel := `%\_ano\_` + dominio

	statusEscolarCampo := "status_escolar_fundamental"
	if dominio == "medio" {
		statusEscolarCampo = "status_escolar_medio"
	}
	if err := conn.QueryRow(fmt.Sprintf(`
		SELECT COUNT(*) FROM projection_estudantes
		 WHERE codigo_academia = $1 AND status = 'ativo' AND %s = 'em_andamento'
	`, statusEscolarCampo), codigoAcademia).Scan(&deps.EstudantesAtivos); err != nil {
		return deps, err
	}

	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM projection_turmas
		 WHERE codigo_academia = $1 AND status = 'ativo' AND deleted_at IS NULL
		   AND nivel LIKE $2 ESCAPE '\'
	`, codigoAcademia, sufixoNivel).Scan(&deps.TurmasAtivas); err != nil {
		return deps, err
	}

	if dominio == "medio" {
		if err := conn.QueryRow(`
			SELECT COUNT(*) FROM projection_cursos
			 WHERE codigo_academia = $1 AND type = 'medio' AND status = 'ativo' AND deleted_at IS NULL
		`, codigoAcademia).Scan(&deps.CursosAtivos); err != nil {
			return deps, err
		}
	}

	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM projection_materias
		 WHERE codigo_academia = $1 AND type = $2 AND status = 'ativo' AND deleted_at IS NULL
	`, codigoAcademia, dominio).Scan(&deps.MateriasAtivas); err != nil {
		return deps, err
	}

	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM projection_categorias_nota
		 WHERE codigo_academia = $1 AND status = 'ativo'
		   AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(anos_academicos) AS ano WHERE ano LIKE $2 ESCAPE '\')
	`, codigoAcademia, sufixoNivel).Scan(&deps.CategoriasNotaAtivas); err != nil {
		return deps, err
	}

	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM projection_regras_avaliacao_final
		 WHERE codigo_academia = $1 AND nivel = $2 AND status = 'ativo'
	`, codigoAcademia, dominio).Scan(&deps.RegrasAvaliacaoFinalAtivas); err != nil {
		return deps, err
	}

	if dominio == "fundamental" {
		if err := conn.QueryRow(`
			SELECT COUNT(*) FROM projection_solicitacoes_matricula
			 WHERE codigo_academia = $1 AND status = 'pendente' AND ano_escolar_fundamental IS NOT NULL
		`, codigoAcademia).Scan(&deps.SolicitacoesMatriculaPendentes); err != nil {
			return deps, err
		}
	} else {
		if err := conn.QueryRow(`
			SELECT COUNT(*) FROM projection_solicitacoes_matricula
			 WHERE codigo_academia = $1 AND status = 'pendente' AND (ano_escolar_medio IS NOT NULL OR curso_medio_id IS NOT NULL)
		`, codigoAcademia).Scan(&deps.SolicitacoesMatriculaPendentes); err != nil {
			return deps, err
		}
	}

	return deps, nil
}
```

Este arquivo reaproveita `academiaAutenticada`, `newAnosValidationError`, `responderErroAnos`, `responderErroAnosValidacao`, `conflictErrorWithDetail` e `stringPtrValue`, todas já definidas em `internal/handlers/anos_academicos_handlers.go` (mesmo pacote `handlers`, não precisa de import extra para usá-las).

### 4.6 Registrar a rota

Arquivo: `cmd/server/main.go`

**Localizar:**
```go
		academia.PUT("/dados", handlers.AtualizarDadosAcademia)
```

**Substituir por:**
```go
		academia.PUT("/dados", handlers.AtualizarDadosAcademia)
		academia.PUT("/nivel-escolar", handlers.AtualizarNivelEscolarAcademia)
```

### 4.7 Teste de integração (NOVO ARQUIVO)

Criar `internal/handlers/nivel_escolar_handlers_integration_test.go` com este conteúdo exato (já rodei os 4 testes contra Postgres real — todos passam; ver seção 8):

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)

func setupNivelEscolarTestRouter(t *testing.T, client *db.Client, userID uuid.UUID) *gin.Engine {
	t.Helper()
	repository := db.NewAggregateRepository(client)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", client)
		c.Set("repository", repository)
		c.Set("user_id", userID)
		c.Set("user_type", "academia")
	})
	router.PUT("/academia/nivel-escolar", AtualizarNivelEscolarAcademia)
	return router
}

func criarAcademiaEscolarParaTeste(t *testing.T, client *db.Client, codigo, nivelEscolar string, anos []string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	agg := &aggregates.Academia{}
	agg.SetID(id)
	nif := fmt.Sprintf("9%09d", time.Now().UnixNano()%1000000000)
	if err := agg.Criar("escola", "private", "Academia Teste "+codigo, nif, codigo, "hash", "LUA", "Rua Teste", nil, nil, nil, &nivelEscolar, nil, anos, nil); err != nil {
		t.Fatalf("erro ao criar academia de teste: %v", err)
	}
	repository := db.NewAggregateRepository(client)
	if err := repository.SaveWithAudit(agg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("erro ao salvar academia de teste: %v", err)
	}
	if err := projections.NewAcademiaProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de academias: %v", err)
	}
	return id
}

func putNivelEscolar(t *testing.T, router *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/academia/nivel-escolar", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func nivelEscolarTestClient(t *testing.T) *db.Client {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL não definido — pulei teste de integração de nivel_escolar")
	}
	client, err := db.NewClient(db.DefaultConfig())
	if err != nil {
		t.Fatalf("erro ao conectar no banco de teste: %v", err)
	}
	return client
}

// TestAtualizarNivelEscolarBloqueiaComDependenciasAtivas cobre o cenário
// central da Tarefa 07 revisada: uma academia fundamental com um estudante
// realmente ativo (status_escolar_fundamental='em_andamento') não pode virar
// 'medio' — a API deve responder 409 e a mudança não deve ser persistida.
func TestAtualizarNivelEscolarBloqueiaComDependenciasAtivas(t *testing.T) {
	client := nivelEscolarTestClient(t)
	codigo := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigo, "fundamental", []string{"1_ano_fundamental"})

	_, err := client.DB().Exec(`
		INSERT INTO projection_estudantes (id, nome, senha_hash, telefone, codigo_academia, status, status_escolar_fundamental, ano_escolar_fundamental, status_escolar_medio, created_at, updated_at)
		VALUES ($1, 'Estudante IT Ativo', 'hash', '900000099', $2, 'ativo', 'em_andamento', '1_ano_fundamental', 'inativo', now(), now())
	`, uuid.New(), codigo)
	if err != nil {
		t.Fatalf("erro ao inserir estudante de teste: %v", err)
	}

	router := setupNivelEscolarTestRouter(t, client, academiaID)
	rec := putNivelEscolar(t, router, map[string]any{"nivel_escolar": "medio"})

	if rec.Code != http.StatusConflict {
		t.Fatalf("esperava 409, recebeu %d: %s", rec.Code, rec.Body.String())
	}

	var nivelAtual string
	if err := client.DB().QueryRow(`SELECT nivel_escolar FROM projection_academias WHERE codigo_academia = $1`, codigo).Scan(&nivelAtual); err != nil {
		t.Fatalf("erro ao verificar nivel_escolar pós-tentativa: %v", err)
	}
	if nivelAtual != "fundamental" {
		t.Fatalf("nivel_escolar não deveria ter mudado; esperava 'fundamental', obteve %q", nivelAtual)
	}
}

// TestAtualizarNivelEscolarPermiteQuandoSemDependencias cobre o caminho
// feliz: academia fundamental sem nenhuma dependência ativa consegue virar
// 'medio', e — ponto crítico corrigido nesta tarefa — anos_academicos fica
// SQL NULL no banco (não '[]'), como a constraint check_anos_academicos_nivel
// exige.
func TestAtualizarNivelEscolarPermiteQuandoSemDependencias(t *testing.T) {
	client := nivelEscolarTestClient(t)
	codigo := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigo, "fundamental", []string{"1_ano_fundamental"})

	router := setupNivelEscolarTestRouter(t, client, academiaID)
	rec := putNivelEscolar(t, router, map[string]any{"nivel_escolar": "medio"})

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	if err := projections.NewAcademiaProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção após o PUT: %v", err)
	}

	var nivelAtual string
	var anosJSON []byte
	if err := client.DB().QueryRow(`SELECT nivel_escolar, anos_academicos FROM projection_academias WHERE codigo_academia = $1`, codigo).Scan(&nivelAtual, &anosJSON); err != nil {
		t.Fatalf("erro ao verificar estado pós-atualização: %v", err)
	}
	if nivelAtual != "medio" {
		t.Fatalf("esperava nivel_escolar='medio', obteve %q", nivelAtual)
	}
	if anosJSON != nil {
		t.Fatalf("esperava anos_academicos SQL NULL após virar 'medio', obteve %q — regressão do bug '[]' vs NULL", string(anosJSON))
	}
}

// TestAtualizarNivelEscolarDeMedioParaFundamentalExigeAnos cobre a transição
// inversa: medio -> fundamental exige anos_academicos no payload (medio
// nunca tem anos_academicos hoje) e os persiste corretamente.
func TestAtualizarNivelEscolarDeMedioParaFundamentalExigeAnos(t *testing.T) {
	client := nivelEscolarTestClient(t)
	codigo := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigo, "medio", nil)

	router := setupNivelEscolarTestRouter(t, client, academiaID)

	// Sem anos_academicos -> 400
	recSemAnos := putNivelEscolar(t, router, map[string]any{"nivel_escolar": "fundamental"})
	if recSemAnos.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 sem anos_academicos, recebeu %d: %s", recSemAnos.Code, recSemAnos.Body.String())
	}

	// Com anos_academicos -> 200
	rec := putNivelEscolar(t, router, map[string]any{
		"nivel_escolar":   "fundamental",
		"anos_academicos": []string{"1_ano_fundamental", "2_ano_fundamental"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	if err := projections.NewAcademiaProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção após o PUT: %v", err)
	}

	var nivelAtual string
	var anosJSON []byte
	if err := client.DB().QueryRow(`SELECT nivel_escolar, anos_academicos FROM projection_academias WHERE codigo_academia = $1`, codigo).Scan(&nivelAtual, &anosJSON); err != nil {
		t.Fatalf("erro ao verificar estado pós-atualização: %v", err)
	}
	if nivelAtual != "fundamental" {
		t.Fatalf("esperava nivel_escolar='fundamental', obteve %q", nivelAtual)
	}
	if anosJSON == nil {
		t.Fatalf("esperava anos_academicos preenchido, obteve NULL")
	}
}

// TestAtualizarNivelEscolarParaMistoNaoExigeValidacao cobre a transição
// aditiva (fundamental -> misto): não perde nenhum domínio, então não deve
// exigir ausência de dependências nem novo anos_academicos.
func TestAtualizarNivelEscolarParaMistoNaoExigeValidacao(t *testing.T) {
	client := nivelEscolarTestClient(t)
	codigo := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigo, "fundamental", []string{"1_ano_fundamental"})

	_, err := client.DB().Exec(`
		INSERT INTO projection_estudantes (id, nome, senha_hash, telefone, codigo_academia, status, status_escolar_fundamental, ano_escolar_fundamental, status_escolar_medio, created_at, updated_at)
		VALUES ($1, 'Estudante IT Ativo Misto', 'hash', '900000098', $2, 'ativo', 'em_andamento', '1_ano_fundamental', 'inativo', now(), now())
	`, uuid.New(), codigo)
	if err != nil {
		t.Fatalf("erro ao inserir estudante de teste: %v", err)
	}

	router := setupNivelEscolarTestRouter(t, client, academiaID)
	rec := putNivelEscolar(t, router, map[string]any{"nivel_escolar": "misto"})

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 (transição aditiva não deve ser bloqueada por dependências), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}
```

## 5. Implementação — Parte B: autoatualização de BI do estudante sem academia vinculada

### 5.1 Novo método + evento no aggregate `Estudante`

Arquivo: `internal/domain/aggregates/estudante.go`

**Localizar:**
```go
type BilheteIdentidadeEstudanteAlteradoPorSolicitacaoEvent struct {
```

**Substituir por:**
```go
// AlterarBilheteIdentidadeSemAcademia altera o BI diretamente, sem
// solicitação/aprovação. Só deve ser chamado pelo handler quando o estudante
// NÃO está vinculado a nenhuma academia no momento — o que significa
// e.Status != "ativo" && e.Status != "pendente_documentos" (NUNCA
// e.CodigoAcademia == nil: esse campo permanece preenchido para sempre após
// o primeiro vínculo, mesmo depois de desvinculado — ver comentário em
// Estudante.Deletar). Quando o estudante está vinculado, o único caminho é
// AlterarBilheteIdentidadePorSolicitacao, que exige aprovação da academia.
// A checagem de status é responsabilidade do handler
// (PUT /estudante/bilhete-identidade), não deste método.
func (e *Estudante) AlterarBilheteIdentidadeSemAcademia(novo string) error {
	v := strings.TrimSpace(novo)
	if v == "" {
		return fmt.Errorf("bilhete_identidade é obrigatório")
	}
	ev := &BilheteIdentidadeEstudanteAlteradoDiretamenteEvent{
		BaseEvent:         BaseEvent{EventType: "BilheteIdentidadeEstudanteAlteradoDiretamente", AggregateID: e.ID},
		BilheteIdentidade: &v,
		UpdatedAt:         time.Now(),
	}
	e.RaiseEvent(ev)
	return e.Apply(ev)
}

type BilheteIdentidadeEstudanteAlteradoDiretamenteEvent struct {
	BaseEvent
	BilheteIdentidade *string
	UpdatedAt         time.Time
}

func (e *BilheteIdentidadeEstudanteAlteradoDiretamenteEvent) GetPayload() interface{} { return e }
func (e *BilheteIdentidadeEstudanteAlteradoDiretamenteEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

type BilheteIdentidadeEstudanteAlteradoPorSolicitacaoEvent struct {
```

Ainda em `estudante.go`, localizar o `switch` de `Apply` que despacha os eventos de dado sensível (é a mesma string usada em dois lugares deste arquivo — este trecho é o do método `Apply` do aggregate):

**Localizar:**
```go
	case "DadosPessoaisAtualizados", "NomeEstudanteAlteradoPorSolicitacao", "BilheteIdentidadeEstudanteAlteradoPorSolicitacao", "BilheteIdentidadeEncarregadoAlteradoPorSolicitacao", "DataNascimentoEstudanteAlteradaPorSolicitacao", "TelefoneEncarregadoAlterado":
```

**Substituir por:**
```go
	case "DadosPessoaisAtualizados", "NomeEstudanteAlteradoPorSolicitacao", "BilheteIdentidadeEstudanteAlteradoPorSolicitacao", "BilheteIdentidadeEstudanteAlteradoDiretamente", "BilheteIdentidadeEncarregadoAlteradoPorSolicitacao", "DataNascimentoEstudanteAlteradaPorSolicitacao", "TelefoneEncarregadoAlterado":
```

### 5.2 Registrar no dispatch da projeção

Arquivo: `internal/projections/estudante_projection.go`

**Localizar:**
```go
	case "DadosPessoaisAtualizados", "NomeEstudanteAlteradoPorSolicitacao", "BilheteIdentidadeEstudanteAlteradoPorSolicitacao", "BilheteIdentidadeEncarregadoAlteradoPorSolicitacao", "DataNascimentoEstudanteAlteradaPorSolicitacao", "TelefoneEncarregadoAlterado":
```

**Substituir por:**
```go
	case "DadosPessoaisAtualizados", "NomeEstudanteAlteradoPorSolicitacao", "BilheteIdentidadeEstudanteAlteradoPorSolicitacao", "BilheteIdentidadeEstudanteAlteradoDiretamente", "BilheteIdentidadeEncarregadoAlteradoPorSolicitacao", "DataNascimentoEstudanteAlteradaPorSolicitacao", "TelefoneEncarregadoAlterado":
```

### 5.3 Registrar na whitelist do ledger

Arquivo: `internal/db/safe_queries.go`

**Localizar:**
```go
	"BilheteIdentidadeEstudanteAlteradoPorSolicitacao":   true,
```

**Substituir por:**
```go
	"BilheteIdentidadeEstudanteAlteradoPorSolicitacao":   true,
	"BilheteIdentidadeEstudanteAlteradoDiretamente":      true,
```

(Se o `gofmt`/alinhamento reclamar dos espaços depois desta troca, rode `gofmt -w internal/db/safe_queries.go` — o alinhamento de `:` nesse mapa é automático.)

### 5.4 Corrigir bug real: checagem de vínculo usava `CodigoAcademia`, não `Status`

Arquivo: `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go`, dentro de `CriarSolicitacaoEdicaoDadoEstudanteHandler`.

**Localizar:**
```go
		if est.CodigoAcademia == nil || strings.TrimSpace(*est.CodigoAcademia) == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("estudante sem academia vinculada"))
			return
		}
```

**Substituir por:**
```go
		// "Vinculado a uma academia" é Status IN ('ativo', 'pendente_documentos')
		// — NUNCA est.CodigoAcademia == nil, porque codigo_academia permanece
		// preenchido para sempre em cada estudante mesmo após desvinculação
		// (ver comentário em Estudante.Deletar e EstudanteProjection.CountVinculadosAtivos).
		// Antes desta correção, esta checagem usava CodigoAcademia == nil, que
		// nunca é verdadeiro na prática: um estudante desvinculado (Status =
		// 'inativo') continuava passando por aqui e a solicitação ia parar na
		// fila de aprovação da academia da qual ele já tinha saído. Estudantes
		// não vinculados usam PUT /estudante/bilhete-identidade (sem aprovação)
		// em vez desta rota.
		if est.Status != "ativo" && est.Status != "pendente_documentos" {
			utils.RespondWithValidationError(c, fmt.Errorf("estudante sem academia vinculada no momento (status atual: %s); use PUT /estudante/bilhete-identidade para autoatualizar sem aprovação", est.Status))
			return
		}
```

**Atenção:** esta função (`CriarSolicitacaoEdicaoDadoEstudanteHandler`) é compartilhada pelos 4 campos sensíveis (`nome`, `bilhete_identidade`, `bilhete_identidade_encarregado`, `data_nascimento` — ver os 4 registros de rota em `main.go` que chamam essa mesma função com parâmetros diferentes). A correção acima vale para os 4, e é a correta para os 4 — não é algo específico de `bilhete_identidade`. Isso está correto e é intencional (ver decisão 7 e decisão 10 na seção 2): os outros 3 campos continuam exigindo vínculo ativo, só que agora a checagem funciona de verdade.

### 5.5 Novo handler (NOVO ARQUIVO)

Criar `internal/handlers/bilhete_identidade_sem_academia_handlers.go` com este conteúdo exato:

```go
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

type bilheteIdentidadeSemAcademiaRequest struct {
	BilheteIdentidade string `json:"bilhete_identidade"`
}

// AtualizarBilheteIdentidadeSemAcademia é a via de autoatualização do BI do
// próprio estudante — só disponível quando ele NÃO está vinculado a nenhuma
// academia no momento (Status != "ativo" && Status != "pendente_documentos";
// NUNCA CodigoAcademia == nil, ver comentário em
// aggregates.Estudante.AlterarBilheteIdentidadeSemAcademia). Quando há
// academia vinculada, o único caminho é a solicitação com aprovação da
// academia: POST /estudante/solicitacoes-edicao/bilhete-identidade (ver
// CriarSolicitacaoEdicaoDadoEstudanteHandler, que exige Status IN ('ativo',
// 'pendente_documentos')). As duas rotas juntas cobrem os dois casos: com e
// sem academia vinculada no momento.
func AtualizarBilheteIdentidadeSemAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	est, err := getEstudanteProjection(c).GetByID(userID)
	if err != nil || est == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	// "Vinculado a uma academia" é Status IN ('ativo', 'pendente_documentos')
	// — NUNCA est.CodigoAcademia == nil: codigo_academia permanece preenchido
	// para sempre em cada estudante mesmo depois de desvinculado (ver
	// comentário em aggregates.Estudante.Deletar e
	// EstudanteProjection.CountVinculadosAtivos). Um estudante desvinculado
	// (Status = 'inativo') tem CodigoAcademia preenchido com a academia
	// ANTERIOR, mas não está mais vinculado a ela — é exatamente esse
	// estudante que esta rota atende.
	if est.Status == "ativo" || est.Status == "pendente_documentos" {
		utils.RespondWithValidationError(c, fmt.Errorf("você está vinculado a uma academia; use POST /estudante/solicitacoes-edicao/bilhete-identidade para solicitar a alteração com aprovação da academia"))
		return
	}

	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decErr := decoder.Decode(&raw); decErr != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("body inválido: envie um JSON com o campo bilhete_identidade"))
		return
	}
	if len(raw) != 1 {
		utils.RespondWithValidationError(c, fmt.Errorf("forneça somente o campo bilhete_identidade"))
		return
	}
	rawValor, ok := raw["bilhete_identidade"]
	if !ok {
		utils.RespondWithValidationError(c, fmt.Errorf("campo bilhete_identidade é obrigatório"))
		return
	}
	var req bilheteIdentidadeSemAcademiaRequest
	if jsonErr := json.Unmarshal(rawValor, &req.BilheteIdentidade); jsonErr != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("bilhete_identidade deve ser uma string"))
		return
	}

	// validarValorSolicitadoEdicao já cobre: obrigatoriedade, formato
	// (utils.ValidateBilhete), unicidade (BilheteIdentidadeExists) e "deve
	// ser diferente do valor atual" — mesma validação usada no fluxo de
	// solicitação, reaproveitada aqui para não divergir de regra.
	_, novoValidado, verr := validarValorSolicitadoEdicao(c, est, aggregates.CampoEdicaoBI, strings.TrimSpace(req.BilheteIdentidade))
	if verr != nil {
		return // validarValorSolicitadoEdicao já escreveu a resposta de erro
	}

	repository := getRepository(c)
	agg, loadErr := repository.Load(est.ID, "Estudante")
	if loadErr != nil {
		utils.RespondWithInternalError(c, loadErr)
		return
	}
	estudanteAgg := agg.(*aggregates.Estudante)
	if applyErr := estudanteAgg.AlterarBilheteIdentidadeSemAcademia(novoValidado); applyErr != nil {
		utils.RespondWithValidationError(c, applyErr)
		return
	}
	if saveErr := repository.SaveWithAudit(estudanteAgg, db.AuditContext{UserID: userID.String(), UserType: "estudante", IP: c.ClientIP()}); saveErr != nil {
		utils.RespondWithInternalError(c, saveErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            "bilhete de identidade atualizado com sucesso",
		"bilhete_identidade": novoValidado,
	})
}
```

### 5.6 Registrar a rota

Arquivo: `cmd/server/main.go`

**Localizar:**
```go
		estudante.POST("/solicitacoes-edicao/nome", handlers.CriarSolicitacaoEdicaoDadoEstudanteHandler("nome"))
```

**Substituir por:**
```go
		estudante.PUT("/bilhete-identidade", handlers.AtualizarBilheteIdentidadeSemAcademia)
		estudante.POST("/solicitacoes-edicao/nome", handlers.CriarSolicitacaoEdicaoDadoEstudanteHandler("nome"))
```

### 5.7 Teste de integração (NOVO ARQUIVO)

Criar `internal/handlers/bilhete_identidade_sem_academia_integration_test.go` com este conteúdo exato (já rodei os 3 testes contra Postgres real, reproduzindo o fluxo real de vincular → desvincular → tentar as duas rotas — todos passam; ver seção 8):

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
	"spuri/internal/storage"
)

func setupBilheteIdentidadeSemAcademiaTestRouter(client *db.Client, userID uuid.UUID) *gin.Engine {
	repository := db.NewAggregateRepository(client)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", client)
		c.Set("repository", repository)
		c.Set("user_id", userID)
		c.Set("user_type", "estudante")
		c.Set("storageProvider", storage.NewLocalProvider())
	})
	router.PUT("/estudante/bilhete-identidade", AtualizarBilheteIdentidadeSemAcademia)
	router.POST("/estudante/solicitacoes-edicao/bilhete-identidade", CriarSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade"))
	return router
}

// criarEstudanteVinculadoParaTeste cria (via evento real, não SQL direto) um
// estudante fundamental vinculado a uma academia recém-criada, e devolve o
// ID do estudante e o código da academia.
func criarEstudanteVinculadoParaTeste(t *testing.T, client *db.Client) (uuid.UUID, string) {
	t.Helper()
	codigoAcademia := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigoAcademia, "fundamental", []string{"1_ano_fundamental"})

	estID := uuid.New()
	agg := &aggregates.Estudante{}
	agg.SetID(estID)
	codigoEstudante := "E" + uuid.New().String()[:6]
	anoEscolar := "1_ano_fundamental"
	telefoneEncarregado := fmt.Sprintf("9%08d", time.Now().UnixNano()%100000000)
	bilheteResp := "999999999999ZZ"
	documentos := map[string]aggregates.DocumentoMatricula{
		"bi_encarregado":   {Path: "teste/bi_encarregado.pdf"},
		"cedula_estudante": {Path: "teste/cedula_estudante.pdf"},
	}
	if err := agg.CriarComVinculo(
		"Estudante Teste "+codigoEstudante, codigoEstudante, "hash",
		nil, nil, &telefoneEncarregado, nil, &bilheteResp,
		"masculino", time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
		&anoEscolar, nil, nil, nil, nil,
		&academiaID, codigoAcademia, documentos,
	); err != nil {
		t.Fatalf("erro ao criar estudante de teste: %v", err)
	}
	repository := db.NewAggregateRepository(client)
	if err := repository.SaveWithAudit(agg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("erro ao salvar estudante de teste: %v", err)
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
	}
	return estID, codigoAcademia
}

func desvincularEstudanteDeTeste(t *testing.T, client *db.Client, estID uuid.UUID, codigoAcademia string) {
	t.Helper()
	repository := db.NewAggregateRepository(client)
	agg, err := repository.Load(estID, "Estudante")
	if err != nil {
		t.Fatalf("erro ao carregar estudante para desvincular: %v", err)
	}
	estAgg := agg.(*aggregates.Estudante)
	if err := estAgg.DesvincularDaAcademia(codigoAcademia, "fim de teste de integração", uuid.New()); err != nil {
		t.Fatalf("erro ao desvincular estudante de teste: %v", err)
	}
	if err := repository.SaveWithAudit(estAgg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("erro ao salvar desvinculação: %v", err)
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
	}
}

// TestEstudanteVinculadoNaoPodeUsarRotaDireta cobre o caminho "com
// academia": a rota direta deve recusar e apontar para a solicitação.
func TestEstudanteVinculadoNaoPodeUsarRotaDireta(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, _ := criarEstudanteVinculadoParaTeste(t, client)
	router := setupBilheteIdentidadeSemAcademiaTestRouter(client, estID)

	body, _ := json.Marshal(map[string]any{"bilhete_identidade": generateBITest()})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/estudante/bilhete-identidade", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 (estudante vinculado não pode usar rota direta), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}

// TestEstudanteVinculadoAindaConsegueSolicitarComAprovacao cobre o caso
// positivo da correção: Status='ativo' continua autorizado a usar o fluxo
// de solicitação (regressão do fix da seção 5.4).
func TestEstudanteVinculadoAindaConsegueSolicitarComAprovacao(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, _ := criarEstudanteVinculadoParaTeste(t, client)
	router := setupBilheteIdentidadeSemAcademiaTestRouter(client, estID)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("novo_valor", generateBITest())
	fw, _ := createPDFFormFile(w, "documento", "doc.pdf")
	_, _ = fw.Write(minimalPDFBytesForTest())
	_ = w.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/estudante/solicitacoes-edicao/bilhete-identidade", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperava 201 (estudante vinculado pode solicitar), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}

// TestEstudanteDesvinculadoPodeUsarRotaDiretaMasNaoSolicitacao é o teste
// central da correção: reproduz o bug real (CodigoAcademia permanece
// preenchido após desvincular) e confirma que, depois do fix, um estudante
// desvinculado (Status='inativo') consegue autoatualizar o BI pela rota
// direta e é corretamente barrado da rota de solicitação (que exigiria
// aprovação de uma academia da qual ele já não faz parte).
func TestEstudanteDesvinculadoPodeUsarRotaDiretaMasNaoSolicitacao(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	desvincularEstudanteDeTeste(t, client, estID, codigoAcademia)

	var codigoAcademiaPosDesvinculo *string
	if err := client.DB().QueryRow(`SELECT codigo_academia FROM projection_estudantes WHERE id = $1`, estID).Scan(&codigoAcademiaPosDesvinculo); err != nil {
		t.Fatalf("erro ao verificar codigo_academia pós-desvínculo: %v", err)
	}
	if codigoAcademiaPosDesvinculo == nil {
		t.Fatalf("premissa do bug não reproduzida: codigo_academia deveria continuar preenchido após desvincular")
	}

	router := setupBilheteIdentidadeSemAcademiaTestRouter(client, estID)

	// Rota direta deve funcionar.
	body, _ := json.Marshal(map[string]any{"bilhete_identidade": generateBITest()})
	recDireta := httptest.NewRecorder()
	reqDireta := httptest.NewRequest(http.MethodPut, "/estudante/bilhete-identidade", bytes.NewReader(body))
	reqDireta.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recDireta, reqDireta)
	if recDireta.Code != http.StatusOK {
		t.Fatalf("esperava 200 na rota direta para estudante desvinculado, recebeu %d: %s", recDireta.Code, recDireta.Body.String())
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("novo_valor", generateBITest())
	fw, _ := createPDFFormFile(w, "documento", "doc.pdf")
	_, _ = fw.Write(minimalPDFBytesForTest())
	_ = w.Close()

	recSolicitacao := httptest.NewRecorder()
	reqSolicitacao := httptest.NewRequest(http.MethodPost, "/estudante/solicitacoes-edicao/bilhete-identidade", &buf)
	reqSolicitacao.Header.Set("Content-Type", w.FormDataContentType())
	router.ServeHTTP(recSolicitacao, reqSolicitacao)
	if recSolicitacao.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 na rota de solicitação para estudante desvinculado (sem academia para aprovar), recebeu %d: %s", recSolicitacao.Code, recSolicitacao.Body.String())
	}
}

// minimalPDFBytesForTest gera um PDF mínimo válido o bastante para passar
// por readAndValidatePDF nos testes de integração deste arquivo.
func minimalPDFBytesForTest() []byte {
	return []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF")
}

// createPDFFormFile é como multipart.Writer.CreateFormFile, mas com
// Content-Type "application/pdf" (CreateFormFile hardcoda
// application/octet-stream, e readAndValidatePDF exige application/pdf).
func createPDFFormFile(w *multipart.Writer, fieldName, fileName string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+fileName+`"`)
	h.Set("Content-Type", "application/pdf")
	return w.CreatePart(h)
}

// generateBITest gera um bilhete de identidade único e válido (12 números +
// 2 letras) para evitar colisão de unicidade entre estudantes de teste
// quando vários testes deste arquivo rodam na mesma execução/banco.
func generateBITest() string {
	n := time.Now().UnixNano() % 1000000000000
	return fmt.Sprintf("%012dAB", n)
}
```

Este arquivo depende de `criarAcademiaEscolarParaTeste` e `nivelEscolarTestClient`, definidas em `nivel_escolar_handlers_integration_test.go` (seção 4.7) — os dois arquivos de teste precisam existir juntos.

## 6. Comandos de verificação que o Codex deve rodar

```bash
go build ./...
go vet ./...
go test ./internal/db/... -run TestValidateEventTypeAcceptsEventsDiscoveredInCode -v
go test ./...
```

**Nota importante sobre o ambiente do Codex:** os testes de integração desta tarefa (`nivel_escolar_handlers_integration_test.go` e `bilhete_identidade_sem_academia_integration_test.go`) só rodam de verdade se a variável `DATABASE_URL` estiver definida e apontando para um Postgres acessível — algo que o ambiente do Codex provavelmente não tem (`apt` bloqueado, sem Docker, sem `psql`). Isso **já foi previsto**: a função `nivelEscolarTestClient` chama `t.Skip(...)` quando `DATABASE_URL` está vazio, então esses testes aparecem como `SKIP`, não `FAIL`, e `go test ./...` continua verde. **Um `SKIP` nesses dois arquivos é o resultado esperado no ambiente do Codex — não é motivo para investigar mais.** Eu (Claude) já rodei esses testes com Postgres real e o resultado está na seção 8 abaixo; o Codex não precisa repetir isso.

O que o Codex **deve** conseguir rodar e ver passar de verdade, sem precisar de banco: `go build ./...`, `go vet ./...`, e todo o resto de `go test ./...` que não seja os dois arquivos de integração novos (tudo isso já passava antes desta tarefa e continua passando).

## 7. O que eu (Claude) já validei com Postgres real

Para que fique registrado e o Codex não precise repetir (e não consiga, dada a limitação do ambiente):

1. Instalei PostgreSQL 16 num sandbox com acesso root, criei um banco `spuri_test`/`spuri_it` e apliquei as 130 migrações do repositório do zero, em ordem — todas aplicaram sem erro.
2. Semeei manualmente dados realistas (3 academias: uma fundamental com 1 dependência ativa em cada uma das 6 categorias aplicáveis, uma médio com 1 dependência ativa em cada uma das 7 categorias aplicáveis, e uma fundamental "limpa" só com dados históricos/inativos/reprovados) e rodei as 7 queries SQL exatas usadas em `contarDependenciasNivelEscolar` diretamente no `psql` — todas retornaram exatamente os números esperados, inclusive confirmando que o `LIKE ... ESCAPE '\'` não cai na armadilha do `_` como wildcard do SQL.
3. Antes de escrever qualquer Go, reproduzi manualmente via `UPDATE` direto no Postgres o bug do `anos_academicos = '[]'` vs `NULL` (seção 2, decisão 6) — confirmando o erro de constraint e depois confirmando que `NULL` funciona.
4. Implementei todo o código desta tarefa, rodei `go build ./...` e `go vet ./...` — limpos.
5. Rodei a suíte completa (`go test ./...`) do repositório inteiro, com `DATABASE_URL` apontando para o Postgres real — **todos os pacotes passaram**, incluindo os testes já existentes (nada quebrou) e os 7 testes de integração novos desta tarefa (4 de `nivel_escolar`, 3 de `bilhete-identidade-sem-academia`).
6. O teste `TestEstudanteDesvinculadoPodeUsarRotaDiretaMasNaoSolicitacao` reproduz o fluxo real ponta a ponta: cria um estudante vinculado via evento real (`CriarComVinculo`), desvincula via evento real (`DesvincularDaAcademia`), confirma que `codigo_academia` de fato continua preenchido no banco (reproduzindo o bug antes do fix), e confirma que depois do fix a rota direta funciona e a rota de solicitação é corretamente barrada.
7. Rodei `go test ./internal/db/... -run TestValidateEventTypeAcceptsEventsDiscoveredInCode -v` e confirmei que o novo tipo de evento `BilheteIdentidadeEstudanteAlteradoDiretamente` foi descoberto automaticamente e passou (esse teste existente varre o código por literais `EventType: "..."` e falha se algum não estiver na whitelist de `safe_queries.go` — é a rede de segurança que garante que a seção 5.3 não foi esquecida).

## 8. Checklist de aceite

- [ ] `PUT /academia/dados` rejeita `type` e `nivel_escolar` sempre, com as novas mensagens (seção 4.3).
- [ ] `PUT /academia/nivel-escolar` existe, autenticado só para a academia dona.
- [ ] Transições que perdem um domínio (`→ medio` vindo de `fundamental`/`misto`; `→ fundamental` vindo de `medio`/`misto`) são bloqueadas com 409 quando há dependência ativa/pendente em qualquer uma das 7 categorias.
- [ ] Transições aditivas (`→ misto`) nunca são bloqueadas por dependências.
- [ ] `anos_academicos` é tratado corretamente nas 3 situações: `medio` como destino exige vazio (e limpa o que já existia); saindo de `medio` exige novo valor não vazio e validado; nos demais casos o campo é rejeitado se enviado.
- [ ] `anos_academicos` vira SQL `NULL` (não `'[]'`) ao virar `medio` — confirmar direto no banco, não só na resposta da API.
- [ ] `PUT /estudante/bilhete-identidade` existe e só aceita quando `Status` não é `ativo` nem `pendente_documentos`.
- [ ] `POST /estudante/solicitacoes-edicao/bilhete-identidade` (e os outros 3 campos que passam pela mesma função) só aceita quando `Status IN ('ativo', 'pendente_documentos')`.
- [ ] O novo tipo de evento está registrado nos 3 lugares (aggregate, projeção, whitelist do ledger).
- [ ] `go build ./...`, `go vet ./...` e `go test ./...` (com os dois testes de integração pulados via `SKIP`, se `DATABASE_URL` não estiver disponível) passam sem erro.

## 9. Procedimento de conclusão

1. Aplique todas as seções 4 e 5, na ordem.
2. Rode os comandos da seção 6.
3. Resolva qualquer erro de compilação antes de seguir — não deveria haver nenhum, já que todo este código foi compilado e testado antes de ser escrito aqui.
4. Confira o checklist da seção 8.
5. Não faça commit/push a menos que instruído separadamente.
