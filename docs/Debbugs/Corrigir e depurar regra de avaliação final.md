---
modificado: 2026-06-30 21:09
criado: 2026-06-30 21:09
---
Preciso que você faça uma auditoria crítica, completa e arquivo por arquivo da implementação da tarefa:

[[Corrigir regra de avaliacao final automatica por materia e pendencias]]

Essa funcionalidade é crítica para o sistema, porque a avaliação final automática deve calcular e registrar a `nota_final` por matéria disciplinar aplicável, e não como uma nota única/global do estudante. A tarefa também exige regras específicas por nível de ensino, execução de regras descendentes, criação e baixa de matérias pendentes, bloqueio de progressão/conclusão e remoção total do contrato antigo `tipo_ensino` em favor de `nivel`.

Quero que você atue como engenheiro sênior responsável por validação final de uma feature crítica, fazendo uma investigação profunda do código inteiro relacionado a avaliação final, regras de avaliação, notas, matérias, estudantes, progressão acadêmica, eventos, projeções, DTOs, validações, migrações, testes e documentação.

## Objetivo da auditoria

Verificar se a tarefa foi totalmente e corretamente implementada conforme o arquivo de especificação, e caso encontre qualquer problema, inconsistência, lacuna, bug, comportamento parcial, validação ausente, teste ausente ou documentação incompleta, você deve terminar a implementação.

A auditoria deve cobrir, no mínimo:

1. Contrato público e interno de regras de avaliação final.
2. Renomeação completa de `tipo_ensino` para `nivel`.
3. Validações por nível: `fundamental`, `medio` e `superior`.
4. Escopo correto de matérias por nível.
5. Cálculo de `nota_final` por matéria.
6. Registro auditável dos resultados por matéria.
7. Execução de regras descendentes por matéria.
8. Suporte a `materias_chave` no ensino médio.
9. Suporte a `limite_materias_pendentes`.
10. Criação, consulta, bloqueio e baixa de matérias pendentes.
11. Bloqueio por `pendencia_nivel_conclusao`.
12. Avaliação/regularização de matérias pendentes.
13. Idempotência da avaliação final e das pendências.
14. Segurança do parser/fórmula.
15. Migrações e índices.
16. Testes unitários, integração/e2e, contratos e documentação.

## Regras obrigatórias da especificação a validar

### 1. `nivel` deve substituir completamente `tipo_ensino`

Confirme que:

- O campo público da regra é obrigatoriamente `nivel`.
- `tipo_ensino` foi removido do contrato público e interno novo.
- Não existe alias, fallback, compatibilidade silenciosa, tradução automática ou duplicação entre `tipo_ensino` e `nivel`.
- Payloads contendo `tipo_ensino` falham com erro de validação claro.
- Toda regra tem `nivel`.
- O backend preenche `nivel` automaticamente pela academia autenticada quando a academia não é mista.
- Academia mista só pode criar regra para `fundamental` ou `medio`.
- Academia não mista não pode criar regra para nível incompatível.
- Banco, DTOs, schemas, validações, handlers, documentação e testes usam a nova nomenclatura.

Faça busca ampla no repositório por `tipo_ensino`, `tipoEnsino`, `tipo ensino`, `nivel`, `nível`, e analise cada ocorrência. Não basta listar ocorrências: explique se cada uma é aceitável, histórica, migração inevitável, documentação de remoção ou bug ativo.

### 2. Validação de `anos_academicos`

Confirme que:

- `anos_academicos` só é aceito em regra `fundamental`.
- `medio` e `superior` rejeitam `anos_academicos` no payload público.
- No médio, o escopo é resolvido pelo `ano_escolar_medio` do estudante e pelas matérias disciplinares do curso.
- No superior, o escopo é resolvido pelo `periodo` da matéria e pelo período/semestre atual do estudante.

### 3. Fórmula por nível

Confirme que:

- A fórmula continua declarativa e validada pelo parser próprio.
- Fundamental e médio aceitam fórmula com categoria + período conforme contrato existente.
- Superior não aceita período explícito na fórmula.
- Para superior, o backend infere automaticamente o período usando o `periodo` da matéria avaliada.
- Extração de categorias, carregamento de notas e validações funcionam corretamente tanto com período explícito quanto com período inferido.
- Não há uso inseguro de `eval`, execução de código do usuário ou SQL dinâmico inseguro com fórmula.

### 4. Execução da regra raiz por nível

Audite se a regra raiz é aplicada por matéria:

#### Fundamental

- Deve avaliar cada matéria disciplinar da academia correspondente ao `ano_escolar_fundamental` do estudante.
- Deve calcular `nota_final` individual por matéria.
- Aprovação direta só ocorre se todas as matérias avaliadas atingirem a nota mínima.
- Matérias abaixo da mínima acionam regra descendente aplicável.

#### Médio

