# Manual de Configuração Inicial da Academia

Este manual orienta a academia recém-criada e ativada a configurar o ambiente acadêmico em **ordem cronológica**, do que tem menos dependências até os fluxos operacionais. Ele foi revisado contra as rotas registradas no backend e contra a documentação pública da API.

> Objetivo: ao final deste fluxo, a academia estará pronta para cadastrar/aprovar estudantes, organizar turmas, lançar notas e faltas e deixar a avaliação final automática funcionar corretamente: em escolas, pelo padrão fixo do sistema; no Superior, pelas categorias e regras configuradas pela própria academia.

---

## 1. Visão geral da ordem recomendada

Antes dos passos operacionais, confirme o tipo da academia ativada conforme a seção 2. Depois siga esta sequência para evitar erros de dependência:

1. **Confirmar academia ativa e tipo institucional**.
2. **Definir o primeiro ano letivo ativo da academia**.
3. **Definir anos acadêmicos do Fundamental**, quando a academia for escola `fundamental` ou `misto`.
4. **Criar cursos médios ou superiores**, quando aplicável.
5. **Conferir os anos/períodos derivados dos cursos**.
6. **Criar matérias disciplinares**.
7. **Ativar matérias superiores**, pois elas nascem inativas.
8. **Criar categorias de nota superiores**, somente quando a academia ofertar Superior; escolas usam catálogo fixo.
9. **Criar regras de avaliação final superiores**, somente quando a academia ofertar Superior; escolas usam regras fixas.
10. **Cadastrar estudantes ou aprovar solicitações de matrícula**.
11. **Criar turmas**.
12. **Adicionar estudantes às turmas**.
13. **Iniciar a operação acadêmica**: notas, faltas, acompanhamentos, finalização de ano letivo e demais funcionalidades.


### 1.1 Roteiro por tipo de academia

Use este roteiro como mapa rápido antes de seguir as seções detalhadas. Ele mostra o que cada tipo de academia deve configurar e o que deve ignorar.

- **Escola**
  - **Ensino Fundamental (`nivel_escolar="fundamental"`)**
    1. Defina o ano letivo ativo da academia.
    2. Cadastre os anos fundamentais ofertados em `POST /academia/anos-academicos`.
    3. Crie matérias fundamentais para os anos cadastrados.
    4. Não crie cursos, categorias de nota nem regras de avaliação final: categorias e regras escolares são fixas do sistema.
    5. Cadastre/aprove estudantes, crie turmas fundamentais e vincule os estudantes às turmas.
  - **Ensino Médio (`nivel_escolar="medio"`)**
    1. Defina o ano letivo ativo da academia.
    2. Crie cada curso médio em `POST /academia/curso`, informando `modelo="liceu"` ou `modelo="tecnico"`.
    3. Confira os anos médios derivados do modelo do curso.
    4. Crie matérias médias vinculadas ao curso e ao ano médio correto.
    5. Não use `/academia/anos-academicos` para Médio e não crie categorias/regras escolares por endpoint: o sistema usa catálogo e avaliação final fixos.
    6. Cadastre/aprove estudantes, crie turmas médias com `curso_id` e vincule os estudantes às turmas.
  - **Escola Mista (`nivel_escolar="misto"`)**
    1. Defina o ano letivo ativo da academia.
    2. Configure os anos fundamentais na academia.
    3. Crie os cursos médios com `modelo` para gerar os anos médios.
    4. Crie matérias fundamentais e médias nos escopos corretos.
    5. Não crie categorias/regras escolares por endpoint; Fundamental e Médio usam o padrão fixo do sistema.
    6. Cadastre/aprove estudantes, crie turmas fundamentais/médias e vincule cada estudante à turma compatível.
- **Ensino Superior (`nivel="superior"`)**
  1. Defina o ano letivo ativo da academia.
  2. Crie cada curso superior em `POST /academia/curso`, informando `periodos` como quantidade total de semestres.
  3. Confira semestres e anos superiores derivados do curso.
  4. Crie matérias superiores com `curso_id`, ano superior e `periodo`; depois ative cada matéria superior.
  5. Crie categorias de nota superiores.
  6. Crie as regras de avaliação final superiores com fórmula textual e, quando quiser disparo automático da raiz, `nota_despertadora`.
  7. Cadastre/aprove estudantes, crie turmas superiores com `curso_id` e vincule os estudantes às turmas.

