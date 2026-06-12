---
modificado: 2026-06-12 23:11
criado: 2026-06-11 20:08
---
Este documento descreve duas novas funcionalidades a serem implementadas no backend do Spuri. Cada secção cobre: contexto, entidades novas, eventos do ledger, projeções, regras de negócio, fluxo de execução e endpoints da API.

---

## 1. Módulo de Solicitação de Matrícula

### 1.1 Visão Geral

Atualmente, o cadastro de um estudante só pode ser feito **pela academia**. Esta atualização introduz uma nova modalidade: o **estudante pode solicitar a sua própria matrícula** numa instituição, preenchendo os dados pessoais e académicos e anexando os documentos exigidos. A academia então analisa a solicitação e a aprova ou reprova.

Quando aprovada, o sistema **executa automaticamente** o fluxo de cadastro de estudante que já existe (`EstudanteCriadoComVinculo`), como se a academia tivesse feito o registo manualmente. Nenhum novo código de criação de estudante deve ser duplicado — o aggregate de estudante existente deve ser reutilizado.

Esta funcionalidade é inteiramente nova e deve seguir o padrão **Event Sourcing + CQRS** do sistema: toda mutação de estado passa pelo ledger (`spuri_ledger`) antes de atualizar as projeções.

---

### 1.2 Nova Entidade: SolicitacaoMatricula

#### 1.2.1 Campos

|Campo|Tipo|Obrigatório|Descrição|
|---|---|---|---|
|`id`|UUID|Sim (gerado)|Identificador único da solicitação|
|`codigo_solicitacao`|String (11 chars alfanumérico)|Sim (gerado)|Código único alfanumérico de 11 caracteres. Usado como nome do diretório de documentos. Gerado no backend — nunca enviado pelo cliente. Formato: letras maiúsculas e dígitos, ex: `A3F9K2BPQ7X`|
|`codigo_academia`|String|Sim|Código da academia-alvo (ex: `LDA20261`)|
|`nome`|String|Sim|Nome completo do estudante|
|`genero`|Enum|Sim|`masculino` ou `feminino`|
|`data_nascimento`|Date (`YYYY-MM-DD`)|Sim|Deve ser anterior à data atual|
|`email`|String|Não|Email do estudante|
|`telefone`|String|Não|Telefone do estudante|
|`bilhete_identidade`|String|Condicional|Obrigatório se `bilhete_identidade_responsavel` estiver vazio|
|`bilhete_identidade_responsavel`|String|Condicional|Obrigatório se `bilhete_identidade` estiver vazio|
|`ano_escolar_fundamental`|String|Não|Ex: `3_ano_fundamental`|
|`ano_escolar_medio`|String|Não|Ex: `1_ano_medio`|
|`curso_medio_id`|UUID|Não|UUID de curso médio existente na academia-alvo|
|`ano_superior`|String|Não|Ex: `1_ano_superior`|
|`curso_superior_id`|UUID|Não|UUID de curso superior existente na academia-alvo|
|`status`|Enum|Sim (gerado)|`pendente` / `aprovada` / `reprovada`|
|`motivo_reprovacao`|String|Não|Preenchido pela academia ao reprovar|
|`documentos`|Object|Sim|Mapa dos caminhos dos ficheiros no armazenamento. Ver secção 1.2.2.|
|`codigo_estudante_gerado`|String|Não|Preenchido após aprovação com o código do estudante criado|
|`aprovada_por`|UUID|Não|ID da academia que aprovou|
|`reprovada_por`|UUID|Não|ID da academia que reprovou|
|`created_at`|RFC3339|Sim (gerado)|Data de criação|
|`updated_at`|RFC3339|Sim (gerado)|Data da última atualização|
|`version`|Int|Sim|Versão do aggregate|

#### 1.2.2 Estrutura de Documentos

O campo `documentos` armazena os caminhos remotos dos ficheiros enviados no armazenamento externo (Mega ou Google Drive). Os ficheiros válidos são PDFs.

```json
{
  "bi_estudante": "caminho/remoto/bi_estudante_A3F9K2BPQ7X.pdf",
  "bi_responsavel": "caminho/remoto/bi_responsavel_A3F9K2BPQ7X.pdf",
  "cedula": "caminho/remoto/cedula_A3F9K2BPQ7X.pdf",
  "declaracao": "caminho/remoto/declaracao_A3F9K2BPQ7X.pdf",
  "certificado": "caminho/remoto/certificado_A3F9K2BPQ7X.pdf"
}
```

**Regras de obrigatoriedade dos documentos:**

|Documento|Campo no mapa|Obrigatoriedade|
|---|---|---|
|Bilhete de Identidade do estudante|`bi_estudante`|Condicional — obrigatório se `bi_responsavel` não for enviado|
|Bilhete de Identidade do responsável|`bi_responsavel`|Condicional — obrigatório se `bi_estudante` não for enviado|
|Cédula|`cedula`|Obrigatório quando o estudante não tem BI próprio (ou seja, quando apenas `bi_responsavel` é enviado)|
|Declaração|`declaracao`|**Configurável por academia** — obrigatório apenas se o ano académico informado na solicitação (`ano_escolar_fundamental`, `ano_escolar_medio` ou `ano_superior`) estiver na lista `documentos_obrigatorios.declaracao` da academia-alvo. Ver secção 1.2.3.|
|Certificado|`certificado`|**Configurável por academia** — obrigatório apenas se o ano académico informado na solicitação estiver na lista `documentos_obrigatorios.certificado` da academia-alvo. Ver secção 1.2.3.|

A regra de BI segue a mesma lógica da matrícula existente: pelo menos um entre `bi_estudante` ou `bi_responsavel` deve estar presente. Se apenas `bi_responsavel` for enviado, a `cedula` também é obrigatória.

> **Nota:** `declaracao` e `certificado` deixam de ser campos sempre obrigatórios. A obrigatoriedade agora depende da configuração `documentos_obrigatorios` da academia-alvo, cruzada com o ano académico informado na solicitação (ver secção 1.2.3). Se a academia não configurou nenhum ano para um desses documentos, ele é opcional para todos os anos. Se a solicitação não informar nenhum ano académico, ambos são tratados como opcionais.

---

### 1.2.3 Configuração de Documentos Obrigatórios por Academia (`documentos_obrigatorios`)

Cada academia define, **para os seus próprios anos académicos** (fundamental, médio e/ou superior, conforme o seu `nivel`/`nivel_escolar`), em quais anos a **declaração** e o **certificado** são documentos obrigatórios na solicitação de matrícula.

#### Estrutura do campo `documentos_obrigatorios`

Novo campo na entidade `Academia` (armazenado em `projection_academias`):

```json
{
  "declaracao": ["1_ano_fundamental", "2_ano_fundamental", "1_ano_medio"],
  "certificado": ["9_ano_fundamental", "3_ano_medio", "1_ano_superior"]
}
```

