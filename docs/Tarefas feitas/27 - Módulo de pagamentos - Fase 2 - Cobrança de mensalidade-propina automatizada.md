---
criado: 2026-08-12 00:00
origem: pedido direto do Spuri (evolução do módulo financeiro após a tarefa 17)
status: feito
depende_de: "26 - Módulo de pagamentos - Fase 1 - Bases (tipo do amount e cancelamento de cobrança).md"
---

# Módulo de pagamentos — Fase 2 — Cobrança de mensalidade/propina automatizada (feito)

## Prompt recomendado para executar a atualização

Implemente, sobre as bases da Fase 1 (`26 - Módulo de pagamentos - Fase 1 - Bases...md`, que deve estar concluída antes de iniciar esta tarefa), o rastreio automatizado da obrigação de pagamento de mensalidade/propina por estudante, exclusivo de academias de natureza privada. Esta fase **não** cria cobranças na AppyPay — ela apenas determina, de forma automática e auditável, quais meses cada estudante deve pagar e quais já pagou. A criação efetiva da cobrança AppyPay é responsabilidade da Fase 3 (`28 - ...md`). Siga o padrão de event sourcing já estabelecido (nenhuma mudança de estado sensível sem evento correspondente no ledger) e o padrão de tarefa já usado no repositório. Ao final, atualize testes, `Documentação.md` e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou fallbacks temporários.

## Contexto

Auditoria do código relevante:

- `Academia.Type` já distingue `"public"`/`"private"` — mapeia diretamente para "natureza da academia" mencionada no pedido. Mensalidade/propina é exclusiva de `Type == "private"`.
- O formato canónico de ano académico já usado no sistema (`internal/utils/validation.go`, `Estudante`) é `[n]_ano_fundamental`, `[n]_ano_medio`, `[n]_ano_superior`. Para o **fundamental**, a lista de anos oferecidos vive em `Academia.AnosAcademicos`. Para o **médio e o superior**, não existe uma lista de anos ao nível da Academia — os anos oferecidos vivem em `Curso.AnosAcademicos`, por curso (`internal/domain/aggregates/curso.go`). Isto significa que "quanto a academia cobra por cada ano académico" não pode ser uma configuração plana por ano no médio/superior sem também considerar o curso, porque cursos diferentes da mesma academia podem (e tipicamente vão) ter mensalidades diferentes para o "mesmo" número de ano. Esta tarefa assume, como extensão consciente de escopo, que a configuração de valor é: **por (nível, ano) para o fundamental**, e **por (curso, ano) para o médio e o superior**. Se a equipa decidir diferente durante a implementação, registre isso explicitamente no PR.
- O período letivo hoje é **fixo e imutável**: `09_07` (escolar — cobre fundamental e médio) e `10_07` (superior), validado por `validarPeriodoLetivoFixoPayload` (`internal/handlers/ano_letivo_helpers.go`). Esta tarefa **não** mexe nesse período académico fixo (usado para matrícula, faltas, notas, finalização de ano letivo). O "mês final de cobrança" (junho ou julho) pedido aqui é um conceito **novo, exclusivo do módulo financeiro**, que só pode **limitar** até onde a mensalidade é cobrada dentro do período letivo fixo já existente — nunca estendê-lo além do mês final já fixado (`07` para ambos os tipos hoje).
- Não existe nenhum scheduler/cron no repositório (sem `render.yaml`, sem `time.Ticker` de negócio) — o sistema de `internal/jobs` é processamento de lotes assíncronos, não agendamento por tempo. Por isso, "cobrança automática" nesta fase significa **cálculo sob procura** (uma projeção derivada, recalculável a qualquer momento a partir da configuração da academia e dos eventos já registrados), e não um processo em segundo plano que corre todo início de mês. Isto também está alinhado com o princípio já registrado no histórico do projeto de evitar qualquer coisa que impeça o NeonDB de entrar em idle.
- Não existe hoje nenhuma tabela, aggregate ou conceito de "mensalidade"/"propina" no código — esta fase é inteiramente nova.

