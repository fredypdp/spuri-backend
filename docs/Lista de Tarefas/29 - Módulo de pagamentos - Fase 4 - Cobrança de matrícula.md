---
criado: 2026-08-12 00:00
origem: pedido direto do Spuri (evolução do módulo financeiro após a tarefa 17)
status: pendente
depende_de: "26 - Módulo de pagamentos - Fase 1 - Bases (tipo do amount e cancelamento de cobrança).md"
---

# Módulo de pagamentos — Fase 4 — Cobrança de matrícula (pendente)

## Prompt recomendado para executar a atualização

Implemente, sobre a Fase 1 (`26 - ...md`, que deve estar concluída antes de iniciar esta tarefa), a cobrança opcional de taxa de matrícula no canal de solicitação de matrícula (`internal/domain/aggregates/solicitacao_matricula.go`, `internal/handlers/solicitacao_matricula_handlers.go`). A solicitação de matrícula continua gratuita; apenas a **aprovação** passa a poder exigir pagamento antes do vínculo efetivo do estudante à academia, quando a academia tiver configurado cobrança de matrícula para aquele nível/ano. A cobrança em si **não** é criada automaticamente na aprovação — é gerada sob demanda, quando o pagador (via novo endpoint público autenticado por posse do `codigo_solicitacao`) escolhe o método entre os habilitados pela academia, seguindo o mesmo princípio já usado na Fase 3. Esta fase é independente das Fases 2/3 (mensalidade) — aplica-se a **todas** as academias (públicas e privadas), não apenas privadas. Siga o padrão de event sourcing já estabelecido e reutilize `roundAmount`/`amountsEqual` e `CancelCharge` da Fase 1 para qualquer valor monetário/cancelamento. Ao final, atualize testes, `Documentação.md` e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou fallbacks temporários.

## Contexto

Auditoria do fluxo atual de aprovação de matrícula (`internal/handlers/solicitacao_matricula_handlers.go`): hoje, ao aprovar uma `SolicitacaoMatricula`, o handler chama `est.CriarComVinculo(...)`, que cria o aggregate `Estudante` **já vinculado** à academia (`codigo_academia` preenchido) no mesmo passo da aprovação. O aggregate `Estudante` já expõe o padrão `criarComVinculoComStatus(..., statusGeral)`, usado hoje para o caso de vínculo pendente de documentos (`StatusGeral = "pendente_documentos"`) — um precedente direto de "vínculo que existe mas está condicionado a algo pendente".

O pedido do Spuri, porém, é mais restrito que esse precedente: "o estudante já não é vinculado à academia" enquanto o pagamento da matrícula estiver pendente — ou seja, não basta um `Estudante` vinculado com status pendente; o vínculo em si não deve existir ainda. Por isso, esta fase **não** reaproveita `criarComVinculoComStatus` para este caso — mantém a lógica de pagamento pendente inteiramente dentro do aggregate `SolicitacaoMatricula` (que já tem o campo `codigo_estudante_gerado`, reservado no momento da aprovação, mas hoje só consumido no ato de `CriarComVinculo`), e só invoca `CriarComVinculo` (o "processo natural já existente", conforme pedido pelo Spuri) depois de confirmado o pagamento.

Diferente das Fases 2/3, a cobrança de matrícula não é exclusiva de academias privadas — o pedido original diz explicitamente "as academias (todas)".

**Correção importante face à primeira versão deste documento:** a cobrança de matrícula **não** deve ser gerada automaticamente pela plataforma no instante da aprovação. Duas razões, encontradas ao revisar o desenho junto do Spuri: (1) a academia pode habilitar mais de um método de pagamento para matrícula (seção 1) — se a cobrança fosse criada automaticamente na aprovação, não haveria critério para escolher qual método usar; (2) o princípio geral do módulo (já usado na Fase 3) é que **toda** cobrança é gerada na hora, pelo próprio pagador, no momento em que ele escolhe os parâmetros (mês(es)/método), e não previamente pela plataforma. Esta fase segue o mesmo princípio: a cobrança de matrícula só é criada quando o pagador escolhe o método, num novo passo explícito (seção 3).

Isto expõe uma limitação real do sistema atual: **não existe hoje nenhum endpoint público** para quem submeteu a solicitação (encarregado/candidato) consultar ou agir sobre ela depois da submissão — `POST /solicitacao-matricula` (criação) é a única rota pública; todas as demais (`GET /solicitacao-matricula/:codigo`, aprovar, reprovar) exigem sessão de academia ou admin `fpp`. Não existe conta, password nem PIN para o solicitante — apenas o `codigo_solicitacao`, gerado com `crypto/rand` sobre um alfabeto de 36 caracteres em 11 posições (~36¹¹ combinações, criptograficamente forte). Esta fase precisa, portanto, de introduzir um novo endpoint público que use o `codigo_solicitacao` como credencial de posse (à semelhança de como uma referência de pagamento já funciona) para permitir que o pagador escolha o método e acione a geração da cobrança — ver seção 3.

