---
criado: 2026-08-12 00:00
origem: pedido direto do Spuri (evolução do módulo financeiro após a tarefa 17)
status: pendente
depende_de: nenhuma (esta é a Fase 1 — as Fases 2, 3 e 4 dependem desta)
---

# Módulo de pagamentos — Fase 1 — Bases: tipo do `amount` e cancelamento de cobrança (pendente)

## Prompt recomendado para executar a atualização

Implemente as duas bases descritas neste documento no módulo financeiro (`internal/finance/appypay.go`, `internal/handlers/financeiro_handlers.go`, `internal/projections/financeiro_projection.go`, `internal/domain/aggregates/financeiro.go`): (1) formalizar e blindar `float64` como o único tipo Go válido para qualquer valor monetário no módulo de pagamentos, presente e futuro; (2) adicionar um mecanismo de cancelamento de cobrança para os métodos REF, GPO e QR Code (GPO), utilizável por Spuri (role `fpp`) ou pela Academia dona da cobrança. Esta é a **Fase 1 de 4** de uma evolução maior do módulo de pagamentos — as Fases 2, 3 e 4 (mensalidade automatizada, mensalidade versátil ao estudante, e matrícula) dependem diretamente das regras fixadas aqui e devem reutilizá-las sem as reinterpretar. Siga rigorosamente a estrutura de eventos/projeção já estabelecida em `internal/finance/appypay.go` (função `record`, padrão `CobrancaAppyPay*`). Ao final, atualize testes, `Documentação.md` e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou fallbacks temporários.

## Contexto

Esta tarefa parte de uma auditoria completa do módulo financeiro atual (pós-rollback, `docs/Tarefas feitas/17 - Modulo base de gestao financeira com AppyPay.md`, migrations 101/102).

**Sobre o tipo do `amount`:** a auditoria confirmou que `internal/finance/appypay.go` já usa `float64` em `ChargeRequest.Amount`, `QRCodeRequest.Amount`/`MinAmount` e nos payloads persistidos (`chargePayload`, `qrCodePayload`) — e a documentação AppyPay (`docs/Parceiros e integrações/AppyPay Documentação.md`) confirma `number<double>` no `amount` de `Post a Charge` e `Post a GPO QR Code`. **Não foi encontrada inconsistência de tipo hoje.** O que falta é (a) tornar esse contrato explícito e obrigatório para todo o módulo, incluindo o que as Fases 2/3/4 vão adicionar (valor de mensalidade, valor de matrícula), e (b) blindar o módulo contra os problemas clássicos de usar `float64` para dinheiro (imprecisão de ponto flutuante em comparações, arredondamento inconsistente), que hoje não têm nenhum tratamento centralizado.

**Sobre o cancelamento:** a documentação AppyPay **não** expõe um endpoint REST genérico de "cancelar cobrança" para REF, GPO ou QR Code. A seção "Charge Operations" (linha ~840 do documento) lista Refund, Reverse e Cancel como conceitos, mas os únicos endpoints REST documentados são `Post a charge refund` (suportado apenas por GPO, UMM, eTPA, SDD — **não** por REF) e `Post a charge reverse` (suportado **apenas** por UMM). Não existe operação de cancelamento de referência (REF) nem de QR Code na documentação. A versão anterior do módulo (pré-rollback, removida pela migration `099_remove_modulo_financeiro_appypay.sql`) chegou a ter funções `CancelarCobrancaFinanceiraBase`, `ReembolsarCobrancaFinanceiraBase` e `ReverterCobrancaFinanceiraBase`, mas a reimplementação atual (v2, migrations 101/102) excluiu deliberadamente Refund/Reverse do escopo (ver "Fora de escopo" da tarefa 17). Esta tarefa **não** reabre Refund/Reverse — introduz um cancelamento **novo, mais restrito e mais seguro**: uma operação puramente do lado Spuri, que nunca chama a API da AppyPay (porque não existe operação equivalente lá para REF/GPO/QR), e que só é permitida enquanto a cobrança ainda não tiver sido paga.

