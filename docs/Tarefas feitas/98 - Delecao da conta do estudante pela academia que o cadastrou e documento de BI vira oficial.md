---
criado: 2026-09-11
origem: Fredy + Claude (orquestração)
status: feito
depende_de: nenhuma
bloqueia: "Tarefa (frontend/spuripainel) - botão de deletar conta de estudante para a academia"
---

# Tarefa 98 — Deleção da conta do estudante pela academia que o cadastrou; documento da solicitação de edição de BI vira o documento oficial

## 0. Prompt recomendado para executar esta tarefa

> Aplique exatamente o que está descrito nesta tarefa, sem replanejar nem propor alternativas de desenho — todas as decisões de arquitetura já foram tomadas e validadas com PostgreSQL real (ver seção 2 e seção 8). Para os blocos "Diff exato", localize o trecho `-` (removido) exatamente como está no arquivo e substitua pelo trecho `+` (adicionado) — não reescreva o arquivo inteiro, não "melhore" nada além do que está pedido. Para os blocos "NOVO ARQUIVO", crie o arquivo com o conteúdo exato fornecido, no caminho indicado. Siga a ordem das seções 6 e 7. Ao final, rode os comandos da seção 10 e resolva qualquer erro de compilação antes de finalizar — mas **não tente instalar PostgreSQL, Docker, nem repetir os testes de integração que dependem de banco real** (seção 9 explica por quê e mostra a evidência já coletada). Leia a seção 11 (achado não relacionado, não mexer) antes de começar, para não confundir aquilo com um problema desta tarefa. Confira o checklist de aceite (seção 12) ao final e siga o procedimento de conclusão (seção 13).

## 1. Contexto

Dois pedidos, ambos em `fredypdp/spuri-backend`:

**Ponto 1 — Deleção da conta pelo estudante ou pela academia.**
Hoje (`Estudante.Deletar`, Tarefa 73) só existe autodeleção: o próprio estudante só pode deletar a conta depois de já estar desvinculado de toda academia (`Status == "inativo"`). Isso continua exatamente igual. O que falta: a academia **atualmente vinculada** ao estudante também poder deletar a conta dele diretamente (sem exigir desvinculação prévia) — mas **só** quando essa mesma academia foi a que **originalmente cadastrou** o estudante no Spuri.

**Ponto 2 — Documento da solicitação de BI vira o documento oficial.**
No fluxo `POST /estudante/solicitacoes-edicao/bilhete-identidade` → aprovação da academia (`PUT /academia/.../bilhete-identidade/:codigo/aprovar`), o PDF enviado pelo estudante é **sempre deletado do storage** depois da aprovação (`DecidirSolicitacaoEdicaoDadoEstudanteHandler` → `aplicarEdicaoAprovada`, em `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go`) — o número do BI é atualizado, mas o documento comprobatório se perde, e `Estudante.Documentos["bi_estudante"]` nunca é criado nem atualizado. Isso é um bug: se o BI foi atualizado através dessa solicitação, o documento usado nela **é** o BI correspondente e deveria passar a ser o documento oficial do estudante — substituindo o anterior, mesmo que não houvesse nenhum documento de BI registrado ainda (comum, já que o cadastro nunca exigiu isso).

Todo o desenho abaixo já foi implementado por mim (Claude) num clone atualizado do repositório (`origin/main` no commit `a5e23bd`, que já inclui as Tarefas 95/96/97 mescladas durante esta sessão — por isso esta tarefa é numerada **98**, não 95), com PostgreSQL 16 real instalado no meu sandbox (não mock), as migrações aplicadas do zero, e a suíte completa de testes — incluindo os testes novos desta tarefa — rodando verde. Ver seção 8 para evidência completa.

## 2. Decisões de design já tomadas (não repensar)

1. **"Academia que cadastrou o estudante" é determinado pelo ledger, sem campo novo.** `Estudante.CodigoAcademia` (estado atual) **não** serve para isso: ele é sobrescrito toda vez que o estudante é revinculado (`applyEstudanteReintegrado`), inclusive para uma academia **diferente** da original — a rota `POST /estudante/solicitacoes-status/revinculacao/:codigo_academia` aceita qualquer `:codigo_academia`, então um estudante pode ser revinculado (efetivamente transferido) para uma academia que nunca o cadastrou. A fonte de verdade correta é o **primeiro evento do ledger** do estudante — sempre `EstudanteCriadoComVinculo` (único evento de criação possível no dispatcher `Apply`) — cujo payload traz o `CodigoAcademia` de quem fez o cadastro original. Isso é buscado via `AggregateRepository.GetEventHistory(id)` (método que já existe, usado hoje só pela auditoria), que retorna os eventos ordenados por versão — `historico[0]` é sempre a criação.

2. **Duas checagens em camadas diferentes, cada uma no lugar certo.** O aggregate (`Estudante.DeletarPorAcademia`) só valida o que ele consegue verificar sozinho a partir do seu próprio estado: que o estudante está de fato **vinculado** (`Status IN ('ativo','pendente_documentos')`, mesmo critério usado em todo o resto do código) à academia informada. A checagem "essa academia foi quem cadastrou originalmente" depende do ledger (histórico de eventos), que o aggregate não tem acesso — por isso fica no **handler**, antes de chamar o método do aggregate. Isso segue o mesmo padrão de separação de responsabilidades já usado em `DesvincularDaAcademia`.

3. **Reaproveita o evento `EstudanteDeletado` existente — nenhuma migration, nenhuma entrada nova em `safe_queries.go`.** `DeletarPorAcademia` levanta o mesmo evento que `Deletar` (autodeleção), só que com `DeletadoPor` = ID da academia em vez do ID do próprio estudante. A projeção (`handleEstudanteDeletado`) já é genérica o suficiente para não precisar de nenhuma mudança. A única mudança de comportamento fora do aggregate é em `auditoria_delecoes_handler.go`, que tinha uma premissa hardcoded ("estudante é sempre autodeleção") — corrigida para resolver o nome/código da academia executora quando `DeletadoPor != AggregateID` (ver seção 6.3).

4. **Rota nova: `DELETE /academia/estudante/:codigo/conta`.** Simétrica à já existente `DELETE /estudante/conta` (autodeleção). Usa o helper `carregarEstudanteDaAcademia` já existente (mesmo padrão de outras rotas de academia sobre um estudante específico, ex. `/estudante/:codigo/revincular`), que já garante que o estudante está vinculado à academia autenticada antes de a checagem do ledger sequer rodar.

5. **Ponto 2: cópia (Read+Upload), não `Move`.** `StorageProvider.Move` existe mas nunca foi usado a partir de nenhum handler em produção (só internamente por `Rename`). Para minimizar risco num fluxo que envolve o documento de identidade oficial do estudante, o documento temporário é **copiado** (`provider.Read` + `provider.Upload`) para o caminho definitivo (mesmo padrão de `storagePathDocumentoEstudante`, usado no cadastro). Isso é deliberadamente mais seguro contra falha parcial: se a gravação do evento falhar depois da cópia, o **documento temporário original continua intacto** em `sol.DocumentoTemporarioPath` (a lógica de limpeza já existente no final de `DecidirSolicitacaoEdicaoDadoEstudanteHandler` cuida de apagá-lo, sem mudança nenhuma necessária ali) e a aprovação pode ser tentada de novo sem perda de dados. Se a cópia for bem-sucedida mas a gravação do evento falhar depois, o documento recém-copiado é removido (best-effort, `limparDocumentoBIOrfao`) para não deixar arquivo órfão. O documento **anterior** (se havia um) só é removido **depois** do evento confirmado no ledger — nunca antes, para que uma falha no meio do caminho deixe o sistema num estado consistente (documento antigo continua sendo o oficial).

6. **Bug real encontrado e corrigido (não fazia parte do pedido original, mas é necessário para o Ponto 2 funcionar de fato): a projeção também precisava ser corrigida.** `Estudante.Documentos` é uma coluna `documentos JSONB` em `projection_estudantes`. O handler de projeção genérico para a família de eventos "dados pessoais atualizados" (`handleDadosPessoaisAtualizados`, em `internal/projections/estudante_projection.go`) faz um `UPDATE` dinâmico só de colunas escalares (nome, email, bilhete_identidade, etc.) — **nunca tocava em `documentos`**. Corrigir só o aggregate (evento + `Apply`) teria deixado o *download* do documento (`documento_download_handlers.go`, que lê da projeção, não do aggregate) quebrado mesmo com o resto certo. Descobri isso rodando o fluxo HTTP completo de ponta a ponta contra Postgres real (não teria aparecido num teste só de aggregate) — ver seção 8. A correção usa merge JSONB (`documentos = documentos || jsonb_build_object('bi_estudante', $N::jsonb)`), preservando as demais chaves do mapa (ex. `bi_encarregado`, `cedula_estudante`).

