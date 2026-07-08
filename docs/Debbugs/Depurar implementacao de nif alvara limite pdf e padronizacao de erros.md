---
modificado: 2026-07-08 00:00
criado: 2026-07-08 00:00
---
# Depurar implementação de NIF, alvará obrigatório, limite PDF de 10MB e padronização de erros

Tarefa: [[Adicionar nif alvara limite pdf e padronizar erros]]

## Objetivo da auditoria

Fazer uma auditoria crítica, extremamente profunda, completa e arquivo por arquivo da implementação da tarefa:

`docs/Tarefas feitas/Adicionar nif alvara limite pdf e padronizar erros.md`

A auditoria deve confirmar se a implementação foi feita corretamente, completamente e **à risca**. Caso qualquer parte esteja incompleta, inconsistente, parcialmente implementada, sem validação, sem teste, sem documentação, com comportamento silencioso incorreto ou divergente do contrato esperado, esta tarefa exige **terminar a implementação e corrigir o que estiver errado**.

Esta funcionalidade é crítica porque altera contratos públicos, persistência, validação documental, upload de arquivos e o formato de erro de todas as rotas. O backend não pode aceitar `nif` inválido, duplicado ou numérico; não pode cadastrar academia sem `alvara`; não pode aceitar PDF acima de 10MB em nenhuma rota; e não pode retornar erros no modelo legado em nenhum fluxo.

## Resultado esperado da depuração

A depuração só pode ser encerrada quando estiver garantido que:

- toda academia possui `nif` obrigatório, persistido e retornado como string de exatamente 10 dígitos;
- zeros à esquerda no `nif` são preservados de ponta a ponta;
- `nif` enviado como número, com máscara, espaço, pontuação, letra, menos de 10 dígitos ou mais de 10 dígitos é rejeitado;
- a unicidade do `nif` é global entre academias, sem filtro por status ativo, inativo, desativado, arquivado ou equivalente;
- existe validação de aplicação/domínio e, sempre que possível, constraint ou índice único de persistência para impedir duplicidade de `nif`, inclusive sob concorrência;
- o cadastro de academia exige `alvara` como documento formal obrigatório;
- o `alvara` aceita apenas PDF e respeita o limite global de 10MB;
- o `alvara` é armazenado dentro de `{codigo_academia}/Documentação formal/`, associado à academia e rastreável nos fluxos administrativos/documentais existentes;
- falhas de validação ou upload do `alvara` não deixam academia criada, ativada ou parcialmente persistida em estado inconsistente;
- todo upload de PDF do sistema usa o mesmo limite máximo de 10MB, sem exceções por rota, entidade, handler, middleware, serviço ou cliente de storage;
- todas as rotas retornam erros exclusivamente no padrão mais recente do backend;
- helpers, wrappers, serializers, handlers e testes do modelo legado de erro foram removidos ou substituídos, sem fallback, alias, flag, header, query param ou conversor de compatibilidade;
- a documentação funcional, documentação de API, OpenAPI/Swagger, guias técnicos, exemplos e qualquer documento afetado estejam de acordo com a nova versão do código;
- a tarefa original receba o sufixo `(feito)` no **título interno do Markdown**, não no nome do arquivo, e seja movida de `docs/Lista de tarefas/` para `docs/Tarefas feitas/` somente depois de tudo estar implementado, corrigido, testado e documentado.

## Regra oficial obrigatória

A implementação final deve obedecer exatamente às regras abaixo:

| Área | Regra obrigatória | Proibição explícita |
| --- | --- | --- |
| `nif` | String obrigatória, única, exatamente 10 dígitos numéricos | Número, máscara, espaços, pontuação, letras, tamanho diferente de 10, duplicidade por qualquer status |
| `alvara` | Documento obrigatório no cadastro de academia, PDF, salvo em `{codigo_academia}/Documentação formal/` | Academia criada/ativada sem `alvara`, arquivo não PDF, upload sem rastreabilidade |
| PDFs | Limite global máximo de 10MB para todo PDF enviado ao sistema | Limites divergentes, exceções por rota, validação somente depois de persistir |
| Erros | Todas as rotas usam somente o padrão mais recente | Modelo legado, wrappers antigos, aliases, fallback temporário ou compatibilidade silenciosa |
| Documentação | Contratos, exemplos e OpenAPI refletem o código real | Documentar campo como número, omitir `alvara`, citar limite diferente, mostrar erro legado |

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. DTOs, schemas, validators, commands e handlers de criação, edição, leitura, listagem, ativação/inativação e administração de academias;
2. modelos de domínio, aggregates, eventos, snapshots, projeções, repositories, migrations e serializers de academia;
3. validações de unicidade e constraints/índices de banco relacionados a academia e `nif`;
4. fluxos transacionais de cadastro de academia, principalmente a ordem entre persistência da academia, geração de código, upload do `alvara` e commit/rollback;
5. adapters, services, gateways e clientes de storage usados para salvar documentos formais;
6. qualquer modelo, tabela, projeção ou endpoint que represente documentos formais de academia;
7. todas as rotas que aceitam upload de PDF, incluindo estudantes, responsáveis, solicitações de matrícula, academias, importações, anexos e jobs assíncronos;
8. middlewares, helpers, constantes e validações de tamanho/MIME/extensão de arquivos;
9. handlers, middlewares, helpers, serializers, interceptors e recovery/global error handler responsáveis por erros de validação, autenticação, autorização, domínio, upload, storage, banco, not found e erro inesperado;
10. testes unitários, testes de handler, testes de integração, testes de repository/migration, snapshots e regressões existentes sobre academias, documentos, uploads e erros;
11. documentação funcional, documentação de API, OpenAPI/Swagger, README técnico, guias de storage, guias de erros e exemplos de payload;
12. qualquer tarefa feita, manual ou documento histórico usado como referência ativa que ainda descreva cadastro de academia, documentação formal, uploads de PDF ou formato de erro;
13. seeds, scripts operacionais, jobs, imports, factories e fixtures que criem academias ou documentos sem `nif`/`alvara`;
14. comportamento sob concorrência para criação/edição de academias com o mesmo `nif`.

## Checklist obrigatório de validação

### 1. Busca ampla e classificação de ocorrências

Fazer busca ampla no repositório por, no mínimo:

- `nif`;
- `NIF`;
- `alvara`;
- `alvará`;
- `Documentação formal`;
- `Documentacao formal`;
- `application/pdf`;
- `pdf`;
- `maxSize`;
- `max_size`;
- `MaxBytes`;
- `10MB`;
- `10 MB`;
- `10485760`;
- `1024 * 1024`;
- `multipart`;
- `upload`;
- `erro`;
- `error`;
- `errors`;
- `message`;
- `details`;
- `legacy`;
- nomes reais dos helpers/structs/envelopes de erro do projeto.

Não basta listar ocorrências. Cada ocorrência relevante deve ser classificada como:

- implementação correta do novo contrato;
- validação obrigatória;
- persistência/constraint necessária;
- documentação atualizada;
- teste cobrindo regressão;
- bug ativo a corrigir;
- resíduo legado a remover;
- código morto a apagar;
- documentação histórica aceitável apenas se não for contrato vigente.

### 2. Contrato público de academia e campo `nif`

Auditar todos os contratos públicos e internos de academia.

Validar que:

- criação de academia exige `nif`;
- atualização de academia valida `nif` quando o campo estiver presente e não permite estado final sem `nif`;
- `nif` é tipado como string em DTOs, schemas, commands, models, projections e documentação;
- o backend rejeita `nif` enviado como número quando o parser/schema consegue distinguir tipo;
- a validação exige regex equivalente a `^[0-9]{10}$` sem trim permissivo que aceite entrada com espaços;
- zeros à esquerda são preservados em request, command, domínio, persistência, projection, serializer e response;
- responses de criação, leitura, listagem e administração retornam `nif` quando o contrato de academia exigir esse campo;
- logs e mensagens de erro não mascaram a causa de invalidez de forma ambígua;
- exemplos da documentação mostram `nif` entre aspas e nunca como número.

