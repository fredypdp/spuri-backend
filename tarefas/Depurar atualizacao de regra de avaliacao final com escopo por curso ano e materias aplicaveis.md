---
modificado: 2026-07-02 00:00
criado: 2026-07-02 00:00
---
# Depurar atualização de regra de avaliação final com escopo por curso/ano e matérias aplicáveis por ano

## Objetivo da auditoria

Fazer uma auditoria crítica, completa e arquivo por arquivo da implementação da tarefa:

`tarefas/Atualizar regra de avaliacao final com escopo por curso ano e materias aplicaveis.md`

A auditoria deve confirmar se a implementação foi feita corretamente, completamente e **à risca**. Caso qualquer parte esteja incompleta, inconsistente, parcialmente implementada, sem validação, sem teste, sem documentação, com comportamento silencioso incorreto ou divergente do contrato esperado, esta tarefa exige **instruir o ajuste e implementar o que falta**.

Esta funcionalidade é crítica porque altera o contrato público e persistente das **regras de avaliação final**, substituindo escopos globais simples por escopos coerentes com **nível**, **curso**, **ano académico** e **matérias aplicáveis daquele escopo**. A execução automática da avaliação final deve continuar por matéria, com regras descendentes, pendências, auditoria e uso de `nivel`, mas agora precisa selecionar regras e filtros de matérias de acordo com o escopo correto.

## Regra adicional obrigatória

Além da especificação original, é obrigatório garantir que a atualização não introduziu compatibilidade silenciosa indevida com o contrato antigo:

- regra `fundamental` deve aceitar `anos_academicos` apenas como array simples de strings;
- regra `medio` deve aceitar `anos_academicos` apenas como lista por `curso_id` + `anos_academicos`;
- regra `superior` deve rejeitar `anos_academicos` no payload público;
- `materias_aplicaveis` não pode mais ser lista simples global de IDs em nenhum nível;
- qualquer formato incompatível por nível deve falhar com erro claro, determinístico e auditável;
- a unicidade de regras ativas não pode depender apenas de validação em memória, pois precisa ser segura contra concorrência;
- dados legados devem ter estratégia explícita de migração/replay/compatibilidade, sem inferência silenciosa de curso ou ano quando isso puder gerar decisão acadêmica incorreta.

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. contratos públicos, DTOs e validações de regras de avaliação final;
2. handlers de criação, edição, leitura, listagem, ativação/inativação e execução de regras;
3. modelos de domínio, aggregates, eventos e snapshots de regra e avaliação final;
4. projeções de regras e avaliações finais;
5. migrations, schemas, constraints e índices relacionados a regras/escopos;
6. consultas de seleção de regra ativa por estudante, nível, curso, ano e tipo;
7. executor/serviço/handler de avaliação final automática;
8. regras descendentes, pendências e progressão acadêmica;
9. validação de cursos, matérias e anos académicos usados no escopo;
10. testes unitários, integração/handler, projeção, migração e regressão;
11. documentação funcional e documentação de API;
12. qualquer fluxo alternativo que persista ou consuma regra de avaliação final.

Também auditar, no mínimo, os arquivos citados na tarefa original:

- `internal/handlers/avaliacao_final_regras.go`;
- `internal/handlers/avaliacao_final_handler.go`;
- `internal/handlers/avaliacao_final_regras_test.go`;
- `internal/handlers/avaliacao_final_formula_test.go`;
- `internal/projections/avaliacao_final_projection.go`;
- `internal/projections/avaliacao_final_projection_test.go`;
- `internal/domain/models.go`;
- `internal/domain/aggregates/estudante_avaliacao.go`;
- `internal/domain/aggregates/estudante_avaliacao_test.go`;
- migrações relacionadas a regras/avaliações finais;
- `docs/Spuri - Documentação.md`, seção **5.6 Avaliação Final de Ano Académico**;
- `docs/Spuri - API.md`, seção **15. Avaliações Finais**.

## Checklist obrigatório de validação

### 1. Busca ampla e classificação de ocorrências

Fazer busca ampla no repositório por:

- `anos_academicos`;
- `anosAcademicos`;
- `materias_aplicaveis`;
- `materiasAplicaveis`;
- `avaliacao_final_regras`;
- `RegraAvaliacaoFinal`;
- `tipo_ensino`;
- `tipoEnsino`;
- `nivel`;
- `curso_id`;
- `cursoId`;
- `ano_academico`;
- `anoAcademico`;
- `materia_id`;
- `materiaId`.