---

## 2. Antes de começar

Este manual deve ser seguido depois que a academia já estiver criada e ativada. A partir desse ponto, confira o tipo da instituição, porque ele determina quais configurações serão permitidas:

| Academia | Configurações curriculares esperadas |
|---|---|
| Escola `fundamental` | Anos acadêmicos fundamentais ficam na própria academia. |
| Escola `medio` | Anos acadêmicos médios ficam nos cursos médios e são derivados automaticamente do `modelo`. |
| Escola `misto` | Fundamental fica na academia; Médio fica nos cursos médios. |
| `superior` | Semestres e anos superiores são derivados dos cursos superiores. |

Todas as rotas operacionais de academia exigem autenticação de academia ativa. Algumas rotas de leitura também aceitam admin ou estudante autenticado, conforme o caso, mas a escrita inicial descrita neste manual é feita pela própria academia.

---

## 3. Passo 1 — Definir o ano letivo da academia

A academia define o seu primeiro ano letivo ativo pela rota:

```http
POST /academia/definir-ano-letivo
```

Regras principais:

- A academia só define diretamente o ano letivo quando ainda não tem ano letivo ativo.
- O backend infere o tipo do ano letivo a partir da academia: `escolar` para academia de escola e `superior` para academia superior.
- O campo `ano_letivo` é opcional; quando omitido, o backend usa o ano letivo global atual definido pelo admin para o tipo da academia. Não envie `periodo`: ele é fixo, imutável e derivado pelo backend (`escolar -> 09_07`, `superior -> 10_07`).
- Se `ano_letivo` for enviado, ele precisa ser igual ao ano letivo global atual.
- Depois disso, a passagem para o próximo ano acontece pela finalização do ano letivo, não por redefinição manual.
- Sem ano letivo ativo, o sistema bloqueia notas, faltas e avaliações finais.

Exemplo conceitual:

```json
{
  "ano_letivo": "2026_2027"
}
```

Também é válido enviar `{}` quando a academia deve assumir o ano letivo global atual. A resposta expõe `periodo` apenas como valor derivado/read-only; escolas retornam `09_07` e ensino superior retorna `10_07`.

**Por que este passo vem antes dos demais processos operacionais?**

Porque notas, faltas e avaliações finais sempre são registradas no contexto do ano letivo ativo da academia. A configuração curricular pode ser preparada antes do primeiro lançamento, mas a operação acadêmica não deve começar sem este passo.

---

## 4. Passo 2 — Configurar anos acadêmicos do Fundamental, se aplicável

Este passo se aplica apenas a escolas com `nivel_escolar`:

- `fundamental`
- `misto`

Escolas exclusivamente médias não configuram anos fundamentais na academia. Instituições superiores também não usam esta rota para anos superiores.

### 4.1 Consultar a situação atual

```http
GET /academia/anos-academicos
```

A resposta traz:

- `academia.anos_academicos`: anos fundamentais armazenados na academia;
- `cursos`: cursos médios/superiores da academia, com seus anos/períodos derivados.

Quando o usuário autenticado for admin, a consulta exige `?codigo_academia=...`. Quando for a própria academia, o backend usa a academia do token.

### 4.2 Adicionar anos fundamentais

```http
POST /academia/anos-academicos
```

Exemplo:

```json
{
  "type": "fundamental",
  "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental", "3_ano_fundamental"]
}
```

A operação **adiciona/une** os anos enviados aos já existentes; não existe substituição em massa. Use somente anos no formato:

```text
[n]_ano_fundamental
```

com `n` de 1 a 9.

### 4.3 Remover anos fundamentais da oferta futura

```http
DELETE /academia/anos-academicos
```

Exemplo:

```json
{
  "type": "fundamental",
  "anos_academicos": ["4_ano_fundamental"]
}
```

A remoção é lógica/prospectiva: o histórico já registrado não é apagado. A operação é bloqueada quando deixaria a academia sem nenhum ano fundamental ativo ou quando houver estudantes ativos vinculados ao ano que se pretende remover.

**Importante:** `PATCH /academia/anos-academicos` não existe no contrato público. Também não envie `codigo_academia`, `substituir`, `replace`, `patch`, `set` ou `update` no payload.

