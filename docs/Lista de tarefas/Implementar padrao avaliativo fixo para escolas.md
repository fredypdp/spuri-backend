# Implementar padrão avaliativo fixo para escolas

## Contexto

O backend já possui um modelo configurável de categorias de nota e regras de avaliação final administrado pela academia. Esse modelo deve continuar existindo, porém deve ficar voltado e permitido apenas para o ensino superior.

Para escolas, o sistema precisa passar a ter um padrão avaliativo próprio, fixo e não manipulável pela academia. As escolas não poderão criar, editar, remover ou sobrescrever categorias de nota e regras de avaliação final escolares. Nem usuários administradores poderão alterar ou remover essas definições, porque elas representam o padrão oficial do fluxo escolar.

## Problema principal

Atualmente a configuração acadêmica permite que regras e categorias sejam tratadas como dados administráveis pela academia. Esse comportamento não atende ao fluxo escolar porque:

1. escolas precisam seguir categorias de nota padronizadas por ano acadêmico;
2. escolas precisam seguir regras finais fixas por ano acadêmico;
3. as notas escolares têm escalas diferentes conforme o nível;
4. o último ano técnico do médio (`4_ano_medio`) possui uma avaliação especial por `nota_pap`;
5. matérias dependentes não devem mais existir para ensino médio escolar, ficando exclusivas para ensino superior;
6. nenhum administrador deve conseguir editar ou remover as definições escolares padrão.

## Objetivo

Criar um modelo avaliativo escolar separado do modelo configurável existente, com categorias de nota, regras de avaliação final e validações de escala fixas para escolas.

O modelo antigo deve ser restringido ao ensino superior. O novo modelo deve ser usado para fundamental e médio, inclusive escolas mistas, sem permitir manipulação pela academia.

## Resultado esperado

Implementar um fluxo em que:

1. categorias de nota escolares sejam criadas/fornecidas automaticamente pelo sistema;
2. regras de avaliação final escolares sejam fixas e versionáveis pelo código/migration/seed controlado;
3. rotas administrativas não permitam criar, editar ou remover categorias/regras escolares;
4. o modelo configurável anterior fique disponível apenas para cursos do ensino superior;
5. o backend valide a escala de notas conforme o ano acadêmico;
6. a avaliação final escolar seja calculada automaticamente conforme as regras abaixo;
7. `4_ano_medio` técnico use apenas `nota_pap` como avaliação do ano letivo;
8. matérias dependentes continuem disponíveis apenas para superior e não sejam aplicadas ao médio escolar.

## Escopo obrigatório

### 1. Separar modelo avaliativo escolar do modelo configurável superior

Criar uma separação clara entre:

- **modelo escolar fixo**: fundamental e médio, não editável pela academia;
- **modelo superior configurável**: regras/categorias administráveis pela academia, permitido apenas para ensino superior.

Regras obrigatórias:

- academias/escolas não podem criar categorias escolares customizadas;
- academias/escolas não podem editar categorias escolares padrão;
- academias/escolas não podem remover categorias escolares padrão;
- admins também não podem editar/remover categorias ou regras escolares padrão;
- rotas e comandos existentes de configuração devem recusar uso com `fundamental` e `medio`, aceitando apenas `superior`;
- consultas devem expor as categorias/regras escolares aplicáveis quando o estudante/curso/academia for escolar, mas indicando que são `system`, `fixed`, `readonly` ou equivalente;
- qualquer tabela/projeção nova deve preservar auditoria e permitir versionamento futuro sem tornar a regra editável pela academia.

### 2. Criar categorias de nota escolares fixas

Criar as seguintes categorias de nota para escolas.

#### 2.1. `1-5_ano_fundamental`, `7-8_ano_fundamental` e `1-2_ano_medio`

Categorias obrigatórias:

| Código | Nome público |
| --- | --- |
| `nota_professor` | `Nota do professor/Avaliação contínua` |
| `prova_trimestral` | `Prova trimestral` |

#### 2.2. `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`

Categorias obrigatórias:

| Código | Nome público |
| --- | --- |
| `nota_professor` | `Nota do professor/Avaliação contínua` |
| `prova_trimestral` | `Prova trimestral` |
| `exame_final` | `Exame final` |
| `exame_recurso` | `Exame de recurso` |

> Observação: usar o código correto `9_ano_fundamental` em toda a implementação e em dados/seeds novos.

#### 2.3. `4_ano_medio` técnico

Para o último ano de um curso do médio do modelo `tecnico`, criar apenas:

| Código | Nome público |
| --- | --- |
| `nota_pap` | `Prova de Aptidão Profissional` |

