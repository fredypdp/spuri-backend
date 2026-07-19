---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Criar fluxo de atualização de documentos de estudantes e academias com aprovação (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento criando uma nova entidade `SolicitacaoAtualizacaoDocumento`, seguindo o padrão de Event Sourcing + CQRS já usado por `SolicitacaoMatricula`, que permita a estudantes e academias submeterem uma versão atualizada de um documento já cadastrado. O arquivo enviado deve ficar num diretório temporário até ser aprovado: para documentos de estudante, quem aprova é a academia à qual o estudante está vinculado; para documentos de academia, quem aprova é um Admin. Se aprovado, o arquivo deve ser movido do diretório temporário para o diretório definitivo do documento, substituindo o anterior. Se reprovado, o diretório/arquivo temporário deve ser removido, sem nenhuma alteração no documento vigente. Deve existir um mecanismo de consulta para a academia e para o Admin saberem da existência de solicitações pendentes. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou fallbacks temporários.

## Contexto

Hoje, depois do cadastro inicial, não existe nenhum caminho para o estudante atualizar um documento de identificação (`bi_estudante`, `bi_encarregado`, `cedula_estudante`, declaração, certificado) nem para a academia atualizar seu documento formal (`alvara`). `PUT /estudante/dados-pessoais` só altera campos de texto; não há upload de arquivo. O mesmo vale para `PUT /academia/dados`.

Isso é uma lacuna real: documentos vencem, são substituídos por versões mais recentes, ou foram enviados com erro no cadastro inicial e precisam ser corrigidos sem exigir um novo cadastro completo. `Lista de tarefas.md` propõe um fluxo com aprovação, para que a substituição de um documento oficial (BI, alvará, etc.) não aconteça sem revisão humana: o arquivo enviado fica num diretório temporário e só é promovido ao diretório definitivo (substituindo o anterior) depois de aprovado por quem tem autoridade sobre aquele tipo de documento — a academia, no caso de documentos de estudante; um Admin, no caso de documentos de academia.

O desenho desta tarefa reaproveita deliberadamente o padrão já estabelecido por `SolicitacaoMatricula` (`docs/Tarefas feitas/Solicitação de matrícula + gerenciamento de arquivos pelo Mega.md`): entidade própria, eventos no ledger, atomicidade de upload antes da gravação do evento, e uso da interface `storage.StorageProvider` já existente.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Nova entidade | `SolicitacaoAtualizacaoDocumento` | Registra pedido, arquivo temporário, status e decisão, com eventos no ledger |
| Documentos de estudante | Aprovação pela academia dona do estudante | Substituição do documento só ocorre após decisão da academia |
| Documentos de academia | Aprovação por Admin | Substituição do documento só ocorre após decisão de um Admin |
| Armazenamento temporário | Diretório `pendentes/` isolado do diretório definitivo | Documento vigente nunca é afetado antes da aprovação |
| Aprovação | Move arquivo do diretório temporário para o definitivo, substituindo o anterior | Documento antigo é substituído apenas na aprovação |
| Reprovação | Remove diretório/arquivo temporário | Nenhuma mudança no documento vigente |
| Visibilidade | Nova consulta de solicitações pendentes | Academia e Admin sabem da existência da solicitação sem precisar procurar manualmente |

---

# 1. Criar a entidade `SolicitacaoAtualizacaoDocumento`

## Objetivo

Modelar o pedido de atualização de documento como uma entidade própria, com ciclo de vida `pendente → aprovada`/`reprovada`, seguindo o mesmo padrão de Event Sourcing + CQRS já usado no sistema.

## Regra de negócio

### 1.1 Campos

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `id` | UUID | Sim (gerado) | Identificador único |
| `codigo_solicitacao` | string | Sim (gerado) | Código curto único, no mesmo padrão alfanumérico já usado por `SolicitacaoMatricula` |
| `tipo_entidade` | enum | Sim | `estudante` ou `academia` |
| `codigo_referencia` | string | Sim | `codigo_estudante` ou `codigo_academia`, conforme `tipo_entidade` |
| `campo_documento` | string | Sim | Chave do documento a ser substituído (ex.: `bi_estudante`, `bi_encarregado`, `cedula_estudante`, `alvara`) |
| `path_temporario` | string | Sim (gerado) | Caminho do arquivo enviado, no diretório temporário |
| `status` | enum | Sim (gerado) | `pendente` \| `aprovada` \| `reprovada` |
| `motivo_reprovacao` | string | Não | Preenchido apenas ao reprovar |
| `solicitado_por` | string | Sim | Identificador de quem enviou (código do estudante ou da academia) |
| `decidido_por` | UUID | Não | Identificador de quem aprovou/reprovou |
| `created_at`/`updated_at` | RFC3339 | Sim (gerado) | |
| `version` | int | Sim (gerado) | |

### 1.2 Eventos do ledger

- `SolicitacaoAtualizacaoDocumentoCriada`
- `SolicitacaoAtualizacaoDocumentoAprovada`
- `SolicitacaoAtualizacaoDocumentoReprovada`

Adicionar os três à whitelist de eventos autorizados (`safe_queries.go`).

## Escopo obrigatório

### 1.3 Validação de `campo_documento`

O backend deve validar que `campo_documento` corresponde a um tipo de documento que a entidade alvo já suporta hoje (ex.: para estudante, apenas os campos de documento já usados em `POST /academia/estudante/register`/`POST /solicitacao-matricula`; para academia, apenas `alvara`). Um `campo_documento` desconhecido para o `tipo_entidade` informado deve ser rejeitado com `400`.

### 1.4 Validação do arquivo

O arquivo enviado deve seguir exatamente as mesmas regras já usadas em todo o sistema para documentos PDF: `Content-Type: application/pdf`, extensão `.pdf`, assinatura `%PDF`, limite máximo de 10MB.

---

# 2. Fluxo de submissão pelo estudante

## Objetivo

Permitir que o estudante autenticado envie uma nova versão de um documento próprio, ficando pendente de aprovação pela academia.

## Regra de negócio

Criar `POST /estudante/documentos/atualizar`, protegido por autenticação de estudante, aceitando `multipart/form-data` com `campo_documento` e o arquivo correspondente.

O backend deve:

1. validar que o estudante possui vínculo ativo com uma academia (`codigo_academia` preenchido); estudante sem academia vinculada não pode submeter esta solicitação, pois não há quem aprove;
2. validar `campo_documento` e o arquivo conforme seções 1.3 e 1.4;
3. fazer upload do arquivo para um diretório temporário isolado do diretório definitivo, por exemplo `{codigo_academia}/estudantes/{codigo_estudante}/documentos_pendentes/{campo_documento}_{codigo_solicitacao}.pdf`;
4. só gravar `SolicitacaoAtualizacaoDocumentoCriada` depois do upload concluído com sucesso;
5. se o upload falhar, não criar a solicitação e não deixar arquivo órfão.

## Escopo obrigatório

### 2.1 Um pedido pendente por campo

Não permitir mais de uma solicitação `pendente` para o mesmo `codigo_estudante` + `campo_documento` simultaneamente. Uma nova tentativa enquanto já existir uma pendente deve ser rejeitada com `409 Conflict`, orientando o estudante a aguardar a decisão da anterior.

### 2.2 Testes obrigatórios

1. estudante com academia vinculada envia atualização de `bi_estudante` válida: solicitação criada com `status = pendente`;
2. estudante sem academia vinculada tenta enviar: rejeitado;
3. arquivo inválido (não PDF, acima de 10MB): rejeitado, nenhuma solicitação criada;
4. segunda tentativa de atualização do mesmo `campo_documento` enquanto a primeira está pendente: rejeitado com `409`;
5. falha simulada de upload: nenhuma solicitação é criada e nenhum arquivo órfão permanece.

---

# 3. Fluxo de submissão pela academia

## Objetivo

Permitir que a academia autenticada envie uma nova versão de um documento próprio (ex.: `alvara`), ficando pendente de aprovação por um Admin.

## Regra de negócio

Criar `POST /academia/documentos/atualizar`, protegido por autenticação de academia ativa, com o mesmo contrato e as mesmas regras da seção 2, adaptadas para `tipo_entidade = "academia"`, armazenando o arquivo temporário em `{codigo_academia}/Documentação formal/pendentes/{campo_documento}_{codigo_solicitacao}.pdf`.

## Escopo obrigatório

Aplicar as mesmas regras de unicidade de pedido pendente (seção 2.1) e os mesmos testes obrigatórios (seção 2.2), adaptados para academia.

---

# 4. Aprovação e reprovação

## Objetivo

Permitir que a academia decida sobre solicitações de seus estudantes, e que um Admin decida sobre solicitações de academias, promovendo ou descartando o arquivo temporário.

## Regra de negócio

### 4.1 Decisão da academia sobre documento de estudante

Criar `PUT /academia/solicitacao-documento/:codigo/aprovar` e `PUT /academia/solicitacao-documento/:codigo/reprovar`, protegidos por autenticação de academia ativa, restritos a solicitações de estudantes vinculados à própria academia.

**Ao aprovar:**

1. mover o arquivo de `path_temporario` para o caminho definitivo do documento (o mesmo caminho já usado por `campo_documento` no cadastro do estudante), substituindo o arquivo anterior;
2. atualizar o mapa `documentos` do estudante com os novos metadados (`documento_id`, `path`, `file_url`, `download_url`, `versao` incrementada);
3. gravar `SolicitacaoAtualizacaoDocumentoAprovada` no ledger da solicitação **e** um evento correspondente no ledger do estudante, para que a substituição do documento seja auditável a partir do histórico do próprio estudante;
4. atualizar `status = "aprovada"`.

**Ao reprovar:**

1. exigir `motivo_reprovacao` não vazio;
2. remover o arquivo/diretório temporário do storage;
3. gravar `SolicitacaoAtualizacaoDocumentoReprovada`;
4. o documento vigente do estudante permanece inalterado.

### 4.2 Decisão do Admin sobre documento de academia

Criar `PUT /admin/solicitacao-documento/:codigo/aprovar` e `PUT /admin/solicitacao-documento/:codigo/reprovar`, protegidos por autenticação de Admin (qualquer role, salvo decisão explícita em contrário registrada no PR), com a mesma lógica da seção 4.1 aplicada à academia e ao evento correspondente no ledger da academia.

## Escopo obrigatório

### 4.3 Atomicidade na aprovação

Se a movimentação do arquivo do diretório temporário para o definitivo falhar, a solicitação **não** deve ser marcada como aprovada nem o evento `SolicitacaoAtualizacaoDocumentoAprovada` deve ser gravado; o erro deve ser retornado de forma clara, permitindo nova tentativa.

### 4.4 Solicitação decidida é terminal

Uma solicitação `aprovada` ou `reprovada` não pode ser decidida novamente. Tentativas subsequentes devem retornar `409 Conflict`.

### 4.5 Testes obrigatórios

1. academia aprova solicitação de documento de seu estudante: arquivo definitivo é substituído, `documentos` do estudante atualizado, evento gravado no estudante;
2. academia reprova solicitação de seu estudante com `motivo_reprovacao`: arquivo temporário removido, documento vigente inalterado;
3. academia tenta decidir solicitação de estudante de **outra** academia: rejeitado com `403`;
4. Admin aprova solicitação de documento de academia: arquivo definitivo substituído, evento gravado na academia;
5. Admin reprova solicitação de academia: arquivo temporário removido, documento vigente inalterado;
6. reprovação sem `motivo_reprovacao`: rejeitado;
7. tentativa de decidir uma solicitação já decidida: rejeitado com `409`;
8. falha simulada na movimentação do arquivo na aprovação: solicitação permanece `pendente`, nenhum evento de aprovação é gravado.

---

# 5. Consulta de solicitações pendentes

## Objetivo

Garantir que a academia e o Admin tenham como saber, sem busca manual, quais solicitações de atualização de documento estão aguardando decisão.

## Regra de negócio

Criar:

- `GET /academia/solicitacoes-documentos` — lista as solicitações de documentos de estudantes vinculados à academia autenticada, com filtro opcional por `status`;
- `GET /admin/solicitacoes-documentos-academias` — lista as solicitações de documentos de academias, restrito a Admin, com filtro opcional por `status`.

Ambas devem seguir o mesmo padrão de paginação (`limit`/`offset`, padrão 50, teto 100) já usado em outras listagens do sistema.

## Escopo obrigatório

### 5.1 Testes obrigatórios

1. academia lista solicitações pendentes de seus estudantes: retorna apenas as da própria academia;
2. Admin lista solicitações pendentes de academias: retorna todas, independentemente da academia;
3. filtro por `status` funciona corretamente nas duas rotas;
4. paginação respeita o teto de 100 itens por página mesmo que o cliente peça mais.

---

# 6. Atualização obrigatória da documentação

Atualizar `Documentação.md` com:

- a nova entidade `SolicitacaoAtualizacaoDocumento`, seus campos, eventos e ciclo de vida;
- os seis novos endpoints (submissão por estudante e academia, aprovação/reprovação por academia e Admin, consultas de listagem);
- a estrutura de diretórios temporários usada;
- a atualização do mapa `documentos` do estudante/academia após aprovação, incluindo o campo `versao`.

---

# Fora de escopo

- Permitir que o próprio estudante ou a própria academia aprovem sua própria solicitação.
- Notificações por email/push desta funcionalidade (fica para tarefa futura, se necessário).
- Alterar os fluxos de cadastro inicial (`POST /academia/estudante/register`, `POST /solicitacao-matricula`, `POST /dominis/academia/register`); esta tarefa cobre apenas atualização posterior de documento já cadastrado.
- Permitir mais de uma solicitação pendente simultânea para o mesmo documento.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `SolicitacaoAtualizacaoDocumento` existir com os três eventos auditáveis na whitelist;
2. estudante conseguir submeter atualização de documento, ficando pendente até decisão da academia;
3. academia conseguir submeter atualização de documento próprio (`alvara`), ficando pendente até decisão de Admin;
4. aprovação substituir o documento definitivo apenas após sucesso da movimentação do arquivo;
5. reprovação remover o arquivo temporário sem alterar o documento vigente;
6. existir consulta de solicitações pendentes para academia e para Admin;
7. `Documentação.md` estar atualizada com a nova entidade e os novos endpoints;
8. testes automatizados cobrirem os cenários das seções 2.2, 3, 4.5 e 5.1;
9. o PR explicar claramente o novo fluxo e sua relação com `SolicitacaoMatricula`.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Criar fluxo de atualização de documentos de estudantes e academias com aprovação (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