- Cada chave (`declaracao`, `certificado`) é um array de strings com os anos académicos no formato canônico já usado no sistema (`[1-9]_ano_fundamental`, `[n]_ano_medio`, `[n]_ano_superior`).
- Arrays vazios ou ausentes significam que o documento **não é obrigatório para nenhum ano** dessa academia.
- Valor padrão na criação da academia: `{"declaracao": [], "certificado": []}`.

#### Validação dos anos informados

Ao definir/atualizar `documentos_obrigatorios`, o backend valida que **todos os anos informados pertencem aos anos académicos da própria academia**:

- Anos `[n]_ano_fundamental`: devem estar em `anos_academicos` da academia (quando `nivel_escolar` é `fundamental` ou `misto`).
- Anos `[n]_ano_medio`: devem corresponder a `anos_academicos` de algum curso `medio` ativo da academia.
- Anos `[n]_ano_superior`: devem corresponder a `anos_academicos` de algum curso `superior` ativo da academia.
- Qualquer ano informado que não pertença à academia é rejeitado com `400`.

#### Resolução da obrigatoriedade na criação da solicitação

Na criação da solicitação (secção 1.7.1), o backend determina o **ano académico-alvo** a partir do campo preenchido entre `ano_escolar_fundamental`, `ano_escolar_medio` ou `ano_superior`, e verifica:

1. Se esse ano está em `documentos_obrigatorios.declaracao` da academia-alvo → `declaracao` é obrigatória.
2. Se esse ano está em `documentos_obrigatorios.certificado` da academia-alvo → `certificado` é obrigatória.

Se nenhum ano académico for informado na solicitação, `declaracao` e `certificado` são tratados como **não obrigatórios**.

---

### 1.3 Código de Solicitação (`codigo_solicitacao`)

- Gerado pelo **backend** no momento da criação da solicitação.
- Formato: **11 caracteres alfanuméricos** em maiúsculas (letras A–Z e dígitos 0–9), ex: `A3F9K2BPQ7X`.
- Deve ser **único no sistema** — verificado contra a projeção `projection_solicitacoes_matricula` antes de confirmar.
- Gerado com `crypto/rand` para garantir aleatoriedade segura.
- Serve como identificador do diretório de documentos no armazenamento externo (ver Módulo 2).
- **Nunca deve ser enviado pelo cliente** no body do request.

---

### 1.4 Estados da Solicitação

```
pendente → aprovada
         → reprovada
```

- `pendente`: estado inicial após a criação pelo estudante.
- `aprovada`: a academia aceitou a solicitação. O sistema executa a matrícula automaticamente.
- `reprovada`: a academia rejeitou. Os documentos são apagados do armazenamento (ver Módulo 2, secção 2.5.3).

Não existe transição de `aprovada` ou `reprovada` de volta para `pendente`. Uma solicitação reprovada é final. Se o estudante quiser tentar novamente, deve criar uma nova solicitação.

---

### 1.5 Eventos do Ledger

O aggregate `SolicitacaoMatricula` deve emitir os seguintes eventos, todos gravados no `spuri_ledger`:

|Evento|Quando ocorre|
|---|---|
|`SolicitacaoMatriculaCriada`|Estudante cria a solicitação com dados e documentos|
|`SolicitacaoMatriculaAprovada`|Academia aprova a solicitação|
|`SolicitacaoMatriculaReprovada`|Academia reprova a solicitação (com motivo obrigatório)|
|`AcademiaDocumentosObrigatoriosAtualizados`|Academia define/atualiza `documentos_obrigatorios` (aggregate `Academia`, evento adicional aos já existentes)|

**Payload de `AcademiaDocumentosObrigatoriosAtualizados`:**

```json
{
  "codigo_academia": "LDA20261",
  "documentos_obrigatorios": {
    "declaracao": ["1_ano_fundamental", "2_ano_fundamental"],
    "certificado": ["9_ano_fundamental"]
  }
}
```

> Este evento pertence ao aggregate `Academia` já existente (assim como `CursosAtualizados`, `AnoLetivoAcademiaDefinido`, etc.), não ao aggregate `SolicitacaoMatricula`. Está listado aqui por ser parte desta atualização.

**Payload de `SolicitacaoMatriculaCriada`:**

Todos os campos da solicitação, incluindo `codigo_solicitacao`, `codigo_academia`, dados pessoais, dados académicos e o mapa `documentos` com os caminhos remotos dos ficheiros já enviados.

**Payload de `SolicitacaoMatriculaAprovada`:**

```json
{
  "codigo_solicitacao": "A3F9K2BPQ7X",
  "codigo_academia": "LDA20261",
  "aprovada_por": "uuid-da-academia",
  "codigo_estudante_gerado": "ABC1234"
}
```

**Payload de `SolicitacaoMatriculaReprovada`:**

```json
{
  "codigo_solicitacao": "A3F9K2BPQ7X",
  "codigo_academia": "LDA20261",
  "reprovada_por": "uuid-da-academia",
  "motivo_reprovacao": "Documentos ilegíveis."
}
```

---

### 1.6 Projeção: `projection_solicitacoes_matricula`

Nova tabela de leitura com os seguintes campos principais:

|Coluna|Tipo|Descrição|
|---|---|---|
|`id`|UUID|PK|
|`codigo_solicitacao`|VARCHAR(11)|Único. Índice único.|
|`codigo_academia`|VARCHAR|Academia-alvo|
|`nome`|VARCHAR|Nome do estudante solicitante|
|`genero`|VARCHAR|`masculino` / `feminino`|
|`data_nascimento`|DATE||
|`email`|VARCHAR|Nullable|
|`telefone`|VARCHAR|Nullable|
|`bilhete_identidade`|VARCHAR|Nullable|
|`bilhete_identidade_responsavel`|VARCHAR|Nullable|
|`ano_escolar_fundamental`|VARCHAR|Nullable|
|`ano_escolar_medio`|VARCHAR|Nullable|
|`curso_medio_id`|UUID|Nullable|
|`ano_superior`|VARCHAR|Nullable|
|`curso_superior_id`|UUID|Nullable|
|`status`|VARCHAR|`pendente` / `aprovada` / `reprovada`|
|`motivo_reprovacao`|TEXT|Nullable|
|`documentos`|JSONB|Mapa com caminhos dos ficheiros|
|`codigo_estudante_gerado`|VARCHAR|Nullable — preenchido após aprovação|
|`aprovada_por`|UUID|Nullable|
|`reprovada_por`|UUID|Nullable|
|`created_at`|TIMESTAMPTZ||
|`updated_at`|TIMESTAMPTZ||
|`version`|INT||

O Projection Manager deve registar esta projeção e processá-la como as demais.

#### Alteração em `projection_academias`

Adicionar a coluna:

|Coluna|Tipo|Descrição|
|---|---|---|
|`documentos_obrigatorios`|JSONB|`{"declaracao": string[], "certificado": string[]}`. Padrão: `{"declaracao": [], "certificado": []}`|

