---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: feito
---

# Validar e reforçar a integridade do event sourcing e dos rebuilds (feito)

## Prompt recomendado para executar a atualização

Execute uma auditoria técnica completa da cadeia de hashes do ledger, da whitelist de eventos, do processo de rebuild de projeções e das permissões do PostgreSQL usadas pela aplicação, comprovando com testes automatizados que o sistema realmente **impede** a adulteração de dados — não apenas a detecta depois do fato. Sempre que a auditoria mostrar que a única proteção hoje é a verificação de hash chain sob demanda (`GET /verificar-integridade/:codigo`), implemente reforço em nível de banco de dados (revogação de permissões e/ou triggers) para transformar a tabela do ledger em append-only de verdade. Corrija qualquer lacuna encontrada nos rebuilds (idempotência, abortagem em ledger corrompido, concorrência) e documente os resultados da auditoria.

## Contexto

`Documentação.md` descreve três mecanismos de integridade já implementados: a cadeia de hashes do ledger (`hash(evento_N) = SHA256(conteúdo_N + hash(evento_N-1))`), a whitelist de eventos autorizados (`safe_queries.go`) e o uso de prepared statements em todas as queries SQL. Também descreve que, antes de qualquer rebuild, "o sistema verifica a integridade completa do ledger" e aborta se qualquer aggregate estiver com hash chain inválida, e que existe um lock global limitando a **1 rebuild por vez**.

Nenhuma dessas garantias foi comprovada nesta lista de tarefas por teste automatizado ponta a ponta. Além disso, existe uma diferença conceitual importante entre **detectar** adulteração e **impedir** adulteração:

- a cadeia de hashes é um mecanismo de **tamper-evidence** (detecção): ela permite descobrir, quando alguém verifica, que um evento foi alterado — mas só depois do fato, e só se alguém chamar a verificação;
- ela não impede, por si só, que uma conexão com acesso de escrita ao banco (seja por comprometimento da aplicação, seja por acesso direto via `psql` ou ferramenta administrativa) execute um `UPDATE` ou `DELETE` diretamente na tabela do ledger.

A tarefa original em `Lista de tarefas.md` pede explicitamente para testar "se o sistema (tanto o código GO, quanto o banco de dados) realmente impede a adulteração dos dados". Isso exige investigar também as permissões da role PostgreSQL usada pela aplicação em runtime, não apenas o comportamento do código Go.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Cadeia de hashes | Testar detecção de adulteração ponta a ponta | Confirmar que qualquer alteração manual em um evento já persistido é detectada por `verify_hash_chain`/`GET /verificar-integridade/:codigo`, com o ponto exato da quebra identificado |
| Prevenção em nível de banco | Auditar e revisar permissões da role de runtime sobre a tabela do ledger | Impedir `UPDATE`/`DELETE` diretos no ledger pela role usada pela aplicação, não apenas detectar a alteração depois |
| Whitelist de eventos | Testar rejeição de eventos não autorizados | Confirmar que `safe_queries.go` bloqueia `event_type` desconhecido antes de qualquer escrita no banco |
| Rebuild de projeções | Testar idempotência e abortagem em ledger corrompido | Rebuild reproduz exatamente o mesmo estado ao ser repetido e aborta sem escrever dado novo quando a integridade falha |
| Concorrência de rebuild | Testar o lock global | Confirmar `409 Conflict` quando um segundo rebuild é solicitado enquanto o primeiro está em execução |
| Queries dinâmicas | Auditar interpolação de nomes de tabela/coluna | Confirmar que nenhuma interpolação usa valor vindo diretamente do request fora de um switch fechado com constantes |

---

# 1. Testar e comprovar a detecção de adulteração na cadeia de hashes

## Objetivo

Confirmar que qualquer alteração manual de um evento já gravado no ledger é detectável pela verificação de integridade existente, e que o ponto exato da quebra é identificado corretamente.

## Regra de negócio

`hash(evento_N) = SHA256(conteúdo_N + hash(evento_N-1))`. Qualquer alteração em `conteúdo_N` deve invalidar `hash(evento_N)` e, por consequência, todos os hashes seguintes na mesma cadeia de aggregate. `GET /verificar-integridade/:codigo` deve retornar `integro=false` sempre que essa condição ocorrer, sem exceção.

