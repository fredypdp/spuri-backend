---
criado: 2026-07-06 00:00
status: pronto_para_implementacao
---

# Fixar anos acadêmicos dos cursos médios por modelo (feito)

## Prompt recomendado para executar a atualização

Implemente no backend a fixação dos `anos_academicos` dos cursos de ensino médio a partir do `modelo` do curso, sem permitir manipulação manual desse dado por APIs, comandos, handlers, aggregates, projeções ou rotas administrativas. A partir desta mudança, ao criar um curso médio com `modelo="liceu"`, o backend deve adicionar automaticamente `anos_academicos=["1_ano_medio", "2_ano_medio", "3_ano_medio"]`; ao criar um curso médio com `modelo="tecnico"`, deve adicionar automaticamente `anos_academicos=["1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"]`. Remova suporte a qualquer código legado que aceite, atualize, una, remova, reduza, expanda ou normalize `anos_academicos` manuais em cursos médios.

## Contexto do problema

Hoje o backend ainda possui contratos e validações que tratam `anos_academicos` de cursos médios como uma lista configurável pela academia. Esse comportamento entrou em conflito com o modelo atual do produto: cursos médios já possuem `modelo` obrigatório (`liceu` ou `tecnico`), e a duração acadêmica deve ser consequência fixa desse modelo.

A regra nova é simples e fechada:

| Modelo do curso médio | Anos acadêmicos obrigatórios e derivados |
| --- | --- |
| `liceu` | `1_ano_medio`, `2_ano_medio`, `3_ano_medio` |
| `tecnico` | `1_ano_medio`, `2_ano_medio`, `3_ano_medio`, `4_ano_medio` |

Portanto, `anos_academicos` deixa de ser uma entrada editável para cursos médios e passa a ser um dado derivado, persistido/projetado apenas para leitura, compatibilidade operacional e consultas já existentes.

## Objetivo funcional

Garantir que todo curso médio tenha seus anos acadêmicos definidos automaticamente, de forma determinística, exclusivamente pelo `modelo` informado na criação do curso.

Regras obrigatórias:

1. Curso médio com `modelo="liceu"` sempre deve resultar em:

```json
{
  "modelo": "liceu",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"]
}
```

2. Curso médio com `modelo="tecnico"` sempre deve resultar em:

```json
{
  "modelo": "tecnico",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"]
}
```

3. Nenhum endpoint público, autenticado ou administrativo deve aceitar `anos_academicos` como campo de entrada para criar, editar, adicionar, remover, ativar, desativar ou reordenar anos de curso médio.
4. Nenhuma operação deve permitir deixar um curso médio com lista parcial, vazia, duplicada, fora de ordem, maior que o modelo ou menor que o modelo.
5. O valor projetado em respostas deve continuar disponível quando necessário, mas sempre como consequência do modelo do curso.

## Escopo obrigatório

### 1. Centralizar a derivação dos anos acadêmicos médios

Criar ou ajustar uma função/helper de domínio para derivar os anos a partir do modelo do curso médio.

Comportamento esperado:

```text
modelo=liceu   -> [1_ano_medio, 2_ano_medio, 3_ano_medio]
modelo=tecnico -> [1_ano_medio, 2_ano_medio, 3_ano_medio, 4_ano_medio]
```

Critérios:

- a função deve ser usada por criação de curso, aggregate, command handler e/ou camada de validação relevante;
- não duplicar arrays literais em vários pontos sem necessidade;
- rejeitar modelo ausente ou inválido antes de derivar os anos;
- manter o campo `modelo` exclusivo de cursos médios;
- manter cursos superiores sem `modelo` e sem `anos_academicos` manuais.

### 2. Alterar criação de curso médio

Na criação de curso médio:

- exigir `modelo` com valor `liceu` ou `tecnico`;
- ignorar a ideia antiga de lista manual e **não aceitar** `anos_academicos` no payload;
- derivar `anos_academicos` no backend logo antes de criar o curso/evento/snapshot;
- persistir/projetar a lista derivada para manter compatibilidade com matérias, regras de avaliação, progressão de estudantes, respostas de API e consultas.

Payload novo esperado para curso médio:

```json
{
  "nome": "Curso de Ciências e Tecnologias",
  "type": "medio",
  "modelo": "liceu"
}
```

Payload legado que deve ser rejeitado:

```json
{
  "nome": "Curso Técnico de Informática",
  "type": "medio",
  "modelo": "tecnico",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"]
}
```

Erro esperado: `400`, com envelope claro indicando que `anos_academicos` não é aceito para cursos médios porque os anos são fixos e derivados de `modelo`.

### 3. Remover suporte a edição/manipulação manual dos anos do curso médio

Remover ou bloquear definitivamente qualquer fluxo que permita manipular anos acadêmicos de cursos médios, incluindo:

- endpoints de `POST`, `PUT`, `PATCH` ou `DELETE` relacionados a `/academia/anos-academicos` quando `type="medio"`;
- campos acadêmicos em edição cadastral de curso médio (`anos_academicos`, `anosAcademicos`, `anos`, listas equivalentes ou aliases legados);
- validações antigas de sequência manual crescente iniciada em `1_ano_medio` para payloads de curso médio;
- lógicas de merge/união de anos enviados com anos existentes;
- lógicas de remoção/desativação de anos médios do curso;
- mensagens que orientem a academia a adicionar/remover anos médios manualmente.

A rota de anos acadêmicos pode continuar existindo para o que ainda for válido no domínio, como fundamental na academia ou superior via `periodos`, mas não deve mais suportar `type="medio"`.

