---
criado: 2026-09-12
origem: Claude (orquestrador) — diagnóstico e correção pré-validados com PostgreSQL real antes de virar tarefa
status: concluída
repositório: https://github.com/fredypdp/rastreio-backend
branch base: main
---

# Corrigir status `pendente_documentos` não atualizado após edição de BI aprovada

## Leia isto antes de tudo

Este documento já contém o diagnóstico completo, o código exato da correção e o
resultado de testes reais (PostgreSQL de verdade, não mock). **Não é necessário
investigar a causa, planejar a abordagem, nem decidir a implementação — tudo
isso já foi feito e validado.** A tarefa é executar os passos abaixo, na ordem,
e confirmar com os comandos de verificação da seção 5.

Se em algum passo o trecho de código "antes" não bater 100% com o que está no
arquivo real (por exemplo, porque `main` mudou desde que este documento foi
escrito), **pare e reporte a diferença** em vez de tentar adaptar por conta
própria — isso pode indicar que outra mudança já mexeu na mesma área.

### Nota sobre o ambiente do Codex (leia com atenção)

Este documento foi escrito por um outro agente (Claude), que tem um sandbox
com acesso a `apt`, Docker e conseguiu instalar e rodar um PostgreSQL real
para validar a correção fim a fim, com dados reais, antes de escrever esta
tarefa. **O ambiente do Codex não tem isso** (`apt` retorna 403 Forbidden, sem
Docker, sem `psql`) — e está tudo bem, porque essa parte já foi feita.

O que o Codex **deve** fazer no seu ambiente:
- Aplicar as mudanças (seção 4).
- Rodar `go build ./...` e `go vet ./...` normalmente (usando o toolchain Go
  normal do projeto — não é necessário nenhum truque de versão; isso só foi
  necessário no sandbox do Claude por uma limitação específica dele).
- Rodar `go test ./...`. Os testes que dependem de banco de dados (todos os
  arquivos `*_integration_test.go`) vão aparecer como `--- SKIP` porque a
  variável de ambiente `DATABASE_URL` não estará definida — **isso é o
  comportamento esperado e correto**, não é uma falha. Não tente contornar
  isso instalando Postgres, Docker, ou subindo algum serviço — vai bater no
  bloqueio de rede/apt e é desnecessário: a suíte inteira (incluindo esses
  mesmos testes de integração) já rodou com sucesso contra um Postgres real,
  do zero, com o patch aplicado (ver seção 3).

O que o Codex **não deve** fazer:
- Não tente instalar PostgreSQL, Docker, ou qualquer pacote via `apt`.
- Não tente rodar migrations ou subir um banco.
- Não marque os testes `SKIP`-ados como pendência ou problema.

---

## 1. Diagnóstico

### 1.1 Bug relatado: status `pendente_documentos` não atualiza após edição de BI

Fluxo: um estudante com `Status = "pendente_documentos"` (falta pelo menos um
documento obrigatório de matrícula) solicita a edição do seu bilhete de
identidade (BI), anexando o número novo e o PDF do documento. A academia
aprova. O número e o documento são atualizados corretamente — mas o `Status`
do estudante permanece `"pendente_documentos"` para sempre, mesmo que esse
documento fosse exatamente a única pendência.

**Causa raiz:** em
`internal/handlers/solicitacao_edicao_dado_estudante_handlers.go`, função
`aplicarEdicaoAprovada`, a aprovação de uma edição de BI chama
`agg.AlterarBilheteIdentidadePorSolicitacao(...)`
(`internal/domain/aggregates/estudante.go`), que dispara o evento
`BilheteIdentidadeEstudanteAlteradoPorSolicitacao`. Esse evento é tratado por
`applyDadosPessoaisAtualizados`, que atualiza `BilheteIdentidade` e
`Documentos["bi_estudante"]` — mas **nunca verifica nem atualiza `Status`**.

A única função que sabe fazer essa verificação e transição
(`CompletarDocumentosPendentes`, também em `estudante.go`) só é chamada pelo
endpoint separado de upload avulso pela academia
(`CompletarDocumentosEstudantePendente`), nunca pelo fluxo de aprovação de
edição.

A criação da solicitação já permite explicitamente estudantes
`pendente_documentos` (`solicitacao_edicao_dado_estudante_handlers.go`, linha
~46: `est.Status != "ativo" && est.Status != "pendente_documentos"` →
erro), confirmando que esse cruzamento de fluxos é esperado pelo design —
só nunca foi fechado.

### 1.2 Segundo gap, no mesmo processo: documento do BI do encarregado nunca era promovido a oficial

Ao investigar o processo completo, foi encontrado um problema irmão: quando a
academia aprova uma edição de `bilhete_identidade_encarregado` (BI do
responsável), o handler chamava
`agg.AlterarBilheteIdentidadeEncarregadoPorSolicitacao(...)` **sem nunca
passar o documento anexado à solicitação**. Apenas o número era gravado — o
PDF enviado pelo estudante era descartado junto com o arquivo temporário da
solicitação, e `Documentos["bi_encarregado"]` nunca era preenchido por essa
via (diferente do que já acontecia, corretamente, para o BI do próprio
estudante desde a Tarefa 98).

Isso tem duas consequências:
- Um estudante nunca conseguia usar essa via para satisfazer a pendência de
  `bi_encarregado` exigida por `ValidarDocumentosMatricula` quando
  `bilhete_identidade_encarregado` está preenchido.
- Havia uma segunda camada com o mesmo tipo de lacuna: além do aggregate
  (`estudante.go`), a projeção de leitura
  (`internal/projections/estudante_projection.go`, função
  `handleDadosPessoaisAtualizados`) tem sua **própria implementação
  independente** de parse do evento e geração do `UPDATE` SQL — e ela também
  só sabia lidar com o documento do BI do estudante, não do encarregado.

Este documento cobre a correção **dos dois problemas juntos**, porque a
correção do primeiro (seção 1.1) só funciona de verdade para o BI do
encarregado se o documento realmente for promovido a oficial — são a mesma
tarefa, em dois campos irmãos.

---

## 2. O que foi corrigido, e onde

