---
criado: 2026-08-16
origem: Levantamento feito para orquestrar a tarefa de frontend "Frontend do módulo de pagamentos"
repositório afetado: spuri-backend
tipo: problema/gap de backend, não é uma tarefa de frontend
status: pendente de virar tarefa
---

# Dois problemas de backend encontrados no módulo de pagamentos

Este documento descreve dois problemas encontrados ao ler o código-fonte do backend (`spuri-backend`, branch `main`) enquanto eu preparava a tarefa de frontend do módulo de pagamentos. Nenhum dos dois é uma limitação inventada ou uma preferência de design — são comportamentos confirmados lendo os arquivos indicados abaixo. Os dois impedem partes do que foi pedido para o módulo de pagamentos e por isso a tarefa de frontend teve o escopo ajustado para contornar cada um deles. Este documento serve de insumo para abrir uma tarefa de correção no backend.

---

## Problema 1 — Não existe nenhum endpoint de listagem de pagamentos/cobranças

### Para que eu precisava disso

A tarefa de frontend pedia uma tela `/financas/pagamentos`, para academia e administrador, com a seguinte descrição: **"Visualizar a lista de todos os pagamentos deles em todos os estados, permitindo a abertura de subtelas para ver com mais detalhes cada cobrança/pagamento."** Isso pressupõe uma rota que devolva, para uma academia (ou para o admin, em nome de uma academia), o conjunto de cobranças/pagamentos daquele contexto — de mensalidade e de matrícula —, com filtro por estado (pendente, pago, cancelado etc.), para popular uma tabela/lista navegável.

### O que a API oferece hoje

Levantei **todas** as rotas de leitura relacionadas a cobrança/pagamento existentes no backend (`cmd/server/main.go`, grupo `/financeiro`, mais as rotas de solicitação de matrícula):

| Rota | O que retorna | Exige saber de antemão |
| --- | --- | --- |
| `GET /financeiro/appypay/cobrancas/:id` | Uma cobrança específica | O `id` interno ou o `merchantTransactionId` da cobrança |
| `GET /financeiro/mensalidades/estudante/:codigo` | As mensalidades (propinas) de um estudante específico, em todas as academias | O `codigo_estudante` |
| `GET /solicitacoes-matricula` (admin) / `GET /academia/solicitacoes-matricula` (academia) | Lista de solicitações de matrícula, agora incluindo `valor_matricula`/`metodos_pagamento_matricula` quando aplicável | Nada — mas é uma lista de *solicitações*, não de *cobranças*; uma solicitação pode não ter cobrança nenhuma associada visível (ver Problema 2 sobre a falta de vínculo entre solicitação e cobrança) |

Não existe nenhuma rota do tipo "listar cobranças de uma academia" ou "listar todos os pagamentos", com ou sem filtro de estado. Confirmei isso de duas formas:

1. **Nas rotas registradas** (`cmd/server/main.go`, grupo `/financeiro` a partir da linha ~341): as únicas rotas de leitura no grupo são `GET /appypay/cobrancas/:id`, `GET /appypay/credenciais`, `GET /mensalidades/configuracoes`, `GET /matriculas/configuracoes` e `GET /mensalidades/estudante/:codigo` — nenhuma delas é uma listagem de cobranças.
2. **Na camada de projeção** (`internal/projections/financeiro_projection.go`), que é onde ficaria a lógica de consulta caso existisse: o arquivo só tem métodos de **escrita** da projeção (`upsertMensalidadeCobrancas`, `Handle`, `Rebuild`, `ApplyNow`, `ApplyLatestForAggregate`). Não há nenhum método de consulta/listagem — nem uma função "pronta mas não exposta por rota". Ou seja, a funcionalidade não existe em nenhuma camada, não é só uma rota faltando por cima de uma consulta já pronta.

### Por que isso não atende à tarefa