Testar ou exigir testes para os cenários:

- `"0123456789"` válido e preservado;
- campo ausente rejeitado;
- `1234567890` numérico rejeitado;
- `"123456789"` rejeitado;
- `"12345678901"` rejeitado;
- `"12345A7890"` rejeitado;
- `"123 456789"` rejeitado;
- `"123.456.789"` rejeitado;
- `" 1234567890"` e `"1234567890 "` rejeitados;
- consulta/listagem retorna exatamente o valor persistido.

### 3. Persistência e unicidade global do `nif`

Auditar migrations, models de banco, repositories, queries e constraints.

Validar que:

- existe coluna/campo persistente textual para `nif`, com tamanho compatível com 10 caracteres;
- migrations novas são idempotentes quando aplicável e seguras para ambientes com dados existentes;
- backfill ou estratégia para academias existentes está explícita, segura e documentada;
- existe índice/constraint única para `nif` sempre que a tecnologia de persistência permitir;
- a unicidade não é parcial por `ativo`, `status`, `deleted_at`, `archived_at` ou campos equivalentes;
- validação de aplicação retorna erro padronizado e legível antes ou ao capturar violação de constraint;
- criação concorrente com mesmo `nif` não gera duplicidade;
- edição de academia para `nif` já usado por outra academia é rejeitada;
- manter o mesmo `nif` na própria academia em update idempotente é aceito;
- academias inativas, desativadas, arquivadas, removidas logicamente ou equivalentes continuam bloqueando reuso do `nif` quando ainda representarem cadastro existente.

### 4. Cadastro de academia com `alvara` obrigatório

Auditar o fluxo completo de cadastro de academia.

Validar que:

- o contrato de criação aceita e exige o arquivo/campo documental `alvara` conforme o padrão de upload do projeto;
- ausência de `alvara` bloqueia o cadastro;
- arquivo não PDF bloqueia o cadastro;
- PDF acima de 10MB bloqueia o cadastro;
- arquivo válido é salvo em `{codigo_academia}/Documentação formal/`;
- o nome lógico, metadados, tipo documental e vínculo com academia permitem identificar que o documento é o `alvara`;
- o armazenamento usa a estrutura e abstrações já existentes, sem caminhos hardcoded incompatíveis com o storage real;
- se o código da academia é gerado durante o cadastro, o fluxo garante que o caminho final use o código correto;
- falha no upload, falha no storage, falha no banco ou falha no vínculo documental não deixa academia cadastrada sem documento obrigatório;
- se não houver transação distribuída com storage, existe compensação explícita para remover arquivo órfão ou rollback do registro;
- endpoints administrativos de consulta/download/listagem de documentos incluem ou respeitam o `alvara` conforme autorização vigente;
- permissões de acesso ao `alvara` não expõem documento formal para usuário não autorizado.

### 5. Limite global de 10MB para todo PDF

Auditar todas as rotas e serviços que aceitam PDF.

Validar que:

- existe uma constante/helper central, ou uma regra claramente única, para `10 * 1024 * 1024` bytes;
- não existem limites divergentes para PDFs em handlers, middlewares, validators, DTOs, storage clients, configs, docs ou testes;
- o tamanho é validado antes de persistir o arquivo;
- PDF exatamente com 10MB é aceito ou rejeitado conforme a decisão implementada, e essa decisão está documentada de forma explícita;
- PDF com 10MB + 1 byte é rejeitado em todas as rotas;
- validação de tamanho não depende apenas de header manipulável quando o backend consegue medir bytes reais;
- MIME type, extensão e assinatura/conteúdo são tratados de acordo com o padrão vigente do projeto;
- erro por tamanho excedido usa o padrão mais recente;
- jobs assíncronos, imports e endpoints administrativos não contornam a validação global.

