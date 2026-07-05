# Adicionar campo `modelo` aos cursos médios

## Objetivo

Implementar um novo campo público e persistente chamado `modelo` para cursos de `type='medio'`.

Esse campo deve indicar o modelo pedagógico/curricular do curso médio e deve aceitar exclusivamente um dos seguintes valores:

- `liceu`
- `tecnico`

O campo `modelo` deve estar disponível apenas para cursos médios. Cursos de outros tipos, como `superior`, não devem aceitar, persistir nem expor esse campo.

## Contexto e motivação

Atualmente, os cursos médios são identificados pelo nível/tipo do curso, mas não há uma distinção explícita entre cursos médios no modelo liceal e cursos médios técnicos. Essa distinção é necessária para permitir regras, filtros, relatórios, validações e futuras evoluções funcionais específicas para cada modelo.

A implementação deve seguir o padrão já existente no aggregate de cursos, incluindo eventos, projeções, DTOs, validações, handlers, documentação e testes.

## Regra de negócio a implementar

### Novo campo em cursos médios

Adicionar aos cursos de `type='medio'` o campo `modelo` com o seguinte formato conceitual:

```json
{
  "nome": "Ciências Físicas e Biológicas",
  "type": "medio",
  "modelo": "liceu",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"]
}
```

Exemplo para curso médio técnico:

```json
{
  "nome": "Técnico de Informática",
  "type": "medio",
  "modelo": "tecnico",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"]
}
```

### Valores permitidos

O campo `modelo` deve aceitar somente os valores abaixo:

| Valor | Significado |
| --- | --- |
| `liceu` | Curso médio liceal/geral |
| `tecnico` | Curso médio técnico |

Qualquer outro valor deve ser rejeitado com erro de validação claro.

Exemplos de valores inválidos:

- `tecnologico`
- `profissional`
- `geral`
- `LICEU`
- `TÉCNICO`
- string vazia
- `null`, quando o curso for médio

### Escopo do campo

- `modelo` deve existir apenas para cursos de `type='medio'`.
- Cursos de `type='superior'` não devem aceitar `modelo` nos payloads de criação ou edição.
- Cursos superiores não devem persistir nem expor `modelo` nas respostas públicas.
- O campo deve estar disponível nos fluxos de criação, edição, leitura, listagem, eventos e projeções de cursos médios, conforme o padrão atual do aggregate de cursos.
- Para cursos médios, `modelo` deve ser obrigatório na criação.
- Para atualização de cursos médios, a ausência de `modelo` deve seguir o padrão atual de atualização do recurso: manter o valor existente em atualizações parciais ou exigir o valor em atualizações completas, conforme o contrato já implementado para cursos.

## Validações obrigatórias

### Criação de curso médio

Ao criar um curso com `type='medio'`:

- `modelo` é obrigatório;
- `modelo` deve ser exatamente `liceu` ou `tecnico`;
- a validação deve ser case-sensitive, salvo se o backend já tiver uma política global explícita de normalização para enums;
- mensagens de erro devem indicar os valores permitidos.

Payload válido:

```json
{
  "nome": "Técnico de Contabilidade",
  "type": "medio",
  "modelo": "tecnico",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"]
}
```

Payload inválido:

```json
{
  "nome": "Técnico de Contabilidade",
  "type": "medio",
  "modelo": "profissional",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"]
}
```

### Criação de curso superior

Ao criar um curso com `type='superior'`:

- payload contendo `modelo` deve ser rejeitado;
- a mensagem de erro deve explicar que `modelo` é exclusivo para cursos médios.

Payload inválido:

```json
{
  "nome": "Engenharia Informática",
  "type": "superior",
  "modelo": "tecnico",
  "periodos": 10
}
```

### Atualização de curso

Ao atualizar um curso médio:

- se `modelo` for enviado, deve ser `liceu` ou `tecnico`;
- se o contrato permitir alteração de `type`, a transição entre tipos deve validar/remover `modelo` de forma explícita e segura;
- se um curso deixar de ser médio, `modelo` não deve permanecer exposto ou inconsistente;
- se um curso passar a ser médio, `modelo` deve ser informado e validado.

Ao atualizar um curso superior:

- payload contendo `modelo` deve ser rejeitado;
- o backend não deve aceitar `modelo` silenciosamente nem simplesmente ignorar o campo sem validação, caso o padrão da API seja rejeitar campos inválidos.