Sem essa rota, não há como montar a tela `/financas/pagamentos` como uma lista real de "todos os pagamentos, em todos os estados". O único jeito de ver uma cobrança é já sabendo o identificador dela, e o único jeito de ver mensalidades é já sabendo o código de um estudante específico — ou seja, dá para *consultar um item pontual*, mas não para *descobrir quais itens existem*. Isso é qualitativamente diferente do que foi pedido: uma tela de gestão financeira precisa permitir que a academia veja, sem saber de antemão nenhum identificador, tudo que está pendente, tudo que já foi pago, etc.

### O que ficou combinado no frontend por causa disso

A tela `/financas/pagamentos` foi redesenhada como três ferramentas de busca pontual (por estudante, por identificador de cobrança, e a lista de solicitações de matrícula que já tem valor de pagamento) em vez de uma listagem geral — um contorno, não uma solução.

### Sugestão de correção (para dimensionar a tarefa futura)

Precisaria de:
- Uma nova consulta na projeção financeira (`internal/projections/financeiro_projection.go`) capaz de listar cobranças por `codigo_academia` (e/ou contexto `spuri` para o admin), com filtros por `estado`/`status` e paginação — seguindo o mesmo padrão de paginação já usado em outras listagens do sistema (ex.: `GET /estudantes`, `GET /solicitacoes-matricula`).
- Uma rota nova, algo como `GET /financeiro/cobrancas?codigo_academia=&estado=&limit=&offset=`, com a mesma autorização já usada nas demais rotas de `/financeiro` (`authorizeFinanceScope`, em `internal/handlers/financeiro_handlers.go`).
- Decidir se essa listagem deve unificar cobranças de mensalidade e de matrícula num só resultado (com um campo indicando a origem) ou expor duas rotas separadas — a tela de frontend consegue se adaptar a qualquer uma das duas abordagens, mas isso precisa ser decidido antes de implementar.

---

## Problema 2 — Pagamento por QR Code (`GPO_QR`) não devolve o QR Code para quem paga

### Para que eu precisava disso

Duas telas da tarefa de frontend dependem de mostrar ao pagador o método de pagamento escolhido, incluindo QR Code quando a academia o tiver habilitado como opção:
- A página pública de solicitação de matrícula (candidato pagando a taxa de matrícula).
- A página `/pagamentos` do estudante (pagando mensalidade/propina).

Nos dois casos, se o método selecionado for `GPO_QR`, a expectativa é que a tela mostre a imagem/conteúdo do QR Code para o pagador escanear.

### O que a API faz hoje

O backend gera o QR Code normalmente — o problema é só na resposta que chega até o pagador. Rastreei a estrutura exata dos tipos envolvidos:

```go
// internal/finance/appypay.go
type ChargeResult struct {
    ID                    uuid.UUID
    ProviderChargeID      string
    MerchantTransactionID string
    Status                string
    Response              map[string]any
}

type QRCodeResult struct {
    ChargeResult
    QRCodeArr string `json:"qrCodeArr,omitempty"`   // conteúdo do QR Code
}
```

`QRCodeResult` é `ChargeResult` mais o campo `QRCodeArr`, que é onde o conteúdo do QR Code fica. Só que as duas respostas que chegam até o pagador final foram declaradas com o tipo mais estreito (`ChargeResult`, sem o campo extra):

```go
// internal/finance/mensalidade.go
type MensalidadePagamentoView struct {
    Charge ChargeResult `json:"cobranca"`
    Meses  []MensalidadeSelecaoMes
}

// internal/finance/matricula.go
type MatriculaPagamentoView struct {
    Charge ChargeResult `json:"cobranca"`
}
```

E quando o método de pagamento é `GPO_QR`, o código que monta a resposta faz isto (mesmo padrão nos dois arquivos):

```go
qr, err := s.CreateGPOQRCode(...)                     // qr é QRCodeResult, TEM o QR Code
return MensalidadePagamentoView{Charge: qr.ChargeResult}, nil   // pega só a parte ChargeResult de dentro de qr
```