---

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Quem pode cobrar matrícula | Qualquer academia (pública ou privada) | Diferente das Fases 2/3, não há restrição por `Type` |
| Granularidade do valor | Por nível/ano académico da academia (mesma granularidade da Fase 2: fundamental por ano; médio/superior por curso+ano) | Consistência entre as fases |
| Solicitação de matrícula | Continua gratuita | Nenhuma cobrança antes da aprovação |
| Aprovação sem cobrança configurada | Fluxo atual inalterado — vínculo imediato | Retrocompatível com academias que não cobram matrícula |
| Aprovação com cobrança configurada | Estudante **não** é vinculado imediatamente; solicitação entra em estado de pagamento pendente | Vínculo só ocorre após confirmação de pagamento |
| Quem aciona a geração da cobrança | O próprio pagador (solicitante), escolhendo o método entre os habilitados pela academia, via novo endpoint público autenticado por posse do `codigo_solicitacao` | Cobrança gerada na hora, nunca previamente pela plataforma — mesmo princípio da Fase 3 |
| Confirmação de pagamento | Dispara `CriarComVinculo`, o fluxo natural já existente | Sem duplicar nem reinventar a criação do estudante |

---

# 1. Configuração de cobrança de matrícula por academia

## Objetivo

Permitir que qualquer academia decida se cobra taxa de matrícula e, em caso afirmativo, quanto cobra por cada nível/ano académico que oferece, e qual(is) método(s) de pagamento disponibiliza para essa cobrança.

## Regra de negócio

- Toda academia (pública ou privada) pode configurar cobrança de matrícula — sem a restrição de `Type` usada nas Fases 2/3.
- Mesma granularidade da Fase 2: fundamental configurado por ano; médio/superior configurado por `(curso, ano)`.
- Ausência de configuração para um nível/ano específico significa que a matrícula para aquele nível/ano permanece gratuita e o fluxo atual (vínculo imediato na aprovação) continua inalterado — sem exigir migração de dados nem opt-in explícito para academias que nunca configurarem nada.
- A academia escolhe, entre os métodos que já configurou na base do módulo financeiro (tarefa 17), quais ficam disponíveis para cobrança de matrícula (mesmo princípio da Fase 3, seção 1, mas para matrícula).

## Escopo obrigatório

### 1.1 Configuração

Criar (ou estender a estrutura da Fase 2, se a equipa preferir um único aggregate de configuração financeira por academia — decisão a registrar explicitamente no PR) a configuração de valor de matrícula por `nivel+ano[+curso_id]`, mais o(s) método(s) habilitado(s) para matrícula.

### 1.2 Validação

- validar que o `ano`/`curso_id` existe na academia, reaproveitando a mesma validação da Fase 2;
- validar valor conforme Fase 1 (`float64`, `roundAmount`, `> 0`, no máximo 2 casas decimais);
- validar que o(s) método(s) escolhido(s) tem(êm) credencial configurada.

### 1.3 Testes obrigatórios

1. configurar valor de matrícula para academia pública, com sucesso (diferente da Fase 2, aqui deve funcionar);
2. configurar valor de matrícula para academia privada, com sucesso;
3. tentar configurar para ano/curso que a academia não oferece → rejeitado;
4. academia sem nenhuma configuração: aprovação de matrícula segue o fluxo atual (gratuito, vínculo imediato) sem qualquer alteração de comportamento.

---

# 2. Aprovação de solicitação com pagamento de matrícula pendente

## Objetivo

Alterar o fluxo de aprovação de `SolicitacaoMatricula` para que, quando houver cobrança de matrícula configurada para o nível/ano da solicitação, o estudante **não** seja vinculado imediatamente — a solicitação entra num estado de pagamento pendente.

## Regra de negócio

1. No momento da aprovação (`AprovarSolicitacaoMatricula`), verificar se existe configuração de valor de matrícula (seção 1) para o nível/ano da solicitação:
   - **se não existir:** comportamento atual inalterado — `CriarComVinculo` é chamado imediatamente, como hoje;
   - **se existir:** a solicitação muda para um novo status (ex.: `aprovada_pendente_pagamento_matricula`), **sem** chamar `CriarComVinculo` — o `Estudante` ainda não é criado. `codigo_estudante_gerado` continua reservado (já é gerado no momento da aprovação hoje) para ser usado apenas quando o vínculo finalmente ocorrer.