Auditar explicitamente uploads de:

- `alvara` de academia;
- documentos formais de academia;
- documentos de estudante;
- documentos de responsável;
- solicitações de matrícula;
- anexos administrativos;
- importações ou PDFs de batch;
- qualquer outro endpoint que aceite `application/pdf`.

### 6. Padronização completa de erros

Identificar, primeiro, qual é o padrão mais recente de erro do backend pela implementação real e documentação atualizada. Em seguida, auditar todas as rotas e camadas para garantir que somente esse padrão seja emitido.

Validar que erros de todas as classes usam o mesmo envelope/corpo vigente:

- validação de payload/body/query/path;
- autenticação ausente/inválida;
- autorização insuficiente;
- domínio/regra de negócio;
- recurso não encontrado;
- conflito/duplicidade, incluindo `nif` duplicado;
- upload ausente, inválido, MIME inválido ou tamanho excedido;
- falha de storage;
- falha de banco/constraint;
- erro interno sanitizado;
- erros de parsing multipart;
- erros de jobs/SSE quando retornarem resposta HTTP;
- healthcheck ou endpoints administrativos, quando aplicável.

Remover ou substituir completamente:

- structs antigas de erro;
- helpers antigos;
- wrappers legados;
- campos legados;
- serializers antigos;
- respostas manuais em handlers;
- snapshots de teste no formato antigo;
- documentação e exemplos do modelo legado.

Não aceitar como correto:

- compatibilidade por header;
- compatibilidade por query param;
- alias de campo antigo e novo ao mesmo tempo;
- conversor automático para formato legado;
- fallback quando algum handler não sabe usar o padrão novo;
- rotas “pequenas” ou antigas retornando erro fora do padrão.

### 7. Documentação e OpenAPI/Swagger

Auditar a documentação como contrato público e operacional do sistema.

Validar que a documentação esteja de acordo com a nova versão do código, incluindo:

- `nif` documentado como string obrigatória, única e de exatamente 10 dígitos;
- exemplos de criação/edição/listagem de academia contendo `nif` como string;
- regra de unicidade do `nif` explicitando que academias ativas e não ativas bloqueiam duplicidade;
- `alvara` documentado como obrigatório no cadastro de academia;
- caminho `{codigo_academia}/Documentação formal/` documentado quando a documentação tratar storage/localização documental;
- limite de PDF documentado como 10MB em todos os pontos que mencionam uploads;
- erros documentados somente no padrão mais recente;
- remoção de exemplos, tabelas ou respostas no modelo legado;
- OpenAPI/Swagger com schemas, required fields, examples, multipart/form-data, status codes e error responses atualizados;
- README técnico, manuais, guias e tarefas feitas relevantes sem instruções conflitantes com o código atual.

Não considerar a depuração concluída se a implementação estiver correta mas a documentação ainda estiver desatualizada, incompleta ou contraditória.

### 8. Testes obrigatórios

Garantir que existam testes automatizados cobrindo, no mínimo:

- criação de academia com `nif` válido;
- preservação de zeros à esquerda;
- rejeição de `nif` ausente;
- rejeição de `nif` numérico;
- rejeição de `nif` com tamanho inválido;
- rejeição de `nif` com caracteres não numéricos;
- consulta/listagem retornando `nif`;
- criação com `nif` duplicado em academia ativa;
- criação com `nif` duplicado em academia inativa/desativada/arquivada ou estado equivalente;
- update para `nif` duplicado em outra academia;
- constraint/índice único de persistência para `nif`;
- cadastro com `alvara` PDF válido;
- cadastro sem `alvara` rejeitado;
- cadastro com `alvara` não PDF rejeitado;
- cadastro com `alvara` acima de 10MB rejeitado;
- falha de upload do `alvara` sem academia inconsistente;
- caminho/metadados do `alvara` em `{codigo_academia}/Documentação formal/`;
- PDF abaixo de 10MB aceito nas rotas de upload;
- PDF exatamente no limite coberto conforme a regra definida;
- PDF acima de 10MB rejeitado em todas as rotas relevantes;
- erro de validação no padrão novo;
- erro de autenticação no padrão novo;
- erro de autorização no padrão novo;
- erro de domínio no padrão novo;
- erro de not found no padrão novo;
- erro de upload no padrão novo;
- erro interno sanitizado no padrão novo;
- ausência completa de campos/envelopes do modelo legado em responses e snapshots.