---

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Quem cobra mensalidade | Apenas academias com `Type == "private"` | Academias públicas nunca geram obrigação de mensalidade |
| Granularidade da configuração de valor | Fundamental: por (nível, ano). Médio/Superior: por (curso, ano) | Reflete que cursos diferentes podem ter valores diferentes |
| Mês final de cobrança | Configurável por ano académico, valor `6` (junho) ou `7` (julho) | Limitado ao mês final do período letivo fixo já existente (nunca ultrapassa) |
| Cálculo de meses devidos/pagos | Projeção derivada, calculada sob procura, sem cron | Não compromete o comportamento de idle do NeonDB |
| Identidade do mês devido | Chave única `(estudante, academia, ano_letivo, mês)` | Reinicia a cada ano letivo; distingue academias diferentes se o estudante transferir; pendências de anos anteriores continuam pendentes e distinguíveis |
| Entrada a meio do ano letivo | Mês de início de cobrança configurável, válido apenas no ano letivo de entrada | A partir do ano letivo seguinte, prevalece a regra normal do período completo |
| Isenção individual | Evento auditável por (estudante, academia, ano_letivo, mês), reversível apenas por novo evento explícito | Nunca apaga o histórico; nunca é feito por admin `fpp`, só pela própria academia |
| Consulta de mensalidades | Academia dona, o próprio estudante, ou qualquer admin `fpp` sem restrição | Leitura é irrestrita para supervisão; ações (anular/reativar) não são |
| Valor de mês pendente | Sempre resolvido pelo ano académico/curso e pelo preço **em vigor na data de referência daquele mês**, nunca pelo estado/preço atuais | Meses de anos letivos anteriores mantêm o valor correto mesmo após progressão do estudante ou mudança de preço |

---

# 1. Configuração de valor de mensalidade por ano académico

## Objetivo

Permitir que academias privadas configurem quanto cobram de mensalidade por cada ano académico que oferecem.

## Regra de negócio

- Só academias com `Type == "private"` podem configurar e ter mensalidade cobrada. Uma tentativa de configuração numa academia pública deve ser rejeitada com erro explícito.
- Valor monetário segue exatamente o contrato da Fase 1 (`float64`, `roundAmount`, validado com no máximo 2 casas decimais, `> 0`).
- Granularidade (ver "Contexto"): fundamental por `(nivel_escolar_implicito, ano)` — na prática, apenas por `ano` dado que fundamental já é um nível único; médio e superior por `(curso_id, ano)`.
- Ausência de configuração para um determinado ano/curso significa que **nenhuma** obrigação de mensalidade é gerada para estudantes nesse ano/curso — sem valor padrão implícito, seguindo o mesmo princípio já usado noutras configurações do sistema (ex.: `limite_faltas_percentual`).

## Escopo obrigatório

### 1.1 Novo aggregate

Criar um aggregate dedicado (sugestão: `ConfiguracaoMensalidade`, um por academia, com uma coleção interna de entradas `{nivel, ano, curso_id?, valor, mes_fim_cobranca}` — ver seção 2 para `mes_fim_cobranca`) em vez de sobrecarregar o aggregate `Academia`, dado o volume de combinações possíveis (fundamental tem até 9 anos; médio e superior variam por curso). Evento sugerido: `MensalidadeConfigurada` (upsert por chave `nivel+ano[+curso_id]`).

### 1.2 Endpoints

Criar rotas para configurar (`POST`/`PUT`) e listar a configuração de mensalidade da academia. Autorização: a própria academia (dona da configuração) ou um admin `fpp`, seguindo o padrão de autorização já usado noutras rotas administrativas de academia.

### 1.3 Validação

- rejeitar configuração para academia com `Type != "private"`;
- validar que `ano` pertence a `Academia.AnosAcademicos` (fundamental) ou a `Curso.AnosAcademicos` do `curso_id` informado (médio/superior), reaproveitando validação já existente para esses formatos;
- validar `valor` conforme Fase 1.

### 1.4 Testes obrigatórios

1. configurar valor para academia privada, fundamental, com sucesso;
2. configurar valor para academia privada, médio/superior, por curso, com sucesso;
3. tentar configurar em academia pública → rejeitado;
4. tentar configurar para ano/curso que a academia não oferece → rejeitado;
5. reconfigurar (upsert) um valor já existente gera novo evento auditável, sem apagar o histórico anterior no ledger.

---

# 2. Mês final de cobrança por ano académico (junho ou julho)

## Objetivo

Permitir que a academia defina, por ano académico, até que mês a mensalidade é cobrada — refletindo que alguns anos académicos terminam mais cedo (junho) que outros (julho).

## Regra de negócio

