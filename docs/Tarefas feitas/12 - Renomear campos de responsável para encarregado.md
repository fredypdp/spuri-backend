---
criado: 2026-07-19 00:00
origem: solicitação do usuário
status: feito
---

# Renomear campos de "responsável" para "encarregado" (feito)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento substituindo, em todo o sistema, o termo "encarregado" por "encarregado" — nos campos `bilhete_identidade_encarregado`, `telefone_encarregado`, `telefone_encarregado_verificado` e no campo de arquivo `bi_encarregado` (incluindo a chave equivalente no mapa `documentos`), e em todo texto relacionado a esses campos (mensagens de erro, mensagens de sucesso, documentação técnica, comentários de código). Os nomes antigos devem deixar de ser aceitos em qualquer requisição e deixar de aparecer em qualquer resposta. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a código legado, aliases, wrappers de compatibilidade, fallbacks temporários ou período de transição para os nomes antigos.

## Contexto

O sistema usa hoje, em `Estudante` e em `SolicitacaoMatricula`, os campos `bilhete_identidade_encarregado`, `telefone_encarregado` e `telefone_encarregado_verificado`, além do campo de upload `bi_encarregado` (e da chave `bi_encarregado` no mapa `documentos`), para representar o Bilhete de Identidade e o telefone da pessoa legalmente encarregado pelo estudante. A decisão de produto agora é adotar o termo "encarregado" no lugar de "encarregado" em todo o sistema — campos, mensagens de erro, documentação e comentários — de forma definitiva, sem manter os nomes antigos em paralelo.

Todas as ocorrências de "encarregado"/"encarregado" no sistema se referem exclusivamente a esse papel (a pessoa encarregado legal pelo estudante); não há nenhum outro uso do termo em outro sentido de negócio (campos como `aprovada_por`, `reprovada_por`, `definido_por`, `alterado_por` não usam a palavra "encarregado" e não são afetados por esta tarefa). Isso torna a substituição direta e sem ambiguidade, mas o volume de ocorrências é grande: o termo aparece em `EstudanteDTO`, `SolicitacaoMatriculaDTO`, nos campos de texto e de arquivo de `POST /academia/estudante/register`, `POST /academia/estudante/register/async`, `POST /academia/estudante/{codigo_estudante}/documentos`, `PUT /estudante/dados-pessoais`, `POST /solicitacao-matricula`, nas regras automáticas de documentos de matrícula, nas mensagens de erro de BI e nos exemplos de payload em `Documentação.md`.

> **Nota técnica sobre event sourcing — isto não é suporte legado.** Eventos já gravados no `spuri_ledger` antes desta mudança guardam, no `payload` histórico, os nomes antigos dos campos, e esses eventos são imutáveis: reescrevê-los quebraria a cadeia de hashes (o mesmo mecanismo de integridade auditado na tarefa "Validar e reforçar a integridade do event sourcing e dos rebuilds" deste conjunto). Por isso, o código de aplicação de eventos usado internamente pelo rebuild de projeções precisa continuar capaz de interpretar corretamente o payload desses eventos antigos — isso é uma exigência técnica inevitável de um sistema de Event Sourcing com ledger imutável, e não uma forma de suporte legado de API. Nenhum endpoint, DTO, mensagem de erro, OpenAPI/Swagger ou documentação deve aceitar, expor, mencionar ou testar os nomes antigos como opção válida de uso pelo cliente.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Nomenclatura de campos | `bilhete_identidade_encarregado` → `bilhete_identidade_encarregado`; `telefone_encarregado` → `telefone_encarregado`; `telefone_encarregado_verificado` → `telefone_encarregado_verificado`; `bi_encarregado` → `bi_encarregado` | "Encarregado" passa a ser o único nome aceito e exposto em todo o sistema |
| Entrada (requests) | Aceitar exclusivamente o nome novo; rejeitar o nome antigo | Nenhum caminho de burla ou aceitação silenciosa do nome antigo |
| Saída (responses) | Expor exclusivamente o nome novo | Nenhuma duplicidade de campo, nenhum alias em nenhuma resposta |
| Persistência/projeções | Migrar colunas e metadados de documentos já existentes para o nome novo | Nenhuma coluna, chave ou registro ativo com o nome antigo |
| Eventos históricos no ledger | Preservados como estão (imutáveis); interpretados corretamente apenas internamente pelo rebuild | Rebuild continua correto sem reintroduzir o nome antigo no contrato público |
| Textos (erro, sucesso, documentação, comentários) | Atualizar integralmente para "encarregado" | Nenhuma menção ativa a "encarregado" fora do histórico imutável do ledger |

