---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Bloquear finalização de ano letivo fora do ano final do período (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que `POST /academia/anos-letivos/finalizar` só permita a finalização quando o **ano** da data atual for igual ao ano final do `ano_letivo` informado (o segundo componente de `YYYY_YYYY`), além de continuar respeitando a janela mensal já existente derivada do `periodo` fixo do tipo. Ao final, atualize testes e a documentação técnica afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou exceções não documentadas para o bloqueio.

## Contexto

`Documentação.md` (seção 7, "Finalização de ano letivo por academia") já descreve uma validação de janela mensal para `POST /academia/anos-letivos/finalizar`: "A janela mensal é inclusiva no mês final e exclusiva no mês inicial: o mês atual precisa ser maior ou igual ao mês de fim do período letivo e menor que o mês de início do período letivo. Exemplo: se `periodo=10_07`, a finalização é permitida somente em julho, agosto e setembro."

Essa validação, como está descrita, compara **apenas o mês** da data atual contra os meses do `periodo` fixo do tipo (`escolar -> 09_07`, `superior -> 10_07`), sem verificar se o **ano** da data atual corresponde ao ano final do `ano_letivo` que está sendo finalizado. Isso cria uma janela de meses que se repete todo ano civil, sem amarrar essa janela ao ano letivo específico:

- para `ano_letivo=2025_2026` e `periodo=10_07` (superior), o ano final é `2026` e a janela de meses permitida é julho, agosto e setembro;
- sem a checagem de ano, a validação atual aceitaria julho, agosto ou setembro de **qualquer ano**, inclusive antes do ano letivo sequer começar (por exemplo, setembro de 2025, um mês antes do início real do `ano_letivo=2025_2026`, que só começa em outubro de 2025) ou em qualquer ano muito posterior ao esperado.

Isso é uma lacuna real de validação num fluxo que já está em produção e que impacta diretamente a progressão de todas as academias de um tipo (a finalização de todas as academias ativas do mesmo tipo determina, automaticamente, o avanço do ano letivo global — ver `Documentação.md`, seção "Bloqueio de retrocesso global").

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Validação de ano | Exigir que o ano da data atual seja igual ao ano final do `ano_letivo` informado | `ano_letivo=2025_2026` só pode ser finalizado quando o ano atual for `2026` |
| Validação de mês | Manter a janela mensal já existente, derivada do `periodo` fixo do tipo | Nenhuma mudança na regra de meses já documentada |
| Combinação das duas validações | Ambas devem passar simultaneamente | Finalização só ocorre dentro do ano final **e** dentro da janela de meses permitida |
| Mensagem de erro | Indicar claramente qual condição falhou (ano ou mês) | Facilita diagnóstico para a academia |

---

# 1. Adicionar validação de ano na finalização de ano letivo

## Objetivo

Impedir que `POST /academia/anos-letivos/finalizar` finalize um `ano_letivo` quando o ano da data atual não for igual ao ano final desse `ano_letivo`, mesmo que o mês esteja dentro da janela permitida pelo `periodo` fixo do tipo.

## Regra de negócio

Para um `ano_letivo` no formato `YYYY_YYYY` (ex.: `2025_2026`), o ano final é o segundo componente (`2026`). A finalização só deve ser permitida quando:

1. o **ano** da data atual for exatamente igual ao ano final do `ano_letivo`; **e**
2. o **mês** da data atual estiver dentro da janela já documentada, derivada do `periodo` fixo do tipo (`escolar -> 09_07`, `superior -> 10_07`), com a mesma regra de inclusão/exclusão já existente (inclusiva no mês de fim, exclusiva no mês de início).

Se qualquer uma das duas condições falhar, a operação deve ser rejeitada com `400` e mensagem clara indicando qual condição não foi atendida.

## Escopo obrigatório

### 1.1 Localizar e ajustar a validação existente

Localizar o handler responsável por `POST /academia/anos-letivos/finalizar` e a função que hoje calcula a janela mensal permitida a partir do `periodo` fixo do tipo. Adicionar a checagem de ano como uma validação adicional, sem remover ou enfraquecer a validação de mês já existente.

### 1.2 Calcular o ano final corretamente

Reaproveitar (ou criar, se ainda não existir de forma centralizada) uma função pura que extrai o ano final de um `ano_letivo` no formato `YYYY_YYYY`, evitando parsing duplicado e divergente em múltiplos pontos do código.

### 1.3 Mensagens de erro específicas

A mensagem de erro deve deixar claro qual condição falhou, por exemplo:

```text
não é possível finalizar o ano letivo 2025_2026: o ano atual (2025) ainda não é o ano final do período letivo (2026)
```

```text
não é possível finalizar o ano letivo 2025_2026: fora da janela mensal de finalização; permitido apenas em julho, agosto e setembro de 2026
```

### 1.4 Consistência com o `ano_letivo` informado no payload

A validação deve usar o `ano_letivo` efetivamente ativo da academia (o mesmo que já é validado contra o payload opcional de `ano_letivo` na rota atual), não um valor recalculado de forma diferente. Se o `ano_letivo` não for enviado no payload, a validação de ano deve ser aplicada sobre o `ano_letivo` ativo da academia.

### 1.5 Testes obrigatórios

Adicionar ou ajustar testes cobrindo, no mínimo:

1. `ano_letivo=2025_2026`, tipo `superior` (`periodo=10_07`), data atual em julho/agosto/setembro de **2026**: finalização permitida;
2. `ano_letivo=2025_2026`, tipo `superior`, data atual em julho/agosto/setembro de **2025**: finalização rejeitada por ano incorreto, mesmo com o mês dentro da janela;
3. `ano_letivo=2025_2026`, tipo `superior`, data atual em julho/agosto/setembro de **2027**: finalização rejeitada por ano incorreto;
4. `ano_letivo=2025_2026`, tipo `escolar` (`periodo=09_07`), data atual em julho/agosto/setembro de **2026**: finalização permitida;
5. `ano_letivo=2025_2026`, mês fora da janela mensal, mesmo com ano correto (`2026`): finalização continua rejeitada pela regra de mês já existente;
6. mensagens de erro diferenciando claramente falha por ano e falha por mês;
7. teste de regressão confirmando que a regra de mês continua funcionando exatamente como antes desta tarefa para os casos que já eram cobertos.

---

# Fora de escopo

- Alterar o cálculo do `periodo` fixo por tipo (`escolar -> 09_07`, `superior -> 10_07`).
- Alterar a regra de avanço automático do `ano_letivo` da academia após a finalização.
- Alterar a regra de bloqueio de retrocesso global (`GET /admin/sistema/anos-letivos/finalizacao-limites`).
- Criar exceção/flag para permitir finalização fora do ano final em casos especiais.
- Alterar `POST /admin/definir-ano-letivo-geral` ou `POST /academia/definir-ano-letivo`.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `POST /academia/anos-letivos/finalizar` rejeitar a finalização quando o ano da data atual for diferente do ano final do `ano_letivo`, mesmo com o mês dentro da janela permitida;
2. a regra de mês já existente continuar funcionando sem regressão;
3. as mensagens de erro diferenciarem claramente falha por ano e falha por mês;
4. testes automatizados cobrirem os cenários da seção 1.5;
5. `Documentação.md` refletir a nova condição de validação na seção de finalização de ano letivo;
6. o PR explicar claramente a lacuna corrigida e o impacto no comportamento já documentado.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Bloquear finalização de ano letivo fora do ano final do período (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