| # | Arquivo | O que muda |
|---|---|---|
| 1 | `internal/domain/aggregates/estudante.go` | `AlterarBilheteIdentidadeEncarregadoPorSolicitacao` passa a aceitar um `documento *DocumentoMatricula` opcional (mesmo padrão já usado em `AlterarBilheteIdentidadePorSolicitacao`); novo campo `DocumentoBIEncarregado` nos eventos `DadosPessoaisAtualizadosEvent` e `BilheteIdentidadeEncarregadoAlteradoPorSolicitacaoEvent`; `applyDadosPessoaisAtualizados` grava esse documento em `Documentos["bi_encarregado"]`. |
| 2 | `internal/projections/estudante_projection.go` | `handleDadosPessoaisAtualizados` passa a reconhecer `DocumentoBIEncarregado` no payload e atualizar a coluna `documentos` da projeção de leitura (mesmo padrão do `DocumentoBI` já existente, combinando os dois num único `SET` para nunca gerar duas atribuições à mesma coluna). |
| 3 | `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go` | `promoverDocumentoBIParaOficial` generalizada para aceitar o tipo de documento (`"bi_estudante"` ou `"bi_encarregado"`); `aplicarEdicaoAprovada` passa a promover também o documento do BI do encarregado quando o campo editado é `bilhete_identidade_encarregado`; e — o núcleo do bug relatado — depois de aplicar qualquer campo editado, se `Status == "pendente_documentos"`, tenta `CompletarDocumentosPendentes` com um mapa vazio (reaproveita os documentos já no aggregate); se completar, o estudante passa a `"ativo"` no mesmo `SaveWithAudit`; se ainda faltar algo, é ignorado silenciosamente (só log) e o estudante continua pendente normalmente. |
| 4 | `internal/handlers/solicitacao_edicao_bi_documento_integration_test.go` | Dois testes de integração novos, com banco real, cobrindo exatamente os dois bugs acima (ver seção 3). |

---

## 3. Validação já realizada (PostgreSQL real, não mock)

Isto **já foi executado com sucesso** pelo Claude antes de escrever esta
tarefa. O Codex não precisa refazer nada disto — é só para dar confiança de
que a correção funciona de ponta a ponta antes de aplicá-la.

**Ambiente de validação:** PostgreSQL 16 real, as 133 migrations do projeto
aplicadas do zero, e depois **reaplicadas novamente do zero num clone limpo
do `main`** com o patch já aplicado (para eliminar qualquer dúvida de que a
validação refletia exatamente o que está descrito aqui).

**Passos executados e resultado:**

1. Reproduzido o bug relatado (seção 1.1) com um teste de integração que cria
   um estudante `pendente_documentos` cuja única pendência é o documento do
   BI do estudante, aprova uma edição de BI e verifica o status final →
   **falhou antes da correção**, exatamente como esperado (`Status =
   "pendente_documentos"`, esperado `"ativo"`).
2. Aplicada a correção da seção 1.1 → o mesmo teste passou a **passar**.
3. Durante a extensão para o BI do encarregado (seção 1.2), um teste
   simétrico revelou a segunda camada do problema (a projeção de leitura) —
   corrigida, e o teste passou a **passar**.
4. Suíte completa do projeto (`go test ./...`, todos os pacotes) rodada
   **três vezes** ao longo do processo (após cada mudança relevante) contra
   PostgreSQL real → **sempre 100% verde, zero regressões**.
5. Validação final, do zero, num clone novo do `main` com o patch aplicado
   via `git apply`, banco novo, migrations do zero:

```
=== TESTES (banco novo, patch aplicado do zero) ===
ok  	spuri/cmd/server	0.018s
ok  	spuri/internal/db	0.048s
?   	spuri/internal/domain	[no test files]
ok  	spuri/internal/domain/aggregates	0.014s
ok  	spuri/internal/finance	0.009s
ok  	spuri/internal/handlers	1.857s
?   	spuri/internal/jobs	[no test files]
ok  	spuri/internal/middleware	0.008s
?   	spuri/internal/monitoring	[no test files]
ok  	spuri/internal/projections	0.007s
ok  	spuri/internal/services	0.012s
ok  	spuri/internal/storage	0.003s
ok  	spuri/internal/utils	0.006s
```

Os quatro testes centrais desta tarefa, isolados:

```
=== RUN   TestAprovarSolicitacaoEdicaoBIPromoveDocumentoQuandoNaoHaviaNenhum
--- PASS: TestAprovarSolicitacaoEdicaoBIPromoveDocumentoQuandoNaoHaviaNenhum (0.13s)
=== RUN   TestAprovarSolicitacaoEdicaoBISubstituiDocumentoAnterior
--- PASS: TestAprovarSolicitacaoEdicaoBISubstituiDocumentoAnterior (0.16s)
=== RUN   TestAprovarSolicitacaoEdicaoBICompletaDocumentosPendentes
--- PASS: TestAprovarSolicitacaoEdicaoBICompletaDocumentosPendentes (0.13s)
=== RUN   TestAprovarSolicitacaoEdicaoBIEncarregadoPromoveDocumentoECompletaPendencia
--- PASS: TestAprovarSolicitacaoEdicaoBIEncarregadoPromoveDocumentoECompletaPendencia (0.14s)
PASS
ok  	spuri/internal/handlers	0.583s
```

Os dois primeiros já existiam (Tarefa 98) e continuam passando sem alteração
— confirma que nada foi quebrado. Os dois últimos são os testes de
regressão novos desta tarefa.

`gofmt -l` e `go vet ./...` também rodados, sem apontamentos.

**Conclusão da validação: resultado positivo, sem ressalvas.** A correção
pode ser aplicada como está.

---

## 4. Aplicar a correção

### 4.1 Opção recomendada: aplicar o patch diretamente

Salve o conteúdo abaixo em um arquivo (por exemplo `bi-pendente-documentos.patch`)
na raiz do repositório e aplique com `git apply bi-pendente-documentos.patch`.
O patch foi gerado e testado (`git apply --check`) contra um clone limpo e
atual de `main` — deve aplicar sem conflitos.