---

## 5. Passo 3 — Criar cursos médios ou superiores, se aplicável

Cursos são necessários para:

- escolas médias;
- escolas mistas que ofertam médio;
- instituições superiores.

Rota de escrita registrada para criação:

```http
POST /academia/curso
```

> Atenção: a criação/edição/ativação/desativação/deleção usa o caminho singular `/academia/curso...`. Para consulta, o backend expõe `GET /academia/cursos` e `GET /academia/curso/:id`.

O tipo efetivo do curso é inferido a partir da academia autenticada. O campo `type` pode ser enviado para explicitar a intenção, mas precisa corresponder ao tipo permitido para a academia.

### 5.1 Curso médio

Um curso médio deve informar `modelo` com valor exatamente `liceu` ou `tecnico`. O backend deriva automaticamente os anos acadêmicos:

- `liceu`: `1_ano_medio`, `2_ano_medio`, `3_ano_medio`;
- `tecnico`: `1_ano_medio`, `2_ano_medio`, `3_ano_medio`, `4_ano_medio`.

Exemplo:

```json
{
  "nome": "Ciências Físicas e Biológicas",
  "type": "medio",
  "modelo": "liceu"
}
```


### 5.2 Curso superior

Um curso superior recebe a quantidade total de semestres em `periodos`, como número inteiro positivo. O backend deriva automaticamente:

- os semestres: `1_semestre`, `2_semestre`, ..., `N_semestre`;
- os anos acadêmicos superiores: `1_ano_superior`, `2_ano_superior`, etc., calculados a partir dos semestres.

Exemplo:

```json
{
  "nome": "Engenharia Informática",
  "type": "superior",
  "periodos": 8
}
```

Não envie `modelo` nem `anos_academicos` para cursos superiores. Cursos recém-criados nascem ativos; as rotas de ativação/desativação são usadas apenas em manutenção posterior.

**Dependências:** cursos devem existir antes de matérias médias/superiores, turmas médias/superiores, estudantes médios/superiores, categorias superiores e regras superiores. Cursos médios não exigem criação de regras finais pela academia.

---

## 6. Passo 4 — Conferir anos e períodos derivados dos cursos

Após criar os cursos, consulte:

```http
GET /academia/anos-academicos
GET /academia/cursos
GET /academia/curso/:id
```

Use essas respostas para confirmar os escopos disponíveis antes de criar matérias, turmas e estudantes.

Regras atuais:

- Cursos médios não aceitam adição ou remoção manual de anos; os anos são fixos por `modelo`.
- `POST` ou `DELETE /academia/anos-academicos` com `type="medio"` retorna erro, porque o Médio é derivado do curso.
- Cursos superiores não aceitam adição/remoção direta de anos acadêmicos, períodos ou semestres por `/academia/anos-academicos` nem por `PUT /academia/curso/:id/dados`.

---

## 7. Passo 5 — Criar matérias disciplinares

As matérias dependem dos anos acadêmicos e, em Médio/Superior, também dependem do curso.

Rota de escrita registrada para criação:

```http
POST /academia/materia
```

> Atenção: a criação/edição/ativação/desativação/deleção usa o caminho singular `/academia/materia...`. Para consulta, o backend expõe `GET /academia/materias` e `GET /academia/materia/:id`.

### 7.1 Matéria fundamental

Requer anos fundamentais já configurados na academia.

Exemplo:

```json
{
  "nome": "Matemática",
  "type": "fundamental",
  "anos_academicos": ["6_ano_fundamental"]
}
```

Matérias fundamentais nascem ativas. Para Fundamental, `anos_academicos` aceita de 1 a 9 itens válidos. Payloads escolares não aceitam `pendencia_permitida` nem `pendencia_nivel_conclusao`.

### 7.2 Matéria média

Requer curso médio já criado e exatamente um ano acadêmico médio.

Exemplo:

```json
{
  "nome": "Biologia",
  "type": "medio",
  "curso_id": "uuid-do-curso-medio",
  "anos_academicos": ["1_ano_medio"]
}
```

Matérias médias nascem ativas. Matérias dependentes/pendências não são permitidas no Médio escolar; `pendencia_permitida` e `pendencia_nivel_conclusao` são exclusivos do Superior e são rejeitados em payloads escolares.

### 7.3 Matéria superior

