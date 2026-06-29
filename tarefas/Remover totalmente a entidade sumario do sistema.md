---
modificado: 2026-06-29 00:00
criado: 2026-06-29 00:00
---
# Remover totalmente a entidade sumário do sistema

## Objetivo

Remover por completo a entidade **sumário/aula** do sistema, impedindo que academias criem, consultem, atualizem ou removam sumários e eliminando qualquer possibilidade de vincular faltas a um sumário.

A remoção deve ser definitiva no código e no contrato da API: não deve permanecer alias, compatibilidade retroativa, endpoint oculto, campo legado aceito silenciosamente, projeção ativa, evento de domínio, tabela operacional nova ou qualquer outro resquício funcional desse tipo de dado.

## Contexto e motivação

O sistema passou a suportar sumários/aulas como registros de conteúdo ministrado por academia, matéria e período. As faltas também passaram a aceitar vínculo opcional com `sumario_id`, gravando `sumario_titulo` como snapshot histórico.

A nova regra de produto é eliminar esse conceito do backend. A academia não deve mais registrar sumários e a falta deve voltar a existir de forma independente, sem referência direta ou indireta a sumários. Essa decisão simplifica o fluxo acadêmico e evita manter uma entidade que não será mais utilizada pelo produto.

A implementação deve tratar a remoção como limpeza total de legado, não como simples desativação de rotas.

## Regra de negócio a implementar

### Regra geral

- Academias não podem criar sumários.
- Academias não podem listar sumários.
- Academias não podem consultar sumário por ID.
- Academias não podem atualizar sumários.
- Academias não podem remover sumários.
- Faltas não podem receber `sumario_id` no payload de criação.
- Faltas não podem receber `sumario_id` no payload de atualização.
- Faltas não devem expor `sumario_id` nem `sumario_titulo` nas respostas.
- O backend não deve validar, buscar, carregar, projetar ou persistir dados de sumário.
- Não deve existir modo de compatibilidade, alias, fallback ou aceitação silenciosa de campos legados de sumário.

### Comportamento esperado para clientes

- Chamadas para endpoints antigos de sumários devem deixar de existir no roteamento.
- Payloads de falta contendo `sumario_id`, `sumario_titulo` ou campos equivalentes devem falhar com erro de validação por campo não suportado, se o padrão atual da API rejeitar campos desconhecidos.
- Se o padrão atual da API não rejeita campos desconhecidos, a tarefa deve incluir ajuste para rejeitar explicitamente campos legados de sumário nas rotas de faltas, evitando aceitação silenciosa.
- Respostas de faltas devem conter apenas os dados próprios da falta e seus vínculos acadêmicos válidos, sem snapshots ou referências a sumários.

## Escopo dos ajustes necessários

### Rotas e handlers

Remover totalmente as rotas de sumários do servidor:

- `GET /academia/sumarios`
- `GET /academia/sumarios/:id`
- `POST /academia/sumarios`
- `PUT /academia/sumarios/:id`
- `DELETE /academia/sumarios/:id`

Remover os handlers, requests, validações auxiliares e qualquer função dedicada ao fluxo de sumários. A remoção não deve ser substituída por stubs, aliases ou respostas de compatibilidade.

### Domínio e eventos

Remover o agregado de sumário/aula e seus eventos de domínio, incluindo criação, atualização e desativação.

Também remover o registro desse tipo de agregado em fábricas, carregadores, allowlists de eventos, safe queries, testes de allowlist e qualquer outro ponto que reconheça `SumarioAula` ou eventos equivalentes.

### Projeções

Remover a projeção de sumários do gerenciador de projeções e apagar o código responsável por rebuild, checkpoint, scan, DTOs e consultas de sumários.

O sistema não deve continuar processando eventos de sumário nem manter projeção ativa apenas para histórico.

### Faltas

Remover das faltas qualquer dependência de sumários:

- campos de request `sumario_id` e equivalentes;
- validação de existência ou compatibilidade de sumário;
- busca em projeção de sumários;
- persistência de `sumario_id` e `sumario_titulo` nos eventos de falta;
- campos `SumarioID` e `SumarioTitulo` no agregado/eventos de falta;
- colunas, scans e DTOs de projeção de falta ligados a sumário;
- exposição desses campos nas respostas da API;
- mensagens de erro que mencionem sumário.

A criação e atualização de faltas devem continuar validando estudante, matéria, ano acadêmico, quantidade, data e demais regras atuais, mas sem qualquer vínculo com sumário.

### Banco de dados e migrações

Criar migração de limpeza que remova a estrutura operacional de sumários e o vínculo nas faltas, incluindo:

- tabela `projection_sumarios_aulas`;
- índices específicos de sumários;
- coluna `projection_faltas.sumario_id`;
- coluna `projection_faltas.sumario_titulo`;
- índices e comentários relacionados.

Se o projeto mantiver migrações antigas imutáveis, não editar a migração histórica que introduziu sumários; criar uma nova migração posterior para desfazer a estrutura. Se o padrão do projeto permitir reescrever migrações ainda não publicadas, confirmar antes de alterar histórico.

### Documentação

Atualizar a documentação da API e a documentação funcional para remover:

- seção de Sumários/Aulas;
- exemplos de criação, listagem, atualização e remoção de sumários;
- menções de `sumario_id` e `sumario_titulo` em faltas;
- regras de preservação histórica específicas de sumários;
- qualquer descrição indicando que faltas podem apontar para sumários.

A documentação final deve deixar claro que faltas são independentes e não aceitam vínculo com sumário.

## Compatibilidade com clientes existentes

Não deve haver compatibilidade retroativa para sumários.

- Não manter endpoints antigos retornando `410 Gone` apenas para preservar contrato.
- Não aceitar payloads antigos de falta com `sumario_id` como no-op.
- Não manter aliases como `aula`, `conteudo_ministrado`, `resumo`, `summary` ou outro nome para o mesmo conceito.
- Não manter DTOs ou campos JSON obsoletos para evitar quebra de cliente.

Clientes integrados devem remover o uso de sumários e adaptar o lançamento de faltas para enviar somente dados próprios da falta.

## Fora de escopo

- Criar uma entidade substituta para sumário/aula.
- Criar novo fluxo de conteúdo ministrado.
- Migrar dados históricos de sumários para outra entidade.
- Preservar consulta administrativa de sumários antigos.
- Manter vínculo histórico entre faltas e sumários.
- Implementar aliases ou modo legado temporário.

## Validações obrigatórias após a mudança

- Criar falta sem `sumario_id` deve continuar funcionando quando os demais campos forem válidos.
- Criar falta com `sumario_id` deve falhar por campo não suportado ou validação explícita.
- Atualizar falta sem `sumario_id` deve continuar funcionando quando os demais campos forem válidos.
- Atualizar falta com `sumario_id` deve falhar por campo não suportado ou validação explícita.
- Listar faltas não deve retornar `sumario_id` nem `sumario_titulo`.
- Consultar falta por ID não deve retornar `sumario_id` nem `sumario_titulo`.
- Rebuild de projeções não deve registrar nem processar projeção de sumários.
- Inicialização do servidor não deve registrar rotas de sumários.
- Safe queries e allowlists não devem aceitar agregado ou eventos de sumário.
- Migrações devem remover colunas e tabela de sumários sem quebrar a estrutura restante de faltas.

## Fluxo operacional proposto

1. Remover as rotas de sumários do servidor.
2. Remover handlers, requests e validações de sumários.
3. Remover agregado, eventos e registros de `SumarioAula`.
4. Remover projeção de sumários e seu registro no gerenciador.
5. Remover campos de sumário dos eventos, agregados, handlers, projeções e DTOs de faltas.
6. Adicionar validação explícita para rejeitar campos legados de sumário em payloads de faltas, se necessário.
7. Criar migração de banco para apagar tabela, índices e colunas relacionados.
8. Atualizar documentação removendo qualquer contrato público de sumários.
9. Atualizar ou remover testes que assumiam existência de sumários.
10. Executar testes e verificações de build para garantir que não restem referências funcionais ao conceito.

## Impactos esperados

- A academia deixa de ter qualquer operação de sumário no backend.
- Faltas passam a ser lançadas e atualizadas sem dependência de sumário.
- O contrato da API fica menor e sem campos legados relacionados a sumários.
- O banco deixa de manter estrutura operacional para sumários.
- O código deixa de carregar, projetar ou validar a entidade removida.
- Clientes que ainda dependem de sumários precisarão remover essa integração.

## Documentação da API

Atualizar a documentação para refletir que:

- não existe mais recurso de Sumários/Aulas;
- faltas não aceitam `sumario_id`;
- faltas não retornam `sumario_titulo`;
- exemplos de payload de falta devem conter somente campos suportados;
- qualquer menção à preservação histórica de sumários deve ser removida.

## Testes recomendados

### Rotas de sumários

- `POST /academia/sumarios` não deve estar registrado.
- `GET /academia/sumarios` não deve estar registrado.
- `GET /academia/sumarios/:id` não deve estar registrado.
- `PUT /academia/sumarios/:id` não deve estar registrado.
- `DELETE /academia/sumarios/:id` não deve estar registrado.

### Faltas

- Criar falta com payload válido e sem `sumario_id`: deve passar.
- Criar falta com `sumario_id`: deve falhar.
- Criar falta com `sumario_titulo`: deve falhar.
- Atualizar falta com payload válido e sem `sumario_id`: deve passar.
- Atualizar falta com `sumario_id`: deve falhar.
- Atualizar falta com `sumario_titulo`: deve falhar.
- Listar faltas deve omitir campos de sumário.
- Consultar falta específica deve omitir campos de sumário.

### Projeções e eventos

- Rebuild de projeções deve concluir sem projeção de sumários.
- Testes de safe queries devem confirmar que `SumarioAula`, `SumarioAulaCriado`, `SumarioAulaAtualizado` e `SumarioAulaDesativado` não são aceitos.
- Busca textual no código por `sumario`, `sumários`, `SumarioAula`, `sumario_id` e `sumario_titulo` deve retornar apenas referências não funcionais inevitáveis, como a própria tarefa ou migrações históricas imutáveis.

### Banco de dados

- Migração deve remover `projection_sumarios_aulas`.
- Migração deve remover `projection_faltas.sumario_id`.
- Migração deve remover `projection_faltas.sumario_titulo`.
- Consultas de faltas devem continuar funcionando após a migração.
