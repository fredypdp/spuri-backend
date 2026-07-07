# Manual de Configuração Inicial da Academia

Este manual orienta a academia recém-criada e ativada a configurar o ambiente acadêmico em **ordem cronológica**, começando pelos domínios que não dependem de outros dados e avançando até os processos que dependem da configuração curricular completa.

> Objetivo: ao final deste fluxo, a academia estará pronta para cadastrar estudantes, organizar turmas, lançar notas e faltas, e permitir que a avaliação final automática funcione corretamente: em escolas, pelo padrão fixo do sistema; no Superior, pelas categorias e regras configuradas pela academia.

---

## 1. Visão geral da ordem recomendada

Antes dos passos operacionais, identifique o tipo da academia ativada conforme a seção 2. Depois siga esta sequência para evitar erros de dependência:

1. **Definir o primeiro ano letivo ativo da academia**.
2. **Definir os anos acadêmicos fundamentais**, quando a academia for escola fundamental ou mista.
3. **Criar cursos médios ou superiores**, quando aplicável.
4. **Conferir anos acadêmicos de cursos médios**, derivados automaticamente pelo modelo do curso.
5. **Criar matérias disciplinares**.
6. **Ativar matérias superiores**, pois elas nascem inativas.
7. **Configurar matérias-chave dos cursos médios**, quando aplicável.
8. **Criar categorias de nota superiores**, somente quando a academia ofertar Superior; escolas usam catálogo fixo.
9. **Criar regras de avaliação final superiores**, somente quando a academia ofertar Superior; escolas usam regras fixas.
10. **Cadastrar ou aprovar estudantes**.
11. **Criar turmas**.
12. **Adicionar estudantes às turmas**.
13. **Iniciar a operação acadêmica**: usar normalmente as funcionalidades da plataforma.

---

## 2. Antes de começar

Este manual deve ser seguido depois que a academia já estiver criada e ativada. A partir desse ponto, confira o tipo da instituição, porque ele determina quais configurações serão permitidas:

| Academia | Configurações curriculares esperadas |
|---|---|
| Escola `fundamental` | Anos acadêmicos ficam na própria academia. |
| Escola `medio` | Anos acadêmicos ficam nos cursos médios. |
| Escola `misto` | Fundamental fica na academia; Médio fica nos cursos médios. |
| `superior` | Semestres e anos são derivados dos cursos superiores. |

---

## 3. Passo 1 — Definir o ano letivo da academia

A academia define seu primeiro ano letivo ativo:

```http
POST /academia/definir-ano-letivo
```

Regras principais:

- A academia só define diretamente o ano letivo uma vez.
- Depois disso, o avanço acontece pela finalização do ano letivo.
- Sem ano letivo ativo, o sistema bloqueia notas, faltas e avaliações finais.

**Por que este passo vem antes dos demais processos operacionais?**

Porque notas, faltas e avaliações finais sempre são registradas no contexto do ano letivo ativo da academia. A configuração curricular pode ser preparada antes do primeiro lançamento, mas a operação acadêmica não deve começar sem este passo.

---

## 4. Passo 2 — Configurar anos acadêmicos do Fundamental, se aplicável

Este passo se aplica apenas a escolas com nível escolar:

- `fundamental`
- `misto`

Escolas exclusivamente médias não configuram anos fundamentais na academia. Instituições superiores também não usam esta rota para anos superiores.

### 4.1 Consultar a situação atual

```http
GET /academia/anos-academicos
```

### 4.2 Adicionar anos fundamentais

```http
POST /academia/anos-academicos
```

Exemplo conceitual:

```json
{
  "type": "fundamental",
  "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental", "3_ano_fundamental"]
}
```

Use somente anos no formato:

```text
[n]_ano_fundamental
```

com `n` de 1 a 9.

**Dependências:** este passo não depende de cursos ou matérias. Ele deve vir antes da criação de matérias fundamentais e estudantes fundamentais. Categorias e regras do Fundamental não são configuradas pela academia; o backend expõe o catálogo e as regras fixas automaticamente.

---

## 5. Passo 3 — Criar cursos médios ou superiores, se aplicável

Cursos são necessários para:

- escolas médias;
- escolas mistas que ofertam médio;
- instituições superiores.

Rota:

```http
POST /academia/curso
```

### 5.1 Curso médio

Um curso médio deve informar `modelo` (`liceu` ou `tecnico`). O backend deriva automaticamente os anos acadêmicos: `liceu` gera 1º a 3º ano médio e `tecnico` gera 1º a 4º ano médio.

Exemplo:

```json
{
  "nome": "Ciências Físicas e Biológicas",
  "type": "medio",
  "modelo": "liceu"
}
```

Não envie `materias_chave` na criação do curso médio. Essas matérias só podem ser configuradas depois que as matérias disciplinares existirem.

### 5.2 Curso superior

Um curso superior recebe a quantidade total de semestres em `periodos`. O backend deriva automaticamente:

- os semestres: `1_semestre`, `2_semestre`, etc.;
- os anos acadêmicos superiores: `1_ano_superior`, `2_ano_superior`, etc.

Exemplo:

```json
{
  "nome": "Engenharia Informática",
  "type": "superior",
  "periodos": 8
}
```

Não envie `anos_academicos` nem `materias_chave` para cursos superiores.

**Dependências:** cursos devem existir antes de matérias médias/superiores, turmas médias/superiores, estudantes médios/superiores, categorias superiores e regras superiores. Cursos médios não exigem criação de regras finais pela academia.

---

## 6. Passo 4 — Conferir anos acadêmicos derivados dos cursos médios

Não adicione nem remova anos de cursos médios manualmente. Os anos são fixos por `modelo`:

- `liceu`: `1_ano_medio`, `2_ano_medio`, `3_ano_medio`;
- `tecnico`: `1_ano_medio`, `2_ano_medio`, `3_ano_medio`, `4_ano_medio`.

A rota `/academia/anos-academicos` rejeita `type="medio"`.

---

## 7. Passo 5 — Criar matérias disciplinares

As matérias dependem dos anos acadêmicos e, em médio/superior, também dependem do curso.

Rota:

```http
POST /academia/materia
```

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

Matérias fundamentais nascem ativas.

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

Matérias médias nascem ativas. Matérias dependentes/pendências não são permitidas no Médio escolar; `pendencia_permitida` e `pendencia_nivel_conclusao` são exclusivos do Superior.

### 7.3 Matéria superior

Requer curso superior já criado, um ano acadêmico superior compatível e o semestre da matéria.

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

Matérias superiores nascem inativas e precisam ser ativadas no próximo passo.

---

## 8. Passo 6 — Ativar matérias superiores

Este passo só se aplica ao ensino superior.

Depois de criar e revisar uma matéria superior, ative-a:

```http
PUT /academia/materia/:id/ativar
```

Sem ativação, a matéria superior não será considerada como matéria ativa para os processos acadêmicos.

---

## 9. Passo 7 — Configurar matérias-chave de cursos médios

Este passo só se aplica a cursos médios.

As matérias-chave pertencem ao curso médio, por ano acadêmico, como configuração curricular do curso. Elas não pertencem a regras de avaliação final e não tornam o Médio configurável pela academia.

Pré-requisitos:

1. Curso médio criado e ativo.
2. Anos acadêmicos do curso médio definidos.
3. Matérias médias criadas, ativas, do mesmo curso e do mesmo ano acadêmico.

Rota:

```http
PUT /academia/curso/:id/materias-chave
```

Exemplo:

```json
{
  "materias_chave": [
    {
      "ano_academico": "1_ano_medio",
      "materias_chave": ["uuid-materia-biologia", "uuid-materia-quimica"]
    },
    {
      "ano_academico": "2_ano_medio",
      "materias_chave": ["uuid-materia-fisica"]
    }
  ]
}
```

Regras importantes:

- Todo ano do curso médio deve ter configuração.
- Cada ano precisa ter pelo menos uma matéria-chave.
- As matérias precisam estar ativas.
- As matérias precisam pertencer ao mesmo curso médio e ao ano acadêmico informado.

**Por que este passo vem antes das notas?**

As matérias-chave continuam sendo uma configuração curricular obrigatória do curso médio, mas não são configuradas em regras de avaliação final. No padrão avaliativo escolar fixo, escolas não criam categorias nem regras; o backend usa as categorias/regras oficiais do sistema e mantém as matérias-chave do curso disponíveis para validações, auditoria e fluxos curriculares do Médio.

---

## 10. Passo 8 — Categorias de nota

Este passo só exige ação de academias com oferta Superior. Escolas devem apenas consultar o catálogo fixo se quiserem validar a configuração exibida.

As categorias de nota seguem dois modelos:

- **Escolas (fundamental/médio):** não criam categorias por endpoint. O backend fornece automaticamente o catálogo fixo do sistema por ano acadêmico, marcado como `source="system"`, `fixed=true` e `readonly=true` na listagem. Para Médio, a listagem deriva os anos dos cursos médios ativos; `academia.anos_academicos` continua sendo usado para Fundamental.
- **Superior:** cria categorias explicitamente antes de lançar notas e antes de configurar fórmulas de avaliação final.

