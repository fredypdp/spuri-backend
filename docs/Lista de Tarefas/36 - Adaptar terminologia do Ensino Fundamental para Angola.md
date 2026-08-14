#Adaptar terminologia do "Ensino Fundamental" para Angola

Repositório: `fredypdp/spuri-backend`
Escopo analisado: **todo o código-fonte, exceto o diretório `/docs`** (migrations, docs internos de tarefas/debugs e o histórico de SQL não fazem parte do escopo).
Analisado por: Claude (orquestrador) — mapeamento completo por grep + leitura de contexto de cada ocorrência.
Executor: Codex.

---

## 1. Objetivo

A API usa internamente o formato `N_ano_fundamental` (ex.: `"5_ano_fundamental"`) para representar os anos do que hoje é chamado de "Ensino Fundamental". **Isso não muda.** O que muda é somente o **texto em português retornado ao cliente** (mensagens de erro, mensagens de sucesso, rótulos de documentos etc.), que deve passar a refletir a terminologia usada em Angola:

| Nível interno | Angola |
|---|---|
| 1_ano_fundamental … 6_ano_fundamental | Ensino Primário — 1ª à 6ª Classe |
| 7_ano_fundamental … 9_ano_fundamental | Ensino Secundário (Iº Ciclo) — 7ª à 9ª Classe |

### Regras de rotulagem (definidas pelo dono do produto)

1. **Citação genérica** a "Ensino Fundamental" (sem apontar um ano específico) → **"Ensino Primário e Iº Ciclo"**
2. **Referência isolada a um ano específico** ("N.º Ano do Fundamental", "certificado do N.º ano fundamental" etc.) → **"Nª Classe"**
3. **Referência em contexto de escola/academia mista** (que abrange do fundamental ao médio) → **"Ensino Primário ao Médio"**

> Importante: **nenhum valor literal aceito pela API muda** — nem `N_ano_fundamental`, nem os valores de enum `fundamental`/`medio`/`superior` usados nos campos `nivel`, `type`, `tipo_ensino`. Só o texto explicativo em português muda.

---

## 2. Mecanismo a criar (fonte única da verdade para os rótulos)

Criar o arquivo **`internal/utils/terminologia_angola.go`** (pacote `utils`, já importado pela maioria dos arquivos que precisam ser alterados — ver seção 5):

```go
package utils

import "strings"

// RotuloEnsinoFundamentalGenerico é o rótulo a usar sempre que uma mensagem
// citar o "Ensino Fundamental" de forma genérica, sem apontar um ano
// específico (Regra 1).
const RotuloEnsinoFundamentalGenerico = "Ensino Primário e Iº Ciclo"

// RotuloEnsinoMisto é o rótulo a usar em referências que abrangem, em
// conjunto, o ensino fundamental e o ensino médio — contexto de
// academias/escolas mistas (Regra 3).
const RotuloEnsinoMisto = "Ensino Primário ao Médio"

// RotuloClasseFundamental converte o código interno de um ano do ensino
// fundamental (formato "N_ano_fundamental") no rótulo de exibição usado em
// Angola: "Nª Classe" (Regra 2).
//
// O valor interno "N_ano_fundamental" NUNCA é alterado em lógica/validação/
// persistência — esta função serve apenas para gerar texto amigável em
// mensagens devolvidas ao cliente.
//
// Caso a entrada não esteja no formato esperado, a função devolve o valor
// original sem modificação (fallback seguro, nunca quebra uma mensagem).
func RotuloClasseFundamental(anoFundamental string) string {
	trimmed := strings.TrimSpace(anoFundamental)
	numero := strings.TrimSuffix(trimmed, "_ano_fundamental")
	if numero == "" || numero == trimmed {
		return trimmed
	}
	for _, r := range numero {
		if r < '0' || r > '9' {
			return trimmed
		}
	}
	return numero + "ª Classe"
}
```

Uso típico dentro de uma mensagem já existente:

```go
// antes
return fmt.Errorf("ano_academico fundamental não ofertado pela academia: %s", ano)

// depois
return fmt.Errorf("ano acadêmico do %s não ofertado pela academia: %s", utils.RotuloEnsinoFundamentalGenerico, utils.RotuloClasseFundamental(ano))
```

---

## 3. Regra de classificação — o que muda vs. o que NÃO muda

Esta é a regra que resolve os casos ambíguos e deve ser aplicada de forma consistente pelo Codex.

### NÃO alterar quando a palavra "fundamental" aparecer:

- **(a) Listando/citando entre aspas o valor literal aceito por um campo**, ex.: `"nivel deve ser fundamental, medio ou superior"`, `"type deve ser 'fundamental' ou 'medio'"`, `"Use: fundamental, medio ou superior"`. É exatamente o valor que o cliente deve enviar — mudar o texto induziria o cliente a erro.
- **(b) Logo após o nome do campo que a contém como valor**, ex.: `"nivel fundamental"`, `"regras de nivel fundamental"`, `"tipo_ensino deve ser fundamental"`. Mesmo raciocínio de (a).
- **(c) Como parte do nome de um campo JSON/identificador** citado no próprio texto do erro para apontar ao cliente qual campo falhou, ex.: `ano_escolar_fundamental inválido`, `status_escolar_fundamental`. O nome do campo continua sendo `ano_escolar_fundamental` — não pode virar outra coisa dentro da própria mensagem que o referencia.
- **(d) Como parte do formato técnico/exemplo de valor**, ex.: `"Formato esperado: [1-9]_ano_fundamental (ex.: 1_ano_fundamental)"`. É a sintaxe real que a API espera.
- **(e)** Em comentários de código, tags de struct, colunas de banco, SQL, nomes de função/variável, tipos de evento (`FundamentalRetomado`, `FundamentalInterrompido`, etc.) — tudo isso é lógica interna, não texto devolvido ao cliente.
- **(f)** Em qualquer arquivo dentro de `/migrations` — são registros históricos de schema, nunca devem ser editados.

### MUDAR quando a palavra "fundamental"/"fundamentais" for usada como prosa descritiva dirigida ao usuário

Ou seja, quando **não** se enquadra em nenhum dos casos (a)–(f) acima — normalmente frases como "ensino fundamental", "ano fundamental", "anos fundamentais", "matérias fundamentais", "certificado do N.º ano fundamental", "sequência fundamental", "interromper fundamental em andamento".

Aplicar a Regra 1, 2 ou 3 (seção 1) conforme o caso.

---

## 4. Lista exaustiva de alterações confirmadas

Todas as ocorrências abaixo foram localizadas com grep completo do repositório (exceto `/docs`) e revisadas com o contexto do arquivo. Formato: `arquivo:linha` — texto atual → texto novo.

### Regra 1 — citação genérica ("Ensino Fundamental" → "Ensino Primário e Iº Ciclo")