Esse ano não deve exigir aulas, trimestres ou provas convencionais. A avaliação do ano letivo deve ocorrer pela defesa/trabalho final representado por `nota_pap`.

### 3. Criar regras de avaliação final escolares fixas

Todas as regras devem ser aplicadas por matéria do estudante no ano letivo/ano acadêmico corrente, exceto a regra especial do `4_ano_medio` técnico quando o domínio não tratar a PAP como matéria convencional.

#### 3.1. `1-5_ano_fundamental`: `avaliacao_final` — `Avaliação final`

Regra:

1. Para cada matéria do estudante no ano, calcular a média de cada trimestre:
   - `(nota_professor do 1_trimestre + prova_trimestral do 1_trimestre) / 2`
   - `(nota_professor do 2_trimestre + prova_trimestral do 2_trimestre) / 2`
   - `(nota_professor do 3_trimestre + prova_trimestral do 3_trimestre) / 2`
2. Calcular `nota_final`:
   - `nota_final = (media_1_trimestre + media_2_trimestre + media_3_trimestre) / 3`
3. Para aprovar na matéria, `nota_final >= 5`.
4. Se o estudante tiver uma matéria abaixo da mínima, deve ser considerado reprovado conforme a política escolar aplicável.

#### 3.2. `7-8_ano_fundamental` e `1-2_ano_medio`: `avaliacao_final` — `Avaliação final`

Regra:

1. Para cada matéria do estudante no ano, calcular a média de cada trimestre:
   - `(nota_professor do 1_trimestre + prova_trimestral do 1_trimestre) / 2`
   - `(nota_professor do 2_trimestre + prova_trimestral do 2_trimestre) / 2`
   - `(nota_professor do 3_trimestre + prova_trimestral do 3_trimestre) / 2`
2. Calcular `nota_final`:
   - `nota_final = (media_1_trimestre + media_2_trimestre + media_3_trimestre) / 3`
3. Para aprovar na matéria, `nota_final >= 10`.
4. Se o estudante tiver uma matéria abaixo da mínima, deve ser considerado reprovado conforme a política escolar aplicável.

#### 3.3. `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`: `avaliacao_final` — `Avaliação final`

Regra raiz:

1. Para cada matéria do estudante no ano, calcular as médias trimestrais:
   - `1_trimestre`: `(nota_professor + prova_trimestral) / 2`
   - `2_trimestre`: `(nota_professor + prova_trimestral) / 2`
   - `3_trimestre`: `(nota_professor + exame_final) / 2`
2. Calcular `nota_final`:
   - `nota_final = (media_1_trimestre + media_2_trimestre + media_3_trimestre) / 3`
3. Mínimos de aprovação:
   - `6_ano_fundamental`: `nota_final >= 5`;
   - `9_ano_fundamental`: `nota_final >= 10`;
   - `3_ano_medio`: `nota_final >= 10`.

#### 3.4. `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`: `exame_recurso` — `Exame de recurso`

A regra `exame_recurso` depende de reprovação prévia em `avaliacao_final`.

Regras:

1. O estudante só pode realizar `exame_recurso` nas matérias em que reprovou na `avaliacao_final` do mesmo ano letivo.
2. `6_ano_fundamental`:
   - para cada matéria reprovada com `nota_final < 5`, quando a nota `exame_recurso` for lançada, ela deve ser `>= 5`;
   - se houver uma única matéria com `exame_recurso < 5`, o estudante reprova.
3. `9_ano_fundamental` e `3_ano_medio`:
   - para cada matéria reprovada com `nota_final < 10`, quando a nota `exame_recurso` for lançada, ela deve ser `>= 10`;
   - se houver uma única matéria com `exame_recurso < 10`, o estudante reprova.
4. Não permitir `exame_recurso` para matéria já aprovada na `avaliacao_final`.
5. Não permitir `exame_recurso` sem avaliação final anterior reprovada.
6. Se todas as matérias reprovadas forem aprovadas no recurso, o estudante deve ser aprovado no ano conforme as regras de progressão existentes.

#### 3.5. `4_ano_medio` técnico: `avaliacao_final` — `Prova de Aptidão Profissional`

Regra especial para o último ano de curso médio do modelo `tecnico`:

1. Aplica-se somente ao `4_ano_medio` de cursos médios com `modelo = tecnico`.
2. O ano letivo deve ter apenas a categoria `nota_pap`.
3. A regra de aprovação é:
   - `nota_pap >= 10`.
4. Se aprovado, o estudante finaliza o curso e o ensino médio.
5. Não exigir notas trimestrais, `nota_professor`, `prova_trimestral`, `exame_final` ou `exame_recurso` nesse ano.


### 4. Definir gatilhos da avaliação final automática