---

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Tipo do `amount` | `float64` (Go) / `number<double>` (AppyPay), único tipo permitido em todo o módulo | Contrato explícito, documentado e testado; obrigatório também nas Fases 2/3/4 |
| Precisão monetária | Função utilitária centralizada de arredondamento (2 casas) e de comparação com tolerância | Elimina divergências de reconciliação por imprecisão de `float64` |
| Cancelamento — quem | Spuri (`fpp`) ou a Academia dona da cobrança | Reaproveita `authorizeFinanceScope` já existente |
| Cancelamento — como | Operação interna do Spuri (evento no ledger); **nunca** chama a API da AppyPay para REF/GPO/QR | Não existe endpoint equivalente na documentação AppyPay para esses métodos |
| Cancelamento — quando | Apenas enquanto a cobrança não estiver paga; reconsulta obrigatória à AppyPay antes de cancelar | Reduz (sem eliminar) risco de corrida com pagamento assíncrono |
| Corrida pós-cancelamento | Evento de conflito dedicado, nunca aceite/ignorado silenciosamente | Preserva auditoria e força reconciliação manual FPP |

---

# 1. Padronizar o tipo do `amount` em todo o módulo de pagamentos

## Objetivo

Garantir que **todo** valor monetário em todo o módulo de pagamentos — hoje e nas Fases 2, 3 e 4 — usa exatamente o tipo indicado pela documentação AppyPay (`number<double>`), sem exceções, e que essa escolha é resiliente aos problemas usuais de ponto flutuante em dinheiro.

## Regra de negócio

- `float64` é o **único** tipo Go permitido para representar `amount`/valor monetário em `internal/finance` e em qualquer pacote financeiro construído sobre ele nas Fases 2/3/4 (ex.: valor de mensalidade, valor de matrícula). Não é permitido usar `int`/`int64` (ex.: representar em cêntimos), `string`, nem bibliotecas de decimal (`shopspring/decimal` ou equivalentes) — a AppyPay especifica `number<double>` e é isso que deve ser respeitado, inclusive na serialização JSON enviada ao provedor.
- Todo campo Go que representar dinheiro deve chamar-se de forma consistente (`Amount` quando corresponde 1:1 a um campo enviado/recebido da AppyPay; `Valor...` para campos internos do Spuri que nunca são enviados à AppyPay, ex.: `ValorMensalidade`), mas o **tipo** é sempre `float64` em ambos os casos.
- Moeda continua fixa em `AOA` (já validado em `validateCharge`); esta tarefa não introduz suporte a outras moedas.

## Escopo obrigatório

### 1.1 Documentar o contrato

Adicionar um comentário explícito no topo de `internal/finance/appypay.go` (ou um `README.md` no pacote, se preferir) declarando que `float64` é o tipo oficial de qualquer valor monetário no módulo, citando a documentação AppyPay (`number<double>`) como origem da decisão, para que as Fases 2/3/4 (e qualquer manutenção futura) não precisem redescobrir isto por auditoria.

### 1.2 Blindar contra imprecisão de `float64`

- Criar uma função utilitária de arredondamento a 2 casas decimais (ex.: `roundAmount(v float64) float64`), com a regra de arredondamento escolhida (recomenda-se arredondamento padrão — "half away from zero" — mas registre explicitamente qual foi escolhida no código e na documentação) e aplicá-la:
  - a qualquer valor antes de o enviar à AppyPay (`ChargeRequest.Amount`, `QRCodeRequest.Amount`/`MinAmount`);
  - a qualquer valor configurado internamente (mensalidade, matrícula — Fases 2/4).
- Criar uma função de comparação com tolerância (ex.: `amountsEqual(a, b float64) bool`, tolerância sugerida `0.005`) e usá-la em **todo** ponto do módulo que hoje compara valores monetários (ex.: reconciliação entre valor esperado e valor devolvido pela AppyPay em `ConsultCharge`, comparações que forem introduzidas nas Fases 2/3/4). Nunca comparar `float64` monetário com `==` diretamente em nenhum ponto do módulo.

### 1.3 Validação de entrada