7. **Escopo do Ponto 2 é só `bilhete_identidade` do próprio estudante (`bi_estudante`), não `bilhete_identidade_encarregado`.** O pedido original fala especificamente do "bilhete de identidade do estudante". A solicitação de edição de BI do encarregado (`CampoEdicaoBIEncarregado`) tem exatamente o mesmo bug (documento descartado em vez de promovido) mas **não foi pedida** — ver seção 5 (fora de escopo) para a recomendação sobre isso.

## 3. Ponto 1 — Regra de negócio detalhada

- **Quem pode chamar `DELETE /academia/estudante/:codigo/conta`:** uma academia autenticada, sobre um estudante que está **atualmente vinculado a ela** (`Status IN ('ativo', 'pendente_documentos')` — os mesmos dois status tratados como "vinculado" em todo o resto do código, ex. `CountVinculadosAtivos`).
- **Condição adicional (o ponto central do pedido):** essa mesma academia precisa ter sido a que **cadastrou** o estudante originalmente — isto é, o primeiro evento do ledger do estudante (`EstudanteCriadoComVinculo`) precisa ter `CodigoAcademia` igual ao código da academia autenticada. Se o estudante foi transferido/revinculado para outra academia depois do cadastro, **só a academia original** pode deletar — a academia atual (se diferente) não pode, mesmo estando vinculada a ele agora.
- **`motivo` é obrigatório** no corpo da requisição (`{"motivo": "..."}"`), mesma exigência já existente na autodeleção.
- **Resposta:** `200 OK` com `{"message": "...", "codigo_estudante": "..."}"`, mesmo formato de `DeletarContaEstudante`.
- **Erros:**
  - `estudante` não encontrado ou não pertence a esta academia (não vinculado atualmente a ela) → `404`/`403` via `carregarEstudanteDaAcademia` (comportamento já existente, reaproveitado).
  - estudante vinculado mas nesta academia não foi quem cadastrou → `403 Forbidden`, mensagem "apenas a academia que cadastrou o estudante no Spuri pode deletar a conta dele".
  - estudante não vinculado no momento (já desvinculado ou nunca vinculado a esta academia) → `400`.
  - motivo vazio → `400`.
- **Autodeleção pelo estudante (`Deletar`/`DeletarContaEstudante`) fica exatamente como estava** — nenhuma mudança de comportamento, só de comentário/documentação.
- **Auditoria:** `AuditContext{UserType: "academia"}` na gravação do evento (mesmo padrão de `salvarEventoEstudante`, já usado por outras rotas de academia sobre estudante). O endpoint de auditoria de deleções (`GET /admin/auditoria/delecoes` ou equivalente, ver `auditoria_delecoes_handler.go`) passa a expor `deletado_por_tipo` (`"estudante"` ou `"academia"`) e, quando for academia, `deletado_por_nome`/`deletado_por_codigo_academia`.

## 4. Ponto 2 — Regra de negócio detalhada

- Aplica-se apenas quando `Campo == "bilhete_identidade"` (constante `aggregates.CampoEdicaoBI`) **e** a decisão é aprovação (`aprovar == true`) em `PUT /academia/solicitacoes-edicao-estudante/bilhete-identidade/:codigo/aprovar`.
- Ao aprovar, o documento PDF anexado à solicitação (`sol.DocumentoTemporarioPath`) é copiado para o caminho definitivo de documentos de identificação do estudante e passa a ser `Estudante.Documentos["bi_estudante"]` — **substituindo** o que estivesse lá antes, **mesmo que não houvesse nenhum documento ainda** (caso mais comum, já que o cadastro normal não exige documento de BI do próprio estudante).
- O documento anterior (se existia) é removido do storage **depois** do evento de aprovação confirmado no ledger — nunca antes.
- O número do BI (`Estudante.BilheteIdentidade`) continua sendo atualizado exatamente como já era.
- **Reprovação (`aprovar == false`) continua exatamente como estava** — o documento temporário é descartado, nenhum documento oficial é criado/alterado.
- **Os demais campos de edição (`nome`, `bilhete_identidade_encarregado`, `data_nascimento`) continuam exatamente como estavam** — sem nenhuma mudança de comportamento.

## 5. Fora de escopo (não implementar aqui)

1. **BI do encarregado (`bilhete_identidade_encarregado`) com o mesmo tratamento do Ponto 2.** Tem exatamente o mesmo bug (documento descartado em vez de promovido), mas não foi pedido. Recomendo uma tarefa separada, curta, que reaproveitaria quase todo o código desta (só trocaria a chave do mapa de `"bi_estudante"` para `"bi_encarregado"` e o campo de destino).
2. **Qualquer pré-condição financeira para a academia deletar o estudante** (ex. mensalidades em aberto). Não foi pedido; a autodeleção também nunca teve essa checagem, e a deleção é lógica (soft delete) — notas, faltas e avaliações permanecem intactas e consultáveis, mesma garantia que já existia.
3. **UI no frontend (`spuripainel`)** para a academia acionar a nova rota `DELETE /academia/estudante/:codigo/conta`. Este documento cobre só o backend, conforme pedido. Recomendo uma tarefa de frontend em seguida (fica registrado no `bloqueia:` do front-matter).
4. **Qualquer alteração em `internal/storage/storage.go`** (o provider em si). O Ponto 2 só *usa* métodos já existentes (`Read`, `Upload`, `Delete`) a partir do handler — o comentário de aviso no topo desse arquivo ("não alterar provider ou fluxo de arquivos sem solicitação explícita") continua respeitado; nada nele foi tocado.
5. **A fragilidade pré-existente e não relacionada na suíte de testes de integração** (academias inseridas via SQL direto em testes financeiros, sem evento no ledger, quebrando qualquer `EstudanteProjection.Rebuild()` global que rode depois na mesma execução do `go test`). Documentada em detalhe na seção 11 — **não é para ser corrigida como parte desta tarefa.**

## 6. Diffs exatos (arquivos já existentes)

Localize o bloco `-` exatamente como está no arquivo atual e substitua pelo bloco `+`. Contexto (linhas sem `+`/`-`) é só para localização, não faz parte da mudança.

### 6.1 `cmd/server/main.go`
```diff
diff --git a/cmd/server/main.go b/cmd/server/main.go
index c12d28e..cec13e2 100644
--- a/cmd/server/main.go
+++ b/cmd/server/main.go
@@ -577,6 +577,7 @@ func setupRouter() *gin.Engine {
 		academia.POST("/estudante/:codigo/desvincular/reprovar", handlers.ReprovarSolicitacaoStatusAcademicoHandler("desvinculacao"))
 		academia.POST("/estudante/:codigo/revincular", handlers.AprovarSolicitacaoStatusAcademicoHandler("revinculacao"))
 		academia.POST("/estudante/:codigo/revincular/reprovar", handlers.ReprovarSolicitacaoStatusAcademicoHandler("revinculacao"))
+		academia.DELETE("/estudante/:codigo/conta", handlers.DeletarContaEstudantePorAcademia) // Tarefa 98
 
 		// ── Cursos ────────────────────────────────────────────────────────
 		academia.POST("/curso", handlers.CriarCurso)
```

