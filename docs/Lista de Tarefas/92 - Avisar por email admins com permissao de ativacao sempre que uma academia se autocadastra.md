---
tarefa: 92
titulo: Avisar por email os administradores com permissão de ativação sempre que uma academia se autocadastra (POST /academia/cadastro)
repo: fredypdp/spuri-backend
status: pronta para execução pelo Codex
orquestrado_e_pre_testado_por: Claude
validado_contra: PostgreSQL 16 real + Go 1.24 (via apt), suíte completa existente + 5 testes novos (4 rodam sem banco, 1 gated por SPURI_RUN_DB_INTEGRITY_TESTS e já executado com sucesso contra Postgres real)
---

# Como usar este documento (leia antes de tocar em qualquer arquivo)

Este documento foi pré-testado de ponta a ponta pelo Claude contra infraestrutura real (Postgres 16, Go 1.24) — não é um plano teórico. Todos os diffs abaixo já foram aplicados, compilados (`go build ./...`, `go vet ./...`, `gofmt -l` limpo) e testados (suíte completa existente + 5 testes novos, todos passando, incluindo 2 testes de "revert-and-confirm" que provam que os testes novos realmente pegam regressão) numa cópia de trabalho do repositório.

Regras:
1. Aplique os diffs **exatamente como estão**, na ordem listada. Não refatore, não renomeie nada além do que está aqui, não "melhore" nada por conta própria (em particular: não adicione fallback de template, não adicione goroutine/assincronismo, não notifique no fluxo admin — ver "Decisões de design" na seção 1, elas são deliberadas).
2. Você (Codex) **não tem PostgreSQL, Docker nem psql** no seu ambiente. Não tente rodar o teste gated por `SPURI_RUN_DB_INTEGRITY_TESTS=1` (arquivo `internal/projections/admin_notificar_nova_academia_test.go`) — ele já foi rodado e confirmado por mim contra Postgres real (evidência na seção 4). Rode `go build ./...`, `go vet ./...` e `go test ./...` (sem definir `SPURI_RUN_DB_INTEGRITY_TESTS`) — isso já cobre a suíte inteira, inclusive os 4 testes novos que não dependem de banco.
3. Se qualquer diff não aplicar limpo (contexto não bate — por exemplo se `academia_handlers.go` já tiver mudado de algum jeito desde este documento), **pare e sinalize** — não tente adivinhar o que mudou.
4. Não é necessária nenhuma migration nova. Não crie nenhuma. `projection_admins` já tem índices em `role` e `status` separadamente; como a tabela é pequena (poucos administradores em produção), um índice composto novo não se justifica — decisão já tomada, não é pendência.
5. Nenhuma mudança no frontend (`spuripainel`) é necessária. A página `/academias` do painel já existe e já restringe o botão "Ativar" a `role === 'adm' || role === 'fpp'` (mesma regra usada aqui). Não crie nem edite nada nesse repositório para esta tarefa.
6. Ao terminar e validar (seção 3), mova este arquivo para `docs/Tarefas feitas/92 - Avisar por email admins com permissao de ativacao sempre que uma academia se autocadastra.md`.

---

## 1. Contexto / diagnóstico (investigação já feita — não repetir)

### 1.1 Duas rotas de cadastro de academia — o aviso é só para uma delas

- `POST /academia/cadastro` (pública, sem autenticação) → `internal/handlers/academia_handlers.go`, função `RegisterAcademiaPublica`. É o **autocadastro**: a própria academia preenche o formulário e cria a própria conta.
- `POST /dominis/academia/cadastro` (autenticada, `middleware.RequireFPP()`) → mesmo arquivo, função `RegisterAcademia`. É um admin `fpp` cadastrando uma academia em nome dela.

**Decisão de design confirmada com quem pediu a tarefa: o aviso por email é só para o autocadastro público (`RegisterAcademiaPublica`).** `RegisterAcademia` (fluxo admin) não deve chamar a nova função — quem cadastrou pela rota admin já é, ele mesmo, um admin com permissão de ativação e já sabe da academia que acabou de criar; notificá-lo de novo seria ruído. Isso está coberto por teste (seção 2, arquivo novo 3/3) com verificação nos dois sentidos.

Ambas as rotas sempre criam a academia com `status = "inativo"` (`Academia.Criar()`, sem exceção, nem pela rota admin) — mas isso é só contexto de negócio, não muda o que precisa ser codificado aqui.

### 1.2 Quem pode ativar uma academia (e portanto quem deve ser avisado)

`PUT /dominis/academia/:codigo/ativar` é protegida por `middleware.RequireAdm()`, que exige nível de hierarquia >= 2 (`fpp`=3, `adm`=2, `gerente`=1 — ver `internal/middleware/admin_auth_middleware.go`). Ou seja: **role `adm` ou `fpp`**, nunca `gerente`.

Além do role, `RequireAdminRole` (usado por toda rota `/dominis`) bloqueia:
- admin com `status != 'ativo'` → "administrador inativo";
- admin com `email_verificado = false` → bloqueado.

Ou seja: um admin inativo ou com email não verificado **não consegue** acessar `/dominis` nem agir no painel mesmo que receba o email. Por isso o critério de notificação usado é exatamente:

```sql
role IN ('adm', 'fpp') AND status = 'ativo' AND email_verificado = TRUE
```