Esta coluna é populada/atualizada pelo evento `AcademiaDocumentosObrigatoriosAtualizados` (aggregate `Academia`).

---

### 1.7 Fluxo de Negócio

#### 1.7.1 Criação da Solicitação (pelo estudante)

1. O estudante (não autenticado ou autenticado) envia um `multipart/form-data` com os campos da solicitação e os ficheiros PDF anexados.
    
2. O handler valida os campos obrigatórios (nome, genero, data_nascimento, codigo_academia, regra de BI). Para `declaracao` e `certificado`, o handler primeiro identifica o ano académico-alvo da solicitação e consulta `documentos_obrigatorios` da academia-alvo para determinar se cada um é obrigatório (ver secção 1.2.3).
    
3. O handler valida que todos os ficheiros enviados são PDF (verificar `Content-Type` e extensão).
    
4. O handler verifica que a academia existe e está ativa (`projection_academias`).
    
5. O backend gera o `codigo_solicitacao` (11 chars alfanumérico único, com `crypto/rand`).
    
6. O backend faz upload dos ficheiros para o armazenamento externo, na estrutura:
    
    ```
    {codigo_academia}/matriculas/matricula_{codigo_solicitacao}/
    ```
    
    Com os nomes de ficheiro definidos na secção 2.4.2.
    
7. Se qualquer upload falhar, os ficheiros já enviados são deletados e a solicitação não é criada (atomicidade).
    
8. O backend constrói o mapa `documentos` com os caminhos remotos retornados.
    
9. O aggregate `SolicitacaoMatricula` é criado e emite `SolicitacaoMatriculaCriada`.
    
10. O repositório grava o evento no ledger.
    
11. O Projection Manager projeta a solicitação em `projection_solicitacoes_matricula`.
    
12. O handler responde com `201` e o `codigo_solicitacao`.
    

#### 1.7.2 Aprovação da Solicitação (pela academia)

1. Academia autenticada chama `PUT /academia/solicitacao-matricula/:codigo/aprovar`.
2. O handler carrega a solicitação do ledger (aggregate `SolicitacaoMatricula`).
3. O aggregate valida que o status é `pendente` — qualquer outro status retorna erro de negócio.
4. O aggregate valida que `codigo_academia` da solicitação corresponde à academia autenticada.
5. O aggregate emite `SolicitacaoMatriculaAprovada`.
6. **Automaticamente**, o handler executa o mesmo fluxo de `POST /academia/estudante/register` usando os dados da solicitação:
    - Gera o código do estudante (formato `AAA1234`, verificando ledger e projeção).
    - Emite `EstudanteCriadoComVinculo` no aggregate `Estudante`.
    - O campo `codigo_estudante_gerado` é preenchido e salvo no payload do evento de aprovação.
7. A projeção `projection_solicitacoes_matricula` é atualizada com `status = aprovada` e `codigo_estudante_gerado`.
8. Handler responde com `200` incluindo o `codigo_estudante_gerado`.

> **Importante:** o fluxo de criação do estudante na aprovação deve reutilizar o aggregate e repositório existentes — não deve haver duplicação de lógica. O handler de aprovação orquestra os dois aggregates em sequência.

#### 1.7.3 Reprovação da Solicitação (pela academia)

1. Academia autenticada chama `PUT /academia/solicitacao-matricula/:codigo/reprovar`.
2. O handler carrega o aggregate `SolicitacaoMatricula`.
3. O aggregate valida que o status é `pendente`.
4. O aggregate valida que `codigo_academia` da solicitação corresponde à academia autenticada.
5. O `motivo_reprovacao` é obrigatório e não pode ser vazio.
6. O aggregate emite `SolicitacaoMatriculaReprovada`.
7. O handler chama o módulo de armazenamento (Módulo 2) para **deletar o diretório inteiro** `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/` com todos os ficheiros dentro.
8. A projeção é atualizada com `status = reprovada` e `motivo_reprovacao`.
9. Handler responde com `200`.

---

### 1.8 Regras de Negócio

|Regra|Detalhe|
|---|---|
|Pelo menos um BI obrigatório|`bilhete_identidade` ou `bilhete_identidade_responsavel` — os dois não podem estar vazios ao mesmo tempo|
|Cédula obrigatória sem BI do estudante|Se apenas `bi_responsavel` for enviado (sem `bi_estudante`), a `cedula` é obrigatória|
|`declaracao` obrigatória conforme configuração da academia|Obrigatória apenas se o ano académico-alvo da solicitação estiver em `documentos_obrigatorios.declaracao` da academia|
|`certificado` obrigatório conforme configuração da academia|Obrigatório apenas se o ano académico-alvo da solicitação estiver em `documentos_obrigatorios.certificado` da academia|
|`documentos_obrigatorios` só aceita anos da própria academia|Anos fora de `anos_academicos` da academia (ou dos cursos médio/superior) são rejeitados com `400`|
|Ficheiros apenas em PDF|Qualquer ficheiro que não seja PDF deve ser rejeitado com `400`|
|Academia deve estar ativa|Solicitações para academias inativas são rejeitadas com `403`|
|`codigo_solicitacao` único no sistema|Verificado na projeção antes de criar|
|Status imutável após decisão|Solicitação aprovada ou reprovada não pode ser alterada novamente|
|Aprovação só pela academia dona|Apenas a academia cujo código está na solicitação pode aprovar/reprovar|
|Motivo de reprovação obrigatório|Não pode ser vazio|
|Documentos deletados na reprovação|Por segurança e privacidade, todos os ficheiros do diretório são removidos|
|Aprovação cria o estudante automaticamente|Nenhuma ação manual adicional é necessária após aprovação|
|`codigo_academia` imutável após criação|Não pode ser alterado depois de criada a solicitação|

---

### 1.9 Permissões

|Ação|Quem pode|
|---|---|
|Criar solicitação|Público (não exige autenticação) ou estudante autenticado|
|Listar solicitações da academia|Academia autenticada (apenas as suas)|
|Listar todas as solicitações|Admin (qualquer role)|
|Consultar solicitação por código|Academia dona, admin, ou o próprio estudante (se autenticado)|
|Aprovar solicitação|Academia autenticada (apenas as suas)|
|Reprovar solicitação|Academia autenticada (apenas as suas)|

---

### 1.10 Endpoints

#### POST /solicitacao-matricula

Cria uma nova solicitação de matrícula.

**Proteção:** pública (não exige autenticação)

**Content-Type:** `multipart/form-data`

**Campos do formulário:**

|Campo|Tipo|Obrigatório|
|---|---|---|
|`codigo_academia`|string|Sim|
|`nome`|string|Sim|
|`genero`|string (`masculino`/`feminino`)|Sim|
|`data_nascimento`|string (`YYYY-MM-DD`)|Sim|
|`email`|string|Não|
|`telefone`|string|Não|
|`bilhete_identidade`|string|Condicional|
|`bilhete_identidade_responsavel`|string|Condicional|
|`ano_escolar_fundamental`|string|Não|
|`ano_escolar_medio`|string|Não|
|`curso_medio_id`|UUID string|Não|
|`ano_superior`|string|Não|
|`curso_superior_id`|UUID string|Não|