### 4. Remover código legado em vez de manter compatibilidade silenciosa

Esta tarefa exige remoção ativa de suporte legado. Não implementar fallback silencioso que aceite `anos_academicos` e simplesmente ignore o campo.

Obrigatório:

- procurar ocorrências de `anos_academicos` relacionadas a cursos médios e classificar cada uma como leitura derivada ainda válida, validação necessária, documentação/teste a atualizar ou legado a remover;
- remover DTOs/campos de request usados apenas para manipular anos médios;
- remover comandos/eventos/métodos que representem adição/remoção manual de anos médios, se não forem usados por outro domínio válido;
- remover testes que validem adição/remoção manual e substituí-los por testes de rejeição;
- remover documentação que mostre `anos_academicos` no payload de criação/edição de curso médio.

Comando sugerido para auditoria inicial:

```bash
rg -n "anos_academicos|anosAcademicos|ano_medio|modelo|liceu|tecnico" internal migrations docs
```

### 5. Preservar leitura e compatibilidade operacional

Embora a entrada manual seja removida, a leitura de `anos_academicos` projetados pode continuar existindo porque outras partes do sistema dependem desse dado para:

- criar matérias médias por ano acadêmico;
- configurar `materias_chave` por ano do curso;
- validar regras de avaliação final de ensino médio por `curso_id` + `ano_academico`;
- avançar estudantes no ensino médio;
- consultar cursos e exibir sua duração.

Essa compatibilidade é apenas de leitura/derivação. Nenhum consumidor deve tratar o campo como editável.

### 6. Tratar dados existentes e migrations

Avaliar se existem cursos médios já persistidos com anos incompatíveis com o próprio `modelo`.

A implementação deve incluir uma estratégia explícita:

- migration idempotente para corrigir `projection_cursos.anos_academicos` de cursos médios existentes a partir de `modelo`, quando a arquitetura exigir correção direta de projeção;
- ou replay/rebuild de projeções, se este for o padrão do projeto;
- ou comando operacional documentado, se a correção depender de rotina já existente.

A decisão deve ser descrita no PR. O estado final não pode manter curso médio `liceu` com 4 anos nem curso médio `tecnico` com 3 anos, salvo se houver justificativa de legado arquivado explicitamente não operacional e coberto por teste/consulta.

### 7. Atualizar documentação pública e funcional

Atualizar a documentação para refletir o novo contrato:

- `POST /academia/curso` para curso médio recebe `modelo`, mas não recebe `anos_academicos`;
- respostas de curso médio podem exibir `anos_academicos` derivados;
- `modelo=liceu` implica 3 anos;
- `modelo=tecnico` implica 4 anos;
- `/academia/anos-academicos` não manipula mais anos de curso médio;
- remover exemplos antigos de criação de curso médio com `anos_academicos` no payload.

## Validações esperadas

### Criação de curso médio

- `type="medio"` sem `modelo` deve retornar `400`.
- `type="medio"` com `modelo` diferente de `liceu` ou `tecnico` deve retornar `400`.
- `type="medio"` com `anos_academicos` no payload deve retornar `400`.
- `type="medio"` com `modelo="liceu"` deve criar/projetar exatamente 3 anos médios.
- `type="medio"` com `modelo="tecnico"` deve criar/projetar exatamente 4 anos médios.

### Edição de curso médio

- qualquer payload com `anos_academicos`, `anosAcademicos`, `anos` ou aliases equivalentes deve retornar `400` sem mutação parcial;
- alteração de `modelo` deve ser bloqueada, a menos que o domínio já possua regra explícita segura para troca de modelo. Se for permitido no futuro, deverá ser outra tarefa porque trocar `liceu` por `tecnico` altera duração, matérias, progressão, estudantes, avaliações, pendências e histórico.

### Endpoint de anos acadêmicos

- `type="medio"` em endpoint de gestão de anos acadêmicos deve retornar `400` ou `410`, com mensagem clara de que anos médios são fixos por modelo de curso;
- `type="fundamental"` e o fluxo superior por `periodos`, se existirem, não devem regredir.

## Testes obrigatórios

Adicionar ou atualizar testes cobrindo pelo menos:

1. criação de curso médio `liceu` sem `anos_academicos` gera 3 anos;
2. criação de curso médio `tecnico` sem `anos_academicos` gera 4 anos;
3. criação de curso médio com `anos_academicos` é rejeitada;
4. criação de curso médio com modelo inválido é rejeitada;
5. edição de curso médio com campos acadêmicos legados é rejeitada;
6. endpoint de gestão de anos acadêmicos rejeita `type="medio"`;
7. criação/configuração de matéria média continua validando o ano contra a lista derivada do curso;
8. configuração de `materias_chave` continua exigindo cobertura para todos os anos derivados do curso;
9. progressão de estudante médio respeita o limite derivado: 3º ano para `liceu`, 4º ano para `tecnico`;
10. cursos superiores continuam rejeitando `modelo` e `anos_academicos` manuais.

## Critérios de aceite

- Cursos médios não aceitam mais `anos_academicos` em nenhum payload de escrita.
- Cursos médios derivam anos exclusivamente de `modelo`.
- `liceu` sempre possui exatamente `["1_ano_medio", "2_ano_medio", "3_ano_medio"]`.
- `tecnico` sempre possui exatamente `["1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"]`.
- Não existe mais suporte legado para adicionar/remover anos médios manualmente.
- Documentação e exemplos não orientam o cliente a enviar `anos_academicos` para cursos médios.
- Testes automatizados cobrem criação, rejeição de legado, leitura derivada e não regressão dos fluxos dependentes.
