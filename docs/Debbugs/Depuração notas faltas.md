# Relatório de Depuração — Módulos de Notas e Faltas (spuri-backend)

**Repositório analisado:** `fredypdp/spuri-backend` (branch `main`, snapshot de 06/08/2026)
**Escopo:** `internal/db/`, `internal/domain/aggregates/` (foco em `estudante_notas.go`, `estudante_falta.go`, `academia_categorias_nota.go`, `aggregate.go`), `internal/domain/models.go`, `internal/handlers/` (foco em `notas_handlers.go`, `faltas_handlers.go`, `batch_handlers.go`, `async_batch_handlers.go`), `internal/projections/` (foco em `notas_projection.go`, `faltas_projection.go`, `categorias_nota_projection.go`), `internal/middleware/auth.go`, migrations relacionadas a notas/faltas (006, 037, 038, 042, 052, 053×2, 056, 057×2, 088, 096) e o event store/repositório de agregados.
**Metodologia:** leitura integral dos arquivos acima + rastreamento cruzado entre aggregate → evento → projeção → constraint SQL → rota HTTP, para cada fluxo de gravação. Não foi executado o servidor (sem banco disponível neste ambiente); as observações são de leitura estática de código, mas cada achado crítico foi confirmado cruzando pelo menos 2 camadas (ex.: código Go + migration SQL correspondente).

**Como usar este documento (orientação para o Codex):** cada achado tem um ID (`SEC-xx`, `PROD-xx`, `UNIQ-xx`, `TRACE-xx`), severidade, arquivo(s) exato(s), evidência, impacto e uma correção proposta. Trate a seção 6 ("Plano de execução") como a ordem de trabalho. Depois de cada correção, rode os testes indicados na seção 5 antes de passar para o próximo item. Não altere nada fora do escopo listado sem necessidade comprovada.

---

## Sumário executivo

| Categoria | Crítico | Alto | Médio | Baixo |
|---|---|---|---|---|
| Segurança (SEC) | 1 | 0 | 1 | 1 |
| Bloqueadores de produção (PROD) | 1 | 1 | 2 | 3 |
| Unicidade (UNIQ) | 0 | 0 | 1 | 2 |
| Rastreabilidade (TRACE) | 0 | 0 | 3 | 1 |

Os dois achados **críticos** são:

1. **`SEC-01`** — o segredo JWT tem um *fallback* fixo e público que só é bloqueado se a variável `ENV` for **exatamente** a string `"production"`; qualquer desvio (não configurada, digitada diferente, ambiente com outro nome) sobe o servidor silenciosamente usando um segredo conhecido publicamente neste repositório, permitindo forjar tokens de admin/academia e adulterar notas/faltas de qualquer estudante.
2. **`PROD-01`** — notas e faltas são **estritamente append-only**: não existe nenhuma rota de edição, anulação ou correção. Combinado com o bloqueio de duplicata por chave de negócio no aggregate, um erro de digitação (ex.: professor lança 15 em vez de 5) **trava permanentemente** aquela combinação (estudante+academia+ano letivo+período+matéria+tipo+categoria) — a nota errada nunca poderá ser substituída pela correta.

---

## 1. Vulnerabilidades de segurança (SEC)

### SEC-01 — CRÍTICO — Segredo JWT com fallback inseguro e verificação de ambiente frágil

**Arquivo:** `internal/middleware/auth.go`, linhas 28–41.

```go
func init() {
    secret := os.Getenv("JWT_SECRET")
    env := os.Getenv("ENV")

    if secret == "" {
        if env == "production" {
            log.Fatalf("[FATAL] JWT_SECRET não configurado em produção...")
        }
        secret = "seu_segredo_muito_secreto_aqui_mude_em_producao"
        log.Printf("⚠️  [JWT] Usando secret padrão — NÃO USE EM PRODUÇÃO...")
    }
    jwtSecret = []byte(secret)
}
```

**Problema:** a única proteção contra subir em produção com o segredo padrão é a comparação exata `env == "production"`. Esse padrão se repete em `cmd/server/main.go` (3 ocorrências) e `internal/storage/storage.go`. Se a variável `ENV` não for definida, tiver espaço, letra maiúscula, "Production", "PRODUCAO", ou simplesmente não existir no painel do provedor de hosting (erro operacional comum), o servidor **não falha** — ele sobe normalmente e assina/valida tokens JWT com o segredo `"seu_segredo_muito_secreto_aqui_mude_em_producao"`, que está **publicado neste repositório público**.

**Impacto:** qualquer pessoa pode gerar localmente um JWT válido com HS256 usando esse segredo, com `user_type: "admin"` e um UUID arbitrário, e:
- Ler notas e faltas de qualquer estudante de qualquer academia (`GET /academias`, `GET /notas`, `GET /faltas`, `GET /notas-estudante/:codigo`, `GET /faltas-estudante/:codigo`);
- Se também conseguir um `user_id` de academia válido (mais fácil de obter, ex. por engenharia social ou enumeração), **lançar/forjar notas e faltas** via `POST /academia/notas-aluno` e `POST /academia/faltas-aluno`;
- Disparar rebuilds de projeção (`POST /admin/projections/rebuild/:name`), afetando a disponibilidade de todo o sistema de notas/faltas.

Este é o achado de maior severidade do relatório porque não depende de nenhum bug de lógica de negócio — depende apenas de uma variável de ambiente mal configurada, o que é um erro operacional plausível especialmente durante o primeiro deploy.

