---
criado: 05-09-2026
origem: Fredy + Claude (orquestração)
status: pronto para execução
tipo: backend (spuri-backend)
depende_de: Tarefas 83, 84 e 85 (já implementadas)
---

# Tarefa 86 — Módulo de Serviços Extras — Correções pós-implementação

### Documento de execução para o Codex (orquestrado e pré-testado pelo Claude)

## 0. Leia isto primeiro — contexto desta tarefa

As Tarefas 83, 84 e 85 (Módulo de Serviços Extras, Fases 1-3) foram implementadas. Revisei o código real do repositório — não o documento de especificação, o código que de fato está no branch principal hoje — arquivo por arquivo, nos pontos de maior risco. **A implementação está, na esmagadora maioria, correta e fiel à especificação**, especificamente nos pontos mais perigosos:

- Whitelist do ledger (`internal/db/safe_queries.go`) e factory (`aggregate.go`): **corretas**, os 13 eventos e os 2 aggregate types novos estão todos registrados.
- Aggregates `ServicoExtra` e `SolicitacaoServicoExtra`: implementação fiel à especificação, incluindo o campo `VinculadaEm` adicionado na Fase 3.
- Integração financeira (`ChargeRequest`/`QRCodeRequest`/`gerarCobrancaInput`, persistência do payload, categorização de `origem`, `origensClause`): **correta**, inclusive a ordem de precedência delicada entre `matricula`/`servico_extra`/`mensalidade`/`avulsa`.
- Os três pontos de confirmação de pagamento (resposta síncrona, consulta, webhook) chamam corretamente `DadosServicoExtraDaCobranca` e despacham para `efetivarVinculoServicoExtraPago` ou `ConfirmarLancamentoServicoExtraPago` conforme o tipo de lançamento.
- `FinanceiroProjection.Handle` grava as obrigações com proteção de idempotência via `ON CONFLICT (event_id) DO NOTHING`.
- Reapliquei as 120 migrations (117 já existentes + as 3 novas) do zero num PostgreSQL 16 real: sem erro.
- `gofmt -l .` no repositório inteiro: limpo (nenhum arquivo, novo ou existente). Não consegui compilar (`go build`) neste ambiente pela mesma limitação de rede já documentada nas Tarefas 83-85 (proxy bloqueia `golang.org/x/*`, `google.golang.org/protobuf`, `gopkg.in/yaml.v3` mesmo para pacotes já presentes no `go.sum` do projeto) — isto não é specífico desta tarefa, tentei de novo e o resultado foi o mesmo.

**Encontrei 4 lacunas reais, das quais a primeira é a mais importante — corta um requisito explícito do pedido original:**

