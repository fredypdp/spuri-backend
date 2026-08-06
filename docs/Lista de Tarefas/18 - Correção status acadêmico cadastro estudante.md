---
título: Correção — status acadêmico incorreto no cadastro de estudante
tarefa: 18
criticidade: crítica
autor: Claude (orquestrador)
executor: OpenAI Codex (free tier)
---

# Correção — status acadêmico incorreto no cadastro de estudante

## Recomendação de prompt para o Codex

> Leia este documento por completo antes de alterar qualquer arquivo. Siga a
> ordem das seções. Depois de cada alteração, rode `go build ./...` e
> `go test ./...` e cole o output. Não corrija nada que não esteja descrito
> aqui sem antes reportar o achado.

---

# 1. Contexto

Ao cadastrar um estudante (síncrono ou assíncrono), o backend grava
`status_escolar_fundamental`, `status_escolar_medio` e `status_superior` com
valores **fixos**, ignorando qual ano acadêmico (`ano_escolar_fundamental`,
`ano_escolar_medio` ou `ano_superior`) foi de fato informado no payload.

Resultado observado: uma escola mista cadastrou estudantes do **médio** e eles
nasceram com `status_escolar_fundamental = "em_andamento"` e
`status_escolar_medio = "inativo"` — o inverso do esperado.

## Regra de negócio correta

O status depende exclusivamente de qual campo de ano acadêmico foi informado
no cadastro:

| Nível informado | `status_escolar_fundamental` | `status_escolar_medio` | `status_superior` |
|---|---|---|---|
| `ano_escolar_fundamental` | `em_andamento` | `inativo` | `inativo` |
| `ano_escolar_medio` | `finalizado` | `em_andamento` | `inativo` |
| `ano_superior` | `finalizado` | `finalizado` | `em_andamento` |

Isso vale para **todas** as academias (fundamental, médio, misto e superior),
já que o nível é determinado pelo campo enviado no payload, não pelo tipo da
academia.

---

# 2. Diagnóstico — origem exata do erro

**Arquivo:** `internal/domain/aggregates/estudante.go`
**Função:** `criarComVinculoComStatus`

Esta função é o **único ponto de emissão** do evento
`EstudanteCriadoComVinculoEvent`. Todos os fluxos de cadastro convergem para
ela:

- `POST /academia/estudante/register` (síncrono)
- `POST /academia/estudante/register/async` (assíncrono)
- `PUT /academia/solicitacao-matricula/:codigo/aprovar` (aprovação de matrícula pública)

Dentro dela, os três status são atribuídos como constantes, sem consultar os
parâmetros `anoEscolar`, `anoEscolarMedio` e `anoSuperior` que a função já
recebe:

```go
statusFund := "em_andamento"
statusMed := "inativo"
statusSup := "inativo"
```

A projeção (`internal/projections/estudante_projection.go`, função
`handleEstudanteCriadoComVinculo`) e o `applyEstudanteCriadoComVinculo` do
próprio aggregate estão corretos — eles apenas persistem o que veio no
payload do evento, que já chega errado. **Não mexer nesses dois pontos.**

## Efeito colateral grave (não é só cosmético)

`Estudante.InterromperPercursoAcademico` (mesmo arquivo) conta quantas das
três etapas estão `"em_andamento"` e assume que é exatamente uma:

```go
if e.StatusEscolarFundamental == "em_andamento" {
    etapas++
    exec = func() error { return e.InterromperFundamental(motivo, aprovadoPor) }
}
if e.StatusEscolarMedio == "em_andamento" { ... }
if e.StatusSuperior == "em_andamento" { ... }
```

Para um estudante do médio afetado pelo bug, é o fundamental que aparece como
`"em_andamento"`. Uma interrupção/desvinculação desse estudante vai disparar
`FundamentalInterrompidoEvent` em vez de `MedioInterrompidoEvent` — um evento
errado gravado no ledger (append-only, não é editável depois).

Também afeta qualquer filtro por `status_escolar_medio=em_andamento` em
`GET /estudantes` (`internal/handlers/estudante_handlers.go`).

---

# 3. Correção

## 3.1. Nova função de derivação de status

Adicionar em `internal/domain/aggregates/estudante.go` (o pacote `strings` já
está importado no arquivo):

```go
// deriveStatusPorAnoAcademico determina o status_escolar_fundamental,
// status_escolar_medio e status_superior a partir de qual ano acadêmico foi
// informado no cadastro. Exatamente um nível fica "em_andamento"; os níveis
// anteriores ficam "finalizado" (o estudante já os concluiu para estar
// matriculado no nível informado); os níveis posteriores ficam "inativo"
// (ainda não alcançados).
func deriveStatusPorAnoAcademico(anoEscolar, anoEscolarMedio, anoSuperior *string) (fund, medio, sup string) {
	switch {
	case anoSuperior != nil && strings.TrimSpace(*anoSuperior) != "":
		return "finalizado", "finalizado", "em_andamento"
	case anoEscolarMedio != nil && strings.TrimSpace(*anoEscolarMedio) != "":
		return "finalizado", "em_andamento", "inativo"
	case anoEscolar != nil && strings.TrimSpace(*anoEscolar) != "":
		return "em_andamento", "inativo", "inativo"
	default:
		return "inativo", "inativo", "inativo"
	}
}
```