| # | Arquivo:linha | Texto atual | Texto novo |
|---|---|---|---|
| 1 | `internal/utils/validation.go:392` | `"ano '%s' inválido para o ensino fundamental. " + "Formato esperado: [1-9]_ano_fundamental (ex.: 1_ano_fundamental, 9_ano_fundamental)"` | `"ano '%s' inválido para o " + utils.RotuloEnsinoFundamentalGenerico + ". " + "Formato esperado: [1-9]_ano_fundamental (ex.: 1_ano_fundamental, 9_ano_fundamental)"` (nota: está dentro do próprio pacote `utils`, então usar `RotuloEnsinoFundamentalGenerico` sem prefixo) |
| 2 | `internal/utils/validation.go:401` | idêntico ao #1, para o erro de número fora do intervalo | mesma substituição do #1 |
| 3 | `internal/utils/validation.go:479` | `"matérias fundamentais devem ter pelo menos um ano em anos_academicos"` | `"matérias do " + RotuloEnsinoFundamentalGenerico + " devem ter pelo menos um ano em anos_academicos"` |
| 4 | `internal/finance/matricula.go:134` | `"curso_id não é permitido para ensino fundamental"` | `"curso_id não é permitido para o " + utils.RotuloEnsinoFundamentalGenerico` (requer adicionar import `"spuri/internal/utils"`) |
| 5 | `internal/finance/matricula.go:138` | `"ano_academico fundamental não é oferecido pela academia"` | `"ano acadêmico do " + utils.RotuloEnsinoFundamentalGenerico + " não é oferecido pela academia"` |
| 6 | `internal/finance/mensalidade.go:406` | igual ao #4 | mesma substituição do #4 (requer import `utils`) |
| 7 | `internal/finance/mensalidade.go:410` | igual ao #5 | mesma substituição do #5 |
| 8 | `internal/handlers/notas_handlers.go:349` | `"matrícula no ensino fundamental do estudante não está em andamento"` | `"matrícula no " + utils.RotuloEnsinoFundamentalGenerico + " do estudante não está em andamento"` |
| 9 | `internal/handlers/solicitacao_matricula_handlers.go:824` | `"o ano fundamental informado não pertence a uma academia de nível superior"` | `"o ano do " + utils.RotuloEnsinoFundamentalGenerico + " informado não pertence a uma academia de nível superior"` |
| 10 | `internal/handlers/turmas_handler.go:373` | `"estudante não possui ano escolar fundamental definido — configure o ano escolar antes de vincular à turma '%s'"` | `"estudante não possui ano escolar do " + utils.RotuloEnsinoFundamentalGenerico + " definido — configure o ano escolar antes de vincular à turma '%s'"` |
| 11 | `internal/domain/aggregates/estudante.go:710` | `"só pode interromper fundamental em andamento"` | `"só pode interromper o " + utils.RotuloEnsinoFundamentalGenerico + " em andamento"` |
| 12 | `internal/handlers/avaliacao_final_regras.go:280` | `"anos_academicos de regra fundamental deve ser array simples de strings; não envie escopo por curso"` | `"anos_academicos de regra do " + utils.RotuloEnsinoFundamentalGenerico + " deve ser array simples de strings; não envie escopo por curso"` |
| 13 | `internal/handlers/avaliacao_final_regras.go:542` | `"anos_academicos inválido para fundamental: %w"` | `"anos_academicos inválido para o " + utils.RotuloEnsinoFundamentalGenerico + ": %w"` |
| 14 | `internal/handlers/avaliacao_final_regras.go:599` | `"materias_aplicaveis fundamental não aceita curso_id"` | `"materias_aplicaveis do " + utils.RotuloEnsinoFundamentalGenerico + " não aceita curso_id"` |
| 15 | `internal/handlers/avaliacao_final_handler.go:1414` | `"nivel_atual '%s' não pertence à sequência fundamental (1_ano_fundamental..9_ano_fundamental)"` | `"nivel_atual '%s' não pertence à sequência do " + utils.RotuloEnsinoFundamentalGenerico + " (1_ano_fundamental..9_ano_fundamental)"` — manter o trecho entre parênteses sem alterar (é o formato técnico) |
| 16 | `internal/handlers/anos_academicos_handlers.go:98` | `"...Use 'fundamental' para anos do ensino fundamental, 'medio' para cursos médios..."` | trocar somente **"anos do ensino fundamental"** → **"anos do " + utils.RotuloEnsinoFundamentalGenerico**; manter `'fundamental'`, `'medio'`, `'superior'` citados entre aspas (são valores literais do campo `type`) |
| 17 | `internal/handlers/anos_academicos_handlers.go:105` | `"Esta academia não pode gerenciar anos do ensino fundamental porque o nível cadastrado é nivel='%s' e nivel_escolar='%s'. Somente academias escolares com nivel_escolar 'fundamental' ou 'misto' podem alterar anos fundamentais."` | trocar **"anos do ensino fundamental"** → **"anos do " + RotuloEnsinoFundamentalGenerico**, e **"anos fundamentais"** (no final) → **"anos do " + RotuloEnsinoFundamentalGenerico**; manter `nivel_escolar 'fundamental' ou 'misto'` (valores literais do campo) |
| 18 | `internal/handlers/anos_academicos_handlers.go:108` | `"...Exemplo válido para fundamental: ['1_ano_fundamental', '2_ano_fundamental']."` | `"...Exemplo válido para o " + RotuloEnsinoFundamentalGenerico + ": [...]"` (manter o array de exemplo intacto) |
| 19 | `internal/handlers/anos_academicos_handlers.go:115` | `"...Academias fundamental/misto precisam manter pelo menos um ano ativo."` | `"...Academias do " + RotuloEnsinoFundamentalGenerico + " ou mistas precisam manter pelo menos um ano ativo."` |

### Regra 2 — referência isolada a um ano específico ("Nª Classe", via `RotuloClasseFundamental`)

| # | Arquivo:linha | Texto atual | Texto novo |
|---|---|---|---|
| 20 | `internal/handlers/solicitacao_matricula_handlers.go:876` (`documentLabel`) | `return "certificado do 6.º ano fundamental"` | `return "certificado da " + utils.RotuloClasseFundamental("6_ano_fundamental")` → renderiza **"certificado da 6ª Classe"** |
| 21 | `internal/handlers/solicitacao_matricula_handlers.go:878` (`documentLabel`) | `return "certificado do 9.º ano fundamental"` | `return "certificado da " + utils.RotuloClasseFundamental("9_ano_fundamental")` → **"certificado da 9ª Classe"** |
| 22 | `internal/domain/aggregates/solicitacao_matricula.go:423` (`certificadoObrigatorioParaAno`) | `return "certificado_6_ano_fundamental", "certificado do 6.º ano fundamental"` | `return "certificado_6_ano_fundamental", "certificado da " + utils.RotuloClasseFundamental("6_ano_fundamental")` (o 1º valor, código do campo, **não muda**) |
| 23 | `internal/domain/aggregates/solicitacao_matricula.go:425` (`certificadoObrigatorioParaAno`) | `return "certificado_9_ano_fundamental", "certificado do 9.º ano fundamental"` | `return "certificado_9_ano_fundamental", "certificado da " + utils.RotuloClasseFundamental("9_ano_fundamental")` |

