---
criado: 2026-07-11 00:00
origem: solicitação do usuário
status: feito
---

# Substituir MEGAcmd por go-mega no storage Mega (feito)

## Prompt recomendado para executar a atualização

Substitua a integração do storage Mega baseada em processos externos do MEGAcmd por uma biblioteca Go de API Mega usada pela comunidade, validando antes se ela atende às operações atuais do sistema: autenticação com `MEGA_EMAIL` e `MEGA_PASSWORD`, criação de diretórios, upload, download, listagem, exclusão, mover/renomear arquivos e consulta de quota. Garanta timeouts explícitos, preserve o provider local para testes e documente a decisão técnica.

## Contexto

A integração anterior dependia do MEGAcmd instalado no ambiente de deploy e de uma sessão persistente local. Isso podia causar dois problemas operacionais: deploy preso quando a autenticação era executada na inicialização e requisições aguardando comandos externos sem resposta previsível quando a sessão ou as credenciais estavam inválidas.

A biblioteca escolhida foi `github.com/t3rm1n4l/go-mega`, pois a documentação pública da biblioteca informa suporte direto às demandas atuais do sistema: login de usuário, árvore de arquivos, upload, download, criação de diretório, mover, renomear, deletar, upload/download paralelo, eventos de filesystem e testes.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Cliente Mega | Trocar MEGAcmd por `github.com/t3rm1n4l/go-mega` | Remover dependência de binários e sessões externas no deploy |
| Autenticação | Login pela API Go com timeout e retries reduzidos | Falhar com erro sanitizado quando credenciais estiverem inválidas |
| Operações de arquivo | Usar métodos da biblioteca para diretórios, upload, download, listagem, mover, renomear e deletar | Preservar a interface `StorageProvider` atual |
| Testes locais | Manter provider local por `STORAGE_PROVIDER=local` ou `ENV=test` | Evitar chamadas reais ao Mega durante testes automatizados |
| Quota | Usar `GetQuota` da biblioteca para totais remotos | Expor total, usado e disponível quando suportado pela conta |

---

# 1. Verificação da biblioteca

## Objetivo

Confirmar que a biblioteca escolhida atende ao escopo atual do storage antes de substituir a integração.

## Critérios verificados

1. login com e-mail e senha;
2. leitura da árvore remota;
3. criação de diretórios;
4. upload de arquivo;
5. download de arquivo;
6. listagem de filhos de uma pasta;
7. exclusão de arquivo ou diretório;
8. mover arquivo entre pastas;
9. renomear arquivo;
10. consulta de quota da conta;
11. operação sem depender de sessão persistente do MEGAcmd.

## Resultado

A biblioteca atende às operações usadas pelo `StorageProvider` atual. A única limitação mantida é que a análise detalhada de uso por academia no Mega remoto continua dependente da árvore de arquivos, enquanto a quota remota retorna os totais fornecidos pela API.

---

# 2. Alteração técnica implementada

## Objetivo

Remover a dependência de processos externos do MEGAcmd e executar as operações Mega via cliente Go.

## Escopo implementado

1. substituição de `exec.Command`/MEGAcmd por cliente `go-mega`;
2. login no Mega via `client.Login`;
3. configuração de timeout e retries no cliente;
4. serialização das operações remotas por mutex para manter estado do cliente consistente;
5. timeout por operação para evitar espera eterna;
6. sanitização de erros para não vazar `MEGA_EMAIL` ou `MEGA_PASSWORD`;
7. criação incremental de diretórios remotos;
8. upload via arquivo temporário e `UploadFile`;
9. download via arquivo temporário e `DownloadFile`;
10. listagem via `FS.GetChildren`;
11. exclusão via `Delete`;
12. mover/renomear via `Move` e `Rename`;
13. quota remota via `GetQuota`.

---

# 3. Validações esperadas

## Testes obrigatórios

1. `go test ./internal/storage` deve validar o modo local e configurações básicas;
2. `go test ./...` deve validar que a alteração não quebrou o restante do backend;
3. em ambiente com rede e credenciais reais, inicializar com `STORAGE_PROVIDER=mega`, `MEGA_EMAIL` e `MEGA_PASSWORD` deve autenticar e operar sem MEGAcmd instalado;
4. senha inválida em `MEGA_PASSWORD` deve retornar erro de autenticação sanitizado;
5. deploy não deve depender de binários `mega-login`, `mega-put`, `mega-get`, `mega-ls`, `mega-rm` ou `mega-mv`.

---

# 4. Observações operacionais

## Variáveis preservadas

- `STORAGE_PROVIDER=mega` continua ativando o Mega remoto;
- `STORAGE_PROVIDER=local` continua usando filesystem local;
- `ENV=test` continua forçando fallback local para testes;
- `MEGA_LOCAL_ROOT` continua definindo a raiz local;
- `MEGA_ROOT_FOLDER` continua definindo a pasta raiz remota dentro da conta Mega.

## Risco controlado

A biblioteca é de comunidade e não SDK oficial da MEGA, mas é a opção Go pública encontrada que cobre diretamente as operações do sistema sem depender de binários externos. Caso a conta use MFA obrigatório, será necessário evoluir configuração para informar o token de múltiplo fator.
