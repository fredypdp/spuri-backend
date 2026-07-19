---
criado: 2026-07-19 00:00
origem: solicitação do usuário
status: pendente
---

# Renomear campos de "responsável" para "encarregado" com suporte a campos legados (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento substituindo, em todo o sistema, o termo "responsável" por "encarregado" — nos campos `bilhete_identidade_responsavel`, `telefone_responsavel`, `telefone_responsavel_verificado` e no campo de arquivo `bi_responsavel` (incluindo a chave equivalente no mapa `documentos`), e em todo texto relacionado a esses campos (mensagens de erro, mensagens de sucesso, documentação técnica, comentários de código). Ao contrário de todas as demais tarefas deste repositório, **esta tarefa exige explicitamente suporte a compatibilidade com os campos legados**: os nomes antigos (`bilhete_identidade_responsavel`, `telefone_responsavel`, `telefone_responsavel_verificado`, `bi_responsavel`) devem continuar sendo aceitos nas requisições e devem continuar aparecendo nas respostas, lado a lado com os novos nomes, enquanto a compatibilidade não for explicitamente encerrada por uma decisão de produto futura. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada.

> **Atenção — esta tarefa inverte a regra padrão do repositório.** As demais tarefas em `docs/Tarefas feitas/` proíbem deliberadamente aliases, wrappers de compatibilidade e fallbacks legados. Esta tarefa é uma exceção intencional e explícita a essa regra: a compatibilidade com os nomes antigos **deve** ser criada e mantida. Não remova o suporte legado achando que está seguindo o padrão do repositório — aqui o padrão é o oposto.

## Contexto

O sistema usa hoje, em `Estudante` e em `SolicitacaoMatricula`, os campos `bilhete_identidade_responsavel`, `telefone_responsavel` e `telefone_responsavel_verificado`, além do campo de upload `bi_responsavel` (e da chave `bi_responsavel` no mapa `documentos`), para representar o Bilhete de Identidade e o telefone da pessoa legalmente responsável pelo estudante. A decisão de produto agora é adotar o termo "encarregado" no lugar de "responsável" em todo o sistema — campos, mensagens de erro, documentação e comentários — sem quebrar clientes/integrações que já usam a nomenclatura atual.

Todas as ocorrências de "responsavel"/"responsável" no sistema se referem exclusivamente a esse papel (a pessoa responsável legal pelo estudante); não há nenhum outro uso do termo em outro sentido de negócio (campos como `aprovada_por`, `reprovada_por`, `definido_por`, `alterado_por` não usam a palavra "responsavel" e não são afetados por esta tarefa). Isso torna a substituição direta e sem ambiguidade, mas o volume de ocorrências é grande: o termo aparece em `EstudanteDTO`, `SolicitacaoMatriculaDTO`, nos campos de texto e de arquivo de `POST /academia/estudante/register`, `POST /academia/estudante/register/async`, `POST /academia/estudante/{codigo_estudante}/documentos`, `PUT /estudante/dados-pessoais`, `POST /solicitacao-matricula`, nas regras automáticas de documentos de matrícula, nas mensagens de erro de BI e nos exemplos de payload em `Documentação.md`.

Por se tratar de um sistema com Event Sourcing, também é preciso lidar com o fato de que eventos já gravados no `spuri_ledger` guardam, no `payload` histórico, os nomes antigos dos campos — esses eventos são imutáveis e não podem ser reescritos. A lógica de leitura/replay precisa reconhecer tanto o nome antigo (em eventos históricos) quanto o nome novo (em eventos gravados a partir desta mudança), normalizando sempre para o campo novo no modelo interno e na projeção.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Nomenclatura de campos | `bilhete_identidade_responsavel` → `bilhete_identidade_encarregado`; `telefone_responsavel` → `telefone_encarregado`; `telefone_responsavel_verificado` → `telefone_encarregado_verificado`; `bi_responsavel` → `bi_encarregado` | "Encarregado" passa a ser o nome canônico em todo o sistema |
| Entrada (requests) | Aceitar os dois nomes, novo e legado | Nenhum cliente existente quebra ao continuar enviando o nome antigo |
| Conflito de valores | Rejeitar quando os dois nomes forem enviados com valores diferentes | Evita ambiguidade silenciosa de dado |
| Saída (responses) | Expor os dois nomes, com o mesmo valor espelhado | Clientes que já leem o nome antigo continuam funcionando sem alteração |
| Persistência interna | Usar exclusivamente o nome novo a partir do domínio para dentro (aggregate, projeção, novos eventos) | O nome legado existe apenas como alias na borda da API |
| Eventos históricos | Preservados como estão; replay reconhece nome antigo e novo | Rebuild de projeções continua correto mesmo com eventos antigos no ledger |
| Textos (erro, sucesso, documentação, comentários) | Atualizar para "encarregado", mencionando o alias legado apenas onde o nome do campo aparece literalmente | Nenhuma inconsistência terminológica nova |