Isso já bate com o que foi pedido ("administradores com permissão para ativar uma academia... com e-mail verificado") e evita mandar email para quem não conseguiria agir mesmo se quisesse.

### 1.3 Schema real de `projection_admins` (confirmado contra Postgres 16 real)

```
id, nome, email, senha_hash, role, status, email_verificado, telefone,
telefone_verificado, created_by, created_at, updated_at, version,
last_event_id, total_acoes_realizadas, deleted_at, deletado_por
```

Índices relevantes já existentes (não precisa criar nada): `idx_proj_admins_role`, `idx_proj_admins_status`, `idx_proj_admins_email`. Constraints de check já garantem `role IN ('fpp','adm','gerente')` e `status IN ('ativo','inativo','deletado')` — a query nova não precisa validar esses valores de novo.

### 1.4 Página do painel a referenciar no email

Confirmado em `spuripainel` (repo irmão, só leitura — nenhuma mudança lá): a listagem de academias fica em `/academias` (`src/app/(painel)/academias/page.tsx` — o grupo de rota `(painel)` não adiciona segmento na URL). O botão "Ativar" já usa exatamente `role === 'adm' || role === 'fpp'` (`PageContent.tsx`, variável `canAlterarSituacaoAcademia`) — a mesma regra usada aqui no backend. Não existe filtro por query string (`?status=inativo`) hoje, então o link do email é a própria página; o texto do email (fora do escopo de código, ver seção 5) deve orientar o admin a localizar a academia pelo nome ou código e conferir o status "inativo".

### 1.5 Decisão de design: sem fallback de template

`SendAdminWelcomeEmail` (já existente) cai para `templateReset` quando `EMAILJS_TEMPLATE_ADMIN_WELCOME` não está configurado. **A função nova NÃO segue esse padrão de propósito**: o conteúdo de um email de "redefinir senha" não tem nada a ver com um aviso de "academia pendente de análise" — mandar com o template errado confundiria o admin em vez de ajudar. Sem `EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA` configurado, o aviso apenas é logado no servidor e nenhum email é enviado (não é erro, não bloqueia nada). Isso está coberto por teste (seção 2, arquivo novo 1/3, terceiro caso).

### 1.6 Decisão de design: envio síncrono, sem goroutine

Todo o serviço de email do projeto (`SendAdminWelcomeEmail`, `SendVerificationEmail` etc.) é chamado de forma síncrona dentro do handler, sem goroutines. A função nova segue o mesmo padrão: um `for` simples sobre os admins retornados, chamando `SendAcademiaCadastradaEmail` uma vez por admin, de forma síncrona. Isso é seguro porque o número de admins com role `adm`/`fpp` tende a ser pequeno (poucas unidades) — não inventar concorrência aqui não é uma omissão, é a escolha certa para não fugir do padrão do resto do código sem necessidade real.

### 1.7 Falha ao notificar nunca bloqueia o cadastro

O cadastro da academia já foi persistido (evento já gravado no ledger) antes da chamada de notificação. Qualquer erro ao buscar admins ou ao enviar algum email é só logado (`log.Printf`) — a resposta HTTP do cadastro nunca muda por causa disso, e nenhum campo novo foi adicionado ao JSON de resposta.

---

## 2. Diffs a aplicar, nesta ordem

Todos os diffs abaixo foram extraídos com `git diff` a partir de uma cópia limpa de `main` após aplicar e validar as mudanças. Aplique com `git apply` ou reproduza manualmente as linhas `-`/`+` — o resultado final tem que ser idêntico.

### Arquivo 1/4 — `internal/projections/admin_projection.go` (nova query)

```diff
--- a/internal/projections/admin_projection.go
+++ b/internal/projections/admin_projection.go
@@ -425,6 +425,55 @@ func (p *AdminProjection) GetAll() ([]AdminDTO, error) {
 	return result, rows.Err()
 }
 
+// GetAdminsParaNotificarNovaAcademia retorna os administradores que devem
+// ser avisados por email sempre que uma academia se autocadastra via
+// POST /academia/cadastro (RegisterAcademiaPublica) e fica pendente de
+// análise/ativação.
+//
+// Critério — o mesmo exigido para efetivamente ativar uma academia via
+// PUT /dominis/academia/:codigo/ativar (middleware.RequireAdm()):
+//   - role 'adm' ou 'fpp' (RequireAdm() exige nível >= 2 na hierarquia
+//     fpp=3, adm=2, gerente=1 — ver internal/middleware/admin_auth_middleware.go).
+//     'gerente' nunca é retornado: não tem permissão para ativar.
+//   - status = 'ativo': um admin 'inativo' não consegue autenticar nem agir
+//     no painel mesmo que receba o email (RequireAdminRole bloqueia com
+//     "administrador inativo") — notificá-lo não teria utilidade.
+//   - email_verificado = TRUE: RequireAdminRole bloqueia toda rota /dominis
+//     para admin sem email verificado — de nada adianta avisar quem ainda
+//     não confirmou o próprio email, e essa é exatamente a exigência já
+//     pedida para este aviso.
+func (p *AdminProjection) GetAdminsParaNotificarNovaAcademia() ([]AdminDTO, error) {
+	rows, err := p.client.DB().Query(`
+		SELECT id, nome, email, senha_hash, role, status, email_verificado, telefone, telefone_verificado,
+			created_by, created_at, updated_at, version, total_acoes_realizadas
+		FROM projection_admins
+		WHERE status = 'ativo' AND email_verificado = TRUE AND role IN ('adm', 'fpp')
+		ORDER BY created_at ASC
+	`)
+	if err != nil {
+		return nil, err
+	}
+	defer rows.Close()
+	var result []AdminDTO
+	for rows.Next() {
+		var dto AdminDTO
+		var createdBy sql.NullString
+		if err := rows.Scan(
+			&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash, &dto.Role, &dto.Status,
+			&dto.EmailVerificado, &dto.Telefone, &dto.TelefoneVerificado, &createdBy, &dto.CreatedAt, &dto.UpdatedAt,
+			&dto.Version, &dto.TotalAcoesRealizadas,
+		); err != nil {
+			continue
+		}
+		if createdBy.Valid {
+			cid, _ := uuid.Parse(createdBy.String)
+			dto.CreatedBy = &cid
+		}
+		result = append(result, dto)
+	}
+	return result, rows.Err()
+}
+
 func scanAdmin(row *sql.Row) (*AdminDTO, error) {
 	var dto AdminDTO
 	var createdBy sql.NullString
```