Não basta listar ocorrências. Cada ocorrência relevante deve ser classificada como:

- implementação correta do novo contrato;
- compatibilidade/migração legada necessária e documentada;
- documentação histórica aceitável;
- bug ativo a corrigir;
- teste cobrindo regressão;
- código morto a remover.

### 2. Contrato público de `anos_academicos`

Confirmar e, se necessário, implementar que o payload público respeita exatamente o contrato por nível.

#### Fundamental

Validar que regra `nivel="fundamental"`:

- exige `anos_academicos` como array simples de strings;
- rejeita array vazio;
- rejeita item que não seja string;
- rejeita estrutura por curso;
- rejeita `curso_id` no escopo fundamental;
- rejeita ano fundamental inválido;
- rejeita ano não ofertado pela academia autenticada;
- rejeita ano duplicado;
- preserva o formato simples em evento, projeção, resposta, replay e documentação.

#### Médio

Validar que regra `nivel="medio"`:

- exige `anos_academicos` como lista de objetos por curso;
- cada item possui `curso_id` válido, ativo, não deletado, médio e da academia autenticada;
- cada item possui `anos_academicos` não vazio;
- rejeita formato antigo de array simples;
- rejeita item sem `curso_id`;
- rejeita item sem `anos_academicos`;
- rejeita dois itens com o mesmo `curso_id`;
- rejeita ano duplicado dentro do mesmo item;
- rejeita ano que não pertence ao curso médio informado;
- persiste, projeta, expõe e documenta o novo formato.

#### Superior

Validar que regra `nivel="superior"`:

- rejeita qualquer envio de `anos_academicos` no payload público;
- mantém o escopo superior resolvido por curso/período/semestre do estudante e das matérias;
- não documenta exemplos sugerindo envio de `anos_academicos` para superior;
- não possui fallback oculto para anos acadêmicos superiores em regra.

### 3. Contrato público de `materias_aplicaveis`

Confirmar e, se necessário, implementar que `materias_aplicaveis` usa apenas o novo formato por escopo.

#### Fundamental

Validar que cada item possui:

```json
{
  "ano_academico": "1_ano_fundamental",
  "materias": ["id_materia_1", "id_materia_2"]
}
```

E confirmar que:

- a unicidade do item depende somente de `ano_academico`;
- `ano_academico` precisa estar coberto por `anos_academicos` da regra;
- matérias existem, estão ativas, não deletadas, pertencem à academia e ao nível fundamental;
- cada matéria contém o `ano_academico` informado em seu próprio escopo;
- matéria fora do ano informado é rejeitada;
- matéria duplicada dentro do item é rejeitada;
- lista simples global de IDs é rejeitada com mensagem clara.

#### Médio

Validar que cada item possui:

```json
{
  "curso_id": "id do curso médio",
  "ano_academico": "1_ano_medio",
  "materias": ["id_materia_1", "id_materia_2"]
}
```

E confirmar que:

- a unicidade do item depende do par `curso_id` + `ano_academico`;
- `curso_id` existe, está ativo, não deletado, pertence à academia e é curso médio;
- `ano_academico` pertence ao curso médio informado;
- o par `curso_id` + `ano_academico` está coberto por `anos_academicos` da regra;
- matérias existem, estão ativas, não deletadas, pertencem à academia, são de nível médio, pertencem ao mesmo curso e ao mesmo ano;
- matéria fora do curso ou fora do ano informado é rejeitada;
- matéria duplicada dentro do item é rejeitada;
- lista simples global de IDs é rejeitada.

#### Superior

Validar que cada item possui:

```json
{
  "curso_id": "id do curso superior",
  "ano_academico": "1_ano_superior",
  "materias": ["id_materia_1", "id_materia_2"]
}
```

E confirmar que:

- a unicidade do item depende do par `curso_id` + `ano_academico`;
- `curso_id` existe, está ativo, não deletado, pertence à academia e é curso superior;
- `ano_academico` é válido para o curso superior conforme o modelo de períodos/semestres existente;
- a validação mapeia corretamente ano superior para períodos/semestres;
- matérias existem, estão ativas, não deletadas, pertencem à academia, são de nível superior, pertencem ao mesmo curso e estão em período compatível com o ano informado;
- matéria fora do curso/período/ano derivado é rejeitada;
- matéria duplicada dentro do item é rejeitada;
- lista simples global de IDs é rejeitada.

### 4. Unicidade e concorrência de regras ativas

Confirmar e, se necessário, implementar que não existe mais de uma regra ativa para a mesma academia, `nivel` e `type` com escopo sobreposto.

Validar que:

- regras fundamentais conflitantes são detectadas quando compartilham pelo menos um mesmo `ano_academico`;
- regras médias conflitantes são detectadas quando compartilham pelo menos um mesmo par `curso_id` + `ano_academico`;
- regras fundamentais com anos disjuntos podem coexistir;
- regras médias com cursos/anos disjuntos podem coexistir;
- regras superiores preservam a unicidade coerente com o modelo atual sem `anos_academicos`;
- a validação ocorre em criação, edição, ativação/re-ativação e replay/projeção quando aplicável;
- a solução é segura contra corrida concorrente, preferencialmente por constraint/índice/tabela auxiliar transacional;
- não há dependência exclusiva de comparação em memória no handler;
- mensagens de conflito indicam claramente o escopo conflitante.

### 5. Regras descendentes

Auditar e corrigir a compatibilidade de escopo das regras descendentes.

Confirmar que:

- não é possível criar descendente órfã;
- não é possível criar ciclo;
- descendente não pode ter `nivel` incompatível com a ascendente;
- descendente não pode ter `type`/encadeamento incompatível;
- a decisão de design está explícita: descendente deve ter mesmo escopo da ascendente ou pode ter subconjunto;
- se subconjunto for permitido, fundamental não declara ano fora da ascendente e médio não declara par `curso_id` + `ano_academico` fora da ascendente;
- `materias_aplicaveis` da descendente é subconjunto coerente do escopo da descendente e da cadeia;
- matéria fora do escopo ascendente não é recalculada por descendente;
- testes cobrem descendente válida, descendente fora de escopo, ciclo e dependência inativa.

### 6. Persistência, eventos, projeções e replay

Confirmar que o novo contrato atravessa persistência e leitura sem perda de informação.

Validar que:

- eventos de criação/edição de regra registram os novos formatos;
- snapshots de regra e avaliação final são compatíveis com o novo modelo;
- projeções armazenam e expõem `anos_academicos` e `materias_aplicaveis` nos formatos corretos;
- responses de criação, detalhe e listagem retornam o formato correto por nível;
- replay de eventos antigos não quebra;
- regras fundamentais antigas permanecem como array simples;
- regras médias antigas têm estratégia explícita, sem inventar `curso_id` silenciosamente;
- migrations são idempotentes conforme o padrão do projeto;
- schema JSON/JSONB, defaults, constraints e índices são adequados;
- não há dados inconsistentes sendo aceitos silenciosamente pela execução da avaliação final.

### 7. Execução da avaliação final automática

Confirmar que a execução da avaliação final usa o novo escopo corretamente.

#### Fundamental

Validar que:

- a regra aplicável é selecionada pelo `ano_escolar_fundamental` do estudante;
- o ano do estudante precisa estar coberto por `anos_academicos` da regra;
- sem `materias_aplicaveis`, todas as matérias disciplinares aplicáveis ao ano são avaliadas;
- com `materias_aplicaveis`, somente matérias do item daquele `ano_academico` são avaliadas/recalculadas;
- matéria fora do ano não entra no cálculo;
- aprovação/reprovação geral deriva dos resultados por matéria.

#### Médio

Validar que:

- a regra aplicável é selecionada pelo curso médio atual do estudante e `ano_escolar_medio`;
- o par `curso_id` + `ano_academico` precisa estar coberto por `anos_academicos` da regra;
- sem `materias_aplicaveis`, todas as matérias disciplinares aplicáveis ao curso/ano são avaliadas;
- com `materias_aplicaveis`, somente matérias do item daquele curso/ano são avaliadas/recalculadas;
- matérias de outro curso ou outro ano não interferem;
- matérias-chave do curso médio continuam sendo resolvidas pelo curso/ano do estudante, e não pela regra;
- pendências e regras descendentes continuam funcionando no escopo correto.