A avaliação final escolar não deve depender de uma rota manual para ser aplicada. O cálculo deve ser despertado automaticamente pelo lançamento das notas que encerram cada contexto avaliativo.

Gatilhos obrigatórios:

1. **Avaliação final regular com `prova_trimestral` no 3º trimestre**
   - para `1-5_ano_fundamental`, `7-8_ano_fundamental` e `1-2_ano_medio`, o lançamento da nota `prova_trimestral` do `3_trimestre` deve despertar a tentativa de cálculo da `avaliacao_final`;
   - o backend deve verificar se todas as notas obrigatórias da matéria/estudante/ano letivo já existem antes de calcular;
   - se ainda faltarem notas de outras matérias ou categorias obrigatórias, o sistema não deve reprovar/aprovar parcialmente de forma definitiva, apenas deve manter a avaliação pendente até que os dados necessários estejam completos.

2. **Avaliação final regular com `exame_final`**
   - para `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`, o lançamento da nota `exame_final` deve despertar a tentativa de cálculo da `avaliacao_final`;
   - esse gatilho substitui o uso de `prova_trimestral` no `3_trimestre` para esses anos, porque a média do terceiro trimestre deve usar `nota_professor + exame_final`;
   - o backend deve validar que as notas dos trimestres anteriores e a `nota_professor` do `3_trimestre` estão disponíveis antes de consolidar a avaliação.

3. **Avaliação por `exame_recurso`**
   - para `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`, o lançamento da nota `exame_recurso` deve despertar a tentativa de cálculo da regra `exame_recurso`;
   - esse cálculo só pode ocorrer para matérias reprovadas na `avaliacao_final` anterior;
   - se o estudante tiver mais de uma matéria em recurso, a decisão final do recurso só deve ser consolidada quando todas as notas `exame_recurso` exigidas tiverem sido lançadas;
   - uma única matéria abaixo da mínima no recurso reprova o estudante.

4. **PAP do `4_ano_medio` técnico**
   - para o `4_ano_medio` de cursos médios do modelo `tecnico`, o lançamento da nota `nota_pap` deve despertar a avaliação final especial do ano;
   - a avaliação deve ser consolidada somente com base em `nota_pap >= 10`.

Orientação de implementação:

- centralizar a lógica em um serviço/handler idempotente chamado após persistir uma nota;
- o handler deve receber o contexto da nota lançada (`estudante`, `academia`, `ano_letivo`, `ano_academico`, `materia`, `periodo`, `categoria`) e decidir se aquela categoria/período é um gatilho válido;
- se a nota lançada não for um gatilho, o handler deve encerrar sem efeito colateral;
- se for gatilho, o handler deve carregar a regra escolar fixa aplicável e verificar completude de todas as notas exigidas antes de emitir evento/projeção de avaliação final;
- a operação precisa ser idempotente para evitar duplicidade quando uma nota for relançada, editada ou processada novamente por job/rebuild;
- a unicidade da avaliação final deve considerar estudante, academia, ano letivo, ano acadêmico, tipo/regra de avaliação e, quando aplicável, matéria;
- o cálculo deve salvar snapshot da regra/gatilho usado para auditoria;
- erros de nota incompleta devem ser tratados como estado pendente, não como falha técnica.

### 5. Validar escala de notas por nível

Implementar proteção no sistema para impedir lançamento de notas fora da escala do ano acadêmico.

Escalas obrigatórias:

| Anos acadêmicos | Escala |
| --- | --- |
| `[1-6]_ano_fundamental` | `0` a `10` |
| `[7-9]_ano_fundamental` | `0` a `20` |
| `[1-4]_ano_medio` | `0` a `20` |
| todos os níveis de qualquer curso do superior | `0` a `20` |

Requisitos:

- aceitar notas inteiras e decimais dentro da escala permitida, por exemplo `8.5` em escala `0-10` e `15.5` em escala `0-20`;
- validar no comando/rota de lançamento de nota;
- validar também em qualquer importação, job, handler ou fluxo interno que grave notas;
- retornar erro claro quando a nota estiver fora da escala;
- cobrir limites inclusivos (`0`, `10`, `20`) e valores decimais válidos em testes;
- garantir que `6_ano_fundamental` use escala `0-10`, embora tenha regras com `exame_final` e `exame_recurso`.

### 6. Remover matérias dependentes do ensino médio escolar

A partir desta implementação:

- matérias dependentes não devem ser aplicadas ao ensino médio escolar;
- matérias dependentes devem ficar exclusivas para ensino superior;
- qualquer configuração, cálculo ou projeção que aplique dependência ao médio deve ser removida, desativada ou protegida por validação de tipo de ensino;
- se existirem dados legados de dependência para médio, criar migração/backfill ou plano de compatibilidade para ignorá-los sem quebrar consultas.