- `mes_fim_cobranca` é parte da mesma configuração da seção 1 (por `nivel+ano[+curso_id]`), com valores válidos restritos a `6` (junho) ou `7` (julho), conforme pedido explicitamente pelo Spuri.
- Este valor **nunca** pode exceder o mês final do período letivo fixo já existente para o tipo daquele ano (`periodoLetivoEscolar`/`periodoLetivoSuperior`, hoje ambos terminando em `07`) — validar isso explicitamente contra o tipo da academia/curso em questão, para não criar uma inconsistência entre "quando a academia pode ter aulas/avaliações" e "até quando cobra mensalidade".
- Este mês final é **exclusivo do domínio financeiro**. Não deve, em nenhuma circunstância, alterar `periodoLetivoEscolar`/`periodoLetivoSuperior` nem `validarPeriodoLetivoFixoPayload`, que continuam a reger matrícula, faltas, notas e finalização de ano letivo exatamente como hoje.
- Ausência de configuração para um ano/curso significa que esse ano/curso não gera mensalidade (ver seção 1).

## Escopo obrigatório

### 2.1 Validação de faixa

Validar `mes_fim_cobranca ∈ {6, 7}` e que não excede o mês final do período letivo fixo do tipo correspondente.

### 2.2 Testes obrigatórios

1. configurar `mes_fim_cobranca = 6` com sucesso;
2. configurar `mes_fim_cobranca = 7` com sucesso;
3. tentar configurar um valor fora de `{6,7}` → rejeitado;
4. confirmar, por teste, que nenhuma alteração desta configuração afeta `validarPeriodoLetivoFixoPayload` nem qualquer fluxo de matrícula/faltas/notas.

---

# 3. Cálculo automático de meses devidos e pagos

## Objetivo

Sem exigir qualquer configuração manual por estudante, determinar automaticamente, para cada estudante de academia privada, quais meses do ano letivo corrente estão pendentes e quais já foram pagos.

## Regra de negócio

1. O conjunto de meses "devidos" por um estudante num ano letivo é: do mês de início do período letivo fixo do seu tipo (`09` escolar, `10` superior) — ou do mês de início configurado pela academia para o ano letivo de entrada dela na plataforma (ver seção 4), se aplicável — até `mes_fim_cobranca` configurado para o `(nível/curso, ano académico)` em que o estudante se encontra naquele ano letivo.
2. Cada mês devido tem uma identidade única e estável: a chave composta `(codigo_estudante, codigo_academia, ano_letivo, mês)` — **inclui a academia**, não apenas estudante+ano_letivo+mês. Isto é necessário porque `Estudante.CodigoAcademia` não é fixo: o estudante pode ser desvinculado de uma academia e reintegrado noutra (`DesvincularDaAcademia`/`Reintegrar`, já existentes no aggregate `Estudante`) — nesse caso, pendências geradas enquanto ele estava na academia anterior pertencem a essa academia especificamente, e nunca devem ser confundidas com pendências da academia atual, mesmo que o mês/ano_letivo coincidam. Esta chave também garante que pendências de anos letivos anteriores continuam pendentes e distinguíveis das do ano letivo corrente — nunca reutilizar apenas "mês" isolado, nem apenas "mês+ano_letivo" sem a academia, como chave.
3. Este conjunto reinicia a cada novo ano letivo (o cálculo do item 1 é sempre relativo ao ano letivo corrente do estudante), mas pendências de anos letivos anteriores **não desaparecem** — continuam consultáveis e pendentes até serem pagas (Fase 3) ou anuladas (seção 5).
4. Um mês só passa de "pendente" para "pago" através de um evento de pagamento real (produzido pela Fase 3) — nunca por inferência silenciosa dentro desta fase.
5. Esta fase **não** cria nenhuma cobrança na AppyPay. É puramente contabilística/interna ao Spuri.
6. Este cálculo determina **apenas** o estado (`pendente`/`pago`/`anulado`) de cada mês — a determinação do **valor exato** a cobrar por cada mês pendente (incluindo meses de anos letivos anteriores, com ano académico/curso e preço potencialmente diferentes dos atuais) segue a regra dedicada da seção 6, que deve ser tratada como parte integrante deste cálculo, não como um detalhe posterior.

## Escopo obrigatório

### 3.1 Projeção derivada