> ⚠️ `solicitacao_matricula.go` já importa `spuri/internal/utils` — confirmar que não há conflito de nome (`utils` já é usado nesse arquivo para outras validações).

### Recomendado (melhoria de UX) — usar `RotuloClasseFundamental` para valores dinâmicos já embutidos em mensagens

Estes dois pontos hoje mostram o código bruto (`5_ano_fundamental`) dentro de uma mensagem já destinada ao cliente. Não é uma citação genérica nem um valor de contrato — é literalmente "o ano X" sendo comunicado ao usuário, então cabe a Regra 2:

| # | Arquivo:linha | Texto atual | Texto novo |
|---|---|---|---|
| 24 | `internal/handlers/avaliacao_final_regras.go:572` | `fmt.Errorf("ano_academico fundamental não ofertado pela academia: %s", ano)` | `fmt.Errorf("ano acadêmico do %s não ofertado pela academia: %s", utils.RotuloEnsinoFundamentalGenerico, utils.RotuloClasseFundamental(ano))` |
| 25 | `internal/handlers/turmas_handler.go:378-381` (dentro do `case "fundamental":`) | `fmt.Errorf("estudante está no %s mas a turma é do nível %s", *anoEscolar, nivel)` | `fmt.Errorf("estudante está na %s mas a turma é do nível %s", utils.RotuloClasseFundamental(*anoEscolar), utils.RotuloClasseFundamental(nivel))` — **atenção**: alterar somente o bloco `case "fundamental":` (linhas ~377-382); os blocos `case "medio":` e `case "superior":` (linhas ~391-395 e ~423-427) usam a mesma frase só que para médio/superior e **ficam como estão** |

