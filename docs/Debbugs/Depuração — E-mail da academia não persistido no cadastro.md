---
modificado: 2026-08-04 00:00
criado: 2026-08-04 00:00
---
# Depuração — E-mail da academia não persistido no cadastro

## Sintoma reportado

Ao fazer `GET /academias` ou `GET /consultar-academia/:codigo`, o campo `email` da academia
aparece vazio/nulo, mesmo quando o admin informou o e-mail no momento do cadastro
(`POST /dominis/academia/register`).

## Objetivo desta depuração

Rastrear o caminho completo do campo `email` — request → handler → aggregate → evento no
ledger → projeção → leitura — para confirmar em qual ponto o valor é perdido.

---

## Resultado geral

**Causa raiz confirmada e isolada a um único ponto do código.** O e-mail (e também o
**telefone**, pelo mesmo motivo) informado no cadastro da academia é descartado dentro do
handler `RegisterAcademia`, antes de chegar ao aggregate. Todos os demais elos da cadeia
(binding do request, evento, projeção, DTO de leitura) estão corretos.

---

## Rastreamento passo a passo

### 1. Captura do request — correta

Em `internal/handlers/academia_handlers.go`, `bindRegisterAcademiaRequest` popula
`req.Email` corretamente nos dois formatos aceitos pela rota:

```go
// multipart/form-data
if v := get("email"); v != "" {
    req.Email = &v
}
```

```go
// application/json (fallback)
if err := c.ShouldBindJSON(&req); err != nil { ... }
```

`RegisterAcademiaRequest.Email` tem `json:"email"`, então o binding JSON também funciona.
**Nada de errado aqui.**

### 2. Chamada ao aggregate — 🔴 BUG ENCONTRADO AQUI

Ainda em `internal/handlers/academia_handlers.go`, dentro de `RegisterAcademia`:

```go
academia := aggregates.NewAcademia()
if err := academia.Criar(
    req.Nivel,
    req.Type,
    req.Nome,
    req.NIF,
    codigoAcademia,
    string(hashedPassword),
    codigoProvincia,
    req.Endereco,
    nil,              // ← posição 9: deveria ser req.Telefone
    nil,              // ← posição 10: deveria ser req.Email
    req.Website,
    req.NivelEscolar,
    req.Cursos,
    req.AnosAcademicos,
    &userID,
); err != nil {
    utils.RespondWithValidationError(c, err)
    return
}
```

A assinatura do método chamado, em `internal/domain/aggregates/academia.go`, é:

```go
func (a *Academia) Criar(
    tipo string,
    academiaType string,
    nome string,
    nif string,
    codigoAcademia string,
    senhaHash string,
    provincia string,
    endereco string,
    telefone *string,   // posição 9
    email *string,      // posição 10
    website *string,
    nivelEscolar *string,
    cursos []string,
    anosAcademicos []string,
    criadoPor *uuid.UUID,
) error {
```

Ou seja: a posição 9 (`telefone`) e a posição 10 (`email`) da assinatura recebem literalmente
`nil` no lugar de `req.Telefone` e `req.Email`. O valor que o admin digitou nunca chega ao
aggregate — é descartado antes mesmo de existir um evento.

### 3. Evento gravado no ledger — consequência do bug acima

`AcademiaCriadaEvent.Email` é preenchido com o parâmetro `email` recebido por `Criar`, que já
chega como `nil`:

```go
event := &AcademiaCriadaEvent{
    ...
    Telefone:       telefone, // nil
    Email:          email,    // nil
    ...
}
```

Isso significa que o evento `AcademiaCriada` gravado no `spuri_ledger` **nunca contém o
e-mail**, mesmo que o request HTTP o tivesse. Como o ledger é apenas-anexo e é a fonte de
verdade do sistema, **não existe rebuild de projeção capaz de recuperar esse dado** para
academias já cadastradas com esse bug — o valor simplesmente nunca foi gravado no evento
original.

### 4. Projeção — correta (mas nunca recebe o dado)

`handleAcademiaCriada`, em `internal/projections/academia_projection.go`, desserializa
`payload.Email *string \`json:"Email"\`` e grava corretamente na coluna `email` via `INSERT`.
O handler está certo; ele só nunca recebe um valor não-nulo por causa do item 2.

### 5. Leitura (DTO/API) — correta

`scanAcademia` lê a coluna `email` para `AcademiaDTO.Email`, e os handlers
`ListarTodasAcademias` / `GetAcademiaPorCodigo` expõem esse campo no JSON de resposta
(`acadMap["email"] = aca.Email`, `resp["email"] = academia.Email`). Está tudo correto — o
campo só aparece vazio porque a coluna está `NULL`.

---

## Impacto

1. **Todo cadastro de academia feito via `POST /dominis/academia/register`** (síncrono ou em
   lote via `POST /dominis/academia/register/async`, que reaproveita o mesmo handler
   `RegisterAcademia`) grava a academia com `telefone = NULL` e `email = NULL` na projeção,
   independentemente do que o admin enviou no formulário.