---

# 1. Levantamento e nomenclatura oficial dos campos afetados

## Objetivo

Mapear, de forma exaustiva, todas as ocorrências de "responsavel"/"responsável" no código e na documentação, e fixar a nomenclatura oficial de substituição antes de qualquer alteração.

## Regra de negócio

| Nome atual | Nome novo | Onde aparece |
| --- | --- | --- |
| `bilhete_identidade_responsavel` | `bilhete_identidade_encarregado` | `EstudanteDTO`, `SolicitacaoMatriculaDTO`, campos de texto de `POST /academia/estudante/register` (e `/async`), `PUT /estudante/dados-pessoais`, `POST /solicitacao-matricula` |
| `telefone_responsavel` | `telefone_encarregado` | `EstudanteDTO`, campos de `POST /academia/estudante/register` (e `/async`), `POST /solicitacao-matricula` |
| `telefone_responsavel_verificado` | `telefone_encarregado_verificado` | `EstudanteDTO` |
| `bi_responsavel` (campo de arquivo) | `bi_encarregado` | `POST /academia/estudante/register`, `POST /academia/estudante/register/async` (inclusive no padrão `<codigo_temporario>.bi_responsavel`), `POST /academia/estudante/{codigo_estudante}/documentos`, `POST /solicitacao-matricula` |
| `documentos.bi_responsavel` (chave no mapa de documentos) | `documentos.bi_encarregado` | Respostas de estudante e de solicitação de matrícula, rotas de download de documento |
| "BI do responsável", "telefone do responsável", "responsável" em prosa | "BI do encarregado", "telefone do encarregado", "encarregado" | Mensagens de erro, `Documentação.md`, comentários de código |

## Escopo obrigatório

### 1.1 Busca ampla obrigatória

Antes de alterar qualquer código, executar busca ampla e classificar cada ocorrência:

```bash
rg -n -i "responsavel|responsável" .
```

Cada ocorrência deve ser classificada como: campo a renomear (com alias legado), texto a atualizar, evento histórico do ledger (não deve ser alterado), documentação de tarefa já concluída em `docs/Tarefas feitas/` (não deve ser reescrita, pois é registro histórico), ou falso positivo.

### 1.2 Não reescrever documentação histórica

Arquivos já existentes em `docs/Tarefas feitas/` (incluindo `Cadastro de estudante escolar - BI do responsável obrigatório.md`) são registros históricos de tarefas concluídas e **não devem ser reescritos** para usar "encarregado"; eles descrevem a implementação como ela era no momento em que foram concluídos. Apenas `Documentação.md` (a documentação viva do sistema) e o código-fonte devem refletir a nova nomenclatura.

---

# 2. Aceitar os dois nomes na entrada (requests), com resolução de conflito

## Objetivo

Garantir que todo endpoint que hoje aceita `bilhete_identidade_responsavel`, `telefone_responsavel` ou o arquivo `bi_responsavel` passe a aceitar, alternativamente, `bilhete_identidade_encarregado`, `telefone_encarregado` e `bi_encarregado`, sem quebrar clientes que ainda usam os nomes antigos.

## Regra de negócio

Para cada campo lógico (BI do encarregado, telefone do encarregado, documento de BI do encarregado), o backend deve:

1. aceitar o nome novo e o nome legado como formas alternativas de preencher o mesmo campo lógico;
2. se **apenas um** dos dois for enviado (novo ou legado), aceitar normalmente e normalizar internamente para o campo novo;
3. se **os dois** forem enviados com o **mesmo valor**, aceitar normalmente (idempotente);
4. se **os dois** forem enviados com valores **diferentes**, rejeitar com `400` e mensagem clara indicando o conflito entre o nome novo e o nome legado do campo;
5. a partir da camada de validação/handler para dentro (serviços de domínio, aggregates, eventos novos, projeções), usar **exclusivamente** o nome novo — o nome legado nunca deve se propagar além da borda de entrada da API.

## Escopo obrigatório

### 2.1 Endpoints afetados

Aplicar a regra da seção anterior em, no mínimo:

- `POST /academia/estudante/register` (campo de texto `bilhete_identidade_responsavel`/`bilhete_identidade_encarregado`, `telefone_responsavel`/`telefone_encarregado`, arquivo `bi_responsavel`/`bi_encarregado`);
- `POST /academia/estudante/register/async`, nos dois modos: JSON (`com_arquivo=false`, campos por estudante do lote) e multipart (`com_arquivo=true`, incluindo o padrão de arquivo `<codigo_temporario>.bi_responsavel`/`<codigo_temporario>.bi_encarregado`);
- `POST /academia/estudante/{codigo_estudante}/documentos` (upload posterior de documentos pendentes);
- `PUT /estudante/dados-pessoais` (JSON, campo `bilhete_identidade_responsavel`/`bilhete_identidade_encarregado`);
- `POST /solicitacao-matricula` (campos de texto `bilhete_identidade_responsavel`/`bilhete_identidade_encarregado`, `telefone_responsavel`/`telefone_encarregado`, arquivo `bi_responsavel`/`bi_encarregado`).

### 2.2 Conflito no arquivo enviado

Se o mesmo request multipart enviar **os dois** campos de arquivo (`bi_responsavel` **e** `bi_encarregado`) simultaneamente, rejeitar com `400` por ambiguidade, independentemente do conteúdo dos arquivos — o backend não deve tentar comparar bytes dos dois arquivos para decidir qual usar.

### 2.3 Mensagem de erro de conflito

```json
{
  "error": "VALIDATION_ERROR",
  "message": "os campos 'bilhete_identidade_encarregado' e 'bilhete_identidade_responsavel' foram enviados com valores diferentes; envie apenas um deles ou envie os dois com o mesmo valor",
  "details": [
    {
      "field": "bilhete_identidade_encarregado",
      "code": "conflito_com_campo_legado",
      "message": "..."
    }
  ]
}
```

### 2.4 Testes obrigatórios

1. envio apenas do nome novo (`bilhete_identidade_encarregado`, `telefone_encarregado`, `bi_encarregado`): aceito normalmente;
2. envio apenas do nome legado (`bilhete_identidade_responsavel`, `telefone_responsavel`, `bi_responsavel`): aceito normalmente, com o mesmo resultado de negócio de antes desta tarefa;
3. envio dos dois nomes com o mesmo valor: aceito normalmente;
4. envio dos dois nomes com valores diferentes: rejeitado com `400` e mensagem de conflito;
5. envio dos dois arquivos (`bi_responsavel` e `bi_encarregado`) no mesmo request: rejeitado com `400`;
6. os cenários acima cobertos para todos os endpoints listados na seção 2.1, incluindo os dois modos de `POST /academia/estudante/register/async`.

---

# 3. Expor os dois nomes na saída (responses)

## Objetivo

Garantir que toda resposta que hoje expõe os campos antigos continue expondo-os, ao lado dos novos, com o mesmo valor.

## Regra de negócio

Toda resposta de `EstudanteDTO` e `SolicitacaoMatriculaDTO` (e qualquer resposta derivada, como `GET /consultar-estudante/:codigo`, `GET /meu-perfil`, `GET /estudantes`, `GET /academia/solicitacoes-matricula`, `GET /academia/solicitacao-matricula/:codigo`, `GET /solicitacoes-matricula`) deve expor simultaneamente:

- `bilhete_identidade_encarregado` (novo) e `bilhete_identidade_responsavel` (legado), com o mesmo valor;
- `telefone_encarregado` (novo) e `telefone_responsavel` (legado), com o mesmo valor;
- `telefone_encarregado_verificado` (novo) e `telefone_responsavel_verificado` (legado), com o mesmo valor;
- no mapa `documentos`, tanto a chave `bi_encarregado` (novo) quanto `bi_responsavel` (legado), apontando para os mesmos metadados (`path`, `file_url`, `download_url`).

## Escopo obrigatório

### 3.1 Consistência garantida entre os campos espelhados

Os dois nomes nunca devem divergir em valor dentro da mesma resposta. Isso deve ser garantido na camada de serialização (DTO), a partir de uma única fonte de verdade interna (o campo novo), nunca por duas gravações independentes que possam ficar dessincronizadas.

### 3.2 Rotas de download de documento

As rotas já existentes de download (`/documentos/estudantes/{codigo_estudante}/{campo}/download`, `/documentos/solicitacoes-matricula/{codigo_solicitacao}/{campo}/download`) devem aceitar tanto `bi_encarregado` quanto `bi_responsavel` como valor de `{campo}`, resolvendo para o mesmo arquivo.

### 3.3 Testes obrigatórios

