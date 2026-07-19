---
modificado: 2026-07-08 00:00
criado: 2026-07-08 00:00
---
# Depurar remoção de edição/exclusão de faltas e notas e validação de período letivo

Tarefa: [[Remover edicao exclusao faltas notas e validar periodo letivo]]

## Objetivo da auditoria

Fazer uma auditoria crítica, completa, extremamente profunda e arquivo por arquivo da implementação da tarefa:

`docs/Lista de tarefas/Remover edicao exclusao faltas notas e validar periodo letivo.md`

Esta é uma tarefa de **depuração orientada**, não um relatório de execução. Ao executá-la futuramente, a pessoa ou agente encarregado deve investigar o código real, confirmar se a tarefa original foi implementada corretamente e, caso qualquer parte esteja incompleta, inconsistente, parcial, sem teste, sem migration, sem validação, sem documentação ou com compatibilidade silenciosa, deve **terminar a implementação e corrigir o que estiver errado** no mesmo ciclo.

A depuração só pode ser considerada concluída quando estiver comprovado que faltas e notas são recursos de criação e leitura, sem qualquer capacidade ativa de edição, exclusão, restauração, alias legado, shim, fallback ou rota equivalente, e quando toda criação de falta validar a data contra o período letivo aplicável da academia antes de qualquer persistência, evento ou projeção.

## Regra oficial obrigatória

A implementação final deve obedecer exatamente à decisão de produto abaixo:

- faltas só podem ser criadas e consultadas;
- notas só podem ser criadas e consultadas;
- não existe endpoint `PUT`, `PATCH` ou `DELETE` funcional para faltas;
- não existe endpoint `PUT`, `PATCH` ou `DELETE` funcional para notas;
- não existe handler, comando, caso de uso, service, aggregate, método auxiliar, permissão ou rota alternativa que edite, elimine, restaure ou faça soft delete operacional de faltas e notas;
- não criar nem manter aliases, rotas antigas, wrappers de compatibilidade, respostas `410 Gone`, feature flags, fallbacks temporários ou código morto para os fluxos removidos;
- a criação de falta deve validar a data dentro do período letivo inclusivo do ano letivo aplicável à matéria/disciplina;
- falta escolar deve usar o período fixo do ano letivo escolar da academia;
- falta superior deve usar o período fixo do ano letivo superior da academia;
- o tipo letivo usado na validação deve ser inferido pelo domínio a partir da matéria/disciplina, curso, ano acadêmico, turma, vínculo ou estrutura equivalente, e nunca por campo manipulável do payload;
- datas fora do período letivo devem ser rejeitadas antes de registrar evento, inserir projeção ou alterar qualquer estado;
- documentação funcional, documentação de API, Swagger/OpenAPI, manuais, exemplos e tarefas atuais devem refletir a nova versão do código.

## Resultado esperado da depuração

Ao executar este debug, ele só poderá ser encerrado quando estiver garantido que:

- todas as rotas de edição/exclusão de faltas e notas foram removidas do roteador;
- não há métodos HTTP mutáveis além de `POST` para criar faltas/notas e `GET` para ler/listar faltas/notas;
- não há handler ativo, DTO, comando, service, aggregate ou repository usado para editar/excluir faltas ou notas;
- permissões, policies, middlewares e escopos de edição/exclusão foram removidos ou deixaram de ser referenciados por fluxo ativo;
- testes confirmam a inexistência funcional das rotas antigas;
- criação e leitura de faltas continuam funcionando;
- criação e leitura de notas continuam funcionando;
- toda criação de falta valida a data contra o período letivo correto, inclusivo, antes de persistir;
- a validação de período não aceita bypass por payload, endpoint administrativo, batch, importação ou rota alternativa;
- mensagens de erro para datas fora do período são claras, seguras e consistentes com o padrão do projeto;
- não há soft delete operacional ativo substituindo a exclusão removida;
- migrations, schema, seeds, fixtures e rebuild/replay não reintroduzem mutações proibidas;
- documentação atual está de acordo com a nova versão do código e não descreve edição/exclusão como fluxo vigente, legado, depreciado ou compatível;
- a tarefa original recebe o sufixo `(feito)` no **título interno do Markdown**, não no nome do arquivo, e é movida de `docs/Lista de tarefas/` para `docs/Tarefas feitas/` somente depois de tudo estar implementado, testado e documentado.

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. registro de rotas HTTP, grupos versionados, middlewares e wiring do servidor;
2. handlers de faltas e notas, incluindo criação, leitura, listagem, atualização, exclusão, restauração, batch e rotas administrativas;
3. DTOs de request/response, validadores, serializers e normalizadores relacionados a faltas e notas;
4. services, use cases, repositories, helpers e funções utilitárias usadas por faltas e notas;
5. aggregates, comandos, eventos, applies, snapshots e reconstrução de estado para faltas e notas;
6. projections, queries SQL, modelos de leitura, filtros de listagem e relatórios de frequência/notas;
7. migrations que criam, alteram, fazem soft delete, auditoria, histórico ou constraints de faltas e notas;
8. permissões, roles, policies, middlewares e mensagens de erro ligadas a editar/excluir faltas ou notas;
9. jobs, batch, importações, scripts operacionais, seeds e rotinas de manutenção;
10. testes unitários, testes de handler, testes de integração, fixtures, builders e mocks;
11. cálculo de período letivo, anos letivos escolares/superiores, período fixo por tipo e funções de intervalo de datas;
12. inferência do tipo letivo da matéria/disciplina e vínculo com curso, ano acadêmico, turma e academia;
13. documentação funcional, documentação de API, Swagger/OpenAPI, README, manuais e exemplos JSON;
14. tarefas atuais em `docs/Lista de tarefas/` e documentos históricos que possam estar sendo usados como referência ativa;
15. comportamento esperado para clientes antigos que tentem enviar `PUT`, `PATCH` ou `DELETE`.

## Checklist obrigatório de validação

### 1. Rotas e superfície HTTP

Auditar o roteador e confirmar que a superfície pública foi realmente reduzida.

Validar que:

- não existe rota ativa para editar falta;
- não existe rota ativa para excluir falta;
- não existe rota ativa para restaurar falta;
- não existe rota ativa para editar nota;
- não existe rota ativa para excluir nota;
- não existe rota ativa para restaurar nota;
- não existe rota equivalente usando outro caminho, alias, pluralização alternativa, rota administrativa ou endpoint de manutenção;
- `POST` continua disponível apenas para criação de falta e nota;
- `GET` continua disponível apenas para consulta/listagem de falta e nota;
- chamadas às rotas antigas têm comportamento natural de rota inexistente ou método não permitido conforme o roteador, sem handler legado customizado;
- Swagger/OpenAPI não expõe `PUT`, `PATCH` ou `DELETE` para faltas/notas;
- testes cobrem explicitamente que rotas antigas não estão registradas.

### 2. Handlers, DTOs e contratos de entrada

Auditar todos os handlers e contratos públicos relacionados a faltas e notas.

Validar que foram removidos:

- handlers de update de falta;
- handlers de delete de falta;
- handlers de restore de falta;
- handlers de update de nota;
- handlers de delete de nota;
- handlers de restore de nota;
- DTOs de edição/exclusão/restauração;
- campos de request usados apenas por mutações antigas, como motivo de exclusão, justificativa de edição, deleted_at, restaurado_por ou equivalentes;
- validações exclusivas dos fluxos removidos;
- mensagens de erro que orientem editar, apagar, restaurar ou corrigir faltas/notas por mutação.

Também validar que:

- criação de falta e nota mantém validação de contrato completa;
- leitura/listagem não depende de campos removidos;
- payloads antigos não são aceitos silenciosamente por wrappers de compatibilidade;
- documentação dos contratos descreve apenas criação e leitura.

### 3. Domínio, comandos, eventos e snapshots

Auditar profundamente o domínio e a arquitetura orientada a eventos, se aplicável.

