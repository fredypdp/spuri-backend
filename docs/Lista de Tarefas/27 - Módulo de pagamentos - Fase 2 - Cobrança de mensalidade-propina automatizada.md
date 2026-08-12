---
criado: 2026-08-12 00:00
origem: pedido direto do Spuri (evolução do módulo financeiro após a tarefa 17)
status: pendente
depende_de: "26 - Módulo de pagamentos - Fase 1 - Bases (tipo do amount e cancelamento de cobrança).md"
---

# Módulo de pagamentos — Fase 2 — Cobrança de mensalidade/propina automatizada (pendente)

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
| Identidade do mês devido | Chave única `(estudante, ano_letivo, mês)` | Reinicia a cada ano letivo; pendências de anos anteriores continuam pendentes e distinguíveis |
| Entrada a meio do ano letivo | Mês de início de cobrança configurável, válido apenas no ano letivo de entrada | A partir do ano letivo seguinte, prevalece a regra normal do período completo |
| Isenção individual | Evento auditável por (estudante, ano_letivo, mês) | Nunca apaga o histórico; apenas remove a pendência dali em diante |

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
2. Cada mês devido tem uma identidade única e estável: a chave composta `(codigo_estudante, ano_letivo, mês)`. Esta chave é o que garante que pendências de anos letivos anteriores continuam pendentes e distinguíveis de pendências do ano letivo corrente — nunca reutilizar apenas "mês" isolado como chave, porque isso colidiria entre anos letivos diferentes.
3. Este conjunto reinicia a cada novo ano letivo (o cálculo do item 1 é sempre relativo ao ano letivo corrente do estudante), mas pendências de anos letivos anteriores **não desaparecem** — continuam consultáveis e pendentes até serem pagas (Fase 3) ou anuladas (seção 5).
4. Um mês só passa de "pendente" para "pago" através de um evento de pagamento real (produzido pela Fase 3) — nunca por inferência silenciosa dentro desta fase.
5. Esta fase **não** cria nenhuma cobrança na AppyPay. É puramente contabilística/interna ao Spuri.

## Escopo obrigatório

### 3.1 Projeção derivada

Implementar o cálculo dos meses devidos/pagos como uma função/projeção derivada (não um evento em si), combinando: configuração de mensalidade da academia (seção 1/2), configuração de mês de início (seção 4, quando aplicável), eventos de pagamento já registrados (dependência futura da Fase 3 — nesta fase, a lista de "pagos" pode estar vazia por falta de eventos, mas a função deve já estar preparada para os consumir) e eventos de anulação (seção 5).

### 3.2 Endpoint de consulta

Criar uma rota de consulta (ex.: `GET /financeiro/mensalidades/estudante/:codigo`) que devolve, para um estudante, a lista de meses do ano letivo corrente com o respetivo estado (`pendente`/`pago`/`anulado`), acessível pela academia dona do estudante, por um admin `fpp`, ou pelo próprio estudante autenticado.

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

- Evento `ObrigacaoMensalidadeAnulada`, escopado a `(codigo_estudante, ano_letivo, mês)` (podendo aceitar múltiplos meses num único pedido, desde que cada um gere rastreabilidade individual), com `motivo` opcional.
- A partir da gravação deste evento, aquele(s) mês(es) deixa(m) de aparecer como pendente(s) para aquele estudante especificamente — não afeta outros meses do mesmo estudante nem qualquer outro estudante.
- O evento nunca é apagado nem substituído por atualização direta — é permanente no ledger, disponível para auditoria, mesmo que uma anulação futura seja "desfeita" por um novo evento explícito (ex.: `ObrigacaoMensalidadeReativada`, se a equipa decidir que este caso é necessário; caso contrário, documentar explicitamente que uma anulação é irreversível nesta versão).
- Autorização: a academia dona do estudante, ou admin `fpp`.

## Escopo obrigatório

### 5.1 Endpoint

Criar rota dedicada para anular a obrigação de um estudante para um ou mais meses de um ano letivo, seguindo o padrão de autorização de rotas administrativas de academia sobre estudante já existente no sistema.

### 5.2 Integração com o cálculo da seção 3

O cálculo de meses devidos deve tratar meses anulados como um terceiro estado (`anulado`), distinto de `pendente` e `pago`, para que a academia sempre saiba a razão de um mês não estar mais em aberto.

### 5.3 Testes obrigatórios

1. anular um mês específico de um estudante: aquele mês passa a `anulado`, os demais meses do mesmo estudante continuam inalterados;
2. anulação não afeta outros estudantes da mesma academia;
3. o evento de anulação continua visível numa consulta de auditoria/histórico do estudante, mesmo depois de o mês deixar de aparecer como pendente;
4. tentar anular um mês fora do período configurado para o ano académico do estudante → rejeitado.

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
| Confundir "mês/ano civil" com "ano letivo" na chave de unicidade | Usar sempre a chave composta `(codigo_estudante, ano_letivo, mês)`, nunca mês isolado |
| Configuração de mês final de cobrança ultrapassar o período letivo académico fixo | Validação explícita contra `periodoLetivoEscolar`/`periodoLetivoSuperior` (seção 2.1) |
| Mês de início de cobrança da seção 4 "vazar" para anos letivos seguintes | Escopo estrito por `(academia, ano_letivo)`, testado explicitamente (4.3.2) |

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. a Fase 1 estiver concluída e as funções `roundAmount`/`amountsEqual` forem reutilizadas aqui;
2. só academias privadas conseguirem configurar e gerar mensalidade;
3. a configuração de valor respeitar a granularidade definida (fundamental por ano; médio/superior por curso+ano);
4. `mes_fim_cobranca` aceitar apenas `6` ou `7` e nunca exceder o período letivo fixo;
5. o cálculo de meses devidos/pagos funcionar sob procura, sem depender de nenhum processo agendado por tempo;
6. a chave `(codigo_estudante, ano_letivo, mês)` for usada de forma consistente e testada entre transições de ano letivo;
7. o mês de início de cobrança para entrada a meio de ano letivo funcionar apenas no ano letivo em que foi definido;
8. a anulação individual de mensalidade funcionar com auditoria completa e sem apagar o histórico;
9. todos os testes das seções 1 a 5 passarem;
10. `Documentação.md` estiver atualizada com os novos conceitos, eventos e endpoints.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Módulo de pagamentos — Fase 2 — Cobrança de mensalidade/propina automatizada (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