### 6.2 `internal/domain/aggregates/estudante.go`
```diff
diff --git a/internal/domain/aggregates/estudante.go b/internal/domain/aggregates/estudante.go
index feea64e..4006c94 100644
--- a/internal/domain/aggregates/estudante.go
+++ b/internal/domain/aggregates/estudante.go
@@ -260,8 +260,13 @@ type EstudanteDesvinculadoDaAcademiaEvent struct {
 func (e *EstudanteDesvinculadoDaAcademiaEvent) GetPayload() interface{} { return e }
 func (e *EstudanteDesvinculadoDaAcademiaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
 
-// EstudanteDeletadoEvent — Tarefa 73. Autodeleção lógica e auditável do
-// estudante. DeletadoPor é sempre igual ao AggregateID (self-service).
+// EstudanteDeletadoEvent — Tarefa 73. Deleção lógica e auditável do
+// estudante. Até a Tarefa 98, DeletadoPor era sempre igual ao AggregateID
+// (só existia autodeleção via Estudante.Deletar). A Tarefa 98 introduziu
+// Estudante.DeletarPorAcademia, então DeletadoPor agora também pode ser o ID
+// da academia que cadastrou o estudante — para distinguir os dois casos,
+// compare DeletadoPor com o AggregateID (iguais = autodeleção) ou consulte
+// AuditContext.UserType gravado no metadata do evento no ledger.
 type EstudanteDeletadoEvent struct {
 	BaseEvent
 	Motivo      string
@@ -306,6 +311,10 @@ type DadosPessoaisAtualizadosEvent struct {
 	TelefoneAlterado      bool
 	TelefoneEncAlterado   bool
 	UpdatedAt             time.Time
+	// DocumentoBI (Tarefa 98): presente somente quando o evento de origem é
+	// BilheteIdentidadeEstudanteAlteradoPorSolicitacao com documento anexo.
+	// Ver applyDadosPessoaisAtualizados.
+	DocumentoBI *DocumentoMatricula
 }
 
 func (e *DadosPessoaisAtualizadosEvent) GetPayload() interface{} { return e }
@@ -935,6 +944,10 @@ func (e *Estudante) AlterarCurso(cursoID uuid.UUID, tipoEnsino string) error {
 // O registro nunca é fisicamente apagado — apenas marcado como 'deletado'.
 // Notas, faltas e avaliações já lançadas permanecem intactas e consultáveis
 // (nenhuma FK em cascata as remove).
+//
+// Ver também DeletarPorAcademia (Tarefa 98): via de deleção alternativa,
+// acionada pela academia que cadastrou o estudante, enquanto ele ainda está
+// vinculado — o caminho oposto deste método (que exige desvinculação prévia).
 func (e *Estudante) Deletar(motivo string, deletadoPor uuid.UUID) error {
 	if e.Status == "deletado" {
 		return fmt.Errorf("estudante já está deletado")
@@ -956,6 +969,51 @@ func (e *Estudante) Deletar(motivo string, deletadoPor uuid.UUID) error {
 	return e.Apply(event)
 }
 
+// DeletarPorAcademia executa a deleção lógica (soft delete) e auditável do
+// estudante a pedido da academia à qual ele está atualmente vinculado.
+//
+// Regra de negócio (Tarefa 98): ao contrário de Deletar (autodeleção, que
+// exige o estudante já desvinculado), esta via é acionada pela ACADEMIA
+// enquanto o estudante ainda está vinculado a ela — mas só quando essa
+// academia foi a que originalmente cadastrou o estudante no Spuri. Este
+// método valida apenas os invariantes que o aggregate consegue checar
+// sozinho a partir do seu próprio estado (vínculo atual com
+// codigoAcademiaSolicitante); a confirmação de que codigoAcademiaSolicitante
+// é de fato a academia de cadastro original depende do histórico de eventos
+// no ledger (primeiro evento = EstudanteCriadoComVinculo) e por isso é
+// responsabilidade do handler, ANTES de chamar este método — ver
+// handlers.DeletarContaEstudantePorAcademia.
+//
+// "Vinculado" aqui é e.Status IN ('ativo', 'pendente_documentos') — o mesmo
+// critério usado em todo o resto do código (ver comentário em Deletar acima).
+//
+// DeletadoPor recebe o ID da academia (não mais sempre o ID do próprio
+// estudante) — ver comentário em EstudanteDeletadoEvent.
+func (e *Estudante) DeletarPorAcademia(motivo, codigoAcademiaSolicitante string, deletadoPor uuid.UUID) error {
+	if e.Status == "deletado" {
+		return fmt.Errorf("estudante já está deletado")
+	}
+	if e.Status != "ativo" && e.Status != "pendente_documentos" {
+		return fmt.Errorf("estudante não está vinculado a esta academia no momento")
+	}
+	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademiaSolicitante {
+		return fmt.Errorf("estudante não pertence a esta academia")
+	}
+	motivo = strings.TrimSpace(motivo)
+	if motivo == "" {
+		return fmt.Errorf("motivo da deleção é obrigatório")
+	}
+
+	event := &EstudanteDeletadoEvent{
+		BaseEvent:   BaseEvent{EventType: "EstudanteDeletado", AggregateID: e.ID},
+		Motivo:      motivo,
+		DeletadoPor: deletadoPor,
+		DeletedAt:   time.Now(),
+	}
+	e.RaiseEvent(event)
+	return e.Apply(event)
+}
+
 // ============================================================================
 // Apply handlers
 // ============================================================================
@@ -1214,6 +1272,15 @@ func (e *Estudante) applyDadosPessoaisAtualizados(event DomainEvent) error {
 	if ev.DataNascimento != nil {
 		e.DataNascimento = *ev.DataNascimento
 	}
+	// Tarefa 98: documento da solicitação de edição de BI substitui o
+	// documento oficial do estudante — mesmo que não houvesse nenhum
+	// documento em Documentos["bi_estudante"] ainda.
+	if ev.DocumentoBI != nil {
+		if e.Documentos == nil {
+			e.Documentos = map[string]DocumentoMatricula{}
+		}
+		e.Documentos["bi_estudante"] = *ev.DocumentoBI
+	}
 	return nil
 }
 
@@ -1319,12 +1386,22 @@ func (e *Estudante) AlterarNomePorSolicitacao(novo, codigoSolicitacao, decididoP
 	e.RaiseEvent(ev)
 	return e.Apply(ev)
 }
-func (e *Estudante) AlterarBilheteIdentidadePorSolicitacao(novo, codigoSolicitacao, decididoPor string) error {
+// AlterarBilheteIdentidadePorSolicitacao altera o BI do estudante após
+// aprovação da academia. documento é opcional (pode ser nil): quando
+// presente (Tarefa 98), é o documento anexado à solicitação de edição, que
+// passa a ser o documento oficial do BI do estudante
+// (Estudante.Documentos["bi_estudante"]) — substituindo o anterior, mesmo
+// que não houvesse nenhum documento registrado ainda. Quem monta esse
+// DocumentoMatricula (promovendo o arquivo temporário da solicitação para o
+// caminho definitivo no storage) é o handler
+// (handlers.aplicarEdicaoAprovada), não este método — o aggregate só grava o
+// que recebe.
+func (e *Estudante) AlterarBilheteIdentidadePorSolicitacao(novo, codigoSolicitacao, decididoPor string, documento *DocumentoMatricula) error {
 	v := strings.TrimSpace(novo)
 	if v == "" {
 		return fmt.Errorf("bilhete_identidade é obrigatório")
 	}
-	ev := &BilheteIdentidadeEstudanteAlteradoPorSolicitacaoEvent{BaseEvent: BaseEvent{EventType: "BilheteIdentidadeEstudanteAlteradoPorSolicitacao", AggregateID: e.ID}, BilheteIdentidade: &v, CodigoSolicitacao: codigoSolicitacao, DecididoPor: decididoPor, UpdatedAt: time.Now()}
+	ev := &BilheteIdentidadeEstudanteAlteradoPorSolicitacaoEvent{BaseEvent: BaseEvent{EventType: "BilheteIdentidadeEstudanteAlteradoPorSolicitacao", AggregateID: e.ID}, BilheteIdentidade: &v, CodigoSolicitacao: codigoSolicitacao, DecididoPor: decididoPor, UpdatedAt: time.Now(), DocumentoBI: documento}
 	e.RaiseEvent(ev)
 	return e.Apply(ev)
 }
@@ -1406,6 +1483,11 @@ type BilheteIdentidadeEstudanteAlteradoPorSolicitacaoEvent struct {
 	BilheteIdentidade              *string
 	CodigoSolicitacao, DecididoPor string
 	UpdatedAt                      time.Time
+	// DocumentoBI é opcional (Tarefa 98): quando presente, substitui
+	// Estudante.Documentos["bi_estudante"] pelo documento anexado à
+	// solicitação de edição aprovada — mesmo que não houvesse nenhum
+	// documento registrado ainda. Ver applyDadosPessoaisAtualizados.
+	DocumentoBI *DocumentoMatricula
 }
 
 func (e *BilheteIdentidadeEstudanteAlteradoPorSolicitacaoEvent) GetPayload() interface{} { return e }
```

### 6.3 `internal/handlers/auditoria_delecoes_handler.go`
```diff
diff --git a/internal/handlers/auditoria_delecoes_handler.go b/internal/handlers/auditoria_delecoes_handler.go
index e8eab26..c34bfdc 100644
--- a/internal/handlers/auditoria_delecoes_handler.go
+++ b/internal/handlers/auditoria_delecoes_handler.go
@@ -117,8 +117,18 @@ func ListarAuditoriaDelecoes(c *gin.Context) {
 				item["identificador"] = est.CodigoEstudante
 				item["nome"] = est.Nome
 			}
-			// Estudante é sempre autodeleção — deletado_por == entidade_id,
-			// não há um "executor" terceiro a resolver aqui.
+			// Tarefa 98: até então, estudante era sempre autodeleção
+			// (deletado_por == entidade_id). Agora a academia que cadastrou
+			// o estudante também pode deletar a conta dele enquanto
+			// vinculado — nesse caso deletado_por é o ID da academia, não
+			// do próprio estudante.
+			if payload.DeletadoPor == event.AggregateID {
+				item["deletado_por_tipo"] = "estudante"
+			} else if executor, err := academiaProj.GetByID(payload.DeletadoPor); err == nil && executor != nil {
+				item["deletado_por_tipo"] = "academia"
+				item["deletado_por_nome"] = executor.Nome
+				item["deletado_por_codigo_academia"] = executor.CodigoAcademia
+			}
 		}
 
 		resultado = append(resultado, item)
```