Rota de criação, exclusiva do Superior:

```http
POST /academia/categorias-nota
```

Exemplo superior:

```json
{
  "codigo": "prova_parcelar_1",
  "nome": "Prova Parcelar 1",
  "anos_academicos": ["1_ano_superior", "2_ano_superior"]
}
```

Catálogo escolar fixo:

| Anos acadêmicos | Categorias |
|---|---|
| `1_ano_fundamental` a `5_ano_fundamental`, `7_ano_fundamental`, `8_ano_fundamental`, `1_ano_medio`, `2_ano_medio` | `nota_professor`, `prova_trimestral` |
| `6_ano_fundamental`, `9_ano_fundamental`, `3_ano_medio` | `nota_professor`, `prova_trimestral`, `exame_final`, `exame_recurso` |
| `4_ano_medio` de curso técnico (`modelo="tecnico"`) | `nota_pap` |

Regras importantes:

- Escolas não podem criar, editar ou remover categorias escolares; tentativas em `POST/DELETE /academia/categorias-nota` falham.
- Categorias escolares legadas eventualmente existentes na projeção não devem ser usadas para orientar lançamentos ou avaliação final: o backend valida notas escolares contra o catálogo fixo aplicável ao ano/curso.
- Superior continua usando categorias configuráveis pela academia.
- A categoria superior precisa incluir os anos/períodos acadêmicos nos quais poderá receber notas.
- O lançamento de nota valida a escala do ano: `0–10` para `1_ano_fundamental` a `6_ano_fundamental`; `0–20` para `7_ano_fundamental` a `9_ano_fundamental`, Médio e Superior.

---

## 11. Passo 9 — Regras de avaliação final superior

Este passo só exige ação de academias com oferta Superior. Escolas (`fundamental`/`medio`) não criam, editam ou removem regras de avaliação final; o backend fornece as regras fixas do sistema e bloqueia tentativas de configuração escolar por endpoint.

### 11.1 Fundamental e Médio escolar

Não crie regras por endpoint para escolas. `POST`, `PUT/PATCH` e `DELETE` de regras escolares são bloqueados; a listagem e a execução automática devem ser interpretadas como catálogo oficial do sistema. O backend aplica automaticamente o padrão fixo:

- anos regulares usam média dos três trimestres com `nota_professor` e `prova_trimestral`;
- `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio` usam `exame_final` no 3º trimestre e permitem `exame_recurso` apenas para matérias reprovadas;
- `4_ano_medio` técnico usa somente `nota_pap >= 10`;
- matérias dependentes/pendências não existem no Médio escolar;
- se a categoria lançada não for gatilho oficial da etapa (`nota_professor`, por exemplo), nenhuma regra configurável/legada é usada como fallback.

### 11.2 Superior

Configuração típica:

- `nivel="superior"`;
- sem `anos_academicos` na regra;
- fórmula com referência apenas à categoria, pois o semestre é inferido pela matéria;
- `limite_materias_pendentes` obrigatório;
- pendências (`pendencia_permitida` e `pendencia_nivel_conclusao`) são exclusivas de matérias superiores.

Exemplo de fórmula:

```text
([prova_parcelar_1]+[prova_parcelar_2])/2
```

Ordem dentro da cadeia superior:

1. Criar regra raiz, por exemplo `avaliacao_final`.
2. Criar regra descendente que aponta para a raiz, por exemplo `avaliacao_final_com_exame` com `aplica_se_reprovado_em_type="avaliacao_final"`.
3. Criar novas descendentes, se houver, sempre apontando para uma etapa anterior ativa e sem criar ciclos.

---

## 12. Passo 10 — Cadastrar ou aprovar estudantes

Depois que a estrutura curricular básica estiver pronta, cadastre os estudantes pela academia ou aprove solicitações de matrícula.

Cadastro direto:

```http
POST /academia/estudante/register
```

Ao cadastrar, informe os vínculos acadêmicos compatíveis com o tipo de estudante:

| Estudante | Campos acadêmicos esperados |
|---|---|
| Fundamental | `ano_escolar_fundamental` compatível com os anos da academia. |
| Médio | `curso_medio_id` e `ano_escolar_medio` compatíveis com o curso médio. |
| Superior | `curso_superior_id`, `ano_superior`/semestre inicial conforme contrato do cadastro. |

**Por que estudantes vêm depois da estrutura curricular?**

Porque o cadastro acadêmico do estudante precisa apontar para anos e cursos existentes. Além disso, notas, faltas, turmas e avaliações dependem do estudante estar vinculado corretamente à academia.