Implementar o cálculo dos meses devidos/pagos como uma função/projeção derivada (não um evento em si), combinando: configuração de mensalidade da academia (seção 1/2), configuração de mês de início (seção 4, quando aplicável), eventos de pagamento já registrados (dependência futura da Fase 3 — nesta fase, a lista de "pagos" pode estar vazia por falta de eventos, mas a função deve já estar preparada para os consumir) e eventos de anulação (seção 5).

### 3.2 Endpoint de consulta

Criar uma rota de consulta (ex.: `GET /financeiro/mensalidades/estudante/:codigo`) que devolve, para um estudante, a lista de meses do ano letivo corrente com o respetivo estado (`pendente`/`pago`/`anulado`), acessível pela academia dona do estudante, por **qualquer** admin `fpp` — sem restrição por academia, uma vez que a supervisão financeira da plataforma abrange todas as academias — ou pelo próprio estudante autenticado. Quando o pedido for feito por uma academia, a resposta deve conter **apenas** as pendências cuja chave (ver item 2 da regra de negócio) pertence a essa academia — nunca pendências que o estudante tenha noutra academia (passada ou presente). Quando o pedido for feito pelo próprio estudante ou por um admin `fpp`, a resposta pode (e deve, se o estudante tiver histórico em mais de uma academia) agrupar as pendências por `codigo_academia`, deixando claro a qual instituição cada mês pertence.

### 3.3 Testes obrigatórios

1. estudante recém-matriculado, sem qualquer pagamento: todos os meses do período configurado aparecem como pendentes;
2. estudante com mensalidade paga em meses anteriores (simulado via evento de pagamento, mesmo que a criação real desse evento só exista a partir da Fase 3): esses meses aparecem como pagos, os restantes como pendentes;
3. transição de ano letivo: pendências do ano letivo anterior continuam a existir e distintas das do ano letivo corrente;
4. estudante de academia pública: nenhuma mensalidade é calculada;
5. estudante em ano/curso sem configuração de valor: nenhuma mensalidade é calculada para aquele ano/curso.

---

# 4. Mês de início de cobrança para academias que entram a meio do ano letivo

## Objetivo

Permitir que uma academia que se integra à plataforma já com o ano letivo em curso defina a partir de que mês a cobrança automática deve começar a contabilizar, evitando cobrar meses que já passaram antes da integração.

## Regra de negócio

- Esta configuração é por `(academia, ano_letivo)` — exemplo do pedido original: ano letivo `2026_2027`, a academia entra em janeiro de 2027, já passados setembro→dezembro (escolar) ou outubro→dezembro (superior); a academia define, por exemplo, janeiro como mês de início de cobrança **apenas para esse ano letivo**.
- O mês de início configurado não pode ser anterior ao mês natural de início do período (`09` escolar / `10` superior) — isto seria voltar atrás no tempo, sem sentido de negócio — nem posterior ao `mes_fim_cobranca` configurado para o ano académico em questão.
- Esta configuração vale **apenas** para o ano letivo em que foi definida (o ano letivo de entrada da academia na plataforma). A partir do ano letivo seguinte, prevalece integralmente a regra da seção 3 (mês natural de início do período), sem qualquer resquício desta configuração.
- Se a academia não configurar este mês de início, assume-se o comportamento padrão da seção 3 (mês natural de início do período).

## Escopo obrigatório

### 4.1 Evento e persistência

Evento `MesInicioCobrancaDefinido`, escopado a `(academia, ano_letivo)`, com validação de faixa conforme regra de negócio acima.

### 4.2 Integração com o cálculo da seção 3

O cálculo de meses devidos (seção 3, item 1) deve consultar esta configuração **apenas** quando o `ano_letivo` em questão for exatamente o ano letivo para o qual ela foi definida; para qualquer outro ano letivo (anterior ou posterior), o mês de início natural do período prevalece sem exceção.

### 4.3 Testes obrigatórios

1. academia define mês de início `01` (janeiro) para o ano letivo corrente: meses de setembro/outubro a dezembro não são contabilizados como devidos naquele ano letivo específico;
2. no ano letivo seguinte, a mesma academia não tem mais essa exceção — o cálculo volta a considerar o mês natural de início;
3. tentar configurar um mês de início anterior ao mês natural de início do período → rejeitado;
4. tentar configurar um mês de início posterior ao `mes_fim_cobranca` → rejeitado.

---

# 5. Anulação de cobrança de um estudante específico

## Objetivo