### 6.4 `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go`
```diff
diff --git a/internal/handlers/solicitacao_edicao_dado_estudante_handlers.go b/internal/handlers/solicitacao_edicao_dado_estudante_handlers.go
index 530b207..f3468fa 100644
--- a/internal/handlers/solicitacao_edicao_dado_estudante_handlers.go
+++ b/internal/handlers/solicitacao_edicao_dado_estudante_handlers.go
@@ -5,6 +5,7 @@ import (
 	"database/sql"
 	"errors"
 	"fmt"
+	"io"
 	"log"
 	"net/http"
 	"strconv"
@@ -214,11 +215,35 @@ func aplicarEdicaoAprovada(c *gin.Context, sol *projections.SolicitacaoEdicaoDad
 		return err
 	}
 	agg := loaded.(*aggregates.Estudante)
+
+	// Tarefa 98: quando a solicitação aprovada é de bilhete_identidade, o
+	// documento anexado a ela passa a ser o documento oficial do BI do
+	// estudante (Estudante.Documentos["bi_estudante"]), substituindo
+	// qualquer documento anterior — mesmo que não houvesse nenhum ainda.
+	// Promove o documento ANTES de gravar o evento: se a promoção falhar,
+	// nada no estudante é alterado e a solicitação continua pendente para
+	// nova tentativa. promoverDocumentoBIParaOficial usa cópia (Read +
+	// Upload), não Move — se a gravação do evento abaixo falhar depois, o
+	// documento temporário original em sol.DocumentoTemporarioPath permanece
+	// intacto e a aprovação pode ser refeita sem perda de dados.
+	var documentoBI *aggregates.DocumentoMatricula
+	var docAntigoBI aggregates.DocumentoMatricula
+	var tinhaDocAntigoBI bool
+	if sol.Campo == aggregates.CampoEdicaoBI {
+		docAntigoBI, tinhaDocAntigoBI = agg.Documentos["bi_estudante"]
+		doc, err := promoverDocumentoBIParaOficial(c, sol.CodigoAcademia, sol.CodigoEstudante, sol.DocumentoTemporarioPath)
+		if err != nil {
+			utils.RespondWithInternalError(c, fmt.Errorf("falha ao promover documento do bilhete de identidade: %w", err))
+			return err
+		}
+		documentoBI = doc
+	}
+
 	switch sol.Campo {
 	case aggregates.CampoEdicaoNome:
 		err = agg.AlterarNomePorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor)
 	case aggregates.CampoEdicaoBI:
-		err = agg.AlterarBilheteIdentidadePorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor)
+		err = agg.AlterarBilheteIdentidadePorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor, documentoBI)
 	case aggregates.CampoEdicaoBIEncarregado:
 		err = agg.AlterarBilheteIdentidadeEncarregadoPorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor)
 	case aggregates.CampoEdicaoDataNascimento:
@@ -226,17 +251,80 @@ func aplicarEdicaoAprovada(c *gin.Context, sol *projections.SolicitacaoEdicaoDad
 		err = agg.AlterarDataNascimentoPorSolicitacao(dt, sol.CodigoSolicitacao, decididoPor)
 	}
 	if err != nil {
+		limparDocumentoBIOrfao(c, documentoBI, sol.CodigoSolicitacao, "erro de validação")
 		utils.RespondWithValidationError(c, err)
 		return err
 	}
 	audit := db.AuditContext{UserID: decididoPor, UserType: "academia", IP: c.ClientIP()}
 	if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
+		limparDocumentoBIOrfao(c, documentoBI, sol.CodigoSolicitacao, "erro ao salvar solicitação")
 		utils.RespondWithInternalError(c, err)
 		return err
 	}
+	// Só remove o documento de BI anterior DEPOIS do evento confirmado no
+	// ledger — se o SaveWithAudit acima tivesse falhado, o documento antigo
+	// continuaria sendo o oficial (estado consistente para nova tentativa).
+	if sol.Campo == aggregates.CampoEdicaoBI && tinhaDocAntigoBI && strings.TrimSpace(docAntigoBI.Path) != "" && docAntigoBI.Path != documentoBI.Path {
+		if p := getStorageProvider(c); p != nil {
+			if delErr := p.Delete(docAntigoBI.Path); delErr != nil {
+				log.Printf("[WARN] falha ao remover documento de BI anterior %s do estudante %s: %v", docAntigoBI.Path, sol.CodigoEstudante, delErr)
+			}
+		}
+	}
 	return nil
 }
 
+// promoverDocumentoBIParaOficial copia (Read + Upload) o documento temporário
+// de uma solicitação de edição de bilhete_identidade para o caminho
+// definitivo dos documentos de identificação do estudante — o mesmo padrão
+// usado no cadastro (ver storagePathDocumentoEstudante), com um DocumentoID
+// novo. Copia em vez de mover para que o documento temporário original só
+// seja removido depois de o evento de aprovação ser gravado com sucesso (ver
+// aplicarEdicaoAprovada); assim, uma falha após a cópia não perde o arquivo
+// original nem deixa o estudante sem documento de BI.
+func promoverDocumentoBIParaOficial(c *gin.Context, codigoAcademia, codigoEstudante, tempPath string) (*aggregates.DocumentoMatricula, error) {
+	provider := getStorageProvider(c)
+	if provider == nil {
+		return nil, fmt.Errorf("storage não configurado")
+	}
+	rc, err := provider.Read(tempPath)
+	if err != nil {
+		return nil, fmt.Errorf("falha ao ler documento temporário: %w", err)
+	}
+	defer rc.Close()
+	data, err := io.ReadAll(rc)
+	if err != nil {
+		return nil, fmt.Errorf("falha ao ler documento temporário: %w", err)
+	}
+	downloadURL := estudanteDocumentoDownloadURL(codigoEstudante, "bi_estudante")
+	_, doc := documentoMatriculaNormalizado("bi_estudante", "", downloadURL, "", "")
+	destPath := fmt.Sprintf("%s/estudantes/%s/documentos/identificacao/%s/%s.pdf", codigoAcademia, codigoEstudante, doc.Tipo, doc.DocumentoID)
+	stored, err := provider.Upload(destPath, bytes.NewReader(data), int64(len(data)))
+	if err != nil {
+		return nil, fmt.Errorf("falha ao gravar documento definitivo: %w", err)
+	}
+	doc.Path = stored.Path
+	doc.FileURL = stored.FileURL
+	return &doc, nil
+}
+
+// limparDocumentoBIOrfao remove (best-effort) uma cópia de documento de BI já
+// promovida para o caminho definitivo quando um passo POSTERIOR falha (a
+// alteração no aggregate ou a gravação do evento) — evita deixar um arquivo
+// órfão sem nenhuma referência no ledger. Não fatal: falha aqui só gera log.
+func limparDocumentoBIOrfao(c *gin.Context, documento *aggregates.DocumentoMatricula, codigoSolicitacao, motivo string) {
+	if documento == nil {
+		return
+	}
+	p := getStorageProvider(c)
+	if p == nil {
+		return
+	}
+	if err := p.Delete(documento.Path); err != nil {
+		log.Printf("[WARN] falha ao limpar documento de BI órfão %s (solicitação %s, %s): %v", documento.Path, codigoSolicitacao, motivo, err)
+	}
+}
+
 func AtualizarTelefoneEncarregado(c *gin.Context) {
 	userID, _ := middleware.GetUserID(c)
 	var raw map[string]string
```

### 6.5 `internal/projections/estudante_projection.go`
```diff
diff --git a/internal/projections/estudante_projection.go b/internal/projections/estudante_projection.go
index fd8463f..b347155 100644
--- a/internal/projections/estudante_projection.go
+++ b/internal/projections/estudante_projection.go
@@ -669,6 +669,11 @@ func (p *EstudanteProjection) handleDadosPessoaisAtualizados(event db.Event) err
 		EmailAlterado         bool       `json:"EmailAlterado"`
 		TelefoneAlterado      bool       `json:"TelefoneAlterado"`
 		TelefoneEncAlterado   bool       `json:"TelefoneEncAlterado"`
+		// DocumentoBI (Tarefa 98): presente somente quando o evento de
+		// origem é BilheteIdentidadeEstudanteAlteradoPorSolicitacao com
+		// documento anexo — substitui Documentos["bi_estudante"] na
+		// projeção, espelhando applyDadosPessoaisAtualizados no aggregate.
+		DocumentoBI *aggregates.DocumentoMatricula `json:"DocumentoBI"`
 	}
 	if err := json.Unmarshal(event.Payload, &payload); err != nil {
 		return fmt.Errorf("handleDadosPessoaisAtualizados: parse error: %w", err)
@@ -722,6 +727,15 @@ func (p *EstudanteProjection) handleDadosPessoaisAtualizados(event db.Event) err
 		args = append(args, *payload.DataNascimento)
 		idx++
 	}
+	if payload.DocumentoBI != nil {
+		docJSON, err := json.Marshal(payload.DocumentoBI)
+		if err != nil {
+			return fmt.Errorf("handleDadosPessoaisAtualizados: falha ao serializar DocumentoBI: %w", err)
+		}
+		setClauses = append(setClauses, fmt.Sprintf("documentos = documentos || jsonb_build_object('bi_estudante', $%d::jsonb)", idx))
+		args = append(args, string(docJSON))
+		idx++
+	}
 
 	if len(setClauses) == 0 {
 		_, err := p.client.DB().Exec(`
