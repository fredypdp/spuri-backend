# Depurar eventos de progressão e status acadêmico do estudante

## Escopo

Depuração da tarefa `docs/Tarefas feitas/05 - Revisar e ampliar eventos de progressão e status acadêmico do estudante.md`, verificando se o contrato novo está refletido no código, nas rotas, no allowlist de eventos e nos testes automatizados.

## Verificações executadas

1. Busquei referências a rotas e eventos legados removidos:
   - `matricula/fundamental`, `matricula/medio`, `matricula/superior`;
   - `interrupcao/fundamental`, `interrupcao/medio`, `trancamento/superior`;
   - `MatriculaFundamentalEfetivada`, `MatriculaMedioEfetivada`, `MatriculaSuperiorEfetivada`;
   - `SuperiorTrancado`;
   - uso funcional de `status = "arquivado"`.
2. Conferi o registro de rotas em `cmd/server/main.go`.
3. Conferi o allowlist de eventos em `internal/db/safe_queries.go`.
4. Conferi a implementação de interrupção, desvinculação e revinculação em `internal/domain/aggregates/estudante.go` e `internal/handlers/academia_status_escolar_handlers.go`.
5. Executei a suíte automatizada completa com `go test ./...`.

## Problemas encontrados

- Os eventos de interrupção (`FundamentalInterrompido`, `MedioInterrompido`, `SuperiorInterrompido`) não carregavam `solicitacao_id`, embora o fluxo exija referência auditável da solicitação aprovada.
- A anotação do `solicitacao_id` no último evento cobria apenas `EstudanteDesvinculadoDaAcademia` e `EstudanteReintegrado`, deixando interrupções sem vínculo explícito com a solicitação.
- A decisão de solicitação não validava se o `:codigo` informado na rota batia com o estudante da solicitação, permitindo uma chamada semanticamente inconsistente mesmo que a solicitação pertencesse à academia.
- Não havia teste de debug dedicado garantindo 404 nas rotas legadas removidas, rejeição dos eventos antigos no allowlist e invariantes principais de status/histórico.

## Correções aplicadas

- Adicionado `SolicitacaoID` aos eventos `FundamentalInterrompidoEvent`, `MedioInterrompidoEvent` e `SuperiorInterrompidoEvent`.
- Atualizada a anotação do último evento para preencher `SolicitacaoID` também nos eventos de interrupção.
- Adicionada validação para rejeitar decisão quando o `:codigo` da rota não corresponde ao `codigo_estudante` da solicitação.
- Adicionados testes de debug para:
  - rotas legadas de matrícula/interrupção/trancamento removidas retornarem `404`;
  - eventos removidos serem rejeitados por `ValidateEventType`;
  - desvinculação definir `status = "inativo"` e preservar histórico acadêmico;
  - interrupção exigir exatamente uma etapa acadêmica em andamento;
  - revinculação exigir estudante inativo.

## Resultado

A implementação está coerente com os critérios principais auditados da tarefa. A suíte completa passou com sucesso em `go test ./...`.