Permitir que a academia isente um estudante específico do dever de pagar um ou mais meses de mensalidade, preservando auditoria completa via event sourcing.

## Regra de negócio

- Evento `ObrigacaoMensalidadeAnulada`, escopado a `(codigo_estudante, codigo_academia, ano_letivo, mês)` (podendo aceitar múltiplos meses num único pedido, desde que cada um gere rastreabilidade individual), com `motivo` opcional. `codigo_academia` é sempre a academia autora da anulação (só pode anular as suas próprias obrigações — ver autorização abaixo), por isso não introduz ambiguidade adicional na prática, mas mantém a chave consistente com a seção 3.
- A partir da gravação deste evento, aquele(s) mês(es) deixa(m) de aparecer como pendente(s) para aquele estudante especificamente — não afeta outros meses do mesmo estudante nem qualquer outro estudante.
- O evento nunca é apagado nem substituído por atualização direta — é permanente no ledger, disponível para auditoria. Uma anulação **pode** ser desfeita, mas apenas através de um novo evento explícito, `ObrigacaoMensalidadeReativada`, escopado à mesma chave `(codigo_estudante, codigo_academia, ano_letivo, mês)` — nunca por remoção/alteração do evento de anulação original. Após a reativação, o mês volta ao estado `pendente` (ou `pago`, se entretanto tiver sido pago por outra via — ver validação abaixo).
- **Autorização: exclusiva da academia dona do estudante.** Nenhum admin `fpp` pode anular nem reativar a obrigação de mensalidade de nenhum estudante, mesmo podendo consultar livremente o estado de qualquer estudante (seção 3.2) — isenção/reativação de mensalidade é uma decisão de negócio que pertence unicamente à academia, não à plataforma. Isto é uma restrição deliberadamente mais estrita do que o padrão geral de acesso irrestrito de admins usado noutros pontos do módulo financeiro (ver também Fase 1, regra de negócio do cancelamento de cobrança, que segue o mesmo princípio de restringir por dono do recurso).

## Escopo obrigatório

### 5.1 Endpoints

Criar rotas dedicadas para anular e para reativar a obrigação de um estudante para um ou mais meses de um ano letivo, acessíveis **apenas** pela academia dona do estudante — nunca por admin `fpp`, mesmo que o restante padrão de autorização do módulo financeiro conceda acesso irrestrito a admins noutros endpoints.

### 5.2 Integração com o cálculo da seção 3

O cálculo de meses devidos deve tratar meses anulados como um terceiro estado (`anulado`), distinto de `pendente` e `pago`, para que a academia sempre saiba a razão de um mês não estar mais em aberto. Ao processar `ObrigacaoMensalidadeReativada`, validar que o mês não foi entretanto pago por outra via (nesse caso, a reativação deve ser rejeitada — não é possível "reativar" um mês já pago, apenas um mês anulado).

### 5.3 Testes obrigatórios

1. anular um mês específico de um estudante: aquele mês passa a `anulado`, os demais meses do mesmo estudante continuam inalterados;
2. anulação não afeta outros estudantes da mesma academia;
3. o evento de anulação continua visível numa consulta de auditoria/histórico do estudante, mesmo depois de o mês deixar de aparecer como pendente;
4. tentar anular um mês fora do período configurado para o ano académico do estudante → rejeitado;
5. reativar um mês anulado: volta a `pendente`, evento `ObrigacaoMensalidadeReativada` visível na auditoria junto do evento de anulação original;
6. tentar reativar um mês que nunca foi anulado, ou que já está `pago` → rejeitado;
7. admin `fpp` tentando anular ou reativar uma obrigação de qualquer estudante → rejeitado, independentemente de ter acesso de consulta (seção 3.2) sobre o mesmo estudante.

---

# 6. Determinação exata do valor de cada mês devido (histórico de vínculo e de preço)

## Objetivo

Garantir que o sistema sabe sempre, com exatidão, quanto cobrar por um mês pendente específico — inclusive para meses de anos letivos anteriores ao corrente, em que o ano académico/curso do estudante e o preço configurado pela academia podem já ter mudado entretanto.

## Regra de negócio

