---
criado: 2026-07-20 00:00
modificado: 2026-07-20 00:00
---
# Depurar rotas async e unicidade de cadastros

## Objetivos da auditoria

1. Garantir que todas as rotas públicas terminadas em `/async` realmente criem job de background e possam ser acompanhadas por `GET /jobs/:id` ou `GET /jobs/stream`.
2. Garantir que o cadastro assíncrono de academias e estudantes não aceite repetição de códigos gerados.
3. Corrigir especificamente `POST /academia/estudante/register/async` com arquivos para também ser assíncrono, mantendo a unicidade dos estudantes.

## Busca ampla executada

```bash
rg -n "async" cmd/server internal/handlers internal/jobs -g '*.go'
rg -n "RegisterHandler|JobType" cmd/server/main.go internal/jobs internal/handlers -g '*.go'
rg -n "GenerateUniqueCodigo|codigo_academia|codigo_estudante" internal/handlers internal/utils migrations -g '*.go' -g '*.sql'
```

## Resultado encontrado

| Área | Situação antes da correção | Correção/garantia |
| --- | --- | --- |
| Demais rotas `/async` de academia/admin | Já chamavam `enqueueAsyncBatch` ou fluxo específico de job, retornando `202` com `job_id`. | Mantido. |
| `POST /academia/estudante/register/async` JSON sem arquivos | Já havia sido ajustado para job. | Mantido. |
| `POST /academia/estudante/register/async` multipart com arquivos | Ainda processava imediatamente como lote síncrono. | Agora valida e lê os PDFs no enqueue, serializa os bytes no payload do job e retorna `202` para monitoramento posterior. |
| Worker de estudantes | Tinha adapter de item e `JobTypeRegisterEstudanteBatch` registrado. | O adapter agora processa itens com arquivos como cadastro completo, e itens sem arquivos em `pendente_documentos`. |
| Unicidade de estudantes | O cadastro chamava `GenerateUniqueCodigoEstudante`, mas a auditoria exigiu fechar também a janela de concorrência antes da projeção/ledger. | Criada `codigo_estudante_reservas`; a geração agora consulta projeção, ledger e reserva, e só retorna depois de reservar o código. |
| Unicidade de academias | O cadastro gera `codigo_academia` pela função de banco `spuri_generate_codigo_academia`, mas a auditoria exigiu fechar a janela concorrente entre geração e gravação do evento. | Criada `codigo_academia_reservas`; a função SQL e o fallback Go só retornam código depois de reservar, e o worker async continua reutilizando `RegisterAcademia`. |

## Validação

- Todas as rotas `/async` de lote passam por jobs ou, quando específicas, expõem job de background próprio.
- O cadastro de estudante com arquivos deixou de ser a exceção síncrona e agora devolve `job_id` como os demais.
- O cadastro assíncrono de estudantes continua usando a geração única de `codigo_estudante` no handler reutilizado pelo worker, agora com reserva transacional prévia.
- O cadastro assíncrono de academias continua reutilizando o handler singular, que delega a geração e reserva do código para `spuri_generate_codigo_academia`.