---

# 1. Levantamento e nomenclatura oficial dos campos afetados

## Objetivo

Mapear, de forma exaustiva, todas as ocorrências de "encarregado"/"encarregado" no código e na documentação, e fixar a nomenclatura oficial de substituição antes de qualquer alteração.

## Regra de negócio

| Nome atual (a remover) | Nome novo (único aceito) | Onde aparece |
| --- | --- | --- |
| `bilhete_identidade_encarregado` | `bilhete_identidade_encarregado` | `EstudanteDTO`, `SolicitacaoMatriculaDTO`, campos de texto de `POST /academia/estudante/register` (e `/async`), `PUT /estudante/dados-pessoais`, `POST /solicitacao-matricula` |
| `telefone_encarregado` | `telefone_encarregado` | `EstudanteDTO`, campos de `POST /academia/estudante/register` (e `/async`), `POST /solicitacao-matricula` |
| `telefone_encarregado_verificado` | `telefone_encarregado_verificado` | `EstudanteDTO` |
| `bi_encarregado` (campo de arquivo) | `bi_encarregado` | `POST /academia/estudante/register`, `POST /academia/estudante/register/async` (inclusive no padrão `<codigo_temporario>.bi_encarregado`), `POST /academia/estudante/{codigo_estudante}/documentos`, `POST /solicitacao-matricula` |
| `documentos.bi_encarregado` (chave no mapa de documentos) | `documentos.bi_encarregado` | Respostas de estudante e de solicitação de matrícula, rotas de download de documento |
| "BI do encarregado", "telefone do encarregado", "encarregado" em prosa | "BI do encarregado", "telefone do encarregado", "encarregado" | Mensagens de erro, `Documentação.md`, comentários de código |

## Escopo obrigatório

### 1.1 Busca ampla obrigatória

Antes de alterar qualquer código, executar busca ampla e classificar cada ocorrência:

```bash
rg -n -i "encarregado|encarregado" .
```

Cada ocorrência deve ser classificada como: campo/texto ativo a renomear sem deixar resquício, evento histórico do ledger (dado imutável, não deve ser alterado), documentação de tarefa já concluída em `docs/Tarefas feitas/` (registro histórico, não deve ser reescrita), ou falso positivo.

### 1.2 Não reescrever documentação histórica nem eventos do ledger

Arquivos já existentes em `docs/Tarefas feitas/` (incluindo `Cadastro de estudante escolar - BI do encarregado obrigatório.md`) são registros históricos de tarefas concluídas e **não devem ser reescritos**. Eventos já gravados no ledger também não devem ser alterados, por serem imutáveis (ver nota técnica no Contexto). Apenas `Documentação.md` (documentação viva), o código-fonte ativo e os dados de projeção (que são reconstruíveis, ao contrário do ledger) devem refletir a nova nomenclatura.

---

# 2. Substituir totalmente os nomes de campo na entrada (requests)

## Objetivo

Garantir que todo endpoint que hoje aceita `bilhete_identidade_encarregado`, `telefone_encarregado` ou o arquivo `bi_encarregado` passe a aceitar **exclusivamente** os nomes novos, rejeitando os nomes antigos.

## Regra de negócio

1. os endpoints afetados devem deixar de reconhecer `bilhete_identidade_encarregado`, `telefone_encarregado` e o arquivo `bi_encarregado` como nomes de campo válidos;
2. qualquer payload contendo esses nomes antigos deve ser rejeitado com `400` e mensagem clara indicando o nome correto a ser usado;
3. não deve existir nenhuma forma de aceitação silenciosa, tradução automática, mapeamento implícito, alias, header opcional ou flag de compatibilidade que reative o nome antigo — o cliente precisa migrar para o nome novo.

## Escopo obrigatório

### 2.1 Endpoints afetados

Aplicar a regra desta seção em, no mínimo:

- `POST /academia/estudante/register` (campo de texto `bilhete_identidade_encarregado`, `telefone_encarregado`, arquivo `bi_encarregado`);
- `POST /academia/estudante/register/async`, nos dois modos: JSON (`com_arquivo=false`, campos por estudante do lote) e multipart (`com_arquivo=true`, incluindo o padrão de arquivo `<codigo_temporario>.bi_encarregado`);
- `POST /academia/estudante/{codigo_estudante}/documentos` (upload posterior de documentos pendentes);
- `PUT /estudante/dados-pessoais` (JSON, campo `bilhete_identidade_encarregado`);
- `POST /solicitacao-matricula` (campos de texto `bilhete_identidade_encarregado`, `telefone_encarregado`, arquivo `bi_encarregado`).

