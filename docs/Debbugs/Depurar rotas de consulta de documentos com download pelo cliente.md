# Depurar rotas de consulta de documentos com download pelo cliente

## Contexto

As rotas que consultam estudantes, solicitações de matrícula e inventários documentais precisam retornar metadados suficientes para que o cliente baixe cada PDF sem conhecer credenciais, IDs internos ou links diretos do provider de storage. O contrato vigente é que cada documento exposto em resposta de consulta contenha `path`, `file_url` e principalmente `download_url` apontando para uma rota autenticada do backend.

## Objetivo do debug

Garantir que todas as rotas de consulta de documentos retornem `download_url` utilizável pelo cliente e que esse campo seja normalizado para rotas do backend mesmo quando o metadado persistido contenha link legado do storage.

## Escopo verificado

1. Consulta global de estudante: `GET /consultar-estudante/:codigo`.
2. Consulta própria do estudante: `GET /estudante/documentos`.
3. Inventário documental da academia: `GET /academia/documentos`.
4. Listagem de solicitações da academia: `GET /academia/solicitacoes-matricula`.
5. Consulta detalhada de solicitação da academia: `GET /academia/solicitacao-matricula/:codigo`.
6. Listagem administrativa de solicitações: `GET /solicitacoes-matricula`.
7. Rotas diretas de download protegidas para academia, estudantes e solicitações.

## Correções aplicadas

- Os helpers de serialização documental passam a sobrescrever `download_url` com a rota autenticada correta do backend para o escopo da resposta.
- A consulta de estudante por código passa a retornar o mapa `documentos` com URLs de download.
- As consultas de solicitações de matrícula passam a normalizar `documentos` antes de serializar as respostas.
- Foi adicionado teste de regressão cobrindo os escopos de consulta documental para impedir retorno de link legado no campo `download_url`.
- A documentação principal foi atualizada para explicitar que o cliente deve usar `download_url` das rotas do backend e para refletir exemplos com URLs internas.

## Critérios de aceite

- [x] Todo documento retornado por helpers de consulta contém `download_url` de rota backend.
- [x] Consultas de estudante expõem documentos quando existirem.
- [x] Consultas de solicitação de matrícula expõem documentos com `download_url` normalizado.
- [x] Inventários documentais por perfil preservam URLs específicas de `/estudante/documentos/...` e `/academia/documentos/...`.
- [x] Exemplos da documentação não instruem o front end a depender de URLs diretas do storage para download.

## Observação de testes

A suíte Go não concluiu no ambiente porque o módulo externo `github.com/t3rm1n4l/go-mega` não está disponível e a tentativa de obtê-lo via proxy retornou `Forbidden`. A validação está documentada no PR e deve ser reexecutada em ambiente com dependências disponíveis.
