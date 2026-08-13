---
criado: 2026-08-12 00:00
origem: pedido direto do Spuri (evolução do módulo financeiro após a tarefa 17)
status: pendente
depende_de: "27 - Módulo de pagamentos - Fase 2 - Cobrança de mensalidade-propina automatizada.md"
---

# Módulo de pagamentos — Fase 3 — Cobrança de mensalidade/propina versátil para o estudante (pendente)

## Prompt recomendado para executar a atualização

Implemente, sobre a Fase 2 (`27 - ...md`, que deve estar concluída antes de iniciar esta tarefa), o fluxo pelo qual o próprio estudante escolhe qual(is) mensalidade(s) quer pagar (de qualquer academia com que já teve vínculo, atual ou anterior — a dívida é exigível independentemente do vínculo atual) e por qual método (dentre os que a academia disponibilizou), e a plataforma gera automaticamente a cobrança correspondente na AppyPay em nome da academia. O estudante nunca cria a cobrança diretamente — apenas seleciona a sua intenção de pagamento; a criação da cobrança real na AppyPay é automática, imediatamente após a seleção. Inclua a exceção estreita de autenticação (seção 6) que permite a um estudante sem nenhum vínculo ativo autenticar-se exclusivamente para pagar dívidas antigas, sem reabrir acesso a mais nada. Reaproveite integralmente `Service.CreateCharge`/`CreateGPOQRCode` (Fase 1) e a projeção de meses devidos/pagos (Fase 2). Siga o padrão de event sourcing já estabelecido. Ao final, atualize testes, `Documentação.md` e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou fallbacks temporários.

## Contexto

A Fase 2 entrega uma projeção derivada de meses `pendente`/`pago`/`anulado` por estudante, mas **não** cria nenhuma cobrança na AppyPay. Esta fase é onde a cobrança real passa a existir, disparada pela escolha do próprio estudante.

Interpretação adotada para "a academia recebe essas informações e gera a cobrança para o estudante" (frase do pedido original): a geração da cobrança é **automática e imediata**, feita pela plataforma em nome da academia, no mesmo instante em que o estudante confirma a sua seleção — não exige uma ação manual de um funcionário da academia. "A academia gera" significa que a cobrança é criada no contexto financeiro da academia (`contexto_tipo="academia"`, `codigo_academia` do estudante), não que um humano da academia precisa de intervir a cada pedido. Se a equipa entender, durante a implementação, que deveria existir aprovação manual da academia antes da geração da cobrança, isso deve ser registrado explicitamente no PR como desvio consciente desta interpretação.

`Service.CreateCharge` e `Service.CreateGPOQRCode` (`internal/finance/appypay.go`) já suportam `ContextoTipo="academia"` com `CodigoAcademia`, `Amount` (`float64`, Fase 1), `PaymentMethod` (`GPO`/`REF`, incluindo variantes `GPO_*`/`REF_*`) e `MerchantTransactionID` idempotente — esta fase reutiliza essas funções sem alterá-las, apenas orquestra a chamada a partir da seleção do estudante.

