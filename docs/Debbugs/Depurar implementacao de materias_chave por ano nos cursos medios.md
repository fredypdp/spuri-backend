---
modificado: 2026-07-02 1:01
criado: 2026-07-01 00:00
---
# Depurar implementação de `materias_chave` por ano nos cursos médios

## Objetivo da auditoria

Fazer uma auditoria crítica, completa e arquivo por arquivo da implementação da tarefa:

Tarefa: [[Adicionar materias_chave por ano nos cursos medios]]

A auditoria deve confirmar se a implementação foi feita corretamente, completamente e **à risca**. Caso qualquer parte esteja incompleta, inconsistente, parcialmente implementada, sem validação, sem teste, sem documentação, com comportamento silencioso incorreto ou divergente do contrato esperado, esta tarefa exige **instruir o ajuste e implementar o que falta**.

Esta funcionalidade é crítica porque as matérias-chave deixam de pertencer à regra de avaliação final e passam a ser uma responsabilidade curricular do **curso médio**, configurada por **ano académico**. A avaliação final do ensino médio deve usar exatamente a configuração do par **curso médio + ano académico atual do estudante**.

## Regra adicional obrigatória

Além da especificação original, é obrigatório garantir que:

- todo curso de `type='medio'` tenha **pelo menos uma `materia_chave` para cada ano académico do curso**;
- não pode existir curso médio com ano académico sem configuração correspondente em `materias_chave`;
- não pode existir entrada de `materias_chave` com lista vazia;
- criação, edição, batch, importação assíncrona ou qualquer outro fluxo que crie/atualize curso médio deve respeitar essa regra;
- a avaliação final não deve compensar silenciosamente curso médio mal configurado: a configuração incompleta deve ser impedida na escrita e, se dados legados/inconsistentes forem encontrados, a execução deve falhar com erro claro e auditável.

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. aggregate, eventos e validações de cursos;
2. handlers e DTOs de criação, edição, leitura, listagem e batch/async de cursos;
3. projeção de cursos;
4. migrations e schema das projeções de cursos;
5. testes de cursos;
6. handlers, DTOs, aggregate, projeção e migrations de regras de avaliação final;
7. executor/serviço/handler de avaliação final automática;
8. testes de avaliação final, regras descendentes, pendências e matérias-chave;
9. documentação funcional e documentação de API;
10. qualquer fluxo alternativo que persista curso, regra de avaliação final ou avaliação final.

## Checklist obrigatório de validação

### 1. Modelo público e persistente de cursos médios

Confirmar que cursos de `type='medio'` possuem campo público e persistente `materias_chave` no formato:

```json
[
  {
    "ano_academico": "1_ano_medio",
    "materias_chave": ["uuid-materia-1", "uuid-materia-2"]
  }
]
```

Validar que:

- o campo é aceito em criação de curso médio;
- o campo é aceito em edição de curso médio;
- o campo é retornado na leitura de curso médio;
- o campo é retornado na listagem de cursos médios;
- o campo é persistido em evento/projeção conforme o padrão atual do aggregate de cursos;
- o campo é preservado em reconstrução/replay de projeção;
- o formato público é consistente entre request, response, evento, projeção e documentação.

### 2. Obrigatoriedade por ano académico do curso médio

Confirmar e, se necessário, implementar que:

- todo ano em `anos_academicos` de um curso médio tenha exatamente uma entrada correspondente em `materias_chave`;
- toda entrada em `materias_chave` referencie um ano presente em `anos_academicos` do próprio curso;
- não haja entradas duplicadas para o mesmo `ano_academico`;
- cada lista `materias_chave` tenha pelo menos um ID;
- ao adicionar um ano académico ao curso médio, a configuração de matérias-chave desse ano seja exigida;
- ao remover um ano académico, a configuração correspondente seja removida ou rejeitada de forma consistente e documentada;
- não seja possível salvar curso médio em estado parcialmente configurado;
- eventuais dados antigos inconsistentes sejam tratados por validação operacional clara, sem decisão silenciosa na avaliação final.

### 3. Validações de matérias-chave

Para cada matéria indicada como chave, confirmar que a validação rejeita, com erro claro:

- ID inexistente;
- ID duplicado dentro da lista do mesmo ano;
- matéria inativa;
- matéria deletada/soft-deleted;
- matéria de outra academia;
- matéria de outro curso;
- matéria sem `curso_id` quando o curso médio exige vínculo;
- matéria de outro nível/type;
- matéria não aplicável ao `ano_academico` informado;
- matéria de ano académico diferente;
- matéria pertencente a curso superior ou fundamental;
- payload que tente configurar `materias_chave` em curso superior.

Também validar que mensagens de erro orientem claramente o operador sobre qual campo, ano ou matéria está inválido.

### 4. Cursos superiores e outros níveis

Confirmar que:

- cursos de `type='superior'` rejeitam `materias_chave` em criação;
- cursos de `type='superior'` rejeitam `materias_chave` em edição;
- responses de curso superior não expõem `materias_chave`, salvo se o padrão da API exigir omissão/zero value documentado;
- não há documentação sugerindo que `materias_chave` exista em cursos superiores;
- qualquer validação genérica de curso não permita bypass por batch, async ou update parcial.

### 5. Remoção total de `materias_chave` da regra de avaliação final

Confirmar que `materias_chave` não pertence mais à regra de avaliação final.

Validar que:

- `POST /academia/avaliacao-final/regras` rejeita payload contendo `materias_chave`;
- `PUT /academia/avaliacao-final/regras/:id` rejeita payload contendo `materias_chave`;
- DTOs de regra não aceitam `materias_chave`;
- eventos de regra não persistem `materias_chave`;
- projeções de regra não expõem `materias_chave`;
- responses de criação, leitura e listagem de regras não expõem `materias_chave`;
- testes garantem que não existe alias, compatibilidade silenciosa, migração conceitual ou fallback do campo antigo da regra para o curso;
- a mensagem de erro orienta que matérias-chave do médio agora são configuradas no curso médio, por `ano_academico`.

Fazer busca ampla por `materias_chave`, `materiasChave`, `matérias-chave`, `materias chave` e analisar cada ocorrência. Não basta listar ocorrências: classificar cada uma como válida, histórica/documental aceitável ou bug ativo.

### 6. Execução da avaliação final do ensino médio

Confirmar que, durante a avaliação final automática de estudante do médio, o backend:

1. identifica o estudante avaliado;
2. obtém o `curso_medio_id` ou curso médio atual do estudante;
3. obtém o `ano_escolar_medio` atual do estudante;
4. carrega o curso médio correspondente;
5. localiza em `curso.materias_chave` a entrada cujo `ano_academico` seja igual ao `ano_escolar_medio`;
6. usa essa lista como conjunto de matérias-chave da decisão de aprovação direta;
7. continua usando a regra de avaliação final apenas para fórmula, nota mínima, regras descendentes, limite de pendências e demais parâmetros próprios da avaliação;
8. falha com erro claro se o curso/ano não possuir configuração válida, especialmente em dados antigos inconsistentes.

Confirmar também que:

- estudante em curso A usa matérias-chave do curso A, mesmo que exista regra média compartilhada;
- estudante em ano diferente do mesmo curso usa a lista correspondente ao próprio ano;
- matérias-chave de outro curso não interferem na decisão;
- a decisão geral do médio não depende mais de `materias_chave` na regra;
- regras descendentes e pendências continuam funcionando sem depender de `materias_chave` na regra.

### 7. Auditoria e snapshots

Confirmar que eventos/projeções/snapshots da avaliação final permitem auditar a decisão tomada.

O snapshot da avaliação final deve registrar, sempre que o modelo permitir:

- `curso_id` usado;
- `ano_academico` usado;
- lista de `materias_chave` resolvida para aquele curso/ano;
- origem dos dados de curso usada no momento da avaliação;
- lista de matérias avaliadas;
- resultado por matéria;
- regra de avaliação final usada;
- pendências criadas, quando aplicável.

Confirmar que alteração posterior do curso não altera a explicação histórica de uma avaliação já registrada.

### 8. Migrações e consistência de banco

Auditar migrações e schema para garantir que:

- a projeção de cursos consegue persistir `materias_chave` por ano;
- a migração é idempotente conforme o padrão do projeto;
- tipos JSON/JSONB, defaults, constraints e índices são adequados;
- não há quebra de replay de eventos antigos;
- se houver backfill ou normalização, ele não inventa matérias-chave sem validação;
- dados inválidos não passam despercebidos para a avaliação final.