Requer curso superior já criado, exatamente um ano acadêmico superior compatível e o semestre da matéria.

Exemplo:

```json
{
  "nome": "Algoritmos",
  "type": "superior",
  "curso_id": "uuid-do-curso-superior",
  "anos_academicos": ["1_ano_superior"],
  "periodo": "1_semestre",
  "pendencia_permitida": true,
  "pendencia_nivel_conclusao": "4_semestre"
}
```

Matérias superiores nascem ativas por padrão. O campo `periodo` é obrigatório na criação e não pode ser editado depois por `PUT /academia/materia/:id/dados`. O request superior mantém `pendencia_permitida`; quando ele não for enviado, o backend infere e persiste `pendencia_permitida=true`. `pendencia_nivel_conclusao` também é exclusivo do Superior e deve apontar para um semestre válido do curso.

---

## 8. Passo 6 — Revisar status das matérias superiores

Matérias superiores novas já são criadas ativas por padrão, portanto não existe etapa obrigatória de ativação após o cadastro. A rota abaixo permanece para reativar matérias superiores que tenham sido desativadas explicitamente:

```http
PUT /academia/materia/:id/ativar
```

Sem ativação, a matéria superior não será considerada ativa para os processos acadêmicos. Matérias superiores sem `periodo` definido não podem ser ativadas.

---

## 9. Observação — Não existe configuração de matérias especiais no curso

O modelo atual não possui etapa, rota ou payload para configurar listas especiais de disciplinas no curso. Em especial, não use campos como `materias_chave`, `disciplinas_chave`, `materias_aplicaveis` ou equivalentes ao criar/editar cursos médios: as disciplinas são cadastradas pela rota de matérias e a avaliação final escolar usa o catálogo fixo do sistema. Depois de criar cursos e matérias disciplinares, siga diretamente para categorias/regras de avaliação, matrículas, notas e faltas.

## 10. Passo 8 — Categorias de nota

Este passo só exige ação de academias com oferta Superior. Escolas devem apenas consultar o catálogo fixo se quiserem validar o que será exibido e aceito nos lançamentos.

As categorias de nota seguem dois modelos:

- **Escolas (Fundamental/Médio):** não criam categorias por endpoint. O backend fornece automaticamente o catálogo fixo do sistema, marcado na listagem com campos como `source="system"`, `fixed=true` e `readonly=true`. Para Médio, a listagem deriva os anos dos cursos médios ativos; `academia.anos_academicos` continua sendo usado para Fundamental.
- **Superior:** cria categorias explicitamente antes de lançar notas e antes de configurar fórmulas de avaliação final.

### 10.1 Criar categorias superiores

Rota exclusiva do Superior:

```http
POST /academia/categorias-nota
```

Exemplo superior:

```json
{
  "codigo": "prova_parcelar_1",
  "nome": "Prova Parcelar 1",
  "descricao": "Primeira prova parcelar",
  "anos_academicos": ["1_ano_superior", "2_ano_superior"]
}
```

O `codigo` é normalizado: letras maiúsculas viram minúsculas, espaços internos viram `_`, e caracteres especiais diferentes de `_` são rejeitados.

### 10.2 Consultar categorias

```http
GET /academia/categorias-nota
```

Catálogo escolar fixo:

| Anos acadêmicos | Categorias |
|---|---|
| `1_ano_fundamental` a `5_ano_fundamental`, `7_ano_fundamental`, `8_ano_fundamental`, `1_ano_medio`, `2_ano_medio` | `nota_professor`, `prova_trimestral` |
| `6_ano_fundamental`, `9_ano_fundamental`, `3_ano_medio` | `nota_professor`, `prova_trimestral`, `exame_final`, `exame_recurso` |
| `4_ano_medio` de curso técnico (`modelo="tecnico"`) | `nota_pap` |

Regras importantes:

- Escolas não podem criar nem remover categorias escolares; tentativas em `POST /academia/categorias-nota` ou `DELETE /academia/categorias-nota/:codigo` falham.
- Categorias escolares legadas eventualmente existentes na projeção não devem orientar lançamentos ou avaliação final: o backend valida notas escolares contra o catálogo fixo aplicável ao ano/curso.
- Superior continua usando categorias configuráveis pela academia.
- A categoria superior precisa incluir os anos acadêmicos nos quais poderá receber notas.
- O lançamento de nota valida a escala do ano: `0–10` para `1_ano_fundamental` a `6_ano_fundamental`; `0–20` para `7_ano_fundamental` a `9_ano_fundamental`, Médio e Superior.