`qr.ChargeResult` acessa apenas a sub-struct embutida — o campo `QRCodeArr` nunca chega a existir dentro do `Charge` da resposta, porque o tipo `Charge` (`ChargeResult`) não tem espaço para ele. Isso acontece dentro do próprio código Go, antes de virar JSON — não é um problema de serialização, de rede, nem do provedor de pagamento (AppyPay). O QR Code é gerado e fica salvo (é possível recuperá-lo depois via `existingQRCodeResult`, usado internamente), só não é devolvido nessas duas respostas específicas.

A única rota que devolve o `qrCodeArr` de fato é `GET /financeiro/appypay/cobrancas/:id` (`internal/handlers/financeiro_handlers.go`, função `ConsultarCobrancaAppyPay`), que consulta uma cobrança já existente — só que essa rota é restrita a academia/admin (`authorizeFinanceScope`), não ao estudante nem ao candidato anônimo que efetivamente pagou.

### Por que isso não atende à tarefa

Se um estudante ou um candidato à matrícula escolher `GPO_QR` como método de pagamento, a cobrança é criada e fica pendente no AppyPay, mas a tela não tem como mostrar a ele nenhum QR Code — nem na resposta do próprio pagamento, nem em nenhuma consulta posterior que ele tenha permissão de fazer. Não há também nenhum outro campo de resposta que ajude a compensar isso (ex.: um link para a imagem do QR Code hospedada em algum lugar) — o dado simplesmente não sai do backend para esse ator nesse fluxo.

### O que ficou combinado no frontend por causa disso

Nas duas telas voltadas ao pagador final (matrícula pública e pagamentos do estudante), `GPO_QR` não é oferecido como opção selecionável de pagamento, mesmo que a academia o tenha habilitado na configuração — um contorno de UX, não uma solução. O método continua disponível normalmente como opção de *configuração* (a academia pode marcá-lo como aceito) e continua funcionando integralmente no fluxo interno de cobrança feita por academia/admin (que usa `GET /financeiro/appypay/cobrancas/:id` e recebe o `qrCodeArr` corretamente).

### Sugestão de correção (para dimensionar a tarefa futura)

Duas abordagens possíveis, a decidir por quem for corrigir:

1. **Trocar o tipo do campo `Charge`** em `MensalidadePagamentoView` e `MatriculaPagamentoView` de `ChargeResult` para `QRCodeResult` (ou adicionar um campo irmão `qr_code_arr,omitempty` ao lado de `Charge`), e garantir que o valor de `QRCodeArr` gerado por `CreateGPOQRCode` seja propagado até a resposta, em vez de descartado na atribuição `Charge: qr.ChargeResult`. É uma mudança pequena e localizada nesses dois arquivos (`internal/finance/mensalidade.go` e `internal/finance/matricula.go`), sem precisar mexer em `appypay.go` nem no provedor externo.
2. Alternativa, caso haja alguma razão para não expor o QR Code diretamente nessas respostas: criar uma rota de consulta segura e escopada ao próprio pagador (ex.: o estudante autenticado consultando a própria cobrança em aberto, ou o candidato consultando via o mesmo código de solicitação já usado no fluxo público) para buscar o QR Code depois de iniciar o pagamento.

A abordagem 1 é mais simples e resolve o problema na origem; a abordagem 2 evita aumentar o tamanho da resposta de pagamento, mas exige uma rota nova com sua própria autorização.

---

## Resumo para quem for escrever a tarefa de correção

| # | Problema | Está em | Tamanho estimado da correção |
| --- | --- | --- | --- |
| 1 | Sem endpoint de listagem de cobranças/pagamentos por academia | Rota nova + consulta nova na projeção (nada disso existe hoje) | Médio/alto — funcionalidade nova, não um ajuste |
| 2 | `qrCodeArr` descartado nas respostas de pagamento de mensalidade e matrícula | `internal/finance/mensalidade.go` e `internal/finance/matricula.go`, na construção de `MensalidadePagamentoView`/`MatriculaPagamentoView` quando `metodo_pagamento = "GPO_QR"` | Baixo — ajuste de tipo/atribuição, lógica de geração do QR Code já existe e funciona |