Reforçar a validação já existente (`in.Amount <= 0`) para também rejeitar valores com mais de 2 casas decimais efetivas (ex.: `15.999`), usando a mesma lógica de arredondamento do item 1.2 para decidir se o valor recebido "sobrevive" ao arredondamento sem mudar (se `roundAmount(v) != v`, rejeitar com erro explícito em vez de arredondar silenciosamente uma entrada do utilizador).

### 1.4 Testes obrigatórios

1. round-trip de um valor monetário através de `json.Marshal`/`json.Unmarshal` preserva o valor exato;
2. `amountsEqual` trata como iguais valores que diferem apenas por erro de ponto flutuante típico (ex.: `0.1 + 0.2` vs `0.3`) e como diferentes valores que diferem por 1 cêntimo ou mais;
3. `roundAmount` arredonda corretamente casos de fronteira (ex.: `10.005`);
4. requisição com `amount` de mais de 2 casas decimais é rejeitada com erro claro, sem arredondamento silencioso;
5. requisição com `amount <= 0` continua a ser rejeitada (comportamento já existente, cobrir com teste explícito se ainda não houver).

### 1.5 Vínculo obrigatório com as Fases 2, 3 e 4

Registrar explicitamente (neste documento e no código) que qualquer campo monetário introduzido pelas Fases 2, 3 e 4 (`ValorMensalidade`, `ValorMatricula`, etc.) **deve** reutilizar `float64`, `roundAmount` e `amountsEqual` definidos aqui — não é permitido reinventar tratamento monetário próprio nessas fases.

---

# 2. Cancelamento de cobrança (REF, GPO, QR Code)

## Objetivo

Permitir que Spuri (role `fpp`) ou a Academia dona da cobrança cancelem uma cobrança AppyPay ainda não paga, para os métodos REF, GPO e QR Code (que é uma variante de GPO), fechando a lacuna identificada pelo Spuri face à documentação AppyPay — com o entendimento correto (ver "Contexto") de que isto é uma operação interna do Spuri, não uma chamada à API da AppyPay.

## Regra de negócio

1. Cancelamento é permitido apenas enquanto a cobrança **não** estiver num estado terminal de sucesso (pago). Cobranças já `cancelada`, `falhada` ou pagas não podem ser canceladas novamente nem "reabertas".
2. Antes de cancelar, o sistema **deve** reconsultar o estado mais recente da cobrança junto da AppyPay (reaproveitando a lógica já usada por `ConsultCharge`) para reduzir a janela de corrida entre cancelamento e confirmação assíncrona de pagamento. Se a reconsulta já mostrar sucesso, o cancelamento é rejeitado e o estado atual (pago) é devolvido ao chamador.
3. Mesmo com o passo 2, pode ainda existir corrida (a AppyPay processa alguns métodos de forma assíncrona). Se, **depois** de uma cobrança ter sido cancelada localmente, uma nova consulta ou webhook indicar sucesso de pagamento para essa mesma cobrança, o sistema deve gravar um evento de conflito dedicado (`CobrancaAppyPayConflitoPosCancelamento`) — nunca aceitar o pagamento silenciosamente como se nada tivesse acontecido, nem ignorá-lo. Este conflito deve ficar visível para reconciliação manual por um admin `fpp`.
4. Uma cobrança cancelada é definitiva: não pode ser reaberta. Se for necessário cobrar novamente, deve ser criada uma nova cobrança (novo `merchantTransactionId`), seguindo o fluxo normal já existente.
5. Autorização: Spuri (`fpp`) pode cancelar qualquer cobrança; uma Academia só pode cancelar cobranças do seu próprio contexto (`contexto_tipo="academia"` e `codigo_academia` correspondente) — reaproveitar exatamente `authorizeFinanceScope`, já usado pelos demais endpoints financeiros.
6. Aplica-se a cobranças criadas via `CreateCharge` (REF e GPO) e via `CreateGPOQRCode` (QR Code) — ambas persistem na mesma tabela `financeiro_cobrancas`.
7. Além do endpoint direto desta seção, `CancelCharge` também é acionado automaticamente pela Fase 2/3 (mensalidade) quando a academia anula uma obrigação de mensalidade que já tenha uma cobrança em aberto associada — ver Fase 3, seção 5. `CancelCharge` deve ser implementado como uma função de serviço reutilizável (não só como handler HTTP) exatamente para permitir essa chamada interna.
8. **Limitação inerente à AppyPay:** como não existe endpoint de cancelamento real para REF/GPO/QR (ver "Contexto"), marcar uma cobrança como `cancelada` no Spuri impede o **fluxo interno** (o estudante não a verá mais como pagável na plataforma, não poderá gerar outra para o mesmo mês enquanto esta existir em aberto), mas **não invalida tecnicamente** a referência/QR já emitido do lado da AppyPay/banco até ela expirar naturalmente. Um pagamento ainda pode, em teoria, concretizar-se por essa via já cancelada — é precisamente esse cenário que o item 3 (evento de conflito) cobre. Esta limitação deve ficar documentada de forma explícita e visível (`Documentação.md`), para que ninguém assuma uma garantia de invalidação que a AppyPay não oferece.

