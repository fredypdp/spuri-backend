Preciso que você faça uma auditoria crítica, completa e arquivo por arquivo da documentação do sistema, comparando cada contrato documentado com o código real implementado no backend.

A documentação deve ser tratada como documentação de contrato público e operacional do sistema. O objetivo desta tarefa não é reescrever explicações de funcionamento, nem melhorar textos conceituais: o foco exclusivo é verificar se os **campos, valores, formatos, objetos, enums, requests e responses** documentados refletem exatamente o que o código real aceita, persiste, emite e retorna.

## Objetivo da auditoria

Verificar se cada secção relevante da documentação está alinhada com a implementação real do código, especialmente:

1. `Convenções Globais`.
2. `Estruturas de Dados`.
3. Cada escopo de rota existente na documentação da API.
4. Requests reais aceitos por cada rota.
5. Responses reais retornados por cada rota.
6. Campos e valores de todas as entidades do sistema.
7. Campos e valores presentes em eventos, projeções, DTOs, schemas, validações, handlers e serializers.
8. Remoção completa da secção `Telefones nativos`.

A auditoria deve garantir que todas as estruturas de dados de todas as entidades do sistema estejam concentradas e atualizadas na secção `Estruturas de Dados`, e que cada rota documente corretamente o contrato real de entrada e saída.

## Arquivos e áreas obrigatórias de análise

Analise, no mínimo:

- Documentação principal do sistema.
- Documentação da API.
- DTOs de request e response.
- Schemas de validação.
- Controllers/handlers/rotas.
- Casos de uso/services chamados pelas rotas.
- Aggregates, eventos e projeções.
- Serializers/mappers/adapters de resposta.
- Migrações e modelos persistidos quando forem necessários para confirmar campos e valores.
- Testes existentes que expressem contratos de request/response.

Não assuma que a documentação está correta. O código real é a fonte da verdade.

## Escopo obrigatório da documentação

### 1. `Convenções Globais`

Confirme que a secção `Convenções Globais` documenta apenas convenções realmente usadas pelo backend.

Verifique, pelo código real:

- Formato de datas e timestamps.
- Formato de IDs.
- Convenções de paginação.
- Convenções de ordenação e filtros.
- Formato padrão de erro.
- Formato padrão de sucesso, quando existir.
- Regras para autenticação e headers.
- Convenções de nomes de campos.
- Valores padrão aplicados pelo backend.
- Campos opcionais, obrigatórios, nullable e ausentes.

Corrija apenas campos, valores e formatos que estejam errados, incompletos ou ausentes. Não altere descrições conceituais que não estejam relacionadas ao contrato de dados.

### 2. `Estruturas de Dados`

A secção `Estruturas de Dados` deve conter a representação atualizada das estruturas de dados de todas as entidades do sistema.

Confirme, entidade por entidade, se estão documentados corretamente:

- Todos os campos públicos retornados por APIs.
- Todos os campos aceitos em criação/edição, quando a entidade tiver request próprio.
- Campos gerados pelo backend.
- Campos persistidos que aparecem em projeções ou responses.
- Campos derivados/calculados retornados ao cliente.
- Campos removidos ou legados que não devem mais aparecer.
- Tipos reais dos campos.
- Valores possíveis de enums e literais.
- Campos obrigatórios, opcionais, nullable e arrays.
- Objetos aninhados.
- Relações por ID e dados expandidos, quando retornados.

A auditoria deve cobrir todas as entidades existentes no sistema, incluindo, mas não se limitando a:

- Admin.
- Academia.
- Estudante.
- Curso.
- Matéria disciplinar.
- Turma.
- Notas.
- Faltas.
- Aulas/sumários, se existirem no contrato público.
- Regras de avaliação final.
- Resultados/eventos/projeções de avaliação final.
- Matérias pendentes, se existirem.
- Solicitação de matrícula.
- Arquivos/anexos, se existirem no contrato público.
- Jobs assíncronos/batches, se existirem no contrato público.
- Qualquer outra entidade ou recurso exposto por rota, DTO, evento/projeção ou documentação.

Se uma entidade existe no código e é exposta publicamente, mas não está em `Estruturas de Dados`, adicione-a. Se uma entidade documentada não existe mais ou não é exposta do modo descrito, corrija ou remova o contrato incorreto.

### 3. Escopos de rota existentes

Para cada escopo de rota documentado, compare a documentação com as rotas reais registradas no código.

Confirme, rota por rota:

- Método HTTP real.
- Path real.
- Parâmetros de path reais.
- Query params reais.
- Body real aceito.
- Headers relevantes reais.
- Status codes reais, quando definidos.
- Response real de sucesso.
- Response real de erro quando houver formato específico.
- Campos obrigatórios, opcionais, nullable e defaults.
- Diferenças entre criação, edição, listagem, leitura, remoção, ativação/inativação, execução e operações em lote.

