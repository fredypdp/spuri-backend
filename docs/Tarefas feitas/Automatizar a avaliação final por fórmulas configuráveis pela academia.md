---
modificado: 2026-06-28 17:05
criado: 2026-06-18 16:04
---
# Automatizar a avaliação final por fórmulas configuráveis pela academia (feito)
## Contexto do código atual

O fluxo atual de avaliação final ainda recebe a decisão pronta no payload de `POST /academia/avaliacao-final`: a academia envia `aprovado` e o backend apenas valida algumas notas obrigatórias quando a aprovação é enviada sem observação. A decisão fica persistida em `projection_avaliacao_final.aprovado`, e as consultas legadas `GET /aprovacoes` e `GET /reprovacoes` continuam filtrando essa mesma projeção.

Hoje a validação de notas é rígida por tipo de ensino:

- fundamental e médio esperam a categoria `nota_escola` nos trimestres/períodos esperados;
- superior espera `nota_exame` no período configurado da matéria;
- não existe cálculo persistido de média/nota final;
- o evento `AvaliacaoFinalEscolar`/`AvaliacaoFinalSuperior` possui um campo `tipo`, mas ele é gravado como valor técnico fixo (`avaliacao_final`), não como tipo de regra acadêmica (`normal`, `recurso`, etc.).

Já existe base para categorias de nota configuráveis por academia em `projection_categorias_nota`, inclusive com `anos_academicos`, mas a avaliação final ainda não usa essas categorias dinamicamente para calcular a decisão.

## Objetivo

Substituir a avaliação final manual por um fluxo automático, configurável e auditável, no qual a academia define regras de aprovação por tipo de avaliação final. O backend deve calcular `nota_final`, decidir `aprovado/reprovado` de forma determinística e registrar a avaliação final com os dados da regra usada.

## Resultado esperado

Implementar um modelo em que:

1. A academia consiga configurar fórmulas de avaliação final por ano acadêmico (mais de um se ela quiser), tipo de ensino e tipo de avaliação.
2. O registro da avaliação final não aceite mais a decisão manual `aprovado` como fonte de verdade.
3. O backend calcule `nota_final` a partir das notas existentes e da fórmula configurada.
4. O backend compare `nota_final` com a nota mínima da regra e defina automaticamente se o estudante foi aprovado.
5. A avaliação final passe a ter um campo público `type`, com valor padrão `normal`.
6. Tipos adicionais, como `recurso`, possam ser cadastrados pela academia e vinculados a uma regra anterior que precisa ter sido reprovada.
7. Escolas mistas consigam configurar padrões diferentes para fundamental e médio; para médio, a regra vale para o ano acadêmico médio em geral, e não para o ano acadêmico de um curso específico.

## Requisitos funcionais

### 1. Configuração de regras de avaliação final

Criar uma configuração administrada pela academia para regras de avaliação final. Uma regra deve conter, no mínimo:

- `codigo_academia`;
- `type` da avaliação final (`normal` por padrão; exemplos: `normal`, `recurso`, `especial`);
- `nome`/`descricao` para exibição (obrigatório);
- `tipo_ensino` ou escopo equivalente (`fundamental`, `medio`, `superior`, quando aplicável), o sistema pega diretamente do tipo da academia mas se ela for misto então ela pode definir esse campo;
- `anos_academicos` aos quais a regra se aplica (um ano não pode estar aplicado a duas regras do mesmo tipo);
- `nota_minima_aprovacao` (deve ser maior que zero e é definida pela academia);
- `categorias_envolvidas` na fórmula;
- `formula` em formato seguro e interpretável pelo backend;
- campo opcional para indicar dependência de reprovação anterior, por exemplo `aplica_se_reprovado_em_type`;
- estado ativo/inativo e metadados de auditoria.

Validações obrigatórias:

- `type` deve ser único por academia dentro do mesmo escopo de ensino/anos acadêmicos, quando isso causaria ambiguidade.
- `normal` deve existir como padrão ou ser criado automaticamente com uma regra compatível de migração/backfill.
- `aplica_se_reprovado_em_type` não pode apontar para si mesmo nem criar ciclos.
- categorias usadas na fórmula precisam existir para a academia e estar disponíveis para os anos acadêmicos da regra.
- a fórmula não pode executar código arbitrário, acessar funções externas, fazer SQL dinâmico ou depender de input não validado.

### 2. Linguagem/estrutura segura para fórmulas

Não usar `eval` textual livre. Preferir uma DSL estruturada em JSON, AST validada ou conjunto limitado de operações.

A fórmula deve suportar, pelo menos:

- soma de categorias por período;
- média/divisão por constante;
- composição de subexpressões;
- referência a categorias configuradas pela academia;
- períodos letivos existentes (`1_trimestre`, `2_trimestre`, `3_trimestre`, `N_semestre` etc.);
- regra de agregação por matéria quando houver várias matérias.

Exemplo sugerido de representação estruturada:

```json
{
  "op": "div",
  "left": {
    "op": "sum_periods",
    "categories": ["nota_escola", "nota_professor"],
    "periods": ["1_trimestre", "2_trimestre", "3_trimestre"]
  },
  "right": 3
}
```

Para uma fórmula com exame final:

```json
{
  "op": "div",
  "left": {
    "op": "add",
    "items": [
      {
        "op": "div",
        "left": {
          "op": "sum_periods",
          "categories": ["nota_escola", "nota_professor"],
          "periods": ["1_trimestre", "2_trimestre", "3_trimestre"]
        },
        "right": 3
      },
      { "op": "category_total", "category": "nota_exame_final" }
    ]
  },
  "right": 2
}
```

Para recuperação/recurso, a academia poderia cadastrar um novo `type = recurso`, com `aplica_se_reprovado_em_type = normal`, e trocar `nota_exame_final` por `nota_exame_recurso` na fórmula.

### 3. Registro automático da avaliação final

Remover as rotas responsáveis por aplicar a avaliação final do estudante, agora a avaliação final adequada é acionada assim que todas as notas necessárias para aquela avaliação final forem registradas para o estudante. Se a academia tiver alguma avaliação para aqueles reprovados numa anterior o estudante ainda não deve ser reprovado, ele só é provado definitivamente se também reprovar nessa avaliação final que depende de uma primeira

Depois que a avaliação final for acionada o backend deve:
1. validar que o estudante pertence à academia;
2. inferir/validar tipo de ensino e ano acadêmico atual;
3. localizar exatamente uma regra ativa aplicável ao `type`, tipo de ensino e ano acadêmico;
4. se o `type` depender de reprovação em outro tipo, validar que existe avaliação final anterior reprovada para o estudante no mesmo ano letivo/ano acadêmico;
5. carregar notas do estudante no ano letivo;
6. validar presença das categorias/períodos/matérias exigidas pela fórmula;
7. calcular `nota_final` com precisão decimal adequada;
8. definir `aprovado = nota_final >= nota_minima_aprovacao`;
9. calcular `proximo_ano_academico` somente se aprovado;
10. registrar evento e projeção com `type`, `nota_final`, `nota_minima_aprovacao`, `formula_snapshot` e identificador/versão da regra usada.

### 4. Persistência e eventos

Adicionar migrations e adaptar projeções/eventos para incluir:

- tabela/projeção de regras de avaliação final;
- colunas em `projection_avaliacao_final`:
  - `type` (`normal` por padrão);
  - `nota_final`;
  - `nota_minima_aprovacao`;
  - `regra_avaliacao_final_id` ou identificador equivalente;
  - `formula_snapshot` para auditoria/reprodutibilidade;
  - opcionalmente `aplica_se_reprovado_em_type` resolvido no momento do registro.

O evento de avaliação final deve carregar esses dados para que rebuilds sejam determinísticos mesmo se a academia alterar a regra depois.

### 5. Duplicidade e sequência de avaliações

Revisar a regra de unicidade atual. Como agora podem existir tipos diferentes de avaliação final no mesmo ano letivo e ano acadêmico, a unicidade deve considerar também `type`.

Regras esperadas:

- um estudante não pode ter duas avaliações finais do mesmo `type`, no mesmo ano letivo, escopo e ano acadêmico;
- um estudante pode ter `normal` reprovado e depois `exame_recurso`, se a regra `recurso` permitir isso;
- não deve ser possível registrar `exame_recurso` antes de existir uma reprovação no `type` configurado como pré-requisito;
- se `normal` já aprovou, não deve ser possível registrar `exame_recurso` dependente de `normal`.

### 6. Compatibilidade das rotas de consulta

Manter `GET /avaliacoes`, `GET /aprovacoes` e `GET /reprovacoes`, mas enriquecer as respostas com:

- `type`;
- `nota_final`;
- `nota_minima_aprovacao`;
- dados mínimos da regra usada, quando útil.

Adicionar filtro opcional por `type` nas consultas de avaliação final/aprovações/reprovações.

## Exemplos de regra

### Exemplo 1 — avaliação normal simples

- `type`: `normal`
- `nota_minima_aprovacao`: `10`
- categorias: `nota_escola`, `nota_professor`
- fórmula: somar `nota_escola + nota_professor` de cada trimestre, dividir por `3`, salvar o resultado em `nota_final`.
- decisão: `aprovado = nota_final >= 10`.

### Exemplo 2 — avaliação normal com exame final

- `type`: `normal`
- `nota_minima_aprovacao`: `10`
- categorias: `nota_escola`, `nota_professor`, `nota_exame_final`
- fórmula: `((soma_trimestres(nota_escola + nota_professor) / 3) + nota_exame_final) / 2`
- se `nota_final >= 10`, aprova;
- se `nota_final < 10`, reprova e permite seguir para o tipo de recuperação configurado.

### Exemplo 3 — exame de recurso/recuperação

- `type`: `recurso`
- `aplica_se_reprovado_em_type`: `normal`
- `nota_minima_aprovacao`: definido pela academia;
- fórmula similar à normal, mas usando `nota_exame_recurso` em vez de `nota_exame_final`.

## Observações de implementação

- Tratar `type` como campo de negócio da avaliação final. Evitar confusão com o campo técnico/eventual que hoje recebe `avaliacao_final`.
- Salvar snapshot da fórmula no evento/projeção para auditoria, pois regras podem mudar depois.
- Usar `numeric`/decimal no banco para notas, evitando perda de precisão por `float` quando possível.
- A validação de categorias deve reutilizar a infraestrutura existente de categorias de nota por academia e `anos_academicos`.
- Manter `proximo_ano_academico` calculado no backend, como já ocorre hoje.