## Escopo obrigatório

### 2.1 Novo evento no ledger

Adicionar o evento `CobrancaAppyPayCancelada` ao aggregate `Financeiro` (`internal/domain/aggregates/financeiro.go`), seguindo exatamente o padrão já usado pelos demais eventos `CobrancaAppyPay*`/`QRCodeAppyPay*` (`Registrar`, `Aplicar`).

### 2.2 Novo método de serviço

Adicionar `func (s *Service) CancelCharge(ctx context.Context, contexto, academia, identifier, motivo, actorID, actorType, ip string) (ChargeResult, error)` em `internal/finance/appypay.go`:

- carregar a cobrança via `loadCharge` (já existente);
- validar `contexto`/`academia` exatamente como `ConsultCharge` já faz (`row.Contexto != contexto || row.Academia != academia` → `ErrNotFound`);
- rejeitar se `row.Status` já corresponder a um estado terminal (cancelada, falhada, ou sucesso — comparar de forma tolerante a maiúsculas/minúsculas contra o valor documentado pela AppyPay, `"Success"`, além dos estados internos `"cancelada"`/`"falhada"`);
- reconsultar o estado atual junto da AppyPay reaproveitando a lógica interna já usada por `ConsultCharge` (idealmente extraindo essa lógica de consulta para uma função privada partilhada, para não duplicar código);
- se a reconsulta indicar sucesso, **não** cancelar — devolver o estado atualizado e um erro claro;
- caso contrário, gravar `CobrancaAppyPayCancelada` (payload incluindo `motivo`, quando fornecido, além dos campos já padrão dos demais eventos: `charge_id`, `contexto_tipo`, `codigo_academia`, `status="cancelada"`) via `s.record`, seguindo exatamente o padrão de `CreateCharge`.

### 2.3 Novo endpoint HTTP

Adicionar `POST /financeiro/cobrancas/:id/cancelar` em `internal/handlers/financeiro_handlers.go`, seguindo o mesmo padrão dos handlers existentes (`authorizeFinanceScope`, extração de `id`/`t` do contexto de autenticação, chamada ao serviço, resposta JSON). Aceitar `motivo` opcional no corpo do pedido.

### 2.4 Atualizar a projeção

Em `internal/projections/financeiro_projection.go`, adicionar o `case` para `CobrancaAppyPayCancelada` (e para `CobrancaAppyPayConflitoPosCancelamento`, do item 2.6) no `switch` que já trata os demais eventos, atualizando `status` (e demais campos relevantes) na tabela `financeiro_cobrancas`.

### 2.5 QR Code

Confirmar por teste que o mesmo mecanismo funciona para cobranças criadas via `CreateGPOQRCode` (mesma tabela, mesmo fluxo de cancelamento — não deve haver tratamento especial de QR Code além do já existente em `loadCharge`).

### 2.6 Conflito pós-cancelamento

Implementar a gravação do evento `CobrancaAppyPayConflitoPosCancelamento` sempre que `ConsultCharge` (ou o fluxo de webhook, se já processar este caso) detectar sucesso numa cobrança cujo status local já seja `cancelada`. Isto exige revisitar `ConsultCharge` para reconhecer esse cenário específico e tratá-lo de forma distinta de uma atualização de status normal.

### 2.7 Testes obrigatórios

