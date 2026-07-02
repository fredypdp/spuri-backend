---
modificado: 2026-07-01 00:00
criado: 2026-07-01 00:00
---
# Adicionar `materias_chave` por ano nos cursos médios e remover da regra de avaliação final

## Objetivo

Implementar a configuração de **matérias-chave do ensino médio diretamente no curso médio**, por ano académico do curso, e remover esse dado do contrato e da responsabilidade da **regra de avaliação final**.

A partir desta mudança, a regra de avaliação final de `nivel='medio'` não deve mais aceitar nem persistir `materias_chave`. Durante a execução da avaliação final, as matérias-chave devem ser descobertas a partir do par **curso médio + ano académico atual do estudante**.

Também é obrigatório atualizar a documentação funcional e a documentação de API para refletir o novo modelo, principalmente:

- `docs/Spuri - Documentação.md`, com foco especial na seção **5.6 Avaliação Final de Ano Académico** e em todas as seções que descrevem cursos;
- `docs/Spuri - API.md`, em todos os contratos, exemplos, validações e explicações relacionados a cursos médios e avaliação final.

## Contexto e motivação

Atualmente, o modelo documentado e/ou implementado trata `materias_chave` como parte da configuração da regra raiz de avaliação final do ensino médio. Isso acopla uma característica curricular do curso à regra de cálculo da avaliação.

O comportamento desejado é separar responsabilidades:

- **curso médio** define quais matérias são chave em cada ano académico ofertado;
- **regra de avaliação final** define como calcular, aprovar, reprovar, acionar descendentes e lidar com pendências;
- **execução da avaliação final** combina a regra aplicável com as matérias-chave configuradas no curso/ano atual do estudante.

Essa separação evita duplicação de configuração, reduz risco de regras inconsistentes entre cursos ou anos diferentes e permite que mudanças curriculares do curso sejam documentadas e auditadas no lugar correto.

## Regra de negócio a implementar

### Novo campo em cursos médios

Adicionar aos cursos de `type='medio'` o campo público e persistente `materias_chave` com o seguinte formato:

```json
[
  {
    "ano_academico": "1_ano_medio",
    "materias_chave": ["uuid-materia-1", "uuid-materia-2"]
  },
  {
    "ano_academico": "2_ano_medio",
    "materias_chave": ["uuid-materia-3", "uuid-materia-4"]
  }
]
```

Cada item representa a configuração de matérias-chave de um ano académico específico daquele curso médio.

Campos de cada item:

- `ano_academico`: string com o ano académico médio do curso, por exemplo `1_ano_medio`, `2_ano_medio`, `3_ano_medio`;
- `materias_chave`: array de IDs das matérias disciplinares consideradas chave naquele ano do curso.

### Escopo do campo

- `materias_chave` deve existir apenas para cursos de `type='medio'`.
- Cursos de `type='superior'` não devem aceitar `materias_chave`.
- O campo deve estar disponível nos fluxos de criação, edição, leitura, listagem, projeção e eventos de cursos médios, conforme o padrão atual do aggregate de cursos.
- Quando um curso médio não possuir matérias-chave configuradas para determinado ano, o comportamento deve ser validado explicitamente: ou a criação/edição deve impedir essa lacuna quando o ano existir, ou a avaliação final deve falhar com erro claro de configuração ausente. A decisão deve ser consistente, testada e documentada.

### Validações obrigatórias em cursos médios

Para cada entrada de `materias_chave` em um curso médio:

- `ano_academico` deve pertencer a `anos_academicos` do próprio curso;
- não deve existir mais de uma entrada para o mesmo `ano_academico`;
- todos os IDs em `materias_chave` devem corresponder a matérias disciplinares existentes, ativas, de `type='medio'`, pertencentes à mesma academia, vinculadas ao mesmo `curso_id` e aplicáveis ao `ano_academico` informado;
- não deve haver IDs duplicados dentro da lista de `materias_chave` do mesmo ano;
- matérias de outro curso, outra academia, outro nível, outro ano académico, inativas, deletadas ou inexistentes devem ser rejeitadas com erro de validação claro;
- alterações em `anos_academicos` do curso devem manter `materias_chave` coerente, removendo ou rejeitando configurações órfãs de anos que deixaram de existir, conforme a estratégia adotada;
- se o sistema permitir curso médio sem matérias-chave durante cadastro inicial, deve existir validação operacional antes da execução da avaliação final para impedir decisão silenciosa incorreta.