**Ficheiros (todos em PDF):**

|Campo do ficheiro|Obrigatório|
|---|---|
|`bi_estudante`|Condicional|
|`bi_responsavel`|Condicional|
|`cedula`|Condicional|
|`declaracao`|Condicional — depende de `documentos_obrigatorios.declaracao` da academia-alvo para o ano académico informado (ver 1.2.3)|
|`certificado`|Condicional — depende de `documentos_obrigatorios.certificado` da academia-alvo para o ano académico informado (ver 1.2.3)|

**Response 201:**

```json
{
  "message": "solicitação de matrícula criada com sucesso",
  "codigo_solicitacao": "A3F9K2BPQ7X",
  "codigo_academia": "LDA20261",
  "status": "pendente"
}
```

**Erros:**

- `400` — campo obrigatório ausente, data inválida, ficheiro não é PDF, BI em falta, ou documento (`declaracao`/`certificado`) ausente quando exigido pela configuração `documentos_obrigatorios` da academia para o ano académico informado
- `403` — academia inativa ou não encontrada
- `500` — falha no upload dos ficheiros (todos os ficheiros parcialmente enviados são deletados antes de retornar o erro)

---

#### GET /academia/solicitacoes-matricula

Lista as solicitações de matrícula recebidas pela academia autenticada.

**Proteção:** autenticado + academia ativa

**Query Params:**

- `status` — filtro por status (`pendente`, `aprovada`, `reprovada`). Aceita múltiplos valores.
- `limit` — padrão 50, máximo 1000
- `offset` — padrão 0

**Response 200:**

```json
{
  "solicitacoes": [SolicitacaoMatriculaDTO],
  "total": 10,
  "limit": 50,
  "offset": 0
}
```

---

#### GET /academia/solicitacao-matricula/:codigo

Consulta uma solicitação específica pelo `codigo_solicitacao`.

**Proteção:** autenticado + academia ativa (apenas as suas)

**Response 200:**

```json
{
  "solicitacao": SolicitacaoMatriculaDTO
}
```

**Erros:**

- `403` — solicitação não pertence à academia autenticada
- `404` — solicitação não encontrada

---

#### PUT /academia/solicitacao-matricula/:codigo/aprovar

Aprova uma solicitação pendente e cria o estudante automaticamente.

**Proteção:** autenticado + academia ativa

**Request:** sem payload

**Response 200:**

```json
{
  "message": "solicitação aprovada e estudante registado com sucesso",
  "codigo_solicitacao": "A3F9K2BPQ7X",
  "codigo_estudante_gerado": "ABC1234"
}
```

**Erros:**

- `403` — solicitação não pertence à academia autenticada
- `404` — solicitação não encontrada
- `409` — solicitação já foi aprovada ou reprovada

---

#### PUT /academia/solicitacao-matricula/:codigo/reprovar

Reprova uma solicitação pendente e deleta os documentos do armazenamento.

**Proteção:** autenticado + academia ativa

**Request:**

```json
{
  "motivo_reprovacao": "string"
}
```

**Response 200:**

```json
{
  "message": "solicitação reprovada com sucesso",
  "codigo_solicitacao": "A3F9K2BPQ7X"
}
```

**Erros:**

- `400` — motivo ausente ou vazio
- `403` — solicitação não pertence à academia autenticada
- `404` — solicitação não encontrada
- `409` — solicitação já foi aprovada ou reprovada

---

---

#### PUT /academia/documentos-obrigatorios

Define/atualiza a configuração `documentos_obrigatorios` da academia autenticada.

**Proteção:** autenticado + academia ativa

**Request:**

```json
{
  "declaracao": ["1_ano_fundamental", "2_ano_fundamental"],
  "certificado": ["9_ano_fundamental"]
}
```

Ambos os campos são opcionais; o que não for enviado permanece inalterado. Para limpar uma lista, enviar array vazio `[]`.

**Response 200:**

```json
{
  "message": "configuração de documentos obrigatórios atualizada com sucesso",
  "documentos_obrigatorios": {
    "declaracao": ["1_ano_fundamental", "2_ano_fundamental"],
    "certificado": ["9_ano_fundamental"]
  }
}
```

**Erros:**

- `400` — ano académico informado não pertence aos `anos_academicos` da academia (ou dos seus cursos médio/superior)

---

#### GET /academia/documentos-obrigatorios

Retorna a configuração `documentos_obrigatorios` da academia alvo.

**Proteção:** autenticado + academia ativa **ou** admin

**Query params:**

- `codigo_academia` (opcional para academia, obrigatório para admin): código da academia alvo.
    - Se o usuário for `academia`, o backend ignora o parâmetro e retorna a própria configuração.
    - Se o usuário for `admin`, deve informar `?codigo_academia=...`.

**Response 200:**

```json
{
  "codigo_academia": "LDA20261",
  "documentos_obrigatorios": {
    "declaracao": ["1_ano_fundamental", "2_ano_fundamental"],
    "certificado": ["9_ano_fundamental"]
  }
}
```

**Erros:**

- `404` — academia não encontrada (incluindo admin sem `codigo_academia`)

---

#### GET /solicitacoes-matricula (admin)

Lista todas as solicitações do sistema.

**Proteção:** autenticado + admin (qualquer role)

**Query Params:**

- `status` — filtro por status. Aceita múltiplos.
- `codigo_academia` — filtro por academia. Aceita múltiplos.
- `limit`, `offset`

**Response 200:**

```json
{
  "solicitacoes": [SolicitacaoMatriculaDTO],
  "total": 100,
  "limit": 50,
  "offset": 0
}
```

---

### 1.11 DTO da Solicitação

```typescript
interface SolicitacaoMatriculaDTO {
  id: string                         // UUID
  codigo_solicitacao: string         // 11 chars
  codigo_academia: string
  nome: string
  genero: 'masculino' | 'feminino'
  data_nascimento: string            // YYYY-MM-DD
  email?: string
  telefone?: string
  bilhete_identidade?: string
  bilhete_identidade_responsavel?: string
  ano_escolar_fundamental?: string
  ano_escolar_medio?: string
  curso_medio_id?: string
  ano_superior?: string
  curso_superior_id?: string
  status: 'pendente' | 'aprovada' | 'reprovada'
  motivo_reprovacao?: string
  documentos: {
    bi_estudante?: string
    bi_responsavel?: string
    cedula?: string
    declaracao: string
    certificado: string
  }
  codigo_estudante_gerado?: string
  aprovada_por?: string
  reprovada_por?: string
  created_at: string
  updated_at: string
  version: number
}
```

---

## 2. Módulo de Gerenciamento de Arquivos

### 2.1 Visão Geral