2. Como consequência, fluxos que dependem de e-mail da academia ficam quebrados
   silenciosamente para academias novas: verificação de e-mail
   (`POST /email/verificar-email/solicitar`), recuperação de senha por e-mail, login por
   e-mail (o login por `codigo_academia` continua funcionando normalmente).
3. **Dados históricos não são recuperáveis por rebuild.** Um rebuild da projeção `academias`
   reprocessa o `AcademiaCriada` do ledger, mas o payload desse evento já não contém o
   e-mail — ele nunca existiu ali.

---

## Correção

### Correção principal — obrigatória

Em `internal/handlers/academia_handlers.go`, função `RegisterAcademia`, trocar os dois `nil`
fixos pelos valores do request:

```diff
 	academia := aggregates.NewAcademia()
 	if err := academia.Criar(
 		req.Nivel,
 		req.Type,
 		req.Nome,
 		req.NIF,
 		codigoAcademia,
 		string(hashedPassword),
 		codigoProvincia,
 		req.Endereco,
-		nil,
-		nil,
+		req.Telefone,
+		req.Email,
 		req.Website,
 		req.NivelEscolar,
 		req.Cursos,
 		req.AnosAcademicos,
 		&userID,
 	); err != nil {
 		utils.RespondWithValidationError(c, err)
 		return
 	}
```

Isso corrige tanto `email` quanto `telefone` de uma vez, pois os dois sofrem exatamente do
mesmo bug (mesma linha, dois parâmetros).

O aggregate já valida o telefone internamente (`utils.ValidatePhone`) quando não é `nil`, então
nenhuma validação extra é necessária no handler para esse campo.

### Melhoria recomendada (opcional, não bloqueante)

`Academia.Criar` não valida o **formato** do e-mail recebido (diferente de outros aggregates
do sistema, como `Admin.Criar`, que usa `emailRegex.MatchString(email)`). Recomenda-se
adicionar essa validação em `internal/domain/aggregates/academia.go`, dentro de `Criar`,
próximo à validação de telefone:

```go
if email != nil && *email != "" {
    if err := utils.ValidateEmail(*email); err != nil {
        return err
    }
}
```

Isso evita que e-mails malformados sejam aceitos silenciosamente no cadastro de academia,
alinhando o comportamento com o resto do sistema.

### Remediação de dados já afetados

Para academias **já cadastradas** antes da correção, o e-mail não pode ser restaurado via
rebuild, porque nunca foi gravado no evento. Duas opções operacionais:

1. **Preferencial:** a própria academia faz login (com `codigo_academia` + senha padrão ou
   senha já trocada) e usa a rota já existente `PUT /me/email` para definir o e-mail. Isso
   gera um evento `AcademiaDadosAtualizados` correto, com o e-mail persistido corretamente
   desta vez.
2. Levantamento administrativo: um FPP/ADM pode consultar quais academias estão com
   `email IS NULL` em `projection_academias` e contatá-las para que completem o cadastro pela
   rota acima.

Não é recomendado tentar "corrigir" o ledger diretamente (ele é apenas-anexo por design —
ver triggers `prevent_update_ledger`/`prevent_delete_ledger`/`prevent_truncate_ledger`); o
caminho correto é sempre um novo evento.

---

## Verificação sugerida após a correção

1. Teste manual: `POST /dominis/academia/register` (multipart, com `email` e `telefone`
   preenchidos) → confirmar no `spuri_ledger` que o evento `AcademiaCriada` contém
   `Email` e `Telefone` não nulos no payload:
   ```sql
   SELECT payload->>'Email', payload->>'Telefone'
   FROM spuri_ledger
   WHERE event_type = 'AcademiaCriada'
   ORDER BY id DESC LIMIT 1;
   ```
2. `GET /consultar-academia/:codigo` (autenticado) → confirmar que `email` e `telefone`
   aparecem preenchidos na resposta.
3. Repetir o mesmo teste via `POST /dominis/academia/register/async` (rota em lote), já que
   reaproveita o mesmo handler `RegisterAcademia` e está sujeita ao mesmo bug.
4. Teste automatizado recomendado (não existe hoje): um teste de handler/integração que
   cadastra uma academia com `email` e `telefone` preenchidos e verifica que
   `academia.Email`/`academia.Telefone` no aggregate resultante (ou na projeção) não estão
   nulos. Os testes atuais em `academia_criar_test.go` cobrem apenas `nivel_escolar` e não
   pegam esse tipo de regressão porque testam `aggregates.Academia.Criar` diretamente com
   argumentos corretos — o bug está no *call site* do handler, não no aggregate.

## Checklist

- [ ] `internal/handlers/academia_handlers.go` → `RegisterAcademia` → `academia.Criar(...)`
      passa `req.Telefone` e `req.Email` em vez de `nil, nil`
- [ ] (opcional) `Academia.Criar` valida formato de e-mail via `utils.ValidateEmail`
- [ ] Teste manual de cadastro síncrono confirma e-mail/telefone no ledger e na leitura
- [ ] Teste manual de cadastro assíncrono (`/async`) confirma o mesmo
- [ ] Teste automatizado adicionado para prevenir regressão
- [ ] Comunicar/gerar plano para academias já cadastradas sem e-mail completarem o cadastro
      via `PUT /me/email`