**Correção recomendada:**
1. Trocar a lógica para **fail-safe por padrão**: exigir `JWT_SECRET` sempre, independente de `ENV`, e permitir o fallback fraco **apenas** quando `ENV` for explicitamente `"development"` ou `"test"` (allow-list), nunca "qualquer coisa diferente de production" (deny-list invertida). Ou seja, inverter a condição de `if env == "production" { fatal }` para `if env != "development" && env != "test" { fatal }`.
2. Normalizar a leitura de `ENV` com `strings.ToLower(strings.TrimSpace(...))` em todos os pontos onde é comparado (`auth.go`, `main.go` ×3, `storage.go`) para eliminar sensibilidade a caixa/espacos.
3. Remover o valor literal do segredo de fallback do código-fonte (usar algo claramente inválido/curto que quebre a assinatura, ou gerar um segredo aleatório em memória a cada boot em dev — isso também evita tokens de dev acidentalmente válidos entre reinicializações).
4. Adicionar um teste de integração que sobe a aplicação com `ENV` ausente/variado e `JWT_SECRET` vazio e garante que o processo termina com erro (exceto quando `ENV=development` ou `ENV=test`).

---

### SEC-02 — MÉDIO — Logs de depuração não têm controle de nível e expõem dados sensíveis de notas/faltas em produção

**Arquivos:** `internal/handlers/notas_handlers.go` (linhas 44–47, 71–74, 108–111, 141–144), `internal/domain/aggregates/aggregate.go` (`RaiseEvent`, `ClearUncommittedEvents`), e de forma geral qualquer `log.Printf("[DEBUG] ...")` no pacote.

**Problema:** o `.env.example` documenta `LOG_LEVEL=info`, mas essa variável **nunca é lida** em nenhum lugar do código (`grep` não encontra `LOG_LEVEL` fora do `.env.example`). Todos os `log.Printf("[DEBUG] ...")` — incluindo os que imprimem `codigo_estudante`, `nota`, `periodo`, `categoria` a cada requisição de `RegistrarNota` — rodam **sempre**, em qualquer ambiente, sem possibilidade de desativação em produção via configuração.

```go
log.Printf(
    "[nota-debug] payload recebido: estudante=%s materia_id=%s tipo=%s categoria=%s periodo=%q nota=%.2f",
    req.CodigoEstudante, req.MateriaDisciplinarID, req.Tipo, req.Categoria, req.Periodo, req.Nota,
)
```

**Impacto:** dados educacionais (nota, presença/falta, identificador do estudante) — informação sensível sob qualquer política razoável de proteção de dados educacionais — vão parar em texto puro nos logs de stdout, que em plataformas de hospedagem gerenciadas costumam ser retidos por mais tempo, exportados a ferramentas terceiras de observabilidade, e têm controle de acesso mais frouxo que o banco de dados. Além disso, o volume de logging (múltiplos `Printf` por evento, incluindo dentro do hot path de `RaiseEvent`) é custo de infraestrutura desnecessário em produção.

**Correção recomendada:**
1. Implementar um logger estruturado mínimo (ex. wrapper simples em `internal/utils` ou usar `log/slog` da stdlib) que respeite `LOG_LEVEL` do ambiente (`debug`/`info`/`warn`/`error`).
2. Trocar todos os `log.Printf("[DEBUG] ...")` por `logger.Debug(...)`, que só imprime quando `LOG_LEVEL=debug`.
3. Nos logs que permanecerem ativos por padrão (info/warn), remover o valor da nota e da observação do texto — manter `event_id`, `codigo_estudante` (ou um hash, se a política de dados exigir) e `codigo_academia`, mas não o conteúdo da nota, para permitir troubleshooting sem exposição direta do dado acadêmico completo em log.

---

### SEC-03 — BAIXO — Endpoints de notas/faltas aceitam campos desconhecidos no JSON (não usam decodificação estrita)

**Arquivos:** `internal/handlers/notas_handlers.go` (`RegistrarNota`, uso de `c.ShouldBindJSON`), `internal/handlers/faltas_handlers.go` (`RegistrarFaltas`, idem).

**Problema:** o próprio repositório já tem um padrão mais seguro implementado (`decodeStrictJSON`, em `internal/handlers/materia_disciplinar_handlers.go:522`) que usa `json.Decoder.DisallowUnknownFields()` para rejeitar campos não esperados no corpo da requisição. `RegistrarNota` e `RegistrarFaltas` **não usam esse padrão** — usam `c.ShouldBindJSON`, que ignora silenciosamente campos desconhecidos.

**Impacto:** não é uma falha de autorização, mas é uma superfície de bugs silenciosos: um cliente com um campo digitado errado (ex. `"notaa"` em vez de `"nota"`, ou tentando reaproveitar por engano o contrato legado de `Materias []Materia` de `models.go` — ver `PROD-07`) recebe uma resposta `201 Created` "de sucesso" mesmo que o dado pretendido nunca tenha sido de fato gravado, porque o campo extra foi ignorado e o campo esperado ficou com o zero-value. Isso é diretamente relevante ao pedido do usuário sobre "gravação dos dados esteja funcionando": há hoje uma forma de o cliente *achar* que gravou algo que não gravou, sem qualquer erro.

**Correção recomendada:** adotar `decodeStrictJSON` (ou equivalente) nos `struct` de request de `RegistrarNota` e `RegistrarFaltas`, igual já é feito em `materia_disciplinar_handlers.go`.

---

## 2. Bloqueadores de produção — gravação de dados (PROD)

### PROD-01 — CRÍTICO — Notas e faltas são estritamente append-only: nenhuma via de correção existe

**Arquivos:**
- `internal/handlers/notas_handlers.go` — não existe `EditarNota`/`DeletarNota`.
- `internal/handlers/faltas_handlers.go` — não existe `EditarFalta`/`DeletarFalta`.
- `internal/domain/aggregates/estudante_notas.go`, linhas 158–166 (guarda de duplicata por chave de negócio):