### 2.2 Rejeição explícita, não apenas ignorar

Um payload contendo o nome antigo deve ser **rejeitado** com `400`, e não apenas ter o campo ignorado silenciosamente — ignorar silenciosamente esconderia do cliente que o dado enviado não teve efeito algum.

### 2.3 Mensagem de erro

```json
{
  "error": "VALIDATION_ERROR",
  "message": "o campo 'bilhete_identidade_encarregado' não existe mais neste contrato; use 'bilhete_identidade_encarregado'",
  "details": [
    {
      "field": "bilhete_identidade_encarregado",
      "code": "campo_removido",
      "message": "o campo 'bilhete_identidade_encarregado' não existe mais neste contrato; use 'bilhete_identidade_encarregado'"
    }
  ]
}
```

O mesmo padrão de mensagem deve ser aplicado para `telefone_encarregado` → `telefone_encarregado` e para o arquivo `bi_encarregado` → `bi_encarregado`.

### 2.4 Testes obrigatórios

1. envio do nome novo (`bilhete_identidade_encarregado`, `telefone_encarregado`, `bi_encarregado`): aceito normalmente, com o mesmo resultado de negócio já existente;
2. envio do nome antigo (`bilhete_identidade_encarregado`, `telefone_encarregado`, `bi_encarregado`): rejeitado com `400`, orientando o nome novo;
3. envio simultâneo do nome novo e do nome antigo no mesmo payload: rejeitado (o nome antigo, por si só, já é motivo de rejeição, independentemente de o nome novo também estar presente);
4. os cenários acima cobertos para todos os endpoints listados na seção 2.1, incluindo os dois modos de `POST /academia/estudante/register/async`.

---

# 3. Expor exclusivamente o nome novo na saída (responses)

## Objetivo

Garantir que nenhuma resposta do sistema exponha os nomes antigos, em nenhuma circunstância.

## Regra de negócio

Toda resposta de `EstudanteDTO` e `SolicitacaoMatriculaDTO` (e qualquer resposta derivada, como `GET /consultar-estudante/:codigo`, `GET /meu-perfil`, `GET /estudantes`, `GET /academia/solicitacoes-matricula`, `GET /academia/solicitacao-matricula/:codigo`, `GET /solicitacoes-matricula`) deve conter apenas:

- `bilhete_identidade_encarregado` (nunca `bilhete_identidade_encarregado`);
- `telefone_encarregado` (nunca `telefone_encarregado`);
- `telefone_encarregado_verificado` (nunca `telefone_encarregado_verificado`);
- no mapa `documentos`, apenas a chave `bi_encarregado` (nunca `bi_encarregado`).

## Escopo obrigatório

### 3.1 Serialização única a partir de uma única fonte de verdade

O modelo interno (aggregate, projeção) deve armazenar o valor sob um único nome de campo (o novo); a serialização para JSON deve emitir apenas essa chave, sem duplicação nem alias.

### 3.2 Rotas de download de documento

As rotas de download já existentes (`/documentos/estudantes/{codigo_estudante}/{campo}/download`, `/documentos/solicitacoes-matricula/{codigo_solicitacao}/{campo}/download`) devem passar a aceitar apenas `bi_encarregado` como valor de `{campo}`. Uma tentativa de download usando `bi_encarregado` como `{campo}` deve retornar `404`, com o mesmo tratamento que qualquer chave de documento inexistente já recebe hoje.

### 3.3 Testes obrigatórios

1. resposta de estudante criado/consultado nunca contém `bilhete_identidade_encarregado`, `telefone_encarregado` nem `telefone_encarregado_verificado`;
2. mapa `documentos` de estudante e de solicitação de matrícula nunca contém a chave `bi_encarregado`;
3. tentativa de download usando `bi_encarregado` como `{campo}` retorna `404`;
4. resposta de `SolicitacaoMatriculaDTO` segue o mesmo padrão, sem nenhum campo antigo remanescente.

---

# 4. Persistência, event sourcing e rebuild sem reintroduzir o nome antigo

## Objetivo

Migrar a persistência para o nome novo, garantindo que o rebuild de projeções continue reconstruindo corretamente o estado a partir de eventos gravados antes desta mudança, sem que o nome antigo volte a existir em nenhum ponto do contrato ativo ou da projeção resultante.