---

## 11. Passo 9 — Regras de avaliação final superior

Este passo só exige ação de academias com oferta Superior. Escolas (`fundamental`/`medio`) não criam, editam ou removem regras de avaliação final; o backend fornece as regras fixas do sistema e bloqueia tentativas de configuração escolar por endpoint.

### 11.1 Fundamental e Médio escolar

Não crie regras por endpoint para escolas. `POST`, `PUT` e `DELETE` de regras escolares são bloqueados; a listagem e a execução automática devem ser interpretadas como catálogo oficial do sistema. O backend aplica automaticamente o padrão fixo:

- anos regulares usam média dos três trimestres com `nota_professor` e `prova_trimestral`;
- `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio` usam `exame_final` no 3º trimestre e permitem `exame_recurso` apenas para matérias reprovadas;
- `4_ano_medio` técnico usa somente `nota_pap >= 10`;
- matérias dependentes/pendências não existem no Médio escolar;
- se a categoria lançada não for gatilho oficial da etapa, nenhuma regra configurável/legada é usada como fallback.

### 11.2 Superior

Rota de criação:

```http
POST /academia/avaliacao-final/regras
```

Configuração típica:

- `nivel="superior"`;
- sem `anos_academicos` na regra;
- fórmula textual no modelo atual, usando categorias entre colchetes;
- em Superior, a fórmula pode referenciar apenas `[categoria]`, pois o semestre é inferido pela matéria avaliada;
- `limite_materias_pendentes` obrigatório;
- `nota_despertadora` opcional apenas em regra raiz; sem ela, a raiz não dispara automaticamente por lançamento de nota;
- pendências (`pendencia_permitida` e `pendencia_nivel_conclusao`) são exclusivas de matérias superiores.

Exemplo:

```json
{
  "type": "avaliacao_final",
  "nome": "Avaliação final superior",
  "descricao": "Média das avaliações do semestre",
  "nivel": "superior",
  "nota_minima_aprovacao": 10,
  "formula": "([prova_parcelar_1]+[prova_parcelar_2])/2",
  "limite_materias_pendentes": 2,
  "nota_despertadora": "prova_parcelar_2",
  "aplica_se_reprovado_em_type": null
}
```

Ordem dentro da cadeia superior:

1. Criar regra raiz, por exemplo `avaliacao_final`.
2. Criar regra descendente que aponta para a raiz, por exemplo `avaliacao_final_com_exame` com `aplica_se_reprovado_em_type="avaliacao_final"`.
3. Criar novas descendentes, se houver, sempre apontando para uma etapa anterior ativa e sem criar ciclos.

Regras importantes:

- O formato antigo de fórmula em JSON foi removido; use somente fórmula textual.
- O backend extrai `categorias_envolvidas` a partir da fórmula. Se o campo for enviado, precisa bater exatamente com as categorias extraídas.
- `materias_aplicaveis`, quando usado no Superior, segue itens `{curso_id, ano_academico, materias}`.
- `PUT /academia/avaliacao-final/regras/:id` edita apenas campos seguros como `nome`, `descricao`, `nota_minima_aprovacao`, `formula` e, em raiz, `nota_despertadora`.
- `DELETE /academia/avaliacao-final/regras/:id` é deleção lógica e inativa também descendentes da cadeia.

---

## 12. Passo 10 — Cadastrar ou aprovar estudantes

Depois que a estrutura curricular básica estiver pronta, cadastre os estudantes pela academia ou aprove solicitações de matrícula.

Cadastro direto:

```http
POST /academia/estudante/register
```

O cadastro direto usa `multipart/form-data`. Os documentos são opcionais; quando enviados, precisam ser PDFs válidos, com extensão `.pdf`, assinatura `%PDF` e tamanho máximo de 10MB.

Quando documentos forem retornados nos metadados do estudante ou da solicitação de matrícula, o front end deve usar o campo `download_url` para leitura do PDF. O backend expõe rotas autenticadas de download inline (`GET /documentos/estudantes/{codigo_estudante}/{campo}/download` e `GET /documentos/solicitacoes-matricula/{codigo_solicitacao}/{campo}/download`), sem expor credenciais ou IDs internos do Mega.