```diff
diff --git a/internal/domain/aggregates/estudante.go b/internal/domain/aggregates/estudante.go
index 4006c94..88c3fea 100644
--- a/internal/domain/aggregates/estudante.go
+++ b/internal/domain/aggregates/estudante.go
@@ -315,6 +315,10 @@ type DadosPessoaisAtualizadosEvent struct {
 	// BilheteIdentidadeEstudanteAlteradoPorSolicitacao com documento anexo.
 	// Ver applyDadosPessoaisAtualizados.
 	DocumentoBI *DocumentoMatricula
+	// DocumentoBIEncarregado: presente somente quando o evento de origem é
+	// BilheteIdentidadeEncarregadoAlteradoPorSolicitacao com documento
+	// anexo. Ver applyDadosPessoaisAtualizados.
+	DocumentoBIEncarregado *DocumentoMatricula
 }
 
 func (e *DadosPessoaisAtualizadosEvent) GetPayload() interface{} { return e }
@@ -1281,6 +1285,15 @@ func (e *Estudante) applyDadosPessoaisAtualizados(event DomainEvent) error {
 		}
 		e.Documentos["bi_estudante"] = *ev.DocumentoBI
 	}
+	// Documento do BI do encarregado anexado a uma solicitação de edição de
+	// bilhete_identidade_encarregado aprovada — mesmo tratamento do BI do
+	// estudante acima, mas para Documentos["bi_encarregado"].
+	if ev.DocumentoBIEncarregado != nil {
+		if e.Documentos == nil {
+			e.Documentos = map[string]DocumentoMatricula{}
+		}
+		e.Documentos["bi_encarregado"] = *ev.DocumentoBIEncarregado
+	}
 	return nil
 }
 
@@ -1386,6 +1399,7 @@ func (e *Estudante) AlterarNomePorSolicitacao(novo, codigoSolicitacao, decididoP
 	e.RaiseEvent(ev)
 	return e.Apply(ev)
 }
+
 // AlterarBilheteIdentidadePorSolicitacao altera o BI do estudante após
 // aprovação da academia. documento é opcional (pode ser nil): quando
 // presente (Tarefa 98), é o documento anexado à solicitação de edição, que
@@ -1405,12 +1419,23 @@ func (e *Estudante) AlterarBilheteIdentidadePorSolicitacao(novo, codigoSolicitac
 	e.RaiseEvent(ev)
 	return e.Apply(ev)
 }
-func (e *Estudante) AlterarBilheteIdentidadeEncarregadoPorSolicitacao(novo, codigoSolicitacao, decididoPor string) error {
+
+// AlterarBilheteIdentidadeEncarregadoPorSolicitacao altera o BI do
+// encarregado (responsável) após aprovação da academia. documento é
+// opcional (pode ser nil): quando presente, é o documento anexado à
+// solicitação de edição, que passa a ser o documento oficial do BI do
+// encarregado (Estudante.Documentos["bi_encarregado"]) — substituindo o
+// anterior, mesmo que não houvesse nenhum documento registrado ainda. Quem
+// monta esse DocumentoMatricula (promovendo o arquivo temporário da
+// solicitação para o caminho definitivo no storage) é o handler
+// (handlers.aplicarEdicaoAprovada), não este método — o aggregate só grava
+// o que recebe.
+func (e *Estudante) AlterarBilheteIdentidadeEncarregadoPorSolicitacao(novo, codigoSolicitacao, decididoPor string, documento *DocumentoMatricula) error {
 	v := strings.TrimSpace(novo)
 	if v == "" {
 		return fmt.Errorf("bilhete_identidade_encarregado é obrigatório")
 	}
-	ev := &BilheteIdentidadeEncarregadoAlteradoPorSolicitacaoEvent{BaseEvent: BaseEvent{EventType: "BilheteIdentidadeEncarregadoAlteradoPorSolicitacao", AggregateID: e.ID}, BilheteIdentidadeResp: &v, CodigoSolicitacao: codigoSolicitacao, DecididoPor: decididoPor, UpdatedAt: time.Now()}
+	ev := &BilheteIdentidadeEncarregadoAlteradoPorSolicitacaoEvent{BaseEvent: BaseEvent{EventType: "BilheteIdentidadeEncarregadoAlteradoPorSolicitacao", AggregateID: e.ID}, BilheteIdentidadeResp: &v, CodigoSolicitacao: codigoSolicitacao, DecididoPor: decididoPor, UpdatedAt: time.Now(), DocumentoBIEncarregado: documento}
 	e.RaiseEvent(ev)
 	return e.Apply(ev)
 }
@@ -1500,6 +1525,11 @@ type BilheteIdentidadeEncarregadoAlteradoPorSolicitacaoEvent struct {
 	BilheteIdentidadeResp          *string
 	CodigoSolicitacao, DecididoPor string
 	UpdatedAt                      time.Time
+	// DocumentoBIEncarregado é opcional: quando presente, substitui
+	// Estudante.Documentos["bi_encarregado"] pelo documento anexado à
+	// solicitação de edição aprovada — mesmo que não houvesse nenhum
+	// documento registrado ainda. Ver applyDadosPessoaisAtualizados.
+	DocumentoBIEncarregado *DocumentoMatricula
 }
 
 func (e *BilheteIdentidadeEncarregadoAlteradoPorSolicitacaoEvent) GetPayload() interface{} { return e }
diff --git a/internal/handlers/solicitacao_edicao_dado_estudante_handlers.go b/internal/handlers/solicitacao_edicao_dado_estudante_handlers.go
index f3468fa..826686a 100644
--- a/internal/handlers/solicitacao_edicao_dado_estudante_handlers.go
+++ b/internal/handlers/solicitacao_edicao_dado_estudante_handlers.go
@@ -13,6 +13,7 @@ import (
 	"time"
 
 	"github.com/gin-gonic/gin"
+	"github.com/google/uuid"
 
 	"spuri/internal/db"
 	"spuri/internal/domain/aggregates"
@@ -231,7 +232,7 @@ func aplicarEdicaoAprovada(c *gin.Context, sol *projections.SolicitacaoEdicaoDad
 	var tinhaDocAntigoBI bool
 	if sol.Campo == aggregates.CampoEdicaoBI {
 		docAntigoBI, tinhaDocAntigoBI = agg.Documentos["bi_estudante"]
-		doc, err := promoverDocumentoBIParaOficial(c, sol.CodigoAcademia, sol.CodigoEstudante, sol.DocumentoTemporarioPath)
+		doc, err := promoverDocumentoBIParaOficial(c, sol.CodigoAcademia, sol.CodigoEstudante, sol.DocumentoTemporarioPath, "bi_estudante")
 		if err != nil {
 			utils.RespondWithInternalError(c, fmt.Errorf("falha ao promover documento do bilhete de identidade: %w", err))
 			return err
@@ -239,31 +240,78 @@ func aplicarEdicaoAprovada(c *gin.Context, sol *projections.SolicitacaoEdicaoDad
 		documentoBI = doc
 	}
 
+	// Mesma lógica do bloco acima, mas para o documento do BI do
+	// ENCARREGADO (responsável): antes desta correção, o documento anexado
+	// à solicitação de bilhete_identidade_encarregado nunca era promovido a
+	// oficial — só o número era gravado (AlterarBilheteIdentidadeEncarregado-
+	// PorSolicitacao), e o PDF enviado era descartado junto com o arquivo
+	// temporário da solicitação (ver limpeza de DocumentoTemporarioPath em
+	// DecidirSolicitacaoEdicaoDadoEstudanteHandler). Isso deixava
+	// Documentos["bi_encarregado"] sempre ausente por essa via, mesmo
+	// quando essa era a única pendência de documentos do estudante.
+	var documentoBIEncarregado *aggregates.DocumentoMatricula
+	var docAntigoBIEncarregado aggregates.DocumentoMatricula
+	var tinhaDocAntigoBIEncarregado bool
+	if sol.Campo == aggregates.CampoEdicaoBIEncarregado {
+		docAntigoBIEncarregado, tinhaDocAntigoBIEncarregado = agg.Documentos["bi_encarregado"]
+		doc, err := promoverDocumentoBIParaOficial(c, sol.CodigoAcademia, sol.CodigoEstudante, sol.DocumentoTemporarioPath, "bi_encarregado")
+		if err != nil {
+			utils.RespondWithInternalError(c, fmt.Errorf("falha ao promover documento do bilhete de identidade do encarregado: %w", err))
+			return err
+		}
+		documentoBIEncarregado = doc
+	}
+
 	switch sol.Campo {
 	case aggregates.CampoEdicaoNome:
 		err = agg.AlterarNomePorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor)
 	case aggregates.CampoEdicaoBI:
 		err = agg.AlterarBilheteIdentidadePorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor, documentoBI)
 	case aggregates.CampoEdicaoBIEncarregado:
-		err = agg.AlterarBilheteIdentidadeEncarregadoPorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor)
+		err = agg.AlterarBilheteIdentidadeEncarregadoPorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor, documentoBIEncarregado)
 	case aggregates.CampoEdicaoDataNascimento:
 		dt, _ := time.Parse("2006-01-02", sol.ValorSolicitado)
 		err = agg.AlterarDataNascimentoPorSolicitacao(dt, sol.CodigoSolicitacao, decididoPor)
 	}
 	if err != nil {
 		limparDocumentoBIOrfao(c, documentoBI, sol.CodigoSolicitacao, "erro de validação")
+		limparDocumentoBIOrfao(c, documentoBIEncarregado, sol.CodigoSolicitacao, "erro de validação")
 		utils.RespondWithValidationError(c, err)
 		return err
 	}
+	// Bug: uma edição aprovada (ex.: bilhete_identidade ou bilhete_identidade_
+	// encarregado, cujo documento acaba de ser promovido a oficial acima)
+	// pode ser exatamente a pendência que faltava para um estudante
+	// 'pendente_documentos'. Sem este passo, o
+	// campo/documento era atualizado mas o status ficava 'pendente_documentos'
+	// para sempre, mesmo com todos os documentos obrigatórios completos.
+	// CompletarDocumentosPendentes já faz a verificação correta (só ativa se
+	// TODOS os documentos exigidos por ValidarDocumentosMatricula estiverem
+	// presentes) e é seguro chamar com um mapa vazio: reutiliza agg.Documentos
+	// (já com o documento desta edição, se houver) sem sobrescrever nada. Se
+	// ainda faltar algo, o método retorna erro e o estudante simplesmente
+	// continua pendente — não é uma falha da aprovação em si, por isso o erro
+	// é ignorado aqui (apenas logado) e não interrompe o fluxo.
+	if agg.Status == "pendente_documentos" {
+		if academiaUUID, parseErr := uuid.Parse(decididoPor); parseErr == nil {
+			if err := agg.CompletarDocumentosPendentes(map[string]aggregates.DocumentoMatricula{}, academiaUUID); err != nil {
+				log.Printf("[INFO] estudante %s segue pendente_documentos após edição de %s: %v", sol.CodigoEstudante, sol.Campo, err)
+			}
+		} else {
+			log.Printf("[WARN] não foi possível verificar conclusão de documentos pendentes do estudante %s: decididoPor %q inválido: %v", sol.CodigoEstudante, decididoPor, parseErr)
+		}
+	}
 	audit := db.AuditContext{UserID: decididoPor, UserType: "academia", IP: c.ClientIP()}
 	if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
 		limparDocumentoBIOrfao(c, documentoBI, sol.CodigoSolicitacao, "erro ao salvar solicitação")
+		limparDocumentoBIOrfao(c, documentoBIEncarregado, sol.CodigoSolicitacao, "erro ao salvar solicitação")
 		utils.RespondWithInternalError(c, err)
 		return err
 	}
-	// Só remove o documento de BI anterior DEPOIS do evento confirmado no
-	// ledger — se o SaveWithAudit acima tivesse falhado, o documento antigo
-	// continuaria sendo o oficial (estado consistente para nova tentativa).
+	// Só remove o documento de BI (do estudante ou do encarregado) anterior
+	// DEPOIS do evento confirmado no ledger — se o SaveWithAudit acima
+	// tivesse falhado, o documento antigo continuaria sendo o oficial
+	// (estado consistente para nova tentativa).
 	if sol.Campo == aggregates.CampoEdicaoBI && tinhaDocAntigoBI && strings.TrimSpace(docAntigoBI.Path) != "" && docAntigoBI.Path != documentoBI.Path {
 		if p := getStorageProvider(c); p != nil {
 			if delErr := p.Delete(docAntigoBI.Path); delErr != nil {
@@ -271,18 +319,26 @@ func aplicarEdicaoAprovada(c *gin.Context, sol *projections.SolicitacaoEdicaoDad
 			}
 		}
 	}
+	if sol.Campo == aggregates.CampoEdicaoBIEncarregado && tinhaDocAntigoBIEncarregado && strings.TrimSpace(docAntigoBIEncarregado.Path) != "" && docAntigoBIEncarregado.Path != documentoBIEncarregado.Path {
+		if p := getStorageProvider(c); p != nil {
+			if delErr := p.Delete(docAntigoBIEncarregado.Path); delErr != nil {
+				log.Printf("[WARN] falha ao remover documento de BI do encarregado anterior %s do estudante %s: %v", docAntigoBIEncarregado.Path, sol.CodigoEstudante, delErr)
+			}
+		}
+	}
 	return nil
 }
 
 // promoverDocumentoBIParaOficial copia (Read + Upload) o documento temporário
-// de uma solicitação de edição de bilhete_identidade para o caminho
-// definitivo dos documentos de identificação do estudante — o mesmo padrão
-// usado no cadastro (ver storagePathDocumentoEstudante), com um DocumentoID
-// novo. Copia em vez de mover para que o documento temporário original só
-// seja removido depois de o evento de aprovação ser gravado com sucesso (ver
-// aplicarEdicaoAprovada); assim, uma falha após a cópia não perde o arquivo
-// original nem deixa o estudante sem documento de BI.
-func promoverDocumentoBIParaOficial(c *gin.Context, codigoAcademia, codigoEstudante, tempPath string) (*aggregates.DocumentoMatricula, error) {
+// de uma solicitação de edição de bilhete_identidade (do estudante ou do
+// encarregado, conforme tipoDocumento: "bi_estudante" ou "bi_encarregado")
+// para o caminho definitivo dos documentos de identificação do estudante —
+// o mesmo padrão usado no cadastro (ver storagePathDocumentoEstudante), com
+// um DocumentoID novo. Copia em vez de mover para que o documento temporário
+// original só seja removido depois de o evento de aprovação ser gravado com
+// sucesso (ver aplicarEdicaoAprovada); assim, uma falha após a cópia não
+// perde o arquivo original nem deixa o estudante sem o documento de BI.
+func promoverDocumentoBIParaOficial(c *gin.Context, codigoAcademia, codigoEstudante, tempPath, tipoDocumento string) (*aggregates.DocumentoMatricula, error) {
 	provider := getStorageProvider(c)
 	if provider == nil {
 		return nil, fmt.Errorf("storage não configurado")
@@ -296,8 +352,8 @@ func promoverDocumentoBIParaOficial(c *gin.Context, codigoAcademia, codigoEstuda
 	if err != nil {
 		return nil, fmt.Errorf("falha ao ler documento temporário: %w", err)
 	}
-	downloadURL := estudanteDocumentoDownloadURL(codigoEstudante, "bi_estudante")
-	_, doc := documentoMatriculaNormalizado("bi_estudante", "", downloadURL, "", "")
+	downloadURL := estudanteDocumentoDownloadURL(codigoEstudante, tipoDocumento)
+	_, doc := documentoMatriculaNormalizado(tipoDocumento, "", downloadURL, "", "")
 	destPath := fmt.Sprintf("%s/estudantes/%s/documentos/identificacao/%s/%s.pdf", codigoAcademia, codigoEstudante, doc.Tipo, doc.DocumentoID)
 	stored, err := provider.Upload(destPath, bytes.NewReader(data), int64(len(data)))
 	if err != nil {
diff --git a/internal/projections/estudante_projection.go b/internal/projections/estudante_projection.go
index b347155..b5642a7 100644
--- a/internal/projections/estudante_projection.go
+++ b/internal/projections/estudante_projection.go
@@ -674,6 +674,11 @@ func (p *EstudanteProjection) handleDadosPessoaisAtualizados(event db.Event) err
 		// documento anexo — substitui Documentos["bi_estudante"] na
 		// projeção, espelhando applyDadosPessoaisAtualizados no aggregate.
 		DocumentoBI *aggregates.DocumentoMatricula `json:"DocumentoBI"`
+		// DocumentoBIEncarregado: presente somente quando o evento de
+		// origem é BilheteIdentidadeEncarregadoAlteradoPorSolicitacao com
+		// documento anexo — substitui Documentos["bi_encarregado"] na
+		// projeção, espelhando applyDadosPessoaisAtualizados no aggregate.
+		DocumentoBIEncarregado *aggregates.DocumentoMatricula `json:"DocumentoBIEncarregado"`
 	}
 	if err := json.Unmarshal(event.Payload, &payload); err != nil {
 		return fmt.Errorf("handleDadosPessoaisAtualizados: parse error: %w", err)
@@ -727,15 +732,34 @@ func (p *EstudanteProjection) handleDadosPessoaisAtualizados(event db.Event) err
 		args = append(args, *payload.DataNascimento)
 		idx++
 	}
+	// DocumentoBI e DocumentoBIEncarregado, quando presentes, acumulam-se
+	// numa ÚNICA expressão/atribuição à coluna documentos (em vez de dois
+	// setClauses independentes) — um UPDATE não pode ter duas atribuições
+	// separadas à mesma coluna. Na prática os dois nunca vêm preenchidos no
+	// mesmo evento hoje (vêm de solicitações de campos diferentes), mas a
+	// combinação abaixo é segura mesmo se isso mudar no futuro.
+	documentosExpr := "documentos"
 	if payload.DocumentoBI != nil {
 		docJSON, err := json.Marshal(payload.DocumentoBI)
 		if err != nil {
 			return fmt.Errorf("handleDadosPessoaisAtualizados: falha ao serializar DocumentoBI: %w", err)
 		}
-		setClauses = append(setClauses, fmt.Sprintf("documentos = documentos || jsonb_build_object('bi_estudante', $%d::jsonb)", idx))
+		documentosExpr += fmt.Sprintf(" || jsonb_build_object('bi_estudante', $%d::jsonb)", idx)
 		args = append(args, string(docJSON))
 		idx++
 	}
+	if payload.DocumentoBIEncarregado != nil {
+		docJSON, err := json.Marshal(payload.DocumentoBIEncarregado)
+		if err != nil {
+			return fmt.Errorf("handleDadosPessoaisAtualizados: falha ao serializar DocumentoBIEncarregado: %w", err)
+		}
+		documentosExpr += fmt.Sprintf(" || jsonb_build_object('bi_encarregado', $%d::jsonb)", idx)
+		args = append(args, string(docJSON))
+		idx++
+	}
+	if documentosExpr != "documentos" {
+		setClauses = append(setClauses, fmt.Sprintf("documentos = %s", documentosExpr))
+	}
 
 	if len(setClauses) == 0 {
 		_, err := p.client.DB().Exec(`