Nenhum import novo é necessário — `sql` e `uuid` já são importados neste arquivo (usados por `GetAll`/`scanAdmin` logo acima).

### Arquivo 2/4 — `internal/services/email_service.go` (novo tipo + nova função de envio)

```diff
--- a/internal/services/email_service.go
+++ b/internal/services/email_service.go
@@ -384,6 +384,66 @@ func (s *EmailService) SendAdminWelcomeEmail(email, nome, senhaTemporaria, role
 	return s.sendEmailViaEmailJS(email, nome, templateAdmin, params)
 }
 
+// AcademiaCadastradaInfo agrupa os dados (não sensíveis) da academia recém
+// autocadastrada usados no email de aviso enviado aos administradores com
+// permissão de ativação. Nenhum campo aqui inclui dados de acesso (senha,
+// hash) — o admin apenas revisa e decide ativar ou não pelo painel.
+type AcademiaCadastradaInfo struct {
+	Nome           string
+	CodigoAcademia string
+	NIF            string
+	Nivel          string // "escola" ou "superior"
+	Type           string // "public" ou "private"
+	Provincia      string // código de 3 letras, ex: "LDA"
+}
+
+// SendAcademiaCadastradaEmail avisa um administrador (role 'adm' ou 'fpp',
+// email verificado — ver AdminProjection.GetAdminsParaNotificarNovaAcademia)
+// que uma nova academia se autocadastrou via POST /academia/cadastro e está
+// pendente de análise/ativação no painel.
+//
+// Assim como as demais notificações deste serviço, falha aqui NUNCA deve
+// bloquear o fluxo que a originou — é responsabilidade do chamador apenas
+// logar o erro e seguir (ver uso em internal/handlers/academia_handlers.go).
+//
+// Diferente de SendAdminWelcomeEmail, esta função NÃO cai para
+// templateReset como fallback quando EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA
+// não está configurado: o conteúdo de um email de "redefinir senha" não faz
+// sentido nenhum para um aviso de "academia pendente de análise" — enviar
+// com o template errado confundiria o admin em vez de ajudar. Sem o
+// template dedicado configurado, o aviso apenas é logado no servidor.
+func (s *EmailService) SendAcademiaCadastradaEmail(adminEmail, adminNome string, info AcademiaCadastradaInfo) error {
+	if !s.enabled {
+		log.Printf("[EMAIL] ⚠️  Serviço desabilitado — aviso de nova academia %s (%s) não enviado para admin %s",
+			info.Nome, info.CodigoAcademia, adminEmail)
+		return nil // não é erro: mesmo modo degradado usado em SendAdminWelcomeEmail
+	}
+
+	if adminEmail == "" {
+		return fmt.Errorf("email do admin vazio")
+	}
+
+	templateID := os.Getenv("EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA")
+	if templateID == "" {
+		log.Printf("[EMAIL] ⚠️  EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA não configurado — aviso de nova academia %s (%s) não enviado para admin %s",
+			info.Nome, info.CodigoAcademia, adminEmail)
+		return nil // não bloqueia: só não há template dedicado configurado ainda
+	}
+
+	params := map[string]string{
+		"user_name":          adminNome,
+		"academia_nome":      info.Nome,
+		"academia_codigo":    info.CodigoAcademia,
+		"academia_nif":       info.NIF,
+		"academia_nivel":     info.Nivel,
+		"academia_tipo":      info.Type,
+		"academia_provincia": info.Provincia,
+		"painel_url":         fmt.Sprintf("%s/academias", s.frontendURL),
+	}
+
+	return s.sendEmailViaEmailJS(adminEmail, adminNome, templateID, params)
+}
+
 // GetDefaultPassword retorna a senha padrão para estudantes e academias.
 // O código do estudante/academia é conhecido pelo operador que criou o registro,
 // tornando este mecanismo aceitável para esses perfis.
```

Nenhum import novo é necessário — `fmt`, `log`, `os` já são importados neste arquivo.

### Arquivo 3/4 — `internal/handlers/academia_handlers.go` (liga tudo, só no autocadastro público)