1. cancelar cobrança pendente com sucesso, autor Spuri (`fpp`);
2. cancelar cobrança pendente com sucesso, autor Academia dona da cobrança;
3. Academia tentando cancelar cobrança de **outra** academia → rejeitado, sem vazar existência da cobrança (mesmo padrão de `ErrNotFound` já usado noutros pontos);
4. tentar cancelar cobrança já paga (reconsulta mostra sucesso) → rejeitado, estado devolvido corretamente, nenhum evento de cancelamento gravado;
5. tentar cancelar cobrança já cancelada → rejeitado;
6. tentar cancelar cobrança já falhada → rejeitado;
7. simular corrida: cobrança cancelada localmente e, em seguida, uma consulta simulando resposta de sucesso da AppyPay → evento de conflito gravado, nenhuma alteração indevida ao status `cancelada`;
8. cancelamento aplicado a uma cobrança criada via QR Code (`CreateGPOQRCode`) funciona da mesma forma.

---

# Fora de escopo

- Reabrir Refund (`Post a charge refund`) ou Reverse (`Post a charge reverse`) — permanecem fora de escopo, como já definido na tarefa 17. O cancelamento desta tarefa é uma operação distinta, interna ao Spuri, e não substitui nem implementa essas duas operações da AppyPay.
- Cancelamento de cobranças de métodos ainda não suportados pelo módulo (UMM, SDD, eTPA) — hoje o módulo só suporta GPO e REF.
- Notificação automática ao estudante/encarregado sobre o cancelamento (será tratado quando o módulo de notificações WhatsApp estiver pronto).
- Qualquer alteração ao fluxo de configuração de credenciais (`ConfigureCredential`) ou de webhooks — permanecem como estão.
- Introdução de suporte a moedas diferentes de `AOA`.

# Riscos e mitigações

| Risco | Mitigação |
| --- | --- |
| Corrida entre cancelamento local e confirmação assíncrona de pagamento na AppyPay | Reconsulta obrigatória antes de cancelar (2.2) + evento de conflito dedicado para qualquer sucesso detectado após cancelamento (2.6), nunca aceite silenciosamente |
| Imprecisão de `float64` causando falsos positivos/negativos em comparações de valor | Funções centralizadas `roundAmount`/`amountsEqual` (1.2), usadas em todo o módulo, incluindo nas Fases 2/3/4 |
| Academia cancelar cobrança fora do seu contexto | Reaproveitar `authorizeFinanceScope`, já testado; cobrir com teste dedicado (2.7.3) |
| Cobrança cancelada ser "reaberta" indevidamente | Regra explícita de que cancelamento é definitivo (regra de negócio item 4); nenhum endpoint deve permitir reverter um cancelamento |

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `float64` for o único tipo usado para valores monetários em todo `internal/finance`, com o contrato documentado explicitamente no código;
2. `roundAmount` e `amountsEqual` existirem, estiverem testados e forem usados em todos os pontos do módulo que arredondam ou comparam valores monetários;
3. validação rejeitar `amount` com mais de 2 casas decimais e `amount <= 0`, com testes cobrindo ambos;
4. `CancelCharge` existir, reaproveitar `loadCharge` e `authorizeFinanceScope`, e recusar cancelamento de cobranças já pagas, canceladas ou falhadas;
5. o endpoint `POST /financeiro/cobrancas/:id/cancelar` existir e seguir o padrão de autorização já usado pelos demais endpoints financeiros;
6. a projeção tratar corretamente `CobrancaAppyPayCancelada` e `CobrancaAppyPayConflitoPosCancelamento`;
7. o cenário de corrida (cancelamento seguido de sucesso tardio) estiver coberto por teste e gravar o evento de conflito, sem jamais marcar a cobrança como paga depois de cancelada;
8. QR Code (GPO) puder ser cancelado pelo mesmo mecanismo, com teste dedicado;
9. todos os testes da seção 1.4 e 2.7 passarem;
10. `Documentação.md` estiver atualizada com o novo endpoint, os novos eventos, e o contrato de tipo do `amount`.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Módulo de pagamentos — Fase 1 — Bases: tipo do amount e cancelamento de cobrança (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