## Contrato de API esperado

### Criação de curso médio

Adicionar `modelo` ao contrato de criação de curso médio.

Exemplo de request:

```json
{
  "nome": "Ciências Econômicas e Jurídicas",
  "type": "medio",
  "modelo": "liceu",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"]
}
```

Exemplo de response:

```json
{
  "id": "uuid-curso",
  "nome": "Ciências Econômicas e Jurídicas",
  "type": "medio",
  "modelo": "liceu",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"],
  "status": "ativo"
}
```

### Listagem e detalhe de cursos médios

As respostas de listagem e detalhe devem expor `modelo` para cursos médios.

Exemplo:

```json
{
  "id": "uuid-curso",
  "nome": "Técnico de Informática",
  "type": "medio",
  "modelo": "tecnico"
}
```

### Cursos superiores

Cursos superiores não devem expor `modelo`.

Exemplo esperado:

```json
{
  "id": "uuid-curso",
  "nome": "Direito",
  "type": "superior"
}
```

## Persistência, eventos e projeções

A implementação deve atualizar todos os pontos necessários do fluxo de cursos:

- comandos de criação e atualização de curso;
- validações de entrada;
- aggregate/domain model de curso;
- eventos de curso criado/atualizado;
- snapshots ou payloads de eventos, se existirem;
- projeções de curso;
- queries de listagem e detalhe;
- migrações de banco de dados;
- serializers/deserializers JSON;
- documentação de API e documentação funcional.

O campo deve ser persistido de forma compatível com o padrão atual do projeto para enums ou strings controladas.

## Migração de dados

Se já existirem cursos médios no banco, a migração deve definir uma estratégia explícita para preencher `modelo`.

Opções aceitáveis:

1. definir um valor padrão temporário para cursos médios existentes, com justificativa clara;
2. criar migração que permita `NULL` inicialmente e depois exigir preenchimento após saneamento;
3. exigir script operacional/manual para classificar cursos existentes antes de ativar a constraint obrigatória.

A estratégia escolhida deve ser documentada no PR e não pode deixar cursos médios existentes em estado inválido sem plano de correção.

## Testes obrigatórios

Criar ou atualizar testes cobrindo, no mínimo:

- criação de curso médio com `modelo='liceu'`;
- criação de curso médio com `modelo='tecnico'`;
- rejeição de curso médio sem `modelo`;
- rejeição de curso médio com `modelo` inválido;
- rejeição de curso superior contendo `modelo`;
- leitura/detalhe de curso médio expondo `modelo`;
- listagem de cursos médios expondo `modelo`;
- curso superior não expondo `modelo`;
- atualização válida de `modelo` em curso médio, se a API permitir essa alteração;
- eventos/projeções preservando o valor de `modelo`.

## Documentação obrigatória

Atualizar, no mínimo:

- `docs/Spuri - Documentação.md`, nas seções que descrevem cursos médios e seus atributos;
- `docs/Spuri - API.md`, nos contratos, exemplos, validações e responses de criação, edição, listagem e detalhe de cursos;
- qualquer outra documentação de cursos que mencione os campos aceitos para `type='medio'` ou `type='superior'`.

A documentação deve deixar explícito que:

- `modelo` é exclusivo de cursos médios;
- `modelo` é obrigatório para cursos médios;
- os únicos valores válidos são `liceu` e `tecnico`;
- cursos superiores não aceitam esse campo.

## Critérios de aceite

A tarefa será considerada concluída quando:

- cursos médios puderem ser criados com `modelo='liceu'` ou `modelo='tecnico'`;
- cursos médios sem `modelo` forem rejeitados;
- cursos médios com `modelo` diferente de `liceu` ou `tecnico` forem rejeitados;
- cursos superiores com `modelo` forem rejeitados;
- respostas públicas de cursos médios exibirem `modelo`;
- respostas públicas de cursos superiores não exibirem `modelo`;
- eventos, projeções e persistência mantiverem o valor corretamente;
- migrações tratarem dados existentes de forma segura;
- documentação funcional e documentação de API estiverem atualizadas;
- testes automatizados cobrirem os cenários principais.

## Fora de escopo

Esta tarefa não deve implementar regras acadêmicas diferentes para cursos `liceu` e `tecnico`, salvo se forem necessárias apenas para validar o novo campo.

Também fica fora de escopo criar novos valores além de `liceu` e `tecnico`.