A documentação de cada rota deve conter o request e response reais. Se a rota retorna uma projeção ou DTO diferente da entidade base, documente a estrutura retornada pela rota, não apenas a entidade genérica.

### 4. Requests reais

Audite todos os exemplos e tabelas de request.

Confirme que:

- Não há campo documentado que o backend rejeita.
- Não falta campo que o backend exige.
- Campos opcionais estão marcados como opcionais apenas quando realmente são opcionais.
- Campos nullable estão marcados como nullable apenas quando realmente aceitam `null`.
- Enums e literais contêm exatamente os valores aceitos pelo código.
- Campos calculados/gerados pelo backend não aparecem como se fossem enviados pelo cliente.
- Campos internos, históricos ou de banco não aparecem como contrato público se não forem aceitos pela API.

### 5. Responses reais

Audite todos os exemplos e tabelas de response.

Confirme que:

- Todo campo retornado pelo backend está documentado quando for parte estável do contrato.
- Nenhum campo inexistente está documentado como retornado.
- Objetos aninhados, arrays e metadados estão corretos.
- Paginação, filtros e envelopes de resposta refletem exatamente o retorno real.
- Campos de auditoria, timestamps, IDs e status aparecem com seus tipos e valores reais.
- Responses de criação, edição, leitura, listagem e operações especiais são diferenciados quando o código retornar formatos diferentes.

### 6. Remoção da secção `Telefones nativos`

Remova a secção `Telefones nativos` da documentação.

Também confirme se há referências cruzadas para essa secção ou para um contrato antigo relacionado. Se houver referências obsoletas, remova-as ou ajuste-as apenas no que for necessário para não deixar link, índice ou menção quebrada.

Não remova nem altere estruturas de telefone que ainda existam no código real e façam parte de entidades ou rotas. A remoção solicitada é da secção `Telefones nativos`, não de campos reais de telefone que o backend ainda exponha.

## O que não deve ser alterado

Esta tarefa é estritamente sobre **campos e valores**.

Não altere:

- Descrições documentais de funcionamento que já estejam corretas conceitualmente.
- Explicações de regra de negócio sem divergência de contrato de dados.
- Textos de contexto, motivação ou arquitetura apenas por estilo.
- Nomes de secções que não estejam incorretos ou quebrados.
- Fluxos descritos, salvo quando citarem campos, valores, requests ou responses incorretos.

Não faça refatoração textual ampla. Não transforme a tarefa em revisão editorial. O objetivo é precisão contratual.

## Como executar a auditoria

1. Localize as secções `Convenções Globais`, `Estruturas de Dados` e todos os escopos de rota na documentação.
2. Liste as rotas reais registradas no backend.
3. Para cada rota, encontre o handler/controller, DTO/schema, validações e serializer/mapper de resposta.
4. Para cada entidade, encontre models, projections, eventos, DTOs e responses que exponham campos publicamente.
5. Compare documentação versus código real.
6. Corrija a documentação somente onde houver divergência de campos, valores, tipos, obrigatoriedade, request ou response.
7. Remova a secção `Telefones nativos` e referências obsoletas a ela.
8. Execute buscas amplas por campos documentados suspeitos para confirmar se são reais ou legados.
9. Execute testes/checks disponíveis que ajudem a validar que a alteração foi apenas documental.

## Documento de inconsistências encontradas

Ao final da auditoria, crie um documento separado listando tudo de incorreto que foi encontrado **somente se alguma inconsistência tiver sido encontrada**.

Esse documento deve conter:

- Nome do arquivo/documentação afetada.
- Secção afetada.
- Campo, valor, request ou response incorreto.
- O que estava documentado.
- O que o código real demonstrou.
- Correção aplicada.
- Referência ao arquivo de código usado como fonte da verdade.

Se nenhuma inconsistência for encontrada, **não crie esse documento**.

## Entregáveis esperados

1. Documentação corrigida para refletir o código real em `Convenções Globais`, `Estruturas de Dados` e rotas.
2. Secção `Telefones nativos` removida.
3. Nenhuma alteração em descrições de funcionamento que não envolvam campos, valores, requests ou responses.
4. Documento de inconsistências encontradas, apenas se houver divergência real.
5. Resumo final explicando:
   - quais arquivos de documentação foram alterados;
   - quais escopos de rota foram auditados;
   - se foi ou não criado documento de inconsistências;
   - quais comandos de verificação foram executados.

## Critérios de aceite

- `Estruturas de Dados` contém todas as entidades expostas pelo sistema e seus campos/valores reais.
- Cada rota documentada possui request e response compatíveis com o código real.
- `Convenções Globais` não documenta formatos ou envelopes inexistentes.
- Não existem campos legados ou removidos documentados como contrato atual.
- Não faltam campos públicos relevantes retornados pelo backend.
- A secção `Telefones nativos` foi removida.
- Nenhuma descrição de funcionamento foi alterada sem necessidade contratual.
- O documento de inconsistências só existe se inconsistências reais foram encontradas.