```go
chave := chaveNota(codigoAcademia, anoLectivo, periodo, materiaDisciplinarID, tipo, categoria)
if e.NotasRegistradasPorChave != nil && e.NotasRegistradasPorChave[chave] {
    return fmt.Errorf(
        "nota já registrada para periodo '%s', materia '%s', tipo '%s', categoria '%s' no ano letivo '%s'",
        periodo, materiaDisciplinarID, tipo, categoria, anoLectivo,
    )
}
```
- Mesmo padrão em `internal/domain/aggregates/estudante_falta.go`, linhas 65–71 (`chaveFalta`).
- Migration confirmando que isso é intencional, mas incompleto: `migrations/088_remover_soft_delete_notas_faltas.sql`:

```sql
-- Notas e faltas são recursos somente de criação e leitura. Não há mais fluxo
-- público, administrativo, batch ou assíncrono para editar, excluir, restaurar
-- ou ocultar registros por soft delete.
```

**Problema:** o desenho atual é "ledger imutável" (correto do ponto de vista de auditoria), mas **sem nenhum mecanismo de correção substituto**. A migration 038 originalmente adicionava suporte a soft delete com `deletado_por`/`motivo_exclusao` para notas e faltas — e a migration 088 **removeu esse suporte por completo**, sem introduzir nenhum evento compensatório (`NotaCorrigida`, `NotaAnulada`, etc.) no lugar. O resultado prático:

- Um professor lança nota 15 em vez de 5 para "Matemática, 1º trimestre, categoria nota_professor". A chamada tem sucesso (`201 Created`).
- Ele percebe o erro e tenta corrigir enviando a nota certa (5) para a mesma matéria/período/tipo/categoria.
- O aggregate bloqueia com `"nota já registrada..."` (HTTP 400) — **não existe outra rota para corrigir**.
- A nota errada (15) fica definitivamente nas projeções de leitura (`projection_notas`) e é o valor que entra em qualquer cálculo de avaliação final automática (`tentarAvaliacoesFinaisAutomaticas`, disparado no mesmo request de `RegistrarNota`).

O mesmo vale para faltas: uma falta lançada com `quantidade` errada, matéria errada, ou até para o estudante errado (se o operador digitar o código errado) fica permanente.

**Por que isto bloqueia o lançamento em produção:** erro de digitação humana é estatisticamente inevitável em qualquer sistema de lançamento de notas em escala (fundamental, médio ou superior). Sem via de correção, o suporte só teria uma saída: acesso direto ao banco de produção para manipular manualmente o `spuri_ledger` e as projeções — o que quebra completamente o modelo de auditoria/imutabilidade que o sistema tenta garantir, e é operacionalmente inviável em escala.

**Correção recomendada (padrão de evento compensatório, preservando a imutabilidade do ledger):**
1. Adicionar dois novos eventos ao aggregate `Estudante`:
   - `NotaCorrigidaEvent` — payload com `NotaAnteriorID`/chave de negócio da nota original, `NovaNota`, `NovaObservacao` (opcional), `Motivo` (obrigatório), `CorrigidoPor` (UUID), `CorrigidoEm`.
   - `FaltaCorrigidaEvent` — mesmo padrão para `Quantidade`.
2. Novo método de comando no aggregate, ex. `(e *Estudante) CorrigirNota(chaveOriginal, novaNota, motivo, corrigidoPor, ...)`, que **não apaga** o evento original — apenas emite um evento de correção referenciando-o. Regras de negócio a impor no aggregate:
   - Só é possível corrigir uma chave que já existe em `NotasRegistradasPorChave`/`FaltasRegistradasPorChave`.
   - `motivo` obrigatório e não vazio (auditoria).
3. Rotas novas: `PATCH /academia/notas-aluno/:id` e `PATCH /academia/faltas-aluno/:id` (ou `POST .../corrigir`), restritas a `academia` (dona do registro) com `motivo` obrigatório no corpo.
4. Na projeção (`notas_projection.go`/`faltas_projection.go`), tratar o novo tipo de evento com `UPDATE` do valor atual (mantendo `codigo_estudante`+chave de negócio) e persistir `valor_anterior`, `corrigido_por`, `motivo_correcao`, `corrigido_em` em colunas próprias — assim a projeção mostra o valor vigente, mas o histórico completo (valor original + todas as correções) continua 100% recuperável tanto pelo ledger quanto por uma coluna JSON de histórico na própria projeção, se for desejável evitar reprocessar o ledger para exibir o histórico na UI.
5. Reprocessar `tentarAvaliacoesFinaisAutomaticas` também no fluxo de correção, para que uma avaliação final já calculada com o valor errado seja recalculada (ver também `TRACE-04`).
6. Nova migration SQL adicionando as colunas de correção a `projection_notas`/`projection_faltas` (reintroduzindo, com propósito mais amplo, o que a migration 038 tinha feito para soft delete).

---

### PROD-02 — ALTO — Sem retry automático em falhas de serialização (SQLSTATE 40001) na gravação

**Arquivos:** `internal/db/client.go` (linha 153, `BeginTx` usa `sql.LevelSerializable`), `internal/db/repository.go` (`Save`/`SaveWithAudit`), `internal/utils/errors.go` (`IsTransientDatabaseError`).