- Deve avaliar cada matéria disciplinar do curso da matéria avaliada, usando `curso_id`.
- Deve respeitar `ano_escolar_medio`.
- Deve calcular `nota_final` individual por matéria.
- Regra raiz de médio deve exigir `materias_chave`.
- Aprovação direta depende das matérias em `materias_chave`.
- `materias_chave` só pode conter matérias válidas, ativas, pertencentes ao curso e ao ano escolar médio aplicável.
- Matérias reprovadas acionam regra descendente aplicável por matéria.

#### Superior

- Deve avaliar cada matéria disciplinar do curso da matéria avaliada, usando `curso_id`.
- Deve respeitar o período/semestre atual do estudante.
- Deve calcular `nota_final` individual por matéria.
- Aprovação direta depende de todas as matérias avaliadas.
- A fórmula superior deve usar período inferido pela matéria avaliada.

### 5. Registro/evento/projeção de avaliação final

Confirme que o evento/projeção/resposta da API registra e expõe a lista completa de matérias avaliadas.

Cada item deve conter dados suficientes, incluindo:

- `materia_id`;
- `nota_final`;
- aprovado/reprovado por matéria, quando útil;
- regra usada no cálculo, especialmente quando há regra descendente;
- snapshot suficiente para reconstruir o cálculo;
- dados de pendência quando aplicável.

A decisão geral do estudante deve derivar do conjunto de resultados por matéria e regras de pendência, nunca de uma média global única.

### 6. Regras descendentes

Confirme que:

- Regras descendentes representam recuperação/exame/recurso.
- Podem ter lista de matérias aplicáveis.
- Essa lista deve ser subconjunto das matérias avaliadas pela regra ascendente.
- A descendente só é acionada para matérias abaixo da mínima na etapa anterior.
- A aprovação/reprovação da descendente é calculada por matéria.
- Matéria fora da lista de aplicação não é recalculada.
- Não é possível criar descendente órfã, cíclica, com nível incompatível ou com matérias fora do escopo.
- Continua existindo uma única regra raiz aplicável por escopo.

### 7. Pendências de matérias

Audite e corrija completamente o recurso de matérias pendentes.

Confirme que:

- Pendências são permitidas apenas em `medio` e `superior`.
- Fundamental nunca permite pendência.
- Pendência só é considerada após reprovação na raiz e em todas as descendentes aplicáveis.
- Se não houver descendente, pendência é avaliada após a raiz.
- `pendencia_permitida=true` permite gerar pendência dentro do limite.
- `pendencia_permitida=false` reprova totalmente e não gera pendência.
- O snapshot do valor de `pendencia_permitida` é registrado no momento do cálculo.
- `limite_materias_pendentes` é obrigatório em médio/superior.
- `limite_materias_pendentes` rejeita valores negativos.
- Se o número de matérias abaixo da mínima for menor ou igual ao limite e todas permitirem pendência, o estudante pode aprovar com pendência.
- Se o número de matérias abaixo da mínima ultrapassar o limite, reprova totalmente sem criar pendências.
- Se qualquer matéria reprovada não permitir pendência, reprova totalmente sem criar pendências.
- Limite zero funciona corretamente.

### 8. Novo recurso persistente de matérias pendentes

Confirme se existe recurso/tabela/projeção/modelo/entidade adequado para matérias pendentes.

Cada registro deve conter, no mínimo:

- identificador único;
- estudante;
- matéria;
- academia;
- curso, obrigatório em médio/superior;
- nível;
- ano escolar médio ou período/semestre superior;
- ano letivo da avaliação que gerou a pendência;
- regra e evento de avaliação final que geraram a pendência;
- `pendente`;
- timestamps e metadados de auditoria.

Confirme também:

- Não cria duplicidade aberta para o mesmo estudante/matéria/academia/escopo letivo.
- Permite consultar pendências abertas e históricas.
- Permite verificar rapidamente pendências antes de progressão/conclusão.
- Bloqueios comparam curso atual do estudante com curso salvo na pendência.
- Pendências de curso anterior ficam no histórico, mas não bloqueiam curso atual.
- Criação e baixa acontecem por eventos auditáveis, não atualização silenciosa.

### 9. Bloqueio por `pendencia_nivel_conclusao`

Confirme que:

- A matéria possui `pendencia_nivel_conclusao`.
- Em médio, se esse campo for igual ao `ano_escolar_medio` atual, o estudante pode aprovar com pendência, mas não deve avançar/finalizar automaticamente.
- Em superior, se for igual ao `semestre_atual`, o estudante pode aprovar com pendência, mas não deve avançar/finalizar automaticamente.
- Enquanto houver pendência bloqueante do curso atual, o sistema mantém o estudante em estado de regularização.
- Pendências abertas de cursos anteriores não bloqueiam.
- Ao baixar todas as pendências bloqueantes do curso atual, o sistema retoma automaticamente o fluxo normal.

### 10. Avaliação de matérias pendentes

Confirme que:

- Existe fluxo/API/caso de uso para lançar avaliação final específica de regularização de pendências.
- Essa avaliação gera evento auditável próprio.
- Cada matéria pendente avaliada registra:
  - dados da pendência original;
  - `materia_id`;
  - nota obtida;
  - `aprovado`;
  - regra/critério/configuração usada;
  - operador/ator, quando disponível.
- Aprovação baixa `pendente=false` por projeção/evento.
- Reprovação mantém `pendente=true`.
- Quando não há mais pendências abertas, o sistema retoma automaticamente:
  - avanço de ano no médio;
  - avanço de período/semestre no superior;
  - conclusão/finalização, quando cabível.

### 11. API, DTOs, validações e documentação

Audite todos os endpoints relacionados, especialmente:

- `POST /academia/avaliacao-final/regras`
- `PUT /academia/avaliacao-final/regras/:id`
- listagem/leitura de regras;
- execução automática por lançamento de nota;
- avaliação/regularização de pendência;
- consulta de pendências.

Confirme que:

- Payload, resposta, filtros, DTOs, validações e docs usam `nivel`.
- `tipo_ensino` é rejeitado.
- Campos incompatíveis com nível geram erro claro.
- `materias_chave` é obrigatório no médio em regra raiz.
- `limite_materias_pendentes` é obrigatório no médio/superior.
- Respostas públicas nunca expõem `tipo_ensino`.
- Médio/superior não expõem `anos_academicos` como configuração pública.
- Documentação explica as novas regras e remove o contrato antigo.

### 12. Persistência e migrações

Audite:

- Migrações.
- Entidades/modelos.
- Schemas.
- Índices.
- Constraints.
- Seeds/factories, se existirem.
- Projeções/event stores, se existirem.

Confirme que existem estruturas para:

- resultados por matéria no evento/projeção de avaliação final;
- `materias_chave`;
- matérias aplicáveis em regras descendentes;
- `limite_materias_pendentes`;
- matérias pendentes;
- índices por academia, estudante, matéria, nível, ano letivo, status e escopo acadêmico;
- restrição contra duplicidade de pendência aberta.

### 13. Testes obrigatórios

Verifique se existem testes suficientes e, se faltarem, implemente.

Devem existir testes para:

- parser/validador de fórmula para fundamental, médio e superior;
- criação, edição, listagem e deleção lógica de regras com `nivel`;
- rejeição absoluta de `tipo_ensino`;
- avaliação automática fundamental por matéria;
- avaliação automática média com `materias_chave`;
- avaliação automática superior com período inferido;
- cadeia descendente por matéria;
- pendência permitida e não permitida;
- `limite_materias_pendentes` com limite zero, dentro do limite e acima do limite;
- reprovação total sem criação de pendências quando limite é ultrapassado;
- bloqueio por `pendencia_nivel_conclusao`;
- pendência de curso anterior não bloqueando curso atual;
- avaliação e baixa de matérias pendentes;
- idempotência de avaliação final;
- idempotência de pendências abertas;
- contrato/documentação garantindo respostas públicas com `nivel` e sem `tipo_ensino`.

## Como executar a auditoria

1. Leia primeiro a tarefa original completa.
2. Inspecione a arquitetura do projeto.
3. Localize todos os arquivos relacionados à avaliação final, regras, notas, matérias, pendências, estudantes, cursos, progressão, eventos, projeções, DTOs, controllers/handlers, serviços/use cases, repositórios, migrations, testes e docs.
4. Para cada arquivo relevante:
   - explique o papel do arquivo;
   - diga quais requisitos da tarefa ele cobre;
   - diga se está correto, incompleto ou inconsistente;
   - corrija o que estiver incorreto;
   - adicione ou ajuste testes.
5. Não confie apenas em nomes de arquivos. Faça busca textual e análise semântica.
6. Procure bugs de borda:
   - matéria sem nota exigida;
   - regra descendente parcial;
   - pendência acima do limite;
   - pendência de matéria não permitida;
   - academia mista;
   - academia não mista;
   - médio com curso;
   - superior com período inferido;
   - idempotência;
   - duplicidade de pendência aberta;
   - mudança de curso;
   - conclusão de ciclo;
   - regra com campo legado.
7. Rode os testes relevantes.
8. Se houver falhas, corrija e rode novamente.
9. Ao final, faça commit das alterações no branch atual e crie o pull request com título e corpo adequados.

## Entregáveis esperados

Ao final, entregue:

1. Resumo das correções feitas.
2. Lista dos arquivos alterados.
3. Explicação dos bugs encontrados e como foram corrigidos.
4. Confirmação explícita, requisito por requisito, se a tarefa está implementada.
5. Lista dos testes adicionados/alterados.
6. Resultado dos comandos de teste executados.
7. Quaisquer limitações ou riscos restantes.
8. Commit realizado e PR criado.

Use citações de arquivo no formato exigido pelo ambiente para todo arquivo mencionado no resumo final.