1. resposta de estudante criado/consultado expõe `bilhete_identidade_encarregado` e `bilhete_identidade_responsavel` com valores idênticos;
2. resposta expõe `telefone_encarregado`/`telefone_responsavel` e `telefone_encarregado_verificado`/`telefone_responsavel_verificado` com valores idênticos;
3. mapa `documentos` expõe `bi_encarregado` e `bi_responsavel` apontando para o mesmo `path`/`file_url`/`download_url`;
4. download do documento funciona tanto usando `bi_encarregado` quanto `bi_responsavel` como `{campo}` na URL;
5. resposta de `SolicitacaoMatriculaDTO` segue o mesmo padrão de espelhamento.

---

# 4. Persistência, event sourcing e rebuild com nomes antigos e novos

## Objetivo

Garantir que a mudança de nomenclatura não quebre a reconstrução de projeções a partir de eventos já gravados no ledger, que usam os nomes antigos.

## Regra de negócio

1. a partir desta mudança, todo **novo** evento gravado no ledger (`EstudanteCriadoComVinculo`, `SolicitacaoMatriculaCriada`, e qualquer outro que carregue esses campos) deve usar exclusivamente os nomes novos no `payload`;
2. eventos **já existentes** no ledger, gravados antes desta mudança, continuam com os nomes antigos no `payload` histórico — eles são imutáveis e não devem ser reescritos, migrados ou duplicados;
3. a lógica de aplicação de eventos (event handlers/projection appliers) deve reconhecer tanto o nome antigo quanto o nome novo ao ler o `payload` de um evento, normalizando sempre para o campo novo no modelo interno e na projeção — isso vale tanto para o processamento em tempo real quanto para rebuild;
4. as colunas de projeção correspondentes (em `projection_estudantes` e `projection_solicitacoes_matricula`) devem ser renomeadas para os nomes novos por meio de migration idempotente.

## Escopo obrigatório

### 4.1 Migração de colunas de projeção

Criar migration renomeando as colunas equivalentes a `bilhete_identidade_responsavel`, `telefone_responsavel` e `telefone_responsavel_verificado` para seus nomes novos nas projeções afetadas, seguindo o padrão de migrations idempotentes já usado no projeto.

### 4.2 Compatibilidade de replay com eventos antigos

Ajustar os event handlers de `Estudante` e `SolicitacaoMatricula` para ler, do `payload` de um evento, o valor tanto da chave antiga quanto da chave nova (nesta ordem de prioridade: se a chave nova existir no payload, usá-la; senão, usar a chave antiga), garantindo que o rebuild de projeções continue produzindo o mesmo estado para eventos gravados antes e depois desta mudança.

### 4.3 Testes obrigatórios

1. rebuild de `projection_estudantes` a partir de eventos históricos (gravados com o nome antigo) reconstrói corretamente os campos com o nome novo na projeção;
2. um evento novo, gravado após esta mudança (com o nome novo), é lido e projetado corretamente;
3. um cenário misto — parte dos eventos de um mesmo aggregate gravados antes da mudança (nome antigo) e parte depois (nome novo) — produz o estado final correto no rebuild;
4. nenhuma escrita nova no ledger usa o nome antigo a partir desta mudança.

---

# 5. Documentos armazenados

## Objetivo

Garantir que novos uploads usem a nomenclatura nova nos metadados, sem afetar documentos já armazenados sob a nomenclatura antiga.

## Regra de negócio

1. novos uploads devem ser registrados no mapa `documentos` com metadados usando `tipo`/chave `bi_encarregado`, conforme já reestruturado pela normalização de documentos acadêmicos existente no sistema;
2. documentos já armazenados anteriormente com metadado `bi_responsavel` permanecem como estão — não há necessidade de renomear arquivos já existentes no storage nem de reprocessar uploads antigos;
3. a leitura/consulta de documentos deve reconhecer ambas as chaves (`bi_encarregado` e `bi_responsavel`) de forma equivalente, conforme já especificado na seção 3.2.

## Escopo obrigatório

### 5.1 Testes obrigatórios

1. upload novo gera metadado com chave `bi_encarregado`;
2. documento antigo (simulado com metadado `bi_responsavel` pré-existente) continua sendo consultável e baixável normalmente;
3. resposta de consulta de estudante/solicitação com documento antigo expõe o metadado espelhado em `bi_encarregado` também, conforme a seção 3.

---

# 6. Mensagens de erro, documentação e comentários de código

## Objetivo

Atualizar todo texto voltado ao usuário e todo comentário de código para usar "encarregado" no lugar de "responsável", preservando menção clara ao alias legado onde o nome do campo aparece literalmente.