2. Enquanto a solicitação estiver em `aprovada_pendente_pagamento_matricula`, o estudante não aparece em nenhuma contagem/listagem de estudantes vinculados da academia (porque o `Estudante` ainda não existe).
3. O evento de aprovação (já existente) deve ser complementado/acompanhado por um novo evento explícito que registre a entrada no estado de pagamento pendente, mantendo a auditoria completa de que a academia aprovou, mas o vínculo ainda não se efetivou.

## Escopo obrigatório

### 2.1 Alterar o handler de aprovação

Em `internal/handlers/solicitacao_matricula_handlers.go`, antes de chamar `est.CriarComVinculo(...)`, consultar a configuração de matrícula (seção 1) para o nível/ano da solicitação; se houver cobrança configurada, gravar o novo status/evento em vez de criar o `Estudante`.

### 2.2 Novo estado no aggregate

Adicionar `aprovada_pendente_pagamento_matricula` como status válido de `SolicitacaoMatricula`, com o evento correspondente (ex.: `SolicitacaoMatriculaAprovadaPendentePagamento`), seguindo o padrão de eventos já existente no aggregate.

### 2.3 Cancelamento de solicitação pendente de pagamento

Definir explicitamente (e implementar) o que acontece se o pagamento da matrícula nunca for concluído: a academia deve poder cancelar uma solicitação em `aprovada_pendente_pagamento_matricula`, revertendo-a para um estado terminal (ex.: reaproveitar/estender o mecanismo de cancelamento de solicitação já existente), liberando o `codigo_estudante_gerado` e impedindo que um pagamento tardio ainda vincule o estudante depois do cancelamento. Se já existir uma cobrança em aberto associada (gerada pela seção 3), este cancelamento deve também acionar `Service.CancelCharge` (Fase 1) sobre ela, pelo mesmo motivo e com a mesma limitação residual da AppyPay já descrita na Fase 1 (item 8 da regra de negócio da Fase 1, seção 2) e na Fase 3 (seção 5) — reduz mas não elimina o risco de um pagamento tardio pela via já cancelada.

### 2.4 Testes obrigatórios

1. aprovar solicitação em academia/nível sem cobrança configurada: vínculo imediato, comportamento idêntico ao atual;
2. aprovar solicitação em academia/nível com cobrança configurada: `Estudante` não é criado, solicitação fica em `aprovada_pendente_pagamento_matricula`, sem nenhuma cobrança ainda criada;
3. estudante em `aprovada_pendente_pagamento_matricula` não aparece em listagens/contagens de estudantes vinculados da academia;
4. academia cancela uma solicitação em `aprovada_pendente_pagamento_matricula` sem cobrança gerada: estado terminal correto, `codigo_estudante_gerado` liberado;
5. academia cancela uma solicitação em `aprovada_pendente_pagamento_matricula` com cobrança já gerada e ainda não paga: estado terminal correto e a cobrança é cancelada automaticamente (`CancelCharge`);
6. academia tenta cancelar uma solicitação cuja cobrança já foi paga → rejeitado (o vínculo deve seguir o fluxo normal da seção 4, não o cancelamento).

---

# 3. Geração da cobrança de matrícula, acionada pelo pagador

## Objetivo

Permitir que o pagador (quem submeteu a solicitação) escolha o método de pagamento, entre os habilitados pela academia (seção 1), e acione a geração da cobrança de matrícula na AppyPay — seguindo o mesmo princípio da Fase 3: nenhuma cobrança é criada previamente pela plataforma; é sempre o pagador quem, ao configurar os parâmetros disponíveis, desperta a criação da cobrança.

## Regra de negócio