1. **O upload/anexo de documento na solicitação nunca foi implementado.** `internal/handlers/servico_extra_solicitacao_handlers.go`, função `SolicitarServicoExtra`, chama `s.Criar(sid, serv.CodigoAcademia, est.CodigoEstudante, "", "")` — os dois últimos parâmetros (`documentoPath`, `documentoURL`) estão **hardcoded como string vazia**. Não há `c.FormFile`, não há chamada a `readAndValidatePDF`, não há `getStorageProvider`. Consequência prática: `ServicoExtra.DocumentoObrigatorio` é um campo configurável na Fase 1 que **nunca é verificado em lugar nenhum** — uma academia pode marcar um serviço como exigindo documento e, na prática, qualquer estudante se inscreve sem enviar nada. Isto é parte explícita do pedido original ("a assinatura será feita pela plataforma com ou sem documento anexado") e precisa ser implementado nesta tarefa. Também não existe nenhum endpoint de download do documento (nem para a academia, nem para o próprio estudante).
2. **Falta checagem de posse em `GetServicoExtra`** (`internal/handlers/servico_extra_handlers.go`) — a rota `GET /academia/servicos-extras/:id` não verifica se o serviço pertence à academia autenticada antes de devolver os dados. Qualquer academia autenticada consegue ver a configuração completa (preço, taxa, categoria, detalhes personalizados) de um serviço de **outra** academia, bastando saber o UUID.
3. **Falta o guard de pré-ledger em `SolicitarServicoExtra`** — o handler confia apenas na leitura da projeção (`ExisteAtiva`) e no índice único parcial do banco para impedir solicitações duplicadas. O índice único (`ux_sol_servico_extra_ativa`) continua garantindo que o dado nunca fica inconsistente, mas sem o `db.NewUniqueOperationGuard` (usado em todo o resto do repositório para este mesmo problema, ex. `solicitacao_edicao_dado_estudante_handlers.go`) uma corrida entre duas requisições simultâneas do mesmo estudante pode resultar num erro 500 cru (violação de constraint) em vez de um 409 tratado.
4. **Cobertura de testes muito abaixo do especificado.** Existem só dois arquivos de teste: `internal/domain/aggregates/servico_extra_test.go` (cobre parte da validação do `ServicoExtra`, mas não testa `Atualizar`) e `internal/finance/servico_extra_obrigacao_test.go` (só testa `precedenciaEstado`, que é reaproveitada e nem precisava ser retestada). **Não existe nenhum teste para a máquina de estados de `SolicitacaoServicoExtra`** — nenhum teste de `Aprovar`, `Reprovar`, `VincularAposPagamento`, `CancelarAntesDaVinculacao` ou `Cancelar`, que é a lógica de negócio central de todo este módulo. Não existe nenhum teste de integração (nenhum arquivo `*_integration_test.go` relacionado a serviços extras).

Não encontrei nenhum problema na lógica de negócio, no schema ou na integração financeira em si — os 4 pontos acima são lacunas de implementação incompleta/testes, não erros conceituais. Esta tarefa fecha essas lacunas.

## 1. Prompt recomendado para executar esta tarefa

> Implemente exatamente as 4 correções descritas na seção 0 deste documento, na ordem das seções abaixo. Não altere nada que já está correto — em particular, não mexa na whitelist, na factory, na integração financeira ou nos aggregates além do estritamente necessário para a correção 1. Ao final, rode `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...`, corrija qualquer erro, e preencha o checklist da seção 6.

## 2. Correção 1 — Upload e download do documento anexado

### 2.1 `SolicitarServicoExtra`: implementar o upload real

Em `internal/handlers/servico_extra_solicitacao_handlers.go`, reescreva `SolicitarServicoExtra` para tratar o upload de documento exatamente como `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go` já faz (releia essa função antes de mexer aqui — é o padrão de referência: `c.Request.ParseMultipartForm`, `c.FormFile("documento")`, `readAndValidatePDF`, `getStorageProvider(c)`, `provider.Upload`, limpeza com `provider.Delete` em caso de falha subsequente):