**Problema:** toda gravação de nota/falta abre uma transação `SERIALIZABLE` (`internal/db/client.go:153`) e lê a versão atual do aggregate dentro dela (`getAggregateVersionTx`). Esse é o padrão correto para evitar condição de corrida entre dois `RegistrarNota` concorrentes para o **mesmo estudante** — mas o Postgres resolve esse tipo de conflito abortando uma das duas transações com `SQLSTATE 40001` (`serialization_failure`), e **é responsabilidade da aplicação repetir a transação**. Isso não acontece em nenhum lugar do código: `IsTransientDatabaseError` (`internal/utils/errors.go:347-372`) reconhece apenas erros de conexão (timeout, connection refused, etc.) — não reconhece `40001` nem `40P01` (deadlock). O resultado é que uma transação abortada por conflito de serialização vira um `500 Erro interno do servidor` genérico para o cliente, com a gravação **perdida silenciosamente do ponto de vista do usuário** (ele vê erro, mas não sabe se foi ou não gravado, e não há retry automático).

**Cenário concreto em que isso ocorre com uso legítimo (não é só teoria):** duas requisições simultâneas de lançamento de nota para o mesmo estudante — por exemplo, duas disciplinas diferentes sendo lançadas quase ao mesmo tempo por dois professores em dois dispositivos, ou o front-end disparando duas chamadas em paralelo (uma de nota, uma de falta) para o mesmo aluno. Como ambas operam sobre o mesmo `aggregate_id` (o estudante), o Postgres pode abortar uma delas mesmo sem conflito de negócio real (apenas por tocarem o mesmo intervalo de linhas do ledger). Em época de lançamento de notas (pico de uso concorrente por dezenas/centenas de professores), a probabilidade de ocorrência sobe consideravelmente.

**Correção recomendada:**
1. Adicionar em `internal/utils/errors.go` uma função `IsSerializationFailure(err error) bool` que detecta `pq.Error.Code == "40001"` (e, por robustez, `"40P01"` para deadlock).
2. Envolver as chamadas a `repository.Save` / `repository.SaveWithAudit` em `RegistrarNota` e `RegistrarFaltas` (e em qualquer outro ponto que grava eventos no mesmo aggregate sob concorrência esperada) com um retry curto (ex. 3 tentativas, backoff de 20–100ms) especificamente para esse código de erro — **recarregando o aggregate a cada tentativa** (`repository.Load` de novo), já que o estado em memória pode estar desatualizado após o abort.
3. Cobrir com um teste de integração que dispara N goroutines chamando `RegistrarNota`/`RegistrarFalta` concorrentemente para o mesmo estudante (com chaves de negócio diferentes, sem conflito de regra de negócio) e garante que todas completam com sucesso (sem 500 espúrio).

---

### PROD-03 — MÉDIO — Teto de escala de nota (0–10 / 0–20) só é validado no handler HTTP, não no aggregate

**Arquivos:** `internal/handlers/modelo_avaliativo_escolar.go`, linhas 192–204 (`validarEscalaNotaPorAnoAcademico`); `internal/domain/aggregates/estudante_notas.go`, linha 154 (`if nota < 0 { ... }` — único limite no domínio).

**Problema:** a migration `052_notas_sem_teto_faltas_sem_unicidade.sql` removeu o `CHECK (nota <= 20)` do banco, com o comentário "permitir notas com qualquer valor >= 0". Na prática, essa permissividade só existe no banco: a validação de teto (10 pontos para 1º–6º ano do fundamental, 20 pontos para os demais) foi mantida, mas **apenas na camada HTTP** (`validarEscalaNotaPorAnoAcademico`, chamada dentro de `RegistrarNota` antes de acionar o aggregate). O método de domínio `Estudante.RegistrarNota` — que é o "dono" da regra de negócio em uma arquitetura de event sourcing — só valida `nota >= 0`, sem limite superior algum.

**Impacto:** hoje isso não é explorável porque o único caminho de entrada é o handler HTTP (confirmado: lote síncrono e assíncrono reutilizam o mesmo `handlers.RegistrarNota`). Mas é uma violação do princípio "aggregate é a fonte única de verdade" que o restante do código segue rigorosamente (ex. a chave de duplicata é resolvida no aggregate, não só na projeção). Qualquer novo caminho de entrada futuro — o próprio evento de correção proposto em `PROD-01`, uma importação direta via job interno, uma futura integração — pode reintroduzir o bug de "nota 999 registrada" se não repetir manualmente essa validação, porque o domínio não protege a si mesmo.

**Correção recomendada:** mover a validação de teto de escala para dentro de `Estudante.RegistrarNota` (aggregate), recebendo o teto máximo como parâmetro calculado pelo handler (igual já é feito com `categoriasAdicionais` e `periodosValidos`) — assim o aggregate continua agnóstico de regras de UI/handler específicas, mas a invariante "nota dentro da escala" nunca pode ser violada independentemente de quem chama o método.

---

### PROD-04 — MÉDIO — Nenhuma verificação do status de matrícula antes de registrar nota ou falta

**Arquivos:** `internal/handlers/notas_handlers.go` (`RegistrarNota`), `internal/handlers/faltas_handlers.go` (`RegistrarFaltas`); valores de status em `internal/domain/aggregates/estudante.go` (`"em_andamento"`, `"inativo"`, `"finalizado"` para `StatusEscolarFundamental`, `StatusEscolarMedio`, `StatusSuperior`).

**Problema:** os dois handlers verificam apenas que `estudanteDTO.CodigoAcademia == academiaDTO.CodigoAcademia` (estudante pertence à academia). Nenhum dos dois verifica se o **nível específico** para o qual a nota/falta está sendo lançada está com status `"em_andamento"`. Como o vínculo `CodigoAcademia` permanece preenchido mesmo depois de o estudante concluir (`"finalizado"`) ou ter o vínculo interrompido (`"inativo"`) naquele nível — por design, para preservar o histórico consultável — nada impede uma academia de lançar uma nota nova para um estudante já formado ou desligado daquele nível específico.

