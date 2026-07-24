# Depurar validações na edição de dados cadastrais dos usuários

Tarefa auditada: `docs/Tarefas feitas/06 - Reforçar validações na edição de dados cadastrais dos usuários.md`

## Objetivo do debug

Auditar criticamente a implementação da tarefa 06, seguindo o padrão dos demais debugs do repositório: confirmar no código real se os campos sensíveis foram removidos das rotas genéricas, se alterações efetivas de contato resetam as flags de verificação, se a edição de administradores respeita hierarquia estrita e, quando houver lacuna, completar a implementação no mesmo ciclo.

## Resultado da auditoria

A auditoria confirmou que `PUT /academia/dados` chama `rejectAcademiaDadosRestrictedFields` antes de fazer bind do payload tipado, rejeitando explicitamente `telefone`, `email`, `anos_academicos`, `cursos`, `type`, `nivel_escolar` e `nif` com `400`, `VALIDATION_ERROR`, `campo_nao_permitido` e mensagem orientando a rota ou fluxo correto. Como a validação ocorre antes do carregamento do aggregate e antes de `SaveWithAudit`, a operação é atômica: payload misto com campo permitido e campo proibido não gera mutação parcial.

Também foi confirmado que as rotas genéricas de estudante e admin foram atualizadas posteriormente pela tarefa 12 para rejeitar `email` e `telefone` via `rejectDedicatedContactFields`, direcionando alterações para `PUT /me/email` e `PUT /me/telefone`. As rotas dedicadas usam o aggregate correto pelo tipo do usuário autenticado e reaproveitam os métodos que calculam `EmailAlterado`, `TelefoneAlterado` e, no estudante, `TelefoneEncAlterado` somente quando há mudança efetiva de valor.

A edição de dados de admin (`PUT /dominis/admin/:id/dados`) carrega o admin alvo e, quando não é autoedição, carrega o executor e chama `ValidatePermission(admin.Role)`. Essa regra mantém a hierarquia estrita: `fpp` pode gerir `adm` e `gerente`, `adm` pode gerir `gerente`, e `gerente` não pode gerir `fpp`, `adm` nem outro `gerente`.

## Ajuste feito durante este debug

Foi encontrada uma lacuna na projeção de estudantes: o aggregate `Estudante` já emitia `TelefoneEncAlterado` e aplicava o reset de `TelefoneEncarregadoVerificado` em memória, mas `EstudanteProjection.handleDadosPessoaisAtualizados` não lia esse campo do payload e, portanto, não resetava `telefone_encarregado_verificado` no banco durante processamento normal de eventos ou rebuild de projeções.

A correção adicionou `TelefoneEncAlterado` ao payload lido pela projeção e inclui `telefone_encarregado_verificado = FALSE` quando o evento indicar alteração efetiva do telefone do encarregado.

## Evidências verificadas

- `PUT /academia/dados` permite apenas `nome`, `provincia`, `endereco` e `website`.
- `PUT /academia/dados` rejeita campos com rotas dedicadas ou fluxo documental próprio antes de qualquer mutação.
- Rotas genéricas de estudante e admin rejeitam `email` e `telefone` e orientam para as rotas dedicadas.
- Aggregates de academia, estudante e admin calculam alteração efetiva antes de resetar flags.
- Projeções de academia e admin resetam `email_verificado` e `telefone_verificado` quando o evento indica mudança efetiva.
- Projeção de estudante agora reseta `email_verificado`, `telefone_verificado` e `telefone_encarregado_verificado` quando o evento indica mudança efetiva.
- A hierarquia de edição de admin usa a mesma regra estrita de permissões do aggregate.

## Testes executados

- `go test ./internal/domain/aggregates ./internal/handlers ./internal/projections`

## Conclusão

A implementação da tarefa 06 está correta após o ajuste deste debug. A brecha remanescente encontrada estava restrita ao replay/processamento da projeção de estudantes para `telefone_encarregado_verificado`; ela foi corrigida sem reabrir campos sensíveis nas rotas genéricas e sem criar alias, wrapper de compatibilidade ou caminho alternativo.