Validar que:

- comandos novos de edição/exclusão de faltas e notas não existem em fluxo ativo;
- eventos novos de edição/exclusão de faltas e notas foram removidos quando não forem necessários;
- applies exclusivos de eventos removidos não ficam como código morto;
- snapshots atuais não carregam campos de mutação removida;
- replay/rebuild não depende de eventos novos de edição/exclusão;
- se algum evento histórico precisar permanecer por motivo técnico real, isso está isolado como leitura histórica interna, sem endpoint público e com justificativa explícita na entrega;
- não existe método auxiliar como `AtualizarFalta`, `ExcluirFalta`, `RestaurarFalta`, `AtualizarNota`, `ExcluirNota`, `RestaurarNota` ou equivalente em uso ativo;
- logs e auditorias não indiquem suporte vigente a edição/exclusão.

### 4. Persistência, migrations e soft delete

Auditar schema, migrations e queries.

Validar que:

- não existe coluna, constraint, trigger, função, view ou query ativa que implemente exclusão operacional de faltas/notas como regra vigente;
- campos como `deleted_at`, `deletado_em`, `deleted_by`, `deletado_por`, `motivo_exclusao`, `motivo_delecao`, `updated_by`, `editado_por` ou equivalentes foram removidos quando forem exclusivos dos fluxos removidos, ou justificados como histórico técnico não funcional;
- queries padrão não filtram registros deletados para esconder soft delete ainda funcional;
- repositories não possuem métodos públicos de update/delete para faltas/notas;
- migrations novas são idempotentes quando esse for o padrão do projeto;
- migrations históricas podem permanecer como registro, mas não podem justificar suporte ativo;
- seeds, fixtures e scripts não criam estados deletados/editados para faltas/notas;
- rebuild de projections não recria capacidade de editar/excluir.

### 5. Permissões, policies e segurança

Auditar permissões e autorização.

Validar que:

- permissões específicas de editar falta foram removidas ou deixaram de ser referenciadas;
- permissões específicas de excluir falta foram removidas ou deixaram de ser referenciadas;
- permissões específicas de editar nota foram removidas ou deixaram de ser referenciadas;
- permissões específicas de excluir nota foram removidas ou deixaram de ser referenciadas;
- nenhum perfil privilegiado consegue executar as ações removidas por rota alternativa;
- middlewares não registram exceções para mutações antigas;
- testes garantem que ausência de rota ocorre antes de qualquer autorização permissiva;
- documentação de perfis não lista capacidades de editar/excluir faltas ou notas.

### 6. Validação de período letivo na criação de faltas

Auditar o fluxo completo de criação de falta desde o payload até a persistência.

Validar que:

- a validação ocorre antes de registrar evento, inserir linha, atualizar projection ou disparar job;
- a academia é identificada de forma confiável;
- a matéria/disciplina é carregada e validada como pertencente à academia;
- matéria inexistente ou de outra academia é rejeitada;
- o tipo letivo aplicável é inferido pelo domínio, não por payload manipulável;
- o ano letivo vigente correto é carregado para o tipo inferido;
- falta escolar usa o período escolar fixo da academia;
- falta superior usa o período superior fixo da academia;
- o intervalo é inclusivo em início e fim;
- datas anteriores ao início são rejeitadas;
- datas posteriores ao fim são rejeitadas;
- datas no primeiro dia e no último dia são aceitas;
- timezone, parsing de data e truncamento de hora não criam rejeições indevidas;
- mensagem de erro informa que a data está fora do período letivo aplicável sem vazar dados sensíveis;
- batch, importação e endpoints administrativos passam pela mesma validação;
- testes cobrem sucesso e falha para escolar e superior.

### 7. Integração com anos letivos fixos

Auditar a integração com a regra de período fixo dos anos letivos.

Validar que:

- o cálculo da janela escolar respeita o período fixo escolar vigente;
- o cálculo da janela superior respeita o período fixo superior vigente;
- não há leitura de `periodo` enviado pelo cliente como fonte de verdade;
- não há hardcode local divergente em faltas;
- a criação de falta reutiliza a regra central de ano letivo/período quando ela existir;
- academias mistas usam a janela correta conforme o tipo letivo da matéria;
- identificadores de ano letivo inválidos falham com erro claro;
- ausência de ano letivo aplicável falha antes da persistência;
- testes cobrem limites do período escolar e superior.

### 8. Notas: criação e leitura sem mutação posterior

Auditar especificamente notas para garantir que a remoção não quebrou o fluxo permitido.

Validar que:

- criação de nota continua registrando todos os campos obrigatórios;
- leitura/listagem de notas continua retornando dados esperados;
- cálculo de avaliação final, médias, pendências e relatórios usa notas criadas sem exigir edição posterior;
- não existe substituição de edição por novo endpoint de correção disfarçado;
- não existe exclusão lógica de nota ativa;
- testes antigos que validavam edição/exclusão foram removidos ou reescritos para validar a inexistência dessas capacidades;
- documentação orienta que correções retroativas de notas estão fora do escopo desta regra, salvo se houver novo fluxo de produto documentado em tarefa própria.

### 9. Faltas: criação, leitura e relatórios sem mutação posterior

Auditar especificamente faltas para garantir consistência de frequência.

Validar que:

- criação de falta continua funcionando dentro do período letivo;
- leitura/listagem de faltas continua funcionando;
- relatórios de frequência, contadores e projections não dependem de edição/exclusão;
- não existe substituição de exclusão por campo de cancelamento funcional;
- faltas fora do período nunca chegam ao modelo de leitura;
- filtros de listagem não escondem soft delete operacional ativo;
- testes cobrem contagem/listagem depois da remoção dos fluxos mutáveis.

### 10. Documentação obrigatória

Confirmar e corrigir, se necessário, que toda documentação esteja de acordo com a nova versão do código.

Auditar no mínimo:

- documentação de API/OpenAPI/Swagger;
- `docs/Manual de Configuração Inicial da Academia.md`;
- documentação de domínio acadêmico;
- documentação de faltas/frequência;
- documentação de notas/avaliações;
- documentação de permissões/perfis;
- exemplos de payload de criação de falta;
- exemplos de payload de criação de nota;
- exemplos de resposta de falta e nota;
- coleções de API, se existirem;
- guias operacionais;
- tarefas atuais em `docs/Lista de tarefas/`;
- documentos históricos em `docs/Tarefas feitas/`, apenas para garantir que não estejam sendo usados como documentação vigente.

A documentação atual deve deixar explícito que:

- faltas são apenas criadas e consultadas;
- notas são apenas criadas e consultadas;
- não existe edição, exclusão, restauração, soft delete funcional ou endpoint legado para faltas/notas;
- faltas fora do período letivo aplicável são rejeitadas;
- o período letivo usado na validação vem do domínio da academia e do tipo da matéria;
- exemplos não incluem `PUT`, `PATCH` ou `DELETE` para faltas/notas;
- exemplos não descrevem rotas antigas como `deprecated`, `legacy`, `mantidas por compatibilidade` ou equivalentes;
- a documentação está sincronizada com a versão real do código.

### 11. Testes obrigatórios

Criar ou ajustar testes cobrindo, no mínimo:

1. rota antiga de editar falta não registrada;
2. rota antiga de excluir falta não registrada;
3. rota antiga de restaurar falta não registrada, se existia;
4. rota antiga de editar nota não registrada;
5. rota antiga de excluir nota não registrada;
6. rota antiga de restaurar nota não registrada, se existia;
7. criação de falta continua funcionando;
8. leitura/listagem de faltas continua funcionando;
9. criação de nota continua funcionando;
10. leitura/listagem de notas continua funcionando;
11. não existe permissão funcional para editar falta;
12. não existe permissão funcional para excluir falta;
13. não existe permissão funcional para editar nota;
14. não existe permissão funcional para excluir nota;
15. criação de falta escolar dentro do período escolar com sucesso;
16. criação de falta escolar antes do início do período escolar rejeitada;
17. criação de falta escolar depois do fim do período escolar rejeitada;
18. criação de falta escolar exatamente no primeiro dia do período aceita;
19. criação de falta escolar exatamente no último dia do período aceita;
20. criação de falta superior dentro do período superior com sucesso;
21. criação de falta superior antes do início do período superior rejeitada;
22. criação de falta superior depois do fim do período superior rejeitada;
23. criação de falta superior exatamente no primeiro dia do período aceita;
24. criação de falta superior exatamente no último dia do período aceita;
25. matéria inexistente rejeita criação de falta;
26. matéria de outra academia rejeita criação de falta;
27. tentativa de manipular tipo letivo pelo payload não altera a inferência do domínio;
28. batch/importação, se existir, valida o mesmo período;
29. replay/projeção continua consistente sem novos eventos de edição/exclusão;
30. documentação/contratos não expõem métodos removidos.

## Busca ampla obrigatória

Fazer busca ampla e classificar cada ocorrência encontrada como válida, histórica/documental aceitável ou bug ativo. No mínimo, buscar por:

```bash
rg -n "faltas|falta|notas|nota|frequencia|frequência|avaliacao|avaliação" .
rg -n "PUT|PATCH|DELETE|Update|Atualizar|Editar|Edit|Delete|Deletar|Excluir|Remover|Restore|Restaurar" internal cmd docs migrations .
rg -n "deleted_at|deletado|deletada|deleted_by|deletado_por|motivo_exclusao|motivo_delecao|soft delete|soft_delete|restaur" .
rg -n "periodo|período|ano_letivo|anos_letivos|09_07|10_07|2025_2026|escolar|superior" internal cmd migrations docs
rg -n "CreateFalta|CriarFalta|AtualizarFalta|ExcluirFalta|CreateNota|CriarNota|AtualizarNota|ExcluirNota|UpdateFalta|DeleteFalta|UpdateNota|DeleteNota" .
```

Também pesquisar rotas e métodos HTTP de forma contextual:

```bash
rg -n "faltas|notas" cmd internal
rg -n "\.PUT|\.Patch|\.PATCH|\.DELETE|\.Delete|router\.Put|router\.Patch|router\.Delete|HandleFunc\(.*PUT|HandleFunc\(.*PATCH|HandleFunc\(.*DELETE" cmd internal
```

Para cada ocorrência relacionada aos fluxos removidos ou à validação de período, classificar como uma das opções:

- código ativo a remover;
- código ativo a corrigir;
- teste/fixture a atualizar;
- documentação atual a corrigir;
- migration histórica aceitável;
- tarefa histórica em `docs/Tarefas feitas` aceitável apenas como registro do passado;
- falso positivo sem relação com edição/exclusão de faltas/notas ou período letivo.

Não basta listar ocorrências. Cada ocorrência relevante deve ser analisada no contexto do arquivo, do fluxo de execução e do contrato público.

## Correções esperadas quando houver divergência

Se a auditoria encontrar qualquer divergência, implementar a correção no mesmo ciclo de depuração. Exemplos de correções esperadas:

1. remover rotas `PUT`, `PATCH` e `DELETE` de faltas/notas;
2. remover handlers, DTOs, commands e methods de edição/exclusão/restauração;
3. remover permissões e policies de mutações antigas;
4. remover ou isolar eventos antigos que não devem ser usados por fluxo ativo;
5. ajustar repositories para não expor update/delete operacional;
6. criar migration para remover colunas/índices/constraints de soft delete quando forem exclusivos e ativos;
7. corrigir criação de falta para validar período antes da persistência;
8. centralizar ou reutilizar cálculo de janela letiva escolar/superior;
9. impedir bypass por payload de tipo letivo ou período;
10. ajustar batch, importação e rotas administrativas para usar as mesmas regras;
11. atualizar testes, fixtures e builders;
12. atualizar documentação de API, domínio, permissões e manuais;
13. mover a tarefa original para feitas somente após código, testes e documentação estarem concluídos.