**Impacto:** combinado com `PROD-01` (impossibilidade de correção/exclusão), um lançamento indevido para um estudante já `"finalizado"`/`"inativo"` fica permanentemente nos seus registros históricos, sem qualquer bloqueio no momento da gravação.

**Correção recomendada:** em `RegistrarNota`/`RegistrarFaltas`, antes de chamar o aggregate, resolver qual campo de status corresponde ao `tipoEnsino` inferido para a nota/falta (fundamental → `StatusEscolarFundamental`, médio → `StatusEscolarMedio`, superior → `StatusSuperior`) e rejeitar com erro de validação (400) se o status não for `"em_andamento"`. Se houver um caso de uso legítimo para lançamento retroativo em nível já finalizado (ex. correção de histórico escolar), tratar como uma operação explícita e auditada separadamente (ex. só admin, com motivo obrigatório), não pelo fluxo comum de lançamento.

---

### PROD-05 — BAIXO — Falta de teto de sanidade em `quantidade` de faltas e tamanho de `observacao`

**Arquivo:** `internal/handlers/faltas_handlers.go`, linhas 51–57 (`Quantidade int` com `binding:"required,min=1"`, sem `max`); `Observacao *string` sem limite de tamanho em ambos os handlers.

**Problema:** não há teto máximo para `quantidade` (ex. nada impede `quantidade: 999999`), nem limite de tamanho para o campo de texto livre `observacao` em nota ou falta.

**Correção recomendada:** adicionar `max_quantidade_faltas` às matérias disciplinares, para que a academia possa definir qual é o limite de faltas que aquela matéria pode ter, e definir globalmente no sistema um mínimo de quantidade de faltas `1`. E um limite de tamanho para `observacao` (ex. 1000–2000 caracteres), validado no handler antes de chegar ao aggregate.

---

### PROD-06 — BAIXO / LIMPEZA — Handlers de lote síncronos (`RegistrarNotaBatch`/`RegistrarFaltasBatch`) existem no código mas não são roteados

**Arquivos:** `internal/handlers/batch_handlers.go`, linhas 262–324 (`RegistrarNotaBatch`, `RegistrarFaltasBatch`); confirmado por busca em `cmd/server/main.go` que essas duas funções **não aparecem em nenhum `router.POST(...)`** — apenas as versões assíncronas (`RegistrarNotaBatchAsync`/`RegistrarFaltasBatchAsync`) estão registradas, e a documentação da API (`Documentação da API.md`) também só documenta a versão single-item e a versão `/async`.

**Impacto:** nenhum, funcionalmente (código morto, inacessível). Risco é de manutenção: um desenvolvedor (humano ou IA) pode presumir que essa rota existe e está em produção, ou pode gastar tempo "corrigindo" um caminho de código que nunca executa.

**Correção recomendada:** remover as duas funções (ou, se o objetivo for reativar um endpoint síncrono de lote com limite menor além do assíncrono, registrar a rota explicitamente e documentar).

---

### PROD-07 — BAIXO / LIMPEZA — Modelos legados em `models.go` não refletem o contrato real da API

**Arquivo:** `internal/domain/models.go`, linhas 56–86 e 130–142 (`RegistroNotas`, `RegistroFaltas`, `Materia`, `MateriaFaltas`, `RegistrarNotasRequest`, `RegistrarFaltasRequest`).

**Problema:** essas structs descrevem um contrato de "uma requisição com várias matérias por vez" (`Materias []Materia`) que **não corresponde** ao contrato real implementado pelos handlers (`RegistrarNota`/`RegistrarFaltas` usam um `struct` anônimo local com um único `materia_disciplinar_id` por requisição). Busca confirma que nenhuma dessas structs é referenciada fora de `models.go`.

**Impacto:** nenhum em runtime (dead code), mas é uma fonte real de confusão para quem for ler `models.go` como referência de contrato — inclusive para a própria IA de execução (Codex), que pode ser induzida a "corrigir" endpoints para bater com este modelo errado.

**Correção recomendada:** remover as structs não usadas de `models.go`, ou, se preferível manter por compatibilidade com algum client externo desconhecido, adicionar comentário explícito `// DEPRECATED — não usado pelos handlers atuais` no topo de cada uma.

---

## 3. Unicidade (UNIQ)

### UNIQ-01 — ✅ Confirmado consistente (nenhuma ação corretiva necessária, apenas travar com teste de regressão) — Notas

Verificação cruzada, ponta a ponta:

| Camada | Chave de unicidade |
|---|---|
| Aggregate (`chaveNota`, `estudante_notas.go:104-106`) | `codigoAcademia + anoLectivo + periodo + materiaID + tipo + categoria` |
| Constraint SQL (`uq_nota_unica`, migration `057_unicidade_notas_com_codigo_academia.sql`) | `(codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)` |
| `ON CONFLICT` da projeção (`notas_projection.go:156`) | idênticas colunas da constraint acima |

As três camadas estão alinhadas hoje. Isso já foi motivo de **pelo menos três migrations corretivas anteriores** (`006`, `042`, `053_fix_constraint_legada_projection_notas`, `057`) por causa de desalinhamentos — ou seja, é um ponto historicamente frágil neste projeto.

**Recomendação:** adicionar um teste de integração que registra uma nota, tenta o **rebuild completo da projeção de notas** (`POST /admin/projections/rebuild/notas`) e confirma que o rebuild não gera violação `23505` nem duplica a linha — isso teria pego as três regressões anteriores automaticamente e deve ser mantido como teste de regressão permanente.

---

### UNIQ-02 — MÉDIO — Comentário incorreto/desatualizado sobre unicidade de faltas no aggregate