```diff
--- a/internal/handlers/academia_handlers.go
+++ b/internal/handlers/academia_handlers.go
@@ -422,6 +422,8 @@ func RegisterAcademiaPublica(c *gin.Context) {
 
 	log.Printf("Academia auto-registada (cadastro público, pendente de ativação): %s (%s)", req.Nome, codigoAcademia)
 
+	notificarAdminsSobreNovaAcademiaPendente(c, req, codigoAcademia, codigoProvincia)
+
 	aviso := "guarde o código da academia: ele é o seu identificador de login. você definiu sua própria senha no cadastro."
 	if !temAlvara {
 		aviso += fmt.Sprintf(
@@ -446,6 +448,47 @@ func RegisterAcademiaPublica(c *gin.Context) {
 	})
 }
 
+// notificarAdminsSobreNovaAcademiaPendente avisa por email todos os
+// administradores com permissão de ativar academias (role 'adm' ou 'fpp',
+// ativos, com email verificado — ver AdminProjection.GetAdminsParaNotificarNovaAcademia)
+// sempre que uma academia se autocadastra via POST /academia/cadastro e fica
+// pendente de análise/ativação.
+//
+// Chamada apenas em RegisterAcademiaPublica (autocadastro). O cadastro feito
+// por um admin fpp em POST /dominis/academia/cadastro (RegisterAcademia) não
+// aciona este aviso — quem cadastrou já é, ele mesmo, um admin com permissão
+// de ativação e já sabe da academia que acabou de criar.
+//
+// Bloqueio: NUNCA. O cadastro já foi concluído com sucesso (evento já
+// persistido) antes desta chamada — qualquer falha aqui é só logada, no
+// mesmo padrão não bloqueante do restante do serviço de email do projeto.
+func notificarAdminsSobreNovaAcademiaPendente(c *gin.Context, req RegisterAcademiaRequest, codigoAcademia, codigoProvincia string) {
+	admins, err := getAdminProjection(c).GetAdminsParaNotificarNovaAcademia()
+	if err != nil {
+		log.Printf("[WARN] notificarAdminsSobreNovaAcademiaPendente: falha ao buscar administradores para notificar sobre %s: %v", codigoAcademia, err)
+		return
+	}
+	if len(admins) == 0 {
+		log.Printf("[WARN] notificarAdminsSobreNovaAcademiaPendente: nenhum admin com permissão de ativação e email verificado encontrado — academia %s (%s) ficou sem notificação por email", req.Nome, codigoAcademia)
+		return
+	}
+
+	emailSvc := getEmailService(c)
+	info := services.AcademiaCadastradaInfo{
+		Nome:           req.Nome,
+		CodigoAcademia: codigoAcademia,
+		NIF:            req.NIF,
+		Nivel:          req.Nivel,
+		Type:           req.Type,
+		Provincia:      codigoProvincia,
+	}
+	for _, admin := range admins {
+		if emailErr := emailSvc.SendAcademiaCadastradaEmail(admin.Email, admin.Nome, info); emailErr != nil {
+			log.Printf("[WARN] notificarAdminsSobreNovaAcademiaPendente: falha ao notificar admin %s sobre academia %s: %v", admin.Email, codigoAcademia, emailErr)
+		}
+	}
+}
+
 // ============================================================================
 // PUT /academia/dados
 // ============================================================================
```

Nenhum import novo é necessário — `services`, `log`, `fmt` já são importados neste arquivo. `getAdminProjection` e `getEmailService` já existem em `internal/handlers/helpers.go`.

### Arquivo 4/4 — `.env.example` (documentação da variável nova)

```diff
--- a/.env.example
+++ b/.env.example
@@ -72,6 +72,11 @@ EMAILJS_TEMPLATE_RESET=template_xxxxxxxx
 # Opcional: template dedicado para boas-vindas de administradores.
 # Se vazio, o backend reutiliza EMAILJS_TEMPLATE_RESET como fallback.
 EMAILJS_TEMPLATE_ADMIN_WELCOME=template_xxxxxxxx
+# Opcional: template de aviso a administradores (role adm/fpp, email
+# verificado) sempre que uma academia se autocadastra via
+# POST /academia/cadastro. Sem fallback: se vazio, o aviso apenas é logado
+# no servidor e nenhum email é enviado (ver SendAcademiaCadastradaEmail).
+EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA=template_xxxxxxxx
 EMAILJS_PUBLIC_KEY=xxxxxxxxxxxxxxxxxxxxxx
 EMAILJS_PRIVATE_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
 
```

### Arquivo NOVO 1/3 — `internal/services/academia_cadastrada_email_test.go`

Testes unitários puros (sem banco, sem rede) para os 3 comportamentos de `SendAcademiaCadastradaEmail`: serviço desabilitado, destinatário vazio, template dedicado ausente. Crie com o conteúdo exato abaixo:

```go
package services

import "testing"

func TestSendAcademiaCadastradaEmailDisabledDoesNotError(t *testing.T) {
	t.Setenv("EMAILJS_SERVICE_ID", "")
	t.Setenv("EMAILJS_TEMPLATE_VERIFICATION", "")
	t.Setenv("EMAILJS_TEMPLATE_RESET", "")
	t.Setenv("EMAILJS_PUBLIC_KEY", "")
	t.Setenv("EMAILJS_PRIVATE_KEY", "")
	t.Setenv("EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA", "")

	svc := NewEmailService(nil)
	if svc.IsEnabled() {
		t.Fatal("esperava serviço desabilitado sem EMAILJS_* configurado")
	}

	err := svc.SendAcademiaCadastradaEmail("admin@example.com", "Admin Teste", AcademiaCadastradaInfo{
		Nome:           "Academia Teste",
		CodigoAcademia: "LDA2026A001",
		NIF:            "5417845812",
		Nivel:          "escola",
		Type:           "private",
		Provincia:      "LDA",
	})
	if err != nil {
		t.Fatalf("com serviço desabilitado deveria retornar nil (não bloqueia o cadastro), obteve: %v", err)
	}
}

func TestSendAcademiaCadastradaEmailEmptyRecipientErrors(t *testing.T) {
	t.Setenv("EMAILJS_SERVICE_ID", "svc_test")
	t.Setenv("EMAILJS_TEMPLATE_VERIFICATION", "tpl_verify")
	t.Setenv("EMAILJS_TEMPLATE_RESET", "tpl_reset")
	t.Setenv("EMAILJS_PUBLIC_KEY", "pub_test")
	t.Setenv("EMAILJS_PRIVATE_KEY", "")
	t.Setenv("EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA", "tpl_academia")

	svc := NewEmailService(nil)
	if !svc.IsEnabled() {
		t.Fatal("esperava serviço habilitado com EMAILJS_* mínimos configurados")
	}

	err := svc.SendAcademiaCadastradaEmail("", "Admin Teste", AcademiaCadastradaInfo{Nome: "Academia Teste"})
	if err == nil {
		t.Fatal("esperava erro para destinatário vazio")
	}
}

func TestSendAcademiaCadastradaEmailMissingTemplateDoesNotError(t *testing.T) {
	t.Setenv("EMAILJS_SERVICE_ID", "svc_test")
	t.Setenv("EMAILJS_TEMPLATE_VERIFICATION", "tpl_verify")
	t.Setenv("EMAILJS_TEMPLATE_RESET", "tpl_reset")
	t.Setenv("EMAILJS_PUBLIC_KEY", "pub_test")
	t.Setenv("EMAILJS_PRIVATE_KEY", "")
	t.Setenv("EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA", "") // deliberadamente não configurado

	svc := NewEmailService(nil)
	if !svc.IsEnabled() {
		t.Fatal("esperava serviço habilitado com EMAILJS_* mínimos configurados")
	}

	err := svc.SendAcademiaCadastradaEmail("admin@example.com", "Admin Teste", AcademiaCadastradaInfo{Nome: "Academia Teste"})
	if err != nil {
		t.Fatalf("sem EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA configurado deveria retornar nil (sem fallback de template), obteve: %v", err)
	}
}
```

### Arquivo NOVO 2/3 — `internal/handlers/academia_notificacao_admins_test.go`

Teste de inspeção de código-fonte (mesmo padrão já usado em `internal/handlers/unique_operation_guard_integration_test.go`, reaproveitando as funções `readHandlerSource` e `mustContain` já definidas nesse arquivo — não precisa redefini-las). Confirma que `RegisterAcademiaPublica` chama a função nova e que `RegisterAcademia` **não** chama. Crie com o conteúdo exato abaixo:

```go
package handlers

import (
	"strings"
	"testing"
)

// TestRegisterAcademiaPublicaNotificaAdminsRegistroAcademia garante que:
//   - o autocadastro público (RegisterAcademiaPublica, POST /academia/cadastro)
//     chama notificarAdminsSobreNovaAcademiaPendente logo após persistir a
//     academia com sucesso;
//   - o cadastro feito por um admin fpp (RegisterAcademia,
//     POST /dominis/academia/cadastro) NÃO chama essa função — o aviso é só
//     para o autocadastro público; quem cadastrou pela rota admin já é, ele
//     mesmo, um admin com permissão de ativação e já sabe da academia que
//     acabou de criar.
func TestRegisterAcademiaPublicaNotificaAdminsRegistroAcademia(t *testing.T) {
	source := readHandlerSource(t, "internal/handlers/academia_handlers.go")

	mustContain(t, source, "func notificarAdminsSobreNovaAcademiaPendente(")
	mustContain(t, source, "GetAdminsParaNotificarNovaAcademia()")
	mustContain(t, source, "SendAcademiaCadastradaEmail(")

	publicaBody := extractFuncBody(t, source, "func RegisterAcademiaPublica(")
	if !strings.Contains(publicaBody, "notificarAdminsSobreNovaAcademiaPendente(") {
		t.Fatal("RegisterAcademiaPublica deveria chamar notificarAdminsSobreNovaAcademiaPendente após o cadastro")
	}

	adminBody := extractFuncBody(t, source, "func RegisterAcademia(")
	if strings.Contains(adminBody, "notificarAdminsSobreNovaAcademiaPendente(") {
		t.Fatal("RegisterAcademia (cadastro pelo admin fpp) não deveria chamar notificarAdminsSobreNovaAcademiaPendente — o aviso é só para o autocadastro público")
	}
}

// extractFuncBody devolve o texto da função (do "func Nome(" até a próxima
// declaração de função de nível superior), para checar chamadas feitas
// especificamente dentro dela e não em qualquer lugar do arquivo.
func extractFuncBody(t *testing.T, source, funcSignaturePrefix string) string {
	t.Helper()
	start := strings.Index(source, funcSignaturePrefix)
	if start == -1 {
		t.Fatalf("declaração %q não encontrada", funcSignaturePrefix)
	}
	rest := source[start+len(funcSignaturePrefix):]
	end := strings.Index(rest, "\nfunc ")
	if end == -1 {
		t.Fatalf("não foi possível delimitar o fim da função %q", funcSignaturePrefix)
	}
	return rest[:end]
}
```