## Regra de negócio

1. a partir desta mudança, todo novo evento gravado no ledger (`EstudanteCriadoComVinculo`, `SolicitacaoMatriculaCriada`, e qualquer outro que carregue esses campos) usa exclusivamente os nomes novos no `payload`;
2. eventos já existentes no ledger, gravados antes desta mudança, permanecem com os nomes antigos no `payload` histórico — são imutáveis e não podem ser reescritos, sob risco de quebrar a cadeia de hashes;
3. o código de aplicação de eventos (event handlers/projection appliers), usado internamente pelo processo de rebuild, deve ser ajustado para, ao processar um evento histórico contendo o nome antigo no payload, gravar o valor **já sob o nome novo** na projeção/aggregate resultante — de forma que a projeção nunca exponha o nome antigo, independentemente de o evento de origem ser anterior ou posterior a esta mudança;
4. esse trecho de interpretação de payload histórico deve ficar isolado exclusivamente no ponto de aplicação do evento, nunca na camada de comando/validação/DTO, e deve conter comentário explícito no código indicando que existe apenas para interpretar eventos anteriores à mudança de nomenclatura — não deve ser tratado, documentado ou testado como uma funcionalidade de compatibilidade voltada ao cliente;
5. as colunas de projeção correspondentes (`projection_estudantes`, `projection_solicitacoes_matricula`) devem ser renomeadas por meio de migration idempotente.

## Escopo obrigatório

### 4.1 Migração de colunas de projeção

Criar migration renomeando as colunas equivalentes a `bilhete_identidade_encarregado`, `telefone_encarregado` e `telefone_encarregado_verificado` para seus nomes novos, seguindo o padrão de migrations idempotentes já usado no projeto.

### 4.2 Interpretação isolada de eventos históricos durante o rebuild

Ajustar os event handlers de `Estudante` e `SolicitacaoMatricula` para, ao ler o `payload` de um evento, reconhecer a chave antiga apenas quando a chave nova não estiver presente (ou seja, apenas para eventos gravados antes desta mudança), sempre escrevendo o resultado sob o nome novo no modelo interno e na projeção. Esse comportamento deve ser exclusivo do caminho interno de aplicação de eventos, sem qualquer reflexo em DTOs, validações de comando ou documentação pública.

### 4.3 Testes obrigatórios

1. rebuild de `projection_estudantes` a partir de eventos históricos (gravados com o nome antigo) reconstrói corretamente os valores, expondo-os apenas sob o nome novo na projeção;
2. um evento novo, gravado após esta mudança, é lido e projetado corretamente sob o nome novo;
3. um cenário misto — parte dos eventos de um mesmo aggregate gravados antes da mudança (nome antigo) e parte depois (nome novo) — produz o estado final correto, sem nenhuma referência ao nome antigo na projeção resultante;
4. nenhuma escrita nova no ledger usa o nome antigo a partir desta mudança;
5. nenhuma consulta pública (API) consegue observar o nome antigo em nenhum cenário, incluindo logo após um rebuild completo.

---

# 5. Migrar metadados de documentos já armazenados

## Objetivo

Garantir que documentos já enviados antes desta mudança, cujo metadado usa a chave `bi_encarregado`, passem a ser identificados exclusivamente pela chave `bi_encarregado`, sem exigir novo upload por parte do estudante/academia.

## Regra de negócio

1. novos uploads devem ser registrados no mapa `documentos` exclusivamente com a chave `bi_encarregado`;
2. os metadados de documentos já existentes (a entrada no mapa `documentos` da projeção — não o arquivo físico em si, que pode permanecer com o nome já usado no storage) devem ser migrados de `bi_encarregado` para `bi_encarregado` por meio de migration/rotina de backfill sobre a projeção;
3. depois da migração, nenhum registro ativo deve referenciar `bi_encarregado` como chave de consulta pública.

## Escopo obrigatório

### 5.1 Migration/backfill de metadados de documentos

Criar migration/rotina que percorra os registros existentes com metadado sob a chave antiga e regrave o mesmo metadado (mesmo `path`, `file_url`, apenas recalculando `download_url` se o formato depender do nome do campo) sob a chave nova, removendo a chave antiga do registro.

### 5.2 Testes obrigatórios

