# Configurar Google Drive com service account e Shared Drive

## Por que o erro acontece

O erro abaixo ocorre quando a API tenta criar ficheiros com uma service account numa pasta do **My Drive**:

```text
googleapi: Error 403: Service Accounts do not have storage quota ... storageQuotaExceeded
```

Service accounts não têm quota de armazenamento própria e não podem ser donas dos ficheiros. Para uploads por service account, a pasta configurada em `GOOGLE_DRIVE_ROOT_FOLDER_ID` deve ficar dentro de um **Shared Drive** do Google Workspace, ou a aplicação precisa usar OAuth/delegação para gravar em nome de um utilizador humano.

## O que é Shared Drive

Shared Drive é uma área de armazenamento do Google Workspace cujo conteúdo pertence à organização/equipa, não a um utilizador individual. Isso é diferente do **My Drive**, onde os ficheiros pertencem ao utilizador dono da conta. Para este backend, o Shared Drive é a opção recomendada porque permite que a service account crie os documentos de matrícula sem depender da quota de uma conta pessoal.

## Posso transformar a pasta atual do My Drive num Shared Drive?

Não há um botão para "converter" uma pasta do My Drive em Shared Drive. O caminho seguro é:

1. Criar um Shared Drive novo.
2. Criar dentro dele a pasta raiz do Spuri, por exemplo `spuri-storage`.
3. Mover ou copiar os ficheiros da pasta antiga do My Drive para essa pasta nova.
4. Atualizar `GOOGLE_DRIVE_ROOT_FOLDER_ID` para o ID da pasta nova dentro do Shared Drive.

Atenção: ao mover ficheiros para um Shared Drive, a propriedade passa para o Shared Drive/organização. Antes de migrar produção, confirme permissões, backups e se os links antigos ainda são necessários.

## Como criar e configurar

### 1. Confirmar pré-requisitos

- A conta precisa ser Google Workspace de trabalho/escola com Shared Drives disponíveis.
- O administrador do Workspace precisa permitir criação de Shared Drives para o utilizador, ou criar o Shared Drive por você.
- A service account precisa estar adicionada no Shared Drive com permissão que permita criar arquivos. Use, no mínimo, **Content manager**; **Manager** também funciona, mas é mais permissivo.

### 2. Criar o Shared Drive

1. Acesse `https://drive.google.com` com a conta Workspace.
2. No menu esquerdo, clique em **Shared drives** / **Drives compartilhados**.
3. Clique em **New** / **Novo**.
4. Dê um nome, por exemplo `Spuri Storage`.
5. Clique em **Create** / **Criar**.

### 3. Adicionar a service account

1. Abra o Shared Drive criado.
2. Clique no nome do Shared Drive e depois em **Manage members** / **Gerenciar membros**.
3. Adicione o e-mail da service account, que aparece no JSON de credenciais no campo `client_email`.
4. Defina a permissão como **Content manager**.
5. Salve/Envie.

### 4. Criar a pasta raiz do backend

1. Dentro do Shared Drive, crie uma pasta, por exemplo `spuri-storage`.
2. Abra essa pasta no navegador.
3. Copie o ID da URL. Exemplo:

```text
https://drive.google.com/drive/folders/1AbCDefGhIjKlMnOpQrStUvWxYz
```

Nesse caso, o `GOOGLE_DRIVE_ROOT_FOLDER_ID` é:

```text
1AbCDefGhIjKlMnOpQrStUvWxYz
```

### 5. Configurar o backend

No ambiente de produção, configure:

```env
GOOGLE_DRIVE_CREDENTIALS_PATH=data/spuri-storage.json
# ou GOOGLE_DRIVE_CREDENTIALS_JSON=<json-base64>
GOOGLE_DRIVE_ROOT_FOLDER_ID=<ID_DA_PASTA_DENTRO_DO_SHARED_DRIVE>
GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=false
```

Depois reinicie a aplicação e teste novamente `POST /solicitacao-matricula`.

## Checklist de diagnóstico rápido

Se ainda aparecer `storageQuotaExceeded`, verifique:

- `GOOGLE_DRIVE_ROOT_FOLDER_ID` é da pasta dentro do Shared Drive, não da pasta antiga no My Drive.
- A service account foi adicionada ao Shared Drive, não apenas a uma pasta isolada do My Drive.
- A permissão da service account é **Content manager** ou superior.
- O JSON/base64 usado em produção é da mesma service account adicionada no Shared Drive.
- A aplicação foi reiniciada após mudar variáveis de ambiente.