**Atenção**: se `internal/handlers/unique_operation_guard_integration_test.go` (ou qualquer outro arquivo do pacote) deixar de existir/for renomeado e `readHandlerSource`/`mustContain` sumirem, este arquivo não compila. Isso não deveria acontecer nesta tarefa (você não toca nesse arquivo), é só um aviso.

### Arquivo NOVO 3/3 — `internal/projections/admin_notificar_nova_academia_test.go`

Teste de integração real contra Postgres, gated por `SPURI_RUN_DB_INTEGRITY_TESTS=1` (mesmo padrão de `internal/db/event_store_integrity_test.go`). **Já rodei este teste com sucesso contra Postgres 16 real** (evidência na seção 4) — você só precisa criar o arquivo, ele vai ser pulado (`t.Skip`) no seu ambiente por falta de `SPURI_RUN_DB_INTEGRITY_TESTS=1`/banco, e isso é esperado. Crie com o conteúdo exato abaixo:

```go
package projections

import (
	"os"
	"testing"

	"spuri/internal/db"
)

// TestGetAdminsParaNotificarNovaAcademiaFiltraPorPermissao valida, contra um
// PostgreSQL real e isolado, que a query usada para decidir quem recebe o
// aviso de "nova academia pendente de ativação" (POST /academia/cadastro)
// retorna exatamente os admins com role 'adm' ou 'fpp', status 'ativo' e
// email_verificado = true — o mesmo critério exigido para efetivamente
// ativar uma academia (PUT /dominis/academia/:codigo/ativar).
func TestGetAdminsParaNotificarNovaAcademiaFiltraPorPermissao(t *testing.T) {
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}

	prevDir, _ := os.Getwd()
	_ = os.Chdir("../..")
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	client, err := db.NewClient(db.DefaultConfig())
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer client.Close()
	if err := client.RunMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	// Este teste assume um banco isolado/vazio, mesma exigência dos demais
	// testes gated por SPURI_RUN_DB_INTEGRITY_TESTS (ver README de testes).
	if _, err := client.DB().Exec(`TRUNCATE projection_admins CASCADE`); err != nil {
		t.Fatalf("truncate projection_admins: %v", err)
	}

	const bootstrapID = "00000000-0000-0000-0000-0000000000aa"
	if _, err := client.DB().Exec(`
		INSERT INTO projection_admins (id, nome, email, senha_hash, role, status, email_verificado, created_by, created_at, updated_at, version)
		VALUES ($1, 'FPP Bootstrap Verificado Ativo', 'fpp.bootstrap@example.com', 'x', 'fpp', 'ativo', true, NULL, now(), now(), 1)
	`, bootstrapID); err != nil {
		t.Fatalf("insert bootstrap fixture: %v", err)
	}

	insert := func(nome, email, role, status string, verificado bool) {
		t.Helper()
		if _, err := client.DB().Exec(`
			INSERT INTO projection_admins (id, nome, email, senha_hash, role, status, email_verificado, created_by, created_at, updated_at, version)
			VALUES (gen_random_uuid(), $1, $2, 'x', $3, $4, $5, $6, now(), now(), 1)
		`, nome, email, role, status, verificado, bootstrapID); err != nil {
			t.Fatalf("insert fixture %s: %v", email, err)
		}
	}

	insert("ADM Verificado Ativo", "adm.ok@example.com", "adm", "ativo", true)
	insert("Gerente Verificado Ativo", "gerente.ok@example.com", "gerente", "ativo", true)
	insert("ADM Nao Verificado", "adm.naoverificado@example.com", "adm", "ativo", false)
	insert("ADM Inativo Verificado", "adm.inativo@example.com", "adm", "inativo", true)
	insert("FPP Deletado Verificado", "fpp.deletado@example.com", "fpp", "deletado", true)

	proj := NewAdminProjection(client)
	admins, err := proj.GetAdminsParaNotificarNovaAcademia()
	if err != nil {
		t.Fatalf("GetAdminsParaNotificarNovaAcademia: %v", err)
	}

	got := map[string]bool{}
	for _, a := range admins {
		got[a.Email] = true
	}

	wantPresent := []string{"fpp.bootstrap@example.com", "adm.ok@example.com"}
	for _, email := range wantPresent {
		if !got[email] {
			t.Errorf("esperava %s na lista de notificação, não veio. Retornados: %+v", email, got)
		}
	}

	wantAbsent := []string{
		"gerente.ok@example.com",        // role sem permissão de ativar academia
		"adm.naoverificado@example.com", // email não verificado
		"adm.inativo@example.com",       // admin inativo (não consegue agir mesmo notificado)
		"fpp.deletado@example.com",      // admin deletado
	}
	for _, email := range wantAbsent {
		if got[email] {
			t.Errorf("%s não deveria estar na lista de notificação, mas está. Retornados: %+v", email, got)
		}
	}

	if len(admins) != len(wantPresent) {
		t.Errorf("esperava exatamente %d admins retornados, obteve %d: %+v", len(wantPresent), len(admins), admins)
	}
}
```