### 9. Testes obrigatórios

Criar ou ajustar testes cobrindo, no mínimo:

#### Cursos médios

- criação de curso médio com `materias_chave` válida para todos os anos do curso;
- edição de curso médio alterando `materias_chave`;
- rejeição de curso médio sem `materias_chave`;
- rejeição de curso médio com ano académico sem entrada em `materias_chave`;
- rejeição de entrada de `materias_chave` com lista vazia;
- rejeição de entrada duplicada para o mesmo `ano_academico`;
- rejeição de entrada cujo `ano_academico` não pertence a `anos_academicos`;
- rejeição de matéria inexistente;
- rejeição de matéria de outro curso;
- rejeição de matéria de outra academia;
- rejeição de matéria de outro nível;
- rejeição de matéria inativa/deletada;
- rejeição de matéria que não pertence ao ano informado;
- rejeição de IDs duplicados dentro da lista;
- comportamento ao adicionar/remover ano académico em curso médio já existente.

#### Cursos superiores

- rejeição de criação de curso superior com `materias_chave`;
- rejeição de edição de curso superior com `materias_chave`;
- ausência do campo em responses, se esse for o contrato adotado.

#### Regras de avaliação final

- rejeição de criação de regra contendo `materias_chave`;
- rejeição de edição de regra contendo `materias_chave`;
- ausência de `materias_chave` nas respostas de regra;
- manutenção de regras descendentes e pendências sem `materias_chave` na regra;
- mensagem de erro apontando para configuração no curso médio.

#### Execução da avaliação final média

- avaliação final do médio busca matérias-chave pelo curso e ano atual do estudante;
- curso A e curso B com regras compartilhadas usam listas diferentes;
- anos diferentes do mesmo curso usam listas diferentes;
- erro claro quando dados inconsistentes não têm configuração para o ano;
- snapshot registra a lista de matérias-chave usada;
- alteração posterior do curso não altera auditoria já registrada.

### 10. Documentação obrigatória

Auditar e corrigir:

- `docs/Spuri - Documentação.md`;
- `docs/Spuri - API.md`.

A documentação deve deixar explícito que:

- matérias-chave do médio pertencem ao curso médio;
- a configuração é por ano académico;
- cada ano académico do curso médio deve ter pelo menos uma matéria-chave;
- regra de avaliação final média não aceita `materias_chave`;
- avaliação final média resolve matérias-chave por curso e ano atual do estudante;
- cursos superiores rejeitam `materias_chave`;
- erros de validação esperados estão documentados;
- exemplos de curso médio incluem `materias_chave` completa para todos os anos;
- exemplos de regra média não incluem `materias_chave`.

## Critérios de aceite

A auditoria só pode ser considerada concluída quando todos os itens abaixo forem verdadeiros:

- Todo curso médio exige `materias_chave` para **todos** os seus anos académicos.
- Cada ano académico de curso médio tem pelo menos uma matéria-chave válida.
- Cursos médios não podem ser criados ou editados com configuração parcial, vazia, duplicada ou fora de escopo.
- Cursos superiores rejeitam `materias_chave`.
- Regras de avaliação final rejeitam `materias_chave` em criação e edição.
- Responses de regras não expõem `materias_chave`.
- Avaliação final média resolve matérias-chave exclusivamente pelo curso médio e ano académico atual do estudante.
- Snapshots/auditoria registram a lista de matérias-chave efetivamente usada.
- Testes automatizados cobrem os cenários críticos e regressões do contrato antigo.
- Documentação funcional e API refletem o novo modelo sem ambiguidade.
- Não há ocorrência ativa de `materias_chave` como responsabilidade da regra de avaliação final.

## Resultado esperado da execução desta tarefa

Ao finalizar esta tarefa, produzir um resumo técnico informando:

- arquivos auditados;
- problemas encontrados;
- correções implementadas;
- testes adicionados/alterados;
- comandos executados;
- eventuais decisões de design tomadas;
- confirmação explícita de que todo curso médio deve ter pelo menos uma matéria-chave por ano académico.