O Spuri precisa de armazenamento externo para guardar ficheiros dos estudantes (documentos de matrícula). Como solução **temporária e gratuita** — até a adoção de um serviço pago de CDN/nuvem — o sistema usará o **Mega** (`github.com/t3rm1n4l/go-mega`) como backend de armazenamento.

Este módulo deve expor uma **interface interna única** (`StorageProvider`), implementada exclusivamente pelo backend Mega (`MegaProvider`). A interface é definida de forma desacoplada para permitir, no futuro, a substituição por outro provedor (ex: um serviço de CDN pago) sem alterar os handlers que a consomem — mas, nesta versão, **apenas o Mega é implementado**.

---

### 2.2 Interface Interna (`StorageProvider`)

Criar um pacote `storage` (ex: `internal/storage/`) com a seguinte interface Go:

```go
package storage

import "io"

// StorageProvider é a interface implementada pelo backend de armazenamento.
// Nesta versão, a única implementação é MegaProvider (Mega).
type StorageProvider interface {
    // Upload envia um ficheiro para o caminho remoto especificado.
    // remotePath é o caminho completo incluindo o nome do ficheiro, ex:
    // "LDA20261/matriculas/matricula_A3F9K2BPQ7X/bi_estudante_A3F9K2BPQ7X.pdf"
    Upload(remotePath string, content io.Reader, sizeBytes int64) error

    // Delete remove um ficheiro ou diretório (e todo o seu conteúdo) pelo caminho remoto.
    Delete(remotePath string) error

    // GetQuota retorna informação sobre o espaço de armazenamento.
    GetQuota() (QuotaInfo, error)

    // EnsureDir garante que o diretório remoto existe, criando-o se necessário.
    // Deve criar todos os diretórios intermediários (mkdir -p equivalente).
    EnsureDir(remotePath string) error
}

// QuotaInfo contém informações de uso do armazenamento.
type QuotaInfo struct {
    TotalBytes     uint64
    UsedBytes      uint64
    AvailableBytes uint64
}
```

---

### 2.3 Autenticação no Mega

O backend Mega deve suportar **três modos de autenticação** configuráveis por variáveis de ambiente. O modo é determinado pela variável `MEGA_AUTH_MODE`.

Além disso, o backend deve implementar **persistência automática de sessão**: após a primeira autenticação bem-sucedida via `password` ou `2fa`, o `SessionID` e o `MasterKey` resultantes são salvos automaticamente em disco. Em inicializações subsequentes, o backend detecta essa sessão salva e autentica-se diretamente pelo Modo 3 (`session`), **sem repetir o login com email/senha/TOTP**. Isto elimina a necessidade de qualquer autenticação manual recorrente — a autenticação por credenciais só ocorre uma vez (ou quando a sessão salva expira/torna-se inválida).

#### Modo 1: Email + Senha (`MEGA_AUTH_MODE=password`)

Caso padrão. Usado em desenvolvimento ou quando 2FA não está ativo.

**Variáveis de ambiente necessárias:**

```env
MEGA_AUTH_MODE=password
MEGA_EMAIL=conta@exemplo.com
MEGA_PASSWORD=suasenha
```

**Código:**

```go
m := mega.New()
err := m.Login(os.Getenv("MEGA_EMAIL"), os.Getenv("MEGA_PASSWORD"))
```

#### Modo 2: Email + Senha + 2FA (`MEGA_AUTH_MODE=2fa`)

Usado quando a conta Mega tem autenticação de dois fatores ativada.

**Variáveis de ambiente necessárias:**

```env
MEGA_AUTH_MODE=2fa
MEGA_EMAIL=conta@exemplo.com
MEGA_PASSWORD=suasenha
MEGA_TOTP_CODE=123456
```

> **Nota:** o código TOTP expira rapidamente, mas como a sessão resultante é persistida automaticamente (ver secção 2.3.1), este login só precisa ser feito uma vez — nas próximas inicializações o backend usa a sessão salva (Modo 3) automaticamente.

**Código:**

```go
m := mega.New()
err := m.MultiFactorLogin(
    os.Getenv("MEGA_EMAIL"),
    os.Getenv("MEGA_PASSWORD"),
    os.Getenv("MEGA_TOTP_CODE"),
)
```

#### Modo 3: Session ID + Master Key (`MEGA_AUTH_MODE=session`)

Modo mais seguro para servidores e daemons em produção. Não expõe a senha. Permite reutilizar uma sessão já autenticada sem precisar das credenciais novamente.

Este modo pode ser configurado **manualmente** (via variáveis de ambiente) ou ser usado **automaticamente** pelo backend a partir da sessão persistida em disco (ver secção 2.3.1) — neste segundo caso, `MEGA_AUTH_MODE` pode permanecer como `password` ou `2fa` e o backend ainda assim usará a sessão salva quando disponível, sem necessidade de alterar a variável manualmente.

**Variáveis de ambiente (configuração manual, opcional):**

```env
MEGA_AUTH_MODE=session
MEGA_SESSION_ID=<session_id_exportado>
MEGA_MASTER_KEY=<master_key_em_base64>
```

**Código:**

```go
m := mega.New()
masterKeyBytes, err := base64.StdEncoding.DecodeString(os.Getenv("MEGA_MASTER_KEY"))
if err != nil {
    return nil, fmt.Errorf("MEGA_MASTER_KEY inválido: %w", err)
}
err = m.LoginWithKeys(os.Getenv("MEGA_SESSION_ID"), masterKeyBytes)
```

#### 2.3.1 Persistência Automática de Sessão

Após qualquer autenticação bem-sucedida via `Login` (Modo 1) ou `MultiFactorLogin` (Modo 2), o backend deve:

1. Obter `SessionID` e `MasterKey` da instância autenticada do `mega.Mega`.
2. Codificar `MasterKey` em base64.
3. Persistir ambos os valores num ficheiro local de sessão, ex: `data/mega_session.json`:

```json
{
  "session_id": "xxxxxxxx",
  "master_key": "base64xxxxx",
  "saved_at": "2026-06-12T18:30:00Z"
}
```

> O caminho do ficheiro é configurável via `MEGA_SESSION_FILE` (padrão: `data/mega_session.json`). Este ficheiro **não deve ser versionado** (adicionar a `.gitignore`) por conter credenciais de sessão sensíveis.

**Fluxo de inicialização do `NewMegaProvider()`:**

```
1. Se MEGA_SESSION_FILE existir e for legível:
     a. Tentar autenticar via LoginWithKeys(session_id, master_key) (Modo 3)
     b. Se sucesso → retornar provider autenticado, FIM
     c. Se falhar (sessão expirada/inválida) → seguir para o passo 2
2. Autenticar conforme MEGA_AUTH_MODE (password ou 2fa)
3. Se sucesso → persistir nova sessão em MEGA_SESSION_FILE (sobrescrevendo a anterior, se houver)
4. Retornar provider autenticado
```

Desta forma:

- Na **primeira execução**, o backend autentica via `password`/`2fa` (usando as credenciais do `.env`) e salva a sessão automaticamente.
- Em **execuções seguintes**, o backend detecta o ficheiro de sessão e autentica via Modo 3 diretamente — **nenhuma nova autenticação por credenciais é necessária**.
- Se a sessão salva expirar ou for invalidada (ex: senha alterada na conta Mega), o backend faz fallback automático para `password`/`2fa` e regrava o ficheiro de sessão.

#### 2.3.2 Utilitário de Exportação de Sessão Mega (opcional/manual)

Para os casos em que se deseja configurar a sessão **manualmente** (ex: ambientes onde o ficheiro de sessão não pode ser persistido em disco, como containers efémeros), mantém-se disponível um utilitário CLI em `cmd/mega-export-session/main.go` que:

1. Faz login com email + senha (e opcionalmente TOTP).
2. Imprime o `SessionID` e o `MasterKey` (em base64) no stdout.
3. O operador copia esses valores para `MEGA_SESSION_ID` e `MEGA_MASTER_KEY` e define `MEGA_AUTH_MODE=session`.

```go
// Uso:
// go run cmd/mega-export-session/main.go
// O programa solicita email, senha e, opcionalmente, código TOTP interativamente.
// Imprime:
//   MEGA_SESSION_ID=xxxxx
//   MEGA_MASTER_KEY=base64xxxxx
```

Quando `MEGA_SESSION_ID` e `MEGA_MASTER_KEY` são definidos manualmente via variáveis de ambiente E `MEGA_AUTH_MODE=session`, o backend usa esses valores diretamente, sem passar pelo fluxo de ficheiro de sessão (secção 2.3.1).

#### 2.3.3 Inicialização do Cliente Mega

A função de inicialização do cliente Mega implementa o fluxo descrito em 2.3.1:

```go
func NewMegaProvider() (StorageProvider, error) {
    m := mega.New()
    sessionFile := getEnvOrDefault("MEGA_SESSION_FILE", "data/mega_session.json")

    // 1. Tenta autenticar com sessão persistida (Modo 3 automático)
    if session, err := loadSessionFile(sessionFile); err == nil {
        if err := m.LoginWithKeys(session.SessionID, session.MasterKey); err == nil {
            return &MegaProvider{client: m}, nil
        }
        // sessão inválida/expirada — segue para autenticação por credenciais
    }

    // 2. Configuração manual explícita do Modo 3 (sem ficheiro de sessão)
    mode := os.Getenv("MEGA_AUTH_MODE")
    if mode == "session" {
        masterKeyBytes, err := base64.StdEncoding.DecodeString(os.Getenv("MEGA_MASTER_KEY"))
        if err != nil {
            return nil, fmt.Errorf("MEGA_MASTER_KEY inválido: %w", err)
        }
        if err := m.LoginWithKeys(os.Getenv("MEGA_SESSION_ID"), masterKeyBytes); err != nil {
            return nil, fmt.Errorf("mega session login falhou: %w", err)
        }
        return &MegaProvider{client: m}, nil
    }

    // 3. Autenticação por credenciais (password ou 2fa)
    switch mode {
    case "password":
        if err := m.Login(os.Getenv("MEGA_EMAIL"), os.Getenv("MEGA_PASSWORD")); err != nil {
            return nil, fmt.Errorf("mega login falhou: %w", err)
        }
    case "2fa":
        if err := m.MultiFactorLogin(
            os.Getenv("MEGA_EMAIL"),
            os.Getenv("MEGA_PASSWORD"),
            os.Getenv("MEGA_TOTP_CODE"),
        ); err != nil {
            return nil, fmt.Errorf("mega 2FA login falhou: %w", err)
        }
    default:
        return nil, fmt.Errorf("MEGA_AUTH_MODE inválido ou não definido: %q", mode)
    }

    // 4. Persiste a nova sessão para evitar autenticação por credenciais nas próximas execuções
    if err := saveSessionFile(sessionFile, m.GetSessionID(), m.GetMasterKey()); err != nil {
        log.Printf("aviso: falha ao persistir sessão Mega: %v", err)
    }

    return &MegaProvider{client: m}, nil
}
```

---

### 2.4 Estrutura de Diretórios no Armazenamento Externo

#### 2.4.1 Estrutura Geral

```
/ (raiz da conta)
└── {codigo_academia}/              ex: LDA20261/
    └── matriculas/
        └── matricula_{codigo_solicitacao}/    ex: matricula_A3F9K2BPQ7X/
            ├── bi_estudante_A3F9K2BPQ7X.pdf
            ├── bi_responsavel_A3F9K2BPQ7X.pdf
            ├── cedula_A3F9K2BPQ7X.pdf
            ├── declaracao_A3F9K2BPQ7X.pdf
            └── certificado_A3F9K2BPQ7X.pdf
```

**Regras da estrutura:**

- O diretório de cada academia tem **o código da academia como nome** (ex: `LDA20261`). É criado automaticamente na primeira solicitação de matrícula daquela academia (`EnsureDir`).
- Dentro da academia, existe sempre um diretório `matriculas/`.
- Cada solicitação tem o seu próprio diretório: `matricula_{codigo_solicitacao}/`.
- O `codigo_solicitacao` é **o mesmo** em todos os nomes de ficheiro dentro do diretório.

#### 2.4.2 Nomes dos Ficheiros

|Tipo de documento|Nome do ficheiro|
|---|---|
|BI do estudante|`bi_estudante_{codigo_solicitacao}.pdf`|
|BI do responsável|`bi_responsavel_{codigo_solicitacao}.pdf`|
|Cédula|`cedula_{codigo_solicitacao}.pdf`|
|Declaração|`declaracao_{codigo_solicitacao}.pdf`|
|Certificado|`certificado_{codigo_solicitacao}.pdf`|

Exemplo com `codigo_solicitacao = A3F9K2BPQ7X`:

```
bi_estudante_A3F9K2BPQ7X.pdf
bi_responsavel_A3F9K2BPQ7X.pdf
cedula_A3F9K2BPQ7X.pdf
declaracao_A3F9K2BPQ7X.pdf
certificado_A3F9K2BPQ7X.pdf
```

---

### 2.5 Operações do StorageProvider

#### 2.5.1 Upload de Ficheiros (na criação da solicitação)

O handler de criação da solicitação deve:

1. Receber os ficheiros do `multipart/form-data`.
    
2. Para cada ficheiro recebido, construir o caminho remoto:
    
    ```
    {codigo_academia}/matriculas/matricula_{codigo_solicitacao}/{tipo}_{codigo_solicitacao}.pdf
    ```
    
3. Chamar `EnsureDir` para garantir que o diretório da matrícula existe.
    
4. Chamar `Upload` para cada ficheiro.
    
5. Se qualquer `Upload` falhar, chamar `Delete` no diretório `matricula_{codigo_solicitacao}/` para limpar os ficheiros parcialmente enviados.
    
