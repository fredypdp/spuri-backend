# Testes de integridade dos dados no event sourcing

## Objetivo
Definir uma suíte de testes para garantir que o event store, a hash chain, os aggregates e as projeções mantêm integridade, idempotência e rastreabilidade.

## Contexto
O sistema usa event sourcing com ledger imutável e hash chain. A integridade depende de eventos append-only, versões coerentes, `previous_hash` correto e reconstrução confiável das projeções.

## Áreas que devem ser testadas

### Hash chain
- Inserção de eventos gera `ledger_hash` e `previous_hash` corretamente.
- Primeiro evento de um aggregate não possui `previous_hash` ou usa valor nulo esperado.
- Eventos seguintes apontam para hash do evento anterior.
- Alteração manual de payload quebra a verificação.
- Remoção manual de evento quebra a verificação.
- Troca de ordem/versionamento quebra a verificação.

### Versionamento de aggregate
- Primeiro evento deve ser versão 1.
- Segundo evento deve ser versão 2.
- Não permitir duas escritas concorrentes com mesma versão.
- Repositório deve detectar conflito otimista.
- Reprocessamento idempotente não deve duplicar eventos.

### Projeções
- Rebuild completo recria projeções equivalentes ao estado atual.
- Rebuild deve abortar se qualquer hash chain estiver inválida.
- Erro em uma projeção deve ser registrado sem corromper outras projeções.
- Checkpoints de projeção devem avançar apenas após sucesso.

### Atomicidade
- Evento e projeção crítica não devem ficar em estados contraditórios quando a transação falha.
- Falhas simuladas antes/depois do append devem não duplicar eventos.
- Jobs assíncronos devem manter idempotência por item.

### Auditoria
- Eventos com contexto auditável devem registrar usuário, tipo de usuário, IP e data.
- Ações administrativas sensíveis devem exigir justificativa quando aplicável.
- Eventos de alteração de senha não devem expor senha em texto plano.

## Casos de teste recomendados

### Teste 1: cadeia íntegra após criação de estudante
1. Criar estudante.
2. Carregar eventos do aggregate.
3. Executar verificação de hash chain.
4. Esperar resultado íntegro.

### Teste 2: adulteração de payload
1. Criar estudante e atualizar dados pessoais.
2. Alterar payload diretamente no banco de teste.
3. Executar verificação de hash chain.
4. Esperar falha apontando a versão adulterada.

### Teste 3: adulteração de previous_hash
1. Criar aggregate com ao menos dois eventos.
2. Modificar `previous_hash` do segundo evento.
3. Executar verificação.
4. Esperar erro de cadeia quebrada.

### Teste 4: rebuild bloqueado por ledger inválido
1. Criar eventos válidos.
2. Corromper um evento.
3. Executar rebuild.
4. Esperar bloqueio antes de truncar/recriar projeções.

### Teste 5: concorrência de versão
1. Carregar mesmo aggregate em duas transações.
2. Salvar alteração A.
3. Tentar salvar alteração B com versão antiga.
4. Esperar erro de concorrência e nenhum evento duplicado.

### Teste 6: idempotência de nota/falta/avaliação
1. Enviar mesma operação duas vezes com mesma chave de idempotência lógica.
2. Confirmar que apenas um evento efetivo é gravado ou que a segunda chamada retorna resposta idempotente.
3. Confirmar que projeção não duplica contadores.

## Ferramentas e comandos sugeridos
- Testes unitários de aggregate com Go.
- Testes de integração com banco PostgreSQL real em container.
- Seeds mínimos por aggregate.
- Função SQL `verify_hash_chain` como oráculo de integridade.

## Critérios de aceite
- Testes falham quando um evento é adulterado.
- Rebuild não executa sobre ledger inválido.
- Conflitos concorrentes são detectados.
- Projeções reconstruídas batem com projeções geradas incrementalmente.
- Testes rodam em CI com banco isolado e dados descartáveis.