---

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Quem cria a cobrança | A plataforma, automaticamente, em nome da academia, no momento da seleção do estudante | Estudante nunca chama `CreateCharge` diretamente |
| Métodos disponíveis ao estudante | Apenas os que a academia habilitou para cobrança de propinas (subconjunto de REF/GPO/QR Code) | Reaproveita credenciais e métodos já configurados na Fase base (tarefa 17) |
| Seleção de um único mês | Deve ser exatamente o mês pendente mais antigo | Impede "pular" meses e pagar um mês futuro deixando um mês antigo em aberto |
| Seleção de múltiplos meses | O primeiro selecionado deve ser o mês pendente mais antigo; os demais podem ser quaisquer outros meses pendentes, sem exigência de sequência entre si | Segue exatamente a regra do pedido original |
| Cobrança gerada | Uma única cobrança AppyPay cobrindo a soma dos meses selecionados | Evita múltiplas cobranças fragmentadas para uma única intenção de pagamento |
| Confirmação de pagamento | Marca atomicamente todos os meses cobertos pela cobrança como pagos | Nunca marca pagamento parcial de uma cobrança única |
| Anulação (Fase 2) de mês com cobrança em aberto | Cancela automaticamente a cobrança inteira (Fase 1) | Evita cobrança "órfã" ainda disponível para pagamento após a obrigação deixar de existir; limitação residual documentada (ver seção 5) |
| Pendência de academia diferente da atual | Mesmo endpoint, `codigo_academia` explícito na seleção; sequência independente por academia | Dívida é exigível independentemente do vínculo atual |
| Estudante sem nenhum vínculo ativo (`Status="inativo"`) | Login excecional, restrito a fins financeiros, via nova claim reconhecida só pelos handlers financeiros | Bloqueio geral do middleware permanece inalterado para tudo o resto (ver seção 6) |

---

# 1. Academia define métodos disponíveis para pagamento de propinas

## Objetivo

Permitir que a academia escolha, entre os métodos que já configurou (REF, GPO, QR Code via GPO), quais ficam disponíveis para o estudante pagar mensalidade.

## Regra de negócio

- Reaproveitar a credencial/configuração de métodos já existente na base do módulo financeiro (tarefa 17, `CredentialInput.GPOPaymentMethod`/`REFPaymentMethod`); esta seção apenas adiciona uma seleção adicional de **quais desses métodos ficam disponíveis para propina especificamente** (uma academia pode ter GPO/REF configurados para outros fins financeiros futuros, mas optar por disponibilizar só um deles ao estudante para propina).
- Se a academia não configurar explicitamente nenhum método disponível para propina, o estudante não deve conseguir iniciar nenhum pagamento de mensalidade (mesmo princípio de "ausência de configuração = recurso desativado" já usado na Fase 2).

## Escopo obrigatório

### 1.1 Configuração

Adicionar à mesma configuração de mensalidade da academia (Fase 2, `ConfiguracaoMensalidade`, ou um campo próprio associado à academia) a lista de métodos habilitados para propina (subconjunto de `{GPO, REF, GPO_QR}`), validando que a academia já possui credencial AppyPay configurada para o(s) método(s) escolhido(s).

### 1.2 Testes obrigatórios

1. academia habilita apenas REF para propina: estudante só vê REF como opção;
2. academia não habilita nenhum método: estudante não consegue selecionar nenhum pagamento de propina;
3. tentar habilitar um método para o qual a academia não tem credencial configurada → rejeitado.

---

# 2. Estudante seleciona mensalidade(s) e método de pagamento

## Objetivo

Permitir que o estudante escolha, entre os seus meses pendentes (Fase 2) e os métodos disponibilizados pela academia (seção 1), o que deseja pagar, respeitando a regra de sequência do mês mais antigo.

## Regra de negócio

1. A lista de meses selecionáveis é exatamente a lista de meses com estado `pendente` calculada pela Fase 2 para aquele estudante **numa academia específica** (`codigo_academia`), no(s) ano(s) letivo(s) em que houver pendência (incluindo anos letivos anteriores ao corrente, se ainda pendentes) — incluindo academias diferentes da atual (ver seção 6).
2. **Seleção de um único mês:** só é permitida se for exatamente o mês pendente mais antigo **daquela academia** (o de menor `(ano_letivo, mês)` entre os pendentes **na mesma academia**). Exemplo do pedido original: se setembro e outubro já foram pagos, o estudante só pode selecionar dezembro (o próximo pendente), não um mês posterior.
3. **Seleção de múltiplos meses:** o primeiro mês da seleção (em ordem cronológica) deve obrigatoriamente ser o mês pendente mais antigo **daquela mesma academia**; os demais meses selecionados podem ser quaisquer outros meses pendentes **da mesma academia**, em qualquer ordem, sem exigência de sequência entre si. Exemplo do pedido original: já pago setembro; seleção válida `[outubro, janeiro, março]` (outubro é o mais antigo pendente; janeiro e março não precisam ser sequenciais).
4. **A regra de sequência nunca cruza academias:** pendências de academias diferentes têm filas independentes — o mês mais antigo pendente na Academia A não bloqueia nem depende do mês mais antigo pendente na Academia B (ver seção 6). Uma única seleção/cobrança cobre sempre meses de **uma única** academia.
5. Meses com estado `anulado` (Fase 2) nunca aparecem como selecionáveis.
6. O método de pagamento selecionado deve pertencer ao conjunto habilitado pela academia correspondente (seção 1).