```go
func SolicitarServicoExtra(c *gin.Context) {
	sid, e := uuid.Parse(c.Param("id"))
	if e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	uid, _ := middleware.GetUserID(c)
	est, e := getEstudanteProjection(c).GetByID(uid)
	if e != nil || est == nil || est.CodigoAcademia == nil {
		utils.RespondWithForbiddenError(c, "estudante não está vinculado a uma academia")
		return
	}
	serv, e := getServicosExtrasProjection(c).GetByID(sid)
	if e != nil || serv == nil || !serv.Ativo || serv.CodigoAcademia != *est.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "serviço extra indisponível")
		return
	}
	active, e := getSolicitacoesServicoExtraProjection(c).ExisteAtiva(sid, est.CodigoEstudante)
	if e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	if active {
		utils.RespondWithConflictError(c, "já existe solicitação ativa para este serviço")
		return
	}

	// Guard de pré-ledger (correção 3, feita junto por tocar o mesmo trecho)
	guard, e := db.NewUniqueOperationGuard(getDbClient(c)).WithContext(c.Request.Context()).Reserve(
		"solicitacao_servico_extra:ativa",
		db.CanonicalGuardKey(sid.String(), est.CodigoEstudante),
		db.UniqueGuardOptions{UserID: uid.String(), UserType: "estudante"},
	)
	if e != nil {
		utils.RespondWithConflictError(c, "já existe solicitação ativa para este serviço")
		return
	}
	released := false
	defer func() {
		if !released {
			_ = guard.Release()
		}
	}()

	var documentoPath, documentoURL string
	_ = c.Request.ParseMultipartForm(int64(utils.MaxPDFUploadBytes) + 1024)
	fh, ferr := c.FormFile("documento")
	if ferr != nil {
		if serv.DocumentoObrigatorio {
			utils.RespondWithValidationError(c, fmt.Errorf("documento é obrigatório para este serviço"))
			return
		}
		// sem documento e não é obrigatório: segue sem anexo
	} else {
		pdf, verr := readAndValidatePDF("documento", fh)
		if verr != nil {
			utils.RespondWithValidationError(c, verr)
			return
		}
		provider := getStorageProvider(c)
		if provider == nil {
			utils.RespondWithInternalError(c, errors.New("storage não configurado"))
			return
		}
		path := fmt.Sprintf("%s/estudantes/%s/servicos_extras/%s.pdf", serv.CodigoAcademia, est.CodigoEstudante, sid.String())
		stored, uerr := provider.Upload(path, bytes.NewReader(pdf.data), pdf.size)
		if uerr != nil {
			utils.RespondWithInternalError(c, uerr)
			return
		}
		documentoPath, documentoURL = stored.Path, stored.FileURL
	}

	s := aggregates.NewSolicitacaoServicoExtra()
	if e = s.Criar(sid, serv.CodigoAcademia, est.CodigoEstudante, documentoPath, documentoURL); e != nil {
		if documentoPath != "" {
			_ = getStorageProvider(c).Delete(documentoPath)
		}
		utils.RespondWithValidationError(c, e)
		return
	}
	if e = getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: uid.String(), UserType: "estudante", IP: c.ClientIP()}); e != nil {
		if documentoPath != "" {
			_ = getStorageProvider(c).Delete(documentoPath)
		}
		utils.RespondWithInternalError(c, e)
		return
	}
	released = true
	_ = guard.Consume(s.GetID())
	c.JSON(http.StatusCreated, gin.H{"data": s})
}
```

Confirme os nomes exatos de `readAndValidatePDF`, `getStorageProvider`, `utils.MaxPDFUploadBytes`, `db.NewUniqueOperationGuard`/`db.CanonicalGuardKey`/`db.UniqueGuardOptions` e a struct interna `pdf.data`/`pdf.size` lendo `solicitacao_edicao_dado_estudante_handlers.go` — copie a assinatura exata usada lá; o trecho acima é para orientar a lógica, não para colar sem conferir nomes. Adicione `"bytes"` e `"errors"` aos imports do arquivo, se ainda não estiverem lá.

### 2.2 Dois novos endpoints de download

Adicione a `internal/handlers/servico_extra_solicitacao_handlers.go`:

```go
func DownloadDocumentoSolicitacaoServicoExtraAcademia(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	codigo, _, ok := academy(c)
	if !ok {
		return
	}
	if s.CodigoAcademia != codigo {
		utils.RespondWithForbiddenError(c, "solicitação não pertence à academia")
		return
	}
	downloadDocumentoServicoExtra(c, s)
}

func DownloadDocumentoSolicitacaoServicoExtraEstudante(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	id, _ := middleware.GetUserID(c)
	est, e := getEstudanteProjection(c).GetByID(id)
	if e != nil || est == nil || s.CodigoEstudante != est.CodigoEstudante {
		utils.RespondWithForbiddenError(c, "solicitação não pertence ao estudante")
		return
	}
	downloadDocumentoServicoExtra(c, s)
}

func downloadDocumentoServicoExtra(c *gin.Context, s *aggregates.SolicitacaoServicoExtra) {
	if s.DocumentoPath == "" {
		utils.RespondWithNotFoundError(c, "documento")
		return
	}
	provider := getStorageProvider(c)
	if provider == nil {
		utils.RespondWithInternalError(c, errors.New("storage não configurado"))
		return
	}
	// Mirror do padrão já usado em DownloadDocumentoSolicitacaoEdicaoAcademia /
	// ...Estudante (solicitacao_edicao_dado_estudante_handlers.go): confirme o
	// nome exato do método do provider (Download/Stream/GetURL) lendo essas
	// duas funções antes de escrever esta, e replique o mesmo mecanismo de
	// resposta (stream direto ou redirect para URL assinada, conforme o que
	// já está implementado lá — não invente um terceiro mecanismo).
}
```

Registre as duas rotas em `cmd/server/main.go`, ao lado das já existentes:
```go
academiaRead.GET("/servicos-extras/solicitacoes/:id/documento/download", handlers.DownloadDocumentoSolicitacaoServicoExtraAcademia)
estudante.GET("/servicos-extras/minhas-inscricoes/:id/documento/download", handlers.DownloadDocumentoSolicitacaoServicoExtraEstudante)
```

## 3. Correção 2 — checagem de posse em `GetServicoExtra`

Em `internal/handlers/servico_extra_handlers.go`, `GetServicoExtra` hoje não verifica a academia. Esta rota está em `academiaRead` (academia **ou admin**), então a regra correta é: admin pode ver qualquer um; academia só pode ver os seus próprios. Ajuste para:

```go
func GetServicoExtra(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	if e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	x, e := getServicosExtrasProjection(c).GetByID(id)
	if e != nil || x == nil {
		utils.RespondWithNotFoundError(c, "serviço extra")
		return
	}
	if role, _ := middleware.GetUserType(c); role != "admin" {
		codigo, _, ok := academy(c)
		if !ok {
			return
		}
		if x.CodigoAcademia != codigo {
			utils.RespondWithForbiddenError(c, "serviço extra não pertence a esta academia")
			return
		}
	}
	c.JSON(200, gin.H{"data": x})
}
```

Confirme o nome exato da função que devolve o tipo do ator autenticado (`middleware.GetUserType` é um palpite razoável dado o padrão do resto do repositório — confira em `internal/middleware/auth.go` e ajuste se o nome for outro). Aplique a mesma checagem, se fizer sentido, em qualquer outro `GET` de detalhe que porventura tenha o mesmo problema — a lista de endpoints da Tarefa 84 é a referência do que deveria estar protegido.

## 4. Correção 3 — guard de pré-ledger em `SolicitarServicoExtra`

Já incluída no código da seção 2.1 acima (o upload e o guard tocam a mesma função, por isso foram resolvidos juntos). Não crie uma segunda função separada para isto.

## 5. Correção 4 — testes que faltam

### 5.1 `internal/domain/aggregates/solicitacao_servico_extra_test.go` (não existe — criar do zero)

Cubra, no mínimo, cada transição e cada rejeição de transição inválida:

- `Criar` → status `pendente`.
- `Aprovar(temTaxaInscricao=false, ...)` a partir de `pendente` → status `vinculada`, `VinculadaEm` preenchido.
- `Aprovar(temTaxaInscricao=true, valor>0, metodos válidos)` a partir de `pendente` → status `aprovada_pendente_pagamento_taxa_inscricao`, `VinculadaEm` continua zero.
- `Aprovar` com taxa mas `valor<=0` → erro.
- `Aprovar` chamado duas vezes seguidas → segunda chamada erro.
- `Reprovar` sem motivo → erro; com motivo, a partir de `pendente` → status `reprovada`.
- `Reprovar` a partir de `vinculada` → erro.
- `VincularAposPagamento` chamado sem passar antes por `aprovada_pendente_pagamento_taxa_inscricao` → erro.
- `VincularAposPagamento` a partir do estado correto → `vinculada`; chame duas vezes e confirme que `VinculadaEm` **não muda** na segunda vez que o status já é `vinculada` (a segunda chamada deve retornar erro, já que o método exige o status `aprovada_pendente_pagamento_taxa_inscricao` — teste isso explicitamente, é a proteção de idempotência que a Fase 3 depende).
- `CancelarAntesDaVinculacao` a partir de `pendente` (nunca aprovada) → erro; a partir de `aprovada_pendente_pagamento_taxa_inscricao` → `cancelada_antes_da_vinculacao`.
- `Cancelar` a partir de `vinculada` → `cancelada`; a partir de `pendente` → erro.
- `Cancelar`/`CancelarAntesDaVinculacao` com `canceladaPor` diferente de `"academia"`/`"estudante"` → erro.

### 5.2 `internal/domain/aggregates/servico_extra_test.go` — completar o que falta

Adicione um caso de teste para `Atualizar` (não há nenhum hoje): desligar `pago` (de `true` para `false`) sem informar `preco`/`tipo_cobranca`/`metodos_pagamento` deve zerá-los automaticamente, e o resultado final deve continuar passando pela mesma validação usada em `Criar`. Adicione também o caso "gratuito com taxa de inscrição válida" (`pago=false`, `temTaxaInscricao=true`, valor e métodos válidos) → sucesso — é a combinação intencional mais fácil de alguém "corrigir" por engano no futuro, vale ter um teste nomeado explicitamente para isso.

### 5.3 Pelo menos um teste de integração real (requer Postgres — `RUN_POSTGRES_INTEGRATION=1`)

Crie `internal/handlers/servico_extra_integration_test.go` cobrindo o fluxo completo com um servidor HTTP de teste simulando a AppyPay (mesma técnica de `internal/finance/appypay_integration_test.go`):

1. Academia com credencial de teste + `ServicoExtra` com `tem_taxa_inscricao=true` e `documento_obrigatorio=true`.
2. `POST .../solicitacao` **sem** documento → rejeitado (documento obrigatório).
3. `POST .../solicitacao` **com** um PDF válido → `201`; confirme, consultando a tabela `projection_solicitacoes_servico_extra` diretamente, que `documento_path` não está vazio.
4. Academia aprova → `aprovada_pendente_pagamento_taxa_inscricao`.
5. Pagamento confirmado via **webhook** simulado → status `vinculada`; repita a entrega do mesmo webhook e confirme que não há erro nem duplicação (idempotência).
6. `GET` do documento pela academia e pelo estudante dono → sucesso; pelo estudante que não é dono → `403`.
7. Estudante cancela a própria inscrição já vinculada → `cancelada`.

Isto cobre, num único teste, as quatro correções desta tarefa.

## 6. Checklist de aceite

- [ ] `SolicitarServicoExtra` faz upload real do documento (obrigatório ou opcional conforme configuração do serviço) e usa o guard de pré-ledger.
- [ ] Dois endpoints de download implementados e registrados, com checagem de posse.
- [ ] `GetServicoExtra` verifica que o serviço pertence à academia autenticada (exceto para admin).
- [ ] `solicitacao_servico_extra_test.go` criado, cobrindo todas as transições da seção 5.1.
- [ ] Casos de teste adicionados a `servico_extra_test.go` (seção 5.2).
- [ ] Teste de integração da seção 5.3 criado e passando (ou documentado como pulado por falta de Postgres no seu ambiente).
- [ ] `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...` limpos no seu ambiente.
- [ ] Nenhuma alteração feita fora do escopo desta tarefa (whitelist, factory e integração financeira continuam exatamente como estavam).
- [ ] Resultado reportado ao final: o que passou, o que falhou, o que não pôde ser testado no seu ambiente e por quê.