1. **Academia, ano académico e curso históricos:** o valor de um mês pendente `(codigo_estudante, codigo_academia, ano_letivo, mês)` nunca é determinado a partir do vínculo **atual** do estudante (nem a academia atual, nem o ano académico/curso atual) — é determinado a partir de **exatamente onde e como o estudante estava matriculado naquele `ano_letivo` específico**: a academia em questão (que já faz parte da chave — ver seção 3, item 2, sobre transferência entre academias) e, dentro dela, o ano académico (fundamental) ou o curso+ano académico (médio/superior — a mesma granularidade da seção 1: fundamental é só por ano, médio e superior são sempre por curso+ano, nunca só por ano). Isto é necessário porque o estudante progride de ano (e, no médio/superior, pode mudar de curso, ou até de academia) a cada novo ano letivo, e um mês pendente de um ano letivo anterior deve continuar a ser valorizado com base em quem o estudante era **naquele** ano letivo, nessa academia específica — não em quem é hoje. Esta informação já existe no sistema através do histórico de turma por ano letivo (`HistoricoEstudantesAnoLetivo`, na projeção de turmas) — a determinação do valor deve consultar essa fonte (ou equivalente), nunca o estado corrente do `Estudante`.
2. **Preço histórico:** o valor configurado pela academia (seção 1) é versionado por `(academia, nível/ano[, curso])` — cada reconfiguração gera um novo evento `MensalidadeConfigurada`, sem apagar o anterior (já estabelecido). Ao determinar o valor de um mês específico, o sistema deve usar a configuração **dessa mesma academia e dessa mesma combinação nível/ano[/curso]** que estava **em vigor na data de referência daquele mês** (o início do mês civil correspondente àquele `(ano_letivo, mês)`) — nunca a configuração mais recente/atual, nem a de outra academia ou de outro curso/ano. Concretamente: se a academia mudar o valor da mensalidade a meio do ano letivo, essa mudança só se aplica a meses cuja data de referência seja **posterior** à mudança; meses cuja data de referência seja anterior à mudança — mesmo que ainda estejam pendentes de pagamento — continuam a ser cobrados ao preço que estava em vigor quando esse mês passou a ser devido, independentemente de quando o estudante efetivamente vier a pagá-lo.
3. Estas duas regras aplicam-se sempre em conjunto, pela **mesma combinação exata**: o valor final de um mês pendente é sempre "o preço que **aquela academia específica** tinha configurado para **aquele ano académico (fundamental) ou aquele curso+ano académico (médio/superior) específico**, ambos na data de referência daquele mês" — nunca uma combinação de dados atuais com dados históricos, nem de uma academia/curso diferente daquela em que o estudante efetivamente estava.
4. Esta resolução de valor é usada tanto pela consulta de meses pendentes (seção 3.2, para informar o estudante quanto vai pagar antes de confirmar) como pela geração da cobrança em si (Fase 3) — a Fase 3 não deve reimplementar nem repetir esta lógica, apenas reutilizá-la.

## Escopo obrigatório

### 6.1 Resolução do vínculo histórico

Implementar a função que, dado `(codigo_estudante, codigo_academia, ano_letivo)`, devolve o `(nível, ano académico, curso_id?)` — `curso_id` presente apenas para médio/superior, ausente para fundamental — em que o estudante estava matriculado **naquela academia específica**, naquele ano letivo, reaproveitando o histórico já existente de turma/ano letivo em vez de recriar essa informação.

### 6.2 Resolução do preço histórico

Implementar a função que, dado `(codigo_academia, nível/ano[/curso_id], data de referência)`, devolve a configuração de mensalidade (`valor`, `mes_fim_cobranca`) **daquela academia, para exatamente aquela combinação de nível/ano[/curso]**, que estava em vigor naquela data, considerando o histórico completo de `MensalidadeConfigurada` (não apenas a versão mais recente).

### 6.3 Testes obrigatórios

1. estudante progride de ano académico entre dois anos letivos (ex.: `6_ano_fundamental` em 2025_2026, `7_ano_fundamental` em 2026_2027): um mês pendente de 2025_2026 é valorizado com o preço configurado para `6_ano_fundamental`, mesmo que hoje o estudante já esteja em `7_ano_fundamental`;
2. estudante muda de curso entre anos letivos (médio/superior): mesmo princípio do teste anterior, aplicado a `curso_id` — o preço usado é sempre o do curso em que ele estava naquele ano letivo, nunca o do curso atual;
3. estudante é desvinculado de uma academia e reintegrado noutra entre dois anos letivos: um mês pendente da academia anterior é valorizado com a configuração **dessa** academia (nível/ano/curso e preço), nunca com a configuração da academia atual;
4. academia muda o valor da mensalidade a meio do ano letivo: meses com data de referência anterior à mudança continuam a ser cobrados ao preço antigo; meses com data de referência posterior usam o novo preço — mesmo que todos ainda estejam pendentes no momento da consulta/pagamento;
5. estudante paga, já depois da mudança de preço, um mês antigo ainda pendente: o valor cobrado é o preço antigo (o vigente na data de referência do mês), não o preço atual;
6. o valor devolvido pela consulta de meses pendentes (seção 3.2) é idêntico ao valor efetivamente usado na geração da cobrança (Fase 3) para o mesmo mês.