## Escopo obrigatório

### 2.1 Endpoint de seleção

Criar rota (ex.: `POST /financeiro/mensalidades/pagamento`) onde o estudante autenticado envia `codigo_academia` (a academia a que a pendência pertence — atual ou anterior), a lista de `(ano_letivo, mês)` que deseja pagar, e o método escolhido. A validação da regra de sequência (itens 2 e 3 da regra de negócio) deve ocorrer aqui, antes de qualquer chamada à AppyPay. Ver seção 6 para autenticação quando o estudante não tem nenhum vínculo ativo.

### 2.2 Testes obrigatórios

1. seleção de um único mês que é o mais antigo pendente **daquela academia** → aceite;
2. seleção de um único mês que **não** é o mais antigo pendente → rejeitado, com mensagem explicando qual é o mês correto;
3. seleção de múltiplos meses cujo primeiro é o mais antigo pendente e os demais são não sequenciais → aceite;
4. seleção de múltiplos meses cujo primeiro **não** é o mais antigo pendente → rejeitado;
5. seleção incluindo um mês já pago ou anulado → rejeitado;
6. seleção com método não habilitado pela academia para propina → rejeitado;
7. estudante com pendências em duas academias diferentes: pagar o mês mais antigo pendente na Academia A não é afetado por, nem afeta, a fila de pendências da Academia B.

---

# 3. Geração automática da cobrança

## Objetivo

A partir de uma seleção válida (seção 2), gerar automaticamente, sem intervenção manual da academia, a cobrança correspondente na AppyPay.

## Regra de negócio

- O valor da cobrança é a soma do valor **histórico** de cada mês selecionado — resolvido exatamente pela regra da Fase 2, Seção 6 (ano académico/curso e preço em vigor na data de referência de cada mês, nunca os valores atuais) — usando `roundAmount` (Fase 1) sobre o total. Esta fase não reimplementa nem simplifica essa resolução; reutiliza-a diretamente.
- Uma única cobrança AppyPay cobre todos os meses selecionados numa mesma seleção — não gerar uma cobrança por mês. O payload da cobrança deve conter, de forma auditável, a lista exata de `(ano_letivo, mês)` cobertos (ex.: no campo `description` legível e também estruturado em `paymentInfo`/campo próprio do payload gravado no ledger), para permitir a confirmação atômica da seção 4.
- A cobrança é criada via `Service.CreateCharge` (REF/GPO) ou `Service.CreateGPOQRCode` (QR Code), com `ContextoTipo="academia"`, `CodigoAcademia` igual ao informado na seleção (seção 2.1) — a academia dona da pendência, que pode ser diferente da academia atual do estudante, ou mesmo a única com que ele já teve vínculo, se estiver sem vínculo ativo (seção 6) — ator/autor sendo o próprio estudante (registrar isso no evento, mesmo sendo uma ação "automática" do sistema em nome da academia).
- Enquanto uma seleção anterior do mesmo estudante ainda não tiver sido concluída (paga, falhada ou cancelada), o estudante não deve conseguir iniciar uma nova seleção sobre os mesmos meses — evitar cobranças duplicadas para o mesmo mês pendente.