6. Só após todos os uploads concluídos com sucesso, gravar o evento no ledger.
    

**Atomicidade:** o evento `SolicitacaoMatriculaCriada` só é emitido se **todos** os uploads foram bem-sucedidos. Em caso de falha parcial, os ficheiros enviados são removidos e a solicitação não é criada.

#### 2.5.2 Deleção de Ficheiros (na reprovação da solicitação)

Ao reprovar uma solicitação:

1. Construir o caminho do diretório:
    
    ```
    {codigo_academia}/matriculas/matricula_{codigo_solicitacao}/
    ```
    
2. Chamar `Delete` nesse caminho — deve deletar o diretório e **todo o seu conteúdo recursivamente**.
    
3. A deleção deve ocorrer após a gravação do evento `SolicitacaoMatriculaReprovada` no ledger.
    
4. Se a deleção de ficheiros falhar, o sistema deve **logar o erro** mas não reverter a reprovação — o estado da solicitação já está no ledger. A limpeza de ficheiros órfãos pode ser feita manualmente depois.
    

#### 2.5.3 Informações de Quota (para admin)

A operação `GetQuota` retorna:

```go
type QuotaInfo struct {
    TotalBytes     uint64  // Espaço total da conta
    UsedBytes      uint64  // Espaço já utilizado
    AvailableBytes uint64  // Espaço disponível (Total - Used)
}
```

No Mega, isso é obtido via `m.GetQuota()`.

---

### 2.6 Endpoints de Administração do Armazenamento

#### GET /dominis/storage/quota

Retorna informações de uso do armazenamento externo.

**Proteção:** autenticado + admin (qualquer role)

**Response 200:**

```json
{
  "provider": "mega",
  "total_bytes": 53687091200,
  "used_bytes": 1073741824,
  "available_bytes": 52613349376,
  "total_human": "50 GB",
  "used_human": "1 GB",
  "available_human": "49 GB"
}
```

**Implementação:** os valores `*_human` são calculados no backend (conversão de bytes para unidades legíveis: KB, MB, GB, TB).

**Erros:**

- `500` — falha ao consultar o armazenamento externo

---

### 2.7 Variáveis de Ambiente Completas

Todas as variáveis de ambiente relacionadas ao módulo de armazenamento:

```env
# Modo de autenticação Mega usado na PRIMEIRA autenticação por credenciais: "password" ou "2fa"
# (irrelevante em execuções subsequentes, quando já existe sessão persistida em MEGA_SESSION_FILE)
MEGA_AUTH_MODE=password

# Modo "password" e "2fa"
MEGA_EMAIL=conta@exemplo.com
MEGA_PASSWORD=suasenha

# Modo "2fa" adicional
MEGA_TOTP_CODE=123456

# Caminho do ficheiro onde a sessão (session_id + master_key) é persistida
# automaticamente após a primeira autenticação. Padrão: data/mega_session.json
MEGA_SESSION_FILE=data/mega_session.json

# Configuração manual do Modo 3 (opcional — alternativa ao MEGA_SESSION_FILE,
# útil em ambientes onde não é possível persistir ficheiros, ex: containers efémeros)
# Quando definidas junto com MEGA_AUTH_MODE=session, têm prioridade sobre MEGA_SESSION_FILE.
MEGA_SESSION_ID=<session_id>
MEGA_MASTER_KEY=<master_key_em_base64>
```

---

### 2.8 Inicialização no Startup da Aplicação

No ponto de inicialização do servidor (ex: `main.go`), o provider de armazenamento deve ser instanciado uma única vez e injetado via dependência nos handlers que precisam dele:

```go
storageProvider, err := storage.NewMegaProvider() // configura autenticação e gerencia sessão automaticamente
if err != nil {
    log.Fatalf("falha ao inicializar armazenamento (Mega): %v", err)
}
```

`NewMegaProvider()` segue o fluxo descrito na secção 2.3.1:

1. Tenta carregar e usar uma sessão persistida em `MEGA_SESSION_FILE` (Modo 3 automático). Se válida, autentica sem usar credenciais.
2. Se `MEGA_AUTH_MODE=session` com `MEGA_SESSION_ID`/`MEGA_MASTER_KEY` definidos manualmente (e sem sessão em disco), usa esses valores diretamente.
3. Caso contrário, autentica via `MEGA_AUTH_MODE` (`password` ou `2fa`) usando `MEGA_EMAIL`/`MEGA_PASSWORD`/`MEGA_TOTP_CODE`, e persiste a sessão resultante em `MEGA_SESSION_FILE` para uso nas próximas inicializações.
4. Qualquer combinação inválida/incompleta de variáveis: retornar erro fatal.

> Caso, no futuro, outro provedor de armazenamento seja adicionado (ex: um serviço de CDN pago), a função de inicialização poderá evoluir para `storage.NewProvider()`, que escolheria o backend via uma variável `STORAGE_PROVIDER`. Por ora, **apenas o Mega é suportado** e a inicialização é direta.

---

### 2.9 Validação de Ficheiros PDF

Antes de qualquer upload, o handler deve validar que o ficheiro é um PDF válido:

1. Verificar que o `Content-Type` do multipart é `application/pdf`.
2. Ler os primeiros 4 bytes do ficheiro e verificar a assinatura mágica do PDF: `%PDF` (bytes: `0x25 0x50 0x44 0x46`).
3. Se qualquer uma das verificações falhar, rejeitar o request com `400` antes de tentar qualquer upload.

---

### 2.10 Rebuild da Projeção de Solicitações

A projeção `projection_solicitacoes_matricula` deve ser incluída na lista de projeções reconstruíveis via:

```
POST /dominis/projections/rebuild/solicitacoes_matricula
POST /dominis/projections/rebuild/solicitacoes_matricula/async
```

**Ordem recomendada no rebuild geral** (adicionando à ordem já existente na documentação):

1. `admins`
2. `academias`
3. `cursos`, `materias`, `categorias_nota`, `telefones_extra`
4. `estudantes`, `turmas`
5. `notas`, `faltas`
6. `avaliacao_final`
7. `solicitacoes_matricula` ← **nova**

> **Atenção:** o rebuild da projeção `solicitacoes_matricula` **não** re-executa uploads de ficheiros. Os caminhos remotos são lidos diretamente dos payloads dos eventos do ledger, onde foram gravados na criação. O armazenamento externo não é afetado pelo rebuild.

---

## Sumário das Alterações

### Novos Ficheiros Sugeridos

```
internal/
  storage/
    provider.go          # Interface StorageProvider
    mega.go              # Implementação MegaProvider + NewMegaProvider()
    session.go           # Persistência/carregamento do ficheiro de sessão Mega (MEGA_SESSION_FILE)
    quota.go             # QuotaInfo struct e helpers de formatação

aggregates/
  solicitacao_matricula.go   # Aggregate SolicitacaoMatricula

projections/
  solicitacoes_matricula.go  # Projection Manager handler

handlers/
  solicitacao_matricula.go   # Handlers HTTP

cmd/
  mega-export-session/
    main.go              # Utilitário CLI de exportação manual de sessão Mega (opcional)

data/
  mega_session.json     # Gerado automaticamente em runtime — NÃO versionar (adicionar ao .gitignore)
```

