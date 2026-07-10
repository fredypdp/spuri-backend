# Configuração de storage com Mega

O backend usa a interface `storage.StorageProvider` para isolar handlers e regras de negócio do provedor externo de arquivos. O provedor principal é o Mega (`STORAGE_PROVIDER=mega`).

## Variáveis de ambiente

```text
STORAGE_PROVIDER=mega
MEGA_EMAIL=
MEGA_PASSWORD=
MEGA_ROOT_FOLDER=spuri
```

Para desenvolvimento e testes sem conexão externa, use:

```text
STORAGE_PROVIDER=local
MEGA_LOCAL_ROOT=data/mega_storage
```

## Adapter escolhido

A integração operacional usa MEGAcmd instalado no ambiente (`mega-login`, `mega-mkdir`, `mega-put`, `mega-ls`, `mega-get`, `mega-rm`, `mega-mv`) encapsulado pelo `MegaProvider`. A escolha preserva autenticação por e-mail/senha e cobre criação/listagem/leitura, upload, deleção, movimentação e renomeação sem vazar detalhes do Mega para handlers ou domínio.

## Migração

Não há migração automática de arquivos do Google Drive porque não existem arquivos remotos atuais a copiar. Referências antigas, se encontradas, devem ser tratadas como metadados legados; novos uploads usam Mega.

## Testes

Os testes unitários usam o provider local e não dependem de conta Mega. Testes de integração reais devem ser opt-in, com credenciais explícitas e pasta temporária isolada.
