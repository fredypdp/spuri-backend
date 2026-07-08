---
criado: 2026-07-08 00:00
origem: depuração da tarefa [[Adicionar nif alvara limite pdf e padronizar erros]]
status: pendente
---
# Depurar implementação de NIF, alvará obrigatório, limite PDF de 10MB e padronização de erros

## Objetivo da auditoria

Fazer uma auditoria crítica, completa, profunda e arquivo por arquivo da implementação da tarefa:

Tarefa: [[Adicionar nif alvara limite pdf e padronizar erros]]

A auditoria deve confirmar se a implementação foi feita corretamente, completamente e **à risca**. Caso qualquer parte esteja incompleta, inconsistente, parcialmente implementada, sem validação, sem teste, sem documentação, com comportamento silencioso incorreto ou divergente do contrato esperado, esta tarefa exige **instruir o ajuste e implementar o que falta**.

Esta funcionalidade é crítica porque altera o contrato de cadastro de academias, o modelo de identificação fiscal, a obrigatoriedade de documentação formal, a política global de upload de PDFs e o contrato de erros de toda a API. A auditoria não pode se limitar a verificar endpoints felizes: deve rastrear DTOs, entidades, migrações, validações, storage, serialização, OpenAPI/Swagger, testes e documentação técnica.

## Regra adicional obrigatória

Além da especificação original, é obrigatório garantir que:

- `nif` seja sempre tratado como **string obrigatória de exatamente 10 dígitos**, nunca como número;
- zeros à esquerda sejam preservados em entrada, persistência, eventos, projeções, responses e documentação;
- a unicidade de `nif` seja global entre academias, sem filtro por status ativo/inativo/desativado/arquivado;
- `alvara` seja documento formal obrigatório no cadastro de academia e não possa ser ignorado por rotas alternativas, seeds, imports, jobs ou atualizações parciais;
- todo PDF aceito pelo backend respeite o limite máximo global de **10MB**;
- todas as respostas de erro usem exclusivamente o padrão mais recente da API;
- o modelo legado de erros esteja removido de handlers, middlewares, helpers, testes, exemplos e documentação;
- a documentação funcional, técnica e OpenAPI/Swagger esteja de acordo com a versão final do código, sem exemplos antigos ou contratos contraditórios.

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. aggregate, entidades, eventos, comandos e validações de academias;
2. DTOs, schemas, validators e serializers de criação, edição, leitura e listagem de academias;
3. handlers, controllers, routers e casos de uso de cadastro/atualização de academias;
4. repositórios, projections, migrations e constraints relacionadas a academias;
5. fluxo de upload, storage e metadados dos documentos formais de academias;
6. todos os pontos que aceitam PDF no sistema;
7. constantes, middlewares, interceptors, validators e clientes de storage que validem tamanho ou mimetype de arquivo;
8. camada global de erros, exceptions, filters, middlewares, helpers e responses manuais;
9. testes unitários, integração, e2e, snapshots e fixtures relacionados;
10. documentação funcional, documentação técnica, OpenAPI/Swagger, exemplos de payload, coleções de API e guias operacionais;
11. seeds, scripts administrativos, factories e importações assíncronas que possam criar academia ou anexar PDF.

## Checklist obrigatório de validação

### 1. Contrato público de `nif`

Confirmar e, se necessário, implementar que:

- criação de academia exige `nif`;
- edição de academia mantém `nif` válido quando o campo for alterado;
- se atualização parcial permitir omitir `nif`, a omissão não apaga nem invalida o valor já persistido;
- leitura detalhada de academia retorna `nif` conforme contrato;
- listagem de academias retorna ou omite `nif` de forma deliberada, documentada e consistente com autorização;
- serializers não convertem `nif` para número;
- mappers entre domínio, DTO, evento e projeção preservam a string original;
- OpenAPI/Swagger documenta `nif` como `type: string`, com exemplo contendo 10 dígitos e, preferencialmente, padrão equivalente a `^[0-9]{10}$`;
- nenhum exemplo, schema ou teste documenta `nif` como inteiro.

Fazer busca ampla por `nif`, `NIF`, `numeroFiscal`, `identificacaoFiscal`, `taxId`, `fiscal`, `academia` e termos correlatos para localizar implementações paralelas ou omissões.

### 2. Validação estrita de `nif`

Validar que o backend rejeita, com erro padronizado:

- `nif` ausente no cadastro;
- `nif` enviado como número, quando o contrato diferencia tipos;
- string vazia;
- string com menos de 10 dígitos;
- string com mais de 10 dígitos;
- letras;
- espaços no começo, no meio ou no fim;
- pontuação, máscara ou separadores;
- caracteres unicode que pareçam números mas não sejam dígitos ASCII `0` a `9`;
- valor `null`, booleano, array ou objeto;
- payload que dependa de trim, cast ou coerção silenciosa.

Confirmar também que valores como `0001234567` são aceitos e retornados exatamente iguais.

### 3. Persistência e unicidade de `nif`

Auditar migrations, schema, constraints, índices, repositories e projections para garantir que:

- o campo persistido é textual e compatível com 10 caracteres;
- existe validação de aplicação/domínio antes da persistência;
- existe constraint ou índice único no banco sempre que o mecanismo de persistência permitir;
- a unicidade não é parcial por `status`, `ativo`, `deleted_at`, `archived_at` ou campos semelhantes;
- academias inativas, desativadas, arquivadas ou em qualquer estado não ativo continuam bloqueando reutilização do mesmo `nif`;
- atualização de academia não permite colisão com outra academia;
- atualização da própria academia não falha por comparar contra ela mesma;
- concorrência entre dois cadastros com o mesmo `nif` é tratada com erro padronizado e sem deixar estado duplicado;
- replay de eventos ou rebuild de projeções mantém consistência;
- seeds/factories/migrações de dados recebem `nif` válido e único.

### 4. `alvara` obrigatório no cadastro de academias

Confirmar e, se necessário, implementar que:

- o cadastro de academia exige arquivo `alvara`;
- o arquivo é validado como PDF real conforme o padrão vigente do projeto;
- o limite de 10MB é aplicado ao `alvara` antes de persistir o arquivo;
- o documento é salvo em `{codigo_academia}/Documentação formal/`;
- metadados, referência de storage e vínculo com academia ficam rastreáveis;
- a ausência de `alvara` retorna erro padronizado;
- `alvara` inválido não cria academia sem documento;
- falha de upload, falha de metadados ou falha transacional não deixa academia ativa/cadastrada em estado inconsistente;
- rotas de ativação, aprovação, importação ou backoffice não conseguem contornar a obrigatoriedade sem justificativa explícita e documentada;
- endpoints administrativos de consulta, listagem ou download incluem ou respeitam `alvara` conforme autorização vigente.

### 5. Storage e caminho do documento formal

Auditar a integração de storage para confirmar que:

- o caminho final usa o código da academia correta;
- a pasta é exatamente `Documentação formal`, respeitando acentuação e capitalização esperadas pelo contrato;
- nomes de arquivos evitam colisão e seguem o padrão de documentos formais;
- logs e auditoria registram falhas sem expor dados sensíveis;
- rollback ou compensação remove arquivo órfão quando a academia não é criada;
- testes não mockam o storage de forma tão ampla que deixem de validar o caminho final.

### 6. Limite global de 10MB para PDFs

Fazer busca ampla por limites de upload e validar cada ocorrência de:

- `10MB`, `10 MB`, `10485760`, `1024 * 1024`, `fileSize`, `maxSize`, `MAX_FILE`, `PDF`, `application/pdf`, `multipart`, `multer`, `upload`, `anexo`, `documento`, `comprovativo`, `alvara` e termos correlatos.

Confirmar que:

- existe regra centralizada ou, no mínimo, regra uniforme de 10MB;
- PDF menor que 10MB é aceito;
- PDF exatamente com 10MB é aceito se o contrato considerar limite inclusivo;
- PDF acima de 10MB é rejeitado;
- validação ocorre antes de persistir o arquivo;
- não existem rotas com limites antigos, maiores, menores ou sem limite;
- upload de documentos de estudantes, responsáveis, solicitações de matrícula, documentos formais de academias, importações administrativas e anexos assíncronos seguem a mesma regra;
- mensagens de erro informam o limite correto e usam o envelope novo;
- documentação e OpenAPI/Swagger mencionam 10MB em todos os lugares relevantes.

### 7. Padronização global de erros

Identificar o padrão mais recente de erro da API e confirmar que todas as rotas usam exclusivamente esse padrão.

Auditar e corrigir:

- exception filters;
- middlewares globais;
- validators;
- pipes/interceptors;
- helpers de resposta;
- handlers que retornam JSON manualmente;
- erros de autenticação;
- erros de autorização;
- erros de validação;
- erros de domínio;
- erros de upload;
- erros de storage;
- erros de banco/constraint;
- erros de recurso não encontrado;
- erros inesperados sanitizados;
- SSE/jobs assíncronos quando expuserem erro HTTP;
- healthcheck e endpoints administrativos, se estiverem no contrato público.

Confirmar que não restam campos, wrappers, aliases ou exemplos do modelo legado. Fazer busca ampla por nomes de campos e helpers antigos depois de identificar o legado real do projeto.

### 8. Remoção total do modelo legado de erros

Após identificar o formato antigo, classificar cada ocorrência como:

- código ativo que precisa ser removido;
- teste antigo que precisa ser atualizado;
- documentação histórica aceitável;
- fixture obsoleta que precisa ser migrada;
- comentário enganoso que precisa ser corrigido.

Não basta manter conversores, wrappers ou fallbacks que aceitem os dois formatos. A tarefa exige contrato único. Qualquer compatibilidade temporária deve ser removida, salvo se existir decisão formal posterior documentada em outra tarefa.

### 9. Testes obrigatórios

Criar ou ajustar testes cobrindo, no mínimo:

#### `nif`

