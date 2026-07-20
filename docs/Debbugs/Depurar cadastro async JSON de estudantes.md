---
criado: 2026-07-20 00:00
modificado: 2026-07-20 00:00
---
# Depurar cadastro async JSON de estudantes

## Objetivo da auditoria

Verificar se `POST /academia/estudante/register/async` no modo JSON sem arquivos (`com_arquivo:false`) possui o mesmo mecanismo dos demais endpoints `/async` em lote: a API deve aceitar a requisição rapidamente, devolver um `job_id` e permitir acompanhar depois o progresso/desempenho por `GET /jobs/:id` ou `GET /jobs/stream`.

## Busca ampla executada

Comandos usados na auditoria:

```bash
rg -n "academia/estudante/register|register/async|async" . -g '!node_modules' -g '!vendor'
rg -n "JobType.*Estudante|RegisterEstudante|processarCadastroEstudanteBatch|BatchAsync" internal/jobs internal/handlers -g '*.go'
rg -n "RegisterHandler|JobTypeRegisterEstudanteBatch|RegisterEstudantePorAcademiaJobItem" cmd internal -g '*.go'
```

## Resultado encontrado

| Item auditado | Resultado |
| --- | --- |
| Rota `POST /academia/estudante/register/async` | Estava ligada a `RegisterEstudanteBatch`, que processava JSON sem arquivos de forma síncrona e só respondia depois de iterar o lote inteiro. |
| Infraestrutura de jobs | Já existia `JobTypeRegisterEstudanteBatch`, registro do worker e adapter `RegisterEstudantePorAcademiaJobItem`. |
| Contrato JSON sem arquivos | Já possuía wrapper `{com_arquivo:false, estudantes:[...]}`, diferente do array bruto dos demais `/async`. |
| Modo multipart com arquivos | Depende dos arquivos enviados no request e permanece como lote imediato, porque o job store persiste JSON e não os streams de upload. |

## Correções aplicadas

1. O modo JSON sem arquivos de `POST /academia/estudante/register/async` agora valida `com_arquivo:false`, tamanho máximo e lote vazio no enqueue, cria um job `register_estudante_batch` e retorna `202 Accepted` com `job_id`, `poll_url` e `sse_url`.
2. O helper comum de resposta de job foi extraído para reaproveitar exatamente o envelope de monitoramento dos demais endpoints `/async`.
3. O worker de cadastro de estudante agora processa cada item em modo `pendente_documentos`, preservando a regra funcional do JSON sem arquivos: valida campos textuais e não cobra PDFs no primeiro passo.
4. A documentação foi atualizada novamente na auditoria seguinte para deixar claro que tanto JSON sem arquivos quanto multipart com arquivos criam job de background.

## Validação

- O endpoint JSON sem arquivos passa a permitir o fluxo esperado: enviar a requisição, receber `job_id`, e monitorar progresso/resultados por polling ou SSE.
- O limite público continua 100 estudantes por requisição.
- O contrato específico `{com_arquivo:false, estudantes:[...]}` foi preservado; apenas o mecanismo de execução mudou para background.