## Escopo obrigatório

### 1.1 Cenários de adulteração a testar

Em ambiente de teste isolado (nunca em produção), simular, para pelo menos um estudante com múltiplos eventos no ledger:

1. alterar o `payload` de um evento já persistido, mantendo o `ledger_hash` antigo daquele evento;
2. alterar o `payload` de um evento e recalcular apenas o hash daquele evento isoladamente, sem propagar a mudança para os hashes dos eventos seguintes;
3. remover fisicamente um evento do meio da cadeia de um aggregate;
4. inserir um evento fora de ordem/sequência (`event_version` fora da sequência esperada);
5. alterar apenas `metadata` (ex.: IP, usuário executante) sem alterar `payload` nem `ledger_hash`.

### 1.2 Resultado esperado por cenário

Para cada cenário do item 1.1, o teste deve confirmar:

- `GET /verificar-integridade/:codigo` retorna `integro=false`;
- a mensagem/versão retornada aponta exatamente o `event_version` a partir do qual a cadeia deixou de bater;
- eventos anteriores ao ponto de quebra continuam sendo relatados como íntegros;
- o cenário 5 (alteração apenas de `metadata` sem invalidar o hash) é testado deliberadamente para confirmar se `metadata` participa ou não do cálculo do hash; se não participar, documentar essa limitação de forma explícita, pois significa que `metadata` (usuário executante, IP) não é protegida pela cadeia de hashes.

### 1.3 Teste de rebuild após adulteração confirmada

Depois de confirmar `integro=false` num aggregate, disparar `POST /dominis/projections/rebuild/:name` (ou a versão `/async`) e confirmar que o rebuild é abortado com erro claro antes de escrever qualquer linha nova na projeção, conforme já documentado em `Documentação.md` (seção 17.2).

---

# 2. Auditar e reforçar prevenção em nível de banco de dados

## Objetivo

Determinar se o banco de dados hoje permite `UPDATE`/`DELETE` na tabela do ledger através da conexão/role usada pela aplicação em runtime, e eliminar essa possibilidade sempre que for encontrada.

## Regra de negócio

"Impedir a adulteração dos dados" implica prevenção, não apenas detecção. A cadeia de hashes é insuficiente sozinha porque só é verificada sob demanda (`GET /verificar-integridade/:codigo`) ou antes de um rebuild; ela não impede fisicamente uma escrita indevida na tabela do ledger enquanto ninguém chama essa verificação.

## Escopo obrigatório

### 2.1 Auditoria de permissões

1. identificar a role/usuário PostgreSQL usada pela conexão de runtime da aplicação (`*sql.DB`/`sqlx.DB`/`pgxpool.Pool`, conforme o driver em uso);
2. listar os `GRANT`s atuais dessa role sobre a tabela do ledger (`spuri_ledger` ou nome equivalente usado no schema atual);
3. confirmar explicitamente, com uma query real, se `UPDATE` e `DELETE` estão disponíveis para essa role sobre essa tabela.

### 2.2 Reforço de prevenção

Adotar pelo menos uma das estratégias abaixo, documentando a escolha e o motivo no PR:

1. revogar `UPDATE` e `DELETE` da role de runtime sobre a tabela do ledger via migration, mantendo apenas `INSERT` e `SELECT`; ou
2. criar um trigger `BEFORE UPDATE OR DELETE` na tabela do ledger que sempre gera exceção (`RAISE EXCEPTION`), preservando a tabela como append-only mesmo que a role tenha permissão ampla por outro motivo.

Se o projeto usar uma única role de banco tanto para runtime quanto para migrations administrativas, avaliar e documentar explicitamente a viabilidade de separar essas responsabilidades; se não for viável nesta tarefa, o trigger da opção 2 deve ser a estratégia adotada, pois independe de qual role está conectada.

### 2.3 Testes obrigatórios

1. tentar `UPDATE` direto na tabela do ledger usando a role/conexão de runtime em ambiente de teste e confirmar rejeição pelo banco;
2. tentar `DELETE` direto e confirmar rejeição pelo banco;
3. confirmar que `INSERT` (fluxo normal de gravação de eventos) continua funcionando sem nenhuma regressão após a mudança;
4. confirmar que migrations legítimas do schema (criação de índices, novas colunas, etc.) continuam possíveis pelo caminho administrativo usado pelo projeto.