1. A cobrança só pode ser gerada enquanto a solicitação estiver exatamente em `aprovada_pendente_pagamento_matricula`. Fora desse estado (pendente, reprovada, cancelada, ou já vinculada), a tentativa deve ser rejeitada com erro claro.
2. Autenticação do pagador nesta ação é por posse do `codigo_solicitacao` (não existe login para quem ainda não é estudante) — ver "Contexto" sobre a entropia do código. O pagador informa o `codigo_solicitacao` e o método escolhido (dentre os habilitados pela academia); não há outro dado de identificação necessário, mas o endpoint deve responder com erro genérico e uniforme para código inexistente/errado ou estado inválido, sem distinguir os dois casos na mensagem (para não vazar existência de códigos por enumeração).
3. Só pode existir uma cobrança em aberto por solicitação. Se o pagador tentar gerar uma nova cobrança enquanto já existir uma em aberto (criada, não paga, não cancelada, não falhada), o pedido deve ser rejeitado, informando a cobrança já existente (ou instruindo a cancelá-la primeiro — reaproveitando o cancelamento da seção 2.3, se a academia permitir; decisão de UX a validar com o Spuri durante a implementação).
4. **Valor fixado no momento da aprovação, não no momento do pagamento:** o valor da cobrança é o que estava configurado (seção 1) para o nível/ano da solicitação **no instante em que ela entrou em `aprovada_pendente_pagamento_matricula`** (seção 2), e deve ser gravado nesse momento (no próprio evento da seção 2.2) — não recalculado a partir da configuração atual quando o pagador finalmente aciona a geração da cobrança (seção 3.1). Isto segue o mesmo princípio da Fase 2 (Seção 6): se a academia mudar o valor da matrícula entretanto, isso não deve afetar solicitações já aprovadas e pendentes de pagamento. O pagador nunca pode alterar este valor.
5. A cobrança é criada com `ContextoTipo="academia"`, associada de forma auditável à `SolicitacaoMatricula` de origem (`codigo_solicitacao` no payload), para permitir a confirmação da seção 4 e o cancelamento em cascata da seção 2.3.

## Escopo obrigatório

### 3.1 Novo endpoint público

Criar rota pública (ex.: `POST /solicitacao-matricula/:codigo/pagamento-matricula`) que recebe o método escolhido, valida o estado da solicitação (regra 1), valida que o método pertence ao conjunto habilitado pela academia (seção 1), verifica ausência de cobrança em aberto (regra 3), e chama `Service.CreateCharge` (REF/GPO) ou `Service.CreateGPOQRCode` com os dados da regra de negócio. Aplicar limitação de taxa (rate limiting) a este endpoint — mesmo com um `codigo_solicitacao` de alta entropia, é uma ação financeira pública e deve ser protegida contra automação abusiva, seguindo qualquer mecanismo de rate limiting já existente no projeto ou, na ausência de um, propor o mínimo necessário para este endpoint especificamente.

### 3.2 Consulta do estado da solicitação pelo pagador

Se ainda não existir, criar também uma rota pública mínima de consulta (ex.: `GET /solicitacao-matricula/:codigo/status`) que devolve apenas o essencial para o pagador saber se há matrícula pendente de pagamento e, se sim, quais métodos estão disponíveis e qual o valor — sem expor dados sensíveis da solicitação (documentos, dados pessoais completos) que hoje só a academia/admin veem.

### 3.3 Testes obrigatórios

1. gerar cobrança com a solicitação em `aprovada_pendente_pagamento_matricula` e método habilitado → sucesso, valor correto;
2. academia muda o valor da matrícula depois de uma solicitação já aprovada (pendente de pagamento): a cobrança gerada usa o valor fixado na aprovação, não o novo valor;
3. tentar gerar cobrança com `codigo_solicitacao` inexistente ou solicitação fora do estado correto → rejeitado com a mesma mensagem genérica em ambos os casos;
4. tentar gerar cobrança com método não habilitado pela academia para matrícula → rejeitado;
5. tentar gerar uma segunda cobrança enquanto já existe uma em aberto → rejeitado;
6. falha na criação da cobrança na AppyPay (ex.: erro de rede) não deixa a solicitação num estado inconsistente, e permite nova tentativa;
7. o endpoint de consulta (3.2) não expõe dados pessoais além do necessário para decidir o pagamento.

---

# 4. Confirmação de pagamento efetiva o vínculo

## Objetivo

Quando a cobrança de matrícula for confirmada como paga, vincular o estudante à academia seguindo o processo natural já existente (`CriarComVinculo`).

## Regra de negócio

- A confirmação (via webhook ou `ConsultCharge`, Fase 1) que detectar sucesso para uma cobrança de matrícula (gerada pela seção 3) associada a uma `SolicitacaoMatricula` em `aprovada_pendente_pagamento_matricula` deve, atomicamente: (a) chamar `CriarComVinculo` com os dados já validados da solicitação (mesmo caminho hoje usado na aprovação direta, sem duplicar lógica de validação), usando o `codigo_estudante_gerado` já reservado; (b) marcar a solicitação como concluída/vinculada.
- Idempotência obrigatória: se a confirmação for processada mais de uma vez (reentrega de webhook), o vínculo não deve ser criado em duplicado.
- Se a solicitação já tiver sido cancelada (seção 2.3) antes da confirmação de pagamento chegar, o pagamento tardio **não** deve criar o vínculo — deve gerar um evento de conflito, análogo ao da Fase 1 (seção 2.6, "conflito pós-cancelamento"), para reconciliação manual FPP (nesse caso, o dinheiro foi cobrado mas o vínculo não pode mais ocorrer automaticamente).