## Escopo obrigatório

### 3.1 Orquestração

Implementar a função/serviço que, a partir de uma seleção validada (seção 2), soma os valores, monta a `ChargeRequest`/`QRCodeRequest` e chama `Service.CreateCharge`/`CreateGPOQRCode`, persistindo a associação entre a cobrança criada e os meses cobertos (para uso na seção 4).

### 3.2 Prevenção de duplicidade

Antes de gerar uma nova cobrança para um mês, verificar se já existe uma cobrança em aberto (não paga, não falhada, não cancelada — reaproveitando os estados definidos pela Fase 1) cobrindo aquele mesmo `(ano_letivo, mês)` para o estudante; se existir, rejeitar a nova seleção e informar a cobrança em aberto já existente (ou, alternativamente, oferecer cancelá-la primeiro via Fase 1, à escolha da implementação — registre a decisão tomada no PR).

### 3.3 Testes obrigatórios

1. seleção de um mês gera uma cobrança de valor igual ao configurado para aquele mês;
2. seleção de múltiplos meses gera uma única cobrança cujo valor é a soma exata dos meses selecionados (usando `roundAmount`);
3. seleção incluindo um mês de um ano letivo anterior, já com ano académico/preço diferentes dos atuais → cobrança usa o valor histórico correto (Fase 2, Seção 6), não o valor/ano atual do estudante;
4. tentar selecionar novamente um mês que já tem cobrança em aberto → rejeitado;
5. falha na criação da cobrança na AppyPay (ex.: erro de rede) não deixa nenhum mês marcado como pago nem em estado inconsistente.

---

# 4. Confirmação de pagamento marca os meses cobertos como pagos

## Objetivo

Quando a AppyPay confirmar o sucesso de uma cobrança gerada pela seção 3, marcar atomicamente todos os meses que ela cobre como pagos.

## Regra de negócio

- A confirmação chega via webhook (fluxo já existente, `AcceptWebhook`) ou via nova consulta (`ConsultCharge`, Fase 1). Em ambos os casos, ao detectar transição de status para sucesso, o sistema deve localizar, a partir do payload da cobrança (seção 3.1), exatamente quais `(ano_letivo, mês)` ela cobre, e gravar o(s) evento(s) de pagamento correspondentes para cada um — atomicamente (todos os meses da cobrança são marcados juntos, nunca parcialmente).
- A academia deve sempre conseguir consultar, para qualquer estudante, quais meses já foram pagos, quando e através de qual cobrança — reaproveitando a projeção de meses da Fase 2, agora populada com dados reais.

## Escopo obrigatório

### 4.1 Evento de pagamento

Definir o evento (ex.: `ObrigacaoMensalidadePaga`) gravado por mês coberto, referenciando o `charge_id`/`merchant_transaction_id` que originou o pagamento, disparado a partir da confirmação de sucesso da cobrança (seção 3).

### 4.2 Atomicidade

Garantir que, se a gravação de algum dos eventos de pagamento (quando há múltiplos meses numa mesma cobrança) falhar, nenhum fique marcado como pago de forma parcial — a operação deve ser transacional ou, no mínimo, ter reconciliação automática que reprocesse os que faltaram, sem duplicar os que já foram gravados (idempotência).

### 4.3 Testes obrigatórios

1. cobrança de um único mês confirmada com sucesso → aquele mês passa a `pago`, referenciando a cobrança;
2. cobrança de múltiplos meses confirmada com sucesso → todos os meses cobertos passam a `pago` atomicamente;
3. reprocessamento do mesmo webhook/consulta (idempotência) não duplica eventos de pagamento nem falha;
4. cobrança que falha (status de falha) não marca nenhum mês como pago, e o(s) mês(es) volta(m) a ficar selecionável(is) numa nova tentativa.

---

# 5. Cancelamento em cascata quando uma obrigação coberta é anulada