```

## 7. Novos arquivos (conteúdo completo)

Crie cada arquivo abaixo exatamente com este conteúdo, no caminho indicado.

### 7.1 `internal/handlers/estudante_delecao_academia_handlers.go` — NOVO ARQUIVO

Handler novo: deleção da conta do estudante pela academia (Ponto 1).

```go
package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/utils"
)

// DeletarContaEstudantePorAcademia — Tarefa 98. Permite que a academia
// atualmente vinculada ao estudante delete a conta dele diretamente, sem
// exigir desvinculação prévia (esse é o caminho de autodeleção do próprio
// estudante, ver DeletarContaEstudante) — mas só quando essa mesma academia
// foi a que ORIGINALMENTE cadastrou o estudante no Spuri.
//
// "Cadastrou originalmente" é determinado consultando o primeiro evento do
// ledger do estudante (EstudanteCriadoComVinculo), e não o campo
// CodigoAcademia do estado atual: CodigoAcademia é sobrescrito sempre que o
// estudante é revinculado (ver Estudante.applyEstudanteReintegrado), inclusive
// para uma academia diferente da original (POST
// /estudante/solicitacoes-status/revinculacao/:codigo_academia aceita
// qualquer codigo_academia). Ou seja, uma academia que apenas recebeu o
// estudante por revinculação/transferência NÃO pode deletar a conta dele —
// só a que fez o cadastro (CriarComVinculo) na origem.
func DeletarContaEstudantePorAcademia(c *gin.Context) {
	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Motivo) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}

	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}

	if ctx.Estudante.Status != "ativo" && ctx.Estudante.Status != "pendente_documentos" {
		utils.RespondWithValidationError(c, fmt.Errorf("estudante não está vinculado a esta academia no momento"))
		return
	}

	repository := getRepository(c).WithContext(c.Request.Context())
	historico, err := repository.GetEventHistory(ctx.Estudante.GetID())
	if err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("falha ao consultar histórico do estudante: %w", err))
		return
	}
	if len(historico) == 0 || historico[0].EventType != "EstudanteCriadoComVinculo" {
		utils.RespondWithInternalError(c, fmt.Errorf("histórico do estudante inconsistente: primeiro evento inesperado"))
		return
	}
	var payloadCriacao struct {
		CodigoAcademia string
	}
	if err := json.Unmarshal(historico[0].Payload, &payloadCriacao); err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("falha ao interpretar histórico do estudante: %w", err))
		return
	}
	if payloadCriacao.CodigoAcademia != ctx.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "apenas a academia que cadastrou o estudante no Spuri pode deletar a conta dele")
		return
	}

	if err := ctx.Estudante.DeletarPorAcademia(req.Motivo, ctx.CodigoAcademia, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if !salvarEventoEstudante(c, ctx) {
		return
	}

	log.Printf("Estudante deletado pela academia que o cadastrou: %s (academia=%s) - Motivo: %s", ctx.Estudante.CodigoEstudante, ctx.CodigoAcademia, req.Motivo)
	c.JSON(http.StatusOK, gin.H{
		"message":          "conta do estudante deletada com sucesso",
		"codigo_estudante": ctx.Estudante.CodigoEstudante,
	})
}
```

### 7.2 `internal/domain/aggregates/estudante_delecao_academia_test.go` — NOVO ARQUIVO

Testes de unidade do aggregate para `DeletarPorAcademia` (Ponto 1).

```go
package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

// Tarefa 98 — Estudante.DeletarPorAcademia: a academia atualmente vinculada
// ao estudante pode deletar a conta dele diretamente (sem exigir
// desvinculação prévia, ao contrário de Deletar/autodeleção), desde que essa
// mesma academia seja a que está registrada em e.CodigoAcademia. A checagem
// de que essa academia também foi quem ORIGINALMENTE cadastrou o estudante
// (via histórico do ledger) é feita no handler, não neste método — ver
// estudante_delecao_academia_integration_test.go no pacote handlers.

func TestDeletarPorAcademiaOKQuandoAtivo(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "ativo"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademia, academiaID); err != nil {
		t.Fatalf("DeletarPorAcademia retornou erro inesperado: %v", err)
	}
	if estudante.Status != "deletado" {
		t.Fatalf("Status = %q, want %q", estudante.Status, "deletado")
	}
}

func TestDeletarPorAcademiaOKQuandoPendenteDocumentos(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "pendente_documentos"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademia, academiaID); err != nil {
		t.Fatalf("DeletarPorAcademia retornou erro inesperado: %v", err)
	}
	if estudante.Status != "deletado" {
		t.Fatalf("Status = %q, want %q", estudante.Status, "deletado")
	}
}

func TestDeletarPorAcademiaFalhaQuandoDesvinculado(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "inativo" // desvinculado

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademia, academiaID); err == nil {
		t.Fatal("esperava erro ao deletar estudante desvinculado via DeletarPorAcademia, mas não houve erro")
	}
	if estudante.Status != "inativo" {
		t.Fatalf("Status mudou apesar do erro: %q", estudante.Status)
	}
}

func TestDeletarPorAcademiaFalhaQuandoJaDeletado(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "deletado"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademia, academiaID); err == nil {
		t.Fatal("esperava erro ao deletar estudante já deletado, mas não houve erro")
	}
}

// TestDeletarPorAcademiaFalhaAcademiaDiferente cobre exatamente o caso que
// motivou a Tarefa 98: uma academia diferente da atualmente vinculada nunca
// pode deletar a conta do estudante através deste método, mesmo que informe
// o motivo corretamente. A checagem "foi essa academia que cadastrou o
// estudante originalmente" é uma camada ADICIONAL no handler (ver pacote
// handlers) — este teste cobre apenas o invariante que o aggregate consegue
// verificar sozinho (vínculo ATUAL).
func TestDeletarPorAcademiaFalhaAcademiaDiferente(t *testing.T) {
	codigoAcademiaVinculada := "ACA_01"
	codigoAcademiaSolicitante := "ACA_02"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademiaVinculada
	estudante.Status = "ativo"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademiaSolicitante, academiaID); err == nil {
		t.Fatal("esperava erro quando a academia solicitante não é a atualmente vinculada, mas não houve erro")
	}
	if estudante.Status != "ativo" {
		t.Fatalf("Status mudou apesar do erro: %q", estudante.Status)
	}
}

func TestDeletarPorAcademiaFalhaMotivoVazio(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "ativo"

	if err := estudante.DeletarPorAcademia("   ", codigoAcademia, academiaID); err == nil {
		t.Fatal("esperava erro com motivo vazio/em branco, mas não houve erro")
	}
}

func TestDeletarPorAcademiaFalhaSemVinculoNenhum(t *testing.T) {
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = nil
	estudante.Status = "inativo"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", "ACA_01", academiaID); err == nil {
		t.Fatal("esperava erro quando o estudante nunca teve academia, mas não houve erro")
	}
}