diff --git a/internal/handlers/solicitacao_edicao_bi_documento_integration_test.go b/internal/handlers/solicitacao_edicao_bi_documento_integration_test.go
index 1048069..2c6f7e9 100644
--- a/internal/handlers/solicitacao_edicao_bi_documento_integration_test.go
+++ b/internal/handlers/solicitacao_edicao_bi_documento_integration_test.go
@@ -3,15 +3,18 @@ package handlers
 import (
 	"bytes"
 	"encoding/json"
+	"fmt"
 	"mime/multipart"
 	"net/http"
 	"net/http/httptest"
 	"testing"
+	"time"
 
 	"github.com/gin-gonic/gin"
 	"github.com/google/uuid"
 
 	"spuri/internal/db"
+	"spuri/internal/domain/aggregates"
 	"spuri/internal/projections"
 	"spuri/internal/storage"
 )
@@ -240,3 +243,295 @@ func TestAprovarSolicitacaoEdicaoBISubstituiDocumentoAnterior(t *testing.T) {
 		t.Fatal("documento antigo ainda está acessível após substituição — deveria ter sido removido")
 	}
 }
+
+// criarEstudantePendenteDocumentosParaTeste cria (via evento real) um
+// estudante fundamental vinculado a uma academia, em status
+// 'pendente_documentos', já com bilhete_identidade preenchido (bilheteInicial)
+// mas SEM o documento oficial Documentos["bi_estudante"] — essa é a ÚNICA
+// pendência (bi_encarregado já está presente). Serve para testar o caso
+// relatado: a academia aprova uma solicitação de edição de BI do estudante,
+// o documento é promovido, mas o status permanece 'pendente_documentos'.
+func criarEstudantePendenteDocumentosParaTeste(t *testing.T, client *db.Client, bilheteInicial string) (uuid.UUID, string) {
+	t.Helper()
+	codigoAcademia := "IT" + uuid.New().String()[:8]
+	academiaID := criarAcademiaEscolarParaTeste(t, client, codigoAcademia, "fundamental", []string{"1_ano_fundamental"})
+
+	estID := uuid.New()
+	agg := &aggregates.Estudante{}
+	agg.SetID(estID)
+	codigoEstudante := "E" + uuid.New().String()[:6]
+	anoEscolar := "1_ano_fundamental"
+	telefoneEncarregado := fmt.Sprintf("9%08d", time.Now().UnixNano()%100000000)
+	bilheteResp := "999999999999ZZ"
+	// bi_estudante deliberadamente ausente dos documentos: é a única
+	// pendência deste estudante (bilhete já informado, mas sem o PDF).
+	documentos := map[string]aggregates.DocumentoMatricula{
+		"bi_encarregado": {Path: "teste/bi_encarregado.pdf"},
+	}
+	if err := agg.CriarComVinculoPendenteDocumentos(
+		"Estudante Teste "+codigoEstudante, codigoEstudante, "hash",
+		nil, nil, &telefoneEncarregado, &bilheteInicial, &bilheteResp,
+		"masculino", time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
+		&anoEscolar, nil, nil, nil, nil,
+		&academiaID, codigoAcademia, documentos,
+	); err != nil {
+		t.Fatalf("erro ao criar estudante pendente de documentos de teste: %v", err)
+	}
+	repository := db.NewAggregateRepository(client)
+	if err := repository.SaveWithAudit(agg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
+		t.Fatalf("erro ao salvar estudante de teste: %v", err)
+	}
+	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
+		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
+	}
+	return estID, codigoAcademia
+}
+
+// setupSolicitacaoEdicaoBIEncarregadoTestRouter é o equivalente de
+// setupSolicitacaoEdicaoBITestRouter, mas para o campo
+// bilhete_identidade_encarregado (rotas registradas em cmd/server/main.go).
+func setupSolicitacaoEdicaoBIEncarregadoTestRouter(client *db.Client, userID uuid.UUID, userType string) *gin.Engine {
+	repository := db.NewAggregateRepository(client)
+	router := gin.New()
+	router.Use(func(c *gin.Context) {
+		c.Set("dbClient", client)
+		c.Set("repository", repository)
+		c.Set("user_id", userID)
+		c.Set("user_type", userType)
+		c.Set("storageProvider", storage.NewLocalProvider())
+	})
+	router.POST("/estudante/solicitacoes-edicao/bilhete-identidade-encarregado", CriarSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade_encarregado"))
+	router.PUT("/academia/solicitacoes-edicao-estudante/bilhete-identidade-encarregado/:codigo/aprovar", DecidirSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade_encarregado", true))
+	router.PUT("/academia/solicitacoes-edicao-estudante/bilhete-identidade-encarregado/:codigo/reprovar", DecidirSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade_encarregado", false))
+	return router
+}
+
+func criarSolicitacaoEdicaoBIEncarregadoParaTeste(t *testing.T, client *db.Client, router *gin.Engine, novoValor string, conteudoPDF []byte) string {
+	t.Helper()
+	var buf bytes.Buffer
+	w := multipart.NewWriter(&buf)
+	if err := w.WriteField("novo_valor", novoValor); err != nil {
+		t.Fatalf("erro ao escrever campo novo_valor: %v", err)
+	}
+	part, err := createPDFFormFile(w, "documento", "bi_encarregado.pdf")
+	if err != nil {
+		t.Fatalf("erro ao criar campo de arquivo: %v", err)
+	}
+	if _, err := part.Write(conteudoPDF); err != nil {
+		t.Fatalf("erro ao escrever PDF de teste: %v", err)
+	}
+	if err := w.Close(); err != nil {
+		t.Fatalf("erro ao fechar multipart writer: %v", err)
+	}
+	rec := httptest.NewRecorder()
+	req := httptest.NewRequest(http.MethodPost, "/estudante/solicitacoes-edicao/bilhete-identidade-encarregado", &buf)
+	req.Header.Set("Content-Type", w.FormDataContentType())
+	router.ServeHTTP(rec, req)
+	if rec.Code != http.StatusCreated {
+		t.Fatalf("esperava 201 ao criar solicitação de edição de BI do encarregado, recebeu %d: %s", rec.Code, rec.Body.String())
+	}
+	var resp struct {
+		CodigoSolicitacao string `json:"codigo_solicitacao"`
+	}
+	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
+		t.Fatalf("erro ao decodificar resposta de criação: %v", err)
+	}
+	if err := projections.NewSolicitacaoEdicaoDadoEstudanteProjection(client).Rebuild(); err != nil {
+		t.Fatalf("erro ao reconstruir projeção de solicitações de edição: %v", err)
+	}
+	return resp.CodigoSolicitacao
+}
+
+func aprovarSolicitacaoEdicaoBIEncarregado(t *testing.T, client *db.Client, router *gin.Engine, codigoSolicitacao string) *httptest.ResponseRecorder {
+	t.Helper()
+	rec := httptest.NewRecorder()
+	req := httptest.NewRequest(http.MethodPut, "/academia/solicitacoes-edicao-estudante/bilhete-identidade-encarregado/"+codigoSolicitacao+"/aprovar", nil)
+	router.ServeHTTP(rec, req)
+	if rec.Code == http.StatusOK {
+		if err := projections.NewSolicitacaoEdicaoDadoEstudanteProjection(client).Rebuild(); err != nil {
+			t.Fatalf("erro ao reconstruir projeção de solicitações de edição após decisão: %v", err)
+		}
+	}
+	return rec
+}
+
+// syncEstudanteProjectionEventosRecentes é como syncEstudanteProjectionAposEvento,
+// mas processa os ÚLTIMOS n eventos do estudante (em vez de só o último) — em
+// produção, o projManager assíncrono processa cada evento do ledger em
+// ordem, um a um (ver Manager.processProjection), então uma aprovação que
+// gera dois eventos no mesmo SaveWithAudit (ex.: edição de campo +
+// EstudanteDocumentosCompletados, quando essa edição completa a última
+// pendência) precisa ter AMBOS replicados na projeção, na mesma ordem, para
+// o teste refletir fielmente o que aconteceria em produção.
+func syncEstudanteProjectionEventosRecentes(t *testing.T, client *db.Client, estID uuid.UUID, n int) {
+	t.Helper()
+	repository := db.NewAggregateRepository(client)
+	historico, err := repository.GetEventHistory(estID)
+	if err != nil || len(historico) < n {
+		t.Fatalf("erro ao buscar histórico do estudante para sincronizar projeção: %v (eventos=%d, esperado>=%d)", err, len(historico), n)
+	}
+	proj := projections.NewEstudanteProjection(client)
+	for _, evt := range historico[len(historico)-n:] {
+		if err := proj.Handle(evt); err != nil {
+			t.Fatalf("erro ao processar evento %s na projeção de estudantes: %v", evt.EventType, err)
+		}
+	}
+}
+
+// TestAprovarSolicitacaoEdicaoBICompletaDocumentosPendentes cobre o bug
+// relatado: um estudante 'pendente_documentos' cuja ÚNICA pendência é o
+// documento bi_estudante consegue, via solicitação de edição de BI aprovada
+// pela academia, anexar esse documento — mas o status do estudante
+// permanecia 'pendente_documentos' mesmo depois de todos os documentos
+// obrigatórios estarem completos.
+func TestAprovarSolicitacaoEdicaoBICompletaDocumentosPendentes(t *testing.T) {
+	client := nivelEscolarTestClient(t)
+	bilheteInicial := generateBITest()
+	estID, codigoAcademia := criarEstudantePendenteDocumentosParaTeste(t, client, bilheteInicial)
+
+	estAntes, err := projections.NewEstudanteProjection(client).GetByID(estID)
+	if err != nil || estAntes == nil {
+		t.Fatalf("erro ao buscar estudante de teste: %v", err)
+	}
+	if estAntes.Status != "pendente_documentos" {
+		t.Fatalf("pré-condição do teste falhou: status = %q, want pendente_documentos", estAntes.Status)
+	}
+	if _, ok := estAntes.Documentos["bi_estudante"]; ok {
+		t.Fatal("pré-condição do teste falhou: estudante já tinha documento bi_estudante")
+	}
+
+	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
+	if err != nil || academia == nil {
+		t.Fatalf("erro ao buscar academia de teste: %v", err)
+	}
+
+	routerEstudante := setupSolicitacaoEdicaoBITestRouter(client, estID, "estudante")
+	routerAcademia := setupSolicitacaoEdicaoBITestRouter(client, academia.ID, "academia")
+
+	// Edita o BI para um novo número, anexando finalmente o documento —
+	// esta deveria ser a última pendência do estudante.
+	novoBI := generateBITest()
+	codigoSolicitacao := criarSolicitacaoEdicaoBIParaTeste(t, client, routerEstudante, novoBI, minimalPDFBytesForTest())
+
+	rec := aprovarSolicitacaoEdicaoBI(t, client, routerAcademia, codigoSolicitacao)
+	if rec.Code != http.StatusOK {
+		t.Fatalf("esperava 200 ao aprovar solicitação, recebeu %d: %s", rec.Code, rec.Body.String())
+	}
+
+	// Esta aprovação produz DOIS eventos no mesmo SaveWithAudit: a edição do
+	// BI (DadosPessoaisAtualizados) e, se a correção funcionar,
+	// EstudanteDocumentosCompletados logo em seguida — replicamos os dois.
+	syncEstudanteProjectionEventosRecentes(t, client, estID, 2)
+	estApos, err := projections.NewEstudanteProjection(client).GetByID(estID)
+	if err != nil || estApos == nil {
+		t.Fatalf("erro ao buscar estudante após aprovação: %v", err)
+	}
+	if estApos.BilheteIdentidade == nil || *estApos.BilheteIdentidade != novoBI {
+		t.Fatalf("BilheteIdentidade = %v, want %q", estApos.BilheteIdentidade, novoBI)
+	}
+	if _, ok := estApos.Documentos["bi_estudante"]; !ok {
+		t.Fatal("Documentos[\"bi_estudante\"] ausente após aprovação")
+	}
+	if estApos.Status != "ativo" {
+		t.Fatalf("BUG: Status = %q, want \"ativo\" — o documento do BI foi completado mas o status pendente_documentos não foi atualizado", estApos.Status)
+	}
+}
+
+// criarEstudantePendenteDocumentosEncarregadoParaTeste é o espelho de
+// criarEstudantePendenteDocumentosParaTeste, mas a ÚNICA pendência é o
+// documento oficial do BI do ENCARREGADO (Documentos["bi_encarregado"]):
+// bilhete_identidade e bi_estudante já estão completos, bilhete_identidade_
+// encarregado já tem um número (bilheteRespInicial), mas falta o PDF.
+func criarEstudantePendenteDocumentosEncarregadoParaTeste(t *testing.T, client *db.Client, bilheteRespInicial string) (uuid.UUID, string) {
+	t.Helper()
+	codigoAcademia := "IT" + uuid.New().String()[:8]
+	academiaID := criarAcademiaEscolarParaTeste(t, client, codigoAcademia, "fundamental", []string{"1_ano_fundamental"})
+
+	estID := uuid.New()
+	agg := &aggregates.Estudante{}
+	agg.SetID(estID)
+	codigoEstudante := "E" + uuid.New().String()[:6]
+	anoEscolar := "1_ano_fundamental"
+	telefoneEncarregado := fmt.Sprintf("9%08d", time.Now().UnixNano()%100000000)
+	bilheteEstudante := generateBITest()
+	// bi_encarregado deliberadamente ausente dos documentos: é a única
+	// pendência deste estudante (bi_estudante já está completo).
+	documentos := map[string]aggregates.DocumentoMatricula{
+		"bi_estudante": {Path: "teste/bi_estudante.pdf"},
+	}
+	if err := agg.CriarComVinculoPendenteDocumentos(
+		"Estudante Teste "+codigoEstudante, codigoEstudante, "hash",
+		nil, nil, &telefoneEncarregado, &bilheteEstudante, &bilheteRespInicial,
+		"masculino", time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
+		&anoEscolar, nil, nil, nil, nil,
+		&academiaID, codigoAcademia, documentos,
+	); err != nil {
+		t.Fatalf("erro ao criar estudante pendente de documentos de teste: %v", err)
+	}
+	repository := db.NewAggregateRepository(client)
+	if err := repository.SaveWithAudit(agg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
+		t.Fatalf("erro ao salvar estudante de teste: %v", err)
+	}
+	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
+		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
+	}
+	return estID, codigoAcademia
+}
+
+// TestAprovarSolicitacaoEdicaoBIEncarregadoPromoveDocumentoECompletaPendencia
+// cobre o segundo gap encontrado junto do bug relatado: o documento anexado
+// a uma solicitação de bilhete_identidade_encarregado aprovada nunca era
+// promovido a Documentos["bi_encarregado"] (só o número era atualizado) — ao
+// contrário do que já acontecia para o BI do próprio estudante. Isso também
+// impedia esta via de completar um estudante 'pendente_documentos' cuja
+// única pendência fosse justamente o documento do BI do encarregado.
+func TestAprovarSolicitacaoEdicaoBIEncarregadoPromoveDocumentoECompletaPendencia(t *testing.T) {
+	client := nivelEscolarTestClient(t)
+	bilheteRespInicial := generateBITest()
+	estID, codigoAcademia := criarEstudantePendenteDocumentosEncarregadoParaTeste(t, client, bilheteRespInicial)
+
+	estAntes, err := projections.NewEstudanteProjection(client).GetByID(estID)
+	if err != nil || estAntes == nil {
+		t.Fatalf("erro ao buscar estudante de teste: %v", err)
+	}
+	if estAntes.Status != "pendente_documentos" {
+		t.Fatalf("pré-condição do teste falhou: status = %q, want pendente_documentos", estAntes.Status)
+	}
+	if _, ok := estAntes.Documentos["bi_encarregado"]; ok {
+		t.Fatal("pré-condição do teste falhou: estudante já tinha documento bi_encarregado")
+	}
+
+	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
+	if err != nil || academia == nil {
+		t.Fatalf("erro ao buscar academia de teste: %v", err)
+	}
+
+	routerEstudante := setupSolicitacaoEdicaoBIEncarregadoTestRouter(client, estID, "estudante")
+	routerAcademia := setupSolicitacaoEdicaoBIEncarregadoTestRouter(client, academia.ID, "academia")
+
+	novoBIResp := generateBITest()
+	codigoSolicitacao := criarSolicitacaoEdicaoBIEncarregadoParaTeste(t, client, routerEstudante, novoBIResp, minimalPDFBytesForTest())
+
+	rec := aprovarSolicitacaoEdicaoBIEncarregado(t, client, routerAcademia, codigoSolicitacao)
+	if rec.Code != http.StatusOK {
+		t.Fatalf("esperava 200 ao aprovar solicitação, recebeu %d: %s", rec.Code, rec.Body.String())
+	}
+
+	// Duas eventos no mesmo SaveWithAudit, como no teste do BI do
+	// estudante: a edição do campo e, se a correção funcionar,
+	// EstudanteDocumentosCompletados.
+	syncEstudanteProjectionEventosRecentes(t, client, estID, 2)
+	estApos, err := projections.NewEstudanteProjection(client).GetByID(estID)
+	if err != nil || estApos == nil {
+		t.Fatalf("erro ao buscar estudante após aprovação: %v", err)
+	}
+	if estApos.BilheteIdentidadeResp == nil || *estApos.BilheteIdentidadeResp != novoBIResp {
+		t.Fatalf("BilheteIdentidadeResp = %v, want %q", estApos.BilheteIdentidadeResp, novoBIResp)
+	}
+	if _, ok := estApos.Documentos["bi_encarregado"]; !ok {
+		t.Fatal("BUG: Documentos[\"bi_encarregado\"] ausente após aprovação — o documento anexado não foi promovido a oficial")
+	}
+	if estApos.Status != "ativo" {
+		t.Fatalf("BUG: Status = %q, want \"ativo\" — o documento do BI do encarregado foi completado mas o status pendente_documentos não foi atualizado", estApos.Status)
+	}
+}
```

### 4.2 Alternativa: aplicar manualmente

Se por algum motivo `git apply` falhar (por exemplo, `main` mudou desde que
este documento foi escrito), aplique as mudanças descritas na tabela da
seção 2, usando os trechos "antes"/"depois" que aparecem no diff acima como
referência exata do que trocar em cada arquivo. Não invente uma abordagem
diferente da descrita — se o contexto não bater, pare e reporte.

---

## 5. Verificação (rodar depois de aplicar)

```bash
go build ./...
go vet ./...
go test ./...
```

- `go build` e `go vet` devem terminar sem nenhuma saída (sem erros).
- `go test ./...` deve terminar com `ok` em todos os pacotes com testes, e
  `--- SKIP` (não `FAIL`) nos testes de integração que dependem de
  `DATABASE_URL` — isso é esperado no ambiente do Codex (ver nota no topo
  deste documento).
- Rode também `gofmt -l` nos quatro arquivos alterados; não deve haver
  nenhuma saída (arquivo já formatado).

```bash
gofmt -l internal/domain/aggregates/estudante.go \
        internal/projections/estudante_projection.go \
        internal/handlers/solicitacao_edicao_dado_estudante_handlers.go \
        internal/handlers/solicitacao_edicao_bi_documento_integration_test.go
```

Se qualquer um desses comandos apontar erro, **não prossiga silenciosamente**
— reporte a saída completa.

---

## 6. O que NÃO fazer

- Não tentar instalar PostgreSQL, Docker, `psql`, ou qualquer coisa via
  `apt` — vai bater em 403 Forbidden e é desnecessário (ver seção 3).
- Não alterar o `go.mod` / `go.sum` do projeto. (O Claude usou substituições
  locais de dependências só dentro do seu próprio sandbox, exclusivamente
  para contornar uma limitação de rede sua; isso não faz parte da correção e
  não deve aparecer em nenhum commit.)
- Não tocar em nenhum outro campo de edição (`nome`, `data_nascimento`) além
  do necessário — o bloco de conclusão de pendências já foi escrito de forma
  genérica (roda depois de qualquer campo, não só BI) porque isso é
  correto e seguro por construção (`CompletarDocumentosPendentes` só ativa o
  estudante se TODOS os documentos exigidos já estiverem completos), mas não
  crie lógica nova específica para esses outros campos.
- Não renomear `promoverDocumentoBIParaOficial` nem os testes existentes —
  só adicione o novo parâmetro/testes conforme descrito.
- Não remova ou "simplifique" os comentários de código incluídos no patch —
  eles documentam decisões (por que copiar em vez de mover, por que ignorar
  o erro de `CompletarDocumentosPendentes`, por que combinar as duas
  atualizações de `documentos` numa única expressão SQL) que evitam
  regressões futuras.

---

## 7. Fora do escopo desta tarefa (não fazer agora)

Nada identificado além do que já está coberto nas seções 1.1 e 1.2 — os dois
problemas do mesmo processo (status não atualizado + documento do
encarregado nunca promovido) foram corrigidos juntos porque são a mesma
causa raiz manifestada em dois campos irmãos.

---

## 8. Checklist final

- [x] Patch aplicado (via `git apply` ou manualmente) nos 4 arquivos da
      seção 2.
- [x] `go build ./...` sem erros.
- [x] `go vet ./...` sem apontamentos.
- [x] `gofmt -l` sem saída nos 4 arquivos.
- [x] `go test ./...` com `ok` em todos os pacotes com teste (SKIPs em
      testes de integração são esperados e aceitáveis).
- [x] Nenhuma mudança em `go.mod` / `go.sum`.
- [x] Commit com mensagem sugerida abaixo (ou equivalente).

### Mensagem de commit sugerida

```
fix(estudante): completar pendente_documentos apos edicao de BI aprovada

- aplicarEdicaoAprovada agora tenta CompletarDocumentosPendentes apos
  qualquer campo editado; se todos os documentos obrigatorios ja
  estiverem presentes, o estudante sai de pendente_documentos para
  ativo no mesmo SaveWithAudit
- o documento anexado a uma solicitacao de bilhete_identidade_encarregado
  aprovada agora e promovido a Documentos["bi_encarregado"], espelhando
  o que ja acontecia para o BI do proprio estudante (Tarefa 98)
- projecao de leitura (handleDadosPessoaisAtualizados) atualizada para
  refletir o novo documento do encarregado
- 2 novos testes de integracao cobrindo os dois casos, com Postgres real
```