## Objetivo

Garantir que, se a academia anular uma obrigação de mensalidade (Fase 2, `ObrigacaoMensalidadeAnulada`) que já tenha uma cobrança em aberto associada (gerada por esta fase e ainda não paga), essa cobrança seja automaticamente cancelada — evitando que continue disponível para pagamento apesar de a obrigação já não existir.

## Regra de negócio

- Ao processar o evento `ObrigacaoMensalidadeAnulada` (Fase 2), o sistema deve verificar se existe alguma cobrança em aberto (criada, não paga, não cancelada, não falhada — Fase 1) cujo payload cubra o `(ano_letivo, mês)` anulado. Se existir, acionar `Service.CancelCharge` (Fase 1) para essa cobrança, com autor igual ao autor da anulação e motivo derivado (ex.: `"obrigação anulada pela academia"`).
- Se a cobrança cobrir **outros** meses além do anulado (seleção múltipla, seção 2), o cancelamento em cascata cancela a cobrança **inteira** — a AppyPay não permite cancelar/alterar parcialmente uma cobrança já criada. Os demais meses cobertos por essa cobrança voltam ao estado `pendente` e podem ser selecionados novamente numa nova cobrança.
- **Limitação inerente à AppyPay, não resolvida por esta regra:** mesmo depois de o Spuri marcar a cobrança como `cancelada`, uma referência (REF) ou QR Code já gerado pode continuar tecnicamente pagável do lado da AppyPay/banco até expirar, porque não existe endpoint de cancelamento real para esses métodos (ver Fase 1, Contexto). Isto significa que, em casos raros, um estudante pode ainda conseguir pagar uma cobrança que o Spuri já considera cancelada. É exatamente para isto que serve o evento `CobrancaAppyPayConflitoPosCancelamento` da Fase 1 — este cenário deve ser tratado como conflito para reconciliação manual FPP, nunca como pagamento válido silencioso, mas **não há forma de o Spuri impedir a transação em si de se concretizar do lado da AppyPay**. Documente esta limitação de forma visível (ex.: no `Documentação.md`) para que a equipa não assuma uma garantia que a AppyPay não oferece.

## Escopo obrigatório

### 5.1 Integração com o evento de anulação

Implementar o gatilho descrito na regra de negócio, reaproveitando `Service.CancelCharge` (Fase 1) sem duplicar a sua lógica de validação/reconsulta.

### 5.2 Testes obrigatórios

1. anular uma obrigação com cobrança em aberto cobrindo exatamente aquele mês → cobrança cancelada automaticamente;
2. anular uma obrigação coberta por uma cobrança que também cobre outros meses ainda pendentes → cobrança inteira cancelada, os outros meses voltam a `pendente` e ficam novamente selecionáveis;
3. anular uma obrigação sem nenhuma cobrança em aberto associada → nenhuma chamada a `CancelCharge`, sem erro;
4. anular uma obrigação cuja cobrança já foi paga → `CancelCharge` rejeita corretamente (comportamento já garantido pela Fase 1), e o sistema não deve tratar isso como falha da anulação em si — a anulação da obrigação e o cancelamento da cobrança são operações distintas; se a cobrança já estiver paga, registrar esse conflito, mas a anulação da obrigação (para fins futuros) continua válida.

---

# 6. Pagamento de pendências de academias diferentes da atual (incluindo estudante sem vínculo ativo)

## Objetivo

Garantir que um estudante consegue liquidar pendências de mensalidade de **qualquer** academia com que já teve vínculo — não apenas a atual — porque a dívida é uma obrigação que subsiste independentemente do vínculo em vigor hoje, inclusive quando o estudante já não está vinculado a nenhuma academia.

## Contexto adicional (achado de auditoria)