// TestDeletarAutoServicoContinuaExigindoDesvinculacao é um teste de
// regressão de baseline: confirma que Deletar (autodeleção) continua com o
// mesmo comportamento de antes da Tarefa 98 — só o próprio estudante,
// somente quando já desvinculado (Status == "inativo").
func TestDeletarAutoServicoContinuaExigindoDesvinculacao(t *testing.T) {
	proprioID := uuid.New()

	vinculado := NewEstudante()
	vinculado.CodigoEstudante = "EST1234"
	vinculado.Status = "ativo"
	if err := vinculado.Deletar("motivo", proprioID); err == nil {
		t.Fatal("esperava erro ao autodeletar estudante ainda vinculado, mas não houve erro")
	}

	desvinculado := NewEstudante()
	desvinculado.CodigoEstudante = "EST1234"
	desvinculado.Status = "inativo"
	if err := desvinculado.Deletar("motivo", proprioID); err != nil {
		t.Fatalf("Deletar (autodeleção) retornou erro inesperado: %v", err)
	}
	if desvinculado.Status != "deletado" {
		t.Fatalf("Status = %q, want %q", desvinculado.Status, "deletado")
	}
}
```

### 7.3 `internal/handlers/estudante_delecao_academia_integration_test.go` — NOVO ARQUIVO

Testes de integração HTTP (Postgres real) para a rota nova (Ponto 1).

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)

// setupDelecaoAcademiaTestRouter registra a rota de deleção da conta do
// estudante pela academia (Tarefa 98), autenticada como a academia userID.
func setupDelecaoAcademiaTestRouter(client *db.Client, academiaID uuid.UUID) *gin.Engine {
	repository := db.NewAggregateRepository(client)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", client)
		c.Set("repository", repository)
		c.Set("user_id", academiaID)
		c.Set("user_type", "academia")
	})
	router.DELETE("/academia/estudante/:codigo/conta", DeletarContaEstudantePorAcademia)
	return router
}

func deletarContaEstudantePorAcademia(router *gin.Engine, codigoEstudante, motivo string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"motivo": motivo})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/academia/estudante/"+codigoEstudante+"/conta", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

// reintegrarEstudanteEmOutraAcademiaParaTeste simula uma revinculação (ou
// transferência) do estudante para uma academia DIFERENTE da que o
// cadastrou — o mesmo evento (EstudanteReintegrado) usado pela rota real
// POST /estudante/solicitacoes-status/revinculacao/:codigo_academia, que
// aceita qualquer codigo_academia (não necessariamente o original). Depois
// disto, estudanteDTO.CodigoAcademia aponta para a nova academia, mas o
// PRIMEIRO evento do ledger (EstudanteCriadoComVinculo) continua apontando
// para a academia original — é exatamente essa diferença que a Tarefa 98
// precisa respeitar.
func reintegrarEstudanteEmOutraAcademiaParaTeste(t *testing.T, client *db.Client, estID uuid.UUID, novaCodigoAcademia string) {
	t.Helper()
	repository := db.NewAggregateRepository(client)
	agg, err := repository.Load(estID, "Estudante")
	if err != nil {
		t.Fatalf("erro ao carregar estudante para reintegrar: %v", err)
	}
	estAgg := agg.(*aggregates.Estudante)
	if err := estAgg.Reintegrar(novaCodigoAcademia, "fundamental", nil, nil, nil, nil, uuid.New()); err != nil {
		t.Fatalf("erro ao reintegrar estudante de teste em outra academia: %v", err)
	}
	if err := repository.SaveWithAudit(estAgg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("erro ao salvar reintegração: %v", err)
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
	}
}

// TestDeletarContaEstudantePorAcademiaOKQuandoMesmaAcademiaCadastrou cobre o
// caminho feliz da Tarefa 98: a academia que cadastrou o estudante pode
// deletar a conta dele enquanto ele ainda está vinculado a ela (sem exigir
// desvinculação prévia).
func TestDeletarContaEstudantePorAcademiaOKQuandoMesmaAcademiaCadastrou(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)

	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}

	router := setupDelecaoAcademiaTestRouter(client, academia.ID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "estudante pediu remoção dos dados")

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes após deleção: %v", err)
	}

	estAtualizado, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estAtualizado == nil {
		t.Fatalf("erro ao buscar estudante após deleção: %v", err)
	}
	if estAtualizado.Status != "deletado" {
		t.Fatalf("Status = %q, want %q", estAtualizado.Status, "deletado")
	}
}

// TestDeletarContaEstudantePorAcademiaForbiddenQuandoAcademiaSoRecebeuPorRevinculacao
// é o teste central da Tarefa 98: uma academia que recebeu o estudante por
// revinculação (ou transferência) — e por isso está vinculada a ele agora —
// mas NÃO foi quem o cadastrou originalmente, não pode deletar a conta dele.
// Sem a checagem via histórico do ledger, esta academia passaria pela
// checagem ingênua "codigo_academia atual == academia autenticada" e
// conseguiria deletar indevidamente.
func TestDeletarContaEstudantePorAcademiaForbiddenQuandoAcademiaSoRecebeuPorRevinculacao(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademiaOriginal := criarEstudanteVinculadoParaTeste(t, client)
	desvincularEstudanteDeTeste(t, client, estID, codigoAcademiaOriginal)

	codigoAcademiaNova := "IT" + uuid.New().String()[:8]
	academiaNovaID := criarAcademiaEscolarParaTeste(t, client, codigoAcademiaNova, "fundamental", []string{"1_ano_fundamental"})
	reintegrarEstudanteEmOutraAcademiaParaTeste(t, client, estID, codigoAcademiaNova)

	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}
	if est.CodigoAcademia == nil || *est.CodigoAcademia != codigoAcademiaNova {
		t.Fatalf("pré-condição do teste falhou: estudante deveria estar vinculado à nova academia, CodigoAcademia=%v", est.CodigoAcademia)
	}

	router := setupDelecaoAcademiaTestRouter(client, academiaNovaID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "tentativa indevida")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperava 403 (academia não foi quem cadastrou o estudante), recebeu %d: %s", rec.Code, rec.Body.String())
	}

	estAposTentativa, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estAposTentativa == nil {
		t.Fatalf("erro ao buscar estudante após tentativa: %v", err)
	}
	if estAposTentativa.Status == "deletado" {
		t.Fatal("estudante foi deletado indevidamente por academia que não o cadastrou")
	}
}

// TestDeletarContaEstudantePorAcademiaForbiddenQuandoEstudanteNaoPertenceAAcademia
// cobre o caso mais simples: uma academia totalmente alheia (nunca teve
// vínculo com o estudante) não pode deletar a conta dele.
func TestDeletarContaEstudantePorAcademiaForbiddenQuandoEstudanteNaoPertenceAAcademia(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, _ := criarEstudanteVinculadoParaTeste(t, client)
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}

	codigoAcademiaAlheia := "IT" + uuid.New().String()[:8]
	academiaAlheiaID := criarAcademiaEscolarParaTeste(t, client, codigoAcademiaAlheia, "fundamental", []string{"1_ano_fundamental"})
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
	}

	router := setupDelecaoAcademiaTestRouter(client, academiaAlheiaID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "tentativa indevida")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperava 403, recebeu %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDeletarContaEstudantePorAcademiaFalhaQuandoDesvinculado cobre a outra
// metade da regra: mesmo sendo a academia que cadastrou o estudante, ela só
// pode deletar enquanto ele está VINCULADO — depois de desvinculado, a via é
// exclusivamente a autodeleção do próprio estudante (DeletarContaEstudante).
func TestDeletarContaEstudantePorAcademiaFalhaQuandoDesvinculado(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	desvincularEstudanteDeTeste(t, client, estID, codigoAcademia)

	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}

	router := setupDelecaoAcademiaTestRouter(client, academia.ID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "tentativa após desvinculação")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 (estudante já desvinculado), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeletarContaEstudantePorAcademiaMotivoObrigatorio(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}

	router := setupDelecaoAcademiaTestRouter(client, academia.ID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 (motivo obrigatório), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}
```

### 7.4 `internal/handlers/solicitacao_edicao_bi_documento_integration_test.go` — NOVO ARQUIVO

Testes de integração HTTP (Postgres real) para o documento de BI virar oficial (Ponto 2).

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/projections"
	"spuri/internal/storage"
)