## 3.2. Usar a função em `criarComVinculoComStatus`

Substituir:

```go
statusFund := "em_andamento"
statusMed := "inativo"
statusSup := "inativo"
```

por:

```go
statusFund, statusMed, statusSup := deriveStatusPorAnoAcademico(anoEscolar, anoEscolarMedio, anoSuperior)
```

Confirmar que os nomes dos parâmetros de entrada da função batem exatamente
com esses (`anoEscolar`, `anoEscolarMedio`, `anoSuperior`) — se os nomes reais
no código forem outros, usar os nomes reais, apenas mantendo a mesma lógica.

## 3.3. Validação de exclusividade (recomendado, evita a mesma classe de bug no futuro)

No início de `criarComVinculoComStatus`, antes da derivação de status,
adicionar:

```go
niveisInformados := 0
if !isNilOrBlank(anoEscolar) {
    niveisInformados++
}
if !isNilOrBlank(anoEscolarMedio) {
    niveisInformados++
}
if !isNilOrBlank(anoSuperior) {
    niveisInformados++
}
if niveisInformados > 1 {
    return fmt.Errorf("apenas um nível acadêmico pode ser informado no cadastro: ano_escolar_fundamental, ano_escolar_medio ou ano_superior")
}
```

`isNilOrBlank` já existe no arquivo. Se `fmt` não estiver importado, importar.
Se a função não retornar `error` neste ponto, ajustar a assinatura/retorno
propagando o erro até o handler (verificar como os demais `return fmt.Errorf`
já existentes na função tratam isso, e seguir o mesmo padrão).

## 3.4. Testes

Criar `internal/domain/aggregates/estudante_criar_vinculo_status_test.go`,
seguindo o padrão de `bilhetes_validation_test.go` (que já usa
`CriarComVinculo`):

```go
func TestCriarComVinculoDerivaStatusPorNivelFundamental(t *testing.T) {
	// cadastrar com ano_escolar_fundamental preenchido
	// assert StatusEscolarFundamental == "em_andamento"
	// assert StatusEscolarMedio == "inativo"
	// assert StatusSuperior == "inativo"
}

func TestCriarComVinculoDerivaStatusPorNivelMedio(t *testing.T) {
	// cadastrar com ano_escolar_medio preenchido
	// assert StatusEscolarFundamental == "finalizado"
	// assert StatusEscolarMedio == "em_andamento"
	// assert StatusSuperior == "inativo"
}

func TestCriarComVinculoDerivaStatusPorNivelSuperior(t *testing.T) {
	// cadastrar com ano_superior preenchido
	// assert StatusEscolarFundamental == "finalizado"
	// assert StatusEscolarMedio == "finalizado"
	// assert StatusSuperior == "em_andamento"
}

func TestCriarComVinculoRejeitaMaisDeUmNivelInformado(t *testing.T) {
	// cadastrar com ano_escolar_fundamental E ano_escolar_medio preenchidos
	// assert erro retornado
}
```

E um teste de regressão para o efeito colateral da seção 2:

```go
func TestInterromperPercursoAcademicoInterrompeNivelCorretoAposCadastroMedio(t *testing.T) {
	// cadastrar estudante do médio via CriarComVinculo
	// chamar InterromperPercursoAcademico
	// assert que o último evento emitido é *MedioInterrompidoEvent
	// (não *FundamentalInterrompidoEvent)
}
```

## 3.5. Critérios de aceite

1. `go build ./...` e `go test ./...` passam sem erro.
2. Cadastro (síncrono, assíncrono e via aprovação de matrícula) com
   `ano_escolar_fundamental` preenchido resulta em
   `status_escolar_fundamental="em_andamento"` e os outros dois `"inativo"`.
3. Cadastro com `ano_escolar_medio` preenchido resulta em
   `status_escolar_medio="em_andamento"`, `status_escolar_fundamental="finalizado"`,
   `status_superior="inativo"`.
4. Cadastro com `ano_superior` preenchido resulta em
   `status_superior="em_andamento"` e os outros dois `"finalizado"`.
5. Cadastro com mais de um nível preenchido simultaneamente é rejeitado com
   erro claro (se o item 3.3 for implementado).
6. Testado manualmente em pelo menos um cenário de cada tipo de academia
   (fundamental, médio, misto, superior) via `POST /academia/estudante/register`.

## 3.6. Fora de escopo desta tarefa

- Qualquer alteração em `EstudanteCriadoComVinculoEvent`,
  `handleEstudanteCriadoComVinculo` ou no schema da tabela
  `projection_estudantes` relacionado a esses três campos de status — eles já
  estão corretos e não devem ser tocados.
- Fluxo de promoção de ano (`PromoverAno` ou equivalente) — este documento
  cobre apenas o momento do **cadastro**.

---

# 4. Extra — descontinuar a coluna/campo legado `ano_escolar`

## Diagnóstico