---

## 3. Checklist de aceitação

- [ ] `AdminProjection.GetAdminsParaNotificarNovaAcademia()` existe em `internal/projections/admin_projection.go` e retorna só admins com `role IN ('adm','fpp')`, `status='ativo'`, `email_verificado=true`.
- [ ] `EmailService.SendAcademiaCadastradaEmail(adminEmail, adminNome string, info AcademiaCadastradaInfo) error` existe em `internal/services/email_service.go`, sem fallback de template.
- [ ] `RegisterAcademiaPublica` (POST /academia/cadastro) chama `notificarAdminsSobreNovaAcademiaPendente` logo após o `log.Printf` de sucesso do cadastro.
- [ ] `RegisterAcademia` (POST /dominis/academia/cadastro, fluxo admin) **não** chama `notificarAdminsSobreNovaAcademiaPendente`.
- [ ] Falha ao buscar admins ou ao enviar algum email é só logada — a resposta HTTP de `RegisterAcademiaPublica` (código 201, corpo JSON) fica idêntica à de antes desta tarefa.
- [ ] `EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA` documentada em `.env.example`, sem valor real (placeholder), com comentário explicando que não há fallback.
- [ ] Nenhuma mudança em `spuripainel`.
- [ ] Nenhuma migration nova criada.
- [ ] `go build ./...` e `go vet ./...` limpos.
- [ ] `gofmt -l` não lista nenhum dos arquivos tocados.
- [ ] `go test ./...` (sem `SPURI_RUN_DB_INTEGRITY_TESTS`) limpo — inclui os 4 testes novos que não dependem de banco (3 em `internal/services`, 1 em `internal/handlers`).
- [ ] O arquivo `internal/projections/admin_notificar_nova_academia_test.go` existe e compila (ele só roda de verdade com `SPURI_RUN_DB_INTEGRITY_TESTS=1` + Postgres — no seu ambiente ele vai aparecer como `SKIP`, isso é esperado, não é falha).

## 4. Validação já feita (evidência real, não suposição)

Rodei tudo isto eu mesmo, numa cópia de trabalho, contra infraestrutura real (não é leitura de código):

- **Ambiente**: Go 1.24.4 + PostgreSQL 16 instalados via apt no meu sandbox (você não tem isso — daí eu ter rodado por você). Para compilar localmente, usei `replace` directives **temporárias e só no meu sandbox** em `go.mod`, apontando `golang.org/x/*`, `google.golang.org/protobuf` e `gopkg.in/yaml.v3` para clones locais dos respectivos mirrors em `github.com/golang/*`, `github.com/protocolbuffers/protobuf-go` e `github.com/go-yaml/yaml` (meu sandbox bloqueia `golang.org`/`gopkg.in`/`google.golang.org` diretamente, mas permite `github.com`). **Nenhuma dessas `replace` está no `go.mod` real** — conferi com `git diff go.mod` no final: vazio, `go.mod` do diff acima é o único ponto de contato e nem esse arquivo foi tocado (a tarefa não mexe em `go.mod`). Isso é só um artefato do MEU sandbox; o seu ambiente normal de Go não deveria ter esse problema.
- **Build**: `go build ./...` → limpo.
- **Vet**: `go vet ./...` → limpo, zero avisos.
- **Formatação**: `gofmt -l` nos 3 arquivos modificados e nos 3 arquivos novos → vazio (já no padrão).
- **Suíte completa existente, sem banco**: `go test ./...` → todos os pacotes `ok`, incluindo os 4 testes novos sem banco.
- **Suíte completa com banco real, banco limpo do zero** (`DROP DATABASE` + `CREATE DATABASE` + `go test ./...` com `SPURI_RUN_DB_INTEGRITY_TESTS=1`): todos os pacotes `ok`, incluindo `TestGetAdminsParaNotificarNovaAcademiaFiltraPorPermissao` passando contra Postgres real com as 6 fixtures descritas (bootstrap fpp ativo+verificado, adm ativo+verificado, gerente ativo+verificado, adm ativo+não-verificado, adm inativo+verificado, fpp deletado+verificado) — retornou exatamente os 2 admins esperados (bootstrap fpp + adm ok), excluindo corretamente os outros 4.
- **Query SQL validada isoladamente antes de escrever qualquer código Go**: rodei a query exata via `psql` contra o schema real (`projection_admins`, migrations até 122 aplicadas) com as mesmas 6 fixtures, confirmando o resultado esperado antes de portar para Go — reduz a chance de erro de tradução SQL→Go.
- **Revert-and-confirm (duas direções, ambas confirmadas)**:
  1. Removi a chamada de `notificarAdminsSobreNovaAcademiaPendente` de dentro de `RegisterAcademiaPublica` → `TestRegisterAcademiaPublicaNotificaAdminsRegistroAcademia` **falhou** corretamente ("RegisterAcademiaPublica deveria chamar..."). Restaurei e reconfirmei verde.
  2. Adicionei uma chamada indevida de `notificarAdminsSobreNovaAcademiaPendente` dentro de `RegisterAcademia` → o mesmo teste **falhou** corretamente na outra ponta ("RegisterAcademia... não deveria chamar..."). Restaurei e reconfirmei verde, com `go build ./...` limpo depois de restaurar.
  Isso prova que o teste de inspeção de código realmente testa o que diz testar, nos dois sentidos, e não é uma tautologia.