**Arquivo:** `internal/domain/aggregates/estudante_falta.go`, linhas 94–95:

```go
// applyFaltasRegistradas — aggregate não mantém estado derivado para faltas.
// A projeção persiste cada registro sem restrição de unicidade por data/matéria.
func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
```

**Problema:** este comentário está **errado** e contradiz tanto o código logo abaixo dele (que mantém sim `e.FaltasRegistradasPorChave` e é usado por `RegistrarFalta` para bloquear duplicata, linhas 65-71 do mesmo arquivo) quanto o estado atual do banco: a migration `052_notas_sem_teto_faltas_sem_unicidade.sql` de fato removeu a constraint `UNIQUE` de faltas em algum momento, mas a migration seguinte, `053_restaurar_unicidade_faltas.sql`, **restaurou** `uq_falta_unica UNIQUE (codigo_estudante, codigo_academia, data, materia_disciplinar_id)`. Ou seja, o comentário reflete um estado intermediário (entre as migrations 052 e 053) que já não é mais verdade há várias migrations.

**Impacto:** este tipo de comentário é uma armadilha específica para uma IA de execução (Codex) ou um desenvolvedor novo: alguém lendo apenas o comentário (sem cruzar com a migration 053) pode concluir, de boa fé, que a checagem de duplicata em `RegistrarFalta` é "código morto" ou inconsistente com o banco, e removê-la — o que reintroduziria duplicatas.

**Correção recomendada:** atualizar o comentário para refletir o estado real:

```go
// applyFaltasRegistradas mantém e.FaltasRegistradasPorChave para permitir que
// RegistrarFalta detecte duplicata por (estudante, academia, ano_lectivo, data,
// materia) antes de emitir o evento. A projeção reforça a mesma unicidade via
// constraint uq_falta_unica (codigo_estudante, codigo_academia, data,
// materia_disciplinar_id) — ver migration 053_restaurar_unicidade_faltas.sql.
```

---

### UNIQ-03 — BAIXO — `ON CONFLICT DO NOTHING` sem alvo explícito no handler de projeção de faltas

**Arquivo:** `internal/projections/faltas_projection.go`, linhas 149–160.

```go
_, err := tx.Exec(`
    INSERT INTO projection_faltas (...)
    VALUES (...)
    ON CONFLICT DO NOTHING
`, ...)
```

**Problema:** `ON CONFLICT DO NOTHING` sem especificar `(colunas)` ou `ON CONSTRAINT nome` faz o Postgres ignorar **qualquer** violação de constraint única/exclusão na tabela, não apenas a de `uq_falta_unica`. Hoje isso funciona porque `uq_falta_unica` é a única constraint de unicidade de negócio na tabela, mas é um padrão frágil: se uma futura migration adicionar outra constraint única em `projection_faltas` (ex. para suportar o evento de correção de `PROD-01`), inserções que deveriam falhar ruidosamente por violarem essa nova constraint passariam a ser silenciosamente descartadas sem log de erro, mascarando um bug.

**Correção recomendada:** trocar para `ON CONFLICT ON CONSTRAINT uq_falta_unica DO NOTHING` (ou lista explícita de colunas), igual ao padrão já usado corretamente em `notas_projection.go` (`ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)`).

---

### UNIQ-04 — INFO — Unicidade de falta não comporta múltiplas aulas da mesma matéria no mesmo dia

**Arquivo:** `internal/domain/aggregates/estudante_falta.go` (`chaveFalta`), constraint `uq_falta_unica`.

**Observação:** a chave de unicidade de falta é `(estudante, academia, data, matéria)` — ou seja, só é possível haver **um único registro de falta por matéria por dia**, com o campo `quantidade` absorvendo o número de aulas daquela matéria naquele dia em que houve falta. Isso é coerente com o desenho atual (quantidade representa "quantas aulas daquela matéria o aluno faltou naquele dia"), mas **precisa ser confirmado com o time de produto/pedagógico**, porque em contextos de ensino médio/superior com grade horária dupla (ex. duas aulas da mesma disciplina no mesmo dia, uma de manhã outra à tarde, lançadas por professores diferentes ou em momentos diferentes) o modelo atual força que a segunda tentativa de lançamento daquele dia seja tratada como correção do mesmo registro (usando o fluxo de correção proposto em `PROD-01`) em vez de um novo registro independente. Não é um bug — é uma decisão de modelagem que deve ser documentada explicitamente (ex. no `Documentação da API.md`) para que o time de front-end e as escolas entendam o comportamento esperado.

---

## 4. Rastreabilidade (TRACE)

### TRACE-01 — MÉDIO — `RegistradoPor` de nota existe no evento mas não chega à projeção nem à API

**Arquivos:** `internal/domain/aggregates/estudante_notas.go`, linha 47 (`RegistradoPor uuid.UUID` no evento, comentário "FIX E-06: campo... adicionado para auditoria self-contained"); `internal/projections/notas_projection.go`, `handleNotasRegistradas` (INSERT não inclui `registrado_por`) e `NotaDTO` (sem o campo).

**Problema:** o payload do evento `NotasRegistradas` já carrega quem lançou a nota (`RegistradoPor`), mas essa informação nunca é propagada para a tabela de leitura `projection_notas`, nem exposta em `NotaDTO`/`NotaRegistroResponse`. Quem quiser responder "quem lançou esta nota?" hoje só consegue fazendo introspecção manual do ledger (`spuri_ledger`) — que não tem nenhum endpoint de consulta exposto (ver `TRACE-03`).

