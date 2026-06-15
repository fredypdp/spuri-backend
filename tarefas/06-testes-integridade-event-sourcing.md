# Testar validação de integridade dos dados no Event Sourcing

## Objetivo

Garantir que a cadeia de eventos (`spuri_ledger`) não possa ser adulterada sem detecção e que rebuilds/projeções dependam de uma verificação de integridade confiável.

## Estado atual observado

O sistema usa `spuri_ledger` como fonte de verdade. Cada evento possui `ledger_hash` e `previous_hash`, e existe função SQL `verify_hash_chain(UUID)`. O endpoint `GET /verificar-integridade/:codigo` consulta a integridade do aggregate de estudante. O rebuild de projeções deve abortar se encontrar ledger inválido.

## Escopo dos testes

### Testes unitários

- Hash gerado deve mudar quando payload muda.
- `previous_hash` do evento N deve ser igual ao `ledger_hash` do evento N-1.
- Evento com aggregate inválido deve ser rejeitado.
- Event type desconhecido deve ser rejeitado por `ValidateEventType`.

### Testes de integração com banco

- Criar estudante e registrar nota/falta.
- Verificar integridade retorna `true`.
- Adulterar payload diretamente no banco em ambiente de teste, desabilitando proteções apenas no setup controlado se necessário.
- Verificar integridade retorna `false` e informa versão quebrada.
- Restaurar payload e verificar novamente.

### Testes de rebuild

- Rebuild com ledger íntegro deve passar.
- Rebuild com ledger adulterado deve abortar antes de truncar/recriar projeções.
- Checkpoint não deve avançar quando projection handler falha em evento inválido.

### Testes de autorização

- Estudante só verifica o próprio código.
- Academia só verifica estudantes da própria academia.
- Admin verifica qualquer estudante.

## Cenários específicos

1. **Adulteração de payload**: alterar nota de 10 para 20 no JSON do evento.
2. **Adulteração de metadata**: mudar `user_id` no metadata.
3. **Remoção lógica impossível**: tentar `DELETE` no ledger deve falhar por trigger.
4. **Update impossível**: tentar `UPDATE spuri_ledger SET payload=...` deve falhar por trigger.
5. **Quebra de ordem**: inserir evento manual com `previous_hash` inconsistente deve ser impossível pela rotina normal e detectado se ocorrer.
6. **Replay determinístico**: rebuild após sequência válida gera mesmas projeções.

## Estratégia prática

- Usar banco de teste isolado.
- Criar helper que semeia aggregate com eventos reais via repository, nunca inserção manual.
- Para testar adulteração, usar transação de teste e mecanismo controlado para desabilitar trigger somente no ambiente de teste, ou criar fixture SQL específica.
- Nunca incluir esse bypass em código de produção.

## Critérios de aceite

- Endpoint de integridade retorna mensagem clara para íntegro e corrompido.
- Qualquer alteração em payload/metadata/event_type/event_version altera o hash esperado.
- Rebuild não executa quando há aggregate inválido.
- Testes documentam que o ledger é imutável por `UPDATE`, `DELETE` e `TRUNCATE`.

## Comandos esperados

- `go test ./internal/db/...`
- `go test ./internal/handlers/... -run Integridade`
- `go test ./internal/projections/... -run Rebuild`
- `go test ./...` em pipeline completo.