O middleware de autenticação (`internal/middleware/auth.go`) já **não** guarda nenhum `codigo_academia` "atual" na sessão de um ator do tipo `estudante` — esse valor só é preenchido para atores do tipo `academia`. Ou seja, todo pedido de pagamento de mensalidade **já precisa** de identificar explicitamente a academia da pendência (seção 2.1, `codigo_academia`); não existe hoje o conceito de "academia atual implícita" na sessão do estudante para este módulo. Isto simplifica bastante o suporte a academias diferentes: não há um caminho "especial" a criar — pendências da academia atual e de academias anteriores usam exatamente o mesmo endpoint e a mesma lógica.

O único obstáculo real é outro: `finalizarVerificacaoStatusUsuario` bloqueia **qualquer** rota protegida para um `estudante` com `Status="inativo"` (o estado que resulta de `DesvincularDaAcademia` quando o estudante fica sem nenhum vínculo ativo). Um estudante nessa situação não consegue autenticar-se para nada hoje — incluindo para pagar uma dívida antiga.

## Regra de negócio

1. O endpoint de seleção (seção 2.1) e o de geração de cobrança (seção 3) funcionam de forma idêntica para a academia atual e para academias anteriores — a única condição é existir uma pendência real (Fase 2) na combinação `(codigo_estudante, codigo_academia, ano_letivo, mês)` informada.
2. A regra de sequência (seção 2, itens 2–4) aplica-se **por academia, de forma independente** — já coberto pela seção 2.
3. Um estudante com `Status="inativo"` (sem nenhum vínculo ativo em academia alguma) deve conseguir autenticar-se, mas **exclusivamente para fins financeiros** — consultar e pagar pendências — permanecendo bloqueado de **todas** as demais rotas protegidas, exatamente como hoje. Esta é uma exceção estreita e específica ao bloqueio de login por status, não uma reabertura geral de acesso para estudantes inativos.
4. Pagar uma pendência de uma academia anterior **não** reintegra o estudante nessa academia nem cria qualquer vínculo — reintegração continua a exigir exclusivamente o fluxo `Reintegrar` já existente, que é um processo académico separado, decidido pela academia, não uma consequência automática de um pagamento.

## Escopo obrigatório

### 6.1 Exceção estreita ao bloqueio de login por status

Introduzir, no fluxo de autenticação de `estudante`, uma via que permita login com `Status="inativo"` marcando a sessão resultante de forma explícita (ex.: uma claim/contexto `acesso_restrito_financeiro=true"`), e que **apenas os handlers do módulo financeiro** reconheçam e aceitem essa sessão — todas as demais rotas protegidas devem continuar a rejeitar exatamente como hoje (`finalizarVerificacaoStatusUsuario` inalterado para qualquer rota fora do financeiro). Esta é uma alteração de segurança sensível: deve ser implementada como uma verificação adicional e explícita nos handlers financeiros, nunca como um enfraquecimento geral do bloqueio existente.

### 6.2 Descoberta de pendências entre academias

Estender a consulta de mensalidades da Fase 2 (seção 3.2) — já preparada para agrupar por `codigo_academia` — para ser acessível também por um estudante autenticado via 6.1 (`acesso_restrito_financeiro`), devolvendo as pendências de todas as academias com que já teve vínculo (reaproveitando o histórico de vínculo já usado na Fase 2, Seção 6, para enumerar essas academias).

### 6.3 Testes obrigatórios

1. estudante ativo na Academia B com pendência antiga na Academia A: consegue selecionar e pagar a pendência da Academia A através do mesmo endpoint (seção 2.1), sem que isso interfira com as pendências da Academia B;
2. estudante com `Status="inativo"` (sem nenhum vínculo ativo) consegue autenticar-se apenas para consultar (6.2) e pagar (seção 2.1/3) pendências;
3. o mesmo estudante `inativo`, com a mesma sessão obtida em 6.1, continua bloqueado em qualquer rota protegida fora do módulo financeiro (testar explicitamente pelo menos uma rota não financeira, ex.: consulta de notas/faltas);
4. pagamento de uma pendência antiga usa o valor histórico correto (Fase 2, Seção 6) da academia/ano/curso em que o estudante estava naquela altura, não o estado atual;
5. pagar uma pendência de uma academia anterior não cria nenhum vínculo nem altera `Status`/`CodigoAcademia` do estudante — permanece `inativo` e sem academia até um `Reintegrar` explícito e separado.