Esses dois itens (#24 e #25) são opcionais/recomendados — se o Codex preferir tratá-los apenas como Regra 1 (trocar só a palavra "fundamental" sem reformatar o `%s`), isso também é aceitável e mais conservador. Mas usar o mecanismo aqui é o que melhor demonstra o objetivo do pedido ("mecanismo para saber que rótulo usar para cada ano do fundamental").

### Regra 3 — contexto de escola/academia mista

Nenhuma ocorrência de "fundamental" no código atende de forma inequívoca ao padrão "citação de intervalo Fundamental→Médio" descrito na Regra 3 (ex.: um texto do tipo "esta academia atende do ensino fundamental ao médio"). As menções a "mista"/"misto" encontradas (`academia mista deve informar nivel fundamental ou medio`, `nivel_escolar 'fundamental' ou 'misto'`) são, na prática, **listagens do valor literal aceito pelo campo** (`nivel`/`nivel_escolar`) — por isso caem na regra "NÃO alterar" (seção 3, itens a/b). Ficam como estão.

Se o Codex encontrar, durante a implementação, alguma mensagem que descreva a abrangência da academia mista de forma narrativa (não como listagem de valor aceito), aplicar a Regra 3 ali (`RotuloEnsinoMisto = "Ensino Primário ao Médio"`).

---

## 5. O que **não** deve ser tocado (para evitar over-editing)

- Todo o diretório `/migrations` — nenhum arquivo `.sql`.
- Todo o diretório `/docs`.
- Nomes de campos JSON e de banco: `ano_escolar_fundamental`, `status_escolar_fundamental`, `certificado_6_ano_fundamental`, `certificado_9_ano_fundamental`, etc.
- Valores de enum comparados em lógica: `"fundamental"`, `"medio"`, `"superior"` (nos campos `nivel`, `type`, `tipo_ensino`, `NivelFundamental`).
- O formato `N_ano_fundamental` em si — inclusive nos exemplos dentro de mensagens de formato (`ex.: 1_ano_fundamental`).
- Nomes de função, tipo, evento e variável (`ValidateAnoFundamental`, `ValidateAnosFundamental`, `FundamentalRetomadoEvent`, `InterromperFundamental`, `isAnoFundamental`, `NivelFundamental`, etc.) — só o texto das *mensagens* que essas funções retornam muda, não os identificadores.
- Comentários de código que não são retornados ao cliente (ex.: `// AnosAcademicos define os anos do ensino fundamental que esta academia oferece.` em `academia.go:35`) — alteração opcional por consistência, mas fora do escopo pedido (mensagens da API).
- Consultas SQL (`type = 'fundamental'`, `nivel = 'fundamental'` etc.).

---

## 6. Testes a atualizar

Foi feita uma busca em todos os arquivos `_test.go` do repositório procurando por asserções literais sobre os textos que serão alterados. Resultado: **apenas um teste** faz asserção direta sobre um texto da lista acima:

- `internal/domain/aggregates/bilhetes_validation_test.go:172` — verifica `strings.Contains(err.Error(), "certificado do 6.º ano fundamental")`. Deve passar a verificar `strings.Contains(err.Error(), "certificado da 6ª Classe")` (correspondente ao item #22 da tabela).

Após aplicar todas as mudanças, **rodar novamente uma busca** por essas mesmas frases-alvo em `*_test.go` (o Codex pode ter introduzido regressões não previstas nesta análise, ou pode haver testes de integração que dependem de banco e não foram varridos por texto estático). Comando sugerido:

```bash
grep -rn "ensino fundamental\|ano fundamental\|anos fundamentais\|certificado do 6\|certificado do 9\|regra fundamental\|interromper fundamental" --include="*_test.go" .
```

Qualquer resultado remanescente deve ser ajustado para refletir o novo texto.

---

## 7. Nota sobre `Documentação da API.md` (fora de `/docs`, na raiz do repo)

Este arquivo **não está dentro de `/docs`** (que foi excluído do escopo), mas documenta exatamente os textos de resposta da API que estão sendo alterados aqui — ex.: linha 2239/2245 reproduzem a mensagem do item #16, linha 3714 menciona "ensino fundamental" na descrição do campo `ano_escolar_fundamental`. Recomenda-se que o Codex também atualize os trechos correspondentes deste arquivo para não deixar a documentação dessincronizada do comportamento real da API — mas isso é secundário; se preferir, deixe como tarefa separada e sinalize ao usuário ao final.

---

## 8. Passo a passo de execução (para o Codex)

1. Criar `internal/utils/terminologia_angola.go` com o conteúdo da seção 2.
2. Aplicar as alterações da seção 4 (Regra 1 e Regra 2), arquivo por arquivo, adicionando o import `"spuri/internal/utils"` onde ainda não existir (`internal/finance/matricula.go`, `internal/finance/mensalidade.go`).
3. Verificar se o import `utils` já não é usado com outro alias nos arquivos tocados, para evitar conflito de nome (ex.: `solicitacao_matricula.go`, que já importa `spuri/internal/utils` para `TipoDeclaracaoParaAno`/`NivelDoAnoAcademico`).
4. Aplicar (ou registrar como não aplicado, com justificativa) os itens recomendados #24 e #25.
5. Atualizar o teste indicado na seção 6.
6. Rodar:
   ```bash
   gofmt -l .
   go build ./...
   go vet ./...
   go test ./... 2>&1 | tee /tmp/test-output.txt
   ```
   Testes de integração que dependem de banco de dados/serviços externos podem falhar por falta de infraestrutura — nesse caso, isolar e reportar separadamente dos testes unitários que falharem por causa da mudança de texto.
7. Rodar a busca de verificação da seção 6 novamente.
8. (Opcional) Atualizar `Documentação da API.md` conforme seção 7.
9. Gerar um resumo final com: lista de arquivos alterados, diff resumido por arquivo, resultado de `go build`/`go vet`/`go test`, e qualquer item da seção 4 que não pôde ser aplicado exatamente como sugerido (com justificativa).

---

## 9. Checklist de aceitação

- [ ] Nenhum valor de `N_ano_fundamental` foi alterado em lógica, JSON, SQL ou comparação.
- [ ] Nenhum valor literal de enum (`fundamental`/`medio`/`superior` nos campos `nivel`/`type`/`tipo_ensino`) foi alterado.
- [ ] Todas as 23 ocorrências obrigatórias da seção 4 (itens 1–23) foram tratadas.
- [ ] Itens 24–25 foram aplicados ou explicitamente marcados como não aplicados (com motivo).
- [ ] `internal/utils/terminologia_angola.go` criado e usado nos pontos indicados.
- [ ] Teste `bilhetes_validation_test.go` atualizado.
- [ ] `go build ./...`, `go vet ./...` sem erros.
- [ ] `go test ./...` sem novas falhas relacionadas a texto (falhas de infraestrutura pré-existentes são aceitáveis, mas devem ser reportadas).
- [ ] Nenhum arquivo em `/migrations` ou `/docs` foi tocado.