Ao cadastrar, informe os vínculos acadêmicos compatíveis com o tipo de estudante:

| Estudante | Campos acadêmicos esperados |
|---|---|
| Fundamental | `ano_escolar_fundamental` compatível com os anos fundamentais da academia. |
| Médio | `curso_medio_id` e `ano_escolar_medio` compatíveis com o curso médio. |
| Superior | `curso_superior_id`; o backend inicia o vínculo superior no começo do curso conforme as regras do domínio. |

Para estudantes escolares, o BI do responsável também é parte das validações de cadastro. Após a criação, o vínculo com a academia nasce ativo; alterações posteriores de status devem usar os endpoints específicos de matrícula, interrupção, trancamento, desvinculação ou revinculação.

**Por que estudantes vêm depois da estrutura curricular?**

Porque o cadastro acadêmico do estudante precisa apontar para anos e cursos existentes. Além disso, notas, faltas, turmas e avaliações dependem do estudante estar vinculado corretamente à academia.

---

## 13. Passo 11 — Criar turmas

Turmas dependem do nível acadêmico e, para Médio/Superior, normalmente do curso.

Rota de escrita registrada para criação:

```http
POST /academia/turma
```

> Atenção: a criação/edição/ativação/desativação/deleção e vínculos de estudantes usam o caminho singular `/academia/turma...`. Para consulta, o backend expõe `GET /academia/turmas` e `GET /academia/turma/:codigo`.

O campo `codigo_turma` é normalizado: espaços antes/depois são descartados, espaços internos viram `_`, e caracteres especiais diferentes de `_` são rejeitados.

Exemplo fundamental:

```json
{
  "codigo_turma": "6A",
  "nivel": "6_ano_fundamental",
  "turno": "manha"
}
```

Exemplo médio:

```json
{
  "codigo_turma": "BIO_1A",
  "nivel": "1_ano_medio",
  "turno": "tarde",
  "curso_id": "uuid-do-curso-medio"
}
```

Exemplo superior:

```json
{
  "codigo_turma": "INF_1A",
  "nivel": "1_ano_superior",
  "turno": "noite",
  "curso_id": "uuid-do-curso-superior"
}
```

Regras importantes:

- `turno` deve ser `manha`, `tarde` ou `noite`.
- `codigo_turma` deve ser único dentro da academia.
- Ao editar `nivel` ou `curso_id` de uma turma existente, o backend valida todos os estudantes já vinculados e bloqueia a alteração se algum ficar incompatível.
- Para deletar uma turma, ela deve estar inativa e sem estudantes.

---

## 14. Passo 12 — Adicionar estudantes às turmas

Depois que estudantes e turmas existirem, vincule estudantes às turmas:

```http
POST /academia/turma/:codigo/estudante
```

Exemplo:

```json
{
  "codigo_estudante": "ABC1234"
}
```

Regras importantes:

- O estudante precisa pertencer à academia.
- O estudante precisa ser compatível com o nível e curso da turma.
- Apenas estudantes do Superior podem estar em múltiplas turmas simultaneamente.
- Para remover vínculo de turma, use `DELETE /academia/turma/:codigo/estudantes/:codigo_estudante`.

---

## 15. Passo 13 — Iniciar lançamentos acadêmicos

Com a configuração concluída, a academia já pode usar normalmente todas as funcionalidades da plataforma, incluindo gestão de estudantes, turmas, matérias, notas, faltas e acompanhamento das avaliações finais automáticas.

Em escolas, as avaliações seguem o padrão fixo do sistema. No Superior, seguem as categorias e regras configuradas pela academia. A avaliação final não possui rota pública de execução manual: ela é disparada automaticamente pelo backend quando uma nota é registrada e encontra as condições da regra aplicável.

Rotas operacionais comuns:

```http
POST /academia/notas-aluno
POST /academia/faltas-aluno
POST /academia/anos-letivos/finalizar
```

---

## 16. Checklist final de prontidão

Use este checklist antes de iniciar os lançamentos em produção:

- [ ] Academia está ativa.
- [ ] Admin já definiu o ano letivo global aplicável.
- [ ] Academia definiu seu ano letivo ativo.
- [ ] Anos fundamentais foram configurados, se a escola for fundamental ou mista.
- [ ] Cursos médios/superiores foram criados, se aplicável.
- [ ] Anos de cursos médios e períodos/anos superiores foram conferidos nas rotas de consulta.
- [ ] Matérias foram criadas para todos os anos, cursos e semestres necessários.
- [ ] Matérias superiores foram ativadas.
- [ ] Categorias de nota foram criadas para todos os anos superiores em uso; para escolas, confirme que o catálogo fixo aparece em `GET /academia/categorias-nota`.
- [ ] Regras de avaliação final superiores foram criadas, se a academia ofertar Superior; para escolas, nenhuma regra deve ser criada, e o padrão fixo deve aparecer em `GET /academia/avaliacao-final/regras`.
- [ ] Estudantes foram cadastrados ou aprovados com vínculo acadêmico correto.
- [ ] Turmas foram criadas com `nivel`, `turno` e `curso_id` corretos.
- [ ] Estudantes foram adicionados às turmas corretas.

---

## 17. Resumo visual das dependências

```text
Academia criada e ativada
        ↓
Ano letivo global definido pelo admin
        ↓
Academia define ano letivo ativo
        ↓
Anos fundamentais ───────────────┐
        ↓                         │
Cursos médios/superiores          │
        ↓                         │
Anos/períodos derivados           │
        ↓                         │
Matérias disciplinares ◄──────────┘
        ↓
Ativar matérias superiores
        ↓
Sem configuração extra de matérias especiais no curso
        ↓
Categorias superiores / catálogo escolar fixo
        ↓
Regras superiores / regras escolares fixas
        ↓
Estudantes
        ↓
Turmas
        ↓
Estudantes nas turmas
        ↓
Operação normal da plataforma
```

---

## 18. Erros comuns que este fluxo evita

| Erro | Causa provável | Como evitar |
|---|---|---|
| Nota bloqueada por ausência de ano letivo | Academia ainda não definiu o ano letivo ativo. | Execute `POST /academia/definir-ano-letivo` antes dos lançamentos. |
| Definição de ano letivo rejeitada | Ano letivo global não existe, academia já tem ano letivo ou payload diverge do global. | Confirme o ano global e não tente redefinir academia que já iniciou o ciclo. |
| `PATCH /academia/anos-academicos` retorna 404 | Rota removida do contrato público. | Use `POST` para adicionar e `DELETE` para remover anos fundamentais. |
| Anos médios rejeitados em `/academia/anos-academicos` | Anos médios são derivados do `modelo` do curso. | Crie curso médio com `modelo="liceu"` ou `modelo="tecnico"`. |
| Escopo superior rejeitado em `/academia/anos-academicos` | Cursos superiores não aceitam gestão direta de anos/períodos por essa rota. | Defina `periodos` na criação do curso superior. |
| Matéria média rejeitada | Curso médio não existe, está inativo, é de outra academia ou ano não pertence ao curso. | Crie/consulte o curso antes da matéria e use um ano derivado do curso. |
| Matéria superior não entra na avaliação | Matéria superior foi desativada explicitamente ou foi criada antes da regra atual de status padrão ativo. | Consulte o status e reative com `PUT /academia/materia/:id/ativar` se necessário. |
| Nota escolar rejeitada por categoria | Categoria enviada não pertence ao catálogo fixo do ano/curso, por exemplo `prova_trimestral` no `4_ano_medio` técnico. | Use somente as categorias fixas exibidas em `GET /academia/categorias-nota`. |
| Nota superior rejeitada por categoria | Categoria superior não existe, está inativa/removida ou não contém o ano aplicável. | Crie categorias superiores antes dos lançamentos e inclua todos os anos necessários. |
| Regra escolar rejeitada | Tentativa de criar, editar ou remover regra `fundamental`/`medio`. | Não configure regras escolares; use o padrão fixo do sistema. |
| Regra superior rejeitada | Escopo, fórmula, categoria, `nota_despertadora` ou cadeia incompatível. | Crie categorias e matérias superiores antes da regra; use `nivel="superior"` e fórmula textual válida. |
| Estudante não pode entrar na turma | Nível ou curso do estudante incompatível com a turma. | Cadastre estudante e turma com o mesmo nível/curso. |
