---
modificado: 2026-07-19 00:00
criado: 2026-07-19 00:00
---
# Depurar bloqueio de finalização de ano letivo fora do ano final do período

Tarefa: [[03 - Bloquear finalização de ano letivo fora do ano final do período]]

## Objetivo do debug

Auditar criticamente a implementação da tarefa `docs/Tarefas feitas/03 - Bloquear finalização de ano letivo fora do ano final do período.md` e confirmar, no código real, que `POST /academia/anos-letivos/finalizar` não aceita finalizar um ano letivo quando o ano civil atual é diferente do segundo componente do `ano_letivo` ativo da academia.

Este debug também verifica se a regra mensal já existente permanece inalterada: a janela mensal continua inclusiva no mês final do período letivo e exclusiva no mês inicial do período letivo seguinte, usando os períodos fixos `escolar -> 09_07` e `superior -> 10_07`.

## Investigação executada

### 1. Handler da rota de finalização

O handler `FinalizarAnoLetivoAcademia` usa o `ano_letivo` ativo da academia como fonte da verdade. O campo opcional `ano_letivo` do payload só é aceito quando coincide com esse valor ativo; caso contrário, a requisição é rejeitada antes da finalização.

A auditoria confirmou que, antes de gravar os eventos de finalização e avanço para o próximo ano letivo, o handler chama `validarDataAtualPermiteFinalizacaoAnoLetivo(getDbClient(c), tipo, ano, time.Now())`, passando exatamente o `ano_letivo` ativo da academia.

### 2. Validação centralizada de ano final

A função `anoFinalAnoLetivo` reaproveita `parseAnoLetivo`, garantindo que o ano final seja extraído do formato validado `YYYY_YYYY` e que o segundo ano continue sendo exatamente o primeiro ano + 1.

A validação `validarDataAtualPermiteFinalizacaoAnoLetivo` compara `agora.UTC().Year()` com esse ano final. Quando os anos divergem, retorna erro específico mencionando o ano atual e o ano final esperado.

### 3. Preservação da janela mensal

Depois da validação de ano, a função mantém o cálculo de mês com o período fixo do tipo:

- `superior -> 10_07`: permite julho, agosto e setembro do ano final;
- `escolar -> 09_07`: permite julho e agosto do ano final.

A auditoria confirmou que a função de decisão mensal `mesPermiteFinalizacaoAnoLetivo(mesAtual, mesFim, mesInicio)` permanece com a regra `mesAtual >= mesFim && mesAtual < mesInicio`, preservando a inclusão do mês final e a exclusão do mês inicial.

### 4. Mensagens de erro

Foram verificadas mensagens distintas para os dois bloqueios:

- falha de ano: `não é possível finalizar o ano letivo 2025_2026: o ano atual (2025) não é o ano final do período letivo (2026)`;
- falha de mês: `não é possível finalizar o ano letivo 2025_2026: fora da janela mensal de finalização; permitido apenas em ... de 2026`.

Isso atende ao critério de diagnóstico claro entre erro de ano e erro de janela mensal.

### 5. Testes automatizados

O arquivo `internal/handlers/ano_letivo_helpers_test.go` cobre a regra combinada de ano e mês para `ano_letivo=2025_2026`, incluindo:

- superior permitido em julho, agosto e setembro de 2026;
- superior rejeitado em julho de 2025 e agosto de 2027 por ano incorreto;
- superior rejeitado em outubro de 2026 por mês fora da janela;
- escolar permitido em julho e agosto de 2026;
- escolar rejeitado em setembro de 2026 porque, para `periodo=09_07`, setembro é o mês inicial do próximo período e permanece exclusivo.

## Ajuste feito durante este debug

A implementação de código e testes já estava presente e consistente com a regra de negócio. A lacuna encontrada foi documental: o manual operacional listava a rota `POST /academia/anos-letivos/finalizar`, mas não explicava a nova restrição de ano final nem a janela mensal por tipo.

Por isso, o manual foi atualizado para deixar explícito que a finalização só pode ocorrer quando o ano atual é o ano final do `ano_letivo` ativo da academia e dentro da janela mensal fixa do tipo.

## Resultado do debug

A tarefa está corretamente implementada no backend:

- o handler usa o `ano_letivo` ativo da academia;
- a validação de ano é obrigatória e ocorre antes da validação mensal;
- a validação mensal não foi enfraquecida;
- as mensagens de erro distinguem ano incorreto de mês fora da janela;
- os testes unitários cobrem os cenários centrais da atualização;
- a documentação operacional foi complementada neste ciclo.