## Escopo obrigatório

### 4.1 Integração com a confirmação de cobrança

Estender o ponto de confirmação de pagamento (`ConsultCharge`/webhook) para reconhecer cobranças do tipo "matrícula" e, ao detectar sucesso, disparar o vínculo conforme a regra de negócio.

### 4.2 Testes obrigatórios

1. confirmação de pagamento de matrícula vincula o estudante corretamente, com os mesmos dados que seriam usados numa aprovação direta;
2. reentrega do mesmo webhook não duplica o vínculo;
3. solicitação cancelada antes da confirmação chegar: pagamento tardio gera evento de conflito, sem criar vínculo;
4. após o vínculo efetivado, o estudante aparece normalmente em listagens/contagens da academia, exatamente como uma matrícula aprovada sem cobrança.

---

# Fora de escopo

- Reembolso da taxa de matrícula, inclusive se o vínculo for desfeito posteriormente (desvínculo, cancelamento de matrícula já efetivada) — trate como tarefa futura separada, se necessário.
- Cobrança de matrícula fora do canal de solicitação de matrícula (ex.: cadastro direto pela academia, `MatriculaContextCadastroDireto` em `internal/services/matricula_validation.go`) — permanece gratuito, conforme o pedido original restringir isto ao "canal de solicitação de matrícula".
- Alteração da lógica de validação de dados da solicitação (`internal/services/matricula_validation.go`) — esta fase só afeta o que acontece **depois** da aprovação.
- Notificação ao estudante/encarregado sobre a cobrança de matrícula (módulo WhatsApp, ainda em desenho).

# Riscos e mitigações

| Risco | Mitigação |
| --- | --- |
| Pagamento confirmado depois de a solicitação já ter sido cancelada | Evento de conflito dedicado (seção 4, regra de negócio), nunca cria vínculo silenciosamente |
| Reentrega de webhook duplicar o vínculo do estudante | Idempotência explícita testada (seção 4.2.2) |
| Estudante em pagamento pendente aparecer indevidamente como vinculado em alguma listagem já existente | Teste dedicado (seção 2.4.3) cobrindo todas as listagens/contagens relevantes de estudantes da academia |
| Falha na criação da cobrança deixar a solicitação "presa" sem caminho para nova tentativa | Tratamento explícito de falha com possibilidade de nova tentativa (seção 3.1) |
| `codigo_solicitacao` ser a única credencial de um endpoint público que aciona uma cobrança financeira | Alta entropia do código (~36¹¹ combinações, `crypto/rand`), resposta genérica e uniforme para código inválido/estado inválido (evita enumeração), e rate limiting dedicado ao endpoint (seção 3.1) |
| Solicitante gerar múltiplas cobranças para a mesma matrícula | Verificação de cobrança em aberto antes de permitir nova geração (seção 3, regra 3) |

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. a Fase 1 estiver concluída e reutilizada (tipo monetário, cancelamento de cobrança);
2. qualquer academia (pública ou privada) puder configurar cobrança de matrícula por nível/ano, com um ou mais métodos habilitados;
3. academias sem configuração mantiverem o comportamento atual (vínculo imediato na aprovação), sem quebra de retrocompatibilidade;
4. academias com configuração passarem a ter aprovação sem vínculo imediato, com solicitação em `aprovada_pendente_pagamento_matricula` e **sem** nenhuma cobrança criada nesse momento;
5. o pagador conseguir gerar a cobrança de matrícula pelo novo endpoint público, autenticado por posse do `codigo_solicitacao`, escolhendo entre os métodos habilitados;
6. não for possível gerar mais de uma cobrança em aberto para a mesma solicitação;
7. a confirmação de pagamento vincular o estudante pelo processo natural já existente (`CriarComVinculo`), de forma idempotente;
8. cancelamento de solicitação pendente de pagamento cancelar automaticamente qualquer cobrança em aberto associada, e o conflito de pagamento tardio pós-cancelamento estar coberto por teste;
9. o endpoint de geração de cobrança tiver rate limiting e respostas genéricas contra enumeração de `codigo_solicitacao`;
10. todos os testes das seções 1 a 4 passarem;
11. `Documentação.md` estiver atualizada com o novo estado de solicitação, os novos endpoints públicos, os novos eventos, e o novo comportamento condicional na aprovação.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Módulo de pagamentos — Fase 4 — Cobrança de matrícula (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