---

# 3. Testar rejeição de eventos fora da whitelist

## Objetivo

Confirmar que a whitelist de eventos (`safe_queries.go`) bloqueia qualquer `event_type` não autorizado antes que ele chegue ao banco de dados.

## Regra de negócio

Apenas eventos previamente autorizados podem ser gravados no ledger. Qualquer evento desconhecido deve ser rejeitado no código Go, antes de qualquer tentativa de escrita.

## Escopo obrigatório

1. localizar todos os pontos de gravação no ledger e confirmar que todos passam pela mesma validação de whitelist, sem caminho alternativo que a ignore;
2. testar o envio de um `event_type` inexistente e confirmar rejeição com erro de negócio claro, sem nenhuma escrita parcial no banco;
3. testar variações de burla: nome com diferença de maiúsculas/minúsculas, nome com espaço extra, nome de evento existente mas com prefixo/sufixo adicional;
4. confirmar, por teste, que a rejeição acontece antes de abrir transação de escrita (ou que a transação é revertida por completo em caso de rejeição), sem deixar rastro parcial.

---

# 4. Testar idempotência e corretude dos rebuilds

## Objetivo

Comprovar que reconstruir uma projeção a partir do ledger sempre produz o mesmo resultado, e que o processo é seguro contra ledger corrompido e contra concorrência.

## Regra de negócio

Um rebuild deve ser uma função pura do ledger: dado o mesmo conjunto de eventos, o estado final da projeção deve ser sempre o mesmo, independentemente de quantas vezes o rebuild for executado.

## Escopo obrigatório

### 4.1 Idempotência

1. capturar o estado atual de cada projeção citada na ordem de rebuild documentada (`admins`, `academias`, `cursos`, `materias`, `categorias_nota`, `estudantes`, `turmas`, `notas`, `faltas`, `avaliacao_final`, `solicitacoes_matricula`);
2. disparar o rebuild de cada uma;
3. comparar o estado resultante campo a campo com o estado capturado antes do rebuild, para uma amostra representativa de registros de cada projeção;
4. repetir o rebuild da mesma projeção duas vezes seguidas e confirmar resultado idêntico entre as duas execuções.

### 4.2 Abortagem em ledger corrompido

1. em ambiente de teste isolado, corromper deliberadamente um evento (reaproveitando os cenários da seção 1.1);
2. disparar rebuild da projeção correspondente e confirmar que ele é abortado **antes** de escrever qualquer dado novo;
3. confirmar que a projeção permanece exatamente no estado anterior à tentativa de rebuild (não fica parcialmente reconstruída).

### 4.3 Concorrência

1. disparar dois rebuilds simultâneos (síncrono + síncrono, e também síncrono + assíncrono) e confirmar que o segundo recebe `409 Conflict`;
2. confirmar que o lock global é liberado corretamente após a conclusão (com sucesso ou com erro) do primeiro rebuild, permitindo uma nova tentativa em seguida;
3. testar o cenário de falha no meio do processamento assíncrono e confirmar que o lock não fica preso indefinidamente.

---

# 5. Auditar uso de prepared statements e interpolação dinâmica de identificadores

## Objetivo

Confirmar que nenhuma query SQL do sistema concatena dado vindo do usuário diretamente na string SQL, e que qualquer interpolação dinâmica de nome de tabela/coluna está restrita a um switch fechado com valores constantes.

## Regra de negócio

Todas as queries SQL devem usar prepared statements com placeholders (`$1`, `$2`, ...). Nomes de tabela interpolados dinamicamente só podem vir de um switch/mapa fechado no código, nunca diretamente do request.

## Escopo obrigatório

1. buscar amplamente no código por concatenação de string SQL com variáveis (`fmt.Sprintf` usado para montar SQL, `+` de strings dentro de queries, etc.);
2. para cada ocorrência de nome de tabela/projeção interpolado dinamicamente (ex.: no endpoint de rebuild `POST /dominis/projections/rebuild/:name`), confirmar que o valor é resolvido contra um switch/mapa fechado de nomes conhecidos, nunca usado cru;
3. adicionar teste de regressão enviando um valor malicioso como parâmetro de rota (ex.: `:name` contendo `; DROP TABLE` ou caminho de diretório) e confirmar erro `404`/`400` controlado, nunca um erro de SQL vazando para o cliente;
4. classificar cada ocorrência encontrada como: uso seguro confirmado, uso a corrigir, ou falso positivo justificado — sem deixar ocorrência sem classificação na entrega.