### Novas Projeções

- `projection_solicitacoes_matricula`

### Novos Eventos no Ledger

- `SolicitacaoMatriculaCriada`
- `SolicitacaoMatriculaAprovada`
- `SolicitacaoMatriculaReprovada`
- `AcademiaDocumentosObrigatoriosAtualizados` (aggregate `Academia`)

Estes eventos devem ser adicionados à whitelist de eventos autorizados em `safe_queries.go`.

### Novos Endpoints

|Método|Rota|Quem acede|
|---|---|---|
|`POST`|`/solicitacao-matricula`|Público|
|`GET`|`/academia/solicitacoes-matricula`|Academia|
|`GET`|`/academia/solicitacao-matricula/:codigo`|Academia|
|`PUT`|`/academia/solicitacao-matricula/:codigo/aprovar`|Academia|
|`PUT`|`/academia/solicitacao-matricula/:codigo/reprovar`|Academia|
|`PUT`|`/academia/documentos-obrigatorios`|Academia|
|`GET`|`/academia/documentos-obrigatorios`|Academia / Admin|
|`GET`|`/solicitacoes-matricula`|Admin|
|`GET`|`/dominis/storage/quota`|Admin|

---

## 3. Orientações Finais de Atualização

### 3.1 Atualização das Documentações (`Spuri - Documentação.md` e `Spuri - API.md`)

Ambos os documentos de referência do sistema devem ser atualizados para refletir esta nova versão. Especificamente:

**Em `Spuri - Documentação.md`:**

- Incrementar `Versão atual` no cabeçalho do documento (ex: de `1.5.0` para `1.6.0`, conforme a magnitude das mudanças — duas novas entidades/módulos justificam um incremento de versão minor).
- Adicionar uma nova entidade na secção "4. Entidades do Sistema": **4.x SolicitacaoMatricula**, com a mesma estrutura usada para as demais entidades (descrição, código único, estados, eventos).
- Adicionar `documentos_obrigatorios` na descrição da entidade **4.2 Academia** (campos e eventos).
- Adicionar um novo processo de negócio na secção "5. Processos de Negócio": **Solicitação e Aprovação/Reprovação de Matrícula**, descrevendo o fluxo completo (criação pelo estudante → análise pela academia → aprovação automática do cadastro ou reprovação com exclusão de documentos).
- Adicionar uma nova subsecção em "6. Regras de Negócio": **Regras de Solicitação de Matrícula**, espelhando a tabela da secção 1.8 deste documento.
- Atualizar a secção "7. Sistema de Permissões" (matriz de permissões) incluindo as novas rotas e quem pode acedê-las.
- Adicionar uma nova secção descrevendo o **Módulo de Armazenamento de Arquivos (Mega)**: visão geral, estrutura de diretórios, autenticação com persistência automática de sessão (secção 2.3 deste documento) — em nível de visão geral, sem repetir todos os trechos de código.
- Atualizar a secção "5.10 Rebuild de Projeções" incluindo `solicitacoes_matricula` na ordem recomendada.
- Revisar o índice (`Índice`) do documento para incluir as novas secções/subsecções.

**Em `Spuri - API.md`:**

- Incrementar `Versão atual` no cabeçalho do documento (ex: de `1.6.2` para `1.7.0`).
- Adicionar `documentos_obrigatorios` à interface `AcademiaDTO` (secção "2.3 Academia").
- Adicionar uma nova interface `SolicitacaoMatriculaDTO` na secção "2. Estruturas de Dados" (usar a definição da secção 1.11 deste documento).
- Adicionar uma nova secção numerada (ex: "9.x Solicitação de Matrícula" ou nova secção principal, renumerando as seguintes) com todos os endpoints da secção 1.10 deste documento, no mesmo formato usado pelas demais rotas (Proteção, Request, Response, Erros).
- Adicionar os dois novos endpoints `PUT`/`GET /academia/documentos-obrigatorios` na secção referente a "Academia — Operações Próprias".
- Adicionar `GET /dominis/storage/quota` na secção referente a Admins ou criar uma nova secção "Armazenamento".
- Atualizar o Índice do documento com as novas secções/subsecções e renumerar as secções subsequentes, se necessário.
- Revisar a tabela "Códigos HTTP" e o "Envelope de Erro" apenas se novos códigos/formatos forem introduzidos (não é o caso nesta atualização — manter como está, apenas confirmar consistência).

> **Importante:** ao final da implementação, ambos os documentos devem estar **consistentes entre si** e com o código — qualquer divergência entre o que a API faz e o que a documentação descreve deve ser corrigida na documentação (a implementação é a fonte de verdade, mas a documentação deve sempre refletir o estado atual do sistema).

### 3.2 Atualização do `.env.example`

O ficheiro `.env.example` na raiz do projeto deve ser atualizado para incluir todas as novas variáveis de ambiente introduzidas por esta atualização (secção 2.7), com valores de exemplo (não reais) e comentários explicativos:

```env
# ============================================
# Armazenamento de Arquivos (Mega)
# ============================================

# Modo de autenticação usado na PRIMEIRA autenticação por credenciais: "password" ou "2fa"
# Em execuções subsequentes, se MEGA_SESSION_FILE já existir, este valor é ignorado.
MEGA_AUTH_MODE=password

# Credenciais da conta Mega (usadas apenas na primeira autenticação ou quando a sessão expira)
MEGA_EMAIL=
MEGA_PASSWORD=

# Código TOTP — apenas se MEGA_AUTH_MODE=2fa
MEGA_TOTP_CODE=

# Caminho do ficheiro de sessão persistida automaticamente (NÃO versionar)
MEGA_SESSION_FILE=data/mega_session.json

# Configuração manual opcional do Modo 3 (alternativa a MEGA_SESSION_FILE)
MEGA_SESSION_ID=
MEGA_MASTER_KEY=
```

**Checklist da atualização do `.env.example`:**

- Adicionar todas as variáveis listadas acima, na secção apropriada do ficheiro (criar uma nova secção "Armazenamento de Arquivos (Mega)" se o ficheiro for organizado por blocos).
- Deixar os valores sensíveis (`MEGA_EMAIL`, `MEGA_PASSWORD`, `MEGA_SESSION_ID`, `MEGA_MASTER_KEY`, `MEGA_TOTP_CODE`) **vazios** no `.env.example` (são apenas placeholders).
- Confirmar que `data/mega_session.json` (ou o caminho definido em `MEGA_SESSION_FILE`) está listado no `.gitignore` do projeto.
- Não remover nenhuma variável de ambiente já existente no `.env.example` — esta é uma adição, não uma substituição.