## Comandos mínimos de validação esperados

Ao executar este debug futuramente, rodar no mínimo:

```bash
rg -n "faltas|falta|notas|nota|frequencia|frequência|avaliacao|avaliação" .
rg -n "PUT|PATCH|DELETE|Update|Atualizar|Editar|Edit|Delete|Deletar|Excluir|Remover|Restore|Restaurar" internal cmd docs migrations .
rg -n "deleted_at|deletado|deletada|deleted_by|deletado_por|motivo_exclusao|motivo_delecao|soft delete|soft_delete|restaur" .
rg -n "periodo|período|ano_letivo|anos_letivos|09_07|10_07|2025_2026|escolar|superior" internal cmd migrations docs
rg -n "CreateFalta|CriarFalta|AtualizarFalta|ExcluirFalta|CreateNota|CriarNota|AtualizarNota|ExcluirNota|UpdateFalta|DeleteFalta|UpdateNota|DeleteNota" .
rg -n "\.PUT|\.Patch|\.PATCH|\.DELETE|\.Delete|router\.Put|router\.Patch|router\.Delete|HandleFunc\(.*PUT|HandleFunc\(.*PATCH|HandleFunc\(.*DELETE" cmd internal
go test ./...
```

Se algum comando não puder ser executado por limitação de ambiente, registrar a limitação explicitamente na entrega.

## Critério de aceite final

A depuração só pode ser considerada concluída quando:

- todos os itens do checklist forem verificados;
- cada bug encontrado for corrigido no código, nas migrations, nos testes e/ou na documentação;
- a suíte relevante de testes passar;
- não existir endpoint `PUT`, `PATCH` ou `DELETE` funcional para faltas;
- não existir endpoint `PUT`, `PATCH` ou `DELETE` funcional para notas;
- não existir handler/comando ativo para editar, excluir ou restaurar faltas;
- não existir handler/comando ativo para editar, excluir ou restaurar notas;
- criação e leitura de faltas continuarem funcionando;
- criação e leitura de notas continuarem funcionando;
- toda criação de falta validar data dentro do período letivo escolar/superior aplicável;
- datas fora do período forem rejeitadas antes da persistência/evento;
- matéria inexistente ou de outra academia for rejeitada antes da persistência/evento;
- payloads não puderem manipular tipo letivo, período ou ano letivo para burlar a validação;
- batch, importações e rotas administrativas não bypassarem a regra;
- documentação de API, domínio, permissões e manuais estiverem atualizadas sem referência a suporte legado;
- OpenAPI/Swagger não expuser rotas removidas;
- testes automatizados cobrirem remoção de rotas e validação de período;
- não houver aliases, shims, fallbacks, feature flags ou código morto dos fluxos removidos;
- ocorrências restantes de edição/exclusão de faltas/notas estiverem restritas a histórico aceitável ou falsos positivos justificados;
- a documentação estiver explicitamente de acordo com a nova versão do código;
- a tarefa original tiver o título interno alterado para `# Remover edição/exclusão de faltas e notas e validar período letivo das faltas (feito)`;
- o arquivo da tarefa original for movido de `docs/Lista de tarefas/` para `docs/Tarefas feitas/`, mantendo o nome do arquivo sem adicionar `(feito)`.

## Entrega esperada da execução futura

Quando este debug for executado, a entrega final deve informar:

- arquivos e camadas auditados;
- rotas encontradas e confirmação dos métodos permitidos/removidos;
- ocorrências encontradas e classificação de cada uma;
- bugs ou lacunas encontrados;
- correções aplicadas;
- migrations criadas ou justificativa para não criar migration;
- testes criados, removidos ou atualizados;
- documentação atualizada;
- confirmação explícita de que a documentação está de acordo com a nova versão do código;
- comandos executados e resultados;
- confirmação explícita de que a tarefa original só foi movida para `docs/Tarefas feitas/` depois de código, testes e documentação estarem concluídos.