// setupSolicitacaoEdicaoBITestRouter registra as rotas de criação (estudante)
// e decisão (academia) da solicitação de edição de bilhete_identidade,
// autenticadas como userID/userType fixos — mesmo padrão dos demais testes
// de integração HTTP deste pacote.
func setupSolicitacaoEdicaoBITestRouter(client *db.Client, userID uuid.UUID, userType string) *gin.Engine {
	repository := db.NewAggregateRepository(client)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", client)
		c.Set("repository", repository)
		c.Set("user_id", userID)
		c.Set("user_type", userType)
		c.Set("storageProvider", storage.NewLocalProvider())
	})
	router.POST("/estudante/solicitacoes-edicao/bilhete-identidade", CriarSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade"))
	router.PUT("/academia/solicitacoes-edicao-estudante/bilhete-identidade/:codigo/aprovar", DecidirSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade", true))
	router.PUT("/academia/solicitacoes-edicao-estudante/bilhete-identidade/:codigo/reprovar", DecidirSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade", false))
	return router
}

func criarSolicitacaoEdicaoBIParaTeste(t *testing.T, client *db.Client, router *gin.Engine, novoValor string, conteudoPDF []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("novo_valor", novoValor); err != nil {
		t.Fatalf("erro ao escrever campo novo_valor: %v", err)
	}
	part, err := createPDFFormFile(w, "documento", "bi.pdf")
	if err != nil {
		t.Fatalf("erro ao criar campo de arquivo: %v", err)
	}
	if _, err := part.Write(conteudoPDF); err != nil {
		t.Fatalf("erro ao escrever PDF de teste: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("erro ao fechar multipart writer: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/estudante/solicitacoes-edicao/bilhete-identidade", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("esperava 201 ao criar solicitação de edição de BI, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		CodigoSolicitacao string `json:"codigo_solicitacao"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao decodificar resposta de criação: %v", err)
	}
	// A criação grava no ledger de forma síncrona (SaveWithAudit), mas a
	// projeção de solicitações só é atualizada pelo projManager assíncrono
	// em produção — nos testes, sem esse worker rodando, precisamos
	// reconstruir explicitamente antes que a academia consiga encontrar a
	// solicitação pelo código (mesmo padrão usado para Estudante/Academia
	// em todo este pacote de testes).
	if err := projections.NewSolicitacaoEdicaoDadoEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de solicitações de edição: %v", err)
	}
	return resp.CodigoSolicitacao
}

func aprovarSolicitacaoEdicaoBI(t *testing.T, client *db.Client, router *gin.Engine, codigoSolicitacao string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/academia/solicitacoes-edicao-estudante/bilhete-identidade/"+codigoSolicitacao+"/aprovar", nil)
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		// Idem: reflete a decisão (status -> aprovada) na projeção antes que
		// o teste prossiga, senão ExistePendente ainda veria a solicitação
		// anterior como pendente numa segunda tentativa de edição.
		if err := projections.NewSolicitacaoEdicaoDadoEstudanteProjection(client).Rebuild(); err != nil {
			t.Fatalf("erro ao reconstruir projeção de solicitações de edição após decisão: %v", err)
		}
	}
	return rec
}

// syncEstudanteProjectionAposEvento processa, via EstudanteProjection.Handle,
// apenas o ÚLTIMO evento do estudante estID — o mesmo que o projManager
// assíncrono processaria em produção depois de um SaveWithAudit. Evitamos
// deliberadamente EstudanteProjection.Rebuild() aqui: Rebuild() é um replay
// GLOBAL de todo o ledger (de qualquer teste que já rodou neste processo) e
// falha se alguma academia referenciada por QUALQUER estudante de QUALQUER
// outro teste de integração deste pacote ainda não estiver projetada — algo
// fora do nosso controle e sem relação com o que este teste verifica.
// Processar só o evento novo é mais preciso (é exatamente o que
// aconteceria em produção) e imune a esse tipo de poluição cruzada entre
// testes que compartilham o mesmo banco.
func syncEstudanteProjectionAposEvento(t *testing.T, client *db.Client, estID uuid.UUID) {
	t.Helper()
	repository := db.NewAggregateRepository(client)
	historico, err := repository.GetEventHistory(estID)
	if err != nil || len(historico) == 0 {
		t.Fatalf("erro ao buscar histórico do estudante para sincronizar projeção: %v", err)
	}
	ultimo := historico[len(historico)-1]
	if err := projections.NewEstudanteProjection(client).Handle(ultimo); err != nil {
		t.Fatalf("erro ao processar último evento (%s) na projeção de estudantes: %v", ultimo.EventType, err)
	}
}

// TestAprovarSolicitacaoEdicaoBIPromoveDocumentoQuandoNaoHaviaNenhum cobre o
// caso central da Tarefa 98: o estudante criado por criarEstudanteVinculadoParaTeste
// NÃO tem nenhum documento em Documentos["bi_estudante"] (só bi_encarregado e
// cedula_estudante). Depois de uma solicitação de edição de BI aprovada, o
// documento anexado à solicitação deve se tornar o documento oficial — "deve
// substituir o atual (mesmo se não tiver um)", como pedido.
func TestAprovarSolicitacaoEdicaoBIPromoveDocumentoQuandoNaoHaviaNenhum(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}
	if _, ok := est.Documentos["bi_estudante"]; ok {
		t.Fatal("pré-condição do teste falhou: estudante já tinha documento bi_estudante")
	}
	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}

	routerEstudante := setupSolicitacaoEdicaoBITestRouter(client, estID, "estudante")
	routerAcademia := setupSolicitacaoEdicaoBITestRouter(client, academia.ID, "academia")

	novoBI := generateBITest()
	codigoSolicitacao := criarSolicitacaoEdicaoBIParaTeste(t, client, routerEstudante, novoBI, minimalPDFBytesForTest())

	rec := aprovarSolicitacaoEdicaoBI(t, client, routerAcademia, codigoSolicitacao)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao aprovar solicitação, recebeu %d: %s", rec.Code, rec.Body.String())
	}

	syncEstudanteProjectionAposEvento(t, client, estID)
	estAtualizado, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estAtualizado == nil {
		t.Fatalf("erro ao buscar estudante após aprovação: %v", err)
	}
	if estAtualizado.BilheteIdentidade == nil || *estAtualizado.BilheteIdentidade != novoBI {
		t.Fatalf("BilheteIdentidade = %v, want %q", estAtualizado.BilheteIdentidade, novoBI)
	}
	doc, ok := estAtualizado.Documentos["bi_estudante"]
	if !ok {
		t.Fatal("Documentos[\"bi_estudante\"] ausente após aprovação — o documento da solicitação não foi promovido a oficial")
	}
	if doc.Path == "" {
		t.Fatal("Documentos[\"bi_estudante\"].Path está vazio")
	}

	// O arquivo tem de existir de fato no caminho definitivo (não só o
	// ponteiro na projeção) — senão um download real falharia.
	provider := storage.NewLocalProvider()
	rc, err := provider.Read(doc.Path)
	if err != nil {
		t.Fatalf("documento oficial do BI não está legível em %q: %v", doc.Path, err)
	}
	_ = rc.Close()
}

// TestAprovarSolicitacaoEdicaoBISubstituiDocumentoAnterior cobre a segunda
// metade da regra: quando JÁ existe um documento oficial de BI, uma nova
// solicitação aprovada substitui (não acumula) — e o documento antigo deixa
// de ser acessível no caminho anterior.
func TestAprovarSolicitacaoEdicaoBISubstituiDocumentoAnterior(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}
	routerEstudante := setupSolicitacaoEdicaoBITestRouter(client, estID, "estudante")
	routerAcademia := setupSolicitacaoEdicaoBITestRouter(client, academia.ID, "academia")

	// Ciclo 1: estabelece o primeiro documento oficial.
	bi1 := generateBITest()
	cod1 := criarSolicitacaoEdicaoBIParaTeste(t, client, routerEstudante, bi1, minimalPDFBytesForTest())
	if rec := aprovarSolicitacaoEdicaoBI(t, client, routerAcademia, cod1); rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 no primeiro ciclo, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	syncEstudanteProjectionAposEvento(t, client, estID)
	estApos1, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estApos1 == nil {
		t.Fatalf("erro ao buscar estudante após ciclo 1: %v", err)
	}
	doc1, ok := estApos1.Documentos["bi_estudante"]
	if !ok || doc1.Path == "" {
		t.Fatal("documento do primeiro ciclo não foi promovido corretamente")
	}

	// Ciclo 2: um segundo PDF diferente (conteúdo maior, para garantir um
	// arquivo distinto) substitui o primeiro.
	bi2 := generateBITest()
	pdf2 := append(minimalPDFBytesForTest(), []byte("\n% segundo documento de teste")...)
	cod2 := criarSolicitacaoEdicaoBIParaTeste(t, client, routerEstudante, bi2, pdf2)
	if rec := aprovarSolicitacaoEdicaoBI(t, client, routerAcademia, cod2); rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 no segundo ciclo, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	syncEstudanteProjectionAposEvento(t, client, estID)
	estApos2, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estApos2 == nil {
		t.Fatalf("erro ao buscar estudante após ciclo 2: %v", err)
	}
	if estApos2.BilheteIdentidade == nil || *estApos2.BilheteIdentidade != bi2 {
		t.Fatalf("BilheteIdentidade = %v, want %q (segundo ciclo)", estApos2.BilheteIdentidade, bi2)
	}
	doc2, ok := estApos2.Documentos["bi_estudante"]
	if !ok || doc2.Path == "" {
		t.Fatal("documento do segundo ciclo não foi promovido corretamente")
	}
	if doc2.Path == doc1.Path {
		t.Fatal("o documento do segundo ciclo deveria ter um caminho diferente do primeiro (substituição, não reaproveitamento)")
	}

	provider := storage.NewLocalProvider()
	if rc, err := provider.Read(doc2.Path); err != nil {
		t.Fatalf("documento oficial atual não está legível em %q: %v", doc2.Path, err)
	} else {
		_ = rc.Close()
	}
	if _, err := provider.Read(doc1.Path); err == nil {
		t.Fatal("documento antigo ainda está acessível após substituição — deveria ter sido removido")
	}
}
```

## 8. Validação já realizada (evidência real, Postgres 16 real)

Ambiente: Go 1.24.4 + PostgreSQL 16 instalados no meu sandbox, todas as 133 migrações do `spuri-backend` (na base atualizada, pós-merge das Tarefas 95/96/97) aplicadas do zero, `STORAGE_PROVIDER=local` (documentos gravados em disco local em vez de Mega, mesma interface `StorageProvider`).

**Baseline antes de qualquer mudança:** suíte completa (`go test -p 1 ./...`, banco recriado do zero) 100% verde.

**Depois de implementar os dois pontos:**
- `go build ./...` e `go vet ./...` limpos.
- Suíte completa pré-existente (banco limpo, `-p 1`) continua 100% verde — nenhuma regressão.
- **Testes novos do Ponto 1** (8 testes de unidade do aggregate + 5 de integração HTTP) — todos passando, de forma determinística, repetido 3x em banco limpo.
- **Testes novos do Ponto 2** (2 testes de integração HTTP, cobrindo tanto "não havia documento nenhum" quanto "substitui documento existente", incluindo verificação de que o arquivo antigo deixa de ser legível via o `StorageProvider` depois da substituição) — todos passando, de forma determinística, repetido 3x em banco limpo.
- **Ciclo reverter→falhar→reaplicar** feito para cada mudança central, confirmando que os testes realmente detectam a ausência da correção (não são só testes "verdes por acaso"):
  1. Removendo a checagem de `codigoAcademiaSolicitante` em `DeletarPorAcademia` → `TestDeletarPorAcademiaFalhaAcademiaDiferente` falha corretamente.
  2. Trocando a checagem via ledger no handler por uma checagem ingênua (só `CodigoAcademia` atual) → `TestDeletarContaEstudantePorAcademiaForbiddenQuandoAcademiaSoRecebeuPorRevinculacao` falha corretamente (prova que uma academia que só recebeu o estudante por revinculação conseguiria deletar indevidamente sem a correção).
  3. Revertendo `AlterarBilheteIdentidadePorSolicitacao` para não receber/gravar o documento (comportamento antigo) → os dois testes do Ponto 2 falham corretamente.
  4. Revertendo só a correção da **projeção** (`handleDadosPessoaisAtualizados`), mantendo o aggregate correto → os dois testes do Ponto 2 **também** falham corretamente — confirma que a correção da projeção (achado não previsto no pedido original, seção 2 item 6) é de fato necessária e está coberta pelos testes, não é redundante.
  Em todos os 4 casos, o arquivo foi restaurado ao estado correto logo em seguida e a suíte voltou a passar.

## 9. Por que não peço para repetir a validação com Postgres real

O ambiente do Codex bloqueia `apt` (403 Forbidden) e não tem Docker nem `psql` — não é possível instalar PostgreSQL nem rodar os testes de integração (`*_integration_test.go`, que exigem `DATABASE_URL` real) nesse ambiente. Isso já foi feito por mim, com evidência na seção 8. **Não tente contornar isso** (ex. mockar o banco, pular os testes de integração do código-fonte, ou marcá-los como `t.Skip` incondicional) — os arquivos de teste devem ser criados exatamente como estão na seção 7, com a mesma lógica de `if os.Getenv("DATABASE_URL") == "" { t.Skip(...) }` que os demais testes de integração do pacote já usam (herdada automaticamente de `nivelEscolarTestClient`, que já implementa esse skip).

## 10. Comandos de validação para o Codex rodar

```bash
cd spuri-backend
go build ./...
go vet ./...
go test ./...          # sem DATABASE_URL definido, os testes de integração são pulados automaticamente
```

Os três comandos devem terminar sem erro. Isso cobre 100% do que é executável no ambiente do Codex — os testes de integração novos (seção 7.3 e 7.4) e os de unidade novos (seção 7.2) já foram validados por mim contra Postgres real (seção 8) e não precisam rodar de novo aqui.

## 11. Achado não relacionado — NÃO corrigir como parte desta tarefa

Durante a validação, descobri que a suíte de testes de integração deste pacote (`internal/handlers`) tem uma fragilidade pré-existente e **completamente não relacionada** a esta tarefa: alguns testes financeiros (`internal/handlers/financeiro_handlers_integration_test.go`, `financeiro_pendencias_handlers_test.go`, `financeiro_remocao_handlers_integration_test.go`) inserem uma academia de teste **diretamente via SQL** (`INSERT INTO projection_academias ...`), sem nunca gravar o evento `AcademiaCriada` correspondente no ledger. Isso é inofensivo *até* que outro teste, rodando depois na mesma execução do `go test` (mesmo processo, banco compartilhado), chame um `Rebuild()` global de projeção (`EstudanteProjection.Rebuild()` ou `AcademiaProjection.Rebuild()` truncam a tabela inteira e reconstroem só a partir do ledger) — nesse momento, a academia "fantasma" desaparece da projeção porque não existe nenhum evento no ledger para recriá-la, e qualquer estudante de teste vinculado a ela (criado com um evento real) fica com um vínculo "órfão", travando o rebuild com um erro do tipo `academia 'WH...' ainda não disponível na projeção — retry automático`.

Isso **não é causado pelos meus testes novos** — é uma característica da suíte que já existia antes desta tarefa e que qualquer teste novo que use o helper `criarEstudanteVinculadoParaTeste` corre o risco de expor, dependendo puramente da ordem alfabética de execução dos arquivos de teste. Os testes novos desta tarefa (seção 7.3 e 7.4) já foram escritos de um jeito deliberadamente imune a isso: usam `EstudanteProjection.Handle(evento)` (processamento direcionado do último evento, o mesmo que o worker assíncrono de produção faria) em vez de `Rebuild()` global — por isso passam de forma 100% determinística tanto isolados quanto junto com o resto do pacote.

**Não tente corrigir isso como parte da Tarefa 98** — é fora de escopo, não foi pedido, e mexer nos fixtures dos testes financeiros é um trabalho maior e separado. Só deixo documentado aqui para não ser motivo de confusão se aparecer de novo no futuro (ex. `go test ./internal/handlers/... -count=1` sem filtro, rodado numa ordem de arquivos que exponha o problema). Se quiser corrigir eventualmente, a forma mais simples seria esses testes financeiros também gravarem um evento `AcademiaCriada` real (via `Academia.Criar` + `SaveWithAudit`) em vez de inserir a linha da projeção diretamente.

## 12. Checklist de aceite

- [ ] `cmd/server/main.go`: rota `DELETE /academia/estudante/:codigo/conta` registrada (seção 6.1).
- [ ] `internal/domain/aggregates/estudante.go`: `DeletarPorAcademia` implementado; `AlterarBilheteIdentidadePorSolicitacao` recebe o novo parâmetro `documento *DocumentoMatricula`; `DadosPessoaisAtualizadosEvent` e `BilheteIdentidadeEstudanteAlteradoPorSolicitacaoEvent` com o campo `DocumentoBI`; `applyDadosPessoaisAtualizados` grava em `Documentos["bi_estudante"]` (seção 6.2).
- [ ] `internal/handlers/auditoria_delecoes_handler.go`: resolve `deletado_por_tipo`/`deletado_por_nome`/`deletado_por_codigo_academia` (seção 6.3).
- [ ] `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go`: `aplicarEdicaoAprovada` promove o documento de BI; novas funções `promoverDocumentoBIParaOficial` e `limparDocumentoBIOrfao` (seção 6.4).
- [ ] `internal/projections/estudante_projection.go`: `handleDadosPessoaisAtualizados` grava `DocumentoBI` na coluna `documentos` via merge JSONB (seção 6.5).
- [ ] 4 novos arquivos criados exatamente como na seção 7.
- [ ] `go build ./...` sem erro.
- [ ] `go vet ./...` sem erro.
- [ ] `go test ./...` (sem `DATABASE_URL`) sem erro — todos os testes de integração são pulados automaticamente, só os de unidade (incluindo os 8 novos de `DeletarPorAcademia`) rodam e passam.
- [ ] Nenhuma migration nova criada (não é necessária — reaproveita colunas e eventos existentes).
- [ ] Nenhuma entrada nova em `internal/db/safe_queries.go` (não é necessária — reaproveita o evento `EstudanteDeletado` existente).
- [ ] `internal/storage/storage.go` **não foi tocado**.

## 13. Procedimento de conclusão

1. Rodar os comandos da seção 10 e confirmar que passam.
2. Fazer commit das mudanças.
3. Mover este arquivo de `docs/Lista de Tarefas/98 - ...md` para `docs/Tarefas feitas/98 - ...md`.
4. Atualizar o front-matter deste arquivo: `status: feito`.