---

## 13. Passo 11 — Criar turmas

Turmas dependem do nível e, para Médio/Superior, normalmente do curso.

Rota:

```http
POST /academia/turma
```

Exemplo fundamental:

```json
{
  "codigo_turma": "6A",
  "nivel": "fundamental",
  "turno": "manha"
}
```

Exemplo médio:

```json
{
  "codigo_turma": "BIO-1A",
  "nivel": "medio",
  "turno": "tarde",
  "curso_id": "uuid-do-curso-medio"
}
```

Exemplo superior:

```json
{
  "codigo_turma": "INF-1A",
  "nivel": "superior",
  "turno": "noite",
  "curso_id": "uuid-do-curso-superior"
}
```

O `codigo_turma` deve ser único dentro da academia.

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
- Apenas estudantes do superior podem estar em múltiplas turmas simultaneamente.

---

## 15. Passo 13 — Iniciar lançamentos acadêmicos

Com a configuração concluída, a academia já pode usar normalmente todas as funcionalidades da plataforma. A partir deste ponto, a operação acadêmica pode seguir o fluxo regular de trabalho da instituição, incluindo gestão de estudantes, turmas, matérias, notas, faltas e acompanhamento das avaliações finais automáticas. Em escolas, essas avaliações seguem o padrão fixo do sistema; no Superior, seguem as regras configuradas pela academia.

---

## 16. Checklist final de prontidão

Use este checklist antes de iniciar os lançamentos em produção:

- [ ] Academia está ativa.
- [ ] Academia definiu seu ano letivo ativo.
- [ ] Anos fundamentais foram configurados, se a escola for fundamental ou mista.
- [ ] Cursos médios/superiores foram criados, se aplicável.
- [ ] Anos de cursos médios estão completos e sequenciais.
- [ ] Matérias foram criadas para todos os anos, cursos e semestres necessários.
- [ ] Matérias superiores foram ativadas.
- [ ] Cursos médios possuem matérias-chave configuradas para todos os anos.
- [ ] Categorias de nota foram criadas para todos os anos/períodos superiores em uso; para escolas, confirme que o catálogo fixo aparece em `GET /academia/categorias-nota`.
- [ ] Estudantes foram cadastrados ou aprovados com vínculo acadêmico correto.
- [ ] Turmas foram criadas.
- [ ] Estudantes foram adicionados às turmas corretas.
- [ ] Regras de avaliação final superiores foram criadas, se a academia ofertar Superior; para escolas, nenhuma regra deve ser criada, e o padrão fixo deve aparecer em `GET /academia/avaliacao-final/regras`.

---

## 17. Resumo visual das dependências

```text
Academia ativada
        ↓
Academia define ano letivo ativo
        ↓
Anos fundamentais ───────────────┐
        ↓                         │
Cursos médios/superiores          │
        ↓                         │
Anos de curso médio               │
        ↓                         │
Matérias disciplinares ◄──────────┘
        ↓
Ativar matérias superiores
        ↓
Matérias-chave do Médio
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
| Matéria média rejeitada | Curso médio não existe, é de outra academia ou ano não pertence ao curso. | Crie/consulte o curso antes da matéria. |
| Matéria superior não entra na avaliação | Matéria superior foi criada, mas continua inativa. | Ative com `PUT /academia/materia/:id/ativar`. |
| Configuração curricular do Médio incompleta | Curso médio sem `materias_chave` para todos os anos do curso. | Configure `PUT /academia/curso/:id/materias-chave` para todos os anos do curso. |
| Nota escolar rejeitada por categoria | Categoria enviada não pertence ao catálogo fixo do ano/curso, por exemplo `prova_trimestral` no `4_ano_medio` técnico. | Use somente as categorias fixas exibidas em `GET /academia/categorias-nota`. |
| Nota superior rejeitada por categoria | Categoria superior não existe, está inativa/removida ou não contém o ano/período inferido. | Crie categorias superiores antes dos lançamentos e inclua todos os anos/períodos necessários. |
| Regra escolar rejeitada | Tentativa de criar, editar ou remover regra `fundamental`/`medio`. | Não configure regras escolares; use o padrão fixo do sistema. |
| Regra superior rejeitada | Escopo, fórmula, categoria ou cadeia incompatível. | Crie categorias e matérias superiores antes da regra; use `nivel="superior"` e o formato correto. |
| Estudante não pode entrar na turma | Nível ou curso do estudante incompatível com a turma. | Cadastre estudante e turma com o mesmo nível/curso. |