#### Superior

Validar que:

- a regra aplicável respeita curso superior e período/semestre atual do estudante conforme modelo existente;
- `ano_academico` de `materias_aplicaveis` é derivado corretamente dos períodos/semestres;
- sem `materias_aplicaveis`, todas as matérias disciplinares aplicáveis ao curso/período são avaliadas;
- com `materias_aplicaveis`, somente matérias do item daquele curso/ano derivado são avaliadas/recalculadas;
- fórmula superior continua usando período inferido pela matéria avaliada;
- pendências e progressão continuam funcionando corretamente.

### 8. Auditoria da avaliação final

Confirmar que eventos/projeções/snapshots da avaliação final permitem reconstruir a decisão tomada.

O snapshot deve registrar, sempre que o modelo permitir:

- regra de avaliação final usada;
- `nivel` usado;
- curso usado, quando aplicável;
- ano académico/período/semestre usado;
- escopo da regra resolvido no momento da avaliação;
- filtro de `materias_aplicaveis` efetivamente aplicado;
- lista completa de matérias avaliadas;
- resultado por matéria;
- regra descendente usada por matéria, quando aplicável;
- pendências criadas, mantidas ou baixadas;
- dados suficientes para que alteração posterior da regra não altere a explicação histórica da avaliação registrada.

### 9. API, erros e documentação viva

Auditar todos os endpoints relacionados, especialmente:

- `POST /academia/avaliacao-final/regras`;
- `PUT /academia/avaliacao-final/regras/:id`;
- listagem/leitura de regras;
- ativação/inativação de regras;
- execução automática por lançamento de nota;
- consulta e regularização de pendências, quando consumirem regra/escopo.

Confirmar que:

- payloads, responses, filtros, DTOs e documentação usam `nivel`;
- `tipo_ensino` continua rejeitado, sem alias ou tradução silenciosa;
- campos incompatíveis com nível geram erro claro;
- erros possuem `field`, `code` e mensagem útil quando esse for o padrão do projeto;
- exemplos da API usam o novo formato correto por nível;
- não existe documentação conflitante com o novo modelo.

### 10. Testes obrigatórios

Criar ou ajustar testes cobrindo, no mínimo:

#### `anos_academicos`

- criação de regra fundamental com array simples válido;
- rejeição de regra fundamental com estrutura por curso;
- rejeição de regra fundamental com ano duplicado;
- rejeição de regra fundamental com ano não ofertado pela academia;
- criação de regra média com lista por curso válida;
- rejeição de regra média com array simples antigo;
- rejeição de regra média com `curso_id` ausente;
- rejeição de regra média com curso inexistente, inativo, deletado, de outra academia ou de nível diferente;
- rejeição de regra média com ano duplicado dentro do item;
- rejeição de regra média com dois itens para o mesmo `curso_id`;
- rejeição de regra média com ano não pertencente ao curso;
- rejeição de `anos_academicos` em regra superior.

#### `materias_aplicaveis`

- criação de filtro fundamental por `ano_academico` válido;
- rejeição de filtro fundamental com duplicidade de `ano_academico`;
- rejeição de matéria fundamental fora do ano;
- criação de filtro médio por `curso_id` + `ano_academico` válido;
- rejeição de filtro médio duplicado por par `curso_id` + `ano_academico`;
- rejeição de matéria média fora do curso;
- rejeição de matéria média fora do ano;
- criação de filtro superior por `curso_id` + `ano_academico` derivado válido;
- rejeição de filtro superior duplicado por par `curso_id` + `ano_academico`;
- rejeição de matéria superior fora do curso/período/ano derivado;
- rejeição de lista simples global de IDs em todos os níveis;
- rejeição de matéria duplicada dentro do mesmo item.

#### Unicidade

- conflito de regra fundamental ativa por mesmo ano acadêmico;
- ausência de conflito fundamental com anos disjuntos;
- conflito de regra média ativa por mesmo `curso_id` + `ano_academico`;
- ausência de conflito médio com cursos/anos disjuntos;
- conflito detectado em edição;
- conflito detectado em ativação/re-ativação;
- garantia por constraint/índice/tabela auxiliar ou teste equivalente de persistência.

#### Execução