**Correção recomendada:**
1. Migration adicionando `registrado_por UUID` a `projection_notas`.
2. `handleNotasRegistradas` passa a inserir `payload.RegistradoPor` nessa coluna.
3. `NotaDTO`/`NotaRegistroResponse` ganham o campo `registrado_por` (e, se fizer sentido para a UI, um `JOIN` para o nome de quem registrou, análogo ao que já é feito com `academia_nome`/`estudante_nome` em `registros_handlers.go`).
4. Adicionar `POST /admin/projections/rebuild/notas` ao checklist de deploy dessa mudança, já que é uma alteração de projeção.

---

### TRACE-02 — MÉDIO — Evento de falta não tem campo `RegistradoPor` equivalente ao de nota

**Arquivo:** `internal/domain/aggregates/estudante_falta.go`, linhas 17–28 (`FaltasRegistradasEvent`) — comparar com `NotasRegistradasEvent` em `estudante_notas.go`, que tem o campo `RegistradoPor` desde a "FIX E-06".

**Problema:** o mesmo cuidado de auditoria aplicado ao módulo de notas não foi replicado ao módulo de faltas — inconsistência entre dois módulos irmãos com o mesmo padrão de uso (ambos lançados por uma academia autenticada, ambos idealmente precisando responder "quem registrou isso"). Hoje a única fonte de "quem registrou uma falta" é o campo genérico `metadata.user_id` do ledger (preenchido por `SaveWithAudit`), que sofre da mesma limitação de `TRACE-03`: não tem endpoint de consulta.

**Correção recomendada:** replicar o padrão de `NotasRegistradasEvent` — adicionar `RegistradoPor uuid.UUID` a `FaltasRegistradasEvent`, propagar em `RegistrarFalta` (o handler já tem `userID` disponível, só precisa passá-lo adiante), persistir em `projection_faltas.registrado_por` (nova migration) e expor em `FaltaDTO`/`FaltaRegistroResponse`. Fazer isso **na mesma migration/PR** de `TRACE-01`, para manter os dois módulos simétricos.

---

### TRACE-03 — MÉDIO — Não existe endpoint de auditoria para consultar o histórico bruto de um evento específico

**Arquivos:** `internal/db/event_store.go` (já expõe `GetEventByID`, `LoadEventStream`, `GetEventsByType` — todos já implementados no nível de repositório); ausência confirmada de qualquer rota em `cmd/server/main.go` que os exponha para notas/faltas.

**Problema:** o event store já tem toda a capacidade necessária (inclusive `GetEventByID`, que devolveria `metadata` com `user_id`/`user_type`/`ip` de quem gravou), mas **nada disso é acessível via API**. `ListarNotas`/`ListarFaltas` retornam apenas o estado atual da projeção, com `event_id` — mas não há rota `GET /admin/eventos/:event_id` (ou equivalente) para transformar esse `event_id` em "quem, quando, de que IP".

**Correção recomendada:** adicionar uma rota restrita a `admin` (ex. `GET /admin/eventos/:event_id`) que usa `AggregateRepository.GetEventHistory`/`eventStore.GetEventByID` já existentes para devolver o evento completo (payload + metadata) dado um `event_id` — a mesma rota serve tanto para notas quanto para faltas quanto para qualquer outro módulo, já que o event store é genérico. É o complemento natural de `TRACE-01`/`TRACE-02`: mesmo sem esperar a correção completa desses dois itens, essa rota já destravaria auditoria manual via `metadata.user_id` hoje mesmo.

---

### TRACE-04 — BAIXO — Sem histórico de correções (dependente de `PROD-01`)

Enquanto `PROD-01` não for implementado, não há "o que" rastrear em termos de histórico de correção (não existem correções). Ao implementar `PROD-01`, garantir que o desenho contemple, desde o início: (a) o valor anterior sempre visível junto ao valor corrigido, (b) quem corrigiu, quando e por quê, e (c) que o evento de correção também dispara o recálculo de qualquer avaliação final que já tenha sido computada com o valor antigo (ver nota já feita em `PROD-01`, item 5).

---

## 5. Matriz de testes recomendados (multi-contexto)

O `internal/handlers/notas_handlers_test.go` atual cobre **apenas** a função `inferirAnoAcademicoParaNota` (6 testes, todos sobre compatibilidade ano-do-estudante × ano-da-matéria). Não há nenhum teste de handler de ponta a ponta, nem de duplicidade, nem de concorrência, nem de faltas. Recomenda-se cobrir, no mínimo, a matriz abaixo antes de considerar o módulo pronto para produção. Contextos: **fundamental** (1º–9º ano), **médio** (1º–4º ano, incl. modelo técnico com PAP), **superior** (por semestre/período do curso).