- **Achado descartado como não-regressão**: numa rodada da suíte completa contra um banco Postgres que eu já tinha reutilizado repetidamente (dados acumulados de testes manuais anteriores meus, nunca truncado por completo), o pacote `cmd/server` falhou num teste de vínculo de turma/estudante — **completamente não relacionado** a esta tarefa (módulo de turmas, nada a ver com academia/admin/email). Recriei o banco do zero (`DROP DATABASE` + `CREATE DATABASE` + migrations) e rodei a suíte completa de novo: tudo `ok`, incluindo `cmd/server`. Confirmado: sujeira de dados acumulados no MEU banco de teste reutilizado, não uma regressão desta tarefa. Se você (ou eu numa sessão futura) rodar a suíte de integração, use sempre um banco limpo por execução.

## 5. Pendência fora do escopo de código: configurar o template no EmailJS

Isto **não é código** e não pode ser feito por um diff — é um passo manual no dashboard do EmailJS (mesmo serviço já usado pelo projeto para verificação de email, reset de senha e boas-vindas de admin), a ser feito por quem tem acesso a essa conta, antes ou depois do deploy:

1. Criar um novo template no EmailJS (mesmo padrão dos templates existentes: `EMAILJS_TEMPLATE_VERIFICATION`, `EMAILJS_TEMPLATE_RESET`, `EMAILJS_TEMPLATE_ADMIN_WELCOME`).
2. As variáveis (merge tags) que o código vai preencher, para usar no corpo do template: `{{user_name}}` (nome do admin), `{{academia_nome}}`, `{{academia_codigo}}`, `{{academia_nif}}`, `{{academia_nivel}}`, `{{academia_tipo}}`, `{{academia_provincia}}`, `{{painel_url}}` (já monta para `{FRONTEND_URL}/academias`). `{{to_email}}`, `{{to_name}}` e `{{from_name}}` são preenchidos automaticamente pelo serviço, como nos outros templates.
3. Sugestão de conteúdo (assunto e corpo), livre para ajustar o tom:
   - Assunto: "Nova academia cadastrada — análise pendente"
   - Corpo: avisar que a academia **{{academia_nome}}** (código **{{academia_codigo}}**, NIF {{academia_nif}}, {{academia_tipo}}, {{academia_nivel}}, província {{academia_provincia}}) acabou de se autocadastrar e está com status "inativo", pendente de análise; pedir para acessar o painel em {{painel_url}} para revisar os dados e decidir se a academia deve ser ativada.
4. Copiar o ID do template gerado para a variável de ambiente `EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA` (ver `.env.example`) no ambiente de produção (e em qualquer outro ambiente onde o aviso deva realmente ser enviado).

**Sem esse passo manual, a funcionalidade fica "instalada" mas silenciosa**: o autocadastro continua funcionando normalmente, mas cada tentativa de aviso só gera uma linha de log `[EMAIL] ⚠️ EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA não configurado...` e nenhum email sai. Isso é intencional (ver seção 1.5), não é bug.

## 6. Avisos de deploy

- Sem impacto em dados existentes: nenhuma migration, nenhuma academia ou admin já cadastrado é afetado.
- Sem impacto em NeonDB/compute: uma query nova (`GetAdminsParaNotificarNovaAcademia`), mas só roda uma vez por autocadastro público de academia — evento raro, tabela pequena, já indexada em `role` e `status`.
- Se `EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA` não estiver configurada em produção, nada quebra — só não há aviso por email até que a seção 5 seja concluída manualmente.

## 6.1 Atualização (mesmo dia): aviso ativo hoje roda pelo frontend, não por aqui

Ao configurar o EmailJS (seção 5), a conta já estava no limite de templates do plano gratuito e não deixou criar `EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA`. Em vez de esperar um upgrade, o aviso passou a ser enviado pelo **spuripainel** (NodeMailer/SMTP, mesmo caminho já usado lá para verificação de email e recuperação de senha) — ver a tarefa irmã `Tarefa - Avisar administradores por email quando uma instituicao se autocadastra (NodeMailer, frontend).md` nesse repositório.

**Nada neste documento muda por causa disso.** Tudo que está descrito e implementado aqui (`GetAdminsParaNotificarNovaAcademia`, `SendAcademiaCadastradaEmail`, a chamada em `RegisterAcademiaPublica`) continua correto e no lugar, só permanece inerte (log apenas) até `EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA` ser configurada — o que pode acontecer a qualquer momento, sem depender de nenhuma mudança de código, se o plano do EmailJS for resolvido no futuro. Os dois caminhos (este, e o do spuripainel) convivem sem conflito: cada academia autocadastrada pode, em tese, gerar aviso pelos dois ao mesmo tempo se ambos algum dia estiverem configurados — isso não foi tratado como problema porque hoje só um dos dois está de fato ativo.

## 7. Ao terminar

Mova este arquivo para `docs/Tarefas feitas/92 - Avisar por email admins com permissao de ativacao sempre que uma academia se autocadastra.md` depois que o checklist da seção 3 estiver 100% marcado.