- execução fundamental sem `materias_aplicaveis` avaliando todas as matérias do ano;
- execução fundamental com `materias_aplicaveis` avaliando apenas matérias do item do ano;
- execução média sem `materias_aplicaveis` avaliando todas as matérias do curso/ano;
- execução média com `materias_aplicaveis` avaliando apenas matérias do par curso/ano;
- execução média em curso A não usando filtro de curso B;
- execução média em anos diferentes do mesmo curso usando filtros diferentes;
- execução superior sem `materias_aplicaveis` avaliando todas as matérias do período/ano derivado;
- execução superior com `materias_aplicaveis` avaliando apenas matérias do par curso/ano derivado;
- erro claro quando não há regra ativa para o escopo do estudante;
- regra descendente com escopo incompatível rejeitada;
- snapshot registra o filtro de matérias efetivamente aplicado.

#### Regressões

- `tipo_ensino` rejeitado;
- `materias_chave` não aceito na regra;
- matérias-chave do médio continuam vindo do curso médio por ano;
- pendências continuam respeitando limite e curso/ano/período;
- replay/projeção/migração de regras existentes conforme estratégia adotada;
- documentação de API não contém exemplos do contrato antigo incompatível.

### 11. Documentação obrigatória

Auditar e corrigir:

- `docs/Spuri - Documentação.md`, seção **5.6 Avaliação Final de Ano Académico**;
- `docs/Spuri - API.md`, seção **15. Avaliações Finais**.

A documentação deve deixar explícito:

- formato de `anos_academicos` por nível;
- fundamental preserva array simples de strings;
- médio usa lista por curso com `curso_id` e `anos_academicos`;
- superior rejeita `anos_academicos` nesta mudança;
- formato de `materias_aplicaveis` por nível;
- unicidade por ano simples no fundamental;
- unicidade por `curso_id` + `ano_academico` no médio;
- exemplos de criação de regra fundamental, média e superior;
- exemplos de regra descendente com escopo compatível;
- erros esperados de duplicidade, conflito, curso inválido, ano fora do curso e matéria fora do escopo;
- impacto na execução da avaliação final por matéria;
- como o frontend deve montar payloads por nível;
- que `materias_chave` do médio pertencem ao curso médio, não à regra.

Remover ou reescrever qualquer trecho que ainda afirme que:

- `anos_academicos` é sempre array simples para todos os níveis;
- médio nunca usa `anos_academicos` na regra;
- superior aceita `anos_academicos` nesta mudança;
- `materias_aplicaveis` é lista simples global de IDs;
- unicidade da regra ignora curso/ano por item.

## Critérios de aceite

A auditoria só pode ser considerada concluída quando todos os itens abaixo forem verdadeiros:

- contrato público novo está implementado e documentado;
- formatos incompatíveis por nível são rejeitados com erro claro;
- fundamental preserva `anos_academicos` como array simples;
- médio usa `anos_academicos` por `curso_id` + anos;
- superior rejeita `anos_academicos`;
- `materias_aplicaveis` usa o novo formato por escopo em todos os níveis;
- unicidade de regra ativa considera ano simples no fundamental e par `curso_id` + `ano_academico` no médio;
- a estratégia de unicidade é segura contra concorrência;
- regras descendentes têm compatibilidade de escopo validada e documentada;
- execução da avaliação final respeita o novo escopo e filtro de matérias;
- snapshots/auditoria registram o escopo e filtro efetivamente usados;
- migrações/replay tratam dados antigos sem inferência silenciosa perigosa;
- testes automatizados cobrem os cenários críticos e regressões do contrato antigo;
- documentação funcional e API refletem o novo modelo sem ambiguidade;
- não há ocorrência ativa de `materias_aplicaveis` como lista simples global nem de `anos_academicos` médio no formato antigo.

## Resultado esperado da execução desta tarefa

Ao finalizar esta tarefa, produzir um resumo técnico informando:

- arquivos auditados;
- ocorrências relevantes classificadas;
- problemas encontrados;
- correções implementadas;
- testes adicionados/alterados;
- comandos executados;
- eventuais decisões de design tomadas;
- estratégia adotada para migração/replay e unicidade concorrente;
- confirmação explícita de que a avaliação final está usando o escopo correto por nível, curso, ano e matérias aplicáveis.