## Escopo obrigatório

### 6.1 Mensagens de erro e de sucesso

Toda mensagem de validação, erro de negócio ou mensagem de sucesso que hoje menciona "responsável" (ex.: mensagens relacionadas a BI do responsável obrigatório, BI do responsável duplicado, telefone do responsável obrigatório) deve passar a mencionar "encarregado". Mensagens que citam o nome literal do campo (ex.: em `details[].message` de erro de validação) devem citar o nome novo como principal, podendo mencionar o alias legado apenas quando a mensagem for especificamente sobre o próprio mecanismo de alias (ex.: a mensagem de conflito da seção 2.3).

### 6.2 `Documentação.md`

Atualizar todas as seções afetadas (Estruturas de Dados, Cadastro de Estudante, Solicitação de Matrícula, regras automáticas de documentos de matrícula) para usar "encarregado" como termo primário, adicionando uma nota clara, em cada endpoint afetado, explicando que os nomes de campo antigos (`bilhete_identidade_responsavel`, `telefone_responsavel`, `telefone_responsavel_verificado`, `bi_responsavel`) continuam aceitos e retornados por compatibilidade.

### 6.3 OpenAPI/Swagger

Se existir especificação OpenAPI/Swagger, atualizar os schemas para refletir ambos os campos (novo e legado) como propriedades válidas, com o campo novo marcado como preferencial e o legado marcado como `deprecated: true` (ou equivalente do formato usado), sem removê-lo do schema.

### 6.4 Comentários de código

Atualizar comentários inline que mencionem "responsavel"/"responsável" para "encarregado", exceto onde o comentário precisar, deliberadamente, explicar o mecanismo de compatibilidade com o nome legado.

### 6.5 Testes obrigatórios

1. busca textual confirma que nenhuma mensagem de erro/sucesso nova introduzida por esta tarefa usa "responsável" fora do contexto de explicar o alias legado;
2. `Documentação.md` não contém instrução alguma orientando o cliente a preferir o nome antigo — o nome novo é sempre apresentado como o principal;
3. revisão cruzada confirma que nenhuma seção de `Documentação.md` ficou contraditória entre a nomenclatura antiga e a nova.

---

# Fora de escopo

- Remover, em qualquer momento desta tarefa, os campos/nomes legados (`bilhete_identidade_responsavel`, `telefone_responsavel`, `telefone_responsavel_verificado`, `bi_responsavel`) das requisições ou das respostas — isso contraria o objetivo explícito desta tarefa.
- Definir uma data ou versão de expiração para o suporte legado; essa decisão fica para um produto/tarefa futura, se e quando for tomada.
- Alterar as regras de negócio de obrigatoriedade do BI/telefone do encarregado por nível de ensino (essas regras continuam exatamente as mesmas; apenas o nome dos campos muda).
- Reescrever documentos já concluídos em `docs/Tarefas feitas/`.
- Criar um terceiro nome de campo além de "encarregado" (novo) e "responsavel" (legado).
- Alterar qualquer campo que não contenha literalmente "responsavel"/"responsável" (ex.: `aprovada_por`, `reprovada_por`, `definido_por`, `alterado_por` permanecem inalterados).

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `bilhete_identidade_encarregado`, `telefone_encarregado`, `telefone_encarregado_verificado` e `bi_encarregado` (incluindo a chave `documentos.bi_encarregado`) existirem como nomes canônicos em todos os endpoints listados na seção 2.1;
2. os nomes legados correspondentes continuarem sendo aceitos em todas as requisições e continuarem aparecendo em todas as respostas, com valor idêntico ao nome novo;
3. o envio simultâneo do nome novo e do nome legado com valores diferentes for rejeitado com erro claro de conflito;
4. o rebuild de projeções a partir de eventos históricos (nome antigo) e de eventos novos (nome novo) produzir o mesmo estado final correto;
5. novos eventos gravados no ledger usarem exclusivamente os nomes novos;
6. documentos já armazenados sob a nomenclatura antiga continuarem consultáveis e baixáveis por ambas as chaves;
7. `Documentação.md`, mensagens de erro/sucesso e comentários de código usarem "encarregado" como termo primário, com nota explícita sobre o alias legado onde aplicável;
8. testes automatizados cobrirem os cenários das seções 2.4, 3.3, 4.3, 5.1 e 6.5;
9. o PR explicar claramente que, ao contrário do padrão do repositório, esta tarefa introduz e mantém deliberadamente suporte a nomes de campo legados.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Renomear campos de "responsável" para "encarregado" com suporte a campos legados (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