---

# Fora de escopo

- Parcelamento de uma única mensalidade em múltiplos pagamentos menores.
- Troca de método de pagamento depois de a cobrança já ter sido criada (o estudante deve cancelar — Fase 1 — e selecionar novamente, se aplicável).
- Reembolso de mensalidade já paga.
- Notificação ao estudante/encarregado sobre confirmação de pagamento (módulo WhatsApp, ainda em desenho).
- Reativação automática de vínculo com uma academia anterior ao pagar uma pendência dela (seção 6). Pagar uma dívida antiga não reintegra o estudante nessa academia — reintegração continua a exigir o fluxo `Reintegrar` já existente, inteiramente separado deste mecanismo de pagamento.

# Riscos e mitigações

| Risco | Mitigação |
| --- | --- |
| Cobrança cobrindo múltiplos meses ser paga parcialmente (não suportado pela AppyPay, mas relevante para falhas do lado Spuri) | Confirmação de pagamento é atômica sobre todos os meses cobertos (seção 4.2); a AppyPay já trata a cobrança como uma unidade única, não parcelável |
| Cobranças duplicadas para o mesmo mês | Verificação de cobrança em aberto antes de gerar nova (seção 3.2) |
| Estudante conseguir pular o mês mais antigo pendente | Validação estrita de sequência na seleção (seção 2), antes de qualquer chamada à AppyPay |
| Webhook/consulta processado mais de uma vez duplicando pagamento | Idempotência explícita na gravação dos eventos de pagamento (seção 4.2), testada (4.3.3) |
| Exceção de login para estudante `inativo` (seção 6.1) enfraquecer o bloqueio de segurança para outras rotas | Verificação adicional e explícita, restrita aos handlers financeiros; testada contra pelo menos uma rota não financeira (6.3.3) |
| Pagamento de pendência antiga ser confundido com reintegração automática na academia anterior | Regra de negócio explícita (seção 6, item 4) e testada (6.3.5): pagamento nunca cria vínculo |

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. as Fases 1 e 2 estiverem concluídas e integralmente reutilizadas (tipos monetários, cancelamento, cálculo de meses devidos);
2. a academia conseguir restringir quais métodos ficam disponíveis para propina;
3. a regra de sequência de seleção (mês único = mais antigo; múltiplos = primeiro é o mais antigo, demais livres) estiver implementada e testada, aplicada de forma independente por academia;
4. a geração da cobrança for automática, sem exigir ação manual de um funcionário da academia, e cobrir exatamente os meses selecionados (de uma única academia) numa única cobrança;
5. duplicidade de cobrança para o mesmo mês estiver bloqueada;
6. a confirmação de pagamento marcar atomicamente todos os meses cobertos, com idempotência testada;
7. a anulação de uma obrigação com cobrança em aberto cancelar automaticamente essa cobrança (seção 5), com a limitação residual da AppyPay documentada de forma explícita;
8. um estudante conseguir pagar pendências de qualquer academia com que já teve vínculo, incluindo estando sem nenhum vínculo ativo, sem que isso reabra acesso a nenhuma rota fora do módulo financeiro (seção 6);
9. todos os testes das seções 1 a 6 passarem;
10. `Documentação.md` estiver atualizada com o novo fluxo, endpoints, eventos, a limitação residual descrita na seção 5, e a exceção de autenticação descrita na seção 6.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Módulo de pagamentos — Fase 3 — Cobrança de mensalidade/propina versátil para o estudante (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
