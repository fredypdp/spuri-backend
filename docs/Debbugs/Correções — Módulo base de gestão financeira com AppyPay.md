---
modificado: 2026-07-30 23:59
criado: 2026-07-30 23:59
---
# Correções — Módulo base de gestão financeira com AppyPay

## Origem

Este documento registra as correções aplicadas a partir da depuração `docs/Debbugs/Depuração — Módulo base de gestão financeira com AppyPay.md`.

A intervenção focou nos pontos com impacto direto na segurança, auditabilidade e reconstrução do módulo financeiro AppyPay:

1. bypass de autorização para o papel `estudante` em credenciais financeiras;
2. perda de `Historico`, autoria e `motivo` durante replay/projeção;
3. ausência de `motivo` nos payloads de eventos relevantes;
4. ciphertexts embutidos na projeção pública de credenciais;
5. ausência de `decrypt()` como pré-requisito para futuro provider AppyPay real;
6. documentação imprecisa sobre o grau atual da integração HTTP com AppyPay.

## Correções implementadas

### 1. Autorização negar-por-padrão em credenciais financeiras

As funções `ListarCredenciais`, `ObterCredencial` e `TestarCredencial` deixaram de usar o padrão antigo de “permitir tudo, negar apenas quando `autorTipo == "academia"` e houver mismatch”.

Agora todas passam pela política centralizada `podeAcessarCredencial`, que só permite:

- `fpp`;
- `admin`;
- `academia`, desde que a credencial seja do contexto `academia` e tenha o mesmo `codigo_academia` do ator autenticado.

Com isso, o papel `estudante` não lista, não consulta e não testa credenciais financeiras.

Além da proteção de domínio, as rotas `/financeiro/*` foram agrupadas com `middleware.RequireAcademiaOuAdmin()` como defesa em profundidade, impedindo acesso HTTP direto por usuários autenticados que não sejam academia/admin.

### 2. `motivo` persistido no ledger e disponível no replay

Os eventos financeiros que recebem `motivo` agora incluem esse valor no payload/metadata usado pelo ledger.

Foram cobertos os fluxos de:

- alteração de status de credencial;
- alteração de modalidade de pagamento;
- cancelamento de cobrança;
- solicitação de reembolso;
- solicitação de reversão.

O helper `payloadWithMotivo` evita duplicação nos payloads derivados de credenciais/cobranças, e `record()` passa a refletir o `motivo` no `EventoFinanceiro` retornado.

### 3. Replay em memória preservando histórico auditável

O replay via `Service.RebuildProjections` passou a reconstruir entradas de `Historico` a partir de cada `LedgerEvent`.

A reconstrução agora preserva, quando disponível:

- tipo do evento;
- timestamp;
- `autor_id`;
- `autor_tipo`;
- `motivo`;
- escopo;
- contexto;
- academia;
- metadata do evento.

Também foram ampliados os campos reconstruídos de cobranças, incluindo pagador, valor, moeda, método, descrição, referência externa, `merchant_transaction_id`, `provider_charge_id` e status bruto do provider.

### 4. `FinanceiroProjection` convergindo sem empobrecer payloads

A projeção canônica `internal/projections/financeiro_projection.go` agora lê o payload projetado existente antes de aplicar um novo evento de credencial ou cobrança.

Isso evita que cada evento novo sobrescreva a linha inteira com um estado sem histórico anterior. O comportamento novo é:

1. carregar o read model atual, se existir;
2. aplicar os campos do evento recebido;
3. anexar um novo item em `Historico` com autoria/motivo vindos de `event.Metadata` e `event.Payload`;
4. gravar o read model atualizado.

Modalidade de pagamento já possuía uma estratégia parecida e foi ajustada para usar o mesmo helper de reconstrução de evento financeiro, preservando autoria e motivo.

### 5. Cofre operacional para ciphertexts AppyPay

Os campos sensíveis cifrados foram removidos do JSON público das credenciais com tags `json:"-"`:

- `CredencialAppyPay.ClientSecretEncrypted`;
- `CredencialAppyPay.WebhookSecretEncrypted`;
- `Application.APIKey`;
- `Application.APIKeyEncrypted`.

A projeção `financeiro_credenciais_appypay` passa a conter apenas metadados e máscaras. Os ciphertexts são gravados separadamente em `financeiro_segredos_appypay`, conforme a migration `098_financeiro_event_sourcing.sql` já previa.

Também foi adicionada leitura dos segredos cifrados mais recentes durante `loadPersisted`, para que um provider real consiga recuperar material operacional cifrado a partir do cofre sem depender da projeção pública.

### 6. `decrypt()` adicionado

Foi adicionada função `decrypt()` simétrica ao `encrypt()`, usando a mesma derivação de chave e AES-GCM.

Isso não implementa ainda um cliente HTTP real da AppyPay, mas remove o bloqueio técnico básico para que um futuro provider real consiga recuperar `client_secret`, `webhook_secret` e `apiKey` a partir dos ciphertexts armazenados no cofre operacional.

### 7. Documentação atualizada

`Documentação.md` foi ajustada para deixar explícito que o módulo atual prepara domínio, auditoria, credenciais e pontos de extensão, mas ainda usa `FakeProvider` e não realiza chamadas HTTP reais contra a AppyPay.

A documentação agora também aponta que `decrypt()` existe como pré-requisito operacional para a futura integração real.

## Testes adicionados

Foram adicionados testes cobrindo os cenários que estavam ausentes na depuração original:

- `TestEstudanteNaoAcessaCredenciaisFinanceiras` — garante que estudante não lista, não obtém e não testa credenciais financeiras;
- `TestReplayPreservaHistoricoEMotivo` — garante que o replay preserva histórico e motivo;
- `TestEncryptDecryptSegredoFinanceiro` — garante que um segredo cifrado por `encrypt()` é recuperável por `decrypt()`.

## Validação executada

A suíte completa foi executada com sucesso:

```bash
go test ./...
```

Resultado: todas as packages com testes passaram.

## Observações e limites mantidos

Alguns pontos documentados na depuração continuam conscientemente fora do escopo desta correção porque dependem de integração/produto adicional:

- o provider HTTP real da AppyPay ainda não foi implementado;
- reembolso, reversão e reconciliação continuam no nível de solicitação/evento base, sem chamada externa real de conclusão;
- a validação estrutural de vínculo estudante↔academia ainda depende do chamador popular `codigo_academia_estudante` até existir integração direta com a fonte canônica de matrícula/estudante nesse fluxo financeiro base.

Esses limites foram mantidos por serem compatíveis com o escopo atual de módulo base e por exigirem desenho adicional fora da correção emergencial de segurança/auditoria.