---

# Fora de escopo

- Migrar o event store para outra tecnologia de armazenamento.
- Alterar o algoritmo de hash (`SHA256` permanece) ou o formato da cadeia.
- Criar mecanismo de assinatura digital externo ou ancoragem em blockchain externo.
- Alterar regras de negócio não relacionadas à integridade do ledger, whitelist de eventos ou rebuilds.
- Criar nova UI/dashboard de auditoria; o escopo é backend e banco de dados.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. todos os cenários de adulteração da seção 1.1 forem cobertos por teste automatizado e confirmarem `integro=false` com o ponto de quebra correto;
2. a auditoria de permissões da seção 2.1 estiver documentada, com a role de runtime identificada e seus `GRANT`s reais listados;
3. `UPDATE` e `DELETE` diretos na tabela do ledger estiverem bloqueados pelo próprio banco de dados para a role/conexão de runtime, comprovado por teste;
4. o fluxo normal de `INSERT` de eventos continuar funcionando sem regressão após o reforço de permissões;
5. o envio de um evento fora da whitelist for rejeitado antes de qualquer escrita no banco, comprovado por teste;
6. rebuilds forem comprovadamente idempotentes para todas as projeções citadas na ordem documentada;
7. rebuild abortar sem escrita parcial quando o ledger estiver corrompido, comprovado por teste;
8. o lock global de rebuild for comprovado por teste de concorrência, incluindo liberação correta após falha;
9. a auditoria de interpolação dinâmica de SQL estiver documentada, com cada ocorrência classificada;
10. um relatório de auditoria (pode ser o próprio corpo do PR) resumir os achados, os riscos encontrados e as correções aplicadas.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Validar e reforçar a integridade do event sourcing e dos rebuilds (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.


## Relatório de auditoria executada

- **Role de runtime**: a aplicação conecta usando `DATABASE_URL` quando presente; caso contrário, usa `DB_USER`, `DB_PASSWORD`, `DB_HOST`, `DB_PORT` e `DB_NAME` definidos em `internal/db/client.go`. Como o mesmo cliente também executa migrations neste projeto, não há separação explícita entre role administrativa e role de runtime no código atual.
- **GRANTs e prevenção**: por não haver separação garantida de roles, a correção adotou triggers `BEFORE UPDATE`, `BEFORE DELETE` e `BEFORE TRUNCATE` em `spuri_ledger`. Essa escolha impede adulteração DML no próprio banco mesmo quando a role conectada mantém privilégios amplos. Superusers ou donos com permissão para desabilitar triggers continuam fora do modelo de ameaça coberto por esta aplicação.
- **Cadeia de hashes**: `verify_hash_chain` foi reforçada para validar versões contíguas, primeiro evento com `previous_hash NULL`, ponteiro `previous_hash` para o evento imediatamente anterior e recálculo do hash armazenado.
- **Limitação documentada**: `metadata` não participa do cálculo histórico de `ledger_hash`; alterações em metadata não são detectadas pela cadeia de hashes. O payload, o tipo do evento, o aggregate, o event id e o previous hash continuam protegidos.
- **Whitelist de eventos**: todos os caminhos de escrita do ledger passam por `AppendTx` ou pelo append interno do pacote `db`, ambos chamando `ValidateAggregateType` e `ValidateEventType` antes do `INSERT`. Testes cobrem variações de burla por evento desconhecido, caixa, espaço e prefixo/sufixo.
- **Rebuilds**: o manager mantém lock global de rebuild, valida a integridade completa do ledger antes de reconstruções e só marca o rebuild como concluído após `projection.Rebuild()` retornar sucesso. Falhas resetam o marcador `is_rebuilding`, e o lock em memória é liberado via `defer`.
- **SQL dinâmico**: a auditoria encontrou usos de `fmt.Sprintf` para montar cláusulas com placeholders ou colunas escolhidas em mapas/switches internos. O parâmetro de rebuild é resolvido contra o mapa fechado de projeções registradas no manager; nomes não registrados retornam erro controlado antes de qualquer SQL dinâmico.