- criação de academia com `nif` válido de 10 dígitos;
- preservação de zeros à esquerda;
- rejeição de `nif` ausente;
- rejeição de `nif` numérico;
- rejeição de `nif` com menos de 10 dígitos;
- rejeição de `nif` com mais de 10 dígitos;
- rejeição de letras, espaços, pontuação, máscara e caracteres unicode não ASCII;
- resposta de leitura/listagem conforme contrato;
- rejeição de duplicidade com academia ativa;
- rejeição de duplicidade com academia inativa/desativada/arquivada;
- atualização mantendo o próprio `nif` sem falso positivo;
- atualização tentando usar `nif` de outra academia.

#### `alvara`

- cadastro de academia com `alvara` válido;
- rejeição de cadastro sem `alvara`;
- rejeição de `alvara` não PDF;
- rejeição de `alvara` acima de 10MB;
- persistência do documento em `{codigo_academia}/Documentação formal/`;
- falha de upload não deixando academia inconsistente;
- erro padronizado para ausência, tipo inválido, tamanho excedido e falha de storage;
- autorização correta para consultar ou baixar o documento, quando houver endpoint.

#### PDFs globais

- PDF menor que 10MB aceito em rotas representativas;
- PDF exatamente com 10MB aceito, se limite inclusivo;
- PDF acima de 10MB rejeitado em todas as rotas que aceitam PDF;
- rotas antigas de upload não mantêm limite divergente;
- erro de tamanho excedido usa o padrão novo.

#### Erros

- erro de validação no padrão novo;
- erro de autenticação no padrão novo;
- erro de autorização no padrão novo;
- erro de domínio no padrão novo;
- erro de upload no padrão novo;
- erro de recurso não encontrado no padrão novo;
- erro interno sanitizado no padrão novo;
- snapshots/assertions garantindo ausência dos campos do modelo legado;
- testes de contrato para rotas representativas de cada módulo.

### 10. Documentação obrigatória

Auditar e corrigir toda documentação afetada, incluindo quando existirem:

- documentação de API/OpenAPI/Swagger;
- README técnico;
- documentação de domínio de academias;
- documentação de uploads e storage;
- documentação de erros da API;
- exemplos de payload;
- coleções de API;
- guias operacionais;
- documentos de tarefas anteriores usados como referência ativa.

A documentação deve deixar explícito que:

- `nif` é string obrigatória, única, com exatamente 10 dígitos;
- zeros à esquerda são preservados;
- o mesmo `nif` não pode existir em mais de uma academia, independentemente de status;
- `alvara` é obrigatório no cadastro de academia;
- `alvara` é PDF e respeita limite de 10MB;
- `alvara` fica salvo em `{codigo_academia}/Documentação formal/`;
- todo PDF enviado ao sistema possui limite máximo de 10MB;
- todas as rotas retornam erros apenas no padrão mais recente;
- o modelo legado de erros não é contrato suportado;
- exemplos antigos foram removidos ou atualizados.

### 11. Auditoria de regressões e fluxos alternativos

Confirmar que não há bypass por:

- endpoints administrativos;
- scripts de seed;
- factories de teste;
- jobs assíncronos;
- importações em lote;
- rotas internas;
- atualização parcial;
- comandos diretos de repository;
- fixtures antigas;
- OpenAPI desatualizado usado por clientes gerados.

Qualquer bypass encontrado deve ser corrigido ou bloqueado com validação explícita.

## Critérios de aceite

A auditoria só pode ser considerada concluída quando todos os itens abaixo forem verdadeiros:

- Academia possui `nif` obrigatório tratado como string de exatamente 10 dígitos.
- `nif` preserva zeros à esquerda em todo o fluxo.
- `nif` é único globalmente entre academias, sem permitir duplicidade em registros ativos ou não ativos.
- Cadastro de academia exige `alvara` obrigatório.
- `alvara` válido é salvo em `{codigo_academia}/Documentação formal/`.
- Falhas relacionadas ao `alvara` não deixam academia cadastrada/ativa em estado inconsistente.
- Todo upload de PDF do sistema aplica limite máximo de 10MB.
- PDFs acima de 10MB são rejeitados em todas as rotas.
- Todas as respostas de erro usam exclusivamente o padrão mais recente.
- O modelo legado de erros foi removido dos handlers, helpers, testes, fixtures e documentação ativa.
- OpenAPI/Swagger, documentação técnica e exemplos refletem exatamente a versão atual do código.
- Testes automatizados cobrem `nif`, `alvara`, limite global de PDFs e padronização de erros.
- Não existe alias, fallback, wrapper de compatibilidade ou contrato paralelo para campos/erros antigos.

## Resultado esperado da execução desta tarefa

Ao finalizar esta tarefa, produzir um resumo técnico informando:

- arquivos auditados;
- padrão novo de erro identificado;
- formato legado removido;
- problemas encontrados;
- correções implementadas;
- testes adicionados/alterados;
- comandos executados;
- documentação atualizada;
- eventuais decisões de design tomadas;
- confirmação explícita de que documentação, OpenAPI/Swagger e exemplos estão compatíveis com a versão final do código.