| # | Contexto | Cenário | Resultado esperado |
|---|---|---|---|
| T1 | Fundamental 1º-6º ano | Lançar nota 10 (limite superior exato) | Sucesso |
| T2 | Fundamental 1º-6º ano | Lançar nota 10.01 | Rejeitado (fora da escala) |
| T3 | Fundamental 7º-9º / Médio | Lançar nota 20 (limite superior exato) | Sucesso |
| T4 | Fundamental 7º-9º / Médio | Lançar nota 20.01 | Rejeitado (fora da escala) |
| T5 | Qualquer | Lançar nota negativa | Rejeitado |
| T6 | Superior | Lançar nota com escala do curso (confirmar se herda 0–20 do sistema ou se deveria ser configurável por curso — ver `PROD-03`) | Definir com produto e testar conforme decisão |
| T7 | Qualquer | Lançar a mesma nota duas vezes seguidas (mesma chave) | 2ª tentativa rejeitada com erro de duplicata |
| T8 | Qualquer | Duas requisições **concorrentes** (goroutines) para o mesmo estudante, chaves de negócio diferentes (sem conflito de regra) | Ambas devem ter sucesso (cobre `PROD-02`) |
| T9 | Qualquer | Duas requisições concorrentes para a **mesma chave exata** | Exatamente uma sucesso, uma rejeitada por duplicata — nunca duas gravações |
| T10 | Escolar (fundamental/médio) | Tentar registrar `tipo=superior` numa academia `nivel=escola` | Rejeitado (`academia do tipo 'escola' só pode registrar notas do tipo 'escolar'`) |
| T11 | Superior | Tentar registrar `periodo` diferente do período fixo da matéria | Rejeitado |
| T12 | Qualquer | `categoria` não configurada para o `ano_academico` inferido | Rejeitado |
| T13 | Qualquer | `materia_disciplinar_id` pertence a outra academia | 403 |
| T14 | Qualquer | `codigo_estudante` pertence a outra academia | 403 |
| T15 | Qualquer (após corrigir `PROD-04`) | Estudante com status `finalizado`/`inativo` no nível da nota | Rejeitado |
| T16 | Rebuild | Registrar N notas, chamar rebuild da projeção de notas, comparar contagem/valores antes e depois | Idêntico, sem erro `23505` (regressão de `UNIQ-01`) |
| T17 | Faltas | Lançar falta com `data` fora da janela do ano letivo (antes de setembro/outubro ou depois de julho) | Rejeitado |
| T18 | Faltas | Lançar falta duplicada (mesmo estudante/academia/data/matéria) | Rejeitado |
| T19 | Faltas | Lançar falta com `quantidade` muito alta (ex. 999) | Rejeitado após `PROD-05` |
| T20 | Faltas | Rebuild da projeção de faltas após N lançamentos | Idêntico, sem erro `23505` |
| T21 | Autenticação | Requisição sem token / token expirado / token com `ENV` mal configurado usando o segredo padrão (ver `SEC-01`) | Deve falhar mesmo após a correção — teste negativo específico de segurança |
| T22 | Auditoria (após `TRACE-01`/`TRACE-02`) | Registrar nota/falta autenticado como academia X, consultar `registrado_por` na resposta | Deve bater com o `user_id` da academia autenticada |
| T23 | Correção (após `PROD-01`) | Registrar nota errada, corrigi-la, consultar valor atual e histórico | Valor atual = corrigido; histórico mostra ambos; ledger nunca perde o evento original |
| T24 | Avaliação final automática | Registrar nota que dispara avaliação final automática (ex. última categoria despertadora do 9º ano) e depois corrigir essa nota (após `PROD-01`) | Avaliação final deve ser recalculada, não deve ficar "presa" no resultado antigo |
| T25 | Lote assíncrono | Enviar lote de 200+ notas via `/notas-aluno/async`, incluindo 1 item inválido no meio | Job final com `status=failed`, `fail_items=1`, os demais itens gravados corretamente (comportamento parcial já existente — cobrir com teste automatizado) |

---

## 6. Plano de execução sugerido (ordem de prioridade)

1. **`SEC-01`** (crítico, isolado, baixo risco de regressão) — corrigir a lógica de `ENV`/`JWT_SECRET` antes de qualquer outra coisa. Sem isso, nada mais neste relatório importa se o segredo padrão vazar em produção.
2. **`PROD-01`** (crítico, maior esforço) — desenhar e implementar o evento de correção para notas e faltas. É o item que mais bloqueia um lançamento real em escola/universidade.
3. **`PROD-02`** (alto) — adicionar detecção + retry de `40001` em `Save`/`SaveWithAudit`. Baixo risco, alto valor para estabilidade.
4. **`UNIQ-02`** (médio, trivial) — corrigir o comentário incorreto antes que induza uma regressão futura; fazer isso **antes** de qualquer refatoração no aggregate de faltas por outra pessoa/IA.
5. **`TRACE-01` + `TRACE-02` + `TRACE-03`** (médios, relacionados) — implementar juntos: campo `RegistradoPor` em faltas, propagação para as duas projeções, e o endpoint de auditoria `GET /admin/eventos/:event_id`.
6. **`PROD-03`** (médio) — mover validação de teto de nota para o aggregate.
7. **`PROD-04`** (médio) — checagem de status de matrícula antes de gravar.
8. **`SEC-02`** (médio) — logger com nível configurável.
9. **`SEC-03`, `PROD-05`, `UNIQ-03`** (baixos, rápidos) — podem ser feitos em qualquer ordem, inclusive em paralelo com os itens acima.
10. **`PROD-06`, `PROD-07`** (limpeza) — por último, sem urgência.

Depois de cada item, rodar os testes da seção 5 relevantes àquele item (a coluna "#" de cada teste está mapeada nos comentários acima) antes de avançar.

---

## 7. Checklist de validação pós-correção

- [ ] Subir a aplicação com `ENV` vazio/ausente e `JWT_SECRET` vazio → processo deve falhar (não usar segredo padrão).
- [ ] Rebuild completo de `projection_notas` e `projection_faltas` após popular dados de teste em fundamental, médio e superior → sem erros `23505`, contagens batem.
- [ ] Duas notas/faltas concorrentes para o mesmo estudante, chaves diferentes → ambas gravadas, sem 500 espúrio.
- [ ] Nota lançada errada → corrigida via novo fluxo → valor atual correto, histórico consultável, ledger original preservado.
- [ ] `GET /notas-estudante/:codigo` e `GET /faltas-estudante/:codigo` retornam `registrado_por`.
- [ ] Logs em ambiente `production` não imprimem valor de nota/observação em nível `info`.
- [ ] Testes automatizados da seção 5 (T1–T25) todos verdes.
- [ ] `go vet ./...` e `go test ./...` sem falhas em todo o repositório (não só nos pacotes tocados).