### Remoção de `materias_chave` da regra de avaliação final

A partir desta tarefa, `materias_chave` **não deve mais ser aceito** em regras de avaliação final.

Regras esperadas:

- `POST /academia/avaliacao-final/regras` deve rejeitar payload contendo `materias_chave`;
- `PUT /academia/avaliacao-final/regras/:id` deve rejeitar payload contendo `materias_chave`;
- respostas de criação, leitura e listagem de regras não devem expor `materias_chave`;
- DTOs, validações, eventos, projeções, testes e documentação de regras devem remover `materias_chave`;
- não deve existir alias, compatibilidade silenciosa nem migração conceitual de `materias_chave` da regra para o curso no momento da execução;
- a mensagem de erro deve orientar que matérias-chave do médio agora são configuradas no curso médio, por ano académico.

### Execução da avaliação final do ensino médio

Durante a avaliação final de estudante do ensino médio:

1. identificar o estudante avaliado;
2. obter `curso_medio_id` do estudante;
3. obter `ano_escolar_medio` atual do estudante;
4. carregar o curso médio correspondente;
5. localizar no curso a entrada de `materias_chave` cujo `ano_academico` seja igual ao `ano_escolar_medio` do estudante;
6. usar essa lista como conjunto de matérias-chave da decisão de aprovação direta e demais validações aplicáveis;
7. continuar usando a regra de avaliação final apenas para fórmula, nota mínima, regra descendente, limite de pendências e demais parâmetros próprios da avaliação.

A decisão geral para o médio deve continuar respeitando o modelo por matéria:

- calcular a avaliação final por matéria aplicável do curso/ano;
- usar as matérias-chave do curso/ano para a decisão de aprovação direta do médio;
- acionar regras descendentes quando houver matéria abaixo da nota mínima;
- avaliar pendências conforme `limite_materias_pendentes`, `pendencia_permitida` e `pendencia_nivel_conclusao`;
- registrar snapshots suficientes para auditoria, incluindo quais matérias-chave foram usadas e de onde foram obtidas.

### Auditoria e snapshots

Eventos e projeções relacionados à avaliação final devem permitir auditar que a decisão foi tomada com base nas matérias-chave configuradas no curso no momento da execução.

Sempre que o modelo atual permitir, registrar no snapshot da avaliação final:

- `curso_id` usado;
- `ano_academico` usado;
- lista de `materias_chave` resolvida para aquele curso/ano;
- versão/dados suficientes do curso ou do evento de curso que originou a configuração, se o sistema já tiver padrão equivalente;
- lista de matérias avaliadas e resultado por matéria.

O objetivo é evitar que uma alteração futura no curso torne impossível explicar por que determinada avaliação final aprovou, reprovou ou aprovou com pendência um estudante.

## Contrato de API esperado

### Cursos médios

Atualizar os contratos de criação, edição, leitura e listagem de cursos para incluir `materias_chave` somente em cursos médios.

Exemplo conceitual de criação/edição de curso médio:

```json
{
  "nome": "Ciências Econômicas e Jurídicas",
  "type": "medio",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"],
  "materias_chave": [
    {
      "ano_academico": "1_ano_medio",
      "materias_chave": ["uuid-portugues", "uuid-matematica"]
    },
    {
      "ano_academico": "2_ano_medio",
      "materias_chave": ["uuid-direito", "uuid-economia"]
    }
  ]
}
```

A resposta pública do curso médio deve expor a configuração persistida em formato equivalente.

### Cursos superiores

Payloads de curso superior contendo `materias_chave` devem ser rejeitados com erro de validação claro, porque matérias-chave por ano são um recurso exclusivo do ensino médio.

### Regras de avaliação final

Atualizar o contrato de regras para deixar claro que `materias_chave` não faz mais parte da regra.

Exemplo conceitual de payload que deve ser rejeitado:

```json
{
  "nivel": "medio",
  "type": "avaliacao_final",
  "formula": "...",
  "nota_minima_aprovacao": 10,
  "limite_materias_pendentes": 2,
  "materias_chave": ["uuid-materia"]
}
```

A resposta deve indicar que `materias_chave` deve ser configurado no curso médio, por `ano_academico`.

## Documentação obrigatória

### `docs/Spuri - Documentação.md`

Atualizar a documentação funcional para refletir que matérias-chave do médio pertencem ao curso, não à regra de avaliação final.

A atualização deve cobrir, no mínimo:

- seção **5.6 Avaliação Final de Ano Académico**, especialmente toda descrição do fluxo do ensino médio;
- explicações sobre criação, edição e finalidade de cursos médios;
- qualquer tabela ou lista de campos de curso que precise incluir `materias_chave`;
- qualquer trecho que diga ou sugira que a regra de avaliação final média possui `materias_chave`;
- exemplos de configuração de cursos médios;
- exemplos de montagem de regra de avaliação final média, agora sem `materias_chave`;
- explicação de que a execução da avaliação final busca matérias-chave pelo `curso_medio_id` e `ano_escolar_medio` do estudante;
- cenários de erro por curso médio sem configuração de matérias-chave para o ano do estudante;
- impacto na auditoria e nos snapshots da avaliação final.

### `docs/Spuri - API.md`

Atualizar a documentação de API para refletir o contrato público novo.

A atualização deve cobrir, no mínimo:

- tipos globais relacionados a curso;
- schema/request/response de criação de curso;
- schema/request/response de atualização de curso;
- schema/response de leitura e listagem de cursos;
- validações e erros de cursos médios;
- validações e erros de cursos superiores quando receberem `materias_chave`;
- schema/request/response de regras de avaliação final;
- exemplos de regras médias, removendo `materias_chave`;
- erros de validação de regra quando `materias_chave` for enviado;
- seção de avaliação final automática, explicando que as matérias-chave vêm do curso/ano do estudante.

## Arquivos e áreas de código a investigar

Antes de implementar, investigar no mínimo:

- aggregate, eventos e validações de cursos;
- handlers e DTOs de criação, edição, leitura, listagem e batch/async de cursos;
- projeção de cursos;
- migrations e schema das projeções de cursos;
- testes de cursos;
- handlers, DTOs, aggregate, projeção e migrations de regras de avaliação final;
- executor/serviço/handler de avaliação final automática;
- testes de avaliação final, regras descendentes, pendências e matérias-chave;
- documentação funcional e documentação de API.

## Testes obrigatórios

Criar ou atualizar testes cobrindo, no mínimo:

### Cursos médios

- criação de curso médio com `materias_chave` válida por ano;
- edição de curso médio alterando `materias_chave`;
- rejeição de `materias_chave` com ano fora de `anos_academicos`;
- rejeição de matéria inexistente;
- rejeição de matéria de outro curso;
- rejeição de matéria de outra academia;
- rejeição de matéria de outro nível;
- rejeição de matéria que não pertence ao ano informado;
- rejeição de IDs duplicados;
- comportamento ao remover ano académico que possui configuração de matérias-chave.

### Cursos superiores

- rejeição de criação de curso superior com `materias_chave`;
- rejeição de edição de curso superior com `materias_chave`.

### Regras de avaliação final

- rejeição de criação de regra contendo `materias_chave`;
- rejeição de edição de regra contendo `materias_chave`;
- ausência de `materias_chave` nas respostas de regra;
- manutenção do comportamento correto de regras descendentes e pendências sem depender de `materias_chave` na regra.

### Execução da avaliação final média

- avaliação final do médio busca matérias-chave pelo curso e ano atual do estudante;
- estudante em curso A usa matérias-chave do curso A, mesmo que exista regra média compartilhada;
- estudante em ano diferente do mesmo curso usa a lista correspondente ao próprio ano;
- erro claro quando o curso não possui configuração de matérias-chave para o ano do estudante;
- snapshots da avaliação registram a lista de matérias-chave usada;
- alteração posterior do curso não altera a auditoria de avaliação já registrada.

## Critérios de aceite

- Cursos médios possuem campo `materias_chave` por ano académico no formato especificado.
- Cursos superiores rejeitam `materias_chave`.
- Regras de avaliação final deixam de aceitar, persistir ou expor `materias_chave`.
- Avaliação final média resolve matérias-chave exclusivamente pelo curso e ano atual do estudante.
- Validações impedem configurações incoerentes entre curso, ano, matéria, academia e nível.
- Eventos/projeções/snapshots preservam dados suficientes para auditoria.
- `docs/Spuri - Documentação.md` é atualizado, especialmente em **5.6 Avaliação Final de Ano Académico** e nas seções relacionadas a cursos.
- `docs/Spuri - API.md` é atualizado em todos os contratos e exemplos afetados.
- Testes automatizados cobrem os cenários principais e regressões do contrato antigo.
- Não há menções documentais ou contratuais indicando que `materias_chave` ainda pertence à regra de avaliação final.