1. documento enviado antes da migração, após a migração, é consultável e baixável exclusivamente pela chave `bi_encarregado`;
2. nenhuma consulta, após a migração, expõe a chave `bi_encarregado`;
3. rebuild completo da projeção (cenário da seção 4.3) também produz diretamente a chave `bi_encarregado` para documentos originados de eventos antigos, sem depender exclusivamente da migration de backfill.

---

# 6. Mensagens de erro, documentação e comentários de código

## Objetivo

Atualizar todo texto voltado ao usuário e todo comentário de código para usar exclusivamente "encarregado" no lugar de "encarregado".

## Escopo obrigatório

### 6.1 Mensagens de erro e de sucesso

Toda mensagem de validação, erro de negócio ou mensagem de sucesso que hoje menciona "encarregado" (ex.: BI do encarregado obrigatório, BI do encarregado duplicado, telefone do encarregado obrigatório) deve passar a mencionar "encarregado". As únicas mensagens que ainda citam o nome antigo são as de rejeição definidas na seção 2.3, cujo propósito é justamente informar que aquele nome não é mais aceito.

### 6.2 `Documentação.md`

Atualizar todas as seções afetadas (Estruturas de Dados, Cadastro de Estudante, Solicitação de Matrícula, regras automáticas de documentos de matrícula) para usar "encarregado" como único termo, sem nenhuma nota sugerindo que o nome antigo continua aceito.

### 6.3 OpenAPI/Swagger

Se existir especificação OpenAPI/Swagger, remover completamente os campos com o nome antigo dos schemas, substituindo-os pelos nomes novos.

### 6.4 Comentários de código

Atualizar comentários inline que mencionem "encarregado"/"encarregado" para "encarregado", com exceção do comentário isolado descrito na seção 4.2, que deve permanecer explicando exclusivamente a interpretação de eventos históricos.

### 6.5 Testes obrigatórios

1. busca textual confirma que nenhuma mensagem de erro/sucesso ativa usa "encarregado", exceto as mensagens de rejeição do nome antigo;
2. `Documentação.md` não contém nenhuma instrução sugerindo que o nome antigo ainda é aceito ou retornado;
3. revisão cruzada confirma que nenhuma seção de `Documentação.md` ficou contraditória entre a nomenclatura antiga e a nova.

---

# Fora de escopo

- Manter, em qualquer forma, aceitação ou exposição dos nomes antigos (`bilhete_identidade_encarregado`, `telefone_encarregado`, `telefone_encarregado_verificado`, `bi_encarregado`) no contrato público de requests/responses.
- Criar aliases, wrappers de compatibilidade, parâmetros opcionais, headers de versão, flags de configuração ou qualquer outro mecanismo que reative o nome antigo.
- Definir período de transição ou depreciação gradual para o nome antigo.
- Alterar as regras de negócio de obrigatoriedade do BI/telefone do encarregado por nível de ensino (essas regras continuam exatamente as mesmas; apenas o nome dos campos muda).
- Reescrever documentos já concluídos em `docs/Tarefas feitas/`.
- Reescrever, apagar ou substituir eventos já gravados no ledger — isso violaria a integridade da cadeia de hashes.
- Alterar qualquer campo que não contenha literalmente "encarregado"/"encarregado" (ex.: `aprovada_por`, `reprovada_por`, `definido_por`, `alterado_por` permanecem inalterados).

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `bilhete_identidade_encarregado`, `telefone_encarregado`, `telefone_encarregado_verificado` e `bi_encarregado` (incluindo `documentos.bi_encarregado`) existirem como únicos nomes aceitos e expostos em todos os endpoints listados na seção 2.1;
2. o envio dos nomes antigos for rejeitado com erro claro em todos os endpoints listados, sem nenhuma aceitação silenciosa;
3. nenhuma resposta do sistema expuser os nomes antigos, em nenhum cenário, incluindo logo após um rebuild completo de projeções;
4. o rebuild de projeções a partir de eventos históricos continuar produzindo o estado correto, gravando os valores exclusivamente sob o nome novo;
5. novos eventos gravados no ledger usarem exclusivamente os nomes novos;
6. os metadados de documentos já armazenados estiverem migrados para a chave nova;
7. `Documentação.md`, mensagens de erro/sucesso e comentários de código usarem exclusivamente "encarregado", sem qualquer menção a suporte legado;
8. testes automatizados cobrirem os cenários das seções 2.4, 3.3, 4.3, 5.2 e 6.5;
9. o PR confirmar explicitamente que não restou nenhum caminho de aceitação ou exposição do nome antigo fora do histórico imutável do ledger.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Renomear campos de "encarregado" para "encarregado" (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