---

# Fora de escopo

- Criação de qualquer cobrança na AppyPay — é responsabilidade exclusiva da Fase 3.
- Notificação ao estudante/encarregado sobre mensalidades pendentes (módulo WhatsApp, ainda em desenho).
- Mensalidade para academias públicas.
- Qualquer alteração aos períodos letivos fixos (`periodoLetivoEscolar`/`periodoLetivoSuperior`) usados por matrícula, faltas, notas e finalização de ano letivo.
- Parcelamento, juros ou multa por atraso — fora do pedido original; se necessário no futuro, deve ser tarefa própria.

# Riscos e mitigações

| Risco | Mitigação |
| --- | --- |
| Alterar valor/mês final a meio do ano letivo afeta retroativamente meses já pagos ou já vencidos | Mudanças de configuração só afetam o cálculo de meses **ainda não vencidos**; nunca recalcular retroativamente o que já foi pago ou já estava vencido antes da mudança |
| Confundir "mês/ano civil" com "ano letivo", ou confundir pendências de academias diferentes, na chave de unicidade | Usar sempre a chave composta `(codigo_estudante, codigo_academia, ano_letivo, mês)`, nunca mês isolado nem mês+ano_letivo sem a academia |
| Configuração de mês final de cobrança ultrapassar o período letivo académico fixo | Validação explícita contra `periodoLetivoEscolar`/`periodoLetivoSuperior` (seção 2.1) |
| Mês de início de cobrança da seção 4 "vazar" para anos letivos seguintes | Escopo estrito por `(academia, ano_letivo)`, testado explicitamente (4.3.2) |
| Cobrar um mês pendente com o ano académico/curso ou o preço errados após o estudante progredir ou a academia mudar o valor | Resolução histórica obrigatória (seção 6), nunca a partir do estado/preço atuais, testada explicitamente (6.3) |
| Admin `fpp` conseguir anular/reativar mensalidade de um estudante por engano ou abuso, aproveitando o acesso de consulta já concedido | Autorização de escrita (anular/reativar) restrita exclusivamente à academia, testada explicitamente (5.3.7), independente do acesso de leitura |

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. a Fase 1 estiver concluída e as funções `roundAmount`/`amountsEqual` forem reutilizadas aqui;
2. só academias privadas conseguirem configurar e gerar mensalidade;
3. a configuração de valor respeitar a granularidade definida (fundamental por ano; médio/superior por curso+ano);
4. `mes_fim_cobranca` aceitar apenas `6` ou `7` e nunca exceder o período letivo fixo;
5. o cálculo de meses devidos/pagos funcionar sob procura, sem depender de nenhum processo agendado por tempo;
6. a chave `(codigo_estudante, codigo_academia, ano_letivo, mês)` for usada de forma consistente e testada entre transições de ano letivo e entre academias (transferência de estudante);
7. o mês de início de cobrança para entrada a meio de ano letivo funcionar apenas no ano letivo em que foi definido;
8. a anulação individual de mensalidade funcionar com auditoria completa, sem apagar o histórico, com reativação disponível via `ObrigacaoMensalidadeReativada`, e ambas as ações restritas exclusivamente à academia (nunca a admin `fpp`);
9. a consulta de mensalidades (seção 3.2) funcionar sem restrição de academia para qualquer admin `fpp`;
10. o valor de qualquer mês pendente for sempre resolvido pelo ano académico/curso e pelo preço em vigor na data de referência daquele mês (seção 6), nunca pelo estado ou preço atuais;
11. todos os testes das seções 1 a 6 passarem;
12. `Documentação.md` estiver atualizada com os novos conceitos, eventos e endpoints.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Módulo de pagamentos — Fase 2 — Cobrança de mensalidade/propina automatizada (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