### 9. Critérios para corrigir durante a depuração

Se a auditoria encontrar divergência, corrigir imediatamente antes de concluir. Exemplos de correções obrigatórias:

- adicionar validação de `nif` ausente ou inválido;
- trocar tipo numérico de `nif` por string;
- criar migration/constraint/índice único para `nif`;
- ajustar repository para consultar duplicidade sem filtro de status;
- tornar `alvara` obrigatório no cadastro;
- adicionar rollback/compensação em falha de upload;
- centralizar limite de PDF em 10MB;
- substituir limite divergente em rota específica;
- trocar erro manual pelo helper novo;
- remover helper legado não usado;
- atualizar testes e snapshots;
- atualizar documentação, OpenAPI/Swagger e exemplos.

Nenhuma correção deve introduzir suporte legado, alias, fallback temporário, wrapper de compatibilidade ou divergência silenciosa do contrato.

## Comandos e verificações recomendadas

Executar comandos compatíveis com a stack real do projeto. No mínimo, adaptar e executar uma combinação de:

```bash
rg -n "nif|NIF|alvara|alvará|Documentação formal|Documentacao formal|application/pdf|multipart|upload|maxSize|max_size|MaxBytes|10485760|10MB|10 MB|legacy|erro|error|errors" .
```

```bash
rg -n "type .*Academia|Academia.*struct|academia|CreateAcademia|UpdateAcademia|CadastroAcademia|documento|Documento|PDF|pdf" internal docs .
```

```bash
rg -n "http\.Error|c\.JSON|json\.NewEncoder|ErrorResponse|errorResponse|message|details|code|legacy" internal .
```

Também executar a suíte de testes relevante do projeto, testes de migration/integração quando existirem, geração/validação de OpenAPI/Swagger quando houver comando disponível e format/lint apropriados para a stack.

## Encerramento obrigatório da tarefa original

Somente depois de confirmar que implementação, testes e documentação estão corretos:

1. abrir `docs/Tarefas feitas/Adicionar nif alvara limite pdf e padronizar erros.md`;
2. alterar o título interno de Markdown para adicionar o sufixo `(feito)`, ficando:

```markdown
# Adicionar NIF, alvará obrigatório, limite PDF de 10MB e padronizar erros (feito)
```

3. mover o arquivo para:

```text
docs/Tarefas feitas/Adicionar nif alvara limite pdf e padronizar erros.md
```

4. não alterar o nome do arquivo para incluir `(feito)`;
5. garantir que nenhum link interno ou referência ativa fique quebrado após a movimentação.

## Critério de aceite da depuração

A depuração só pode ser considerada concluída quando:

- todos os itens do checklist acima forem verificados;
- todos os bugs encontrados forem corrigidos;
- não houver divergência entre código, testes, documentação e OpenAPI/Swagger;
- toda validação de `nif`, `alvara`, upload de PDF e erro padronizado tiver cobertura automatizada;
- não existir rota emitindo erro legado;
- não existir upload de PDF com limite diferente de 10MB;
- não existir fluxo de cadastro de academia sem `nif` válido e `alvara` obrigatório;
- não existir duplicidade possível de `nif` entre academias por falha de validação ou persistência;
- a suíte relevante de testes passar;
- a tarefa original estiver com o título interno sufixado por `(feito)` e movida para `docs/Tarefas feitas/`.