### 7. Ajustar APIs e permissões

Revisar rotas/endpoints/comandos de categorias de nota e regras de avaliação final para garantir que:

1. operações de criação/edição/remoção sejam permitidas apenas para ensino superior;
2. operações escolares retornem erro de domínio quando tentarem manipular configurações fixas;
3. consultas escolares retornem as categorias e regras padrão aplicáveis;
4. respostas indiquem que a origem é sistêmica/fixa e não editável;
5. admins não tenham bypass para editar/remover definições escolares;
6. mensagens de erro sejam claras para frontend e integrações.

### 8. Migrações, seeds e idempotência

Se o modelo fixo for persistido em banco/projeção, criar migrations/seeds idempotentes para:

- inserir categorias escolares padrão;
- inserir regras escolares padrão;
- marcar registros como sistêmicos/fixos/read-only;
- impedir edição/remoção por comandos de aplicação;
- evitar duplicidade em reexecuções;
- preservar compatibilidade com rebuild de projeções/event sourcing.

Se as regras forem definidas em código, garantir que:

- sejam versionadas;
- tenham testes cobrindo lookup por ano acadêmico/modelo;
- possam ser auditadas na avaliação final por snapshot da regra usada;
- não dependam de configuração mutável da academia.

## Critérios de aceite

- Categorias escolares padrão existem para todos os anos listados.
- Regras escolares finais calculam aprovação/reprovação de forma automática e determinística.
- `1-5_ano_fundamental` aprova com mínimo `5`.
- `7-8_ano_fundamental`, `1-2_ano_medio`, `9_ano_fundamental` e `3_ano_medio` aprovam com mínimo `10` quando aplicável.
- `6_ano_fundamental` usa escala `0-10` e aprova com mínimo `5`, inclusive no recurso.
- `4_ano_medio` técnico usa somente `nota_pap >= 10`.
- `exame_recurso` só é permitido após reprovação em `avaliacao_final` e apenas nas matérias reprovadas.
- Uma única matéria abaixo da mínima no recurso reprova o estudante.
- Notas fora da escala do ano acadêmico são rejeitadas.
- Matérias dependentes não são aplicadas ao ensino médio escolar.
- Modelo configurável antigo fica restrito ao ensino superior.
- Nenhum administrador consegue editar/remover categorias ou regras escolares fixas.
- Testes automatizados cobrem categorias, regras, escalas, permissões, gatilhos automáticos e casos de reprovação/aprovação.

## Sugestões de testes

- Teste unitário de lookup das categorias fixas por ano acadêmico.
- Teste unitário de lookup da regra final por ano acadêmico e modelo do curso médio.
- Teste de cálculo para `1-5_ano_fundamental` com `nota_final = 5` aprovando e `4.99` reprovando.
- Teste de cálculo para `7-8_ano_fundamental` e `1-2_ano_medio` com mínimo `10`.
- Teste de cálculo para `6_ano_fundamental` usando `exame_final` no terceiro trimestre e mínimo `5`.
- Teste de cálculo para `9_ano_fundamental` e `3_ano_medio` usando `exame_final` no terceiro trimestre e mínimo `10`.
- Teste de `exame_recurso` aprovando quando todas as matérias reprovadas atingem a mínima.
- Teste de `exame_recurso` reprovando quando uma matéria fica abaixo da mínima.
- Teste bloqueando `exame_recurso` antes de reprovação em `avaliacao_final`.
- Teste bloqueando `exame_recurso` para matéria aprovada.
- Teste de `4_ano_medio` técnico aceitando apenas `nota_pap` e aprovando com `>= 10`.
- Teste garantindo que `prova_trimestral` do `3_trimestre` desperta `avaliacao_final` nos anos que usam prova trimestral regular.
- Teste garantindo que `exame_final` desperta `avaliacao_final` em `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`.
- Teste garantindo que `exame_recurso` desperta apenas a regra de recurso, somente após reprovação anterior e com todas as notas de recurso obrigatórias lançadas.
- Teste garantindo que notas que não são gatilhos não geram avaliação final nem efeitos colaterais.
- Testes de escala `0-10` para `[1-6]_ano_fundamental`, incluindo decimal válido como `8.5`.
- Testes de escala `0-20` para `[7-9]_ano_fundamental`, `[1-4]_ano_medio` e superior, incluindo decimal válido como `15.5`.
- Testes de permissão garantindo que escola/admin não edita nem remove categorias/regras escolares.
- Teste garantindo que configuração de regras/categorias customizadas só funciona para superior.