O campo Go `AnoEscolar` (`internal/domain/models.go`) **não é um campo
duplicado do ponto de vista da aplicação** — ele mapeia via `db` tag para a
coluna `ano_escolar_fundamental`, e é isso que `estudanteCols` em
`estudante_projection.go` seleciona. O nome do campo Go só está desalinhado
(poderia se chamar `AnoEscolarFundamental` para ficar simétrico com
`AnoEscolarMedio`/`AnoSuperior`), mas não representa risco.

O que é de fato legado é a **coluna física `ano_escolar`** no Postgres,
distinta de `ano_escolar_fundamental`:

- `migrations/001_complete_schema.sql` criou `ano_escolar` originalmente.
- `migrations/034_rename_ano_escolar_para_fundamental.sql` criou
  `ano_escolar_fundamental` e fez backfill a partir de `ano_escolar`.
- Desde a migration 034, **nenhum código Go grava mais em `ano_escolar`**.
  Toda a aplicação usa exclusivamente `ano_escolar_fundamental`.

A coluna `ano_escolar` está órfã. Como o banco será resetado, não há dado
histórico a preservar — a remoção pode ser direta, sem se preocupar com
backfill ou sincronização.

## Ponto de atenção: view dependente

`v_estudantes_com_cursos` (última definição na migration 033) ainda faz
`SELECT e.ano_escolar` diretamente. Se a coluna for removida sem recriar a
view antes, a migration falha por dependência. Nenhum handler do backend usa
essa view diretamente (os handlers fazem `SELECT` explícito em
`projection_estudantes`), então é seguro recriá-la sem a coluna legada.

## Ação recomendada

1. Rodar `ls migrations | sort | tail -5` para confirmar o próximo número
   livre de migration (referenciado abaixo como `100`, ajustar se necessário).
2. Criar a migration abaixo.
3. Rodar `grep -rn '"ano_escolar"' --include="*.go" .` e
   `grep -rn '\bano_escolar\b' --include="*.sql" migrations/` para confirmar
   que nenhuma outra query solta referencia a coluna sem sufixo antes de
   aplicar. Reportar qualquer ocorrência encontrada antes de prosseguir.
4. Como o campo Go `AnoEscolar` já mapeia corretamente para
   `ano_escolar_fundamental` via `db` tag, renomear o campo Go para
   `AnoEscolarFundamental` (e todos os usos no código) é opcional, mas
   recomendado para eliminar a confusão de nomenclatura. Se decidir fazer,
   tratar como commit separado do drop de coluna, para manter os dois
   diffs fáceis de revisar isoladamente.

```sql
-- MIGRATION 100 — Remover coluna legada projection_estudantes.ano_escolar
--
-- CONTEXTO:
--   Desde a migration 034, a aplicação usa exclusivamente
--   ano_escolar_fundamental. A coluna "ano_escolar" está órfã e não é mais
--   escrita por nenhum código Go. Banco em desenvolvimento, sem dado a
--   preservar.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Recria v_estudantes_com_cursos e v_estudante_completo (dependente via
--      CASCADE) sem a coluna legada.
--   2. Remove projection_estudantes.ano_escolar.

BEGIN;

DROP VIEW IF EXISTS v_estudantes_com_cursos CASCADE;
DROP VIEW IF EXISTS v_estudante_completo;

ALTER TABLE projection_estudantes DROP COLUMN IF EXISTS ano_escolar;

CREATE VIEW v_estudantes_com_cursos AS
SELECT
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.genero,
    e.codigo_academia,
    e.status,
    e.status_escolar_fundamental,
    e.status_escolar_medio,
    e.status_superior,
    e.ano_escolar_fundamental,
    e.ano_escolar_medio,
    e.ano_superior,
    e.email_verificado,
    cm.id   AS curso_medio_id,
    cm.nome AS curso_medio_nome,
    cm.type AS curso_medio_type,
    cs.id   AS curso_superior_id,
    cs.nome AS curso_superior_nome,
    cs.type AS curso_superior_type,
    e.created_at,
    e.updated_at
FROM projection_estudantes e
LEFT JOIN projection_cursos cm ON e.curso_medio_id    = cm.id AND cm.deleted_at IS NULL
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id AND cs.deleted_at IS NULL;

CREATE VIEW v_estudante_completo AS
SELECT
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas     n WHERE n.codigo_estudante = e.codigo_estudante) AS notas,
    (SELECT json_agg(f.*) FROM projection_faltas     f WHERE f.codigo_estudante = e.codigo_estudante) AS faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id     = e.id)               AS inscricoes
FROM projection_estudantes e;

COMMIT;
```

### Critérios de aceite (item extra)

1. `grep` de confirmação (passo 3 acima) não retorna nenhuma referência
   residual a `ano_escolar` fora de `ano_escolar_fundamental`/`ano_escolar_medio`.
2. Migration aplicada com sucesso em banco de desenvolvimento (após reset).
3. `go build ./...` e `go test ./...` continuam passando.
4. `GET /estudantes` e demais rotas que retornam dados de estudante seguem
   funcionando normalmente, usando `ano_escolar_fundamental`.
