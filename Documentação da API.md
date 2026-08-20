---
modificado: 21-07-2026 00:00
criado: 05-04-2026 13:01
---
Versão atual: 2.3.0
## Índice

1. [Convenções Globais](#1-convenções-globais)
2. [Estruturas de Dados](#2-estruturas-de-dados)
3. [Autenticação](#3-autenticação)
4. [Perfil e Conta](#4-perfil-e-conta)
5. [Email](#5-email)
6. [Academias](#6-academias)
7. [Ano Letivo](#7-ano-letivo)
8. [Estudantes](#8-estudantes)
9. [Solicitação de Matrícula](#9-solicitação-de-matrícula)
   - [9.1 `POST /solicitacao-matricula`](#91-post-solicitacao-matricula)
   - [9.2 `GET /solicitacoes-matricula`](#92-get-solicitacoes-matricula)
   - [9.3 `GET /academia/solicitacoes-matricula`](#93-get-academiasolicitacoes-matricula)
   - [9.4 `GET /academia/solicitacao-matricula/:codigo`](#94-get-academiasolicitacao-matriculacodigo)
   - [9.5 `PUT /academia/solicitacao-matricula/:codigo/aprovar`](#95-put-academiasolicitacao-matriculacodigoaprovar)
   - [9.6 `PUT /academia/solicitacao-matricula/:codigo/reprovar`](#96-put-academiasolicitacao-matriculacodigoreprovar)
   - [9.7 `GET /documentos/solicitacoes-matricula/:codigo/:campo/download`](#97-get-documentossolicitacoes-matriculacodigocampodownload)
   - [9.8 `GET /academia/documentos/solicitacoes-matricula/:codigo/:campo/download`](#98-get-academiadocumentossolicitacoes-matriculacodigocampodownload)
   - [9.9 `GET /solicitacao-matricula/busca`](#99-get-solicitacao-matriculabusca)
   - [9.10 `GET /solicitacao-matricula/:codigo/status`](#910-get-solicitacao-matriculacodigostatus)
   - [9.11 `POST /solicitacao-matricula/:codigo/pagamento-matricula`](#911-post-solicitacao-matriculacodigopagamento-matricula)
   - [9.12 `PUT /academia/solicitacao-matricula/:codigo/cancelar`](#912-put-academiasolicitacao-matriculacodigocancelar)
10. [Cursos](#10-cursos)
    - [10.1 `GET /academia/cursos`](#101-get-academiacursos)
    - [10.2 `GET /academia/curso/:id`](#102-get-academiacursoid)
    - [10.3 `POST /academia/curso`](#103-post-academiacurso)
    - [10.4 `PUT /academia/curso/:id/ativar`](#104-put-academiacursoidativar)
    - [10.5 `PUT /academia/curso/:id/desativar`](#105-put-academiacursoiddesativar)
    - [10.6 `PUT /academia/curso/:id/dados`](#106-put-academiacursoiddados)
    - [10.7 `DELETE /academia/curso/:id`](#107-delete-academiacursoid)
    - [10.8 `POST /academia/curso/async`](#108-post-academiacursoasync)
    - [10.9 `PUT /academia/curso/ativar/async`](#109-put-academiacursoativarasync)
    - [10.10 `PUT /academia/curso/desativar/async`](#1010-put-academiacursodesativarasync)
    - [10.11 `PUT /academia/curso/dados/async`](#1011-put-academiacursodadosasync)
    - [10.12 `DELETE /academia/curso/async`](#1012-delete-academiacursoasync)
11. [Matérias](#11-matérias)
12. [Turmas](#12-turmas)
13. [Notas](#13-notas)
14. [Faltas](#14-faltas)
15. [Avaliações Finais](#15-avaliações-finais)
16. [Admins](#16-admins)
17. [Jobs Assíncronos](#17-jobs-assíncronos)
18. [Batch Assíncrono](#18-batch-assíncrono)
19. [Financeiro / AppyPay](#19-financeiro--appypay)
20. [Armazenamento](#20-armazenamento)
21. [Integrações Externas / Ziett (Teste)](#21-integrações-externas--ziett-teste)

---

## 1. Convenções Globais

### Atores do Sistema

| Ator              | Papel                                                              |
| ----------------- | ------------------------------------------------------------------ |
| **Admin FPP**     | Administrador máximo. Cria academias, outros admins, faz rebuilds. |
| **Admin ADM**     | Ativa/desativa academias e admins de nível inferior.               |
| **Admin Gerente** | Consultas e ações básicas administrativas.                         |
| **Academia**      | Gere estudantes, notas, faltas, cursos, matérias e turmas.         |
| **Estudante**     | Visualiza os próprios dados académicos.                            |

---

### Envelope de Erro

Todas as rotas retornam erros exclusivamente neste envelope padronizado (`utils.RespondWithError`/`RespondWithDetailedError`):

```json
{
  "error": "VALIDATION_ERROR | UNAUTHORIZED | FORBIDDEN | NOT_FOUND | CONFLICT | RATE_LIMIT | INTERNAL_ERROR | ERROR",
  "message": "mensagem de erro para o cliente",
  "request_id": "identificador da requisição",
  "details": [
    {
      "field": "type",
      "code": "required",
      "message": "o campo 'type' é obrigatório"
    }
  ]
}
```

> `details` é opcional. Ele aparece quando a rota consegue apontar exatamente
> o campo, o código interno do problema e uma explicação acionável para o
> cliente corrigir a requisição. Em `/academia/anos-academicos`, `details`
> também pode aparecer em `409 Conflict` quando a alteração é bloqueada por
> estudantes ativos vinculados ao ano/período removido.

---

### 1.1.1 Conflito por operação única em andamento

Rotas que criam ou alteram dados com unicidade funcional usam uma guarda transacional no PostgreSQL antes de uploads, validações caras e gravação de eventos. Quando outra requisição equivalente ainda está em andamento, a API retorna `409 Conflict` com mensagem de operação pendente/em criação, mesmo antes de a projeção ficar consultável.

A proteção cobre explicitamente `POST /estudante/solicitacoes-edicao/nome`, `POST /estudante/solicitacoes-edicao/bilhete-identidade`, `POST /estudante/solicitacoes-edicao/bilhete-identidade-encarregado` e `POST /estudante/solicitacoes-edicao/data-nascimento` pela chave canônica `codigo_estudante + campo`; cobre solicitações de status acadêmico pendentes por `codigo_estudante + codigo_academia + tipo`; e cobre a criação efetiva de estudante por BI principal normalizado nos fluxos de cadastro direto e aprovação de matrícula. Reservas são liberadas em falhas antes da persistência e finalizadas/liberadas quando o fato único deixa de estar pendente.

---

## 2. Estruturas de Dados

### 2.1 Tipos Base

```typescript
type UserType   = 'academia' | 'estudante' | 'admin'
type AdminRole  = 'fpp' | 'adm' | 'gerente'
type AcademiaNivel = 'escola' | 'superior'
type AcademiaType = 'public' | 'private'
type NivelEscolar = 'fundamental' | 'medio' | 'misto'
type StatusGeralEstudante = 'inativo' | 'ativo' | 'pendente_documentos'
type StatusEscolar = 'inativo' | 'em_andamento' | 'finalizado'
type TipoEnsino = 'fundamental' | 'medio' | 'superior'
type Turno = 'manha' | 'tarde' | 'noite'
type CursoType = 'medio' | 'superior'
type ModeloCursoMedio = 'liceu' | 'tecnico'
type MateriaType = 'fundamental' | 'medio' | 'superior'
type Genero = 'masculino' | 'feminino'
type TipoNota = 'escolar' | 'superior'
type JobStatus = 'pending' | 'processing' | 'done' | 'failed'
type JobEventType = 'job_enqueued' | 'job_progress' | 'job_done' | 'job_failed'
type SolicitacaoMatriculaStatus = 'pendente' | 'aprovada' | 'reprovada' | 'cancelada' | 'aprovada_pendente_pagamento_matricula'
```

**Períodos de nota:**

- Escolar (fixos): `1_trimestre`, `2_trimestre`, `3_trimestre`
- Superior (dinâmicos): `1_semestre`, `2_semestre`, ..., `N_semestre`

**Categorias de nota:**

- Escolas (`nivel="escola"`) usam categorias fixas do sistema por ano acadêmico; podem consultá-las por `GET /academia/categorias-nota`, mas não podem criar/remover categorias.
- Academias superiores (`nivel="superior"`) continuam usando categorias configuráveis; toda categoria usada para lançar nota, montar fórmula ou validar regra superior deve ser cadastrada explicitamente pela academia.

```typescript
type CategoriaNotaEscolarFixaCodigo =
  | 'nota_professor'
  | 'prova_trimestral'
  | 'exame_final'
  | 'exame_recurso'
  | 'nota_pap'
```

| Código fixo | Rótulo | Aplicação escolar |
| --- | --- | --- |
| `nota_professor` | Nota do professor/Avaliação contínua | Anos regulares do fundamental e médio; também aparece nos anos com exame. |
| `prova_trimestral` | Prova trimestral | Anos regulares do fundamental e médio; também aparece nos anos com exame. |
| `exame_final` | Exame final | Apenas `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`. |
| `exame_recurso` | Exame de recurso | Apenas `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`. |
| `nota_pap` | Prova de Aptidão Profissional | Apenas `4_ano_medio` de curso médio com `modelo="tecnico"`. |

**Formato do ano letivo:** `YYYY_YYYY` (ex: `2025_2026`)

---

### 2.2 Admin

```typescript
interface AdminDTO {
  id: string               // UUID
  nome: string
  email: string
  role: AdminRole          // 'fpp' | 'adm' | 'gerente'
  status: string           // 'ativo' | 'inativo'
  email_verificado: boolean
  telefone?: string
  telefone_verificado: boolean  // reservado; verificação ainda não implementada
  created_by?: string      // UUID do admin criador (null para o primeiro FPP)
  total_acoes_realizadas?: number
  created_at: string       // RFC3339
  updated_at: string       // RFC3339
  version: number
}
```

---

### 2.3 Academia

```typescript
interface AcademiaDTO {
  id: string
  nivel: AcademiaNivel            // 'escola' | 'superior'
  type: AcademiaType              // 'public' | 'private'
  nome: string
  codigo_academia: string         // ex: 'LDA20261'
  provincia: string               // código de 3 letras, ex: 'LDA'
  endereco: string
  telefone?: string              // 9 dígitos, sem DDI
  telefone_verificado: boolean   // reservado; verificação ainda não implementada
  email?: string
  email_verificado: boolean
  website?: string
  nivel_escolar?: NivelEscolar    // apenas para nivel='escola'
  anos_academicos?: string[]      // anos do fundamental (ex: ['1_ano_fundamental'])
  status: string                  // 'ativo' | 'inativo'
  cursos: string[]                // lista de nomes de cursos
  motivo_desativacao?: string     // apenas para admin ver
  total_estudantes: number
  ano_letivo?: string             // ex: '2025_2026'
  tipo_ano_letivo?: string        // 'escolar' | 'superior'
  ano_letivo_ativado_em?: string  // RFC3339
  anos_letivos_lista: AnoLetivoItem[]
  documentos?: Record<string, SolicitacaoMatriculaDocumentoDTO> // inclui alvara para usuários autenticados
  created_at: string
  updated_at?: string
  version: number
}

interface AnoLetivoItem {
  ano_letivo: string              // ex: '2025_2026'
  tipo: 'escolar' | 'superior'
  definido_por: string            // UUID da academia
  definido_em: string             // RFC3339
}
```

---

### 2.4 Estudante

```typescript
interface EstudanteDTO {
  id: string
  nome: string
  codigo_estudante: string        // ex: 'ABC1234'
  email?: string
  telefone?: string              // 9 dígitos, sem DDI
  telefone_verificado: boolean   // reservado; verificação ainda não implementada
  telefone_encarregado?: string  // 9 dígitos, sem DDI
  telefone_encarregado_verificado: boolean // reservado
  email_verificado: boolean
  bilhete_identidade?: string
  bilhete_identidade_encarregado?: string
  genero: Genero
  data_nascimento: string         // 'YYYY-MM-DD'
  codigo_academia?: string
  status: StatusGeralEstudante    // 'ativo' | 'inativo' | 'pendente_documentos'
  status_escolar_fundamental: StatusEscolar
  status_escolar_medio: StatusEscolar
  status_superior: StatusEscolar
  ano_escolar_fundamental?: string // ex: '3_ano_fundamental'
  ano_escolar_medio?: string      // ex: '2_ano_medio'
  ano_superior?: string           // ex: '1_ano_superior'
  semestre_atual?: number         // apenas superior; inteiro sequencial (1..N)
  curso_medio_id?: string         // UUID
  curso_superior_id?: string      // UUID
  total_notas?: number
  total_faltas?: number
  documentos?: Record<string, SolicitacaoMatriculaDocumentoDTO>
  created_at: string
  updated_at: string
  version: number
}
```

**Valores de `EstudanteDTO.status` (`StatusGeralEstudante`):**

- `ativo` — estudante com vínculo geral ativo e documentação completa.
- `inativo` — estudante desvinculado/inativo no vínculo geral.
- `pendente_documentos` — estudante criado textualmente por fluxo em lote/fallback, mas ainda bloqueado até concluir o envio dos documentos obrigatórios.

---


### 2.5 SolicitacaoMatricula

```typescript
interface SolicitacaoMatriculaDocumentoDTO {
  path: string
  file_url: string
  // Rota autenticada do backend para download inline do PDF no escopo da consulta.
  download_url: string
}

interface SolicitacaoMatriculaDTO {
  id: string
  codigo_solicitacao: string
  codigo_academia: string
  nome: string
  genero: Genero
  data_nascimento: string
  email?: string
  telefone?: string
  telefone_encarregado?: string
  bilhete_identidade?: string
  bilhete_identidade_encarregado?: string
  ano_escolar_fundamental?: string
  ano_escolar_medio?: string
  curso_medio_id?: string
  ano_superior?: string
  curso_superior_id?: string
  status: SolicitacaoMatriculaStatus
  solicitacoes_semelhantes: string[] // calculado pelo backend; somente leitura
  motivo_reprovacao?: string
  documentos: Record<string, SolicitacaoMatriculaDocumentoDTO>
  codigo_estudante_gerado?: string
  valor_matricula?: number
  metodos_pagamento_matricula?: string[]
  aprovada_por?: string
  reprovada_por?: string
  created_at: string
  updated_at: string
  version: number
}
```

### 2.6 Curso

```typescript
interface CursoDTO {
  id: string
  nome: string
  type: CursoType            // preenchido automaticamente pelo backend e imutável
  modelo?: ModeloCursoMedio  // obrigatório e exposto apenas em cursos médios
  anos_academicos: string[]  // ex: ['1_ano_medio', '2_ano_medio', '3_ano_medio']
  periodos?: string[]        // ex: ['1_semestre', '2_semestre'] — apenas para superior
  codigo_academia: string
  status: string             // 'ativo' | 'inativo' | 'deletado'
  created_at: string
  updated_at: string
  version: number
}
```

---

### 2.7 Matéria

```typescript
interface MateriaDTO {
  id: string
  nome: string
  type: MateriaType          // preenchido automaticamente (exceto escola mista, que informa no create)
  anos_academicos?: string[]  // ex: ['2_ano_fundamental'] ou ['1_ano_medio', '2_ano_medio']; médio aceita múltiplos anos, exceto 4_ano_medio
  periodo?: string            // ex: '1_semestre' — obrigatório para superior
  pendencia_permitida: boolean // disponível apenas para superior; define se pode ficar pendente
  pendencia_nivel_conclusao?: string // ex: '2_semestre'; limite máximo com pendência superior
  codigo_academia: string
  curso_id?: string           // UUID — obrigatório para medio e superior
  status: string              // 'ativo' | 'inativo' | 'deletado'
  created_at: string
  updated_at: string
  version: number
}
```

---

### 2.8 Turma

```typescript
interface TurmaDTO {
  id: string
  codigo_turma: string
  codigo_academia: string
  nivel: string               // ex: '3_ano_fundamental'
  curso_id?: string           // UUID
  turno: Turno
  estudantes: string[]        // lista de codigo_estudante (estudante só pode estar em uma turma por vez)
  historico_estudantes_ano_letivo: Record<string, string[]> // ano_letivo -> estudantes que já passaram na turma
  status: string              // 'ativo' | 'inativo' | 'deletado'
  status_alterado_por?: string // UUID
  status_alterado_em?: string  // RFC3339
  created_at: string
  updated_at: string
  version: number
}
```

---

### 2.9 Nota

```typescript
interface NotaDTO {
  id: string
  codigo_estudante: string
  codigo_academia: string
  ano_lectivo: string         // ex: '2025_2026'
  ano_academico: string       // ex: '3_ano_fundamental'
  periodo: string             // ex: '1_trimestre' ou '1_semestre'
  materia_disciplinar_id: string // UUID
  materia_nome?: string
  tipo: TipoNota
  categoria: string           // código de categoria cadastrada pela academia
  nota: number                // >= 0
  observacao?: string
  registered_at: string
  event_id: string
  version: number
}
```

---

### 2.10 Categoria de Nota (ensino superior)

```typescript
interface CategoriaNotaDTO {
  id: string
  codigo_academia: string
  codigo: string
  nome: string
  descricao?: string
  anos_academicos: string[]
  adicionado_por?: string  // UUID
  created_at: string
  version: number
}
```

Usado em: `GET /academia/categorias-nota`. O `status` é interno da projeção e não é exposto no DTO público; a rota lista apenas categorias ativas.

---

### 2.11 Falta

```typescript
interface FaltaDTO {
  id: string
  codigo_estudante: string
  codigo_academia: string
  ano_lectivo: string
  ano_academico: string
  periodo: string              // 1_trimestre, 2_trimestre, 3_trimestre, 1_semestre ou 2_semestre
  data: date                  // date-only (ISO: YYYY-MM-DD)
  materia_disciplinar_id: string
  materia_nome?: string
  quantidade: number
  observacao?: string
  registered_at: string
  event_id: string
  version: number
}
```

---

### 2.12 Registro de Nota (consulta global)

```typescript
interface NotaRegistroDTO {
  id: string
  codigo_estudante: string
  estudante_nome: string
  codigo_academia: string
  academia_nome: string
  ano_lectivo: string
  ano_academico: string
  periodo: string
  materia_disciplinar_id: string
  materia_nome: string
  tipo: TipoNota
  categoria: string
  nota: number
  observacao?: string
  registered_at: string
  event_id: string
  version: number
}
```

Usado em: `GET /notas`

---

### 2.13 Registro de Falta (consulta global)

```typescript
interface FaltaRegistroDTO {
  id: string
  codigo_estudante: string
  estudante_nome: string
  codigo_academia: string
  academia_nome: string
  ano_lectivo: string
  ano_academico: string
  periodo: string              // 1_trimestre, 2_trimestre, 3_trimestre, 1_semestre ou 2_semestre
  data: date
  materia_disciplinar_id: string
  materia_nome: string
  quantidade: number
  observacao?: string
  registered_at: string
  event_id: string
  version: number
}
```

Usado em: `GET /faltas`

---

### 2.13 Avaliação Final

```typescript
interface AvaliacaoFinalDTO {
  id: string
  event_id: string
  codigo_estudante: string
  codigo_academia: string
  ano_lectivo: string
  tipo_ensino: TipoEnsino
  ano_academico_atual: string    // ex: '2_ano_fundamental'
  proximo_ano_academico?: string // ex: '3_ano_fundamental'
  aprovado: boolean
  observacao?: string
  registered_at: string
  version: number
}
```

---

---

### 2.15 Job

```typescript
interface JobSummary {
  id: string
  type: string
  status: JobStatus
  progress: number        // 0 a 100
  total_items: number
  done_items: number
  fail_items: number
  error?: string
  created_at: string
  started_at?: string
  completed_at?: string
}

interface JobDetail extends JobSummary {
  results: JobItemResult[]
}

interface JobItemResult {
  index: number
  sucesso: boolean
  dados?: any
  erro?: string
}
```

---

### 2.16 Resposta de Criação de Job Batch Assíncrono

```typescript
interface AsyncBatchAcceptedResponse {
  message: string
  job_id: string
  total_items: number
  status: JobStatus
  poll_url: string
}
```

---

## 3. Autenticação

### Segurança e Autenticação

### Consulta pública de academias

As rotas `GET /academias` e `GET /consultar-academia/:codigo` são públicas com autenticação opcional. Usuários não autenticados podem consultar a lista de academias ou uma academia específica pelo código, mas a resposta expõe somente os campos públicos: `nivel`, `type`, `nome`, `codigo_academia`, `provincia`, `endereco`, `nivel_escolar` e `anos_academicos`. Para escolas fundamentais ou mistas, `anos_academicos` informa os anos acadêmicos ofertados sem exigir sessão. Usuários autenticados recebem também `documentos.alvara.download_url` apontando para `/documentos/academias/{codigo_academia}/alvara/download`, permitindo download do alvará pelo backend sem expor links diretos do storage.

As rotas `GET /academia/cursos?codigo_academia=...` e `GET /academia/curso/:id` também são públicas com autenticação opcional para consulta dos cursos e dos anos desses cursos em escolas de nível `medio` ou `misto` e academias do nível superior. Academias autenticadas continuam consultando os próprios cursos sem informar `codigo_academia`; admins autenticados continuam informando `codigo_academia` na listagem.

Quando a requisição envia `Authorization: Bearer <jwt_token>` válido, a API preserva o contrato autenticado anterior, retornando também campos operacionais para usuários autenticados e campos administrativos adicionais para admins. Tokens enviados em formato inválido, expirados ou pertencentes a contas inativas devem ser rejeitados com `401`.

### 3.1 JWT

- Algoritmo: HS256
- Expiração configurável via `JWT_EXPIRY_HOURS` (padrão: 24h)
- Secret configurável via `JWT_SECRET` (obrigatório em produção)
- Payload: `user_id` (UUID) e `user_type` (string)

### 3.2 Senhas

- Algoritmo: bcrypt com custo padrão
- Senhas de admins: geradas com `crypto/rand` (segurança criptográfica), nunca hardcoded
- Senhas de academia/estudante: código da entidade como senha inicial
- Todas as alterações de senha passam pelo ledger (evento)

### 3.3 Hash Chain do Ledger

Cada evento no ledger tem um hash SHA256 que inclui o hash do evento anterior:

```
hash(evento_N) = SHA256(conteúdo_N + hash(evento_N-1))
```

Qualquer adulteração de um evento invalida toda a cadeia a partir daquele ponto, tornando a adulteração detectável.

### 3.4 Whitelist de Eventos

Apenas eventos previamente autorizados podem ser gravados no ledger (`safe_queries.go`). Qualquer evento desconhecido é rejeitado antes de chegar ao banco.

### 3.5 Segurança das Queries

- Todas as queries SQL usam prepared statements com placeholders (`$1`, `$2`, ...)
- Nomes de tabelas interpolados dinamicamente só ocorrem em um switch fechado com valores constantes (sem input do usuário)
- Inputs validados e sanitizados antes de qualquer operação

---


### POST /login

Login unificado para todos os tipos de usuário. O tipo é inferido automaticamente.

**Proteção**: pública | Rate limit ativo

**Request:**

```json
{
  "usuario": "string",  // email (admin), código ou email (academia/estudante)
  "senha": "string"
}
```

**Response 200:**

```json
{
  "token": "string",    // JWT
  "nome": "string",
  "type": "academia",   // 'admin' | 'academia' | 'estudante'
  "codigo": "LDA20261", // apenas para academia/estudante
  "email": "x@y.com",  // apenas para admin
  "role": "fpp"         // apenas para admin
}
```

**Erros:**

- `400` — campos obrigatórios ausentes
- `401` — credenciais inválidas ou conta inativa
- `401` — email não verificado (quando login é feito por email e não por código)

---

### POST /logout

Encerra a sessão do usuário autenticado no cliente.

> Observação: como a autenticação usa JWT stateless, este endpoint confirma o logout, mas a invalidação do token depende do cliente remover o token localmente.

**Proteção**: autenticado (qualquer tipo)

**Request:** sem payload

**Response 200:**

```json
{
  "message": "logout realizado com sucesso"
}
```

**Erros:**

- `401` — token ausente, inválido ou expirado

---

### POST /bootstrap

Cria o primeiro admin FPP do sistema. Bloqueado após o primeiro uso (retorna 403).

**Proteção**: pública | Advisory lock PostgreSQL (impede race condition)

**Request:**

```json
{
  "nome": "string",
  "email": "string",
  "telefone": "923456789",
  "senha": "string"
}
```

**Response 201:**

```json
{
  "success": true,
  "message": "Admin FPP criado com sucesso!",
  "data": {
    "id": "uuid",
    "nome": "string",
    "email": "string",
    "telefone": "923456789",
    "role": "fpp"
  },
  "next_steps": ["string"]
}
```

**Erros:**

- `400` — campos obrigatórios ausentes
- `403` — sistema já possui administradores
- `409` — email já cadastrado

---

---

## 4. Perfil e Conta

### GET /meu-perfil

Retorna os dados do usuário autenticado. O formato da resposta varia por tipo.

**Proteção**: autenticado (qualquer tipo)


**Request:** sem payload
**Response 200 — Estudante:**

```json
{
  "tipo": "estudante",
  "estudante": {
    "id": "uuid",
    "nome": "string",
    "codigo_estudante": "ABC1234",
    "email": "string",
    "telefone": "string",
    "email_verificado": false,
    "bilhete_identidade": "string",
    "bilhete_identidade_encarregado": "string",
    "genero": "masculino",
    "data_nascimento": "2000-03-15",
    "codigo_academia": "LDA20261",
    "academia_info": {
      "codigo": "LDA20261",
      "nome": "string",
      "nivel": "escola",
      "type": "public"
    },
    "status": "ativo",
    "status_escolar_fundamental": "em_andamento",
    "status_escolar_medio": "inativo",
    "status_superior": "inativo",
    "ano_escolar_fundamental": "3_ano_fundamental",
    "ano_escolar_medio": null,
    "ano_superior": null,
    "curso_medio": null,
    "curso_superior": null,
    "documentos": {
      "bi_estudante": {
        "path": "LDA20261/estudantes/ABC1234/documentos/bi_estudante_ABC1234.pdf",
        "file_url": "LDA20261/estudantes/ABC1234/documentos/bi_estudante_ABC1234.pdf",
        "download_url": "/documentos/estudantes/ABC1234/bi_estudante/download"
      }
    }
  }
}
```

**Response 200 — Academia:**

```json
{
  "tipo": "academia",
  "academia": {
    "id": "uuid",
    "nivel": "escola",
    "type": "public",
    "nome": "string",
    "codigo_academia": "LDA20261",
    "provincia": "LDA",
    "endereco": "string",
    "telefone": "string",
    "telefone_verificado": false,
    "email": "string",
    "nivel_escolar": "fundamental",
    "anos_academicos": ["1_ano_fundamental", "9_ano_fundamental"],
    "status": "ativo",
    "cursos": [],
    "email_verificado": true,
    "created_at": "2025-01-01T00:00:00Z",
    "total_estudantes": 120
  }
}
```

**Response 200 — Admin:**

```json
{
  "tipo": "admin",
  "admin": {
    "id": "uuid",
    "nome": "string",
    "email": "string",
    "role": "fpp",
    "status": "ativo",
    "email_verificado": true,
    "created_at": "2025-01-01T00:00:00Z"
  }
}
```

---

### PUT /alterar-senha

Altera a senha do usuário autenticado.

**Proteção**: autenticado (qualquer tipo)

**Request:**

```json
{
  "senha_atual": "string",
  "nova_senha": "string"   // mínimo 6 caracteres
}
```

**Response 200:**

```json
{
  "message": "Senha alterada com sucesso!"
}
```

**Erros:**

- `400` — campos obrigatórios ausentes ou nova senha muito curta
- `401` — senha atual incorreta

---

---

---

## 5. Email

Todos os endpoints de email têm rate limiting ativo.


### PUT /me/email

Atualiza o email do usuário autenticado (estudante, academia ou admin), identificado exclusivamente pelo token. Não envie identificadores ou tipo de usuário no body.

**Proteção**: autenticado.

**Request:**

```json
{
  "email": "novo.email@exemplo.com"
}
```

**Regras:** email válido e único no sistema. Se o valor mudar de fato, `email_verificado` volta para `false`; reenviar o mesmo email não altera a flag.

**Response 200:**

```json
{
  "message": "email atualizado com sucesso"
}
```

### PUT /me/telefone

Atualiza o telefone do usuário autenticado (estudante, academia ou admin), identificado exclusivamente pelo token.

**Proteção**: autenticado.

**Request:**

```json
{
  "telefone": "923456789"
}
```

**Regras:** telefone deve ser uma string com exatamente 9 dígitos do número nacional, sem DDI, sem `+`, sem espaços, sem hífens, sem parênteses e sem letras. Valores como `+244923456789`, `244923456789`, `923 456 789`, `923-456-789`, `(923)456789`, `923abc789` e número JSON sem aspas são rejeitados. Se o valor mudar de fato, `telefone_verificado` volta para `false`; reenviar o mesmo telefone não altera a flag.

**Response 200:**

```json
{
  "message": "telefone atualizado com sucesso"
}
```

### POST /email/verificar-email/:token

Verifica o email usando o token recebido no email. Funciona para admin, academia e estudante.

**Proteção**: pública

**Path Params:**

- `token` — token de verificação (hex de 64 caracteres)


**Request:** sem payload
**Response 200:**

```json
{
  "message": "Email verificado com sucesso!",
  "email": "usuario@exemplo.com"
}
```

**Erros:**

- `400` — token inválido, expirado ou já utilizado

---

### POST /email/verificar-email/solicitar

Envia email de verificação para o usuário autenticado (backend envia o email diretamente).

**Proteção**: autenticado (qualquer tipo)


**Request:** sem payload
**Response 200:**

```json
{
  "success": true,
  "message": "Email de verificação enviado com sucesso!",
  "email": "usuario@exemplo.com"
}
```

---

### POST /email/gerar-token/verificacao

Gera e retorna o token de verificação ao frontend, que fica encarregado por enviar o email.

**Proteção**: autenticado (qualquer tipo)


**Request:** sem payload
**Response 200:**

```json
{
  "success": true,
  "token": "hex64chars",
  "email": "usuario@exemplo.com",
  "nome": "string",
  "tipo": "academia",
  "expira_em": "24 horas"
}
```

---

### POST /email/gerar-token/recuperacao

Gera token de recuperação de senha e retorna ao frontend. O frontend envia o email.

**Proteção**: pública

**Request:**

```json
{
  "identificador": "string",  // código, email ou código da academia
  "tipo": "estudante"         // 'estudante' | 'academia' | 'admin'
}
```

**Response 200:**

```json
{
  "success": true,
  "token": "hex64chars",
  "email": "usuario@exemplo.com",
  "nome": "string",
  "tipo": "academia",
  "expira_em": "1 hora"
}
```

**Erros:**

- `400` — campos obrigatórios ausentes
- `403` — email não verificado (deve verificar antes de recuperar senha)
- `404` — usuário não encontrado

---

### POST /email/recuperar-senha/solicitar

Solicita recuperação de senha. O backend envia o email diretamente.

**Proteção**: pública

**Request:**

```json
{
  "identificador": "string",
  "tipo": "estudante"
}
```

**Response 200:**

```json
{
  "success": true,
  "message": "Email de recuperação enviado com sucesso. Verifique sua caixa de entrada.",
  "expira_em": "1 hora"
}
```

---

### POST /email/recuperar-senha/:token

Define nova senha usando o token de recuperação.

**Proteção**: pública

**Path Params:**

- `token` — token de recuperação

**Request:**

```json
{
  "nova_senha": "string"  // mínimo 6 caracteres
}
```

**Response 200:**

```json
{
  "message": "Senha redefinida com sucesso!",
  "email": "usuario@exemplo.com"
}
```

**Erros:**

- `400` — token inválido/expirado ou senha muito curta
- `403` — email não verificado

---

---

## 6. Academias

### Processos de Negócio — Cadastro de Academia

### 6.1 Cadastro de Academia

**Quem faz**: Admin (FPP)

1. Admin envia dados da academia (nivel, type, nome, província, endereço, nível escolar, etc.)
2. Sistema gera o código único consultando o ledger (ex: `LDA20261`)
3. Sistema gera a senha padrão = código da academia (ex: `LDA20261`)
4. Academia é criada com **status `inativo`**
5. Um admin com role `adm` ou `fpp` deve ativar manualmente

**Regras de validação:**

- `nivel` deve ser `escola` ou `superior`
- `type` é obrigatório (campo textual) e deve ser `public` ou `private`
- Para `nivel=escola` com nível escolar `fundamental` ou `misto`: `anos_academicos` é obrigatório (formato `[1-9]_ano_fundamental`)
- Para `nivel=escola` com nível escolar `medio`: `anos_academicos` não deve ser informado
- Província deve ser um código válido de Angola (21 províncias):
	- `{ nome: 'BENGO', codigo: 'BGO' },
	  { nome: 'BENGUELA', codigo: 'BGU' },
	  { nome: 'BIE', codigo: 'BIE' },
	  { nome: 'CABINDA', codigo: 'CAB' },
	  { nome: 'CUANDO', codigo: 'CND' },
	  { nome: 'CUANZA NORTE', codigo: 'CNO' },
	  { nome: 'CUANZA SUL', codigo: 'CUS' },
	  { nome: 'CUBANGO', codigo: 'CBG' },
	  { nome: 'CUNENE', codigo: 'CNN' },
	  { nome: 'HUAMBO', codigo: 'HUA' },
	  { nome: 'HUILA', codigo: 'HUI' },
	  { nome: 'ICOLO E BENGO',  codigo: 'IBG' },
	  { nome: 'LUANDA', codigo: 'LUA' },
	  { nome: 'LUNDA NORTE', codigo: 'LNO' },
	  { nome: 'LUNDA SUL', codigo: 'LSU' },
	  { nome: 'MALANJE', codigo: 'MAL' },
	  { nome: 'MOXICO', codigo: 'MOX' },
	  { nome: 'MOXICO LESTE', codigo: 'MXL' },
	  { nome: 'NAMIBE', codigo: 'NAM' },
	  { nome: 'UIGE', codigo: 'UIG' },
	  { nome: 'ZAIRE', codigo: 'ZAI' },
	];`
### Regras de Negócio — Academia

### 6.2 Regras de Academia

| Regra                                                         | Detalhe                                                |
| ------------------------------------------------------------- | ------------------------------------------------------ |
| Academias nascem inativas                                     | Apenas um admin pode ativar                            |
| Escola com nível fundamental/misto deve ter `anos_academicos` | Sem anos, o cadastro é rejeitado                       |
| Escola com nível médio não deve ter `anos_academicos`         | Anos são do curso, não da academia                     |
| Apenas academias ativas podem operar                          | Middleware valida status em cada request               |
| Admin FPP define diretamente, por tipo, o ano letivo oficial global via `POST /admin/definir-ano-letivo-geral`; depois é atualizado automaticamente quando todas as academias ativas do mesmo tipo estiverem alinhadas | É a referência obrigatória para todo o sistema          |
| Academia só pode definir diretamente uma vez e sempre igual ao oficial global; avanços acontecem pela finalização do ano letivo | Divergência é bloqueada com erro de negócio             |
| Sem ano letivo na academia, notas/faltas/avaliações são bloqueadas | Pré-condição para registos académicos               |
| Histórico `anos_letivos_lista` não duplica `ano_letivo`       | Atualizações repetidas do mesmo ano são ignoradas       |
| Histórico global `anos_letivos_lista` (admin) não duplica `ano_letivo` | Lista global de anos letivos mantém unicidade por ano |
| Senha padrão = código da academia                             | Deve ser alterada após o primeiro login                |
| Desativação exige motivo                                      | Registado no ledger e na projeção para auditoria       |


### POST /dominis/academia/register

Registra uma nova academia via `multipart/form-data`. Criada com status `inativo`. `nif` é obrigatório, string única de exatamente 10 dígitos, inclusive para academias inativas. `alvara` é arquivo obrigatório, deve ser PDF válido com até 10MB e é armazenado em `{codigo_academia}/Documentação formal/`. O front end pode ler esse documento pela rota autenticada `GET /documentos/academias/{codigo_academia}/alvara/download`.

**Proteção**: autenticado + admin (qualquer role)

**Request — Escola:**

```json
{
  "nivel": "escola",
  "type": "public",
  "nome": "Escola Primária Ngola Kiluanje",
  "nif": "0012345678",
  "alvara": "@./alvara.pdf;type=application/pdf",
  "provincia": "luanda",
  "endereco": "Rua Direita, 123",
  "telefone": "+244923000000",
  "email": "escola@exemplo.ao",
  "website": "https://escola.ao",
  "nivel_escolar": "fundamental",
  "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental", "9_ano_fundamental"],
  "cursos": []
}
```

**Request — Universidade:**

```json
{
  "nivel": "superior",
  "type": "private",
  "nome": "Universidade Agostinho Neto",
  "nif": "0098765432",
  "alvara": "@./alvara.pdf;type=application/pdf",
  "provincia": "luanda",
  "endereco": "Av. 4 de Fevereiro",
  "email": "uan@uan.ao"
}
```

**Response 201:**

```json
{
  "message": "academia registada com sucesso",
  "codigo_academia": "LDA20261",
  "data": {
    "id": "uuid",
    "nome": "string",
    "nif": "0012345678",
    "provincia": "LDA",
    "codigo_academia": "LDA20261"
  }
}
```

**Erros:**

- `400` — `nivel` inválido, `type` inválido (`public`/`private`) ou ausente, `nif` ausente/inválido, `alvara` ausente/não PDF/acima de 10MB, campos obrigatórios ausentes ou anos_academicos inválidos
- `409` — academia ou `nif` já existe

---

### POST /academia/registo-publico

Permite que uma academia se autocadastre na plataforma **sem autenticação prévia**, via `multipart/form-data`. Usa exatamente as mesmas regras de validação de `POST /dominis/academia/register` (`nif` obrigatório, único, 10 dígitos; `alvara` obrigatório, PDF válido até 10MB, armazenado em `{codigo_academia}/Documentação formal/`). A academia é sempre criada com status `inativo` — apenas um admin com role `adm` ou `fpp` pode ativá-la, via `PUT /dominis/academia/:codigo/ativar`. Login antes da ativação retorna erro de "academia inativa".

**Proteção**: nenhuma (rota pública)

**Diferença em relação ao cadastro por admin**: exige o campo `senha` (string, 6–128 caracteres). Essa senha é definida como a senha de acesso da academia. Diferentemente do fluxo administrativo, este endpoint público não usa fallback para a senha padrão baseada no `codigo_academia`.

**Request:**

```json
{
  "nivel": "escola",
  "type": "public",
  "nome": "Escola Primária Ngola Kiluanje",
  "nif": "0012345678",
  "alvara": "@./alvara.pdf;type=application/pdf",
  "provincia": "luanda",
  "endereco": "Rua Direita, 123",
  "telefone": "+244923000000",
  "email": "escola@exemplo.ao",
  "website": "https://escola.ao",
  "nivel_escolar": "fundamental",
  "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental", "9_ano_fundamental"],
  "cursos": [],
  "senha": "minhaSenhaSegura123"
}
```

**Response 201:**

```json
{
  "message": "cadastro recebido com sucesso. a conta fica inativa até que um administrador (role adm ou fpp) a ative.",
  "codigo_academia": "LDA20261",
  "data": {
    "id": "uuid",
    "nome": "string",
    "nif": "0012345678",
    "type": "public",
    "provincia": "LDA",
    "codigo_academia": "LDA20261",
    "status": "inativo"
  },
  "aviso": "guarde o código da academia: ele é o seu identificador de login. você definiu sua própria senha no cadastro."
}
```

**Erros:**

- `400` — `nivel` inválido, `type` inválido, `nif` ausente/inválido, `alvara` ausente/não PDF/acima de 10MB, campos obrigatórios ausentes, `anos_academicos` inválidos, `senha` ausente/vazia ou fora do intervalo de 6–128 caracteres
- `409` — `nif` já cadastrado em outra academia

---

### PUT /dominis/academia/:codigo/ativar

Ativa uma academia inativa.

**Proteção**: autenticado + admin role `adm` ou `fpp`

**Path Params:**

- `codigo` — código da academia (ex: `LDA20261`)


**Request:** sem payload
**Response 200:**

```json
{
  "message": "academia ativada com sucesso"
}
```

**Erros:**

- `404` — academia não encontrada
- `409` — academia já está ativa

---

### PUT /dominis/academia/:codigo/desativar

Desativa uma academia ativa.

**Proteção**: autenticado + admin role `adm` ou `fpp`

**Path Params:**

- `codigo` — código da academia

**Request:**

```json
{
  "motivo": "string"  // obrigatório
}
```

**Response 200:**

```json
{
  "message": "academia desativada com sucesso"
}
```

**Erros:**

- `400` — motivo ausente
- `404` — academia não encontrada
- `409` — academia já está inativa

---

### PUT /academia/dados

Atualiza os dados cadastrais da academia autenticada.

**Proteção**: autenticado + academia ativa

**Request:** (todos os campos opcionais, enviar apenas o que deseja alterar)

```json
{
  "nome": "string",
  "provincia": "luanda",
  "endereco": "string",
  "website": "string"
}
```

**Response 200:**

```json
{
  "message": "dados atualizados com sucesso"
}
```

**Nota**: `telefone`, `email`, `anos_academicos`, `cursos`, `type`, `nivel_escolar` e `nif` não são aceitos nesta rota. Use `PUT /me/email` e `PUT /me/telefone` para contatos, `POST/DELETE /academia/anos-academicos` para anos acadêmicos e as rotas `/academia/curso` para cursos. Alterações de `type` e `nivel_escolar` exigem documento comprobativo pelo fluxo dedicado da tarefa 07 e ficam indisponíveis por este caminho. Se qualquer campo não permitido aparecer no payload, a requisição falha inteira com `400` e nenhum campo é alterado.

---

### Solicitações de edição de dados sensíveis de estudantes

Subsecção de operações da academia para consultar, aprovar ou reprovar solicitações documentadas feitas por estudantes vinculados. Cada rota de decisão é específica para um campo, valida o campo da URL contra a solicitação e não existe endpoint genérico com `campo` arbitrário.

#### GET /academia/solicitacoes-edicao-estudante

Lista solicitações documentadas de edição de dados sensíveis dos estudantes vinculados à academia autenticada.

Cada item retornado inclui `documento`, no mesmo formato dos documentos de matrícula, com `path`, `file_url` e `download_url` para que o cliente possa abrir/baixar o PDF pelo backend. No escopo da academia, o `download_url` aponta para `/academia/documentos/solicitacoes-edicao-estudante/{codigo_solicitacao}/documento/download`.

**Proteção**: autenticado + academia ativa

**Query Params:**

- `status` — filtro opcional por `pendente`, `aprovada` ou `reprovada`
- `campo` — filtro opcional por `nome`, `bilhete_identidade`, `bilhete_identidade_encarregado` ou `data_nascimento`
- `codigo_estudante` — restringe a listagem a um estudante da própria academia
- `limit` — quantidade máxima por página (padrão 50, teto 100)
- `offset` — deslocamento da paginação (padrão 0)

**Request:** sem payload

**Response 200:**

```json
{
  "solicitacoes": [
    {
      "codigo_solicitacao": "SED12345678",
      "codigo_estudante": "EST12345678",
      "codigo_academia": "ACA12345678",
      "campo": "nome",
      "valor_atual": "Nome Atual",
      "valor_solicitado": "Nome Corrigido",
      "documento_temporario_path": "ACA12345678/estudantes/EST12345678/edicoes_dados_pendentes/nome_SED12345678.pdf",
      "documento_temporario_url": "string",
      "documento": {
        "tipo": "documento",
        "path": "ACA12345678/estudantes/EST12345678/edicoes_dados_pendentes/nome_SED12345678.pdf",
        "file_url": "string",
        "download_url": "/academia/documentos/solicitacoes-edicao-estudante/SED12345678/documento/download"
      },
      "status": "pendente",
      "motivo_reprovacao": null,
      "solicitado_por": "EST12345678",
      "decidido_por": null,
      "created_at": "2026-07-24T00:00:00Z",
      "updated_at": "2026-07-24T00:00:00Z",
      "version": 1
    }
  ],
  "limit": 50,
  "offset": 0,
  "total": 1
}
```

**Regras de negócio:** a academia só enxerga solicitações de estudantes vinculados a ela; admin não decide solicitações por estas rotas.

---

#### PUT /academia/solicitacoes-edicao-estudante/nome/:codigo/aprovar

Aprova uma solicitação pendente de alteração de `nome`.

**Proteção**: autenticado + academia ativa

**Path Params:**

- `codigo` — `codigo_solicitacao` da solicitação de edição de nome

**Request:** sem payload

**Response 200:**

```json
{
  "message": "solicitação decidida com sucesso",
  "codigo_solicitacao": "SED12345678",
  "status": "aprovada"
}
```

**Regras de negócio:** a solicitação precisa pertencer à academia autenticada, estar `pendente` e ter `campo = nome`. O valor solicitado é revalidado contra o estado atual antes da alteração. A aprovação grava `NomeEstudanteAlteradoPorSolicitacao`, marca a solicitação como `aprovada` e remove o PDF temporário.

**Erros:** `400` campo da rota incompatível ou valor inválido; `403` academia alheia; `404` solicitação inexistente; `409` solicitação já decidida.

---

#### PUT /academia/solicitacoes-edicao-estudante/nome/:codigo/reprovar

Reprova uma solicitação pendente de alteração de `nome`.

**Proteção**: autenticado + academia ativa

**Path Params:**

- `codigo` — `codigo_solicitacao` da solicitação de edição de nome

**Request:**

```json
{
  "motivo_reprovacao": "Documento não comprova a alteração solicitada"
}
```

**Response 200:** igual ao endpoint de aprovação, com `status = "reprovada"`.

**Regras de negócio:** exige `motivo_reprovacao` não vazio, preserva o nome vigente, grava `SolicitacaoEdicaoDadoEstudanteReprovada` e remove o PDF temporário.

---

#### PUT /academia/solicitacoes-edicao-estudante/bilhete-identidade/:codigo/aprovar

Aprova uma solicitação pendente de alteração de `bilhete_identidade`.

**Proteção**: autenticado + academia ativa

**Path Params:** `codigo` — `codigo_solicitacao` da solicitação de BI do estudante.

**Request:** sem payload

**Response 200:** igual ao endpoint de aprovação, com `status = "aprovada"`.

**Regras de negócio:** exige solicitação `pendente`, da própria academia e com `campo = bilhete_identidade`; revalida formato e duplicidade atual; grava `BilheteIdentidadeEstudanteAlteradoPorSolicitacao`; remove o PDF temporário.

---

#### PUT /academia/solicitacoes-edicao-estudante/bilhete-identidade/:codigo/reprovar

Reprova uma solicitação pendente de alteração de `bilhete_identidade`.

**Proteção**: autenticado + academia ativa

**Path Params:** `codigo` — `codigo_solicitacao` da solicitação de BI do estudante.

**Request:**

```json
{
  "motivo_reprovacao": "BI informado não confere com o PDF"
}
```

**Response 200:** igual ao endpoint de aprovação, com `status = "reprovada"`.

**Regras de negócio:** preserva o BI vigente, grava a reprovação terminal e remove o PDF temporário; decisões repetidas retornam `409`.

---

#### PUT /academia/solicitacoes-edicao-estudante/bilhete-identidade-encarregado/:codigo/aprovar

Aprova uma solicitação pendente de alteração de `bilhete_identidade_encarregado`.

**Proteção**: autenticado + academia ativa

**Path Params:** `codigo` — `codigo_solicitacao` da solicitação de BI do encarregado.

**Request:** sem payload

**Response 200:** igual ao endpoint de aprovação, com `status = "aprovada"`.

**Regras de negócio:** exige solicitação `pendente`, da própria academia e com `campo = bilhete_identidade_encarregado`; revalida o valor solicitado; grava `BilheteIdentidadeEncarregadoAlteradoPorSolicitacao`; remove o PDF temporário.

---

#### PUT /academia/solicitacoes-edicao-estudante/bilhete-identidade-encarregado/:codigo/reprovar

Reprova uma solicitação pendente de alteração de `bilhete_identidade_encarregado`.

**Proteção**: autenticado + academia ativa

**Path Params:** `codigo` — `codigo_solicitacao` da solicitação de BI do encarregado.

**Request:**

```json
{
  "motivo_reprovacao": "Documento insuficiente para comprovar o BI do encarregado"
}
```

**Response 200:** igual ao endpoint de aprovação, com `status = "reprovada"`.

**Regras de negócio:** preserva o BI vigente do encarregado, grava a reprovação terminal e remove o PDF temporário; decisões repetidas retornam `409`.

---

#### PUT /academia/solicitacoes-edicao-estudante/data-nascimento/:codigo/aprovar

Aprova uma solicitação pendente de alteração de `data_nascimento`.

**Proteção**: autenticado + academia ativa

**Path Params:** `codigo` — `codigo_solicitacao` da solicitação de data de nascimento.

**Request:** sem payload

**Response 200:** igual ao endpoint de aprovação, com `status = "aprovada"`.

**Regras de negócio:** exige solicitação `pendente`, da própria academia e com `campo = data_nascimento`; revalida formato, idade e coerência temporal; grava `DataNascimentoEstudanteAlteradaPorSolicitacao`; remove o PDF temporário.

---

#### PUT /academia/solicitacoes-edicao-estudante/data-nascimento/:codigo/reprovar

Reprova uma solicitação pendente de alteração de `data_nascimento`.

**Proteção**: autenticado + academia ativa

**Path Params:** `codigo` — `codigo_solicitacao` da solicitação de data de nascimento.

**Request:**

```json
{
  "motivo_reprovacao": "Certidão anexada não corresponde ao estudante"
}
```

**Response 200:** igual ao endpoint de aprovação, com `status = "reprovada"`.

**Regras de negócio:** preserva a data vigente, grava a reprovação terminal e remove o PDF temporário; decisões repetidas retornam `409`. Falhas de remoção do PDF temporário são registradas como log operacional sem desfazer a decisão já gravada.

---

### Regras automáticas de documentos de matrícula

A obrigatoriedade dos documentos e as validações cadastrais comuns são aplicadas por uma política única compartilhada por `POST /solicitacao-matricula`, pela aprovação da solicitação e pelo cadastro direto `POST /academia/estudante/register`. As duas rotas normalizam os mesmos campos de estudante, encarregado, nível de ensino, telefones, bilhetes e documentos antes de qualquer gravação no ledger. A validação considera simultaneamente os campos textuais do request e os PDFs anexados:

- `1_ano_fundamental` não exige `declaracao` nem certificado acadêmico anterior.
- Todo ano escolar sequencial com ano anterior exige `declaracao` acompanhada do campo textual `declaracao_ano_academico`, e esse valor deve ser exatamente o ano acadêmico imediatamente anterior ao ano pretendido.
- `7_ano_fundamental` exige `certificado_6_ano_fundamental` ou `declaracao` com `declaracao_ano_academico=6_ano_fundamental`.
- `1_ano_medio` exige `certificado_9_ano_fundamental` ou `declaracao` com `declaracao_ano_academico=9_ano_fundamental`.
- `1_ano_superior` exige `certificado_ensino_medio` ou `declaracao` com `declaracao_ano_academico=3_ano_medio`.
- Declaração sem `declaracao_ano_academico`, do mesmo ano, de ano posterior ou de ano anterior não imediato é rejeitada quando ela é necessária para cumprir a regra acadêmica.
- Na persistência, a declaração deixa de ser documento acadêmico genérico: uploads e documentos informados no corpo JSON são normalizados para `tipo=declaracao_<ano_academico>` e chave `nivel.ano_academico.declaracao_<ano_academico>`, por exemplo `medio.3_ano_medio.declaracao_3_ano_medio`.
- Quando uma declaração opcional é enviada sem ano acadêmico e não participa da validação obrigatória, ela não é gravada como chave raiz `declaracao`; o backend isola o registro em `escopo_desconhecido.declaracao` até que o escopo possa ser corrigido.
- No ensino superior, `bilhete_identidade` do estudante e PDF `bi_estudante` são obrigatórios; `bilhete_identidade_encarregado` e PDF `bi_encarregado` são opcionais.
- No nível escolar/fundamental/médio, `bilhete_identidade_encarregado` e PDF `bi_encarregado` são sempre obrigatórios.
- No nível escolar/fundamental/médio, o estudante deve ter `bilhete_identidade` + PDF `bi_estudante`, ou PDF `cedula_estudante` quando não tiver BI próprio.
- `bilhete_identidade` e `bilhete_identidade_encarregado`, quando ambos informados para o mesmo estudante, não podem ser iguais.
- Para estudantes escolares/fundamental/médio, o BI do encarregado não pode coincidir com o BI principal de outro estudante escolar/fundamental/médio; ele pode repetir como BI de encarregado de outros estudantes.

**Telefone por nível de ensino:**

- No nível escolar/fundamental/médio, `telefone_encarregado` é obrigatório; `telefone` do estudante é opcional e não substitui o telefone do encarregado.
- No ensino superior, `telefone` do estudante é obrigatório; `telefone_encarregado` é opcional.
- Quando `telefone` e `telefone_encarregado` forem enviados, ambos devem ter formato válido de 9 dígitos locais e não podem ser iguais.

**Ordem operacional e atomicidade documental:**

- As duas rotas validam dados cadastrais comuns, regras de telefone por nível, presença documental, tipo, extensão, assinatura PDF e tamanho máximo antes de gravar eventos no ledger.
- Os uploads obrigatórios são concluídos antes da criação de `SolicitacaoMatriculaCriada` ou `EstudanteCriadoComVinculo`.
- Se validação ou upload falhar, nenhuma solicitação, estudante, matrícula, vínculo ou evento correlato é gravado no ledger.
- Se uma falha posterior ocorrer depois de upload parcial, o backend tenta remover o diretório de destino no storage e retorna o erro principal ao cliente.

---

### GET /academias

Lista todas as academias com paginação e filtro de status.

**Proteção**: pública com autenticação opcional.

- Sem `Authorization`, a rota retorna apenas dados públicos de cada academia.
- Com `Authorization: Bearer <jwt_token>` válido, a rota mantém o contrato autenticado anterior.
- Se um header `Authorization` for enviado, ele deve ser um Bearer token válido; tokens inválidos/expirados retornam `401`.

**Query Params:**

- `limit` — quantidade máxima por página (padrão sem `limit`: 50, teto fixo: 100)
- `offset` — deslocamento (padrão: 0)
- `status` — `ativo` ou `inativo` (omitir = retorna ambos)


**Request:** sem payload
**Response 200 — usuário não autenticado:**

```json
{
  "academias": [
    {
      "nivel": "escola",
      "type": "public",
      "nome": "Escola Exemplo",
      "codigo_academia": "LUA20261",
      "provincia": "Luanda",
      "endereco": "Rua Exemplo, 123",
      "nivel_escolar": "fundamental",
      "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental"]
    }
  ],
  "total": 1,
  "total_geral": 25,
  "limit": 50,
  "offset": 0
}
```

**Campos públicos por academia:** `nivel`, `type`, `nome`, `codigo_academia`, `provincia`, `endereco`, `nivel_escolar`, `anos_academicos`. Para escolas fundamentais ou mistas, `anos_academicos` permite que usuários sem sessão recebam os anos acadêmicos ofertados.

**Response 200 — usuário autenticado:**

```json
{
  "academias": [
    {
      ... (AcademiaDTO) ...,
      "documentos": {
        "alvara": {
          "path": "LDA20261/Documentação formal/alvara_LDA20261.pdf",
          "file_url": "LDA20261/Documentação formal/alvara_LDA20261.pdf",
          "download_url": "/documentos/academias/LDA20261/alvara/download"
        }
      }
    }
  ],
  "total": 25,
  "total_geral": 25,
  "limit": 50,
  "offset": 0
}
```

**Nota**: usuários autenticados veem os campos operacionais do `AcademiaDTO`, incluindo `documentos.alvara.download_url`; admins veem campos extras (`email`, `total_estudantes`, `version`). O backend nunca retorna mais de 100 academias por página, mesmo que o cliente envie `limit` maior. Em listagens paginadas, `total` indica a quantidade de itens retornados na página atual, enquanto `total_geral` indica a quantidade total de itens no escopo da consulta depois dos filtros e antes de aplicar `limit`/`offset`.

---

### GET /consultar-academia/:codigo

Retorna detalhes de uma academia pelo código.

**Proteção**: pública com autenticação opcional.

- Sem `Authorization`, a rota retorna somente os mesmos campos públicos usados em `GET /academias`.
- Com `Authorization: Bearer <jwt_token>` válido, a rota mantém o contrato autenticado anterior.
- Se um header `Authorization` for enviado, ele deve ser um Bearer token válido; tokens inválidos/expirados retornam `401`.


**Request:** sem payload
**Response 200 — usuário não autenticado:**

```json
{
  "nivel": "escola",
  "type": "public",
  "nome": "Escola Exemplo",
  "codigo_academia": "LDA20261",
  "provincia": "LDA",
  "endereco": "Rua Exemplo, 123",
  "nivel_escolar": "fundamental",
  "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental"]
}
```

**Response 200 — usuário autenticado:**

```json
{
  "id": "uuid",
  "nivel": "escola",
  "type": "public",
  "nome": "string",
  "codigo_academia": "LDA20261",
  "provincia": "LDA",
  "endereco": "string",
  "telefone": "+244900000000",
  "telefone_verificado": false,
  "website": "https://exemplo.ao",
  "nivel_escolar": "fundamental",
  "anos_academicos": ["1_ano_fundamental"],
  "status": "ativo",
  "cursos": [],
  "email_verificado": true,
  "created_at": "2026-06-13T00:00:00Z",
  "total_estudantes": 10,
  "ano_letivo": "2026",
  "tipo_ano_letivo": "anual",
  "anos_letivos_lista": ["2026"],
  "documentos": {
    "alvara": {
      "path": "LDA20261/Documentação formal/alvara_LDA20261.pdf",
      "file_url": "LDA20261/Documentação formal/alvara_LDA20261.pdf",
      "download_url": "/documentos/academias/LDA20261/alvara/download"
    }
  }
}
```

**Campos públicos para usuário não autenticado:** `nivel`, `type`, `nome`, `codigo_academia`, `provincia`, `endereco`, `nivel_escolar`, `anos_academicos`.

**Nota**: usuários autenticados veem também `documentos.alvara.download_url`; admins veem também `email` e `motivo_desativacao`.


### 6.3 Categorias de Nota

As rotas desta secção gerem as categorias de nota usadas no lançamento de notas. Academias superiores usam categorias configuráveis para lançar notas, compor fórmulas de avaliação final e validar `nota_despertadora` em regras raízes. Escolas (`nivel="escola"`) usam categorias fixas do sistema, por ano acadêmico, e podem consultá-las por `GET /academia/categorias-nota`, mas não podem configurá-las por endpoint.

**Escopo funcional da secção:**

- Categoria de nota configurável pertence sempre a uma única academia superior; categoria escolar fixa pertence ao modelo do sistema e é exposta no escopo da escola consultada.
- O identificador público da categoria é `codigo`; ele é o valor usado no campo `categoria` de notas e nas fórmulas/regras de avaliação final.
- Categorias removidas deixam de aparecer nas consultas e deixam de poder ser usadas em novos lançamentos/configurações.
- Cada rota abaixo possui escopo próprio de request, response, autorização, regras de negócio e erros.

---

#### GET /academia/categorias-nota

Lista as categorias de nota da academia resolvida: configuráveis no ensino superior e fixas do sistema em escolas.

**Proteção:** autenticado.

**Escopo da rota:** leitura das categorias disponíveis para lançamento de notas na academia resolvida. Para `nivel="superior"`, lê categorias ativas configuradas pela academia. Para `nivel="escola"`, lê categorias fixas do sistema aplicáveis aos anos acadêmicos/cursos ativos da escola. Não cria, altera ou remove categorias.

**Autorização por tipo de usuário:**

- **Academia autenticada:** consulta automaticamente as categorias da própria academia, desde que esteja ativa. Se for escola, recebe categorias fixas; se for superior, recebe categorias configuráveis ativas.
- **Estudante autenticado:** consulta as categorias da academia à qual pertence ou já pertenceu. Para consultar uma academia diferente da atual, deve informar `codigo_academia`; o backend autoriza a consulta se existir vínculo atual na projeção ou vínculo histórico no ledger de eventos do estudante.
- **Admin autenticado:** deve informar `codigo_academia` na query string para escolher a academia consultada.

**Request:** sem payload.

**Query params:**

| Campo | Obrigatório | Exemplo | Descrição |
| --- | --- | --- | --- |
| `codigo_academia` | Sim para admin; opcional para estudante; não usado para academia | `GET /academia/categorias-nota?codigo_academia=ACA-001` | Código público da academia consultada. Para estudante, omitir usa a academia atual; informar permite consultar uma academia à qual já esteve vinculado. |

**Response 200:**

```json
{
  "categorias": [
    {
      "id": "uuid",
      "codigo_academia": "ACA-001",
      "codigo": "prova_parcelar_1",
      "nome": "Prova Parcelar 1",
      "descricao": "Primeira prova parcelar",
      "anos_academicos": ["1_ano_superior", "2_ano_superior"],
      "adicionado_por": "uuid",
      "created_at": "2026-07-21T00:00:00Z",
      "version": 1
    }
  ],
  "total": 1
}
```

Para escolas, a resposta usa o mesmo envelope e retorna categorias fixas com metadados de leitura:

```json
{
  "categorias": [
    {
      "codigo_academia": "ESC-001",
      "codigo": "nota_professor",
      "nome": "Nota do professor/Avaliação contínua",
      "anos_academicos": ["1_ano_fundamental"],
      "source": "system",
      "fixed": true,
      "readonly": true,
      "status": "ativo"
    }
  ],
  "total": 1
}
```

**Regras de negócio:**

- Para academias superiores, retorna apenas categorias configuráveis ativas da academia resolvida.
- Para escolas, retorna categorias fixas do sistema aplicáveis aos anos acadêmicos cadastrados na escola e aos cursos médios ativos quando houver diferenciação por modelo.
- A resposta de categorias configuráveis não expõe `status`; a filtragem por ativo/inativo é interna da projeção.
- Para estudante, a consulta pode usar vínculo atual ou vínculo histórico auditado no ledger.
- Escolas não criam, editam nem removem categorias por endpoint; a rota GET é somente leitura.

**Erros comuns:**
- `400` se o admin não informar `codigo_academia`.
- `400` se o estudante omitir `codigo_academia` e não tiver academia atual associada.
- `403` se o estudante informar uma academia à qual nunca esteve vinculado.
- `403` se a academia autenticada estiver inativa.
- `404` se a academia ou estudante necessários para resolver o escopo não existirem.

---

#### POST /academia/categorias-nota

Cria uma categoria de nota configurável para a academia superior autenticada.

**Proteção:** autenticado + academia ativa.

**Escopo da rota:** escrita síncrona de uma categoria de nota em uma academia superior. A rota é exclusiva para o usuário da própria academia; admin e estudante não criam categorias por este endpoint.

**Request body:**

```json
{
  "codigo": "prova_parcelar_1",
  "nome": "Prova Parcelar 1",
  "descricao": "Primeira prova parcelar do semestre",
  "anos_academicos": ["1_ano_superior", "2_ano_superior"]
}
```

**Campos do request:**

| Campo | Obrigatório | Tipo | Descrição |
| --- | --- | --- | --- |
| `codigo` | Sim | string | Código público da categoria. É normalizado para minúsculas; espaços viram `_`; são aceitos apenas letras minúsculas, números, espaços e `_`. |
| `nome` | Sim | string | Nome/rótulo exibível da categoria. Não pode ser vazio após trim. |
| `descricao` | Não | string | Descrição operacional da categoria. |
| `anos_academicos` | Sim | string[] | Lista de anos acadêmicos nos quais a categoria pode ser usada. Não pode ser vazia nem conter valores vazios. |

**Response 201:**

```json
{
  "message": "categoria criada com sucesso",
  "categoria": "prova_parcelar_1"
}
```

**Regras de negócio:**

- Apenas academias com `nivel="superior"` podem criar categorias configuráveis.
- Escolas usam categorias fixas do modelo avaliativo do sistema e recebem erro de validação nesta rota.
- O `codigo` deve ser único entre as categorias ativas da academia; duplicidade é verificada no estado do aggregate e na projeção.
- `anos_academicos` é obrigatório para limitar onde a categoria pode ser usada em lançamentos de nota e regras de avaliação final.
- A criação emite evento `CategoriaNotaAdicionada` e a projeção `categorias_nota` passa a disponibilizar a categoria após processamento do evento.
- O usuário autenticado da academia é registrado como `adicionado_por` para auditoria.

**Erros comuns:**

- `400` se faltar `codigo`, `nome` ou `anos_academicos`.
- `400` se `codigo` tiver caracteres inválidos ou resultar vazio após normalização.
- `400` se `nome` estiver vazio após trim.
- `400` se `anos_academicos` estiver vazio ou contiver valores vazios.
- `400` se a categoria já existir nesta academia.
- `400` se a academia autenticada não for do ensino superior.
- `403` se a academia autenticada estiver inativa.
- `404` se a academia autenticada não existir na projeção.

---

#### DELETE /academia/categorias-nota/:codigo

Remove uma categoria de nota configurável da academia superior autenticada.

**Proteção:** autenticado + academia ativa.

**Escopo da rota:** remoção/inativação síncrona de uma categoria de nota da própria academia superior autenticada. A rota não remove categorias escolares fixas e não permite apagar categorias de outra academia.

**Path params:**

| Campo | Obrigatório | Exemplo | Descrição |
| --- | --- | --- | --- |
| `codigo` | Sim | `DELETE /academia/categorias-nota/prova_parcelar_1` | Código da categoria a remover. O backend aplica a mesma normalização do cadastro. |

**Request:** sem payload.

**Response 200:**

```json
{
  "message": "categoria removida com sucesso",
  "categoria": "prova_parcelar_1"
}
```

**Regras de negócio:**

- Apenas academias com `nivel="superior"` podem remover categorias configuráveis.
- Escolas não removem categorias por esta rota porque suas categorias são fixas do sistema.
- A categoria deve existir como ativa no estado do aggregate ou na projeção da academia.
- A remoção emite evento `CategoriaNotaRemovida`; a projeção deixa de listar a categoria em consultas ativas.
- O código removido deixa de estar disponível para novos lançamentos de nota e novas regras que validem categorias ativas.

**Erros comuns:**

- `400` se `codigo` estiver vazio ou inválido.
- `400` se a categoria não existir nesta academia.
- `400` se a academia autenticada não for do ensino superior.
- `403` se a academia autenticada estiver inativa.
- `404` se a academia autenticada não existir na projeção.

---

#### POST /academia/categorias-nota/async

Cria categorias de nota em lote por job assíncrono, usando o mesmo contrato de cada item de `POST /academia/categorias-nota`.

**Proteção:** autenticado + academia ativa.

**Escopo da rota:** enqueue de múltiplas criações de categoria para processamento em background. Cada item é validado com as mesmas regras da criação síncrona.

**Request body:** array obrigatório com 1 a 500 itens.

```json
[
  {
    "codigo": "prova_parcelar_1",
    "nome": "Prova Parcelar 1",
    "descricao": "Primeira prova parcelar do semestre",
    "anos_academicos": ["1_ano_superior"]
  }
]
```

**Response 202:**

```json
{
  "message": "job criado com sucesso — use GET /jobs/:id ou GET /jobs/stream para acompanhar o progresso",
  "job_id": "uuid",
  "total_items": 1,
  "status": "pending",
  "poll_url": "/jobs/uuid",
  "sse_url": "/jobs/stream"
}
```

**Regras de negócio:**

- O payload deve ser um array bruto; não use dupla serialização.
- Cada item segue exatamente os campos e validações de `POST /academia/categorias-nota`.
- O processamento individual reaproveita o handler síncrono; erros de um item ficam registrados no resultado do job.
- O limite máximo é 500 itens por requisição.

---

#### DELETE /academia/categorias-nota/async

Remove categorias de nota em lote por job assíncrono.

**Proteção:** autenticado + academia ativa.

**Escopo da rota:** enqueue de múltiplas remoções de categoria para processamento em background. Cada item é validado com as mesmas regras da remoção síncrona.

**Request body:** array obrigatório com 1 a 500 itens.

```json
[
  { "codigo": "prova_parcelar_1" }
]
```

**Campos por item:**

| Campo | Obrigatório | Descrição |
| --- | --- | --- |
| `codigo` | Sim | Código da categoria a remover. |
| `nome` | Não | Compatibilidade: se `codigo` vier vazio, o worker usa `nome` como fallback para o código. |

**Response 202:**

```json
{
  "message": "job criado com sucesso — use GET /jobs/:id ou GET /jobs/stream para acompanhar o progresso",
  "job_id": "uuid",
  "total_items": 1,
  "status": "pending",
  "poll_url": "/jobs/uuid",
  "sse_url": "/jobs/stream"
}
```

**Regras de negócio:**

- O payload deve ser um array bruto; não use dupla serialização.
- Cada item resolve `codigo` e chama a mesma regra de `DELETE /academia/categorias-nota/:codigo`.
- O processamento individual reaproveita o handler síncrono; erros de um item ficam registrados no resultado do job.
- O limite máximo é 500 itens por requisição.


### GET /academia/anos-academicos

Retorna uma visão unificada dos escopos acadêmicos habilitados da academia: anos do fundamental armazenados na própria academia e anos/períodos dos cursos médio ou superior pertencentes a ela.

**Proteção**: autenticado + academia ativa **ou** admin ativo.

**Funcionamento:**

- Quando o usuário autenticado é uma academia, a rota consulta automaticamente a academia do token.
- Quando o usuário autenticado é admin, a rota exige `codigo_academia` na query string para indicar qual academia será consultada.
- A resposta inclui todos os cursos da academia para que o cliente identifique quais escopos de médio/superior podem ser alterados nas rotas de escrita.
- Esta rota é somente leitura; não altera anos acadêmicos, períodos, cursos, estudantes nem histórico.

**Request:** sem payload.

**Query params:**

| Campo | Obrigatório | Quando usar | Descrição |
| --- | --- | --- | --- |
| `codigo_academia` | Sim para admin; não usado para academia | `GET /academia/anos-academicos?codigo_academia=ACA-001` | Código público da academia que o admin quer consultar. |

**Response 200:**

```json
{
  "academia": {
    "nivel": "escolar",
    "nivel_escolar": "misto",
    "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental"]
  },
  "cursos": [
    {
      "id": "uuid-do-curso-medio",
      "codigo_academia": "ACA-001",
      "nome": "Ciências Económicas e Jurídicas",
      "type": "medio",
      "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"],
      "periodos": null,
      "status": "ativo"
    },
    {
      "id": "uuid-do-curso-superior",
      "codigo_academia": "ACA-001",
      "nome": "Engenharia Informática",
      "type": "superior",
      "anos_academicos": ["1_ano_superior", "2_ano_superior"],
      "periodos": ["1_semestre", "2_semestre", "3_semestre", "4_semestre"],
      "status": "ativo"
    }
  ]
}
```

**Erros esperados:**

| Status | Quando ocorre | Response |
| --- | --- | --- |
| `400` | Admin não enviou `codigo_academia`. | Ver envelope detalhado abaixo com `field="codigo_academia"` e `code="campo_obrigatorio"`. |
| `401` | Token ausente ou inválido. | `{ "error": "UNAUTHORIZED", "message": "..." }` |
| `403` | Usuário não é academia nem admin autorizado, ou academia está inativa. | `{ "error": "FORBIDDEN", "message": "..." }` |
| `404` | Academia do token ou `codigo_academia` não encontrada. | `{ "error": "NOT_FOUND", "message": "academia não encontrado" }` |

**Exemplo 400 — admin sem `codigo_academia`:**

```json
{
  "error": "VALIDATION_ERROR",
  "message": "Administradores precisam informar o parâmetro de consulta 'codigo_academia' para o sistema saber de qual academia deve listar/alterar os anos acadêmicos. Exemplo: ?codigo_academia=ACA001",
  "request_id": "uuid-da-requisicao",
  "details": [
    {
      "field": "codigo_academia",
      "code": "campo_obrigatorio",
      "message": "Administradores precisam informar o parâmetro de consulta 'codigo_academia' para o sistema saber de qual academia deve listar/alterar os anos acadêmicos. Exemplo: ?codigo_academia=ACA001"
    }
  ]
}
```

### POST /academia/anos-academicos

Adiciona/habilita novos escopos acadêmicos sem remover os escopos existentes. Não existe mais operação de substituição em massa para anos acadêmicos.

**Proteção**: autenticado + academia ativa. Admins não escrevem por esta rota.

**Funcionamento por `type`:**

| `type` | Onde altera | Campos aceitos | Campos obrigatórios | Resultado |
| --- | --- | --- | --- | --- |
| `fundamental` | Academia autenticada (`projection_academias.anos_academicos`) | `type`, `anos_academicos` | `type`, `anos_academicos` | Une os anos enviados com os anos do Ensino Primário e Iº Ciclo já ativos. |
| `medio` | Não altera por esta rota | nenhum fluxo de escrita permitido | n/a | Retorna erro estruturado. Cursos médios têm anos fixos derivados de `modelo`. |
| `superior` | Não altera por esta rota | nenhum fluxo de escrita permitido | n/a | Retorna erro estruturado. Cursos superiores não aceitam adição direta de anos/períodos por `/academia/anos-academicos`. |

Payloads com `codigo_academia`, campos desconhecidos ou campos de substituição em massa como `substituir`, `replace`, `patch`, `set` e `update` são rejeitados.

**Request — fundamental/misto:**

```json
{
  "type": "fundamental",
  "anos_academicos": ["4_ano_fundamental"]
}
```

**Response 200 — fundamental/misto:**

```json
{
  "message": "anos acadêmicos atualizados com sucesso",
  "type": "fundamental",
  "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental", "4_ano_fundamental"]
}
```

**Request — médio:** não suportado. Enviar `type="medio"` em `POST /academia/anos-academicos` retorna erro porque anos médios são fixos por modelo.

### DELETE /academia/anos-academicos

Desabilita/remover logicamente escopos acadêmicos da oferta futura, preservando histórico. Use esta rota para reduzir a oferta sem apagar dados já registrados.

**Proteção**: autenticado + academia ativa. Admins não escrevem por esta rota.

**Funcionamento por `type`:**

- `fundamental`: remove do cadastro da academia somente os anos enviados em `anos_academicos`.
- `medio`: não é permitido por esta rota. Cursos médios têm anos fixos derivados de `modelo` e não aceitam remoção manual.
- `superior`: não é permitido por esta rota. Cursos superiores não aceitam remoção direta de anos acadêmicos, períodos ou semestres por `/academia/anos-academicos`.
- A remoção é lógica/prospectiva: o backend não apaga eventos, ledger, estudantes, turmas, matérias, notas, faltas, avaliações finais já registrados.

**Request — fundamental/misto:**

```json
{
  "type": "fundamental",
  "anos_academicos": ["4_ano_fundamental"]
}
```

**Request — médio:** não suportado. Enviar `type="medio"` em `DELETE /academia/anos-academicos` retorna erro porque anos médios são fixos por modelo.

**Response 200 — fundamental/médio:**

```json
{
  "message": "anos acadêmicos atualizados com sucesso",
  "type": "fundamental",
  "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental", "3_ano_fundamental"]
}
```

**Response 200 — superior:**

```json
{
  "message": "anos acadêmicos atualizados com sucesso",
  "type": "superior",
  "curso_id": "uuid-do-curso-superior",
  "anos_academicos": ["1_ano_superior", "2_ano_superior", "3_ano_superior"],
  "periodos": ["1_semestre", "2_semestre", "3_semestre", "4_semestre", "5_semestre", "6_semestre"]
}
```

### Validações e erros de `POST` e `DELETE /academia/anos-academicos`

**Validações comuns:**

- `type` é obrigatório e deve ser `fundamental`, `medio` ou `superior`.
- A academia só altera o próprio escopo.
- `curso_id` é obrigatório para `medio` e `superior` e precisa pertencer à academia autenticada.
- O `type` do payload precisa corresponder ao `type` do curso informado.
- `fundamental` só é permitido para academias escolares com `nivel_escolar` igual a `fundamental` ou `misto`.
- `fundamental` aceita somente códigos canônicos `[1-9]_ano_fundamental`.
- `medio` aceita somente anos médios compatíveis com o curso e mantém a lista final em ordem sequencial crescente contínua desde `1_ano_medio`.
- `superior` não possui fluxo de escrita por `/academia/anos-academicos`; tentativas de adicionar/remover anos acadêmicos, períodos ou semestres retornam erro estruturado.
- Academias fundamental/misto devem manter ao menos um ano acadêmico ativo após a operação.
- Reduções em `DELETE` são bloqueadas com `409 Conflict` quando existem estudantes ativos no ano removido (`status_escolar_fundamental` ou `status_escolar_medio` em andamento conforme o escopo operacional).

**Erros esperados:**

| Status | Quando ocorre | Response |
| --- | --- | --- |
| `400` | Payload JSON inválido. | Envelope detalhado com `field="payload"` e `code="json_invalido"`. |
| `400` | `type` ausente ou diferente de `fundamental`, `medio` e `superior`. | Envelope detalhado com `field="type"` e `code="valor_invalido"`. |
| `400` | `curso_id` ausente para médio/superior. | Envelope detalhado com `field="curso_id"` e `code="campo_obrigatorio"`. |
| `400` | Curso inexistente. | Envelope detalhado com `field="curso_id"` e `code="nao_encontrado"`. |
| `400` | Curso pertence a outra academia. | Envelope detalhado com `field="curso_id"` e `code="curso_de_outra_academia"`. |
| `400` | Curso está inativo. | Envelope detalhado com `field="curso_id"` e `code="curso_inativo"`. |
| `400` | `type` do payload não corresponde ao tipo do curso. | Envelope detalhado com `field="type"` e `code="tipo_diferente_do_curso"`. |
| `400` | Academia não pode gerenciar fundamental. | Envelope detalhado com `field="type"` e `code="nivel_incompativel"`. |
| `400` | `anos_academicos` ausente, vazio ou em formato inválido. | Envelope detalhado com `field="anos_academicos"` e `code="campo_obrigatorio"` ou `code="formato_invalido"`. |
| `400` | A operação deixaria academia fundamental/misto sem nenhum ano ativo. | Envelope detalhado com `field="anos_academicos"` e `code="remocao_invalida"`. |
| `400` | Curso médio ficaria sem anos ou com sequência inválida. | Envelope detalhado com `field="anos_academicos"` e `code="remocao_invalida"` ou `code="sequencia_invalida"`. |
| `400` | Tentativa de escrita direta em curso superior. | Envelope detalhado com `field="type"` e `code="operacao_nao_suportada"`. |
| `409` | Redução afetaria estudantes ativos. | Envelope detalhado com `field="anos_academicos"` e `code="estudantes_ativos_vinculados"`. |
| `401` | Token ausente ou inválido. | `{ "error": "UNAUTHORIZED", "message": "..." }` |
| `403` | Usuário não é academia ativa. | `{ "error": "FORBIDDEN", "message": "..." }` |
| `404` | Academia autenticada não encontrada. | `{ "error": "NOT_FOUND", "message": "academia não encontrado" }` |

**Formato detalhado dos erros de anos acadêmicos:**

As rotas `GET`, `POST` e `DELETE /academia/anos-academicos`
mantêm o envelope global de erro, mas agora retornam `details` com um único
item apontando o campo exato que deve ser corrigido.

```json
{
  "error": "VALIDATION_ERROR",
  "message": "O campo 'curso_id' é obrigatório quando type='medio', porque anos de médio/superior pertencem a um curso específico.",
  "request_id": "uuid-da-requisicao",
  "details": [
    {
      "field": "curso_id",
      "code": "campo_obrigatorio",
      "message": "O campo 'curso_id' é obrigatório quando type='medio', porque anos de médio/superior pertencem a um curso específico."
    }
  ]
}
```

**Exemplo 400 — `type` inválido ou ausente:**

```json
{
  "error": "VALIDATION_ERROR",
  "message": "O campo 'type' recebeu '', mas só aceita: 'fundamental', 'medio' ou 'superior'. Use 'fundamental' para anos do Ensino Primário e Iº Ciclo, 'medio' para cursos médios e 'superior' para cursos superiores.",
  "request_id": "uuid-da-requisicao",
  "details": [
    {
      "field": "type",
      "code": "valor_invalido",
      "message": "O campo 'type' recebeu '', mas só aceita: 'fundamental', 'medio' ou 'superior'. Use 'fundamental' para anos do Ensino Primário e Iº Ciclo, 'medio' para cursos médios e 'superior' para cursos superiores."
    }
  ]
}
```

**Exemplo 400 — curso superior com `anos_academicos`:**

```json
{
  "error": "VALIDATION_ERROR",
  "message": "Não envie 'anos_academicos' para curso superior. Para superior, envie apenas 'periodos'; o sistema calcula os anos automaticamente. Exemplo: periodos=8 gera anos como ['1_ano_superior', '2_ano_superior', ...].",
  "request_id": "uuid-da-requisicao",
  "details": [
    {
      "field": "anos_academicos",
      "code": "campo_nao_permitido",
      "message": "Não envie 'anos_academicos' para curso superior. Para superior, envie apenas 'periodos'; o sistema calcula os anos automaticamente. Exemplo: periodos=8 gera anos como ['1_ano_superior', '2_ano_superior', ...]."
    }
  ]
}
```

**Exemplo 409 — estudantes ativos bloqueando remoção/redução:**

```json
{
  "error": "CONFLICT",
  "message": "Não é possível desativar os anos [4_ano_fundamental] porque existem 3 estudante(s) ativo(s) vinculados a eles. Transfira, conclua ou inative esses estudantes antes de remover os anos.",
  "request_id": "uuid-da-requisicao",
  "details": [
    {
      "field": "anos_academicos",
      "code": "estudantes_ativos_vinculados",
      "message": "Não é possível desativar os anos [4_ano_fundamental] porque existem 3 estudante(s) ativo(s) vinculados a eles. Transfira, conclua ou inative esses estudantes antes de remover os anos."
    }
  ]
}
```

---

---

## 7. Ano Letivo

### Processos de Negócio — Configuração do Ano Letivo

### 7.1 Configuração do Ano Letivo

**Quem faz**:

- **Admin FPP** define o **ano letivo oficial global por tipo** via `POST /admin/definir-ano-letivo-geral`, informando `type=escolar` ou `type=superior` e o `ano_letivo` desejado no formato `YYYY_YYYY`, apenas enquanto não houver academia ativa daquele tipo com ano letivo definido
- **Academia** define o seu primeiro ano letivo ativo via `POST /academia/definir-ano-letivo`, sempre alinhado ao global do tipo inferido pelo próprio cadastro (`escola` → `escolar`, `superior` → `superior`); depois avança automaticamente ao finalizar o ano letivo

Antes de registar qualquer nota, falta ou avaliação, a academia deve definir o ano letivo ativo.

Os anos letivos oficiais globais são persistidos em `projection_sistema_config` com as chaves `ano_letivo_atual_escolar` e `ano_letivo_atual_superior`; essa projeção deve existir antes da chamada administrativa.

Além do valor atual, o sistema mantém `anos_letivos_lista` em `projection_sistema_config` como histórico global (sem duplicar `ano_letivo`). Esse histórico pode ser consultado por qualquer usuário autenticado na rota `GET /anos-letivos-lista?type=...` e o valor atual em `GET /ano-letivo?type=...`.

**Formato obrigatório**: `YYYY_YYYY` onde o segundo ano é exatamente o primeiro + 1 (ex: `2025_2026`)

**Tipo**: `escolar` ou `superior`; `escola` não é aceito como alias para tipo de ano letivo.

A academia só pode definir diretamente quando ainda não possui ano letivo. Depois disso, a evolução acontece pela finalização realizada pelas academias, que calcula o próximo período a partir do ano final do anterior. O ano letivo ativo é resolvido automaticamente em todos os novos registos de nota, falta e avaliação.

O período real aceito para faltas não é salvo como datas fixas em cada ano e não é configurável por academia, admin ou payload. O backend deriva sempre o `periodo` pelo tipo do ano letivo: `escolar -> 09_07` e `superior -> 10_07`. Em seguida combina esse período fixo com o `ano_letivo` ativo da academia para calcular o intervalo: com `ano_letivo=2025_2026`, escolar permite `2025-09-01` a `2026-07-31`, e superior permite `2025-10-01` a `2026-07-31`.

**Regra de alinhamento obrigatório**: se a academia tentar definir um ano letivo diferente do ano oficial global do seu tipo definido pelo admin FPP, a operação deve ser rejeitada com erro de negócio.

Sempre que o ano letivo for atualizado, ele é adicionado em `anos_letivos_lista` apenas se ainda não existir para aquela academia. Se já existir, o backend ignora a duplicação.


### POST /admin/definir-ano-letivo-geral

Define diretamente o **ano letivo oficial global por tipo** (`escolar` ou `superior`). O Admin FPP deve informar `type` e o `ano_letivo` desejado no payload, no formato `YYYY_YYYY` com segundo ano igual ao primeiro + 1 (ex.: `2026_2027`). O backend não calcula automaticamente o ano letivo nesta rota; o valor informado pelo admin é persistido para o tipo escolhido. A definição administrativa só é permitida quando nenhuma academia ativa daquele tipo possui ano letivo definido; depois disso, a evolução global daquele tipo passa a ser automática quando todas as academias ativas do mesmo tipo estiverem alinhadas no mesmo ano letivo.

Não há aliases de compatibilidade para esta operação.

**Proteção**: autenticado + admin role `fpp`

**Regras de negócio:**

- Apenas `fpp` pode definir diretamente o ano letivo global.
- O campo `ano_letivo` é obrigatório e deve usar `YYYY_YYYY` com segundo ano = primeiro + 1.
- Esse valor torna-se referência obrigatória para a rota `POST /academia/definir-ano-letivo`.
- A definição é bloqueada se existir qualquer academia ativa do tipo informado com `ano_letivo` já definido.

**Request:** informe o tipo do calendário global e o ano letivo que deve ser definido para esse tipo.

```json
{ "type": "escolar", "ano_letivo": "2026_2027" }
```

**Response 200:**

```json
{
  "message": "ano letivo global definido com sucesso",
  "type": "escolar",
  "ano_letivo": "2026_2027",
  "periodo": "09_07",
  "imutavel": true
}
```

**Erros:**

- `409` — existe academia ativa com ano letivo já definido; a evolução global será automática quando todas estiverem alinhadas
- `403` — usuário não é `fpp`

---

### GET /ano-letivo

Retorna o **ano letivo oficial global atual** da plataforma para o tipo informado em `?type=escolar` ou `?type=superior`.

**Proteção**: autenticado (qualquer usuário logado)


**Request:** sem payload
**Response 200:**

```json
{
  "ano_letivo": "2026_2027"
}
```

**Erros:**

- `404` — ano letivo global ainda não definido

---

### GET /anos-letivos-lista

Retorna a **lista histórica de anos letivos globais** já definidos pelo admin.

**Proteção**: autenticado (qualquer usuário logado)


**Request:** sem payload
**Response 200:**

```json
{
  "anos_letivos_lista": [
    {
      "ano_letivo": "2025_2026",
      "definido_por": "11111111-1111-1111-1111-111111111111",
      "definido_em": "2025-09-01T08:00:00Z"
    },
    {
      "ano_letivo": "2026_2027",
      "definido_por": "11111111-1111-1111-1111-111111111111",
      "definido_em": "2026-09-01T08:00:00Z"
    }
  ]
}
```

**Regra da lista histórica (`anos_letivos_lista`)**: cada ano letivo global é adicionado apenas uma vez; tentativas repetidas do mesmo `ano_letivo` não duplicam itens.

---

### POST /academia/definir-ano-letivo

Define o ano letivo ativo da academia apenas quando ela ainda não possui ano letivo, alinhado ao ano letivo global atual do tipo da própria academia. A academia não envia `type`: o backend infere `escolar` para academias `nivel=escola` e `superior` para academias `nivel=superior`. O campo `ano_letivo` é opcional; quando omitido, o backend usa o ano letivo global atual daquele tipo. A passagem para o ano seguinte não usa uma rota própria: acontece automaticamente ao finalizar o ano letivo.

Não há aliases de compatibilidade para esta operação.

**Request:** opcionalmente informe `ano_letivo`; não envie `tipo` nem `periodo`, pois o tipo é inferido da academia e o período é fixo/read-only.

```json
{
  "ano_letivo": "2026_2027"
}
```

**Response 200:**

```json
{
  "message": "ano letivo definido com sucesso",
  "ano_letivo": "2026_2027",
  "tipo": "escolar",
  "periodo": "09_07",
  "imutavel": true
}
```

**Erros principais:**

- `409` — ano letivo global ainda não definido pelo admin.
- `409` — ano letivo da academia já definido; finalize o ano letivo atual para avançar automaticamente.
- `400` — ano letivo informado diferente do global atual.


---

### GET /academia/ano-letivo

Retorna o ano letivo ativo da academia alvo. Academias continuam consultando o próprio ano letivo; admins e estudantes podem consultar qualquer academia informando o código da academia.

**Proteção**: autenticado. Se o usuário autenticado for uma academia, ela também precisa estar ativa.

**Query params:**

- `codigo_academia` (opcional para academia, obrigatório para admin e estudante): código da academia alvo.
  - Se o usuário for `academia`, o backend ignora o parâmetro e retorna o próprio ano letivo.
  - Se o usuário for `admin` ou `estudante`, deve informar `?codigo_academia=...`.


**Request:** sem payload
**Response 200:**

```json
{
  "ano_letivo": "2025_2026",
  "tipo": "escolar",
  "ativado_em": "2025-01-15T08:00:00Z"
}
```

**Erros:**

- `404` — ano letivo não configurado
- `404` — academia não encontrada (incluindo admin ou estudante sem `codigo_academia`)

---

### GET /academia/anos-letivos-lista

Retorna a lista histórica de anos letivos definidos pela academia alvo. Academias continuam consultando a própria lista; admins e estudantes podem consultar a lista de qualquer academia informando o código da academia.

**Proteção**: autenticado. Se o usuário autenticado for uma academia, ela também precisa estar ativa.

**Query params:**

- `codigo_academia` (opcional para academia, obrigatório para admin e estudante): código da academia alvo.
  - Se o usuário for `academia`, o backend ignora o parâmetro e retorna a própria lista.
  - Se o usuário for `admin` ou `estudante`, deve informar `?codigo_academia=...`.


**Request:** sem payload
**Response 200:**

```json
{
  "anos_letivos_lista": [
    {
      "ano_letivo": "2024_2025",
      "tipo": "escolar",
      "definido_por": "11111111-1111-1111-1111-111111111111",
      "definido_em": "2024-09-01T08:00:00Z"
    },
    {
      "ano_letivo": "2025_2026",
      "tipo": "escolar",
      "definido_por": "11111111-1111-1111-1111-111111111111",
      "definido_em": "2025-09-01T08:00:00Z"
    }
  ]
}
```

**Erros:**

- `404` — academia não encontrada (incluindo admin ou estudante sem `codigo_academia`)

---

### Configurações de período letivo

O backend separa duas coisas que o cliente deve tratar como conceitos diferentes:

1. **Ano letivo ativo** (`ano_letivo`, exemplo `2025_2026`) — valor evolutivo definido pelo Admin FPP no escopo global e pela academia no próprio escopo, sempre alinhado ao global.
2. **Período fixo por tipo** (`periodo`, exemplo `09_07`) — regra sistêmica imutável derivada exclusivamente do tipo de ensino para calcular o intervalo real de datas aceitas.

Cada tipo canônico de ano letivo possui exatamente um período fixo, não configurável por academia ou Admin FPP:

- `escolar` — usado para fundamental e médio, sempre com `periodo=09_07`. O alias legado `escola` não é mais aceito para `type` de ano letivo.
- `superior` — usado para ensino superior, sempre com `periodo=10_07`.

O `periodo` usa o formato `MM_MM`, em que o primeiro mês pertence ao ano inicial de `ano_letivo` e o segundo mês pertence ao ano final. Exemplo: `ano_letivo=2025_2026` com `type=escolar` usa `09_07` e permite datas de `2025-09-01` a `2026-07-31`; com `type=superior` usa `10_07` e permite datas de `2025-10-01` a `2026-07-31`. O cliente não precisa calcular esse intervalo para validar segurança; o backend recalcula e valida em operações sensíveis, especialmente faltas. A definição do ano letivo seguinte também respeita a mesma janela operacional da finalização: enquanto o mês atual ainda estiver dentro do ano letivo em curso delimitado pelo período fixo do tipo, o avanço para o próximo ano letivo é bloqueado.

#### GET `/anos-letivos/configuracoes`

Lista os períodos fixos vigentes, derivados da regra sistêmica imutável.

Request: não possui body.

Response:

```json
{
  "configuracoes": [
    {
      "type": "escolar",
      "periodo": "09_07",
      "imutavel": true
    },
    {
      "type": "superior",
      "periodo": "10_07",
      "imutavel": true
    }
  ]
}
```

#### GET `/admin/sistema/anos-letivos/configuracoes`

Lista as configurações vigentes para Admin FPP. A estrutura do retorno é a mesma de `GET /anos-letivos/configuracoes`; a diferença é a exigência de autenticação como Admin FPP.

Request: não possui body.

Response:

```json
{
  "configuracoes": [
    {
      "type": "escolar",
      "periodo": "09_07",
      "imutavel": true
    },
    {
      "type": "superior",
      "periodo": "10_07",
      "imutavel": true
    }
  ]
}
```

#### PUT `/admin/sistema/anos-letivos/configuracoes/:type`

Apenas Admin FPP. O período do tipo informado é fixo e imutável; este endpoint não transforma `escolar` em `10_07` nem `superior` em `09_07`. Para compatibilidade, payloads que repetem o valor fixo podem receber resposta de sucesso sem alterar a regra; payloads divergentes são rejeitados com erro de validação. O parâmetro `:type` aceita somente `escolar` ou `superior`.

Request params:

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `type` | `string` | Sim | Tipo do ano letivo no path. Valores aceitos: `escolar` ou `superior`. |

Request body:

```json
{
  "periodo": "09_07"
}
```

Response para payload compatível com a regra fixa:

```json
{
  "message": "configuração de ano letivo mantida; periodo é fixo e imutável",
  "type": "escolar",
  "periodo": "09_07",
  "updated_by": "uuid-do-admin-fpp"
}
```

Exemplo rejeitado: `PUT /admin/sistema/anos-letivos/configuracoes/superior` com `{ "periodo": "09_07" }`, pois superior sempre usa `10_07`.

### Validação de faltas pelo período letivo

O registro e a atualização de faltas validam a data no backend usando o tipo inferido da matéria (`superior` ou `escolar` para fundamental/médio), o `ano_letivo` ativo da academia e o `periodo` fixo derivado do tipo. Datas fora do intervalo retornam `400` com mensagem indicando o intervalo permitido.

### Finalização de ano letivo por academia

#### POST `/academia/anos-letivos/finalizar`

A academia autenticada finaliza o ano letivo ativo no próprio escopo e, na mesma operação, avança automaticamente para o ano letivo seguinte. O cliente não envia `academia_id`; o backend obtém a academia pelo token, normaliza `type`, valida que o `ano_letivo` informado, quando presente, corresponde ao ano letivo ativo da academia, valida o formato `YYYY_YYYY` com segundo ano igual ao primeiro + 1, valida que o ano atual é exatamente o ano final desse `ano_letivo`, valida a janela mensal de finalização pelo `periodo` fixo derivado do tipo e grava um evento auditável `AnoLetivoAcademiaFinalizado`. As duas condições temporais precisam passar simultaneamente: para `ano_letivo=2025_2026`, a finalização só é aceita em 2026 e dentro da janela mensal do tipo. A janela mensal é inclusiva no mês final e exclusiva no mês inicial: o mês atual precisa ser maior ou igual ao mês de fim do período letivo e menor que o mês de início do período letivo. Exemplo: se `periodo=10_07`, a finalização de `2025_2026` é permitida somente em julho, agosto e setembro de 2026; em outubro o próximo período já começou, e de novembro a junho o período vigente ainda não chegou ao mês de encerramento.

Request:

```json
{
  "type": "escolar",
  "ano_letivo": "2025_2026",
  "observacao": "Ano letivo encerrado após fechamento de notas e faltas."
}
```

Response:

```json
{
  "message": "ano letivo finalizado com sucesso; academia avançada para o ano letivo seguinte",
  "academia_id": "uuid-da-academia",
  "type": "escolar",
  "ano_letivo_finalizado": "2025_2026",
  "ano_letivo": "2026_2027",
  "finalizado": true,
  "global_atualizado": false
}
```

A operação é auditada por `(academia_id, type, ano_letivo_finalizado)` e avança a academia para o próximo `YYYY_YYYY`. Se, após esse avanço, todas as academias ativas do mesmo tipo estiverem no mesmo ano letivo, o backend atualiza automaticamente o ano letivo global daquele tipo para esse ano. Se o ano atual for diferente do ano final do `ano_letivo` ou se o mês estiver fora da janela mensal permitida, o backend retorna `400` com mensagem específica para a condição que falhou e não grava novo evento.

#### GET `/academia/anos-letivos/finalizacoes`

Lista as finalizações da academia autenticada. O cliente não envia `academia_id`; o backend identifica a academia pelo token.

Request: não possui body nem query params.

Response:

```json
{
  "finalizacoes": [
    {
      "type": "escolar",
      "ano_letivo": "2025_2026",
      "finalizado": true,
      "finalizado_em": "2026-06-26T11:00:00Z",
      "observacao": "Ano letivo encerrado após fechamento de notas e faltas."
    },
    {
      "type": "superior",
      "ano_letivo": "2025_2026",
      "finalizado": true,
      "finalizado_em": "2026-06-26T11:05:00Z",
      "observacao": ""
    }
  ]
}
```

#### GET `/admin/academias/anos-letivos/finalizacoes`

Apenas Admin FPP. Consulta finalizações por academia, com filtros opcionais via query string. A rota registrada no backend é somente `/admin/academias/anos-letivos/finalizacoes`; `type` e `ano_letivo` são parâmetros de consulta opcionais e não fazem parte do path.

Request: não possui body.

Exemplo com filtros: `GET /admin/academias/anos-letivos/finalizacoes?type=escolar&ano_letivo=2025_2026`

Query params opcionais:

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `type` | `string` | Não | Filtra pelo tipo. Valores aceitos: `escolar` ou `superior`. |
| `ano_letivo` | `string` | Não | Filtra pelo ano letivo no formato `YYYY_YYYY`, com o segundo ano igual ao primeiro + 1. |

Response:

```json
{
  "finalizacoes": [
    {
      "academia_id": "uuid-da-academia",
      "codigo_academia": "ACA1",
      "type": "escolar",
      "ano_letivo": "2025_2026",
      "finalizado": true,
      "finalizado_em": "2026-06-26T11:00:00Z",
      "observacao": "Ano letivo encerrado após fechamento de notas e faltas."
    }
  ]
}
```

#### GET `/admin/sistema/anos-letivos/finalizacao-limites`

Apenas Admin FPP. Retorna, por tipo, o maior ano letivo finalizado por todas as academias ativas aplicáveis e o mínimo global permitido.

Request: não possui body nem query params.

Response:

```json
{
  "limites": [
    {
      "type": "escolar",
      "ano_letivo_finalizado_por_todas": "2025_2026",
      "minimo_global_permitido": "2026_2027",
      "academias_total": 12,
      "academias_finalizadas": 12
    },
    {
      "type": "superior",
      "ano_letivo_finalizado_por_todas": "",
      "minimo_global_permitido": "",
      "academias_total": 8,
      "academias_finalizadas": 0
    }
  ]
}
```

### Bloqueio de retrocesso global

Ao definir inicialmente o ano letivo global, o backend bloqueia a operação se alguma academia ativa do tipo informado já tiver ano letivo. Depois da definição inicial, o avanço global não é manual: ele acontece automaticamente quando todas as academias ativas do mesmo tipo passam a estar no mesmo ano letivo após suas finalizações.

Para implementar o cliente de forma segura:

1. Admin FPP consulta `GET /admin/sistema/anos-letivos/configuracoes` para confirmar o `periodo` fixo por tipo; o `PUT` legado não deve ser usado para alterar a regra, pois valores divergentes são rejeitados.
2. Admin FPP define inicialmente o ano global com `POST /admin/definir-ano-letivo-geral`, enviando `type` e `ano_letivo`, desde que não exista academia ativa com ano letivo definido.
3. Cada academia sem ano letivo define o próprio ano letivo com `POST /academia/definir-ano-letivo` usando o ano global atual.
4. Ao encerrar notas/faltas/avaliações de um ciclo, a academia chama `POST /academia/anos-letivos/finalizar`; essa chamada finaliza o ano ativo e já avança para o seguinte.
5. Telas administrativas podem usar `GET /admin/sistema/anos-letivos/finalizacao-limites` para mostrar o marco finalizado por todas as academias e o mínimo global permitido antes de tentar avançar ou corrigir o global.

---

## 8. Estudantes

### Processos de Negócio — Cadastro de Estudante

### 8.1 Cadastro de Estudante

**Quem faz**: Academia (status ativo)

1. Academia envia os dados do estudante em `multipart/form-data`, com ou sem anexos.
2. Sistema mantém obrigatórias as validações cadastrais e acadêmicas, mas não bloqueia o cadastro direto pela ausência de PDFs.
3. Sistema valida que todos os arquivos enviados são PDF, respeitam o limite de 10MB e possuem assinatura `%PDF`.
4. Sistema gera código único (`AAA1234`), verificando ledger e projeção.
5. Quando enviados, os documentos são enviados ao storage definitivo em `{codigo_academia}/estudantes/{codigo_estudante}/documentos/`.
6. Senha padrão = código do estudante (ex: `ABC1234`).
7. Estudante é criado com **status `ativo`**, vinculado à academia e com o mapa `documentos` gravado no evento `EstudanteCriadoComVinculo` e na projeção.
8. Opcionalmente, se `codigo_turma` for informado, a turma é pré-validada antes de uploads/gravações e o vínculo é tentado após a persistência do estudante.
9. Se qualquer validação ou persistência falhar após upload parcial, o diretório de documentos do estudante é removido para evitar ficheiros órfãos.

**Regras de validação:**

- `genero` obrigatório: `masculino` ou `feminino`
- `data_nascimento` obrigatório: deve ser anterior à data atual
- JSON puro não é aceito no cadastro direto; o fluxo deve usar `multipart/form-data`, mesmo quando nenhum anexo for enviado
- `bilhete_identidade_encarregado` e PDF `bi_encarregado` são obrigatórios para estudantes escolares/fundamental/médio; no ensino superior, o encarregado é opcional
- `bilhete_identidade` e `bilhete_identidade_encarregado`, quando ambos informados, não podem ser iguais após normalização
- `bi_estudante` é obrigatório no ensino superior e quando o estudante escolar informa BI próprio; sem BI próprio no escolar, `cedula_estudante` é obrigatória
- o BI do encarregado não pode coincidir com o BI principal de outro estudante escolar/fundamental/médio, mas pode repetir como BI de encarregado de irmãos/outros estudantes
- `declaracao`/certificados seguem a matriz automática por ano acadêmico: sem cobrança no `1_ano_fundamental`; `7_ano_fundamental`, `1_ano_medio` e `1_ano_superior` exigem certificado específico ou declaração
- `ano_escolar_fundamental` deve seguir o formato canônico para o tipo de ensino
- Se informar `curso_medio_id`, o curso deve existir, estar ativo, pertencer à academia e ser do tipo `medio`
- Se informar `curso_superior_id`, o curso deve existir, estar ativo, pertencer à academia e ser do tipo `superior`
- Status inicial padrão para fundamental: `em_andamento`
- Status inicial padrão para médio e superior: `inativo` até eventos específicos de matrícula/curso
### Regras de Negócio — Estudante

### 8.2 Regras de Estudante

| Regra                                                         | Detalhe                                 |
| ------------------------------------------------------------- | --------------------------------------- |
| Apenas academia pode cadastrar estudantes                     | Não existe auto-cadastro                |
| `genero` e `data_nascimento` são obrigatórios                 | Não podem ser omitidos                  |
| `data_nascimento` deve ser anterior a hoje                    | Validação no aggregate                  |
| Senha padrão = código do estudante                            | Ex: `ABC1234` acede com senha `ABC1234` |
| Status superior exige fundamntal e médio finalizados/inativos | Progressão lógica do ensino             |
| Notas e faltas são imutáveis após criação                     | Só podem ser criadas e consultadas      |


### POST /academia/estudante/register

Cadastra um novo estudante vinculado à academia autenticada. O cadastro direto usa `multipart/form-data` e aplica a mesma política documental da solicitação de matrícula para campos textuais e PDFs. JSON puro não é aceito. Para estudantes escolares/fundamental/médio, exige BI textual/PDF do encarregado e BI textual/PDF do estudante ou cédula; para ensino superior, exige BI textual/PDF do estudante e mantém o encarregado opcional.

**Proteção**: autenticado + academia ativa

**Content-Type:** `multipart/form-data`

**Campos de texto:**

| Campo | Obrigatório | Observações |
| --- | --- | --- |
| `nome` | sim | Nome completo válido. |
| `genero` | sim | `masculino` ou `feminino`. |
| `data_nascimento` | sim | Data simples `YYYY-MM-DD`, anterior à data atual. |
| `email` | não | Validado quando informado. |
| `telefone` | condicional | Obrigatório no ensino superior. Opcional para escolar/fundamental/médio. Quando enviado, deve ter 9 dígitos locais e não pode ser igual a `telefone_encarregado`. |
| `telefone_encarregado` | condicional | Obrigatório para escolar/fundamental/médio. Opcional no ensino superior. Quando enviado, deve ter 9 dígitos locais e não pode ser igual a `telefone`. |
| `bilhete_identidade` | condicional | Obrigatório no ensino superior; para escolar/fundamental/médio é obrigatório quando o estudante usa BI próprio em vez de cédula. Deve ser único entre estudantes. |
| `bilhete_identidade_encarregado` | condicional | Obrigatório para estudante escolar/fundamental/médio; opcional no ensino superior. Não pode ser igual ao BI do estudante após normalização nem coincidir com o BI principal de outro estudante escolar/fundamental/médio. |
| `ano_escolar_fundamental` | condicional | Ano fundamental canônico, quando aplicável. |
| `ano_escolar_medio` | condicional | Ano médio canônico, quando aplicável. |
| `curso_medio_id` | condicional | UUID de curso médio ativo da academia, quando o ano médio for informado. |
| `ano_superior` | condicional | Ano superior canônico, quando aplicável. |
| `curso_superior_id` | condicional | UUID de curso superior ativo da academia, quando o ano superior for informado. |
| `codigo_turma` | não | Código da turma ativa da mesma academia para vincular o estudante imediatamente após o cadastro. A existência, status e compatibilidade com ano/curso são validadas antes de qualquer upload ou gravação. |

**Ficheiros PDF aceitos:**

| Campo de arquivo | Regra |
| --- | --- |
| `bi_encarregado` | Obrigatório para escolar/fundamental/médio; opcional no ensino superior. |
| `bi_estudante` | Obrigatório no ensino superior e obrigatório no escolar quando `bilhete_identidade` do estudante for informado. |
| `cedula_estudante` | Obrigatória para estudante escolar/fundamental/médio sem BI próprio. |
| `declaracao` | PDF da declaração acadêmica. Obrigatória nos anos escolares com ano anterior quando não houver certificado substitutivo válido; exige o campo textual `declaracao_ano_academico` com o ano imediatamente anterior. |
| `declaracao_ano_academico` | Campo textual obrigatório quando `declaracao` for enviada para cumprir comprovativo acadêmico; exemplos: `1_ano_fundamental` para ingresso no `2_ano_fundamental`, `6_ano_fundamental` para ingresso no `7_ano_fundamental`, `9_ano_fundamental` para ingresso no `1_ano_medio`, `3_ano_medio` para ingresso no `1_ano_superior`. O valor também define o `tipo`, `nivel`, `ano_academico`, chave lógica e path do documento persistido. |
| `certificado_6_ano_fundamental` | Exigido como alternativa à declaração somente para `7_ano_fundamental`. |
| `certificado_9_ano_fundamental` | Exigido como alternativa à declaração somente para `1_ano_medio`. |
| `certificado_ensino_medio` | Exigido como alternativa à declaração somente para `1_ano_superior`. |

Quando enviados, todos os ficheiros devem ter `Content-Type: application/pdf`, extensão `.pdf`, assinatura `%PDF` e tamanho máximo de 10MB. O cadastro direto usa a mesma validação compartilhada de matrícula aplicada por `POST /solicitacao-matricula` para dados comuns e documentos. Os documentos obrigatórios são validados e enviados para `{codigo_academia}/estudantes/{codigo_estudante}/documentos/` antes de qualquer gravação no ledger; somente após sucesso total dos uploads o evento `EstudanteCriadoComVinculo` é persistido com metadados normalizados em `documentos.<chave>.documento_id`, `documentos.<chave>.tipo`, `documentos.<chave>.nivel`, `documentos.<chave>.ano_academico`, `documentos.<chave>.versao`, `documentos.<chave>.path`, `documentos.<chave>.file_url` e `documentos.<chave>.download_url`. Para identificação, a chave continua sendo o tipo do arquivo (`bi_estudante`, `bi_encarregado`, `cedula_estudante`); para documentos acadêmicos, a chave segue `nivel.ano_academico.tipo`, como `medio.3_ano_medio.declaracao_3_ano_medio`. Se validação/upload falhar, nenhum estudante/vínculo é gravado; se a criação falhar após upload parcial, o backend remove o diretório definitivo do estudante para evitar ficheiros órfãos.

**Request — campos de texto principais (exemplo escolar; requer anexar os PDFs obrigatórios do quadro acima):**

```text
nome=João Silva
genero=masculino
data_nascimento=2010-05-20
telefone=923000000
telefone_encarregado=924000000
bilhete_identidade=001234567LA089
bilhete_identidade_encarregado=009876543LA089
ano_escolar_fundamental=7_ano_fundamental
codigo_turma=TURMA-A
```

**Request — multipart/form-data com documentos obrigatórios:**

```text
nome=João Silva
genero=masculino
data_nascimento=2010-05-20
telefone=923000000
telefone_encarregado=924000000
bilhete_identidade=001234567LA089
bilhete_identidade_encarregado=009876543LA089
ano_escolar_fundamental=7_ano_fundamental
bi_estudante=@./bi_estudante.pdf;type=application/pdf
bi_encarregado=@./bi_encarregado.pdf;type=application/pdf
declaracao=@./declaracao.pdf;type=application/pdf
declaracao_ano_academico=6_ano_fundamental
```

**Exemplo cURL sem documentos:**

```bash
curl -X POST https://api.exemplo.ao/academia/estudante/register \
  -H "Authorization: Bearer <jwt_academia>" \
  -F "nome=João Silva" \
  -F "genero=masculino" \
  -F "data_nascimento=2010-05-20" \
  -F "telefone=923000000" \
  -F "telefone_encarregado=924000000" \
  -F "bilhete_identidade=001234567LA089" \
  -F "bilhete_identidade_encarregado=009876543LA089" \
  -F "ano_escolar_fundamental=1_ano_fundamental"
```

**Exemplo cURL com declaração do ano anterior:**

```bash
curl -X POST https://api.exemplo.ao/academia/estudante/register \
  -H "Authorization: Bearer <jwt_academia>" \
  -F "nome=João Silva" \
  -F "genero=masculino" \
  -F "data_nascimento=2010-05-20" \
  -F "telefone=923000000" \
  -F "telefone_encarregado=924000000" \
  -F "bilhete_identidade=001234567LA089" \
  -F "bilhete_identidade_encarregado=009876543LA089" \
  -F "ano_escolar_fundamental=7_ano_fundamental" \
  -F "bi_estudante=@./bi_estudante.pdf;type=application/pdf" \
  -F "declaracao=@./declaracao.pdf;type=application/pdf" \
  -F "declaracao_ano_academico=6_ano_fundamental"
```

**Status na criação:** o cadastro cria o vínculo ativo com a academia. Por padrão, `status = "ativo"`, `status_escolar_fundamental = "em_andamento"`, `status_escolar_medio = "inativo"` e `status_superior = "inativo"`. Depois do cadastro, alterações de status acontecem somente por endpoints de acontecimentos.

**Response 201:**

```json
{
  "message": "estudante registrado com sucesso",
  "data": {
    "id": "uuid",
    "codigo_estudante": "ABC1234",
    "codigo_academia": "LDA20261",
    "status": "ativo",
    "codigo_turma": "TURMA-A",
    "turma_vinculada": true,
    "documentos": {
      "bi_encarregado": {
        "path": "LDA20261/estudantes/ABC1234/documentos/bi_encarregado_ABC1234.pdf",
        "file_url": "https://...",
        "download_url": "https://..."
      }
    }
  }
}
```

Quando `codigo_turma` é informado e a vinculação pós-criação falha por uma condição concorrente rara, a resposta continua `201` porque o estudante já foi persistido, mas `data.turma_vinculada` vem `false` e `data.turma_aviso` orienta tentar novamente via `POST /academia/turma/:codigo/estudante`. Se `codigo_turma` não for informado, os campos `codigo_turma`, `turma_vinculada` e `turma_aviso` não aparecem na resposta.

**Erros:**

- `400` — `Content-Type` diferente de `multipart/form-data`
- `400` — genero inválido, data_nascimento inválida ou no futuro
- `400` — ano académico em formato incorreto ou incompatível com a academia/curso
- `400` — ficheiro não PDF, sem assinatura `%PDF`, com extensão diferente de `.pdf` ou acima de 10MB
- `400` — BI do estudante igual ao BI do encarregado, ou BI do estudante já cadastrado
- `400` — turma informada está inativa/deletada ou é incompatível com ano/curso do estudante
- `404` — `codigo_turma` informado não existe ou não pertence à academia autenticada

---


### POST /academia/estudante/register/async

Cadastra estudantes em lote. O campo `com_arquivo` é obrigatório e define o contrato da requisição. O endpoint não aceita formatos legados: JSON usa `com_arquivo: false`; `multipart/form-data` usa `com_arquivo=true`.

**Modo JSON sem arquivos (`application/json`)**

```json
{
  "com_arquivo": false,
  "estudantes": [
    {
      "nome": "João Silva",
      "genero": "masculino",
      "data_nascimento": "2010-05-20",
      "telefone_encarregado": "924000000",
      "bilhete_identidade_encarregado": "009876543LA089",
      "ano_escolar_fundamental": "1_ano_fundamental",
      "codigo_turma": "TURMA-A"
    }
  ]
}
```

Neste modo a requisição retorna imediatamente `202 Accepted` e cria um job de background, igual aos demais endpoints `/async` em lote. Use `poll_url` (`GET /jobs/:id`) ou `sse_url` (`GET /jobs/stream`) para acompanhar progresso, desempenho e resultados item a item. Durante o processamento são validados somente os campos textuais pelas mesmas regras de `POST /academia/estudante/register`, sem cobrança de PDFs. Cada estudante criado fica com `status = "pendente_documentos"` e não deve ser tratado como ativo até concluir a documentação pela rota posterior. Quando um item inclui `codigo_turma`, a validação e a vinculação acontecem independentemente para aquele item, sem depender da ordem de processamento dos demais estudantes do lote, e o resultado individual pode trazer `codigo_turma`, `turma_vinculada` e `turma_aviso`. Envio de arquivos com `com_arquivo: false` ou `com_arquivo` ausente/inválido é rejeitado.

**Modo com arquivos (`multipart/form-data`)**

Campos:

- `com_arquivo=true`;
- `estudantes`: JSON array com os mesmos campos textuais, `codigo_turma` opcional por item e um `codigo_temporario` único por estudante;
- arquivos nomeados como `<codigo_temporario>.<campo_documental>`, por exemplo `tmp-1.bi_estudante` e `tmp-1.bi_encarregado`.

```bash
curl -X POST https://api.exemplo.ao/academia/estudante/register/async \
  -H "Authorization: Bearer <jwt_academia>" \
  -F 'com_arquivo=true' \
  -F 'estudantes=[{"codigo_temporario":"tmp-1","nome":"João Silva","genero":"masculino","data_nascimento":"2010-05-20","telefone_encarregado":"924000000","bilhete_identidade_encarregado":"009876543LA089","ano_escolar_fundamental":"1_ano_fundamental"}]' \
  -F 'tmp-1.bi_encarregado=@./bi_encarregado.pdf;type=application/pdf' \
  -F 'tmp-1.cedula_estudante=@./cedula.pdf;type=application/pdf'
```

Arquivos órfãos, `codigo_temporario` duplicado, campos documentais desconhecidos, documentos ausentes obrigatórios e PDFs inválidos seguem as mesmas validações documentais do cadastro singular/solicitação de matrícula.

Se os dados textuais e os PDFs forem válidos, mas o armazenamento externo falhar durante o upload dos documentos (por exemplo erro transitório do Mega, timeout ou resposta JSON incompleta), o item do lote não é perdido. O backend remove a pasta parcial do estudante, conclui o cadastro textual com `status = "pendente_documentos"` e retorna o `codigo_estudante` no item correspondente para permitir repescagem. Nesse caso, a academia deve reenviar os documentos pela rota `POST /academia/estudante/{codigo_estudante}/documentos`; o estudante não deve ser tratado como ativo até a documentação ser concluída.

**Response:** nos modos JSON sem arquivos e multipart com arquivos retorna `202 Accepted` com `{job_id, total_items, status, poll_url, sse_url}`. Os resultados de cada estudante ficam disponíveis no acompanhamento do job. Itens salvos por fallback de falha de storage contam como sucesso de cadastro, mas aparecem com `status = "pendente_documentos"` e `documentos_faltantes` no resultado do item.

### POST /academia/estudante/{codigo_estudante}/documentos

Carrega posteriormente os documentos de estudante cadastrado em lote JSON com `status = "pendente_documentos"`. Aceita apenas `multipart/form-data` com os mesmos campos de arquivo de `POST /academia/estudante/register`. A rota valida documentos com a política compartilhada de matrícula/cadastro direto, armazena em `{codigo_academia}/estudantes/{codigo_estudante}/documentos/` e só grava o evento de conclusão quando todos os documentos obrigatórios estiverem válidos.

A cobrança de Bilhete de Identidade respeita os dados textuais já cadastrados: se houver somente BI textual do encarregado, exige somente `bi_encarregado`; se houver somente BI textual do estudante, exige somente `bi_estudante`; se ambos existirem, exige ambos; outras obrigatoriedades condicionais existentes continuam aplicáveis. Estudantes ativos, inativos, inexistentes ou de outra academia são rejeitados.

```bash
curl -X POST https://api.exemplo.ao/academia/estudante/ABC1234/documentos \
  -H "Authorization: Bearer <jwt_academia>" \
  -F 'bi_encarregado=@./bi_encarregado.pdf;type=application/pdf' \
  -F 'cedula_estudante=@./cedula.pdf;type=application/pdf'
```

**Response 200:** retorna `codigo_estudante`, `status: "ativo"` e o mapa `documentos`.

---

### GET /estudantes

Lista estudantes. Retorna apenas os da academia (para academia) ou todos (para admin).

**Proteção**: autenticado + academia ou admin

**Query Params:**

- `genero` — filtro por gênero (`masculino`, `feminino`). Aceita múltiplos valores (`?genero=masculino,feminino` ou repetindo o parâmetro).
- `status` — filtro pelo status geral do estudante (`ativo`, `inativo`, `pendente_documentos`; aceita múltiplos). Corresponde ao campo `EstudanteDTO.status`.
- `idade_min` — idade mínima (inteiro >= 0).
- `idade_max` — idade máxima (inteiro >= 0).
- `ano_escolar_fundamental` — filtro por ano do fundamental (aceita múltiplos).
- `ano_escolar_medio` — filtro por ano do médio (aceita múltiplos).
- `ano_superior` — filtro por ano superior (aceita múltiplos).
- `semestre_atual` — filtro pelo semestre sequencial atual do estudante superior (inteiro >= 1; aceita múltiplos).
- `curso_id` — filtro por UUID de curso médio ou superior usando o mesmo parâmetro para os dois vínculos (`curso_medio_id` ou `curso_superior_id`; aceita múltiplos). Alias aceito: `curso`.
- `codigo_academia` — filtro por código da academia (aceita múltiplos; disponível para admin).
- `status_escolar_fundamental` — filtro por status escolar no fundamental (`inativo`, `em_andamento`, `finalizado`; aceita múltiplos).
- `status_escolar_medio` — filtro por status escolar no médio (`inativo`, `em_andamento`, `finalizado`; aceita múltiplos).
- `status_superior` — filtro por status no superior (`inativo`, `em_andamento`, `finalizado`; aceita múltiplos).
- `turno` — filtro por turno da turma (`manha`, `tarde`, `noite`; aceita múltiplos).
- `codigo_turma` — filtra estudantes de turma(s) específica(s) via código da turma (aceita múltiplos).
- `com_turma` — booleano (`true`/`false`) para filtrar estudantes com ou sem turma.
- `limit` — quantidade máxima por página (padrão: 50, teto fixo: 100).
- `offset` — deslocamento de paginação (padrão: 0).

> Os filtros acima são **combináveis** entre si (AND), permitindo consultas compostas.
> Exemplos:
> - `GET /estudantes?status=ativo,pendente_documentos&genero=feminino&idade_min=12&idade_max=15&turno=manha`
> - `GET /estudantes?status_escolar_medio=em_andamento&codigo_turma=TURMA-10A&com_turma=true`
> - `GET /estudantes?codigo_academia=LDA20261&semestre_atual=1,2&curso_id=550e8400-e29b-41d4-a716-446655440000`


**Request:** sem payload
**Response 200:**

```json
{
  "estudantes": [EstudanteDTO],
  "total": 50,
  "total_geral": 375,
  "tipo_usuario": "academia",
  "codigo_academia": "LDA20261",
  "nome_academia": "string",
  "limit": 50,
  "offset": 0
}
```

`total` é a quantidade de estudantes retornados na página atual. `total_geral` é a contagem total de estudantes no escopo do usuário e filtros aplicados, ignorando `limit`/`offset`.

**Erros de validação (400):**

- `com_turma` inválido (deve ser `true` ou `false`).
- `semestre_atual` inválido (deve ser inteiro >= 1).
- `status` inválido (deve ser `ativo`, `inativo` ou `pendente_documentos`).
- `curso_id` inválido (deve ser UUID).
- `idade_min` inválida.
- `idade_max` inválida.

---

### GET /consultar-estudante/:codigo

Consulta um estudante por código. Quando o estudante possui documentos, a resposta inclui `documentos` com `path`, `file_url` e `download_url`; o `download_url` é sempre uma rota autenticada do backend (`/documentos/estudantes/{codigo_estudante}/{campo}/download`) para permitir que o cliente baixe o PDF sem depender de links diretos do storage.

**Proteção**: autenticado + academia (apenas próprios) ou admin

**Path Params:**

- `codigo` — código do estudante (ex: `ABC1234`)


**Request:** sem payload
**Response 200:**

```json
{
  "estudante": {
    ... (EstudanteDTO) ...,
    "academia": {
      "codigo": "LDA20261",
      "nome": "string",
      "nivel": "escola",
      "type": "public"
    },
    "curso_medio": {
      "id": "uuid",
      "nome": "string",
      "type": "medio",
      "status": "ativo"
    },
    "curso_superior": null,
    "documentos": {
      "bi_estudante": {
        "path": "LDA20261/estudantes/ABC1234/documentos/bi_estudante_ABC1234.pdf",
        "file_url": "LDA20261/estudantes/ABC1234/documentos/bi_estudante_ABC1234.pdf",
        "download_url": "/documentos/estudantes/ABC1234/bi_estudante/download"
      }
    }
  }
}
```

**Erros:**

- `403` — estudante não pertence à academia autenticada
- `404` — estudante não encontrado

---

### Solicitações de edição de dados sensíveis

A rota genérica `PUT /estudante/dados-pessoais` foi removida. Dados civis sensíveis do estudante não possuem endpoint genérico de edição: `nome`, `bilhete_identidade`, `bilhete_identidade_encarregado` e `data_nascimento` só podem ser solicitados pelas rotas dedicadas abaixo e aplicados pela academia vinculada após aprovação documentada.

A entidade `SolicitacaoEdicaoDadoEstudante` registra `codigo_solicitacao`, `codigo_estudante`, `codigo_academia`, `campo`, `valor_atual`, `valor_solicitado`, `documento_temporario_path`, `documento_temporario_url`, `status`, `motivo_reprovacao`, `solicitado_por`, `decidido_por`, `created_at`, `updated_at` e `version`. Nas respostas GET, o backend também monta o objeto derivado `documento`, com metadados e rota autenticada de download, para o cliente ler/baixar o PDF sem usar links diretos do storage. Existe no máximo uma solicitação `pendente` por estudante e campo.

Eventos gravados no ledger seguro para este fluxo:

- `SolicitacaoEdicaoDadoEstudanteCriada`
- `SolicitacaoEdicaoDadoEstudanteAprovada`
- `SolicitacaoEdicaoDadoEstudanteReprovada`
- `NomeEstudanteAlteradoPorSolicitacao`
- `BilheteIdentidadeEstudanteAlteradoPorSolicitacao`
- `BilheteIdentidadeEncarregadoAlteradoPorSolicitacao`
- `DataNascimentoEstudanteAlteradaPorSolicitacao`
- `TelefoneEncarregadoAlterado`

**Regras documentais comuns às solicitações:** o campo `documento` é obrigatório, deve ser PDF (`Content-Type: application/pdf`, extensão `.pdf`, assinatura `%PDF`) e ter no máximo 10MB por arquivo. O arquivo é salvo temporariamente em `{codigo_academia}/estudantes/{codigo_estudante}/edicoes_dados_pendentes/{campo}_{codigo_solicitacao}.pdf` e é removido depois de aprovação ou reprovação. Não há aliases, wrappers ou endpoint genérico que aceite `campo` arbitrário.

---

#### PUT /estudante/encarregado/telefone

Atualiza exclusivamente o telefone do encarregado do estudante autenticado.

**Proteção**: autenticado + estudante

**Request:**

```json
{
  "telefone_encarregado": "923456789"
}
```

**Regras de negócio:**

- O estudante é identificado exclusivamente pelo token; o payload não aceita `codigo_estudante`, `academia_id`, `codigo_academia` nem seletores de alvo.
- `telefone_encarregado` é obrigatório e deve conter exatamente 9 dígitos nacionais, sem DDI, espaços, hífens, parênteses ou letras.
- A rota altera apenas `telefone_encarregado`. Quando o valor muda de fato, `telefone_encarregado_verificado` volta para `false`.
- Qualquer campo extra no JSON retorna `400` e nenhuma mutação é gravada.

**Response 200:**

```json
{
  "message": "telefone do encarregado atualizado com sucesso"
}
```

**Erros:**

- `400` — telefone vazio/inválido ou campo extra no payload
- `401/403` — token ausente, inválido ou usuário não estudante

---

#### GET /estudante/solicitacoes-edicao

Lista as solicitações de edição de dados sensíveis criadas pelo estudante autenticado.

Cada item retornado inclui `documento`, no mesmo formato dos documentos de matrícula, com `path`, `file_url` e `download_url` para que o estudante possa abrir/baixar o PDF pelo backend. No escopo do estudante, o `download_url` aponta para `/estudante/solicitacoes-edicao/{codigo_solicitacao}/documento/download`.

**Proteção**: autenticado + estudante

**Query Params:**

- `status` — filtro opcional por `pendente`, `aprovada` ou `reprovada`
- `campo` — filtro opcional por `nome`, `bilhete_identidade`, `bilhete_identidade_encarregado` ou `data_nascimento`
- `limit` — quantidade máxima por página (padrão 50, teto 100)
- `offset` — deslocamento da paginação (padrão 0)

**Request:** sem payload

**Response 200:**

```json
{
  "solicitacoes": [
    {
      "codigo_solicitacao": "SED12345678",
      "codigo_estudante": "EST12345678",
      "codigo_academia": "ACA12345678",
      "campo": "nome",
      "valor_atual": "Nome Atual",
      "valor_solicitado": "Nome Corrigido",
      "documento_temporario_path": "ACA12345678/estudantes/EST12345678/edicoes_dados_pendentes/nome_SED12345678.pdf",
      "documento_temporario_url": "string",
      "documento": {
        "tipo": "documento",
        "path": "ACA12345678/estudantes/EST12345678/edicoes_dados_pendentes/nome_SED12345678.pdf",
        "file_url": "string",
        "download_url": "/estudante/solicitacoes-edicao/SED12345678/documento/download"
      },
      "status": "pendente",
      "motivo_reprovacao": null,
      "solicitado_por": "EST12345678",
      "decidido_por": null,
      "created_at": "2026-07-24T00:00:00Z",
      "updated_at": "2026-07-24T00:00:00Z",
      "version": 1
    }
  ],
  "limit": 50,
  "offset": 0,
  "total": 1
}
```

**Regras de negócio:** a listagem nunca aceita seletor de outro estudante; o escopo é sempre o estudante autenticado.

---

#### POST /estudante/solicitacoes-edicao/nome

Cria solicitação documentada para alterar o nome do estudante autenticado.

**Proteção**: autenticado + estudante

**Request:** `multipart/form-data`

- `novo_valor` — nome pretendido, obrigatório, com trim e tamanho compatível com o padrão de nomes do sistema
- `documento` — PDF comprovativo obrigatório

**Response 201:**

```json
{
  "message": "solicitação criada com sucesso",
  "codigo_solicitacao": "SED12345678",
  "campo": "nome",
  "status": "pendente"
}
```

**Regras de negócio:** rejeita estudante sem academia vinculada, valor igual ao vigente e segunda solicitação pendente ou concorrente para `nome` com `409`. O payload não pode escolher estudante, academia ou campo diferente da rota.

---

#### POST /estudante/solicitacoes-edicao/bilhete-identidade

Cria solicitação documentada para alterar o bilhete de identidade do estudante autenticado.

**Proteção**: autenticado + estudante

**Request:** `multipart/form-data`

- `novo_valor` — novo `bilhete_identidade`, obrigatório e validado pelo validador atual de BI/NIF/identificadores aplicável ao estudante
- `documento` — PDF comprovativo obrigatório

**Response 201:** igual ao de criação, com `campo = "bilhete_identidade"`.

**Regras de negócio:** rejeita valor igual ao vigente, BI já usado por outro estudante, estudante sem academia vinculada, documento inválido e segunda solicitação pendente ou concorrente para o campo com `409`.

---

#### POST /estudante/solicitacoes-edicao/bilhete-identidade-encarregado

Cria solicitação documentada para alterar o bilhete de identidade do encarregado.

**Proteção**: autenticado + estudante

**Request:** `multipart/form-data`

- `novo_valor` — novo `bilhete_identidade_encarregado`, obrigatório e validado pelo padrão atual de BI
- `documento` — PDF comprovativo obrigatório

**Response 201:** igual ao de criação, com `campo = "bilhete_identidade_encarregado"`.

**Regras de negócio:** rejeita valor igual ao vigente, estudante sem academia vinculada, documento inválido e segunda solicitação pendente ou concorrente para o campo com `409`.

---

#### POST /estudante/solicitacoes-edicao/data-nascimento

Cria solicitação documentada para alterar a data de nascimento do estudante autenticado.

**Proteção**: autenticado + estudante

**Request:** `multipart/form-data`

- `novo_valor` — nova data no formato `YYYY-MM-DD`, obrigatória e validada pelas regras atuais de idade/coerência temporal
- `documento` — PDF comprovativo obrigatório

**Response 201:** igual ao de criação, com `campo = "data_nascimento"`.

**Regras de negócio:** rejeita data igual à vigente, formato inválido, estudante sem academia vinculada, documento inválido e segunda solicitação pendente ou concorrente para o campo com `409`.

---

### Endpoints de acontecimentos que alteram status do estudante

Os status do estudante não devem ser editados diretamente por payloads genéricos. Nesta API, eles são derivados de acontecimentos reais do domínio acadêmico e gravados no ledger do estudante, preservando auditoria, hash chain e histórico acadêmico. Use estas rotas quando a academia precisar registrar um fato operacional que altera a situação do vínculo ou da etapa acadêmica do estudante.

**Proteção de todas as rotas deste escopo:** autenticado + academia + academia ativa. O estudante informado em `:codigo` precisa existir e pertencer à academia autenticada; caso contrário, a API retorna erro de permissão ou de não encontrado.

**Regras gerais do escopo:**

- Cada operação registra um evento de domínio no ledger do estudante, com o usuário academia que executou a ação e o IP da requisição.
- Interrupções, desvinculações e reintegrações não apagam notas, faltas, avaliações, turmas, documentos ou demais registros históricos.
- `motivo` é obrigatório para interrupção, desvinculação e revinculação, e não pode ser vazio.
- `curso_id`, `curso_medio_id` e `curso_superior_id`, quando enviados, precisam ser UUIDs válidos de cursos existentes e do tipo correto (`medio` ou `superior`).
- Os anos escolares aceitos são validados pelo backend: fundamental usa `1_ano_fundamental` até `9_ano_fundamental`; médio usa o formato numérico `[n]_ano_medio`, como `1_ano_medio`.

#### Fluxo de solicitações para interrupção, desvinculação e revinculação

As operações sensíveis de status acadêmico exigem participação explícita do estudante. O estudante cria uma solicitação autenticada e a academia apenas decide uma solicitação pendente válida; a aprovação é o único momento em que eventos de alteração de status são gravados. Não há suporte legado para as rotas de matrícula de etapa, interrupção por nível ou trancamento superior porque o banco está vazio.

##### `GET /estudante/solicitacoes`

Lista as solicitações de status acadêmico criadas pelo estudante autenticado, permitindo acompanhar pedidos pendentes, aprovados ou reprovados.

**Autorização:** estudante autenticado.

**Response 200:**

```json
{
  "solicitacoes": [
    {
      "codigo_solicitacao": "SSA12345678",
      "codigo_estudante": "EST12345678",
      "codigo_academia": "ACA12345678",
      "tipo": "interrupcao",
      "status": "pendente",
      "motivo": "mudança temporária de cidade",
      "tipo_ensino": "fundamental",
      "motivo_reprovacao": null,
      "observacao_academia": null,
      "created_at": "2026-07-20T00:00:00Z",
      "updated_at": "2026-07-20T00:00:00Z",
      "decidida_at": null
    }
  ],
  "total": 1
}
```

##### `POST /estudante/solicitacoes-status/interrupcao`

Cria uma solicitação para interromper o percurso acadêmico atualmente em andamento do estudante autenticado na academia à qual ele está vinculado.

**Autorização:** estudante autenticado.

**Body:**

```json
{
  "motivo": "mudança temporária de cidade"
}
```

**Regras:**

- `motivo` é obrigatório e não pode ficar vazio após `trim`.
- A academia da solicitação é a academia atual do estudante.
- Só pode existir uma solicitação pendente de `interrupcao` para o mesmo estudante na mesma academia.
- A solicitação não altera status acadêmico por si só; o evento de interrupção só é emitido quando a academia aprova.

**Response 201:**

```json
{
  "message": "solicitação criada com sucesso",
  "codigo_solicitacao": "SSA12345678",
  "status": "pendente"
}
```

##### `POST /estudante/solicitacoes-status/desvinculacao`

Cria uma solicitação para desvincular o estudante autenticado da academia atual.

**Autorização:** estudante autenticado.

**Body:**

```json
{
  "motivo": "transferência solicitada pelo estudante"
}
```

**Regras:**

- `motivo` é obrigatório e não pode ficar vazio após `trim`.
- A academia da solicitação é a academia atual do estudante.
- Só pode existir uma solicitação pendente de `desvinculacao` para o mesmo estudante na mesma academia.
- A solicitação não desvincula o estudante; `EstudanteDesvinculadoDaAcademia` só é gravado na aprovação pela academia.

**Response 201:**

```json
{
  "message": "solicitação criada com sucesso",
  "codigo_solicitacao": "SSA12345678",
  "status": "pendente"
}
```

##### `POST /estudante/solicitacoes-status/revinculacao/:codigo_academia`

Cria uma solicitação para revincular o estudante autenticado à academia indicada em `:codigo_academia`.

**Autorização:** estudante autenticado.

**Path params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---:|:---:|---|
| `codigo_academia` | string | sim | Código público da academia que deverá decidir o pedido de retorno. |

**Body:**

```json
{
  "motivo": "retorno à instituição anterior",
  "tipo_ensino": "fundamental",
  "curso_medio_id": null,
  "curso_superior_id": null
}
```

**Regras:**

- `motivo` é obrigatório e não pode ficar vazio após `trim`.
- `tipo_ensino`, quando necessário para a retomada, deve ser `fundamental`, `medio` ou `superior`.
- Para médio, `curso_medio_id` pode indicar novo curso pretendido; se omitido, a aprovação deve reutilizar o curso médio anterior válido naquela academia.
- Para superior, `curso_superior_id` pode indicar novo curso pretendido; se omitido, a aprovação deve reutilizar o curso superior anterior válido naquela academia.
- Só pode existir uma solicitação pendente de `revinculacao` para o mesmo estudante na mesma academia.
- A solicitação não reativa o estudante; `EstudanteReintegrado` só é gravado na aprovação pela academia.

**Response 201:**

```json
{
  "message": "solicitação criada com sucesso",
  "codigo_solicitacao": "SSA12345678",
  "status": "pendente"
}
```

##### `GET /academia/solicitacoes`

Lista solicitações de status acadêmico recebidas pela academia autenticada. Administradores também podem consultar as solicitações de uma academia informando `codigo_academia` na query string.

**Autorização:** academia autenticada e ativa, ou administrador autenticado.

**Query params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---:|:---:|---|
| `codigo_academia` | string | somente admin | Código público da academia consultada pelo administrador. Academias autenticadas ignoram este parâmetro e sempre consultam a própria academia. |

**Response 200:**

```json
{
  "solicitacoes": [
    {
      "codigo_solicitacao": "SSA12345678",
      "codigo_estudante": "EST12345678",
      "codigo_academia": "ACA12345678",
      "tipo": "interrupcao",
      "status": "pendente",
      "motivo": "mudança temporária de cidade",
      "tipo_ensino": "fundamental",
      "motivo_reprovacao": null,
      "observacao_academia": null,
      "created_at": "2026-07-20T00:00:00Z",
      "updated_at": "2026-07-20T00:00:00Z",
      "decidida_at": null
    }
  ],
  "total": 1
}
```

##### `POST /academia/estudante/:codigo/interromper/percurso-academico`

Aprova uma solicitação pendente de interrupção de percurso acadêmico do estudante indicado em `:codigo`.

**Autorização:** academia autenticada e ativa.

**Path params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---:|:---:|---|
| `codigo` | string | sim | Código do estudante vinculado à academia e dono da solicitação. |

**Body:**

```json
{
  "solicitacao_id": "SSA12345678",
  "observacao_academia": "documentação conferida"
}
```

**Regras:**

- `solicitacao_id` é obrigatório e deve apontar para uma solicitação `interrupcao` pendente.
- A solicitação deve pertencer à academia autenticada e ao estudante informado na rota.
- Deve existir exatamente uma etapa acadêmica em andamento.
- A academia não altera o `motivo` original do estudante; `observacao_academia` é campo separado.
- Na aprovação, o backend identifica a etapa em andamento e emite `FundamentalInterrompido`, `MedioInterrompido` ou `SuperiorInterrompido`, incluindo referência da solicitação aprovada.

**Response 200:**

```json
{
  "message": "solicitação aprovada",
  "codigo_solicitacao": "SSA12345678"
}
```

##### `POST /academia/estudante/:codigo/interromper/percurso-academico/reprovar`

Reprova uma solicitação pendente de interrupção sem alterar o status acadêmico do estudante.

**Autorização:** academia autenticada e ativa.

**Body:**

```json
{
  "solicitacao_id": "SSA12345678",
  "motivo_reprovacao": "documentação insuficiente"
}
```

**Regras:**

- `solicitacao_id` e `motivo_reprovacao` são obrigatórios.
- A solicitação deve estar `pendente`, pertencer à academia autenticada e ao estudante informado na rota.
- A reprovação é terminal e não grava evento de alteração de status acadêmico.

**Response 200:**

```json
{
  "message": "solicitação reprovada"
}
```

##### `POST /academia/estudante/:codigo/desvincular`

Aprova uma solicitação pendente de desvinculação do estudante indicado em `:codigo`.

**Autorização:** academia autenticada e ativa.

**Body:**

```json
{
  "solicitacao_id": "SSA12345678",
  "observacao_academia": "pedido validado"
}
```

**Regras:**

- `solicitacao_id` é obrigatório e deve apontar para uma solicitação `desvinculacao` pendente.
- A solicitação deve pertencer à academia autenticada e ao estudante informado na rota.
- A academia não consegue desvincular sem solicitação pendente válida.
- A aprovação grava `EstudanteDesvinculadoDaAcademia`, preserva histórico acadêmico e define `status = "inativo"`.
- O evento registra o nível acadêmico calculado no momento da saída e a referência da solicitação aprovada.

**Response 200:**

```json
{
  "message": "solicitação aprovada",
  "codigo_solicitacao": "SSA12345678"
}
```

##### `POST /academia/estudante/:codigo/desvincular/reprovar`

Reprova uma solicitação pendente de desvinculação sem alterar vínculo ou status geral do estudante.

**Autorização:** academia autenticada e ativa.

**Body:**

```json
{
  "solicitacao_id": "SSA12345678",
  "motivo_reprovacao": "pedido não atende às regras internas"
}
```

**Regras:**

- `solicitacao_id` e `motivo_reprovacao` são obrigatórios.
- A solicitação deve estar `pendente`, pertencer à academia autenticada e ao estudante informado na rota.
- A reprovação é terminal e não grava `EstudanteDesvinculadoDaAcademia`.

**Response 200:**

```json
{
  "message": "solicitação reprovada"
}
```

##### `POST /academia/estudante/:codigo/revincular`

Aprova uma solicitação pendente de revinculação do estudante indicado em `:codigo`.

**Autorização:** academia autenticada e ativa.

**Body:**

```json
{
  "solicitacao_id": "SSA12345678",
  "observacao_academia": "retorno autorizado"
}
```

**Regras:**

- `solicitacao_id` é obrigatório e deve apontar para uma solicitação `revinculacao` pendente.
- A solicitação deve pertencer à academia autenticada e ao estudante informado na rota.
- A academia não consegue revincular sem solicitação pendente válida.
- Apenas estudante `inativo` por desvinculação pode ser revinculado.
- A aprovação grava `EstudanteReintegrado`, define `status = "ativo"` e reativa a etapa indicada/derivada.
- A retomada deve usar a última posição acadêmica do estudante naquela mesma academia: nível, ano fundamental, ano médio, curso médio, ano superior, semestre atual e curso superior conforme aplicável.
- No fundamental, a aprovação é bloqueada quando houver progressão posterior em outra academia; o retorno só é permitido no mesmo ano do Ensino Primário e Iº Ciclo da desvinculação.
- No médio e no superior, cursos informados precisam existir, estar ativos, pertencer à academia e ter tipo compatível; se omitidos, a aprovação reutiliza o curso anterior válido daquela academia.
- O evento final inclui referência da solicitação aprovada e snapshot da posição acadêmica retomada.

**Response 200:**

```json
{
  "message": "solicitação aprovada",
  "codigo_solicitacao": "SSA12345678"
}
```

##### `POST /academia/estudante/:codigo/revincular/reprovar`

Reprova uma solicitação pendente de revinculação sem reativar o estudante.

**Autorização:** academia autenticada e ativa.

**Body:**

```json
{
  "solicitacao_id": "SSA12345678",
  "motivo_reprovacao": "retorno não autorizado neste período"
}
```

**Regras:**

- `solicitacao_id` e `motivo_reprovacao` são obrigatórios.
- A solicitação deve estar `pendente`, pertencer à academia autenticada e ao estudante informado na rota.
- A reprovação é terminal e não grava `EstudanteReintegrado`.

**Response 200:**

```json
{
  "message": "solicitação reprovada"
}
```



---

## 9. Solicitação de Matrícula

O escopo de solicitação de matrícula registra pedidos externos de ingresso em uma academia, armazena os PDFs obrigatórios, expõe a fila para análise e efetiva o estudante somente após aprovação da academia. O aggregate usado é `SolicitacaoMatricula`, com estados `pendente`, `aprovada`, `reprovada` e `cancelada`.

### 9.1 `POST /solicitacao-matricula`

Cria uma solicitação pública de matrícula para a academia informada.

**Proteção:** pública.

**Content-Type:** `multipart/form-data`.

**Request fields:**

| Campo | Tipo/envio | Obrigatório | Observações |
| --- | --- | --- | --- |
| `codigo_academia` | texto | sim | Academia destino, que deve existir e estar `ativo`. |
| `nome` | texto | sim | Nome completo do candidato. |
| `genero` | texto | sim | Validado pelas regras comuns de matrícula. |
| `data_nascimento` | texto `YYYY-MM-DD` | sim | Deve ser anterior à data atual. |
| `email` | texto | não | Normalizado/validado quando enviado. |
| `telefone` | texto | não | Telefone do candidato; normalizado/validado quando enviado. |
| `telefone_encarregado` | texto | não | Telefone do encarregado; normalizado/validado quando enviado. |
| `bilhete_identidade` | texto | não | BI do candidato; normalizado/validado quando enviado. |
| `bilhete_identidade_encarregado` | texto | não | BI do encarregado; normalizado/validado quando enviado. |
| `ano_escolar_fundamental` | texto | condicional | Use quando a solicitação for para o Ensino Primário e Iº Ciclo. |
| `ano_escolar_medio` | texto | condicional | Use quando a solicitação for para ensino médio. |
| `ano_superior` | texto | condicional | Use quando a solicitação for para ensino superior. |
| `curso_medio_id` | UUID | condicional | Curso `medio`, `ativo` e da mesma academia; usado com `ano_escolar_medio`. |
| `curso_superior_id` | UUID | condicional | Curso `superior`, `ativo` e da mesma academia; usado com `ano_superior`. |
| `declaracao_ano_academico` | texto | não | Classifica o PDF `declaracao` quando enviado. |
| `bi_estudante`, `bi_encarregado`, `cedula_estudante`, `declaracao`, `certificado_6_ano_fundamental`, `certificado_9_ano_fundamental`, `certificado_ensino_medio` | arquivo PDF | condicional | Únicos campos de arquivo aceites; a obrigatoriedade depende do percurso de matrícula. |

**Exemplo de request — ensino médio:**

```bash
curl -X POST "$BASE_URL/solicitacao-matricula" \
  -F codigo_academia=ACAD001 \
  -F nome="Ana Manuel" \
  -F genero=feminino \
  -F data_nascimento=2008-04-15 \
  -F telefone=+244923000000 \
  -F telefone_encarregado=+244922000000 \
  -F bilhete_identidade=000000000LA000 \
  -F bilhete_identidade_encarregado=111111111LA111 \
  -F ano_escolar_medio=1_ano_medio \
  -F curso_medio_id=7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55 \
  -F bi_estudante=@./bi-estudante.pdf \
  -F bi_encarregado=@./bi-encarregado.pdf \
  -F certificado_9_ano_fundamental=@./certificado-9-ano.pdf
```

**Regras de negócio:**

- Qualquer campo de arquivo fora da lista acima é rejeitado.
- Cada arquivo deve ser PDF (`Content-Type: application/pdf`, extensão `.pdf`, assinatura `%PDF`) e ter no máximo 10 MB.
- Os documentos obrigatórios são validados pelas regras automáticas de documentos de matrícula, considerando BI, BI do encarregado e ano pretendido.
- `7_ano_fundamental` exige `certificado_6_ano_fundamental`; `1_ano_medio` exige `certificado_9_ano_fundamental`; ingresso superior exige curso superior/ano compatível e certificado aplicável pelas regras de documentos.
- Para matrícula escolar, `bilhete_identidade_encarregado` não pode coincidir com o BI principal de outro estudante escolar.
- O código da solicitação é único e gerado pelo backend.
- Todos os PDFs são enviados para storage antes de gravar o evento; se algum upload falhar, os arquivos já enviados são removidos e a solicitação não é criada.
- O evento gravado é `SolicitacaoMatriculaCriada`, com `status = pendente` e lista de solicitações semelhantes pendentes quando houver.

**Response 201:**

```json
{
  "message": "solicitação de matrícula criada com sucesso",
  "codigo_solicitacao": "AB12CD34EF5",
  "codigo_academia": "ACAD001",
  "status": "pendente",
  "solicitacoes_semelhantes": ["ZX98YW76VU5"]
}
```

**Erros comuns:** `400` para payload/documentos inválidos, `403` para academia inativa ou inexistente, `500` para falhas internas/storage.

### 9.2 `GET /solicitacoes-matricula`

Lista solicitações em escopo administrativo.

**Proteção:** admin autenticado.

**Query params:**

| Parâmetro | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `codigo_academia` | string repetível | não | Restringe a uma ou mais academias, por exemplo `?codigo_academia=ACAD001&codigo_academia=ACAD002`. |
| `status` | enum repetível | não | Filtra por `pendente`, `aprovada`, `reprovada` ou `cancelada`; aceita múltiplos valores. |
| `limit` | inteiro | não | Padrão `50`, mínimo `1`, máximo `1000`. |
| `offset` | inteiro | não | Padrão `0`, mínimo `0`, máximo `1000000`. |

**Exemplo de request:**

```http
GET /solicitacoes-matricula?codigo_academia=ACAD001&status=pendente&limit=50&offset=0 HTTP/1.1
Authorization: Bearer <token>
```

**Response 200:**

```json
{
  "solicitacoes": [SolicitacaoMatriculaDTO],
  "total": 1,
  "total_geral": 25,
  "limit": 50,
  "offset": 0
}
```

**Regras de negócio:** os documentos retornam com metadados normalizados e URLs de download autenticadas. Admins podem filtrar por academia; sem filtro, a consulta retorna o escopo administrativo permitido.

### 9.3 `GET /academia/solicitacoes-matricula`

Lista solicitações da academia autenticada/resolvida.

**Proteção:** academia ativa ou admin no grupo de leitura de academia.

**Query params:** mesmos de `GET /solicitacoes-matricula`, exceto que `codigo_academia` é derivado da sessão/escopo e não deve ser usado para escapar da academia.

**Exemplo de request:**

```http
GET /academia/solicitacoes-matricula?status=pendente&limit=50&offset=0 HTTP/1.1
Authorization: Bearer <token>
```

**Response 200:** mesmo envelope de listagem de `9.2`.

**Regras de negócio:** a listagem é sempre restrita à academia corrente. Documentos recebem URLs de download no escopo de solicitação de matrícula.

### 9.4 `GET /academia/solicitacao-matricula/:codigo`

Consulta uma solicitação específica da academia.

**Proteção:** academia ativa ou admin no grupo de leitura de academia.

**Path params:**

| Parâmetro | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `codigo` | string | sim | Código único da solicitação. |

**Exemplo de request:**

```http
GET /academia/solicitacao-matricula/AB12CD34EF5 HTTP/1.1
Authorization: Bearer <token>
```

**Response 200:**

```json
{
  "solicitacao": SolicitacaoMatriculaDTO
}
```

**Regras de negócio:** retorna `404` quando o código não existe e `403` quando a solicitação pertence a outra academia. Os documentos incluem URLs autenticadas de download.

### 9.5 `PUT /academia/solicitacao-matricula/:codigo/aprovar`

Aprova uma solicitação pendente. A aprovação tem dois caminhos: cria o estudante imediatamente quando não há taxa de matrícula configurada, ou deixa a solicitação aprovada a aguardar pagamento quando existe uma configuração vigente de taxa.

**Proteção:** academia ativa.

**Request body:** vazio; não envie campos. O backend usa os dados já guardados na solicitação.

**Exemplo de request:**

```http
PUT /academia/solicitacao-matricula/AB12CD34EF5/aprovar HTTP/1.1
Authorization: Bearer <token>
```

**Regras de negócio:**

- A solicitação deve existir, pertencer à academia autenticada e estar `pendente`.
- Documentos e BI do encarregado são revalidados no momento da aprovação.
- Se a solicitação possui `bilhete_identidade`, o backend reserva a chave única antes de prosseguir; conflito de BI responde `409`.
- É gerado `codigo_estudante` em ambos os caminhos. Mesmo quando há taxa, o código fica reservado imediatamente, embora o registo do `Estudante` ainda não exista.
- **Sem taxa configurada:** quando não existe configuração vigente de taxa de matrícula para o nível/ano/curso da solicitação (ver 19.21), a aprovação cria o aggregate `Estudante` imediatamente via `EstudanteCriadoComVinculo`, grava `SolicitacaoMatriculaAprovada`, atribui a senha inicial padrão e marca solicitações concorrentes pendentes por BI como canceladas quando aplicável.
- **Com taxa configurada:** quando existe configuração vigente, a aprovação grava `SolicitacaoMatriculaAprovada` e, em seguida, `SolicitacaoMatriculaAprovadaPendentePagamento`; o status final passa a `aprovada_pendente_pagamento_matricula`. O valor e os métodos da configuração vigente no momento da aprovação ficam congelados na própria solicitação, e alterações posteriores da configuração não mudam o que o candidato tem a pagar.
- No caminho com taxa, nenhum `Estudante` é criado na aprovação: o registo só nasce quando o pagamento é confirmado (ver 9.11) ou o código reservado é libertado se a academia cancelar a solicitação pendente de pagamento (ver 9.12). O cancelamento de solicitações concorrentes por BI só ocorre no caminho sem taxa, quando o estudante é efectivamente criado.
- A operação rejeita solicitações já aprovadas, reprovadas ou canceladas com conflito.

**Response 200 — sem taxa configurada:**

```json
{
  "message": "solicitação aprovada e estudante registado com sucesso",
  "codigo_solicitacao": "AB12CD34EF5",
  "codigo_estudante_gerado": "EST123456"
}
```

**Response 200 — com taxa configurada:**

```json
{
  "message": "solicitação aprovada, aguardando pagamento de matrícula",
  "codigo_solicitacao": "AB12CD34EF5",
  "codigo_estudante_gerado": "EST123456",
  "status": "aprovada_pendente_pagamento_matricula"
}
```

**Erros comuns:** `400` para revalidação inválida, `403` para solicitação de outra academia, `404` para código inexistente, `409` para solicitação não pendente ou BI em uso/em cadastro.

Para o fluxo de consulta e pagamento da taxa pelo próprio candidato, veja as seções 9.9–9.12; para a configuração da taxa pela academia, veja a seção 19.21.

### 9.6 `PUT /academia/solicitacao-matricula/:codigo/reprovar`

Reprova uma solicitação pendente com motivo obrigatório.

**Proteção:** academia ativa.

**Request body:**

| Campo | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `motivo_reprovacao` | string | sim | Motivo legível para auditoria; não pode ser vazio após trim. |

```json
{
  "motivo_reprovacao": "documentação ilegível"
}
```

**Exemplo de request:**

```http
PUT /academia/solicitacao-matricula/AB12CD34EF5/reprovar HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json

{
  "motivo_reprovacao": "documentação ilegível"
}
```

**Regras de negócio:**

- A solicitação deve existir, pertencer à academia autenticada e estar `pendente`.
- `motivo_reprovacao` é obrigatório e não pode ser vazio.
- A reprovação grava `SolicitacaoMatriculaReprovada` e tenta remover a pasta de documentos temporários da solicitação no storage.

**Response 200:**

```json
{
  "message": "solicitação reprovada com sucesso",
  "codigo_solicitacao": "AB12CD34EF5"
}
```

### 9.7 `GET /documentos/solicitacoes-matricula/:codigo/:campo/download`

Faz download administrativo de um documento anexado a uma solicitação de matrícula.

**Proteção:** usuário autenticado.

**Path params:**

| Parâmetro | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `codigo` | string | sim | Código único da solicitação. |
| `campo` | string | sim | Chave normalizada do documento, como `bi_estudante`, `bi_encarregado` ou `medio.1_ano_medio.declaracao`. |

**Exemplo de request:**

```http
GET /documentos/solicitacoes-matricula/AB12CD34EF5/bi_estudante/download HTTP/1.1
Authorization: Bearer <token>
```

**Regras de negócio:** o backend resolve o documento pela projeção da solicitação e serve o arquivo pelo storage; links diretos externos não são fonte de verdade para o cliente.

**Response 200:** arquivo PDF.

### 9.8 `GET /academia/documentos/solicitacoes-matricula/:codigo/:campo/download`

Faz download de documento de solicitação no escopo da academia.

**Proteção:** academia ativa ou admin no grupo de leitura de academia.

**Path params:** mesmos de `9.7`.

**Exemplo de request:**

```http
GET /academia/documentos/solicitacoes-matricula/AB12CD34EF5/bi_estudante/download HTTP/1.1
Authorization: Bearer <token>
```

**Regras de negócio:** além das validações de `9.7`, a solicitação deve pertencer à academia resolvida; caso contrário retorna `403`.

**Response 200:** arquivo PDF.

Estas rotas públicas complementares existem para o próprio candidato, que ainda não tem conta no sistema, consultar e pagar a taxa de matrícula quando a academia exige uma. A posse do código da solicitação funciona como credencial; por isso, as respostas de `busca` e `status` são deliberadamente minimalistas e nunca expõem documentos nem dados pessoais completos.

### 9.9 `GET /solicitacao-matricula/busca`

Localiza solicitações por combinação exacta de dados fornecidos pelo candidato.

**Proteção:** pública, mas limitada por IP.

**Query params:**

| Campo | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `telefone` | string | não | Correspondência exacta, case-sensitive e sem normalização adicional. |
| `telefone_encarregado` | string | não | Correspondência exacta. |
| `email` | string | não | Correspondência exacta. |
| `bilhete_identidade` | string | não | Correspondência exacta. |
| `bilhete_identidade_encarregado` | string | não | Correspondência exacta. |

É preciso informar pelo menos 2 dos 5 campos para a busca ser executada.

**Exemplo de request:**

```http
GET /solicitacao-matricula/busca?telefone=%2B244923000000&email=candidato%40example.com HTTP/1.1
```

**Response 200:**

```json
{
  "solicitacoes": [
    {
      "codigo_solicitacao": "AB12CD34EF5",
      "nome_estudante": "Ana Manuel",
      "academia": "ACAD001",
      "data_submissao": "2026-08-01T10:00:00Z",
      "status": "pendente"
    }
  ]
}
```

**Regras de negócio:**

- Com menos de 2 campos preenchidos, a resposta é `200` com `"solicitacoes": []`, e não `400`, para não permitir enumeração por erro.
- Só devolve dados de reconhecimento: código, nome, academia, data e status.
- Os resultados são ordenados pela data de submissão mais recente primeiro.

**Erros comuns:** `429` ao exceder o limite próprio desta rota: até 5 requisições imediatas por IP, renovando 1 a cada 3 segundos; a resposta é corpo vazio e não segue o envelope global.

### 9.10 `GET /solicitacao-matricula/:codigo/status`

Consulta o estado mínimo de uma solicitação e, quando aplicável, os dados congelados para pagamento da taxa de matrícula.

**Proteção:** pública, com limitador por IP independente de 9.9.

**Path params:** `codigo` — código da solicitação.

**Exemplo de request:**

```http
GET /solicitacao-matricula/AB12CD34EF5/status HTTP/1.1
```

**Response 200 (solicitação não pendente de pagamento):**

```json
{ "status": "pendente", "codigo_academia": "ACAD001" }
```

**Response 200 (solicitação aguardando pagamento de matrícula):**

```json
{
  "status": "aprovada_pendente_pagamento_matricula",
  "codigo_academia": "ACAD001",
  "valor_matricula": 15000.00,
  "metodos_pagamento": ["REF", "GPO"]
}
```

**Regras de negócio:**

- `valor_matricula` e `metodos_pagamento` só aparecem quando `status` é `aprovada_pendente_pagamento_matricula`.
- Para `pendente`, `aprovada`, `reprovada` ou `cancelada`, a resposta traz apenas `status` e `codigo_academia`.
- Não expõe nome, documentos nem qualquer outro dado do candidato.

**Erros comuns:** `404` quando o código não existe ou não está disponível para pagamento de matrícula, com mensagem genérica; `429` no limite de requisições.

### 9.11 `POST /solicitacao-matricula/:codigo/pagamento-matricula`

Cria a cobrança da taxa de matrícula para uma solicitação já aprovada e aguardando pagamento.

**Proteção:** pública, com limitador por IP independente de 9.9 e 9.10.

**Path params:** `codigo` — código da solicitação.

**Request JSON:**

```json
{ "metodo_pagamento": "GPO", "telefone": "+244923000000" }
```

`telefone` só é usado quando `metodo_pagamento` é `GPO`; pode ser omitido para `REF` e `GPO_QR`.

**Response 201:**

```json
{
  "cobranca": {
    "id": "76f2971c-4a7d-48f7-92c2-f8d3b28e9a2d",
    "provider_charge_id": "APPYPAY-987654",
    "merchant_transaction_id": "MATR2608LDA0001",
    "status": "pendente",
    "response": { "status": "Accepted" }
  }
}
```

Quando `metodo_pagamento` é `GPO_QR`, `cobranca` também inclui `qrCodeArr` em base64, no mesmo formato da seção 19.5.

**Regras de negócio:**

- A solicitação precisa estar exactamente em `aprovada_pendente_pagamento_matricula`; qualquer outro status é recusado.
- `metodo_pagamento` precisa estar entre os métodos congelados no momento da aprovação, não na configuração actual da academia.
- Só pode existir uma cobrança de matrícula aberta por solicitação; tentativa concorrente ou repetida enquanto há cobrança aberta é recusada.
- O valor cobrado é sempre o valor congelado na aprovação.
- Esta rota pública responde sempre `409` com a mensagem genérica "solicitação não disponível para pagamento de matrícula" para qualquer recusa depois de validar o JSON, incluindo solicitação inexistente, status inválido, método não habilitado, cobrança aberta ou falha inesperada.
- Se a AppyPay confirmar `Success` de forma síncrona, o vínculo do estudante é efectivado imediatamente.

**Erros comuns:** `400` para JSON malformado, `409` para qualquer outra recusa, `429` no limite de requisições.

### 9.12 `PUT /academia/solicitacao-matricula/:codigo/cancelar`

Cancela uma solicitação que está aguardando pagamento de matrícula e nunca foi paga.

**Proteção:** academia ativa, dona da solicitação.

**Request JSON:**

```json
{ "motivo": "candidato não efetuou o pagamento no prazo" }
```

**Response 200:**

```json
{ "message": "solicitação cancelada com sucesso" }
```

**Regras de negócio:**

- `motivo` é obrigatório e não pode ser vazio após trim.
- A solicitação deve pertencer à academia autenticada e estar exactamente em `aprovada_pendente_pagamento_matricula`; outro status responde `409` com a mensagem "somente solicitação pendente de pagamento de matrícula pode ser cancelada com motivo".
- Antes de cancelar, o backend tenta cancelar qualquer cobrança de matrícula aberta associada; se a cobrança já tiver sido paga, o cancelamento é recusado com `400`.
- Se não havia cobrança aberta, o cancelamento prossegue normalmente.
- O código de estudante reservado na aprovação é libertado; nenhum `Estudante` chegou a ser criado neste caminho.

**Erros comuns:** `400` motivo ausente ou cobrança já paga, `403` solicitação de outra academia, `404` solicitação inexistente, `409` solicitação fora do status esperado.

Depois que a AppyPay confirma `Success` para uma cobrança com `codigo_solicitacao`, por consulta síncrona na criação, consulta manual/posterior em 19.6 ou webhook em 19.10/19.11, o backend cria o `Estudante` com o código reservado e grava `SolicitacaoMatriculaVinculada`, devolvendo a solicitação ao status `aprovada`. Esse é o único momento em que o estudante é criado no fluxo com taxa; a operação é idempotente contra reentrega e reserva novamente a unicidade do bilhete de identidade imediatamente antes da criação para evitar duplicidade em corridas raras.

---

## 10. Cursos

Cursos são cadastros próprios da academia e são a fonte de verdade para vínculo de estudantes, matérias, turmas e regras acadêmicas. Cursos médios existem para escolas de nível `medio` ou `misto`; em escolas mistas, eles representam o domínio do ensino médio, enquanto os anos do fundamental permanecem configurados na própria academia. Cursos superiores existem apenas para academias de nível `superior`. IDs são UUIDs e o tipo do curso é imutável.

### 10.1 `GET /academia/cursos`

Lista cursos de uma academia.

**Proteção:** pública com autenticação opcional.

**Query params:**

| Parâmetro | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `codigo_academia` | string | sim para público/admin; não para academia autenticada | Ignorado para academia autenticada, que sempre usa a própria academia. |

**Exemplo de request:**

```http
GET /academia/cursos?codigo_academia=ACAD001 HTTP/1.1
```

**Response 200:**

```json
{
  "cursos": [CursoDTO],
  "total": 2
}
```

**Regras de negócio:** valida que a academia consultada existe quando o código é recebido por query. Academias autenticadas não podem listar cursos de outra academia por query string.

### 10.2 `GET /academia/curso/:id`

Consulta um curso pelo UUID.

**Proteção:** pública com autenticação opcional.

**Path params:**

| Parâmetro | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `id` | UUID | sim | Identificador do curso. |

**Exemplo de request:**

```http
GET /academia/curso/7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55 HTTP/1.1
```

**Response 200:** `CursoDTO`.

**Regras de negócio:** academia autenticada só pode consultar curso da própria academia. Admin/público consultam pelo UUID existente; UUID inválido retorna erro de validação e curso inexistente retorna `404`.

### 10.3 `POST /academia/curso`

Cria um curso da academia autenticada.

**Proteção:** academia ativa.

**Request body:**

| Campo | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `nome` | string | sim | Nome cadastral do curso. |
| `modelo` | enum | sim apenas para escola `medio` ou `misto` | Aceita `liceu` ou `tecnico`; proibido em academia `superior`. |
| `quantidade_semestres` | inteiro | sim apenas para academia `superior` | Define os períodos/semestres derivados; proibido em curso `medio`. |

**Request body — curso médio:**

```json
{
  "nome": "Ciências Físicas e Biológicas",
  "modelo": "liceu"
}
```

**Request body — curso superior:**

```json
{
  "nome": "Engenharia Informática",
  "quantidade_semestres": 8
}
```

**Exemplo de request — curso superior:**

```http
POST /academia/curso HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json

{
  "nome": "Engenharia Informática",
  "quantidade_semestres": 8
}
```

**Regras de negócio:**

- `nome` é obrigatório.
- O tipo é inferido da academia: escola `medio` ou `misto` cria curso `medio`; academia `superior` cria curso `superior`; outros níveis não criam cursos. Em uma escola `misto`, o curso médio não substitui os anos acadêmicos fundamentais configurados na academia.
- Curso médio exige `modelo` (`liceu` ou `tecnico`), rejeita `anos_academicos` e deriva anos fixos do modelo.
- Curso superior exige quantidade/períodos válidos, rejeita `modelo` e deriva `anos_academicos` a partir dos semestres/períodos.
- A academia deve estar `ativo`.
- Grava evento `CursoCriado` com auditoria da academia.

**Response 201:**

```json
{
  "message": "curso criado com sucesso",
  "data": {
    "id": "uuid-do-curso",
    "nome": "Engenharia Informática",
    "type": "superior",
    "anos_academicos": ["1_ano_superior", "2_ano_superior", "3_ano_superior", "4_ano_superior"],
    "periodos": ["1_semestre", "2_semestre", "3_semestre", "4_semestre", "5_semestre", "6_semestre", "7_semestre", "8_semestre"]
  }
}
```

### 10.4 `PUT /academia/curso/:id/ativar`

Ativa curso inativo da própria academia.

**Proteção:** academia ativa.

**Request body:** vazio; não envie campos.

**Exemplo de request:**

```http
PUT /academia/curso/7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55/ativar HTTP/1.1
Authorization: Bearer <token>
```

**Regras de negócio:** o `id` deve ser UUID válido, o curso deve existir e pertencer à academia autenticada. A transição é delegada ao aggregate `Curso`, que rejeita estados incompatíveis e grava `CursoAtivado`.

**Response 200:**

```json
{
  "message": "curso ativado com sucesso",
  "nome": "Engenharia Informática"
}
```

### 10.5 `PUT /academia/curso/:id/desativar`

Desativa curso da própria academia.

**Proteção:** academia ativa.

**Request body:** vazio; não envie campos.

**Exemplo de request:**

```http
PUT /academia/curso/7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55/desativar HTTP/1.1
Authorization: Bearer <token>
```

**Regras de negócio:** o curso deve existir, pertencer à academia e estar em estado compatível. A transição grava `CursoDesativado`; validações adicionais do aggregate impedem operações inválidas.

**Response 200:**

```json
{
  "message": "curso desativado com sucesso",
  "nome": "Engenharia Informática"
}
```

### 10.6 `PUT /academia/curso/:id/dados`

Atualiza dados cadastrais editáveis de um curso.

**Proteção:** academia ativa.

**Request body:**

| Campo | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `nome` | string | sim | Novo nome cadastral do curso. |

```json
{
  "nome": "Engenharia Informática e Computadores"
}
```

**Exemplo de request:**

```http
PUT /academia/curso/7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55/dados HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json

{
  "nome": "Engenharia Informática e Computadores"
}
```

**Regras de negócio:**

- O curso deve existir e pertencer à academia autenticada.
- `type` é imutável.
- Esta rota altera apenas dados cadastrais; rejeita `anos_academicos`, `periodos`, `semestres`, `quantidade_semestres`, `anos`, `materias_chave` e `modelo`.
- Quando houver alteração que remova anos/períodos em fluxos internos compatíveis, a remoção é bloqueada se existirem estudantes ativos nesses anos/semestres.
- Grava `CursoDadosAtualizados` com auditoria.

**Response 200:**

```json
{
  "message": "curso atualizado com sucesso",
  "nome": "Engenharia Informática e Computadores",
  "type": "superior",
  "anos_academicos": ["1_ano_superior"],
  "periodos": ["1_semestre", "2_semestre"]
}
```

### 10.7 `DELETE /academia/curso/:id`

Remove logicamente um curso.

**Proteção:** academia ativa.

**Request body:**

| Campo | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |
| `motivo` | string | não | Justificativa de auditoria para a deleção lógica. |

```json
{
  "motivo": "curso substituído por nova matriz curricular"
}
```

**Exemplo de request:**

```http
DELETE /academia/curso/7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55 HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json

{
  "motivo": "curso substituído por nova matriz curricular"
}
```

**Regras de negócio:**

- O curso deve pertencer à academia autenticada e estar inativo; curso ativo deve ser desativado antes da deleção.
- Não pode haver estudantes matriculados no curso.
- Não pode haver matérias ativas vinculadas ao curso.
- Matérias inativas/deletáveis e turmas inativas, sem estudantes, podem ser deletadas em cascata antes da deleção do curso.
- Turma ativa ou turma ainda com estudantes bloqueia a operação.
- Grava `CursoDeletado` e eventos de cascata de matérias/turmas quando aplicável.

**Response 200:**

```json
{
  "message": "curso deletado com sucesso",
  "curso_id": "uuid-do-curso",
  "nome": "Engenharia Informática",
  "materias_deletadas": ["Algoritmos"],
  "turmas_deletadas": ["INF-1A"],
  "auditavel": true
}
```

### 10.8 `POST /academia/curso/async`

Cria cursos em lote por job assíncrono.

**Proteção:** academia ativa.

**Request body:** array JSON não vazio com até `200` itens, cada item com o mesmo payload aceito por `POST /academia/curso`.

```json
[
  {
    "nome": "Engenharia Informática",
    "quantidade_semestres": 8
  },
  {
    "nome": "Gestão",
    "quantidade_semestres": 6
  }
]
```

**Response 202:**

```json
{
  "message": "job criado com sucesso — use GET /jobs/:id ou GET /jobs/stream para acompanhar o progresso",
  "job_id": "uuid-do-job",
  "total_items": 2,
  "status": "pending",
  "poll_url": "/jobs/uuid-do-job",
  "sse_url": "/jobs/stream"
}
```

**Regras de negócio:** o body deve ser array JSON não vazio. Cada item é processado pelo mesmo conjunto de regras da criação unitária; sucessos e falhas são reportados nos resultados do job.

### 10.9 `PUT /academia/curso/ativar/async`

Ativa cursos em lote.

**Proteção:** academia ativa.

**Request body:** array JSON não vazio com até `500` itens; cada item deve identificar o curso por `id`.

```json
[
  { "id": "7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55" },
  { "id": "219d26c0-14b2-46a0-8f4f-ea4e07da2e36" }
]
```

**Response 202:** envelope assíncrono de `10.8`.

**Regras de negócio:** cada item segue as regras de `PUT /academia/curso/:id/ativar` e fica auditável no job.

### 10.10 `PUT /academia/curso/desativar/async`

Desativa cursos em lote.

**Proteção:** academia ativa.

**Request body:** array JSON não vazio com até `500` itens; cada item deve identificar o curso por `id`.

```json
[
  { "id": "7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55" },
  { "id": "219d26c0-14b2-46a0-8f4f-ea4e07da2e36" }
]
```

**Response 202:** envelope assíncrono de `10.8`.

**Regras de negócio:** cada item segue as regras de `PUT /academia/curso/:id/desativar`.

### 10.11 `PUT /academia/curso/dados/async`

Atualiza dados cadastrais de cursos em lote.

**Proteção:** academia ativa.

**Request body:** array JSON não vazio com até `500` itens; cada item deve identificar o curso e os campos de dados aceitos pela rota unitária.

```json
[
  {
    "id": "7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55",
    "nome": "Engenharia Informática e Computadores"
  }
]
```

**Response 202:** envelope assíncrono de `10.8`.

**Regras de negócio:** cada item segue as regras de `PUT /academia/curso/:id/dados`; campos acadêmicos proibidos continuam proibidos no lote.

### 10.12 `DELETE /academia/curso/async`

Remove cursos em lote por job assíncrono.

**Proteção:** academia ativa.

**Request body:** array JSON não vazio com até `500` itens; cada item deve identificar o curso por `id` e pode informar `motivo`.

```json
[
  {
    "id": "7f2f4d0e-6e2a-4c39-b7dd-12b63d9d5d55",
    "motivo": "curso substituído por nova matriz curricular"
  }
]
```

**Response 202:** envelope assíncrono de `10.8`.

**Regras de negócio:** cada item segue as regras de `DELETE /academia/curso/:id`, incluindo exigência de curso inativo, ausência de estudantes e bloqueios por matérias/turmas ativas.

---

## 11. Matérias

### Processos de negócio — Matérias

Matérias pertencem sempre a uma academia e são a base para notas, faltas, turmas e avaliações finais. Em academias escolares, o tipo é inferido pelo `nivel_escolar`; academias mistas devem informar `type` como `fundamental` ou `medio`; academias superiores criam apenas matérias `superior`, vinculadas a curso superior e a um `periodo` acadêmico. Uma matéria do médio pode declarar mais de um ano acadêmico em `anos_academicos`, mas o backend bloqueia `4_ano_medio` para matérias convencionais do médio. Campos de pendência (`pendencia_permitida` e `pendencia_nivel_conclusao`) são exclusivos do superior.

Todas as escritas exigem autenticação de academia ativa. Consultas de academia usam a própria instituição; admins podem consultar uma academia específica quando a rota aceitar `codigo_academia`. IDs de matéria são UUIDs.

### `GET /academia/materias`

Lista as matérias da academia alvo.

**Proteção:** academia ativa ou admin autenticado.

**Query params:**

- `codigo_academia` — obrigatório para admin; ignorado/derivado da sessão para academia.

**Response 200:**

```json
{
  "materias": [
    {
      "id": "uuid-da-materia",
      "nome": "Matemática",
      "type": "medio",
      "status": "ativa",
      "codigo_academia": "ACAD001",
      "anos_academicos": ["1_ano_medio", "2_ano_medio"],
      "curso_id": "uuid-do-curso",
      "periodo": null,
      "pendencia_permitida": null,
      "pendencia_nivel_conclusao": null
    }
  ],
  "total": 1
}
```

**Erros comuns:** `400` quando admin não informa `codigo_academia`; `403` quando a academia tenta consultar dados fora do próprio escopo; `500` para falha de projeção.

### `GET /academia/materia/:id`

Consulta uma matéria específica pelo UUID.

**Proteção:** academia ativa ou admin autenticado.

**Path params:**

- `id` — UUID da matéria.

**Query params:**

- `codigo_academia` — usado por admin para delimitar o escopo quando necessário.

**Response 200:** retorna o DTO completo da matéria, com os mesmos campos da listagem.

**Erros comuns:** `400` para UUID inválido; `404` quando a matéria não existe; `403` quando a matéria não pertence à academia autenticada.

### `POST /academia/materia`

Cria uma matéria disciplinar no escopo da academia autenticada.

**Proteção:** academia ativa.

**Request — fundamental/médio:**

```json
{
  "nome": "Matemática",
  "type": "medio",
  "anos_academicos": ["1_ano_medio", "2_ano_medio"],
  "curso_id": "uuid-do-curso-medio"
}
```

**Request — superior:**

```json
{
  "nome": "Algoritmos",
  "curso_id": "uuid-do-curso-superior",
  "anos_academicos": ["1_ano_superior"],
  "periodo": "1_semestre",
  "pendencia_permitida": true,
  "pendencia_nivel_conclusao": "2_semestre"
}
```

**Regras de validação:**

- `nome` e `anos_academicos` são obrigatórios.
- Matérias do médio podem informar múltiplos anos acadêmicos no mesmo array, desde que nenhum item seja `4_ano_medio`.
- Em academia mista, `type` é obrigatório e deve ser `fundamental` ou `medio`.
- Em ensino superior, `curso_id` e `periodo` são obrigatórios.
- Campos de pendência só são aceitos para matérias superiores.
- A matéria nasce ativa quando as validações de tipo, curso, período e anos acadêmicos passam.

**Response 201:**

```json
{
  "message": "matéria criada com sucesso",
  "data": {
    "id": "uuid-da-materia",
    "nome": "Algoritmos",
    "type": "superior",
    "status": "ativa",
    "periodo": "1_semestre",
    "pendencia_permitida": true,
    "pendencia_nivel_conclusao": "2_semestre"
  }
}
```

### `PUT /academia/materia/:id/ativar`

Ativa uma matéria da própria academia para novas operações acadêmicas.

**Proteção:** academia ativa.

**Path params:** `id` — UUID da matéria.

**Request:** sem payload.

**Response 200:**

```json
{
  "message": "matéria ativada com sucesso",
  "id": "uuid-da-materia"
}
```

**Erros comuns:** `400` para UUID inválido ou matéria incompatível com regras atuais; `404` quando não encontrada; `403` quando pertence a outra academia.

### `PUT /academia/materia/:id/desativar`

Desativa uma matéria da própria academia para impedir novos lançamentos, mantendo histórico de notas, faltas e avaliações.

**Proteção:** academia ativa.

**Path params:** `id` — UUID da matéria.

**Request:** sem payload.

**Response 200:**

```json
{
  "message": "matéria desativada com sucesso",
  "id": "uuid-da-materia"
}
```

### `PUT /academia/materia/:id/dados`

Atualiza dados editáveis da matéria.

**Proteção:** academia ativa.

**Path params:** `id` — UUID da matéria.

**Request:** pelo menos um campo editável deve ser enviado.

```json
{
  "nome": "Matemática Aplicada",
  "anos_academicos": ["1_ano_medio", "2_ano_medio"],
  "curso_id": "uuid-do-curso",
  "pendencia_permitida": false,
  "pendencia_nivel_conclusao": null
}
```

**Regras de validação:**

- `periodo` não é editável nesta rota.
- `curso_id` deve apontar para curso compatível com a academia e o tipo da matéria.
- Matérias do médio continuam podendo ter múltiplos anos acadêmicos, mas atualizações com `4_ano_medio` são rejeitadas.
- Pendência continua exclusiva de matérias superiores.
- A atualização preserva o mesmo ID e grava evento de alteração no ledger.

**Response 200:**

```json
{
  "message": "dados da matéria atualizados com sucesso",
  "id": "uuid-da-materia"
}
```

### `DELETE /academia/materia/:id`

Remove logicamente uma matéria da própria academia.

**Proteção:** academia ativa.

**Path params:** `id` — UUID da matéria.

**Request:** sem payload.

**Response 200:**

```json
{
  "message": "matéria removida com sucesso",
  "id": "uuid-da-materia"
}
```

A operação preserva o ledger e registros acadêmicos associados.

### `POST /academia/materia/async`

Cria matérias em lote por job assíncrono.

**Proteção:** academia ativa.

**Request:** array de itens com o mesmo contrato de `POST /academia/materia`.

```json
[
  {
    "nome": "Algoritmos",
    "curso_id": "uuid-do-curso-superior",
    "anos_academicos": ["1_ano_superior"],
    "periodo": "1_semestre"
  }
]
```

**Response 202:** retorna `job_id`, `status`, `total_items`, `poll_url` e `sse_url`. Resultados individuais ficam disponíveis em `GET /jobs/:id?results=true`.

### `PUT /academia/materia/ativar/async`

Ativa matérias em lote.

**Proteção:** academia ativa.

**Request:**

```json
[
  { "id": "uuid-da-materia" }
]
```

**Response 202:** job assíncrono com acompanhamento por polling ou SSE.

### `PUT /academia/materia/desativar/async`

Desativa matérias em lote.

**Proteção:** academia ativa.

**Request:**

```json
[
  { "id": "uuid-da-materia" }
]
```

**Response 202:** job assíncrono com resultados por item.

### `PUT /academia/materia/dados/async`

Atualiza dados de matérias em lote.

**Proteção:** academia ativa.

**Request:** array de itens com `id` e campos aceitos por `PUT /academia/materia/:id/dados`.

```json
[
  {
    "id": "uuid-da-materia",
    "nome": "Matemática Aplicada",
    "anos_academicos": ["10_ano_medio"]
  }
]
```

**Response 202:** job assíncrono com resultados por item.

### `DELETE /academia/materia/async`

Remove logicamente matérias em lote.

**Proteção:** academia ativa.

**Request:**

```json
[
  { "id": "uuid-da-materia" }
]
```

**Response 202:** job assíncrono com preservação histórica igual à rota síncrona.

---

## 12. Turmas

### Processos de negócio — Turmas

Turmas organizam estudantes por `nivel`, `turno` e, quando aplicável, `curso_id`. O backend normaliza `codigo_turma`, valida compatibilidade de nível/curso e mantém histórico por ano letivo. Escritas exigem academia ativa; consultas são escopadas por academia ou por autorização do estudante.

### `GET /academia/turmas`

Lista turmas da academia alvo.

**Proteção:** academia ativa ou admin autenticado.

**Query params:**

- `codigo_academia` — obrigatório para admin; derivado da sessão para academia.

**Response 200:**

```json
{
  "turmas": [
    {
      "id": "uuid-da-turma",
      "codigo_turma": "10A",
      "codigo_academia": "ACAD001",
      "nivel": "10_ano_medio",
      "curso_id": "uuid-do-curso",
      "turno": "manha",
      "status": "ativa",
      "estudantes": ["EST-2026-0001"],
      "historico_estudantes_ano_letivo": {
        "2025_2026": ["EST-2026-0001"]
      }
    }
  ]
}
```

### `GET /academia/turma/:codigo`

Consulta uma turma pelo código normalizado.

**Proteção:** academia ativa ou admin autenticado.

**Path params:** `codigo` — código da turma.

**Query params:** `codigo_academia` para admin.

**Response 200:** retorna o DTO da turma.

**Erros comuns:** `404` para turma inexistente; `403` quando academia tenta acessar turma de outra instituição.

### `GET /turmas-estudante/:codigo`

Lista turmas de um estudante.

**Proteção:** autenticado.

**Path params:** `codigo` — código do estudante.

**Autorização:** estudante consulta apenas o próprio código; academia consulta apenas estudantes da própria instituição; admin consulta qualquer estudante.

**Response 200:**

```json
{
  "codigo_estudante": "EST-2026-0001",
  "turmas": [
    {
      "codigo_turma": "10A",
      "nivel": "10_ano_medio",
      "curso_id": "uuid-do-curso",
      "turno": "manha",
      "ano_letivo": "2025_2026"
    }
  ],
  "total": 1
}
```

### `POST /academia/turma`

Cria uma turma na academia autenticada.

**Proteção:** academia ativa.

**Request:**

```json
{
  "codigo_turma": "10A",
  "nivel": "10_ano_medio",
  "curso_id": "uuid-do-curso-medio-ou-superior",
  "turno": "manha"
}
```

**Regras de validação:**

- `codigo_turma`, `nivel` e `turno` são obrigatórios.
- `curso_id` é obrigatório para médio e superior.
- Turmas fundamentais não usam `curso_id`.
- O código é normalizado e deve ser único dentro da academia.

**Response 201:**

```json
{
  "message": "turma criada com sucesso",
  "id": "uuid-da-turma",
  "codigo_turma": "10A"
}
```

### `PUT /academia/turma/:codigo/ativar`

Ativa uma turma da própria academia.

**Proteção:** academia ativa.

**Path params:** `codigo` — código da turma.

**Request:** sem payload.

**Response 200:**

```json
{
  "message": "turma ativada com sucesso",
  "codigo_turma": "10A"
}
```

### `PUT /academia/turma/:codigo/desativar`

Desativa uma turma da própria academia para novas operações.

**Proteção:** academia ativa.

**Path params:** `codigo` — código da turma.

**Request:** sem payload.

**Response 200:**

```json
{
  "message": "turma desativada com sucesso",
  "codigo_turma": "10A"
}
```

### `PUT /academia/turma/:codigo/dados`

Atualiza dados operacionais da turma.

**Proteção:** academia ativa.

**Path params:** `codigo` — código da turma.

**Request:**

```json
{
  "nivel": "11_ano_medio",
  "curso_id": "uuid-do-curso",
  "turno": "tarde"
}
```

**Regras de validação:**

- Pelo menos um campo deve ser enviado.
- Estudantes já vinculados precisam continuar compatíveis com o novo `nivel` e `curso_id`.
- `curso_id` é obrigatório para médio/superior e não deve ser usado no fundamental.

**Response 200:**

```json
{
  "message": "dados da turma atualizados com sucesso",
  "codigo_turma": "10A"
}
```

### `DELETE /academia/turma/:codigo`

Remove logicamente uma turma da própria academia.

**Proteção:** academia ativa.

**Path params:** `codigo` — código da turma.

**Request:** sem payload.

**Response 200:**

```json
{
  "message": "turma removida com sucesso",
  "codigo_turma": "10A"
}
```

Histórico por ano letivo e eventos permanecem disponíveis para auditoria.

### `POST /academia/turma/:codigo/estudante`

Adiciona estudante à turma. Esta rota continua existindo para vincular ou corrigir vínculos de estudantes já cadastrados; o cadastro de estudante também aceita vinculação direta quando recebe `codigo_turma`.

**Proteção:** academia ativa.

**Path params:** `codigo` — código da turma.

**Request:**

```json
{
  "codigo_estudante": "EST-2026-0001"
}
```

**Regras de validação:**

- Estudante deve existir e pertencer à academia autenticada.
- Turma deve existir, estar no escopo da academia e ser compatível com ano acadêmico e curso do estudante.
- O vínculo é registrado no ano letivo ativo da academia.

**Response 200:**

```json
{
  "message": "estudante adicionado à turma com sucesso",
  "codigo_turma": "10A",
  "codigo_estudante": "EST-2026-0001"
}
```

### `DELETE /academia/turma/:codigo/estudantes/:codigo_estudante`

Remove estudante da turma.

**Proteção:** academia ativa.

**Path params:**

- `codigo` — código da turma.
- `codigo_estudante` — código do estudante.

**Request:** sem payload.

**Response 200:**

```json
{
  "message": "estudante removido da turma com sucesso",
  "codigo_turma": "10A",
  "codigo_estudante": "EST-2026-0001"
}
```

### `POST /academia/turma/async`

Cria turmas em lote por job assíncrono.

**Proteção:** academia ativa.

**Request:** array de itens com o contrato de `POST /academia/turma`.

```json
[
  {
    "codigo_turma": "10A",
    "nivel": "10_ano_medio",
    "curso_id": "uuid-do-curso",
    "turno": "manha"
  }
]
```

**Response 202:** job assíncrono com `job_id`, `status`, `total_items`, `poll_url` e `sse_url`.

### `POST /academia/turma/estudante/async`

Adiciona estudantes a turmas em lote.

**Proteção:** academia ativa.

**Request:**

```json
[
  {
    "codigo_turma": "10A",
    "codigo_estudante": "EST-2026-0001"
  }
]
```

**Response 202:** job assíncrono; cada item aplica as mesmas validações da rota síncrona.

### `PUT /academia/turma/ativar/async`

Ativa turmas em lote.

**Proteção:** academia ativa.

**Request:**

```json
[
  { "codigo_turma": "10A" }
]
```

**Response 202:** job assíncrono.

### `PUT /academia/turma/desativar/async`

Desativa turmas em lote.

**Proteção:** academia ativa.

**Request:**

```json
[
  { "codigo_turma": "10A" }
]
```

**Response 202:** job assíncrono.

### `PUT /academia/turma/dados/async`

Atualiza dados de turmas em lote.

**Proteção:** academia ativa.

**Request:** array de itens com `codigo_turma` e campos aceitos por `PUT /academia/turma/:codigo/dados`.

```json
[
  {
    "codigo_turma": "10A",
    "turno": "tarde"
  }
]
```

**Response 202:** job assíncrono com resultados por item.

### `DELETE /academia/turma/async`

Remove logicamente turmas em lote.

**Proteção:** academia ativa.

**Request:**

```json
[
  { "codigo_turma": "10A" }
]
```

**Response 202:** job assíncrono.

### `DELETE /academia/turma/estudante/async`

Remove estudantes de turmas em lote.

**Proteção:** academia ativa.

**Request:**

```json
[
  {
    "codigo_turma": "10A",
    "codigo_estudante": "EST-2026-0001"
  }
]
```

**Response 202:** job assíncrono; histórico por ano letivo é preservado.

---

### `GET /eventos-estudante/:codigo`

Lista o histórico imutável de eventos de um estudante.

**Proteção:** autenticado.

**Autorização:** admin consulta qualquer estudante; estudante consulta apenas o próprio código; academia consulta apenas estudantes vinculados à sua instituição.

**Path params:** `codigo` — código do estudante.

**Response 200:**

```json
{
  "codigo_estudante": "EST-2026-0001",
  "eventos": [
    {
      "event_id": "uuid-do-evento",
      "aggregate_id": "uuid-do-estudante",
      "event_type": "NotaCorrigida",
      "payload": { "NovaNota": 15.5, "Motivo": "Erro de digitacao" },
      "metadata": { "user_id": "uuid-da-academia", "user_type": "academia", "ip": "127.0.0.1" }
    }
  ],
  "total": 1
}
```

### `GET /eventos/:event_id`

Consulta um evento imutável específico do ledger, incluindo payload e metadados de auditoria.

**Proteção:** autenticado.

**Autorização:** admin consulta qualquer evento; estudante e academia só consultam eventos associados a estudante próprio/da própria instituição. Sem posse, a resposta é `404` para não revelar a existência do evento.

**Path params:** `event_id` — UUID do evento.

**Response 200:**

```json
{
  "evento": {
    "event_id": "uuid-do-evento",
    "aggregate_id": "uuid-do-estudante",
    "event_type": "FaltaCorrigida",
    "payload": { "NovaQuantidade": 2, "Motivo": "Quantidade confirmada" },
    "metadata": { "user_id": "uuid-da-academia", "user_type": "academia", "ip": "127.0.0.1" }
  }
}
```

## 13. Notas

### Processos de negócio — Notas

Notas são registros acadêmicos imutáveis vinculados a estudante, academia, ano letivo ativo, ano acadêmico inferido pelo vínculo do estudante e matéria disciplinar. A correção é permitida exclusivamente por evento compensatório auditado, sem apagar o lançamento original. A academia autenticada registra notas apenas para estudantes da própria instituição e apenas em matérias compatíveis com o nível, curso, ano acadêmico e tipo de ensino. Para academias escolares, `tipo` deve ser `escolar`; para academias superiores, `tipo` deve ser `superior`. O lançamento ou a correção pode acionar avaliações finais automáticas quando a categoria registrada é configurada como `nota_despertadora` de regra ativa.

### `POST /academia/notas-aluno`

Registra uma nota individual.

**Proteção:** academia ativa.

**Request:**

```json
{
  "codigo_estudante": "EST-2026-0001",
  "periodo": "1_trimestre",
  "materia_disciplinar_id": "uuid-da-materia",
  "tipo": "escolar",
  "categoria": "mac",
  "nota": 15.5,
  "observacao": "Bom desempenho"
}
```

**Campos obrigatórios:** `codigo_estudante`, `periodo`, `materia_disciplinar_id`, `tipo`, `categoria` e `nota`.

**Regras de validação:**

- A academia precisa ter ano letivo ativo.
- O estudante precisa pertencer à academia autenticada.
- A matéria precisa existir, pertencer à academia e ser compatível com o estudante.
- `tipo` deve ser `escolar` para escola e `superior` para academia superior.
- A nota é registrada no ano letivo ativo da academia e no ano acadêmico inferido do estudante/matéria.

**Response 201:**

```json
{
  "message": "nota registrada com sucesso",
  "estudante": "EST-2026-0001",
  "materia": "Matemática",
  "tipo": "escolar",
  "categoria": "mac",
  "nota": 15.5,
  "ano_academico": "10_ano_medio",
  "periodo": "1_trimestre",
  "periodos_validos": ["1_trimestre", "2_trimestre", "3_trimestre"],
  "avaliacoes_finais_automaticas": []
}
```

### `PATCH /academia/notas-aluno/:id`

Corrige uma nota por evento compensatório; o lançamento original permanece intacto no ledger. Uma correção pode disparar o recálculo de avaliação final.

**Proteção:** academia ativa e dona da nota.

**Path params:** `id` — UUID da nota retornado nas listagens.

**Request:**

```json
{
  "nota": 15.5,
  "observacao": "Valor confirmado após revisão",
  "motivo": "Erro de digitacao no lançamento original"
}
```

**Regras:** `motivo` é obrigatório; `nota` respeita a escala do ano acadêmico; `observacao` aceita no máximo 2000 caracteres. A chave acadêmica da nota é derivada do registro existente, nunca do corpo enviado.

**Response 200:**

```json
{ "message": "nota corrigida com sucesso", "id": "uuid-da-nota" }
```

**Erros:** `400` para motivo ausente, JSON inválido ou nota fora da escala; `403` quando a nota pertence a outra academia; `404` quando o ID não existe.

### `GET /notas`

Lista notas a partir da projeção global.

**Proteção:** admin ou academia ativa.

**Query params:**

- `limit`, `offset` — paginação.
- `ano_letivo`, `ano_academico`, `curso_id`, `codigo_turma`, `periodo`, `materia_disciplinar_id`, `codigo_academia`, `categoria` — filtros; aceitam repetição ou valores separados por vírgula.
- Para admin, `codigo_turma` exige `codigo_academia`.
- `corrigido=true|false` filtra registros que já receberam (ou não receberam) evento compensatório de correção.
- Para academia, `codigo_academia` é sempre forçado para a própria instituição.

**Response 200:**

```json
{
  "notas": [
    {
      "id": "uuid",
      "codigo_estudante": "EST-2026-0001",
      "estudante_nome": "Maria Silva",
      "codigo_academia": "ACAD001",
      "academia_nome": "Academia Exemplo",
      "ano_lectivo": "2025_2026",
      "ano_academico": "10_ano_medio",
      "periodo": "1_trimestre",
      "materia_disciplinar_id": "uuid-da-materia",
      "materia_nome": "Matemática",
      "tipo": "escolar",
      "categoria": "mac",
      "nota": 15.5,
      "observacao": "Bom desempenho",
      "registrado_por": "uuid-da-academia",
      "valor_anterior": 12.0,
      "motivo_correcao": "Erro de digitacao no lançamento original",
      "corrigido_por": "uuid-da-academia",
      "corrigido_em": "2026-07-22T11:00:00Z",
      "registered_at": "2026-07-21T10:30:00Z",
      "event_id": "event-uuid",
      "version": 1
    }
  ],
  "total": 1,
  "total_geral": 1,
  "limit": 50,
  "offset": 0
}
```

### `GET /notas-estudante/:codigo`

Retorna as notas de um estudante específico.

**Proteção:** autenticado.

**Path params:** `codigo` — código do estudante.

**Autorização:** estudante só consulta o próprio código; academia só consulta estudante da própria instituição; admin pode consultar qualquer estudante.

**Query params:** `ano_letivo`, `ano_academico`, `curso_id`, `periodo`, `materia_disciplinar_id`, `codigo_academia` e `categoria`.

**Response 200:**

```json
{
  "codigo_estudante": "EST-2026-0001",
  "nome": "Maria Silva",
  "notas": [
    {
      "id": "uuid",
      "nota": 15.5,
      "registrado_por": "uuid-da-academia",
      "valor_anterior": 12.0,
      "motivo_correcao": "Erro de digitacao no lançamento original",
      "corrigido_por": "uuid-da-academia",
      "corrigido_em": "2026-07-22T11:00:00Z"
    }
  ],
  "total": 0
}
```

### `POST /academia/notas-aluno/async`

Registra notas em lote por job assíncrono.

**Proteção:** academia ativa.

**Request:** array de itens com o contrato de `POST /academia/notas-aluno`.

```json
[
  {
    "codigo_estudante": "EST-2026-0001",
    "periodo": "1_trimestre",
    "materia_disciplinar_id": "uuid-da-materia",
    "tipo": "escolar",
    "categoria": "mac",
    "nota": 15.5
  }
]
```

**Response 202:** job assíncrono com acompanhamento em `GET /jobs/:id` e `GET /jobs/stream`.

---

## 14. Faltas

### Processos de negócio — Faltas

Faltas são registros acadêmicos imutáveis vinculados a estudante, academia, ano letivo ativo, ano acadêmico inferido e matéria disciplinar. A correção é permitida exclusivamente por evento compensatório auditado, sem apagar o lançamento original. A academia autenticada registra faltas apenas para estudantes da própria instituição e matérias compatíveis. A data do lançamento é validada no intervalo do ano letivo ativo calculado a partir do tipo da academia e da matéria.

### `POST /academia/faltas-aluno`

Registra faltas individuais.

**Proteção:** academia ativa.

**Request:**

```json
{
  "codigo_estudante": "EST-2026-0001",
  "data": "2026-03-15",
  "periodo": "1_trimestre",
  "materia_disciplinar_id": "uuid-da-materia",
  "quantidade": 2,
  "observacao": "Ausência justificada posteriormente"
}
```

**Campos obrigatórios:** `codigo_estudante`, `data`, `periodo`, `materia_disciplinar_id` e `quantidade`.

**Regras de validação:**

- `quantidade` deve ser maior que zero.
- `periodo` é obrigatório e deve ser um dos períodos válidos: para tipo escolar, `1_trimestre`, `2_trimestre` ou `3_trimestre`; para tipo superior, um período configurado no curso.
- Para matérias do tipo superior com período fixo, `periodo` deve ser exatamente igual ao período definido na matéria.
- A academia precisa ter ano letivo ativo.
- O estudante precisa pertencer à academia autenticada.
- A matéria precisa pertencer à academia e ser compatível com o estudante.
- `data` deve estar dentro do intervalo permitido para o ano letivo ativo.

**Response 201:**

```json
{
  "message": "faltas registradas com sucesso",
  "estudante": "EST-2026-0001",
  "materia": "Matemática",
  "quantidade": 2,
  "periodo": "1_trimestre",
  "periodos_validos": ["1_trimestre", "2_trimestre", "3_trimestre"],
  "ano_academico": "10_ano_medio"
}
```

### `PATCH /academia/faltas-aluno/:id`

Corrige uma falta por evento compensatório; o lançamento original permanece intacto no ledger.

**Proteção:** academia ativa e dona da falta.

**Path params:** `id` — UUID da falta retornado nas listagens.

**Request:**

```json
{
  "quantidade": 2,
  "observacao": "Quantidade confirmada após revisão",
  "motivo": "Erro de digitacao no lançamento original"
}
```

**Regras:** `motivo` é obrigatório; `quantidade` deve estar entre 1 e 100; `observacao` aceita no máximo 2000 caracteres. A data, a matéria e o período são derivados do registro existente; `periodo` é imutável e não é aceito no corpo desta rota.

**Response 200:**

```json
{ "message": "falta corrigida com sucesso", "id": "uuid-da-falta" }
```

**Erros:** `400` para motivo ausente, JSON inválido ou quantidade fora da escala; `403` quando a falta pertence a outra academia; `404` quando o ID não existe.

### `GET /faltas`

Lista faltas a partir da projeção global.

**Proteção:** admin ou academia ativa.

**Query params:**

- `limit`, `offset` — paginação.
- `ano_letivo`, `ano_academico`, `curso_id`, `codigo_turma`, `periodo`, `materia_disciplinar_id`, `codigo_academia` — filtros; aceitam repetição ou valores separados por vírgula.
- Em faltas, `periodo` filtra o período do próprio registro de falta, no mesmo formato usado por notas.
- Para admin, `codigo_turma` exige `codigo_academia`.
- `corrigido=true|false` filtra registros que já receberam (ou não receberam) evento compensatório de correção.
- Para academia, `codigo_academia` é sempre forçado para a própria instituição.

**Response 200:**

```json
{
  "faltas": [
    {
      "id": "uuid",
      "codigo_estudante": "EST-2026-0001",
      "estudante_nome": "Maria Silva",
      "codigo_academia": "ACAD001",
      "academia_nome": "Academia Exemplo",
      "ano_lectivo": "2025_2026",
      "ano_academico": "10_ano_medio",
      "periodo": "1_trimestre",
      "data": "2026-03-15",
      "materia_disciplinar_id": "uuid-da-materia",
      "materia_nome": "Matemática",
      "quantidade": 2,
      "observacao": "Ausência justificada posteriormente",
      "registrado_por": "uuid-da-academia",
      "valor_anterior": 1,
      "motivo_correcao": "Quantidade corrigida após conferencia",
      "corrigido_por": "uuid-da-academia",
      "corrigido_em": "2026-07-22T11:00:00Z",
      "registered_at": "2026-07-21T10:30:00Z",
      "event_id": "event-uuid",
      "version": 1
    }
  ],
  "total": 1,
  "total_geral": 1,
  "limit": 50,
  "offset": 0
}
```

### `GET /faltas-estudante/:codigo`

Retorna as faltas de um estudante específico.

**Proteção:** autenticado.

**Path params:** `codigo` — código do estudante.

**Autorização:** estudante só consulta o próprio código; academia só consulta estudante da própria instituição; admin pode consultar qualquer estudante.

**Query params:** `ano_letivo`, `ano_academico`, `curso_id`, `periodo`, `materia_disciplinar_id` e `codigo_academia`. O filtro `periodo` usa o período do próprio registro de falta, não o período da matéria.

**Response 200:**

```json
{
  "codigo_estudante": "EST-2026-0001",
  "nome": "Maria Silva",
  "faltas": [
    {
      "id": "uuid",
      "periodo": "1_trimestre",
      "quantidade": 2,
      "registrado_por": "uuid-da-academia",
      "valor_anterior": 1,
      "motivo_correcao": "Quantidade corrigida após conferencia",
      "corrigido_por": "uuid-da-academia",
      "corrigido_em": "2026-07-22T11:00:00Z"
    }
  ],
  "total": 0
}
```

### `POST /academia/faltas-aluno/async`

Registra faltas em lote por job assíncrono.

**Proteção:** academia ativa.

**Request:** array de itens com o contrato de `POST /academia/faltas-aluno`.

```json
[
  {
    "codigo_estudante": "EST-2026-0001",
    "data": "2026-03-15",
    "periodo": "1_trimestre",
    "materia_disciplinar_id": "uuid-da-materia",
    "quantidade": 2
  }
]
```

**Nota de contrato:** esta é uma mudança breaking; `POST /academia/faltas-aluno` e `POST /academia/faltas-aluno/async` rejeitam itens sem `periodo`.

**Response 202:** job assíncrono com acompanhamento em `GET /jobs/:id` e `GET /jobs/stream`.

---

## 15. Avaliações Finais

#### 15.1.1 Conceitos funcionais

| Conceito | Significado funcional |
|---|---|
| Regra raiz | Regra ativa sem `aplica_se_reprovado_em_type`. É a primeira etapa da cadeia para uma academia, `nivel` e escopo. Deve existir no máximo uma raiz ativa por escopo aplicável. |
| Regra descendente | Regra ativa com `aplica_se_reprovado_em_type`, executada somente depois de reprovação no `type` indicado. Modela recuperação, exame, recurso ou outra nova chance. |
| `type` | Nome público da etapa (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.). É configurado na regra, não enviado para executar avaliação. |
| `nota_despertadora` | Código da categoria de nota que desperta a execução automática da regra raiz. É exclusivo de regras raízes; descendentes não aceitam o campo e continuam sendo acionadas por reprovação ancestral. |
| `nivel` | Campo público de escopo da regra: `fundamental`, `medio` ou `superior`. O contrato novo de regras não aceita `tipo_ensino`. |
| Fórmula | Expressão textual declarativa validada por parser próprio. Calcula a nota final de uma matéria usando notas existentes. Não há `eval`, script, template executável nem código dinâmico. |
| `nota_minima_aprovacao` | Nota mínima para aprovar cada matéria avaliada na etapa. |
| Matérias avaliadas | Matérias ativas da academia que pertencem ao nível/curso/ano/período do estudante e, se houver, ao filtro `materias_aplicaveis` da regra. |
| `materias_aplicaveis` | Lista opcional de matérias que restringe a execução da regra. É especialmente útil em descendentes para recalcular apenas matérias de recuperação/exame/recurso. |
| `limite_materias_pendentes` | Campo configurável apenas no Superior. Em escolas, regras/limites finais são fixos do sistema e não são cadastrados pela academia. |
| `pendencia_permitida` | Campo exclusivo de matéria do Superior. Somente matérias superiores com esse campo verdadeiro podem gerar aprovação com pendência. |
| `pendencia_nivel_conclusao` | Campo exclusivo de matéria do Superior que indica o semestre de conclusão usado para bloqueio funcional de progressão/conclusão quando há pendência aberta. |
| Matéria pendente | Registro persistente em `projection_materias_pendentes`, criado quando uma avaliação do Superior aprova com pendência. Mantém histórico aberto/baixado por estudante, matéria, curso, ano letivo e escopo. |

A avaliação registrada é idempotente no escopo suportado: o sistema evita gravar duas avaliações com o mesmo estudante, academia, ano letivo, nível interno da avaliação, ano/período acadêmico atual e `type`. Eventos e projeções preservam snapshots de fórmula, regra, matérias e pendências suficientes para auditoria.

#### 15.1.2 Montagem e criação de regras de avaliação final

Na versão 2.1.0, as regras configuráveis de avaliação final são exclusivas do ensino superior. Escolas não criam, editam nem removem regras por endpoint: o padrão avaliativo escolar é fixo do sistema, alinhado às categorias fixas e às etapas oficiais (`nota_professor`, `prova_trimestral`, exames quando aplicável e `nota_pap` no técnico). Na execução automática escolar, não há fallback para regras configuráveis ou legadas da projeção; uma categoria sem `nota_despertadora` fixa simplesmente não dispara avaliação final.

A academia monta uma cadeia declarando uma regra raiz e, opcionalmente, regras descendentes. O endpoint de criação usa `nivel` como campo público; `tipo_ensino` é legado e é rejeitado em criação/edição de regras.

**Preenchimento/validação de `nivel` pela academia autenticada:**

| Academia autenticada | Comportamento |
|---|---|
| Superior | O backend força `nivel="superior"`. Se o payload informar outro nível, falha. |
| Escola fundamental/média/mista | Regras escolares são fixas do sistema; endpoints de criação/edição/remoção rejeitam `fundamental` e `medio`. |

**Campos por nível:**

| Campo | Fundamental | Médio | Superior |
|---|---:|---:|---:|
| `nivel` | `fundamental` | `medio` | `superior` |
| `anos_academicos` | Obrigatório e não vazio; array simples de anos do Ensino Primário e Iº Ciclo | Obrigatório; lista de objetos `{curso_id, anos_academicos}` por curso médio | Rejeitado |
| `materias_aplicaveis` | Opcional; lista de itens `{ano_academico, materias}` | Opcional; lista de itens `{curso_id, ano_academico, materias}` | Opcional; lista de itens `{curso_id, ano_academico, materias}` com ano derivado dos semestres |
| `limite_materias_pendentes` | Regras fixas do sistema | Regras fixas do sistema; sem pendências escolares | Obrigatório, `>= 0` |
| `aplica_se_reprovado_em_type` | Ausente na raiz; presente em descendente | Ausente na raiz; presente em descendente | Ausente na raiz; presente em descendente |
| `nota_despertadora` | Fixo do sistema | Fixo do sistema | Opcional na raiz; rejeitado em descendente |

**Regras de cadeia e unicidade:**

- `type`, `nome`, `nivel`, `formula` e `nota_minima_aprovacao > 0` são obrigatórios na criação; `descricao` é opcional.
- `type` aceita letras, números, espaços e `_`; espaços internos são normalizados para `_`.
- `categorias_envolvidas` é extraído da `formula`; se enviado, deve bater exatamente com as categorias extraídas.
- Não pode haver duas regras ativas com o mesmo `codigo_academia`, `nivel`, `type` e escopo sobreposto. No Fundamental a sobreposição é por `ano_academico`; no Médio é por par `curso_id` + `ano_academico`. Superior continua sem `anos_academicos` nesta mudança.
- Deve haver no máximo uma raiz ativa por academia, `nivel` e escopo. Uma cadeia sem raiz ou com múltiplas raízes aplicáveis não é executável de forma determinística.
- `nota_despertadora` só pode ser configurado na regra raiz superior. Nas regras escolares fixas, a raiz é despertada por `prova_trimestral`, `exame_final` ou `nota_pap`, e a descendente fixa `exame_recurso` é despertada diretamente por `exame_recurso` apenas quando existe reprovação anterior. Regra superior raiz antiga sem `nota_despertadora` continua válida para compatibilidade, mas não é despertada automaticamente por lançamento de nota.
- Descendentes superiores devem apontar para regra ativa existente no mesmo `nivel`, não podem apontar para si mesmas, não podem criar ciclo e devem usar exatamente o mesmo escopo da raiz da cadeia: mesmos anos no Fundamental e mesmos pares `curso_id` + `ano_academico` no Médio. Descendentes não expõem, aceitam nem persistem `nota_despertadora`; payload com esse campo falha porque descendentes são ativadas pela reprovação na ancestral.
- Inativar uma regra inativa também suas dependentes diretas/indiretas para preservar a consistência da cadeia.

**Exemplos funcionais de configuração:**

| Cenário | Configuração típica |
|---|---|
| Raiz do Fundamental | Não configurável por endpoint na versão 2.1.0; o padrão escolar fundamental é fixo do sistema e exposto como regra `source="system"`, `fixed=true`, `readonly=true`. |
| Recuperação do Fundamental | Não configurável por endpoint; o sistema fornece as etapas fixas com `exame_final` e `exame_recurso` para os anos oficiais. |
| Raiz do Médio | Não configurável por endpoint na versão 2.1.0; o padrão escolar médio é fixo do sistema. Matérias removidas do modelo continuam sendo configuração curricular do curso médio/ano, não da regra. |
| Descendente do Médio | Não configurável por endpoint; o sistema fornece `exame_recurso` fixo para `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`, limitado às matérias reprovadas na avaliação final. |
| Raiz do Superior | `nivel=superior`, sem `anos_academicos`, fórmula com referências `[categoria]`, `limite_materias_pendentes` definido. |
| Superior com pendência | Mesma regra superior, matérias com `pendencia_permitida=true` e, quando aplicável, `pendencia_nivel_conclusao` indicando o semestre-limite. |

#### 15.1.3 Fórmulas por nível

A regra usa `formula` como texto declarativo. O parser valida referências, operadores, parênteses, constantes, categorias e períodos antes de persistir ou calcular. Erro de fórmula é erro de validação, não falha operacional inesperada.

| Nível | Formato da referência | Exemplo válido | Exemplo inválido |
|---|---|---|---|
| Fundamental | `[categoria,periodo]` | `([prova_trimestral,1_trimestre]+[prova_trimestral,2_trimestre]+[prova_trimestral,3_trimestre])/3` | `[prova_trimestral]` |
| Médio | `[categoria,periodo]` | `[prova_trimestral,1_trimestre]*0.4+[exame_final,3_trimestre]*0.6` | `[exame_final]` |
| Superior | `[categoria]`; período inferido pela matéria/semestre avaliado | `([prova_parcelar_1]+[prova_parcelar_2])/2` | `[prova_parcelar_1,1_semestre]` |

No Superior, o backend preenche o período no momento do cálculo usando a matéria/escopo avaliado (`periodo` da matéria e `semestre_atual` do estudante). Assim, a mesma regra superior pode calcular as matérias do semestre atual sem expor período explícito no payload da regra.

Quando a nota recém-lançada dispara a avaliação final para uma matéria, qualquer referência da fórmula dessa mesma matéria que ainda não tenha nota registrada é calculada como `0` naquele momento. A substituição por zero fica restrita à matéria que recebeu a nota-gatilho (`nota_despertadora` no Superior ou o gatilho escolar fixo equivalente) e é registrada no snapshot de `resultados_materias.notas_substituidas_zero`; matérias que ainda não receberam o próprio gatilho não são forçadas a avaliar. A fórmula sempre lê notas do ano letivo atual, da mesma academia, do mesmo estudante, da matéria avaliada e de categorias extraídas da própria fórmula.

#### 15.1.4 Execução automática por lançamento de notas

1. A academia registra/atualiza nota; o backend valida ano letivo, estudante, matéria, categoria, período, escala numérica e pertencimento ao `ano_escolar_fundamental` ou `ano_escolar_medio` atual do estudante.
2. O backend infere o nível acadêmico do estudante para execução: Superior tem prioridade quando há vínculo/status superior; depois Médio; caso contrário Fundamental.
3. Para Superior, o backend transforma `semestre_atual` em `[n]_semestre` e valida esse período contra o curso.
4. O backend busca regras aplicáveis à academia, ao `nivel` e ao escopo acadêmico atual. Para `fundamental` e `medio`, essa busca é sempre resolvida pelo catálogo fixo do sistema; se a categoria lançada não despertar uma regra fixa, a execução termina sem consultar regras configuráveis/legadas. Para `superior`, a busca usa as regras configuráveis ativas da academia.
5. A execução automática da raiz continua quando a categoria da nota registrada é a `nota_despertadora` da raiz. No padrão escolar fixo, isso significa `prova_trimestral` no 3º trimestre para anos regulares, `exame_final` para anos com exame e `nota_pap` no `4_ano_medio` técnico.
6. Descendentes só são consideradas se a etapa anterior reprovou. A descendente escolar fixa `exame_recurso` também pode ser despertada diretamente por lançamento de `exame_recurso`, mas somente para matérias reprovadas na avaliação final anterior; notas ausentes exigidas pela fórmula dessa etapa e dessa matéria também são substituídas por zero no snapshot da avaliação.
7. Para cada regra executável, o backend resolve as matérias aplicáveis e calcula `nota_final` individual por matéria.
8. O resultado de cada matéria compara `nota_final` com `nota_minima_aprovacao`.
10. A decisão geral é derivada dos resultados por matéria e, no Superior, das condições de pendência.
12. Se a avaliação já existir no escopo idempotente, o backend não duplica o registro.

#### 15.1.5 Modelos escolares fixos de avaliação final por ano

Para escolas (`fundamental` e `medio`), a avaliação final não é configurável pela academia. O backend monta regras fixas em `regraAvaliacaoFinalEscolarFixa`, calcula o resultado **por matéria** e grava a etapa pública indicada em `type`. Em todas as fórmulas abaixo, cada referência lê a nota daquela mesma matéria, no ano letivo atual, no estudante e academia avaliados.

Convenções usadas nas fórmulas:

- `NP1`, `NP2`, `NP3` = `[nota_professor,1_trimestre]`, `[nota_professor,2_trimestre]`, `[nota_professor,3_trimestre]`.
- `PT1`, `PT2`, `PT3` = `[prova_trimestral,1_trimestre]`, `[prova_trimestral,2_trimestre]`, `[prova_trimestral,3_trimestre]`.
- `EF3` = `[exame_final,3_trimestre]`.
- `ER3` = `[exame_recurso,3_trimestre]`.
- `PAP3` = `[nota_pap,3_trimestre]`.
- A média trimestral regular é sempre `(nota_professor + prova_trimestral) / 2` em cada trimestre.
- A avaliação com exame mantém o 1º e 2º trimestres regulares e substitui a prova trimestral do 3º trimestre pelo exame final, isto é, o 3º trimestre vira `(nota_professor_3 + exame_final_3) / 2`.
- O `exame_recurso` não é uma média com notas anteriores: quando permitido, a nota final da etapa de recurso é exatamente a nota `exame_recurso` do 3º trimestre.

Fórmulas textuais exatamente no formato usado pelo backend:

```text
Regular sem exame:
(((([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre])/2)+(([nota_professor,2_trimestre]+[prova_trimestral,2_trimestre])/2)+(([nota_professor,3_trimestre]+[prova_trimestral,3_trimestre])/2))/3)

Com exame final:
(((([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre])/2)+(([nota_professor,2_trimestre]+[prova_trimestral,2_trimestre])/2)+(([nota_professor,3_trimestre]+[exame_final,3_trimestre])/2))/3)

Com exame de recurso:
[exame_recurso,3_trimestre]

PAP do 4º ano médio técnico:
[nota_pap,3_trimestre]
```

As mesmas fórmulas, em notação didática:

```text
Regular sem exame = (((NP1 + PT1) / 2) + ((NP2 + PT2) / 2) + ((NP3 + PT3) / 2)) / 3
Com exame final   = (((NP1 + PT1) / 2) + ((NP2 + PT2) / 2) + ((NP3 + EF3) / 2)) / 3
Com recurso       = ER3
PAP técnico       = PAP3
```

Tabela completa dos modelos fixos por ano escolar:

| Nível | Ano acadêmico | Categorias aceitas para notas | Etapa raiz (`type`) | Gatilho da etapa raiz | Fórmula da etapa raiz | Mínima | Recurso? | Fórmula do recurso | Gatilho do recurso | Mínima do recurso |
|---|---|---|---|---|---|---:|---|---|---|---:|
| Fundamental | `1_ano_fundamental` | `nota_professor`, `prova_trimestral` | `normal` | `prova_trimestral` | Regular sem exame | 5 | Não | — | — | — |
| Fundamental | `2_ano_fundamental` | `nota_professor`, `prova_trimestral` | `normal` | `prova_trimestral` | Regular sem exame | 5 | Não | — | — | — |
| Fundamental | `3_ano_fundamental` | `nota_professor`, `prova_trimestral` | `normal` | `prova_trimestral` | Regular sem exame | 5 | Não | — | — | — |
| Fundamental | `4_ano_fundamental` | `nota_professor`, `prova_trimestral` | `normal` | `prova_trimestral` | Regular sem exame | 5 | Não | — | — | — |
| Fundamental | `5_ano_fundamental` | `nota_professor`, `prova_trimestral` | `normal` | `prova_trimestral` | Regular sem exame | 5 | Não | — | — | — |
| Fundamental | `6_ano_fundamental` | `nota_professor`, `prova_trimestral`, `exame_final`, `exame_recurso` | `normal` | `exame_final` | Com exame final | 5 | Sim, apenas para matérias reprovadas na etapa `normal` | `ER3` | `exame_recurso` | 5 |
| Fundamental | `7_ano_fundamental` | `nota_professor`, `prova_trimestral` | `normal` | `prova_trimestral` | Regular sem exame | 10 | Não | — | — | — |
| Fundamental | `8_ano_fundamental` | `nota_professor`, `prova_trimestral` | `normal` | `prova_trimestral` | Regular sem exame | 10 | Não | — | — | — |
| Fundamental | `9_ano_fundamental` | `nota_professor`, `prova_trimestral`, `exame_final`, `exame_recurso` | `normal` | `exame_final` | Com exame final | 10 | Sim, apenas para matérias reprovadas na etapa `normal` | `ER3` | `exame_recurso` | 10 |
| Médio | `1_ano_medio` | `nota_professor`, `prova_trimestral` | `normal` | `prova_trimestral` | Regular sem exame | 10 | Não | — | — | — |
| Médio | `2_ano_medio` | `nota_professor`, `prova_trimestral` | `normal` | `prova_trimestral` | Regular sem exame | 10 | Não | — | — | — |
| Médio | `3_ano_medio` | `nota_professor`, `prova_trimestral`, `exame_final`, `exame_recurso` | `normal` | `exame_final` | Com exame final | 10 | Sim, apenas para matérias reprovadas na etapa `normal` | `ER3` | `exame_recurso` | 10 |
| Médio técnico | `4_ano_medio` | `nota_pap` | `normal` | `nota_pap` | `PAP3` | 10 | Não | — | — | — |

Observações importantes que vêm diretamente do comportamento fixo do backend:

- Nos anos sem exame, a `prova_trimestral` do 3º trimestre dispara a avaliação da matéria; se notas exigidas de professor ou prova trimestral de trimestres anteriores estiverem ausentes para essa mesma matéria, elas entram como `0` e são listadas em `notas_substituidas_zero`.
- Nos anos com exame (`6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`), a etapa raiz usa `exame_final` no lugar da prova trimestral do 3º trimestre. A prova trimestral do 3º trimestre pode existir como categoria, mas não entra nessa fórmula com exame.
- O recurso existe somente para `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`, depende de reprovação anterior na etapa `normal` e recalcula somente as matérias reprovadas. Se a matéria já foi aprovada na etapa `normal`, lançar `exame_recurso` para ela é inválido.
- O `4_ano_medio` só tem modelo fixo de avaliação final quando o curso médio é técnico; nessa situação a etapa final é a Prova de Aptidão Profissional (`nota_pap`) e não há avaliação regular por trimestres, exame final nem recurso.
- A aprovação geral escolar exige aprovação em todas as matérias avaliadas pela etapa aplicável; escolas não usam aprovação com pendência.

#### 15.1.6 Fundamental

- O escopo é `ano_escolar_fundamental` atual do estudante (`1_ano_fundamental` a `9_ano_fundamental`).
- O catálogo avaliativo fundamental é fixo do sistema; rotas configuráveis de regras não aceitam criação/edição/remoção para Fundamental. Regras superiores não aceitam `anos_academicos`.
- O backend avalia cada matéria fundamental ativa aplicável ao ano do estudante, respeitando `materias_aplicaveis` se configurado.
- Cada matéria recebe `nota_final` própria; aprovação direta exige que todas as matérias avaliadas atinjam a mínima.
- Uma ou mais matérias abaixo da mínima reprovam a etapa e podem acionar regra descendente por matéria reprovada.
- Fundamental não permite aprovação com pendência: regra do Ensino Primário e Iº Ciclo não tem `limite_materias_pendentes` e matérias do Ensino Primário e Iº Ciclo não aceitam `pendencia_permitida`/`pendencia_nivel_conclusao`.
- Aprovado em ano intermediário progride para o próximo ano do Ensino Primário e Iº Ciclo. Se a academia não oferta o próximo ano, o evento registra o motivo `academia_sem_oferta_do_proximo_ano_academico_fundamental`, mantém o ciclo em andamento e não adiciona turma automaticamente.
- Aprovado no `9_ano_fundamental` finaliza o ciclo do Ensino Primário e Iº Ciclo. Reprovado permanece no mesmo ano.

#### 15.1.7 Médio

- O escopo é o `ano_escolar_medio` atual do estudante, validado contra o curso médio ativo vinculado.
- O backend avalia matérias médias ativas do curso e ano atual conforme o padrão fixo escolar.
- O `exame_recurso` fixo recalcula apenas matérias reprovadas na `avaliacao_final` anterior; matéria aprovada não pode receber recurso.
- Médio escolar não permite matérias dependentes nem aprovação com pendência: `pendencia_permitida` e `pendencia_nivel_conclusao` são exclusivos do Superior.
- O `4_ano_medio` técnico usa apenas `nota_pap` (`Prova de Aptidão Profissional`) como avaliação final, com aprovação por `nota_pap >= 10`, sem trimestres, prova trimestral, exame final ou recurso.

Cenários típicos do Médio:

| Cenário | Resultado funcional |
|---|---|
| Matéria reprovada na avaliação final | Resultado por matéria fica reprovado e a decisão geral segue a regra escolar vigente, sem classificação curricular especial. |
| Matéria reprovada e aprovada em etapa descendente aplicável | A cadeia registra a nova etapa e a aprovação da descendente permite progressão/conclusão conforme a regra escolar vigente. |
| Matéria reprovada sem descendente aplicável | Reprovação no ano/etapa escolar conforme padrão fixo. |
| `4_ano_medio` técnico com `nota_pap >= 10` | Aprovação e conclusão do médio técnico. |
| `4_ano_medio` técnico com `nota_pap < 10` | Reprovação no ano final técnico. |

#### 15.1.8 Superior

- O escopo é o curso superior ativo e o `semestre_atual` do estudante, convertido para `1_semestre`, `2_semestre`, etc.
- O backend avalia matérias superiores ativas do curso cujo `periodo` corresponde ao semestre atual.
- Fórmulas superiores não declaram período; o período é preenchido automaticamente para cada matéria avaliada.
- Aprovação direta exige todas as matérias avaliadas com nota final maior ou igual à mínima.
- Reprovação em matéria aciona descendentes aplicáveis; descendentes também trabalham por matéria e podem ser restringidas por `materias_aplicaveis`.
- Após esgotar a cadeia superior, o estudante pode aprovar com pendência se o total de reprovações couber em `limite_materias_pendentes` e todas as matérias reprovadas permitirem pendência.
- Reprovação por limite excedido ou matéria sem pendência permitida mantém o estudante no mesmo `semestre_atual` e não altera o status superior.
- Aprovação em semestre intermediário incrementa `semestre_atual` e recalcula `ano_superior`; aprovação no último semestre finaliza o ciclo superior.
- Pendência de curso anterior permanece histórica e não bloqueia o curso atual.

#### 15.1.9 Regras descendentes por matéria

Regra descendente é qualquer regra com `aplica_se_reprovado_em_type`. Ela representa uma etapa posterior da cadeia e só roda quando a etapa ascendente indicada reprovou. A descendente herda a lógica por matéria: calcula notas para matérias aplicáveis, compara cada resultado com a mínima e grava `type`/regra/fórmula usados naquela etapa.

Pontos importantes:

- A descendente não é uma média global do estudante; ela recalcula matérias no escopo da regra.
- `materias_aplicaveis` funciona como filtro: matéria fora da lista não é recalculada naquela etapa.
- A cadeia termina quando não há descendente ativa aplicável, quando a etapa anterior aprovou ou quando faltam notas para calcular a próxima etapa.
- Ao final da última etapa reprovada, apenas o Superior avalia se a reprovação vira pendência; Fundamental e Médio escolar permanecem reprovados quando não atendem ao padrão fixo.
- Exemplo: raiz `avaliacao_final` reprova Matemática e Física; descendente `avaliacao_final_com_exame` com `materias_aplicaveis=[Matemática]` recalcula somente Matemática. Física continua com o resultado anterior para a decisão final/pendência.

#### 15.1.10 Resultados por matéria, eventos, projeções e auditoria

Cada avaliação final gravada deve ser explicada pelos itens de `resultados_materias`, não por média global única. Cada item contém, no mínimo, `materia_id`, `nota_final`, `aprovado`, `type`, `formula_snapshot`, `regra_avaliacao_final_id`, `pendencia_permitida` e, quando aplicável, `notas_substituidas_zero` com as referências calculadas como zero por ausência de lançamento no momento do gatilho. A projeção também mantém `nota_final` agregada como média dos itens calculados para compatibilidade/consulta resumida, mas a decisão funcional é por matéria.

Eventos `AvaliacaoFinalEscolar` e `AvaliacaoFinalSuperior` preservam snapshots de regra, fórmula, notas calculadas, progressão e pendências geradas. Alterações posteriores de regra, matéria ou nota não reescrevem silenciosamente decisões já registradas; ajustes exigem fluxo operacional próprio/rebuild controlado.

#### 15.1.11 Pendências de matérias

Pendências existem apenas para o Superior. Elas são consideradas depois de reprovação na cadeia aplicável e só são criadas quando a decisão final superior é aprovação com pendência. Se o estudante reprova totalmente, nenhuma nova pendência é criada.

A pendência carrega funcionalmente: estudante, matéria, academia, curso, `nivel`, ano letivo, escopo acadêmico (`periodo_superior`), regra/evento de origem, status `pendente`, dados de origem/snapshot e timestamps. Há proteção contra duplicidade aberta no mesmo estudante, matéria, curso, nível, ano letivo e escopo. A estrutura também possui campos de baixa (`baixada_por_event_id`, `updated_at`) para histórico, mas a documentação funcional reconhece uma limitação atual: **não há rota pública consolidada de regularização/baixa de pendência exposta nesta documentação de API**. Portanto, o sistema já persiste e consulta a base de pendências abertas/históricas, mas a regularização operacional precisa ser implementada ou conduzida por fluxo administrativo/evento específico antes de ser tratada como rotina pública.

#### 15.1.12 Bloqueio por `pendencia_nivel_conclusao` e regularização

`pendencia_nivel_conclusao` pertence à matéria e deve ser usado para identificar pendências bloqueantes do curso atual. Funcionalmente:

- No Superior, pendência aberta cujo limite coincide com semestre/período conclusivo bloqueia conclusão automática até baixa.
- Aprovação com pendência pode permitir progressão intermediária, mas não deve permitir conclusão com pendência bloqueante do curso atual.
- Pendências não bloqueantes permitem progressão conforme regra de avaliação, desde que pertençam a escopo anterior e dentro do limite funcional definido.
- Pendências de curso anterior são históricas e não bloqueiam o curso atual.
- Regularização de pendência é diferente de avaliação final normal: deve avaliar a matéria pendente, registrar evento próprio auditável, baixar a pendência se aprovada e manter aberta se reprovada. Como limitação atual, esse fluxo ainda não está exposto como endpoint público completo; ao ser implementado, deve reutilizar os dados de origem da pendência e retomar progressão/conclusão quando não restarem pendências relevantes abertas.

#### 15.1.13 Cenários de erro e validação

Devem falhar com erro de validação ou bloqueio funcional:

- Payload de regra com `tipo_ensino`; use `nivel`.
- Academia mista criando regra sem `nivel` ou tentando criar regra `superior`.
- Academia não mista criando regra de nível incompatível com sua configuração.
- `anos_academicos` ausente em regra do Ensino Primário e Iº Ciclo ou presente em Médio/Superior.
- `limite_materias_pendentes` enviado em regra escolar, ausente em regra superior ou negativo.
- `materias_aplicaveis` fora do escopo do curso/ano/período aplicável deve ser tratada como configuração inválida ou ineficaz operacionalmente; QA deve validar esse cenário contra a base de matérias da academia.
- Descendente órfã, descendente que aponta para si mesma, ciclo de dependências ou escopo do Ensino Primário e Iº Ciclo diferente da raiz.
- Fórmula Fundamental/Médio sem período explícito (`[categoria]`).
- Fórmula Superior com período explícito (`[categoria,periodo]`).
- Fórmula com categoria inexistente, período inválido, divisão por zero, caracteres fora da gramática ou categorias enviadas que não batem com a fórmula.
- Tentativa de criar pendência em matéria fundamental.
- Tentativa de criar duplicidade de pendência aberta no mesmo escopo.
- Tentativa de concluir/progredir em desacordo com pendência bloqueante do curso atual.

#### 15.1.14 Consultas

- `GET /avaliacoes` → registros de avaliação final, com filtros por `nivel`, ano letivo, ano/período acadêmico atual, turma, academia e `type`. O filtro legado `tipo_ensino` é rejeitado no handler atual.
- `GET /aprovacoes` → apenas aprovados (`aprovado = TRUE`) com os mesmos filtros.
- `GET /reprovacoes` → reprovações definitivas; reprovações intermediárias com descendente ativa posterior não aparecem como definitivas até a cadeia terminar.
- `GET /academia/avaliacao-final/regras` → lista regras da academia autenticada.
- `PUT /academia/avaliacao-final/regras/:id` → edita apenas campos seguros de apresentação/cálculo (`nome`, `descricao`, `nota_minima_aprovacao`, `formula`).
- `DELETE /academia/avaliacao-final/regras/:id` → inativa a regra e suas dependentes em cascata.

**Escopo por academia:** usuário autenticado como academia só consulta/gerencia dados da própria academia. Admin pode consultar de forma ampla com filtros.
### Regras de Negócio — Avaliação Final


#### 15.1.15 Decisão futura para correção de notas após avaliação final

Na versão atual do sistema, notas permanecem imutáveis: elas podem ser criadas e consultadas, mas não há endpoint público, administrativo, batch ou assíncrono para editar, eliminar, restaurar ou substituir notas já registradas. Portanto, esta seção é uma decisão de produto para uma funcionalidade futura de correção de notas; ela não descreve um comportamento já implementado.

Quando uma funcionalidade de correção/edição de notas for especificada no futuro, ela deve verificar se o ano letivo ativo da academia ainda é o mesmo ano letivo da nota corrigida e da avaliação final já registrada para aquele estudante, matéria e escopo. Se for o mesmo ano letivo, a avaliação daquela matéria deve ser recalculada com a nota corrigida, e o resultado deve ser persistido como um novo evento auditável de reavaliação, distinto do evento original de avaliação final. O evento original deve permanecer preservado para auditoria. Se o ano letivo ativo já tiver avançado, a reavaliação automática não deve ocorrer; esse cenário deve ser definido junto com a própria funcionalidade de edição de notas. Qualquer implementação futura de edição/correção de notas deve reutilizar esta regra em vez de introduzir comportamento divergente sem revisão de produto.

### 15.2 Regras de Avaliação Final

| Regra                                       | Detalhe                                    |
| ------------------------------------------- | ------------------------------------------ |
| Escolas usam avaliação fixa do sistema | Academias escolares não criam, editam nem removem regras configuráveis de avaliação final |
| Superior usa regras configuráveis | Criação/edição/remoção de regra é permitida apenas para `nivel=superior` |
| Aprovação exige notas presentes na fórmula | Cada referência `[categoria]` superior exige nota da categoria no período inferido; se faltar, a avaliação aguarda novo lançamento |
| Observação não faz override de nota | `observacao` é apenas metadado; aprovação/reprovação vem de `nota_final >= nota_minima_aprovacao` |
| Fundamental usa sequência fixa 1..9 | `1_ano_fundamental` até `9_ano_fundamental` |
| Médio usa sequência do curso | Avança conforme `anos_academicos` do curso médio ativo vinculado |
| Tipo de ensino é inferido no backend | Não deve ser enviado no payload da avaliação final |
| Superior progride por semestre | `semestre_atual` define o período avaliado (`[n]_semestre`), aprovações intermediárias incrementam o semestre e `ano_superior = ceil(semestre_atual / 2)` |
| Reprovação não altera o ano/status | Mantém nível atual e não finaliza ciclo |
| Aprovação no último nível finaliza ciclo | Define o status do ciclo correspondente como `finalizado` |
| Uma avaliação por type/ano letivo/nível | Idempotência via aggregate e projeção |
| Ativação de regra dependente | Uma regra dependente inativa só pode ser ativada se o `type` apontado em `aplica_se_reprovado_em_type` estiver ativo |
| Reativação com `type` conflitante | Regra inativa não pode ser ativada enquanto houver regra ativa com o mesmo `type`, `nivel` e escopo acadêmico sobreposto |
| Aprovação escolar move para turma do próximo ano; reprovação escolar mantém na turma | Automático na projeção de turmas para fundamental/médio |
| Avaliação superior não altera turmas | Turmas do superior são geridas separadamente |


### Progressão semestral do ensino superior na avaliação final

Para `tipo_ensino = "superior"`, a avaliação final automática usa `semestre_atual` como unidade de progressão. O backend converte o inteiro armazenado no estudante para o período `[n]_semestre` (por exemplo, `semestre_atual = 3` vira `3_semestre`) e esse período deve existir em `curso.periodos`.

Regras superiores usam `nivel = "superior"` e não recebem `anos_academicos`; o backend infere o período semestral (`1_semestre`, `2_semestre`, ...) pela matéria/curso avaliado. Fundamental continua declarando `anos_academicos` como array simples de anos (`[n]_ano_fundamental`), médio passa a declarar `anos_academicos` como lista de escopos por curso (`[{"curso_id":"...","anos_academicos":["1_ano_medio"]}]`) e superior continua sem aceitar `anos_academicos`. A unicidade da avaliação final superior considera estudante, academia, ano letivo, `tipo_ensino`, semestre avaliado e `type`, portanto uma avaliação de `1_semestre` não bloqueia a posterior avaliação de `2_semestre` no mesmo ano letivo.

Na aprovação superior, o backend incrementa `semestre_atual` quando ainda existe próximo semestre no curso e recalcula `ano_superior = ceil(semestre_atual / 2)`. Assim, `1_semestre → semestre_atual = 2` mantém `1_ano_superior`, enquanto `2_semestre → semestre_atual = 3` muda para `2_ano_superior`. Na aprovação no último semestre, `status_superior` passa para `finalizado`; na reprovação, `semestre_atual`, `ano_superior` e `status_superior` permanecem inalterados.

O cliente não envia `proximo_ano_academico`, `proximo_semestre_atual` nem resultado de aprovação: a fórmula da regra calcula `nota_final`, compara com `nota_minima_aprovacao` e emite o evento auditável com `semestre_atual`, `proximo_semestre_atual`, `ano_superior_antes` e `ano_superior_depois` para rebuild determinístico.

### Execução automática da avaliação final

Não existe rota pública/registrada para executar avaliação final manualmente. Em `cmd/server/main.go`, a academia só registra notas (`POST /academia/notas-aluno`) e configura/lista regras (`POST /academia/avaliacao-final/regras`, `GET /academia/avaliacao-final/regras`); a avaliação final é disparada automaticamente pelo backend quando uma nota é registrada.

**Por que o cliente não envia `type` para executar avaliação final:**

- O `type` da avaliação final executada (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.) vem da regra aplicável, não do payload de uma requisição manual.
- Ao registrar notas, o backend identifica o estudante, infere o `tipo_ensino`, descobre o ano acadêmico atual e busca todas as regras ativas aplicáveis àquele ano.
- A cadeia precisa ter exatamente uma regra raiz, isto é, a regra sem `aplica_se_reprovado_em_type`. A raiz pode declarar `nota_despertadora`, cujo valor é o `codigo` da categoria de nota que autoriza o disparo automático.
- O processamento automático começa na raiz somente quando a categoria da nota registrada é igual a `nota_despertadora`. Regras antigas sem esse campo não despertam automaticamente por nota.
- `nota_despertadora` é configurável apenas em regra raiz superior; regras dependentes/descendentes superiores rejeitam esse campo. No padrão escolar fixo, a descendente `exame_recurso` é despertada diretamente por `exame_recurso` quando já existe reprovação anterior na `avaliacao_final`.
- Cada regra dependente é alcançada pelo campo `aplica_se_reprovado_em_type`: por exemplo, `avaliacao_final_com_recurso` pode depender de reprovação em `avaliacao_final`, e `avaliacao_final_com_exame` pode depender de reprovação em `avaliacao_final_com_recurso`.
- O backend só executa uma dependente quando encontra reprovação no `type` pré-requisito. Se o pré-requisito aprovou, a dependente é encerrada e não executa. Se o pré-requisito ainda não existe, a dependente aguarda.
- Portanto, no Superior a ordem correta não é decidida pelo cliente nem pela categoria da nota recém-registrada; ela é calculada a partir da cadeia de regras configurada até a raiz. No escolar fixo, `prova_trimestral`, `exame_final`, `exame_recurso` e `nota_pap` seguem os gatilhos oficiais por ano acadêmico. Se uma categoria escolar não corresponder a um gatilho fixo, o backend não consulta regras configuráveis/legadas como fallback.

**Regras de execução automática:**

- Se não houver regra ativa aplicável no Superior, ou regra fixa despertada no escolar, nenhuma avaliação final é registrada.
- Se a regra raiz aplicável não tiver `nota_despertadora`, ou se a categoria da nota não corresponder ao código configurado, nenhuma avaliação final automática de raiz é registrada naquele lançamento. Exceção escolar: `exame_recurso` pode despertar a etapa fixa de recurso quando houver reprovação anterior.
- Se a cadeia aplicável não tiver exatamente uma raiz, o backend retorna erro para evitar ambiguidade.
- O backend evita duplicidade por `codigo_estudante`, `codigo_academia`, `ano_lectivo`, `tipo_ensino`, `ano_academico_atual` e `type`.
- Quando a categoria lançada é o gatilho da regra executada, notas exigidas pela fórmula e ausentes para a mesma matéria são substituídas por `0`, registradas em `resultados_materias.notas_substituidas_zero` e a avaliação não fica pendente indefinidamente.
- O gatilho da raiz executa somente a raiz; regras descendentes aguardam o próprio gatilho aplicável (por exemplo, `exame_recurso` no escolar fixo) e só executam se a etapa anterior já registrou reprovação para a matéria.
- Quando uma regra é executada, o backend calcula `nota_final`, define `aprovado = nota_final >= nota_minima_aprovacao`, calcula o próximo ano acadêmico e persiste o evento com snapshot da regra.
- O registro de nota retorna o campo `avaliacoes_finais_automaticas` com os resultados automáticos disparados naquele request. Para fundamental aprovado com próximo ano global ainda não ofertado pela academia, o item inclui `motivo_progressao = "academia_sem_oferta_do_proximo_ano_academico_fundamental"` e `sem_oferta_do_proximo_ano_academico_na_academia = true`; o estudante permanece em andamento no próximo ano global e não recebe turma automática.

**Exemplo de resposta parcial de `POST /academia/notas-aluno` quando uma avaliação é disparada:**

```json
{
  "message": "nota registrada com sucesso",
  "estudante": "ABC1234",
  "categoria": "nota_exame_final",
  "periodo": "3_trimestre",
  "avaliacoes_finais_automaticas": [
    {
      "message": "avaliação final registrada automaticamente",
      "tipo_ensino": "fundamental",
      "type": "avaliacao_final",
      "aprovado": true,
      "nota_final": 12.5,
      "nota_minima_aprovacao": 10,
      "resultado": "aprovado → 4_ano_fundamental",
      "turmas_removidas": ["T1A"],
      "motivo_progressao": "academia_sem_oferta_do_proximo_ano_academico_fundamental",
      "sem_oferta_do_proximo_ano_academico_na_academia": true
    }
  ]
}
```

---
### POST /academia/avaliacao-final/regras

Cria uma regra ativa de avaliação final para a academia autenticada **somente no ensino superior**. Regras escolares (`fundamental`/`medio`) são fixas do sistema na versão 2.1.0 e não são configuráveis pela academia.

**Proteção**: academia autenticada + `nivel="superior"`.

**Request:**

```json
{
  "type": "avaliacao_final",
  "nome": "Avaliação final superior",
  "descricao": "Média das avaliações do semestre",
  "nivel": "superior",
  "nota_minima_aprovacao": 10,
  "formula": "([prova_parcelar_1]+[prova_parcelar_2])/2",
  "limite_materias_pendentes": 2,
  "nota_despertadora": "prova_parcelar_2",
  "aplica_se_reprovado_em_type": null
}
```

**Campos e validações:**

- `type` — obrigatório. Identifica a etapa pública (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.). Aceita apenas letras, números, espaços e `_`; espaços são normalizados para `_` antes de persistir (ex.: `exame final` vira `exame_final`), e outros caracteres são rejeitados.
- `nome` — obrigatório. Exemplos: `Avaliação final`, `Avaliação final (com exame)` ou `Avaliação final (com recurso)`.
- `descricao` — opcional.
- `nivel` — na versão 2.1.0, as rotas configuráveis aceitam apenas `superior`. Payloads com `fundamental` ou `medio` retornam erro porque regras escolares são fixas do sistema. O campo legado `tipo_ensino` não é aceito.
- `anos_academicos` — rejeitado para `nivel="superior"`; escolas não configuram regras por esta rota.
- `materias_aplicaveis` — opcional para Superior; usa itens `{curso_id, ano_academico, materias}`. IDs duplicados no mesmo item e itens duplicados por par curso+ano são inválidos. Escolas não configuram regras por esta rota.
- `limite_materias_pendentes` — obrigatório para `nivel="superior"`; inteiro maior ou igual a zero.
- `nota_minima_aprovacao` — obrigatório e maior que zero.
- `categorias_envolvidas` — opcional. O backend extrai automaticamente as categorias usadas em `formula`. Se enviado, deve corresponder exatamente às categorias extraídas da fórmula, sem duplicatas, sobras ou omissões, e todas precisam estar ativas/configuradas pela academia para os anos da regra.
- `formula` — obrigatório; deve ser uma string textual no modelo `formula_textual_v1`. O formato JSON em árvore antigo foi removido e não é aceito.
- `nota_despertadora` — exclusivo de regra raiz. Quando informado, deve ser o `codigo` de uma categoria de nota ativa, não deletada, pertencente à academia autenticada e configurada para os anos/escopo da regra quando essa segmentação existir. O campo é opcional para compatibilidade: raiz sem `nota_despertadora` não dispara automaticamente por lançamento de nota.
- `aplica_se_reprovado_em_type` — opcional para regra raiz; obrigatório para regras dependentes. Quando informado, passa pela mesma normalização de `type`, deve apontar para regra ativa existente na mesma academia/tipo de ensino, não pode ser igual ao próprio `type`, não pode criar ciclo e obriga a regra dependente a usar exatamente os mesmos `anos_academicos` da regra raiz da cadeia. Uma regra dependente inativa não pode ser ativada enquanto a regra da qual ela depende estiver inativa. Payload de dependente com `nota_despertadora` é rejeitado com erro de validação claro, pois dependentes são acionadas por reprovação ancestral.


**Erro ao enviar `nota_despertadora` em regra descendente:**

```json
{
  "error": "nota_despertadora é permitida apenas em regras raízes; regras descendentes são acionadas por reprovação na ancestral"
}
```

**Unicidade e cadeia:**

- Não pode existir outra regra ativa com o mesmo `type`, `nivel` e escopo sobreposto para a mesma academia: ano acadêmico no Fundamental e par `curso_id` + `ano_academico` no Médio. Ao criar ou editar uma regra, é permitido definir um `type` igual ao de uma regra inativa; porém essa regra inativa não poderá ser reativada enquanto existir uma regra ativa com o mesmo `type`, `nivel` e escopo sobreposto.
- Para cada academia, `nivel` e escopo acadêmico, só pode haver uma regra raiz ativa. Regra raiz é a regra sem `aplica_se_reprovado_em_type`.
- Regras dependentes formam uma cadeia de novas chances no Superior; elas precisam manter o mesmo escopo superior da raiz e só executam depois de reprovação no `type` apontado.
- A regra é criada pelo backend com `status = "ativo"` e `version = 1`; esses campos não são enviados na criação.

**Fórmula textual (`formula_textual_v1`):**

A fórmula é uma expressão declarativa interpretada por parser próprio do backend, sem `eval`, sem JavaScript e sem execução dinâmica. O resultado numérico da expressão vira `nota_final`.

- Referência de nota: `[categoria,periodo]`, por exemplo `[nota_escola,1_trimestre]` ou `[nota_exame,2_semestre]`. Em regras de `nivel="superior"`, a fórmula pode referenciar apenas `[categoria]` porque o período é inferido pela matéria avaliada.
- Operadores permitidos: `+`, `-`, `*`, `/`.
- Precedência: `*` e `/` são calculados antes de `+` e `-`. Use parênteses para deixar médias e pesos explícitos.
- Constantes: números positivos ou zero com ponto decimal opcional, como `3`, `0.3` e `10.5`.
- Espaços são opcionais e ignorados.
- Períodos devem seguir os formatos validados pelo backend, como `1_trimestre`, `2_trimestre`, `3_trimestre` ou `[n]_semestre`.
- Cada categoria referenciada precisa pertencer à academia e estar ativa/configurada para os anos da regra; `categorias_envolvidas` é persistido a partir da extração da fórmula.
- Divisão por zero é bloqueada na validação quando o divisor é constante e também durante a execução.
- Fórmulas com caracteres fora da gramática, chamadas de função, chaves JSON, strings, `@`, `;`, comandos SQL ou JavaScript são rejeitadas.
- Se faltar nota para qualquer referência `[categoria,periodo]`, a avaliação fica aguardando novo lançamento.

**Exemplos válidos:**

```text
([nota_escola,1_trimestre]+[nota_escola,2_trimestre]+[nota_escola,3_trimestre])/3
([nota_escola,1_trimestre]*0.3)+([nota_escola,2_trimestre]*0.3)+([nota_exame,3_trimestre]*0.4)
[nota_escola,1_trimestre]+[nota_professor,1_trimestre]
```

**Exemplos inválidos:**

```text
{ "op": "..." }                  # modelo JSON antigo removido
[nota_escola]                     # falta período
[nota_escola,1_trimestre]/0       # divisão por zero
eval([nota_escola,1_trimestre])   # chamadas de função não são permitidas
[nota_inexistente,1_trimestre]    # categoria fora da academia/anos da regra
```

**Response 201:**

```json
{
  "message": "regra de avaliação final criada",
  "id": "7e5f0b8d-8c7a-4b1a-9f4c-1f4cfd0c2f11"
}
```

---

### GET /academia/avaliacao-final/regras

Lista todas as regras de avaliação final da academia autenticada, ordenadas por criação decrescente.

**Proteção**: academia autenticada.


**Request:** sem payload
**Response 200:**

```json
{
  "regras": [
    {
      "id": "7e5f0b8d-8c7a-4b1a-9f4c-1f4cfd0c2f11",
      "codigo_academia": "ACA001",
      "type": "avaliacao_final",
      "nome": "Avaliação final",
      "descricao": "Média dos três trimestres",
      "nivel": "fundamental",
      "anos_academicos": ["3_ano_fundamental"],
      "nota_minima_aprovacao": 10,
      "categorias_envolvidas": ["nota_escola"],
      "formula": "([nota_escola,1_trimestre]+[nota_escola,2_trimestre]+[nota_escola,3_trimestre])/3",
      "nota_despertadora": "nota_escola",
      "aplica_se_reprovado_em_type": null,
      "materias_aplicaveis": [],
      "limite_materias_pendentes": null,
      "status": "ativo",
      "version": 1
    }
  ],
  "total": 1
}
```

---


### PUT /academia/avaliacao-final/regras/:id

Edita uma regra ativa de avaliação final da academia autenticada **somente no ensino superior**. Por segurança, a edição é limitada aos campos que não mudam o desenho da cadeia: `nome`, `descricao`, `nota_minima_aprovacao`, `formula` e, apenas em regra raiz, `nota_despertadora`. O backend recalcula `categorias_envolvidas` a partir da nova fórmula.

**Proteção**: academia autenticada.

**Request:**

```json
{
  "nome": "Avaliação final atualizada",
  "descricao": "Média ponderada atualizada",
  "nota_minima_aprovacao": 10,
  "formula": "([nota_escola,1_trimestre]*0.3)+([nota_escola,2_trimestre]*0.3)+([nota_exame,3_trimestre]*0.4)",
  "nota_despertadora": "nota_exame"
}
```

**Validações de segurança:**

- O `id` precisa ser UUID válido e pertencer à academia autenticada.
- A regra precisa estar `ativo`; regras inativas não são editadas.
- `nome` é obrigatório e não pode ser vazio.
- `nota_minima_aprovacao` precisa ser maior que zero.
- `formula` passa pelo mesmo parser seguro da criação; categorias são extraídas da fórmula e precisam estar ativas/configuradas para os anos da regra.
- Se `categorias_envolvidas` for enviado por compatibilidade, deve bater exatamente com as categorias da fórmula.
- `nota_despertadora`, quando enviado em edição de raiz, é revalidado contra categorias ativas da academia; em edição de descendente, o campo é rejeitado.
- Ao editar, `version` aumenta em 1 e `updated_at` é atualizado. Avaliações finais já registradas continuam preservadas porque carregam `formula_snapshot` e `regra_avaliacao_final_id`.

**Response 200:**

```json
{
  "message": "regra de avaliação final atualizada",
  "id": "7e5f0b8d-8c7a-4b1a-9f4c-1f4cfd0c2f11"
}
```

---

### DELETE /academia/avaliacao-final/regras/:id

Inativa uma regra ativa de avaliação final da academia autenticada **somente no ensino superior**. A deleção é **lógica** (`status = "inativo"`), não física, para preservar histórico, auditoria e snapshots de avaliações já calculadas.

**Proteção**: academia autenticada.

**Comportamento em cadeia:**

- Se a regra tiver dependentes, o backend inativa também todas as dependentes diretas e indiretas.
- Essa cascata evita deixar regras órfãs apontando para um `type` inativo.
- Depois da inativação em cascata, uma regra dependente não pode ser ativada se a regra indicada em `aplica_se_reprovado_em_type` continuar inativa.
- Regra inativa cujo `type` conflite com outra regra ativa no mesmo `nivel` e escopo acadêmico sobreposto não pode ser ativada até que o conflito seja removido.
- A operação não apaga avaliações finais já registradas em `projection_avaliacao_final`; elas continuam auditáveis.
- Cada regra inativada recebe `version = version + 1` e `updated_at` novo.


**Request:** sem payload
**Response 200:**

```json
{
  "message": "regra de avaliação final inativada com dependentes",
  "id": "7e5f0b8d-8c7a-4b1a-9f4c-1f4cfd0c2f11",
  "total_inativadas": 3
}
```

---

### Avaliação final por matéria e pendências

A avaliação final automática calcula uma `nota_final` independente por matéria (`materia_id`) quando uma nota é lançada. O backend resolve o escopo da regra ativa, carrega somente as matérias aplicáveis ao estudante e filtra as notas por `materia_disciplinar_id`, em vez de usar uma massa única de notas do estudante.

**Regras por nível:**

- `nivel` é o campo oficial de escopo das regras (`fundamental`, `medio` ou `superior`).
- `tipo_ensino` é legado e não é aceito nos payloads de criação/edição de regra.
- `materias_aplicaveis` pode restringir uma regra descendente às matérias de recuperação/recurso.
- `limite_materias_pendentes` é obrigatório para Superior configurável e define quantas reprovações finais podem virar pendência; escolas usam padrão fixo do sistema.

**Fórmula e matérias:**

- Fundamental e médio usam referências no formato `[categoria,periodo]`.
- Superior pode usar `[categoria]`; o backend infere o período a partir da matéria avaliada.
- O resultado automático inclui `resultados_materias`, com `materia_id`, `nota_final`, `aprovado`, `type`, `formula_snapshot`, `regra_avaliacao_final_id` e `pendencia_permitida`.
- Para Superior, se todas as reprovações finais couberem em `limite_materias_pendentes` e todas as matérias reprovadas permitirem pendência, o evento é registrado com `aprovado=true` e `aprovado_com_pendencia=true`.
- Pendências geradas são projetadas em `projection_materias_pendentes`, com proteção contra duplicidade aberta para o mesmo estudante, matéria, curso, nível, ano letivo e escopo acadêmico.

**Exemplo de regra média com pendência:**

```json
{
  "type": "normal",
  "nome": "Fechamento anual do médio",
  "nivel": "medio",
  "limite_materias_pendentes": 2,
  "nota_minima_aprovacao": 10,
  "formula": "([prova,1_trimestre]+[prova,2_trimestre]+[prova,3_trimestre])/3"
}
```


```json
{
  "type": "normal",
  "nome": "Fechamento anual do médio",
  "nivel": "medio",
  "limite_materias_pendentes": 2,
  "nota_minima_aprovacao": 10,
  "formula": "([prova,1_trimestre]+[prova,2_trimestre]+[prova,3_trimestre])/3"
}
```


**Exemplo de regra superior com período inferido:**

```json
{
  "type": "normal",
  "nome": "Fechamento semestral superior",
  "nivel": "superior",
  "limite_materias_pendentes": 1,
  "nota_minima_aprovacao": 10,
  "formula": "([prova]+[trabalho])/2"
}
```

---

### GET /avaliacoes

Lista avaliações finais. Escopo varia por tipo de usuário.

**Proteção**: autenticado (qualquer tipo)

**Query Params:**

- `tipo_ensino` — filtro: `fundamental`, `medio`, `superior`
- `ano_letivo` — filtra por ano letivo
- `ano_academico_atual` — filtra pelo ano académico em que o estudante foi re/aprovado
- `codigo_turma` — filtra por turma (requer `codigo_academia` em consultas admin)
- `codigo_academia` — filtro de academia (admin); para academia autenticada, este filtro é sempre forçado ao seu próprio código
- `type` — filtra o tipo de avaliação final (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.)
- `limit` — quantidade máxima de itens retornados (padrão: 50; máximo: 100)
- `offset` — deslocamento inicial para paginação (padrão: 0)


**Request:** sem payload
**Response 200:**

```json
{
  "avaliacoes": [AvaliacaoFinalDTO],
  "total": 50,
  "total_geral": 240,
  "limit": 50,
  "offset": 0
}
```

`total` é a quantidade de avaliações retornadas na página atual. `total_geral` é a contagem total de avaliações no escopo e filtros aplicados, ignorando `limit`/`offset`.

---

### GET /aprovacoes

Lista apenas avaliações com `aprovado = true`.

**Proteção**: autenticado (qualquer tipo)

**Query Params:**

- `tipo_ensino` — filtro: `fundamental`, `medio`, `superior`
- `ano_letivo` — filtra por ano letivo
- `ano_academico_atual` — filtra pelo ano académico em que o estudante foi aprovado
- `codigo_turma` — filtra por turma (requer `codigo_academia` em consultas admin)
- `codigo_academia` — filtro de academia (admin); para academia autenticada, este filtro é sempre forçado ao seu próprio código
- `type` — filtra o tipo de avaliação final (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.)
- `limit` — quantidade máxima de itens retornados (padrão: 50; máximo: 100)
- `offset` — deslocamento inicial para paginação (padrão: 0)


**Request:** sem payload
**Response 200:**

```json
{
  "aprovacoes": [AvaliacaoFinalDTO],
  "total": 35,
  "total_geral": 180,
  "limit": 50,
  "offset": 0
}
```

`total` é a quantidade de aprovações retornadas na página atual. `total_geral` é a contagem total de aprovações no escopo e filtros aplicados, ignorando `limit`/`offset`.

---

### GET /reprovacoes

Lista apenas avaliações com `aprovado = false`.

**Proteção**: autenticado (qualquer tipo)

**Query Params:**

- `tipo_ensino` — filtro: `fundamental`, `medio`, `superior`
- `ano_letivo` — filtra por ano letivo
- `ano_academico_atual` — filtra pelo ano académico em que o estudante foi reprovado
- `codigo_turma` — filtra por turma (requer `codigo_academia` em consultas admin)
- `codigo_academia` — filtro de academia (admin); para academia autenticada, este filtro é sempre forçado ao seu próprio código
- `type` — filtra o tipo de avaliação final (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.)
- `limit` — quantidade máxima de itens retornados (padrão: 50; máximo: 100)
- `offset` — deslocamento inicial para paginação (padrão: 0)


**Request:** sem payload
**Response 200:**

```json
{
  "reprovacoes": [AvaliacaoFinalDTO],
  "total": 15,
  "total_geral": 60,
  "limit": 50,
  "offset": 0
}
```

`total` é a quantidade de reprovações retornadas na página atual. `total_geral` é a contagem total de reprovações no escopo e filtros aplicados, ignorando `limit`/`offset`.

---


### GET /avaliacoes-estudante/:codigo

Retorna avaliações finais de um estudante específico.

**Proteção**: autenticado + academia ou admin

**Query Params:**

- `limit` — quantidade máxima de itens retornados (padrão: 50; máximo: 100)
- `offset` — deslocamento inicial para paginação (padrão: 0)

**Request:** sem payload
**Response 200:**

```json
{
  "codigo_estudante": "ABC1234",
  "nome": "string",
  "avaliacoes": [AvaliacaoFinalDTO],
  "total": 3,
  "total_geral": 9,
  "limit": 50,
  "offset": 0
}
```

`total` é a quantidade de avaliações retornadas na página atual. `total_geral` é a contagem total de avaliações desse estudante no escopo permitido ao usuário, ignorando `limit`/`offset`.

---

---

## 16. Admins

### Processos de Negócio — Administração e Integridade

O escopo administrativo é dividido em dois prefixos reais do backend:

- `/dominis` — painel operacional; nesta seção ficam apenas as rotas administrativas que não pertencem a escopos funcionais próprios (gestão de admins, métricas e rebuild de projeções).
- `/admin` — configurações globais do sistema; as rotas de anos letivos permanecem documentadas no escopo próprio de anos letivos.

Todas as rotas abaixo exigem `Authorization: Bearer <token>` de um usuário com `user_type=admin`. Algumas rotas adicionam guards de role:

| Guard | Roles aceitas | Uso real |
| --- | --- | --- |
| `RequireAdmin()` | qualquer admin ativo e com e-mail verificado | Base dos grupos `/dominis` e `/admin` |
| `RequireAdm()` / permissão mínima `adm` | `adm` e `fpp` | Ativar/desativar admins; consultar admins |
| `RequireFPP()` | somente `fpp` | Criar admins, alterar roles e executar rebuilds |

### 16.1 Verificação de Integridade do Ledger

O sistema suporta verificação da cadeia de hashes do ledger para qualquer estudante:

```http
GET /verificar-integridade/:codigo
```

A função SQL `verify_hash_chain` verifica se todos os hashes encadeados são válidos. Se qualquer evento foi adulterado, a verificação retorna `integro = false` indicando a versão onde a cadeia foi quebrada.

> Observação: embora seja uma ferramenta importante para auditoria administrativa, esta rota está registrada no grupo autenticado geral (`/`) e não no grupo `/dominis`.

---

### 16.2 Rebuild de Projeções

Admins com role `fpp` podem reconstruir projeções a partir do ledger.

**Concorrência de rebuild**: o manager permite apenas **1 rebuild por vez** (lock global). Se outro rebuild já estiver em execução, o endpoint síncrono retorna `409 Conflict`.

**Antes de reconstruir**, o sistema verifica a integridade completa do ledger. Se qualquer aggregate estiver com hash chain inválida, o rebuild é abortado.

**Ordem de rebuild recomendada** (respeita dependências):

1. `admins`
2. `academias`
3. `cursos`, `materias`, `categorias_nota`
4. `estudantes`, `turmas`
5. `solicitacoes_matricula`, `solicitacoes_edicao_dados_estudante`
6. `notas`, `faltas`
7. `avaliacao_final`

A ordem acima reflete a lista explícita usada por `RebuildAllProjections`; novas projeções registradas devem ser incluídas nessa lista para evitar reconstrução apenas pelo fallback alfabético.

#### POST /dominis/projections/rebuild/:name

Reconstrói uma projeção do zero a partir do ledger.

**Proteção**: autenticado + admin role `fpp`

**Path Params:**

- `name` — nome da projeção registrada no projection manager (ex.: `admins`, `academias`, `estudantes`, `notas`).

**Request:** sem payload

**Response 200:**

```json
{
  "message": "projeção reconstruída com sucesso",
  "projection": "estudantes"
}
```

**Erros principais:**

| Status | Quando ocorre |
| --- | --- |
| `400`/`422` | `name` vazio ou inválido conforme validação do manager |
| `409` | já existe outro rebuild em andamento |
| `500` | projeção indisponível, falha interna ou integridade do ledger comprometida |

#### POST /dominis/projections/rebuild/:name/async

Enfileira o rebuild de uma projeção para execução em background. Use quando o rebuild puder demorar vários minutos.

**Proteção**: autenticado + admin role `fpp`

**Path Params:**

- `name` — nome da projeção registrada no projection manager (ex.: `admins`, `estudantes`, `notas`).

**Request:** sem payload

**Response 202:**

```json
{
  "message": "rebuild enfileirado com sucesso — use GET /jobs/:id ou GET /jobs/stream para acompanhar o progresso",
  "projection": "admins",
  "job_id": "8a362f5e-cfcd-4968-ab0a-b6a1cfce8812",
  "status": "pending",
  "total_items": 1,
  "poll_url": "/jobs/8a362f5e-cfcd-4968-ab0a-b6a1cfce8812",
  "sse_url": "/jobs/stream"
}
```

**Acompanhamento:**

- `GET /jobs/:id`
- `GET /jobs/stream` (SSE)

---

### 16.3 Regras de Admin

| Regra | Detalhe |
| --- | --- |
| Hierarquia estrita | Um admin só gerencia admins com role estritamente inferior. |
| Email verificado obrigatório para operar | Sem verificação, o middleware bloqueia acesso ao painel. |
| Apenas FPP cria admins | A rota `POST /dominis/register` tem `RequireFPP()` e ainda valida o criador no handler. |
| Apenas FPP altera roles | A rota `PUT /dominis/admin/:id/role` tem `RequireFPP()`. |
| ADM/FPP ativam e desativam | Rotas de ativação/desativação usam `RequireAdm()` e validação hierárquica no aggregate. |
| Não pode desativar a si próprio | Prevenção de bloqueio acidental em `PUT /dominis/admin/:id/desativar`. |
| Não pode alterar o próprio role | Prevenção explícita em `PUT /dominis/admin/:id/role`. |
| Bootstrap único | Primeiro FPP é criado via `POST /bootstrap` com advisory lock. |
| Senha gerada automaticamente | Senha segura gerada com `crypto/rand`, persistida com bcrypt e enviada por email; a resposta HTTP nunca expõe a senha. |
| Falha de email não reverte criação | Se o email de boas-vindas falhar, a API retorna `201` com `aviso="email_nao_enviado"`. |
| Auditoria | Ações administrativas relevantes registram eventos de auditoria no aggregate Admin. |

---

### 16.4 Gestão de administradores

#### POST /dominis/register

Cria um novo admin. A senha temporária é gerada automaticamente, persistida apenas como hash bcrypt e enviada por email.

**Proteção real**: autenticado + admin role `fpp`.

**Request:**

```json
{
  "nome": "string",
  "email": "admin.exemplo@dominio.com",
  "telefone": "923456789",
  "role": "gerente"
}
```

**Validações e regras reais:**

- `nome`, `email`, `telefone` e `role` são obrigatórios.
- `telefone` deve ser informado para qualquer role (`fpp`, `adm` ou `gerente`) e seguir o formato nativo de 9 dígitos, sem DDI.
- `role` deve ser `fpp`, `adm` ou `gerente`.
- O criador deve existir como admin e ter role `fpp`.
- O aggregate também valida hierarquia via `ValidatePermission(role)`.
- Email duplicado retorna conflito.
- A senha temporária não aparece na resposta.
- Se o envio de email falhar, o admin continua criado e a resposta inclui `aviso="email_nao_enviado"`.

**Response 201 — email enviado:**

```json
{
  "message": "administrador criado com sucesso. A senha temporária foi enviada por email.",
  "data": {
    "id": "uuid",
    "nome": "string",
    "email": "admin.exemplo@dominio.com",
    "telefone": "923456789",
    "role": "gerente"
  }
}
```

**Response 201 — criado, mas email falhou:**

```json
{
  "message": "administrador criado com sucesso. ATENÇÃO: falha ao enviar email — solicite reset de senha via /recuperar-senha/solicitar.",
  "data": {
    "id": "uuid",
    "nome": "string",
    "email": "admin.exemplo@dominio.com",
    "telefone": "923456789",
    "role": "gerente"
  },
  "aviso": "email_nao_enviado"
}
```

#### GET /dominis/admin-lista

Lista todos os admins.

**Proteção real**: autenticado + admin; o handler exige permissão mínima `adm` (`adm` ou `fpp`).

**Request:** sem payload

**Response 200:**

```json
{
  "admins": [
    {
      "id": "uuid",
      "nome": "string",
      "email": "admin.exemplo@dominio.com",
      "email_verificado": true,
      "role": "adm",
      "status": "ativo",
      "telefone": "+244900000000",
      "telefone_verificado": false,
      "created_at": "2026-01-01T10:00:00Z",
      "updated_at": "2026-01-02T10:00:00Z",
      "created_by": "uuid",
      "total_acoes_realizadas": 12
    }
  ],
  "total": 1
}
```

> Cada item usa o `AdminDTO`, portanto inclui `email_verificado` para indicar se o e-mail do administrador foi confirmado. A serialização remove defensivamente `senha_hash`, embora o DTO já não exponha esse campo.

#### GET /dominis/consultar-admin/:email

Busca um admin pelo email.

**Proteção real**: autenticado + admin; o handler exige permissão mínima `adm` (`adm` ou `fpp`).

**Path Params:**

- `email` — email do admin consultado.

**Response 200 — ADM:**

```json
{
  "admin": {
    "id": "uuid",
    "nome": "string",
    "email": "admin.exemplo@dominio.com",
    "email_verificado": true,
    "role": "adm",
    "status": "ativo",
    "created_at": "2026-01-01T10:00:00Z",
    "updated_at": "2026-01-02T10:00:00Z"
  }
}
```

**Response 200 — FPP inclui campos extras:**

```json
{
  "admin": {
    "id": "uuid",
    "nome": "string",
    "email": "admin.exemplo@dominio.com",
    "email_verificado": true,
    "role": "adm",
    "status": "ativo",
    "created_at": "2026-01-01T10:00:00Z",
    "updated_at": "2026-01-02T10:00:00Z",
    "created_by": "uuid",
    "total_acoes_realizadas": 12
  }
}
```

#### PUT /dominis/admin/:id/ativar

Ativa um admin inativo.

**Proteção real**: autenticado + admin role `adm` ou `fpp`, com validação hierárquica contra o role do admin alvo.

**Path Params:**

- `id` — UUID do admin alvo.

**Request:** sem payload

**Response 200:**

```json
{
  "message": "administrador ativado com sucesso",
  "email": "admin.exemplo@dominio.com"
}
```

#### PUT /dominis/admin/:id/desativar

Desativa um admin ativo.

**Proteção real**: autenticado + admin role `adm` ou `fpp`, com validação hierárquica contra o role do admin alvo.

**Path Params:**

- `id` — UUID do admin alvo.

**Request:**

```json
{
  "motivo": "string"
}
```

**Validações reais:**

- `motivo` é obrigatório.
- O admin autenticado não pode desativar a própria conta.

**Response 200:**

```json
{
  "message": "administrador desativado com sucesso",
  "email": "admin.exemplo@dominio.com"
}
```

#### PUT /dominis/admin/:id/role

Altera o role de um admin.

**Proteção real**: autenticado + admin role `fpp`, com validação hierárquica contra o role anterior do admin alvo.

**Path Params:**

- `id` — UUID do admin alvo.

**Request:**

```json
{
  "novo_role": "adm"
}
```

**Validações reais:**

- `novo_role` é obrigatório.
- `novo_role` deve obedecer às regras do aggregate Admin.
- O admin autenticado não pode alterar a própria role.

**Response 200:**

```json
{
  "message": "role atualizado com sucesso",
  "role_anterior": "gerente",
  "novo_role": "adm"
}
```

#### PUT /dominis/admin/:id/dados

Atualiza somente o nome de um admin. Email e telefone do próprio usuário autenticado usam rotas dedicadas.

**Proteção real**: autenticado + admin. Para editar outro admin, o executor deve ter role estritamente superior ao alvo; autoedição do próprio `nome` é permitida.

**Path Params:**

- `id` — UUID do admin alvo.

**Request:** pelo menos um campo deve ser informado.

```json
{
  "nome": "Novo Nome"
}
```

**Validações reais:**

- Body JSON deve ser válido.
- `nome` deve ser fornecido.
- `email` e `telefone` são rejeitados nesta rota; use `PUT /me/email` e `PUT /me/telefone` para o contato do admin autenticado. Nessas rotas dedicadas, mudanças efetivas resetam `email_verificado`/`telefone_verificado` para `false`; reenviar o mesmo valor não altera a flag.
- Ao editar outro admin, aplica-se a hierarquia estrita (`fpp` > `adm` > `gerente`); roles iguais ou superiores ao executor retornam `403`.

**Response 200:**

```json
{
  "message": "dados atualizados com sucesso"
}
```

---

### 16.5 Métricas administrativas

#### GET /dominis/metrics

Retorna métricas do sistema (requisições, erros, autenticação e latência por endpoint).

**Proteção real**: autenticado + admin. Para editar outro admin, o executor deve ter role estritamente superior ao alvo; autoedição do próprio `nome` é permitida.

**Request:** sem payload

**Response 200:**

```json
{
  "metrics": {
    "uptime_seconds": 3600,
    "total_requests": 1500,
    "total_errors": 12,
    "total_auth_failures": 3,
    "requests_per_second": 0.42,
    "error_rate": 0.8,
    "endpoints": [
      {
        "path": "/login",
        "requests": 200,
        "errors": 5,
        "avg_latency": 45000,
        "error_rate": 2.5
      }
    ]
  }
}
```

#### GET /dominis/storage/quota

Retorna o uso de armazenamento da conta configurada no provider ativo, discriminando totais, uso gerenciado pela aplicação, uso fora das pastas de academias e arquivos/pastas encontrados na conta.

**Proteção real**: autenticado + admin. Para editar outro admin, o executor deve ter role estritamente superior ao alvo; autoedição do próprio `nome` é permitida.

**Request:** sem payload

**Response 200:**

```json
{
  "provider": "mega",
  "total_bytes": 53687091200,
  "used_bytes": 10737418240,
  "available_bytes": 42949672960,
  "managed_bytes": 8589934592,
  "outside_academias_bytes": 2147483648,
  "unmanaged_bytes": 2147483648,
  "total_human": "50.0 GB",
  "used_human": "10.0 GB",
  "available_human": "40.0 GB",
  "managed_human": "8.0 GB",
  "outside_academias_human": "2.0 GB",
  "unmanaged_human": "2.0 GB",
  "academias": [
    {
      "codigo_academia": "ACA-001",
      "used_bytes": 1048576,
      "used_human": "1.0 MB"
    }
  ],
  "account_files": [
    {
      "path": "/academias/ACA-001/documento.pdf",
      "name": "documento.pdf",
      "size_bytes": 1048576,
      "size_human": "1.0 MB",
      "managed": true
    }
  ],
  "account_folders": [
    {
      "path": "/academias/ACA-001",
      "name": "ACA-001",
      "size_bytes": 1048576,
      "size_human": "1.0 MB",
      "managed": true
    }
  ]
}
```

**Erros principais:**

| Status | Quando ocorre |
| --- | --- |
| `503` | provider de armazenamento indisponível ou falha ao obter quota |

## 17. Jobs Assíncronos

### Processos de Negócio — Sistema de Jobs Assíncronos

### 17.1 Sistema de Jobs Assíncronos

Para operações em lote com muitos itens, o sistema oferece endpoints `/async` que criam um **job em background**:

```
POST /academia/notas-aluno/async  →  { "job_id": "uuid", "status": "pending", "poll_url": "/jobs/:id", "sse_url": "/jobs/stream" }
GET  /jobs/:id                    →  { "status": "done", "progress": 100, ... }
```

**Pool de workers**: 4 goroutines paralelas processam itens em background.

**Polling recomendado**: intervalo de 1.5s a 8s com backoff exponencial.

**Status do job**: `pending` → `processing` → `done` / `failed`

Se qualquer item falhar, o job fica como `failed` (não `done`), permitindo que o cliente identifique e retente apenas os itens com falha.

**Persistência resiliente de payload e progresso**:

- Cada item processado é persistido imediatamente em `async_jobs.results` (sem janela de 10 itens).
- O `payload` bruto do job é preservado integralmente em `async_jobs.payload`.
- Cada resultado individual inclui também o `payload` do item, permitindo replay exato dos itens que falharam.
- Em reinício/crash, o worker varre jobs `pending`/`processing` e retoma do ponto salvo (`done_items + fail_items`), evitando perda silenciosa de itens.
- O enqueue dos endpoints `/async` valida e conta itens no body bruto (sem dupla serialização do array), reduzindo consumo de CPU/memória e janelas de timeout em lotes grandes (ex.: notas/faltas).

**Erro sempre explícito em jobs assíncronos**:

- Jobs com falha parcial agora finalizam com `status=failed` e `error` contendo o motivo com amostras de itens.
- Rebuild de projeções retorna o erro real no body HTTP, não apenas log interno.
- Rebuild assíncrono de projeções usa o mesmo pipeline de integridade do rebuild síncrono, mas sem manter a conexão HTTP aberta por minutos.

---


### GET /jobs

Lista os jobs recentes do usuário autenticado.

**Proteção**: autenticado (qualquer tipo)


**Request:** sem payload
**Response 200:**

```json
{
  "jobs": [JobSummary],
  "total": 5
}
```

---

### GET /jobs/:id

Retorna o status de um job específico.

**Proteção**: autenticado (apenas o dono do job)

**Query Params:**

- `results=true` — inclui resultados por item (payload maior)


**Request:** sem payload
**Response 200 (sem results):** `JobSummary`

**Response 200 (com results=true):**

```json
{
  "job": JobSummary,
  "results": [JobItemResult]
}
```

**Observações importantes:**

- `results[i].payload` contém o item original enviado no batch.
- Em falha parcial, `job.error` sempre contém o motivo consolidado (com amostras de itens com erro).

---

### GET /jobs/stream

Canal de notificações em tempo real via **Server-Sent Events (SSE)**.

**Proteção**: autenticado (qualquer tipo)

**Headers:**

```
Authorization: Bearer <jwt_token>
Accept: text/event-stream
```

**Eventos enviados:**

- `job_enqueued`
- `job_progress`
- `job_done`
- `job_failed`

**Exemplo:**

```text
event: job_progress
data: {"type":"job_progress","job_id":"uuid","job_type":"register_estudante_batch","status":"processing","progress":56,"done_items":560,"fail_items":0,"total_items":1000}
```

**Heartbeat:** o servidor envia `: ping` periodicamente para manter a conexão ativa.

**Request:** sem payload

**Response 200:** stream `text/event-stream` com eventos SSE `job_enqueued`, `job_progress`, `job_done` e `job_failed`.

---

### DELETE /jobs/:id/sse

Oculta um job do stream SSE da academia autenticada.

**Proteção**: autenticado + academia (apenas o dono do job)


**Request:** sem payload
**Response 200:**

```json
{
  "message": "job ocultado do stream SSE com sucesso",
  "job_id": "uuid"
}
```

---

### POST /jobs/:id/retry-failed

Cria um novo job de retry reaproveitando **somente os itens que falharam** no job original.

**Proteção**: autenticado + academia (apenas o dono do job)

**Regras:**

- O job original deve ter `fail_items > 0`.
- O novo job mantém o mesmo `job_type` do original.
- O payload do retry contém somente os `results[i].payload` com `sucesso = false`.


**Request:** sem payload
**Response 202:**

```json
{
  "message": "job de retry criado com sucesso",
  "original_job_id": "uuid",
  "retry_job_id": "uuid",
  "retry_items": 42,
  "status": "pending",
  "poll_url": "/jobs/uuid",
  "sse_url": "/jobs/stream"
}
```

---

---

## 18. Batch Assíncrono

### Processos de Negócio — Operações em Lote

### 18.1 Batch Assíncrono

Endpoints `/async` criam um job e retornam imediatamente. O processamento ocorre em background.

**Limites:** até 1000-2000 itens (dependendo do endpoint).

**Formato de payload:** array JSON (não objeto com `items`).

```json
[
  {"...": "payload do endpoint síncrono equivalente"}
]
```

**Fluxo:**

```
POST /academia/notas-aluno/async  →  202 Accepted + { job_id }
(polling)
GET  /jobs/:id                    →  { status, progress, done_items, fail_items }
(quando status = "done" ou "failed")
GET  /jobs/:id?results=true       →  { ... resultados por item ... }
```

**Gestão de stream SSE por academia:**

- `DELETE /jobs/:id/sse`: remove/oculta um job específico do stream `GET /jobs/stream` da própria academia.

**Retry parcial de falhas:**

- `POST /jobs/:id/retry-failed`: cria um novo job com o mesmo tipo do original, reenviando apenas os itens com falha (`sucesso=false`).
- Isso evita reprocessar itens que já deram certo.

**Cobertura adicional em 1.0.9 (SSE padronizado em todas as respostas `/async` + novidades prévias):**
- Academia: `PUT /academia/dados/async`, `POST /academia/categorias-nota/async`, `DELETE /academia/categorias-nota/async`.
- Cursos: `PUT /academia/curso/ativar|desativar|dados/async`, `DELETE /academia/curso/async`.
- Matérias: `PUT /academia/materia/ativar|desativar|periodo|dados/async`, `DELETE /academia/materia/async`.
- Turmas: `PUT /academia/turma/ativar|desativar|dados/async`, `DELETE /academia/turma/async`, `DELETE /academia/turma/estudante/async`.
- Admin: `PUT /dominis/admin/ativar/async`, `PUT /dominis/admin/desativar/async`.
- Consulta única por papel: `GET /turmas-estudante/:codigo` (estudante: próprio, academia: da sua academia, admin: qualquer estudante).

---


Todos criam um job e retornam `202 Accepted` com URLs de acompanhamento por polling e por SSE.
Use `poll_url` (`GET /jobs/:id`) e/ou `sse_url` (`GET /jobs/stream`).

**Formato de payload (todos os endpoints `/async`):**

```json
[
  {"...": "objeto com o mesmo payload do endpoint síncrono equivalente"}
]
```

**Regras do payload:**

- O array é obrigatório e deve conter pelo menos 1 item.
- Cada objeto do array deve seguir exatamente o mesmo contrato de payload da versão síncrona da rota.
- O limite máximo de itens por requisição depende do endpoint (tabela abaixo).
- O servidor valida e conta itens diretamente no payload bruto do request (sem dupla serialização), reduzindo risco de timeout no enqueue de lotes grandes.

> Exceção de formato: `POST /academia/estudante/register/async` mantém o contrato específico de cadastro em massa com `com_arquivo` descrito na seção da rota. Tanto JSON sem arquivos (`com_arquivo:false`) quanto multipart com arquivos (`com_arquivo=true`) criam job de background e retornam `202`.

**Response 202 (para endpoints que criam job):**

```json
{
  "message": "job criado com sucesso — use GET /jobs/:id ou GET /jobs/stream para acompanhar o progresso",
  "job_id": "uuid",
  "total_items": 500,
  "status": "pending",
  "poll_url": "/jobs/uuid",
  "sse_url": "/jobs/stream"
}
```

|Endpoint|Payload por item|Resposta|Limite|
|---|---|---|---|
|`POST /academia/estudante/register/async`|`{com_arquivo:false, estudantes:[...]}` ou `multipart/form-data` com `com_arquivo=true`|`202` (job criado)|100|
|`POST /academia/notas-aluno/async`|igual ao `POST /academia/notas-aluno`|`202` (job criado)|2000|
|`POST /academia/faltas-aluno/async`|igual ao `POST /academia/faltas-aluno`|`202` (job criado)|2000|
|`POST /academia/curso/async`|igual ao `POST /academia/curso`|`202` (job criado)|200|
|`POST /academia/materia/async`|igual ao `POST /academia/materia`|`202` (job criado)|500|
|`POST /academia/turma/async`|igual ao `POST /academia/turma`|`202` (job criado)|200|
|`POST /academia/turma/estudante/async`|igual ao `POST /academia/turma/:codigo/estudante`|`202` (job criado)|1000|
|`PUT /academia/dados/async`|igual ao `PUT /academia/dados`|`202` (job criado)|200|
|`POST /academia/categorias-nota/async`|igual ao `POST /academia/categorias-nota`|`202` (job criado)|500|
|`DELETE /academia/categorias-nota/async`|igual ao `DELETE /academia/categorias-nota/:codigo` (`codigo` vai no item)|`202` (job criado)|500|
|`PUT /academia/curso/ativar/async`|igual ao `PUT /academia/curso/:id/ativar` (`id` vai no item)|`202` (job criado)|500|
|`PUT /academia/curso/desativar/async`|igual ao `PUT /academia/curso/:id/desativar` (`id` vai no item)|`202` (job criado)|500|
|`PUT /academia/curso/dados/async`|igual ao `PUT /academia/curso/:id/dados` (`id` vai no item)|`202` (job criado)|500|
|`DELETE /academia/curso/async`|igual ao `DELETE /academia/curso/:id` (`id` vai no item)|`202` (job criado)|500|
|`PUT /academia/materia/ativar/async`|igual ao `PUT /academia/materia/:id/ativar` (`id` vai no item)|`202` (job criado)|1000|
|`PUT /academia/materia/desativar/async`|igual ao `PUT /academia/materia/:id/desativar` (`id` vai no item)|`202` (job criado)|1000|
|`PUT /academia/materia/dados/async`|igual ao `PUT /academia/materia/:id/dados` (`id` vai no item)|`202` (job criado)|1000|
|`DELETE /academia/materia/async`|igual ao `DELETE /academia/materia/:id` (`id` vai no item)|`202` (job criado)|1000|
|`PUT /academia/turma/ativar/async`|igual ao `PUT /academia/turma/:codigo/ativar` (`codigo_turma` vai no item)|`202` (job criado)|500|
|`PUT /academia/turma/desativar/async`|igual ao `PUT /academia/turma/:codigo/desativar` (`codigo_turma` vai no item)|`202` (job criado)|500|
|`PUT /academia/turma/dados/async`|igual ao `PUT /academia/turma/:codigo/dados` (`codigo_turma` vai no item)|`202` (job criado)|500|
|`DELETE /academia/turma/async`|igual ao `DELETE /academia/turma/:codigo` (`codigo_turma` vai no item)|`202` (job criado)|500|
|`DELETE /academia/turma/estudante/async`|igual ao `DELETE /academia/turma/:codigo/estudantes/:codigo_estudante` (`codigo_turma` + `codigo_estudante` no item)|`202` (job criado)|1000|

---

**Formato de payload (todos os endpoints `/async`):**

```json
[
  {"...": "objeto com o mesmo payload do endpoint síncrono equivalente"}
]
```

**Response 202 (igual para todos):**

```json
{
  "message": "job criado com sucesso — use GET /jobs/:id ou GET /jobs/stream para acompanhar o progresso",
  "job_id": "uuid",
  "total_items": 500,
  "status": "pending",
  "poll_url": "/jobs/uuid",
  "sse_url": "/jobs/stream"
}
```

|Endpoint|Proteção|Payload por item|Resposta|Limite|
|---|---|---|---|---|
|`POST /dominis/academia/register/async`|admin|igual ao `POST /dominis/academia/register`|`202` (job criado)|500|
|`PUT /dominis/academia/ativar/async`|admin role `adm`|igual ao `PUT /dominis/academia/:codigo/ativar`|`202` (job criado)|500|
|`PUT /dominis/academia/desativar/async`|admin role `adm`|igual ao `PUT /dominis/academia/:codigo/desativar`|`202` (job criado)|500|
|`PUT /dominis/admin/ativar/async`|admin role `adm`|igual ao `PUT /dominis/admin/:id/ativar` (`id` vai no item)|`202` (job criado)|500|
|`PUT /dominis/admin/desativar/async`|admin role `adm`|igual ao `PUT /dominis/admin/:id/desativar` (`id` + `motivo` no item)|`202` (job criado)|500|


---

---

---

### Inventário de cobertura das rotas ativas

A documentação cobre todas as rotas registradas em `cmd/server/main.go`: públicas (`/health`, `/login`, `/bootstrap`, `/solicitacao-matricula`, `/email/*`, `/academias`, `/academia/cursos`, `/academia/curso/:id`, `/consultar-academia/:codigo`), jobs (`/jobs`, `/jobs/:id`, `/jobs/stream`, `/jobs/:id/sse`, `/jobs/:id/retry-failed`), autenticadas globais (`/logout`, `/alterar-senha`, `/meu-perfil`, `/eventos-estudante/:codigo`, `/verificar-integridade/:codigo`, `/consultar-estudante/:codigo`, `/estudantes`, `/avaliacoes`, `/aprovacoes`, `/reprovacoes`, `/notas`, `/faltas`, `/notas-estudante/:codigo`, `/faltas-estudante/:codigo`, `/ano-letivo`, `/anos-letivos-lista`, `/anos-letivos/configuracoes`, `/solicitacoes-matricula`, `/documentos/*`, `/avaliacoes-estudante/:codigo`, `/turmas-estudante/:codigo`), estudante (`/estudante/*`), academia (`/academia/*`), dominis/admin (`/dominis/*`, `/admin/*`) e todos os endpoints `/async`.



## 19. Financeiro / AppyPay

### Processos e Regras de Negócio — Financeiro / AppyPay e mensalidades

O módulo financeiro integra o backend com a AppyPay para gerir credenciais, criar cobranças GPO/REF, gerar QR Codes GPO, consultar cobranças e receber webhooks do gateway. As operações financeiras são auditadas no ledger com aggregate `Financeiro`; as tabelas `financeiro_*` funcionam como projeções/read models e índices operacionais de consulta e idempotência.

**Regras gerais do escopo financeiro:**

- Todas as rotas `/financeiro/*` exigem autenticação. As rotas de administração aceitam somente `academia` ou admin FPP; a consulta de mensalidades e a consulta do histórico de cobranças (seção 19.8) de um estudante também podem ser feitas pelo próprio estudante autenticado.
- Academia autenticada opera apenas no próprio contexto: o backend força `contexto_tipo="academia"` e `codigo_academia` igual ao código do token, mesmo que esses campos venham vazios no request.
- Admin FPP pode operar o contexto global `spuri` e contextos de academias específicas; admins `adm` e `gerente`, estudantes e usuários anônimos não administram o módulo financeiro.
- Segredos AppyPay (`client_secret`, métodos de pagamento sensíveis) nunca são devolvidos em resposta; a API retorna apenas máscaras e metadados. A única exceção deliberada é o segredo de webhook (`webhook_secret`): como é gerado pelo servidor e o usuário precisa colá-lo no painel da AppyPay, ele é devolvido em texto plano apenas na criação da credencial (seção 19.1) e nas rotas dedicadas de consulta/rotação (seções 19.12 e 19.13) — nunca em `PUT`, listagem ou qualquer outra resposta.
- `ENV=development` ou `ENV=test` usa o gateway TEST; `ENV=production` usa o gateway PROD. O ambiente persistido em credenciais e cobranças segue essa resolução do backend.
- Cada cobrança ou QR Code exige credenciais ativas para o contexto resolvido antes de chamar a AppyPay.
- Todo valor monetário do módulo usa `float64`, em conformidade com o `number<double>` da AppyPay. Valores de entrada devem ter no máximo duas casas decimais; antes de chamar o gateway o backend aplica arredondamento *half away from zero* a duas casas, e comparações monetárias usam tolerância de meio cêntimo. Os futuros campos `ValorMensalidade`, `ValorMatricula` e equivalentes devem reutilizar esse mesmo contrato.
- O cancelamento de uma cobrança REF, GPO ou QR Code é exclusivamente interno ao Spuri: a AppyPay não documenta endpoint de cancelamento para esses métodos. Por isso, o cancelamento deixa de exibir/cobrar pela plataforma, mas não invalida tecnicamente uma referência ou QR já emitido no banco/gateway até a expiração; qualquer sucesso detectado depois dele é registrado como conflito para reconciliação manual FPP.
- Os webhooks são públicos por necessidade do gateway, mas autenticados pelo segredo de webhook gerado automaticamente na criação da credencial, enviado pela AppyPay num único cabeçalho HTTP fixo para toda a plataforma (`webhook_header_name`, sempre `X-Spuri-Webhook-Secret`). Eventos aceitos ou duplicados respondem `200` e são tratados de forma idempotente pelo identificador do evento.
- A taxa de matrícula é configurada pela academia ou pelo admin FPP em `/financeiro/matriculas/configuracoes` (seção 19.21), mas a cobrança em si nunca é criada por uma rota de `/financeiro/*`: ela nasce quando o próprio candidato aprovado paga pelas rotas públicas da seção 9, especialmente 9.10 e 9.11.
- Erros das rotas autenticadas seguem o envelope global `{error, message, request_id, details?}`. Webhooks públicos retornam apenas status HTTP para reduzir acoplamento com o gateway.

| Método | Rota | Escopo resumido |
|---|---|---|
| `POST` | `/financeiro/appypay/credenciais` | Cria/configura credenciais cifradas para `spuri` ou `academia`. |
| `PUT` | `/financeiro/appypay/credenciais/:id` | Substitui a configuração de uma credencial existente pelo `id`. |
| `GET` | `/financeiro/appypay/credenciais` | Lista credenciais mascaradas por contexto autorizado. |
| `GET` | `/financeiro/appypay/credenciais/:id/webhook-secret` | Consulta o segredo de webhook atual (texto plano) de uma credencial. |
| `POST` | `/financeiro/appypay/credenciais/:id/webhook-secret/rotacionar` | Gera um novo segredo de webhook, invalidando o anterior. |
| `POST` | `/financeiro/appypay/cobrancas` | Cria cobrança AppyPay GPO ou REF genérica. |
| `POST` | `/financeiro/appypay/qr-codes` | Gera QR Code GPO e devolve `qrCodeArr` em base64 quando enviado pela AppyPay. |
| `GET` | `/financeiro/appypay/cobrancas/:id` | Consulta cobrança por id AppyPay ou `merchantTransactionId`. |
| `GET` | `/financeiro/cobrancas` | Lista cobranças (mensalidade, matrícula, avulsa) do contexto autorizado, com filtros e paginação. |
| `GET` | `/financeiro/cobrancas/estudante/:codigo` | Lista todas as cobranças de um estudante, em qualquer estado — acessível ao próprio estudante. |
| `POST` | `/financeiro/appypay/cobrancas/:id/cancelar` | Cancela localmente uma cobrança pendente do próprio contexto. |
| `POST` | `/financeiro/mensalidades/configuracoes` | Versiona o valor e mês final de cobrança por ano/curso de academia privada. |
| `GET` | `/financeiro/mensalidades/configuracoes` | Lista a configuração vigente de mensalidade de uma academia. |
| `POST` | `/financeiro/mensalidades/inicio-cobranca` | Define o mês inicial excepcional para uma academia que entrou no ano letivo em curso. |
| `GET` | `/financeiro/mensalidades/estudante/:codigo` | Calcula sob consulta os meses devidos, pagos ou anulados, inclusive históricos. |
| `POST` | `/financeiro/mensalidades/obrigacoes/anular` | Anula, com evento por mês, obrigações de estudante da própria academia. |
| `POST` | `/financeiro/mensalidades/obrigacoes/reativar` | Reativa obrigações antes anuladas da própria academia. |
| `POST` | `/financeiro/matriculas/configuracoes` | Versiona valor e métodos de pagamento da taxa de matrícula por nível/ano/curso de uma academia. |
| `PUT` | `/financeiro/matriculas/configuracoes` | Idêntico ao `POST` acima — mesma rota, mesmo efeito (nova versão da configuração). |
| `GET` | `/financeiro/matriculas/configuracoes` | Lista as configurações vigentes de taxa de matrícula de uma academia. |
| `DELETE` | `/financeiro/appypay/credenciais` | Remove as credenciais AppyPay vigentes de um contexto (evento imutável; bloqueia novas cobranças nesse contexto). |
| `DELETE` | `/financeiro/mensalidades/configuracoes` | Remove a configuração vigente de mensalidade de um escopo, sem apagar o histórico de preço já cobrado. |
| `DELETE` | `/financeiro/mensalidades/inicio-cobranca` | Remove a exceção de início de cobrança, voltando ao mês natural do ano letivo. |
| `DELETE` | `/financeiro/matriculas/configuracoes` | Remove a configuração vigente de taxa de matrícula de um escopo. |
| `POST` | `/webhooks/appypay/gpo` | Recebe webhook público AppyPay para eventos GPO. |
| `POST` | `/webhooks/appypay/ref` | Recebe webhook público AppyPay para eventos REF. |

#### 19.1 POST /financeiro/appypay/credenciais

**Escopo da rota:** configuração inicial de credenciais AppyPay de um contexto financeiro. Use para cadastrar o contexto global `spuri` ou a credencial de uma academia. Não consulta saldos, não cria cobrança e não retorna segredos em claro.

**Proteção:** autenticado + academia dona do próprio contexto ou admin FPP. Para academia, `contexto_tipo` e `codigo_academia` são resolvidos pelo token. Para admin FPP, `contexto_tipo` deve ser `spuri` ou `academia`; quando for `academia`, `codigo_academia` identifica a instituição.

**Request JSON:**

```json
{
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "client_id": "appy-client-id",
  "client_secret": "appy-client-secret",
  "gpo_payment_method": "GPO_METHOD_ID",
  "ref_payment_method": "REF_METHOD_ID"
}
```

**Response 201:**

```json
{
  "id": "2f0f8d8f-27a1-4b2d-9a70-8e26d208f7e4",
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "ambiente": "test",
  "client_id_mask": "appy**********id",
  "gpo_payment_method_mask": "GPO_**********_ID",
  "ref_payment_method_mask": "REF_**********_ID",
  "webhook_header_name": "X-Spuri-Webhook-Secret",
  "webhook_secret": "aB3xY9kLm2PqRtZ",
  "updated_at": "2026-08-08T12:00:00Z"
}
```

**Regras de negócio:**

- `client_id`, `client_secret`, `gpo_payment_method` e `ref_payment_method` são obrigatórios. `resource` não é enviado neste endpoint: é lido da variável de ambiente `APPYPAY_RESOURCE`, com o mesmo valor para todas as academias e para o Spuri no mesmo ambiente.
- O segredo de webhook não é enviado pelo cliente: o backend gera automaticamente um valor alfanumérico de 15 caracteres na criação da credencial e devolve-o em texto plano apenas nesta resposta (campo `webhook_secret`), para o usuário colar no painel de webhooks da AppyPay. O nome do cabeçalho HTTP (`webhook_header_name`) é fixo para toda a plataforma (`X-Spuri-Webhook-Secret`) e não é mais configurável por credencial — a AppyPay confirmou que o painel deles só oferece um único par nome/valor de cabeçalho HTTP, por isso também não existe modo de autenticação alternativo (ex.: Basic Auth) para o webhook.
- Uma academia não pode criar credenciais para `spuri` nem para outra academia.
- O backend cifra segredos em armazenamento próprio e grava no ledger apenas metadados/máscaras.

#### 19.2 PUT /financeiro/appypay/credenciais/:id

**Escopo da rota:** atualização/substituição completa dos dados de conta AppyPay (`client_id`, `client_secret`, métodos GPO/REF) da credencial identificada por `:id`. Não altera o segredo de webhook — para isso, use `POST .../webhook-secret/rotacionar` (seção 19.13).

**Proteção:** autenticado + academia dona do próprio contexto ou admin FPP. O `id` precisa ser UUID válido.

**Request JSON:** igual ao `POST /financeiro/appypay/credenciais`.

**Response 200:** igual ao `POST /financeiro/appypay/credenciais`, com o mesmo `id` informado na URL e `updated_at` atualizado.

**Regras de negócio:**

- A atualização revalida todas as regras do cadastro de credenciais; envie o conjunto completo de campos obrigatórios.
- A atualização é sempre uma substituição completa dos dados de conta (`client_id`, `client_secret`, métodos GPO/REF); o segredo de webhook nunca é alterado por este endpoint — ele só muda por rotação explícita (`POST .../webhook-secret/rotacionar`, seção 19.13). `resource` continua fora do corpo da requisição e vem de `APPYPAY_RESOURCE`.
- A rota não expõe o valor antigo nem o novo valor dos segredos.
- Academia só pode manter o escopo no próprio contexto; mudança para `spuri` ou outra academia é bloqueada por autorização.

#### 19.3 GET /financeiro/appypay/credenciais

**Escopo da rota:** leitura de credenciais mascaradas para telas administrativas/operacionais. Não testa credenciais, não devolve segredos e não cria cobrança.

**Proteção:** autenticado + academia ou admin FPP.

**Query params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---|---|---|
| `contexto_tipo` | string | Não | `spuri` ou `academia`. Para academia autenticada é forçado para `academia`. |
| `codigo_academia` | string | Não | Código da academia. Para academia autenticada é forçado para o código do token. |

**Response 200:**

```json
[
  {
    "id": "2f0f8d8f-27a1-4b2d-9a70-8e26d208f7e4",
    "contexto_tipo": "academia",
    "codigo_academia": "LDA20261",
    "ambiente": "test",
    "client_id_mask": "appy**********id",
    "gpo_payment_method_mask": "GPO_**********_ID",
    "ref_payment_method_mask": "REF_**********_ID",
    "webhook_header_name": "X-Spuri-Webhook-Secret",
    "updated_at": "2026-08-08T12:00:00Z"
  }
]
```

**Regras de negócio:**

- Academias sempre recebem somente a própria credencial.
- Admin FPP pode filtrar por contexto; sem filtro, recebe as credenciais autorizadas pela consulta.
- Máscaras não devem ser usadas como segredos pelo cliente; rotação de `client_secret`/métodos exige `PUT` com os segredos reais. O segredo de webhook tem rotação própria (`POST .../credenciais/:id/webhook-secret/rotacionar`, seção 19.13) e nunca aparece mascarado aqui — só em texto pleno pelas rotas dedicadas (seções 19.1, 19.12 e 19.13).

#### 19.4 POST /financeiro/appypay/cobrancas

**Escopo da rota:** cria uma cobrança AppyPay genérica para pagamento GPO ou REF no contexto autorizado. A rota encaminha ao gateway os campos de cobrança e persiste o resultado para idempotência e consulta posterior.

**Proteção:** autenticado + academia do próprio contexto ou admin FPP.

**Campos do request:**

| Campo | Tipo | Obrigatório | Descrição e regras |
|---|---|---|---|
| `contexto_tipo` | string | Sim para admin FPP; não efetivo para academia | Contexto financeiro: `spuri` ou `academia`. Para um usuário de academia, omita o campo ou envie `academia`; qualquer outro valor é recusado e o backend fixa o contexto como `academia`. |
| `codigo_academia` | string | Sim quando o contexto final for `academia` e o chamador for admin FPP | Código da academia dona da cobrança. Para usuário de academia, omita o campo ou envie o código presente no token; outro código é recusado e o backend usa o valor do token. Não se aplica ao contexto `spuri`. |
| `amount` | número (`float64`) | Sim | Valor da cobrança, estritamente maior que zero, com no máximo duas casas decimais. O contrato segue `number<double>` da AppyPay. |
| `currency` | string | Não | Moeda da cobrança. Aceita somente `AOA`; se omitida, o backend usa `AOA`. |
| `description` | string | Sim | Descrição não vazia da cobrança, por exemplo a mensalidade ou o serviço cobrado. |
| `merchantTransactionId` | string | Não | Identificador externo da transação. Deve ser alfanumérico, sem espaços ou símbolos, com no máximo 15 caracteres. Se omitido, é gerado pelo backend. Reutilize o mesmo valor ao repetir uma tentativa: ele é a chave de idempotência global e também pode ser usado no `GET /financeiro/appypay/cobrancas/:id`. |
| `paymentMethod` | string | Sim | Método a usar: `GPO`, `REF`, ou o identificador configurado na credencial que comece por `GPO_` ou `REF_`. O backend resolve `GPO`/`REF` para o método efetivamente cadastrado nas credenciais do contexto. |
| `paymentInfo` | objeto | Condicional | Dados específicos do método. Para GPO, `paymentInfo.phoneNumber` é obrigatório e não pode estar vazio. Para REF, ele pode ser omitido ou vazio; se tiver qualquer campo, deve conter os três campos string não vazios: `referenceNumber`, `dueDate` e `nib`. O objeto é enviado ao gateway. |
| `options` | objeto | Não | Opções adicionais encaminhadas à AppyPay. Aceita no máximo duas chaves; os nomes e valores devem seguir o contrato do método AppyPay configurado. |
| `notify` | objeto | Não | Dados/instruções de notificação encaminhados à AppyPay. A API não impõe uma estrutura própria; use o formato aceito pelo gateway para o método configurado, por exemplo `{ "email": "..." }`. |
| `async` | booleano | Não | Define se a chamada ao gateway pode ser assíncrona. Quando omitido, vale `false`. |

**Exemplo — GPO com telefone e notificação:**

```json
{
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "amount": 12500,
  "currency": "AOA",
  "description": "Propina de agosto de 2026",
  "merchantTransactionId": "P2608LDA000001",
  "paymentMethod": "GPO",
  "paymentInfo": {
    "phoneNumber": "+244923000000"
  },
  "notify": {
    "email": "encarregado@example.com"
  },
  "async": false
}
```

**Exemplo — REF simples, sem `paymentInfo`:**

```json
{
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "amount": 12500,
  "description": "Propina de agosto de 2026",
  "merchantTransactionId": "P2608LDA000002",
  "paymentMethod": "REF"
}
```

Neste caso, `currency` assume `AOA` e a AppyPay gera os dados de referência segundo a configuração da credencial.

**Exemplo — REF com dados de referência e opções do gateway:**

```json
{
  "contexto_tipo": "spuri",
  "amount": 3500,
  "currency": "AOA",
  "description": "Taxa de inscrição",
  "merchantTransactionId": "TXINSCR260801",
  "paymentMethod": "REF",
  "paymentInfo": {
    "referenceNumber": "202608010001",
    "dueDate": "2026-08-31",
    "nib": "000400000000000000001"
  },
  "options": {
    "expiresIn": 86400
  },
  "async": true
}
```

`contexto_tipo: "spuri"` é destinado a admin FPP. Os nomes de `options` ilustram o encaminhamento ao gateway; confirme os campos aceitos pela configuração AppyPay em uso.

**Response 201:**

```json
{
  "id": "4d2bbf53-c8c0-4c9a-a3f4-5a0f0cf988d1",
  "provider_charge_id": "APPYPAY-987654",
  "merchant_transaction_id": "P2608LDA000001",
  "status": "pendente",
  "response": {
    "status": "Accepted"
  }
}
```

| Campo da resposta | Descrição |
|---|---|
| `id` | UUID interno da cobrança no Spuri. |
| `provider_charge_id` | Identificador retornado pela AppyPay, quando o gateway o fornece. |
| `merchant_transaction_id` | Identificador enviado ou gerado para a cobrança. |
| `status` | Estado retornado pela AppyPay; se ela não retornar um estado, a API informa `criada`. |
| `response` | Resposta bruta sanitizada da AppyPay. Seus campos podem variar por método e versão do gateway. |

**Regras de negócio:**

- Exige credenciais AppyPay configuradas para o contexto resolvido.
- `amount` deve ser positivo e ter no máximo duas casas decimais; `currency`, `description` e `paymentMethod` devem ser coerentes com o método configurado na credencial.
- `merchantTransactionId` é a referência externa recomendada para idempotência e posterior consulta.
- O mesmo `merchantTransactionId` devolve o resultado já persistido e não cria uma nova cobrança. Enquanto a primeira requisição ainda estiver sendo processada, a repetição recebe `409` e pode ser tentada novamente.
- A cobrança é registrada no ledger como solicitação e, conforme resposta da AppyPay, como criada ou falhada.

#### 19.5 POST /financeiro/appypay/qr-codes

**Escopo da rota:** cria uma cobrança GPO com QR Code. Use quando o cliente precisa exibir um QR Code de pagamento gerado pelo gateway. O método GPO é obtido exclusivamente das credenciais do contexto, portanto esta rota não aceita `paymentMethod` nem `paymentInfo`.

**Proteção:** autenticado + academia do próprio contexto ou admin FPP.

**Campos do request:**

| Campo | Tipo | Obrigatório | Descrição e regras |
|---|---|---|---|
| `contexto_tipo` | string | Sim para admin FPP; não efetivo para academia | Contexto financeiro: `spuri` ou `academia`. Para uma academia autenticada, omita o campo ou envie `academia`; outro valor é recusado e o backend fixa o contexto como `academia`. |
| `codigo_academia` | string | Sim quando o contexto final for `academia` e o chamador for admin FPP | Academia dona do QR Code. Para uma academia autenticada, omita o campo ou envie o código do token; outro código é recusado e o backend usa o valor do token. Não se aplica a `spuri`. |
| `amount` | número (`float64`) | Sim | Valor do QR Code, estritamente maior que zero e com no máximo duas casas decimais. |
| `currency` | string | Não | Moeda do QR Code. Se omitida, o backend usa `AOA`. |
| `description` | string | Sim | Descrição não vazia do pagamento. |
| `merchantTransactionId` | string | Não | Referência externa e chave de idempotência. Deve ser alfanumérica e ter no máximo 15 caracteres. Se omitida, é gerada pelo backend. |
| `qrCodeType` | string | Não | Tipo do QR Code: `SINGLE` (padrão) para uma utilização ou `MULTIPLE` para múltiplas utilizações dentro dos limites informados. O valor é normalizado para maiúsculas. |
| `minAmount` | número (`float64`) | Sim para `MULTIPLE` | Valor mínimo positivo, com no máximo duas casas decimais, aceito em cada pagamento do QR Code múltiplo. Não é usado no tipo `SINGLE`. |
| `maxTransactions` | inteiro | Sim para `MULTIPLE` | Quantidade máxima de pagamentos permitidos pelo QR Code múltiplo. Não é usado no tipo `SINGLE`. |
| `startDate` | string | Sim para `MULTIPLE` | Início da validade do QR Code múltiplo, no formato esperado pela AppyPay. Não é usado no tipo `SINGLE`. |
| `endDate` | string | Sim para `MULTIPLE` | Fim da validade do QR Code múltiplo, no formato esperado pela AppyPay. Não é usado no tipo `SINGLE`. |

**Exemplo — QR Code `SINGLE`:**

```json
{
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "amount": 12500,
  "currency": "AOA",
  "description": "Pagamento com QR Code",
  "merchantTransactionId": "Q2608LDA000001",
  "qrCodeType": "SINGLE"
}
```

Neste caso, `currency` foi informada explicitamente; ela poderia ser omitida e assumiria `AOA`.

**Exemplo — QR Code `MULTIPLE`:**

```json
{
  "contexto_tipo": "academia",
  "codigo_academia": "LDA20261",
  "amount": 5000,
  "currency": "AOA",
  "description": "QR Code para pagamentos parciais da propina",
  "merchantTransactionId": "Q2608LDA000002",
  "qrCodeType": "MULTIPLE",
  "minAmount": 1000,
  "maxTransactions": 5,
  "startDate": "2026-08-08T00:00:00Z",
  "endDate": "2026-08-31T23:59:59Z"
}
```

Para `MULTIPLE`, os quatro campos adicionais são obrigatórios. Seus valores e formato são encaminhados à AppyPay; use os limites e o formato de datas aceitos pelo gateway configurado.

**Response 201:**

```json
{
  "id": "76f2971c-4a7d-48f7-92c2-f8d3b28e9a2d",
  "provider_charge_id": "APPYPAY-QR-123",
  "merchant_transaction_id": "Q2608LDA000001",
  "status": "pendente",
  "qrCodeArr": "iVBORw0KGgoAAAANSUhEUgAA...",
  "response": {
    "status": "Accepted"
  }
}
```

| Campo da resposta | Descrição |
|---|---|
| `id` | UUID interno do QR Code/cobrança no Spuri. |
| `provider_charge_id` | Identificador retornado pela AppyPay, quando disponível. |
| `merchant_transaction_id` | Referência enviada ou gerada pela API. |
| `status` | Estado retornado pela AppyPay; quando ausente, a API usa `criada`. |
| `qrCodeArr` | Representação do QR Code retornada pela AppyPay, normalmente em base64. O cliente deve decodificá-la/interpretá-la conforme o formato retornado pelo gateway antes de exibir. |
| `response` | Resposta bruta sanitizada da AppyPay, que pode conter campos adicionais próprios do gateway. |

**Regras de negócio:**

- Usa o método GPO configurado na credencial do contexto.
- `qrCodeType` aceita `SINGLE` (padrão) ou `MULTIPLE`; o segundo exige `minAmount`, `maxTransactions`, `startDate` e `endDate`.
- O mesmo `merchantTransactionId` devolve o QR Code já persistido e não faz uma segunda chamada ao gateway. Enquanto a primeira requisição estiver sendo processada, a repetição recebe `409` e pode ser tentada novamente.
- `qrCodeArr`, quando presente, vem em base64 e deve ser tratado pelo cliente como imagem/representação do QR Code.
- Datas e limites (`minAmount`, `maxTransactions`) são repassados ao gateway conforme suporte da AppyPay.
- O QR Code também gera histórico financeiro no ledger.

#### 19.6 GET /financeiro/appypay/cobrancas/:id

**Escopo da rota:** consulta uma cobrança financeira já criada, por `provider_charge_id` da AppyPay ou por `merchantTransactionId`.

**Proteção:** autenticado + academia do próprio contexto ou admin FPP.

**Query params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---|---|---|
| `contexto_tipo` | string | Não | Contexto financeiro consultado. Para academia autenticada é forçado para `academia`. |
| `codigo_academia` | string | Não | Academia dona da cobrança. Para academia autenticada é forçado para o código do token. |

**Response 200:**

```json
{
  "id": "4d2bbf53-c8c0-4c9a-a3f4-5a0f0cf988d1",
  "provider_charge_id": "APPYPAY-987654",
  "merchant_transaction_id": "P2608LDA000001",
  "status": "paga",
  "response": {
    "status": "Success",
    "paidAt": "2026-08-08T12:30:00Z"
  }
}
```

**Regras de negócio:**

- A consulta respeita isolamento por contexto: academia não consulta cobrança de outra academia nem do `spuri`.
- O `:id` pode ser o identificador retornado pelo provider ou o `merchantTransactionId` informado na criação.
- A consulta grava evento financeiro apenas quando o estado, identificador do provider ou resposta relevante mudar; consultas sem mudança não poluem o ledger.
- Se a consulta detectar `Success` da AppyPay depois de a cobrança ter sido cancelada localmente, grava `CobrancaAppyPayConflitoPosCancelamento` e preserva o status local `cancelada`, para reconciliação manual por admin FPP.

#### 19.7 GET /financeiro/cobrancas

**Escopo da rota:** lista cobranças (mensalidade, matrícula ou avulsa) do contexto autorizado, com paginação e filtros por estado e origem — visão de academia/admin sobre pagamentos recebidos. Para o estudante consultar o próprio histórico de pagamentos, use 19.8.

**Proteção:** autenticado + academia do próprio contexto ou admin FPP. Estudantes recebem `403` nesta rota.

**Query params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---|---|---|
| `contexto_tipo` | string | Não | Contexto financeiro consultado. Para academia autenticada é forçado para `academia`. |
| `codigo_academia` | string | Não | Academia dona das cobranças. Para academia autenticada é forçado para o código do token. |
| `estado` | string, repetível | Não | Filtra pelo texto exato (case-sensitive) persistido em `status` — mistura estados internos (`solicitada`, `criada`, `cancelada`, `falhada`) e estados crus da AppyPay (`Success`, `Pending`, `Failed`, etc). Repita o parâmetro para casar mais de um valor (`?estado=Success&estado=Pending`). |
| `tipo` | string, repetível | Não | Filtra por origem: `matricula`, `mensalidade` ou `avulsa`. |
| `limit` | inteiro | Não | Itens por página. Padrão 50, mínimo 1, máximo 1000. |
| `offset` | inteiro | Não | Deslocamento de paginação. Padrão 0. |

**Response 200:**

```json
{
  "cobrancas": [
    {
      "id": "4d2bbf53-c8c0-4c9a-a3f4-5a0f0cf988d1",
      "provider_charge_id": "APPYPAY-987654",
      "merchant_transaction_id": "P2608LDA000001",
      "contexto_tipo": "academia",
      "codigo_academia": "ACA001",
      "origem": "mensalidade",
      "status": "Success",
      "valor": 1000.00,
      "moeda": "AOA",
      "descricao": "Mensalidade outubro/2025",
      "metodo_pagamento": "GPO",
      "codigo_estudante": "EST0001",
      "mensalidades": [{"ano_letivo": "2025_2026", "mes": 10}],
      "atualizado_em": "2026-08-08T12:30:00Z"
    }
  ],
  "total": 1,
  "total_geral": 1,
  "limit": 50,
  "offset": 0
}
```

**Regras de negócio:**

- Não devolve `payment_info`, `response` (resposta crua da AppyPay) nem `qrCodeArr`; para o detalhe completo de uma cobrança específica, use 19.6.
- `origem` é derivada do payload persistido, nunca gravada separadamente: `matricula` quando a cobrança tem `codigo_solicitacao`, `mensalidade` quando tem `codigo_estudante` (e não tem `codigo_solicitacao`), `avulsa` nos demais casos.
- Ordenação sempre por `updated_at DESC` (atividade mais recente primeiro); não há campo de data de criação separado nesta listagem.
- `total` é o número de itens nesta página; `total_geral` é o total real que casa com os filtros aplicados.

#### 19.8 GET /financeiro/cobrancas/estudante/:codigo

**Escopo da rota:** lista TODAS as cobranças que um estudante já teve, em qualquer estado, academia ou origem — incluindo a cobrança da matrícula original, mesmo que ela tenha sido paga antes de o estudante existir como tal. É a visão do próprio estudante sobre o seu histórico de pagamentos. Para a visão de academia/admin sobre cobranças recebidas, use 19.7.

**Proteção:** o próprio estudante (`:codigo` deve ser o código do token), academia à qual o estudante pertence ou pertenceu (mesmo vínculo histórico de `GET /financeiro/mensalidades/estudante/:codigo`), ou admin FPP.

**Query params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---|---|---|
| `estado` | string, repetível | Não | Mesmo filtro de 19.7. Sem filtro, devolve todos os estados. |
| `limit` | inteiro | Não | Itens por página. Padrão 50, mínimo 1, máximo 1000. |
| `offset` | inteiro | Não | Deslocamento de paginação. Padrão 0. |

**Response 200:** mesma estrutura de 19.7.

**Regras de negócio:**

- Diferente de 19.7, esta consulta não aceita `contexto_tipo` nem `codigo_academia`: um estudante pode ter mensalidades e matrícula em mais de uma academia (histórico), e o histórico mostra tudo — exceto quando quem consulta é uma academia, caso em que o resultado é restrito às cobranças feitas a essa academia especificamente (uma academia nunca vê pagamentos que o estudante fez a outra academia, mesmo com vínculo histórico com as duas).
- A cobrança de matrícula é resolvida pelo vínculo `codigo_estudante_gerado`, já gravado em `projection_solicitacoes_matricula` quando a solicitação é aprovada — o payload da cobrança de matrícula em si nunca grava `codigo_estudante`, porque a cobrança é anterior ao registo do estudante.
- Sem filtro de `estado`, a listagem inclui cobranças pendentes, falhadas e canceladas, não só as pagas — intencional: o objetivo é o estudante conseguir ver tudo que já teve, não só os pagamentos concluídos.

#### 19.9 POST /financeiro/appypay/cobrancas/:id/cancelar

**Escopo da rota:** cancela localmente uma cobrança REF, GPO ou QR Code ainda não paga. Não chama endpoint de cancelamento da AppyPay, pois esse endpoint não é documentado para esses métodos.

**Proteção:** autenticado + academia dona da própria cobrança, ou admin FPP somente quando a cobrança pertence ao contexto `spuri`. Ao contrário das demais operações financeiras, admin FPP nunca pode cancelar cobrança de academia.

**Request JSON:**

```json
{
  "motivo": "cobrança emitida em duplicado"
}
```

`motivo` é opcional. O corpo não aceita contexto nem código de academia: eles são fixados pelo actor autenticado.

**Response 200:** a mesma estrutura da consulta, com `status: "cancelada"`.

**Regras de negócio:**

- Antes de registrar `CobrancaAppyPayCancelada`, o backend reconsulta a AppyPay. Se o estado mais recente já for `Success`, não cancela nem grava evento de cancelamento.
- Cobranças `cancelada`, `falhada` ou `Success` não podem ser canceladas novamente ou reabertas. Para cobrar de novo, crie uma nova cobrança com outro `merchantTransactionId`.
- O evento `CobrancaAppyPayCancelada` é interno ao ledger Spuri. Uma referência/QR já emitido pode continuar tecnicamente pagável fora da plataforma até expirar; sucesso tardio gera `CobrancaAppyPayConflitoPosCancelamento`, sem alterar o status local cancelado.

#### 19.10 POST /webhooks/appypay/gpo

**Escopo da rota:** entrada pública para notificações AppyPay do método GPO.

**Proteção:** pública no roteamento HTTP, autenticada pelo segredo de webhook gerado pelo servidor, enviado no único cabeçalho HTTP fixo da plataforma (`webhook_header_name`, sempre `X-Spuri-Webhook-Secret`). A AppyPay confirmou que a autenticação do webhook sempre viaja por cabeçalho HTTP, nunca por query parameter — por isso este é o único método suportado.

**Request JSON:**

```json
{
  "id": "evt-gpo-0001",
  "merchantTransactionId": "QR-2026-08-LDA20261-0001",
  "status": "Paid",
  "paidAt": "2026-08-08T12:30:00Z"
}
```

**Response 200:** corpo vazio.

**Regras de negócio:** evento sem autenticação válida retorna `401`; JSON inválido ou sem identificador retorna `400`; reentregas com o mesmo identificador respondem `200` e são idempotentes.

#### 19.11 POST /webhooks/appypay/ref

**Escopo da rota:** entrada pública para notificações AppyPay do método REF.

**Proteção:** igual ao webhook GPO: autenticação pelo segredo de webhook no único cabeçalho HTTP fixo da plataforma (`webhook_header_name`, sempre `X-Spuri-Webhook-Secret`).

**Request JSON:**

```json
{
  "id": "evt-ref-0001",
  "merchantTransactionId": "PROP-2026-08-LDA20261-0001",
  "status": "Paid",
  "reference": "123456789"
}
```

**Response 200:** corpo vazio.

**Regras de negócio:** aplica as mesmas regras de autenticação, validação mínima e idempotência do webhook GPO; o método interno é `REF`.

#### 19.12 GET /financeiro/appypay/credenciais/:id/webhook-secret

**Escopo da rota:** consulta do segredo de webhook atual, em texto plano, de uma credencial já cadastrada.

**Proteção:** autenticado + dono do contexto da credencial (a própria academia, ou admin com permissão `fpp`), resolvido a partir do `id` da credencial.

**Response 200:**

```json
{
  "webhook_secret": "aB3xY9kLm2PqRtZ",
  "webhook_header_name": "X-Spuri-Webhook-Secret"
}
```

**Regras de negócio:** `webhook_header_name` é a constante fixa da plataforma; `id` inexistente ou fora do contexto autorizado retorna `404`, e falta de permissão retorna `403`.

#### 19.13 POST /financeiro/appypay/credenciais/:id/webhook-secret/rotacionar

**Escopo da rota:** gera um novo segredo de webhook para a credencial, invalidando o anterior imediatamente.

**Proteção:** igual à seção 19.12.

**Request JSON:** corpo vazio.

**Response 200:** igual à seção 19.12, com o novo valor de `webhook_secret`.

**Regras de negócio:** a rotação é imediata e definitiva, e grava `SegredoWebhookAppyPayRotacionado` no ledger sem expor o valor do segredo no payload do evento.

A obrigação mensal usa a chave estável `(codigo_estudante, codigo_academia, ano_letivo, mes)`. Só academias privadas (`type = "private"`) configuram e geram propinas. Cada mês é resolvido dinamicamente pelo preço vigente no primeiro dia daquele mês e pela turma/curso históricos do estudante no ano letivo; por isso meses antigos não mudam quando há troca posterior de turma, curso ou preço. `mes_fim_cobranca` só aceita `6` ou `7`; setembro é o início natural de fundamental/médio e outubro o de superior, e `POST /financeiro/mensalidades/inicio-cobranca` apenas encurta esse início para o par `(academia, ano_letivo)` informado. Valores seguem a regra financeira única do módulo: `float64`, duas casas, arredondamento *half away from zero*.

#### 19.14 POST e PUT /financeiro/mensalidades/configuracoes

**Escopo da rota:** versiona valor, mês final e métodos de pagamento da mensalidade/propina por nível, ano e curso de uma academia privada.

**Proteção:** academia dona (código forçado pelo token) ou admin FPP; para admin, `codigo_academia` é obrigatório.

**Campos do request:**

| Campo | Tipo | Obrigatório | Regras |
|---|---|---|---|
| `codigo_academia` | string | Sim | Deve existir e ser `private`; para academia é forçado pelo token. |
| `nivel` | string | Sim | `fundamental`, `medio` ou `superior`. |
| `ano_academico` | string | Sim | Deve ser oferecido pela academia/curso. |
| `curso_id` | UUID | Médio/superior | Obrigatório para `medio`/`superior` e proibido para `fundamental`; curso deve pertencer à academia, ter o mesmo `type` e oferecer o ano. |
| `valor` | número (`float64`) | Sim | Maior que zero, no máximo duas casas decimais. |
| `mes_fim_cobranca` | inteiro | Sim | Apenas `6` ou `7`. |
| `metodos_pagamento` | array string | Não | Itens `GPO`, `REF` ou `GPO_QR`, sem duplicados; lista vazia desactiva pagamento de propina para a combinação. Se não vazia, exige credencial AppyPay. |

**Request JSON:**

```json
{
  "codigo_academia": "ACA001",
  "nivel": "medio",
  "ano_academico": "1_ano_medio",
  "curso_id": "550e8400-e29b-41d4-a716-446655440000",
  "valor": 25000.00,
  "mes_fim_cobranca": 7,
  "metodos_pagamento": ["REF", "GPO_QR"]
}
```

**Response 201:**

```json
{
  "codigo_academia": "ACA001",
  "nivel": "medio",
  "ano_academico": "1_ano_medio",
  "curso_id": "550e8400-e29b-41d4-a716-446655440000",
  "valor": 25000.00,
  "mes_fim_cobranca": 7,
  "metodos_pagamento": ["REF", "GPO_QR"],
  "vigente_em": "2026-08-08T12:00:00Z"
}
```

**Regras de negócio:** `POST` e `PUT` usam o mesmo handler e têm o mesmo efeito: não actualizam uma versão in-place nem recebem `:id`; cada chamada válida cria nova versão com `vigente_em` do servidor, mesmo que o payload seja idêntico, preservando histórico para meses passados.

**Erros comuns:** `400` payload/regra inválida, `404` academia/curso inexistente, `403` sem permissão.

#### 19.15 GET /financeiro/mensalidades/configuracoes

**Escopo da rota:** lista as configurações de mensalidade actualmente vigentes para uma academia.

**Proteção:** igual à 19.14.

**Query params:** `codigo_academia` obrigatório para admin; forçado pelo token para academia.

**Response 200:**

```json
{
  "codigo_academia": "ACA001",
  "configuracoes": [
    {
      "codigo_academia": "ACA001",
      "nivel": "medio",
      "ano_academico": "1_ano_medio",
      "curso_id": "550e8400-e29b-41d4-a716-446655440000",
      "valor": 25000.00,
      "mes_fim_cobranca": 7,
      "metodos_pagamento": ["REF", "GPO_QR"],
      "vigente_em": "2026-08-08T12:00:00Z"
    }
  ]
}
```

**Regras de negócio:** devolve apenas a versão mais recente (`vigente_em` mais alto) de cada combinação `(nivel, ano_academico, curso_id)`, não um histórico. Para preço passado, consulte 19.17.

#### 19.16 POST /financeiro/mensalidades/inicio-cobranca

**Escopo da rota:** define uma exceção pontual de início de cobrança para academia privada que entrou no ano letivo fora do mês natural.

**Proteção:** academia dona ou admin FPP.

**Request JSON:**

```json
{ "codigo_academia": "ACA001", "ano_letivo": "2026_2027", "mes_inicio": 1 }
```

**Response 201:** corpo vazio.

**Regras de negócio:** `ano_letivo` segue `YYYY_YYYY` com segundo ano igual ao primeiro + 1; a academia deve ser `private`; `mes_inicio` é validado por posição no ano letivo, isto é, os meses contam desde setembro para fundamental/médio ou outubro para superior, de modo que a posição 1 é o mês inicial e julho do ano seguinte ocupa a última posição natural. Não pode ultrapassar o menor `mes_fim_cobranca` já configurado e vale apenas para o par `(academia, ano_letivo)`.

**Erros comuns:** `400` dados inválidos ou início posterior ao fim configurado, `404` academia inexistente, `403` sem permissão.

#### 19.17 GET /financeiro/mensalidades/estudante/:codigo

**Escopo da rota:** calcula sob consulta todos os meses devidos, pagos ou anulados de um estudante nas academias privadas com vínculo histórico ou actual.

**Proteção:** o próprio estudante, academia com vínculo histórico/actual restrita à própria chave, ou admin FPP.

**Response 200:**

```json
{
  "codigo_estudante": "EST0001",
  "mensalidades": [
    {
      "codigo_estudante": "EST0001",
      "codigo_academia": "ACA001",
      "ano_letivo": "2025_2026",
      "mes": 10,
      "data_referencia": "2025-10-01T00:00:00Z",
      "nivel": "medio",
      "ano_academico": "1_ano_medio",
      "curso_id": "550e8400-e29b-41d4-a716-446655440000",
      "valor": 25000.00,
      "mes_fim_cobranca": 7,
      "estado": "pendente",
      "eventos_auditoria": []
    }
  ],
  "metodos_pagamento_por_academia": {
    "ACA001": ["REF", "GPO_QR"]
  }
}
```

**Regras de negócio:** `estado` é `pendente`, `pago` ou `anulado`; pagamento real prevalece sobre anulação, e reativação só muda mês anulado. `eventos_auditoria` lista IDs de eventos aplicados ao mês. `metodos_pagamento_por_academia` reflecte a configuração vigente mais recente; lista vazia significa propina desactivada para pagamento. Academia vê só a própria academia; estudante e FPP vêem todas.

**Erros comuns:** `404` estudante inexistente, `403` estudante tentando ver outro código, academia sem vínculo ou admin sem `fpp`.

#### 19.18 POST /financeiro/mensalidades/pagamento

**Escopo da rota:** único caminho pelo qual o estudante paga propina, criando uma cobrança única para os meses escolhidos.

**Proteção:** exclusivo de usuário `estudante`; academia e admin recebem `403`.

**Request JSON:**

```json
{
  "codigo_academia": "ACA001",
  "meses": [
    { "ano_letivo": "2025_2026", "mes": 10 },
    { "ano_letivo": "2025_2026", "mes": 1 }
  ],
  "metodo_pagamento": "REF"
}
```

`telefone` é aceite e obrigatório apenas quando `metodo_pagamento` é `GPO`.

**Response 201:**

```json
{
  "cobranca": {
    "id": "4d2bbf53-c8c0-4c9a-a3f4-5a0f0cf988d1",
    "provider_charge_id": "APPYPAY-987654",
    "merchant_transaction_id": "PROP2608LDA0001",
    "status": "pendente",
    "response": { "status": "Accepted" }
  },
  "meses": [
    { "ano_letivo": "2025_2026", "mes": 10 },
    { "ano_letivo": "2025_2026", "mes": 1 }
  ]
}
```

Quando `metodo_pagamento` é `GPO_QR`, `cobranca` inclui `qrCodeArr` em base64.

**Regras de negócio:** método precisa estar habilitado para a academia; todos os meses são validados antes da AppyPay, precisam estar `pendente`, sem duplicados e sem cobrança aberta. A seleção deve incluir o mês pendente mais antigo daquela academia; outros meses pendentes não precisam ser consecutivos. O valor é a soma dos preços históricos, arredondada pela regra financeira única. A confirmação por consulta (19.6) ou webhook (19.10/19.11) grava todos os pagamentos da cobrança numa única transação idempotente.

**Erros comuns:** `400` para violação de regras, inclusive cobrança aberta; `403` para quem não é estudante.

#### 19.19 POST /financeiro/mensalidades/obrigacoes/anular

**Escopo da rota:** a academia isenta um ou mais meses específicos, registando evento imutável por mês.

**Proteção:** exclusiva da academia dona do vínculo; admin FPP recebe `403`.

**Request JSON:**

```json
{
  "codigo_estudante": "EST0001",
  "codigo_academia": "ACA001",
  "ano_letivo": "2026_2027",
  "meses": [1, 2],
  "motivo": "bolsa social"
}
```

**Response 201:** corpo vazio.

**Regras de negócio:** a academia precisa ter vínculo actual ou histórico com o estudante; `meses` deve conter inteiros distintos entre 1 e 12 e corresponder a meses devidos. Mês pago não pode ser anulado. Cada mês gera `ObrigacaoMensalidadeAnulada`. Se um mês estava em cobrança aberta, a cobrança inteira é cancelada localmente, mas os outros meses cobertos não são anulados. Falha best-effort no cancelamento não desfaz a anulação e pode gerar conflito para reconciliação FPP conforme 19.9.

**Erros comuns:** `400` mês pago/fora do período/dados inválidos, `403` academia sem vínculo ou admin FPP.

#### 19.20 POST /financeiro/mensalidades/obrigacoes/reativar

**Escopo da rota:** reverte uma anulação, voltando meses para `pendente`.

**Proteção:** idêntica à 19.19.

**Request JSON:** mesmo formato de 19.19.

**Response 201:** corpo vazio.

**Regras de negócio:** só é possível reativar mês actualmente `anulado`; tentar reativar `pendente` ou `pago` é erro de validação. Cada mês gera `ObrigacaoMensalidadeReativada`; não cria nem cancela cobranças.

**Erros comuns:** `400` mês não anulado, `403` academia sem vínculo ou admin FPP.

A taxa de matrícula abaixo é apenas a configuração financeira feita pela academia/FPP. O pagamento pelo candidato é público e está descrito em 9.10 e 9.11.

#### 19.21 POST e PUT /financeiro/matriculas/configuracoes

**Escopo da rota:** versiona valor e métodos de pagamento da taxa de matrícula por nível, ano e curso.

**Proteção:** academia dona (código forçado pelo token) ou admin FPP; para admin, `codigo_academia` é obrigatório.

**Campos do request:**

| Campo | Tipo | Obrigatório | Regras |
|---|---|---|---|
| `codigo_academia` | string | Sim | Academia deve existir; para academia é forçado pelo token. |
| `nivel` | string | Sim | `fundamental`, `medio` ou `superior`. |
| `ano_academico` | string | Sim | Deve ser oferecido pela academia/curso. |
| `curso_id` | UUID | Médio/superior | Obrigatório para `medio`/`superior` e proibido para `fundamental`; mesmas regras de vínculo curso↔academia↔ano de 19.14. |
| `valor` | número (`float64`) | Sim | Maior que zero, no máximo duas casas decimais. |
| `metodos_pagamento` | array string | Sim | Pelo menos um item; `GPO`, `REF` ou `GPO_QR`, sem duplicados; exige credencial AppyPay. |

**Request JSON:**

```json
{
  "codigo_academia": "ACA001",
  "nivel": "medio",
  "ano_academico": "1_ano_medio",
  "curso_id": "550e8400-e29b-41d4-a716-446655440000",
  "valor": 15000.00,
  "metodos_pagamento": ["REF", "GPO"]
}
```

**Response 201:**

```json
{
  "codigo_academia": "ACA001",
  "nivel": "medio",
  "ano_academico": "1_ano_medio",
  "curso_id": "550e8400-e29b-41d4-a716-446655440000",
  "valor": 15000.00,
  "metodos_pagamento": ["REF", "GPO"],
  "vigente_em": "2026-08-08T12:00:00Z"
}
```

**Regras de negócio:** diferentemente da mensalidade, academias públicas e privadas podem configurar taxa de matrícula. `POST` e `PUT` têm o mesmo efeito: cada chamada válida cria nova versão, nunca update in-place. Sem configuração para a combinação da solicitação, a matrícula continua gratuita e a aprovação cria o estudante imediatamente. Com configuração vigente, a aprovação exige pagamento antes da criação do estudante; valor e métodos ficam congelados na solicitação no momento da aprovação.

**Erros comuns:** `400` payload/regra inválida, `404` academia/curso inexistente, `403` sem permissão.

#### 19.22 GET /financeiro/matriculas/configuracoes

**Escopo da rota:** lista as configurações vigentes da taxa de matrícula de uma academia.

**Proteção:** igual à 19.21.

**Query params:** `codigo_academia` obrigatório para admin; forçado pelo token para academia.

**Response 200:**

```json
{
  "codigo_academia": "ACA001",
  "configuracoes": [
    {
      "codigo_academia": "ACA001",
      "nivel": "medio",
      "ano_academico": "1_ano_medio",
      "curso_id": "550e8400-e29b-41d4-a716-446655440000",
      "valor": 15000.00,
      "metodos_pagamento": ["REF", "GPO"],
      "vigente_em": "2026-08-08T12:00:00Z"
    }
  ]
}
```

**Regras de negócio:** devolve apenas a versão mais recente de cada combinação `(nivel, ano_academico, curso_id)`, não o histórico completo.

#### 19.23 DELETE /financeiro/appypay/credenciais

**Escopo da rota:** remove as credenciais AppyPay vigentes de um contexto (`spuri` ou `academia`), respeitando event sourcing — nenhum evento é apagado do ledger, a remoção é registrada como um novo evento imutável.

**Proteção:** igual à 19.1.

**Request JSON:**

```json
{ "contexto_tipo": "academia", "codigo_academia": "LDA20261" }
```

**Response 204:** corpo vazio.

**Regras de negócio:** grava `CredenciaisAppyPayRemovidas`; o evento `CredenciaisAppyPayConfiguradas` original permanece no ledger, apenas deixa de ser o fato mais recente. A partir deste comando, qualquer tentativa de gerar cobrança nesse contexto (seções 19.4, 19.5, 19.18, 9.11) volta a ser bloqueada por ausência de credenciais, exatamente como antes de terem sido configuradas alguma vez. O cofre operacional de segredos é limpo no mesmo comando. Configurar novamente (19.1) funciona normalmente a qualquer momento depois.

**Erros comuns:** `404` nenhuma credencial configurada para o contexto no ambiente atual (inclusive ao tentar remover uma segunda vez), `403` sem permissão.

#### 19.24 DELETE /financeiro/mensalidades/configuracoes

**Escopo da rota:** remove a configuração vigente de mensalidade (valor, mês final, métodos de pagamento) de um escopo `(nivel, ano_academico, curso_id)`.

**Proteção:** igual à 19.14.

**Request JSON:**

```json
{
  "codigo_academia": "ACA001",
  "nivel": "medio",
  "ano_academico": "1_ano_medio",
  "curso_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

`curso_id` é opcional e segue as mesmas regras de presença/ausência por nível da 19.14.

**Response 204:** corpo vazio.

**Regras de negócio:** grava `MensalidadeConfiguracaoRemovida`; nunca apaga nem reescreve nenhuma linha de configuração anterior, então o preço de um mês já cobrado antes da remoção continua resolvendo normalmente em 19.17. A partir da remoção, o escopo deixa de aparecer em 19.15 e nenhum método de pagamento fica habilitado para essa combinação (novas tentativas de pagamento falham como se nunca tivesse existido configuração). Configurar novamente (19.14) cria uma nova versão normalmente, sem preencher retroativamente o intervalo em que o escopo ficou sem configuração.

**Erros comuns:** `404` nenhuma configuração ativa para o escopo (inclusive numa segunda remoção), `403` sem permissão.

#### 19.25 DELETE /financeiro/mensalidades/inicio-cobranca

**Escopo da rota:** remove a exceção de mês de início de cobrança de um ano letivo, revertendo ao mês natural (setembro para fundamental/médio, outubro para superior).

**Proteção:** igual à 19.16.

**Request JSON:**

```json
{ "codigo_academia": "ACA001", "ano_letivo": "2026_2027" }
```

**Response 204:** corpo vazio.

**Regras de negócio:** grava `MesInicioCobrancaRemovido`; o evento `MesInicioCobrancaDefinido` original permanece no ledger. A partir da remoção, a listagem de mensalidades do estudante (19.17) volta a considerar o mês natural do ano letivo para esse par `(academia, ano_letivo)`. Redefinir novamente (19.16) funciona normalmente.

**Erros comuns:** `404` nenhuma exceção definida para o ano letivo (inclusive numa segunda remoção), `403` sem permissão.

#### 19.26 DELETE /financeiro/matriculas/configuracoes

**Escopo da rota:** remove a configuração vigente de taxa de matrícula de um escopo `(nivel, ano_academico, curso_id)`.

**Proteção:** igual à 19.21.

**Request JSON:**

```json
{
  "codigo_academia": "ACA001",
  "nivel": "medio",
  "ano_academico": "1_ano_medio",
  "curso_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

`curso_id` é opcional e segue as mesmas regras de presença/ausência por nível da 19.21.

**Response 204:** corpo vazio.

**Regras de negócio:** grava `MatriculaConfiguracaoRemovida`; o evento de configuração original permanece no ledger. A partir da remoção, o escopo deixa de aparecer em 19.22 e a matrícula volta a ser gratuita para essa combinação — mesmo comportamento de nunca ter tido configuração (ver 19.21). Configurar novamente cria uma nova versão normalmente.

**Erros comuns:** `404` nenhuma configuração ativa para o escopo (inclusive numa segunda remoção), `403` sem permissão.

Nas rotas de `/financeiro/*` (excepto o fluxo público 9.9–9.12), `404` só representa recurso referenciado inexistente, `409` só representa operação equivalente já em processamento por idempotência do mesmo `merchantTransactionId`, e `503` só representa falha de comunicação com a AppyPay. Qualquer outra violação de regra de negócio — mesmo mensagens que soem como conflito, por exemplo mensalidade com cobrança aberta ou cobrança já paga e não cancelável — responde `400`; não infira `409` pelo texto da mensagem.

**Erros comuns das rotas autenticadas:**

| Status | Quando ocorre |
|---|---|
| `400` | Payload inválido, UUID inválido, contexto financeiro inválido ou credenciais incompletas. |
| `401` | Token ausente/inválido nas rotas `/financeiro/*` ou autenticação inválida nos webhooks. |
| `403` | Usuário autenticado tenta operar contexto sem permissão. |
| `404` | Credencial/cobrança inexistente ou não encontrada no contexto permitido. |
| `409` | Uma cobrança com o mesmo `merchantTransactionId` ainda está sendo processada. |
| `503` | Falha de comunicação, autenticação ou resposta inválida da AppyPay; tente novamente mais tarde. |
| `500` | Falha interna não tratada do serviço financeiro. |

---

## 20. Armazenamento

### Processos e Regras de Negócio — Armazenamento de Arquivos

### Armazenamento de arquivos (Mega)

O backend usa a interface `storage.StorageProvider` para isolar handlers, domínio, projeções e contratos públicos dos detalhes do provedor externo. O provedor principal configurável é o Mega (`STORAGE_PROVIDER=mega`), implementado por `internal/storage` via MEGAcmd (`mega-login`, `mega-mkdir`, `mega-put`, `mega-ls`, `mega-get`, `mega-rm`, `mega-mv`). Essa escolha permite autenticação por e-mail e senha, criação/listagem de pastas, upload, leitura/download, deleção, movimentação e renomeação sem expor tipos ou IDs internos do Mega nas APIs.

Configuração de produção:

- `STORAGE_PROVIDER=mega`: seleciona o adapter Mega; quando a variável não é definida, o padrão também é `mega`.
- `MEGA_EMAIL`: e-mail da conta Mega, fornecido por segredo/variável de ambiente.
- `MEGA_PASSWORD`: senha da conta Mega, fornecida por segredo/variável de ambiente e sanitizada em erros.
- `MEGA_ROOT_FOLDER`: pasta raiz lógica no Mega usada pelo Spuri (ex.: `spuri`).

Configuração local/teste:

- `STORAGE_PROVIDER=local`: seleciona o provider local compatível com a mesma interface, sem conexão externa.
- `MEGA_LOCAL_ROOT`: diretório local usado pelo provider local (padrão `data/mega_storage`).
- `ENV=test`: permite usar o provider local nos testes automatizados.

Os documentos de matrícula continuam sendo gravados em `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/`; documentos formais seguem `{codigo_academia}/Documentação formal/`; documentos de estudantes seguem `{codigo_academia}/Estudantes/{codigo_estudante}/`. `EnsureDir` cria a hierarquia de pastas de forma idempotente, `Upload` envia o conteúdo para o caminho lógico solicitado e retorna metadados internos do projeto (`path`, `file_url`, `download_url`). Nas respostas de consulta, o backend normaliza `download_url` para uma rota autenticada própria do escopo consultado, mesmo quando o metadado persistido contém link legado do storage. O front end deve baixar documentos pelas rotas autenticadas de download do backend (`/documentos/academias/{codigo_academia}/alvara/download`, `/documentos/estudantes/{codigo_estudante}/{campo}/download`, `/documentos/solicitacoes-matricula/{codigo_solicitacao}/{campo}/download`, `/estudante/solicitacoes-edicao/{codigo_solicitacao}/documento/download`, `/academia/documentos/solicitacoes-edicao-estudante/{codigo_solicitacao}/documento/download`, `/estudante/documentos/{campo}/download` ou `/academia/documentos/...`), e não por credenciais, links privados ou IDs internos do Mega. `Read` faz o download para arquivo temporário e entrega um stream fechado pelo handler; `Delete`, `Move` e `Rename` normalizam paths e erros externos. `GetQuota` é suportado no provider local; no Mega real, limitações do MEGAcmd para quota detalhada por diretório são expostas como operação não suportada em vez de simular sucesso.

Não há migração automática de arquivos do Google Drive para o Mega porque não existem arquivos remotos atuais a copiar. Referências antigas, se encontradas, devem ser tratadas como metadados legados; novos uploads, leituras/downloads, deleções, movimentações e renomeações usam Mega ou o fake local em testes.

Falhas de configuração retornam mensagens operacionais explícitas, sem vazar senha/token: credenciais Mega ausentes, MEGAcmd indisponível, caminho remoto inválido, arquivo/pasta inexistente, quota excedida, permissão/autenticação negada, timeout/rede e operação não suportada são convertidos para erros normalizados do pacote de storage.
### Permissões — Solicitação e Armazenamento

### Permissões

|Ação|Quem pode|
|---|---|
|Criar solicitação|Público|
|Listar/consultar solicitações da academia|Academia dona|
|Aprovar/reprovar|Academia dona|
|Listar todas|Admin|
|Configurar documentos obrigatórios|Academia dona|
|Consultar quota de storage|Admin|


### GET /documentos/academias/{codigo_academia}/alvara/download

Faz stream inline do alvará/documento formal da academia pelo backend, sem expor credenciais, links privados ou IDs internos do Mega. As consultas autenticadas de academia (`GET /academias`, `GET /consultar-academia/:codigo` e o inventário `GET /academia/documentos`) retornam esse endereço em `documentos.alvara.download_url`.

**Escopo da rota**: global autenticado (`protected`), fora dos prefixos `/academia`, `/estudante` e `/dominis`, para permitir uso uniforme pelo front end a partir de qualquer tela autorizada.

**Proteção**: autenticado. Permitido para admin ou para a própria academia dona do `codigo_academia`. Estudantes e academias de outro código recebem `403 Forbidden`.

**Parâmetros de path:**

|Campo|Tipo|Obrigatório|Descrição|
|---|---|---|---|
|`codigo_academia`|string|Sim|Código público da academia dona do documento.|

**Response 200:** `application/pdf`, com `Content-Disposition: inline; filename="alvara.pdf"`.

**Erros:**

|Status|Quando ocorre|
|---|---|
|`401`|Token ausente ou inválido.|
|`403`|Usuário autenticado não tem permissão para a academia informada.|
|`404`|Academia ou documento não encontrado.|
|`503`|Storage indisponível ou falha de leitura no provider configurado.|

### GET /documentos/estudantes/{codigo_estudante}/{campo}/download

Faz stream inline de um documento persistido no mapa `documentos` da projeção do estudante. Para documentos acadêmicos normalizados, o backend procura primeiro pela chave exata `nivel.ano_academico.tipo` e também aceita localizar por `campo`/`tipo` quando os query params `nivel` e `ano_academico` forem enviados. Exemplos de chaves atuais: `bi_estudante`, `bi_encarregado`, `cedula_estudante`, `medio.3_ano_medio.declaracao_3_ano_medio`, `fundamental.9_ano_fundamental.certificado_9_ano_fundamental` ou `medio.3_ano_medio.certificado_ensino_medio`.

**Escopo da rota**: global autenticado (`protected`), fora dos prefixos de perfil, para que a mesma URL salva em `download_url` funcione para admin, academia autorizada e estudante dono.

**Proteção**: autenticado. Permitido para admin, para o próprio estudante e para a academia dona do estudante. Outros perfis/escopos recebem `403 Forbidden`.

**Parâmetros de path:**

|Campo|Tipo|Obrigatório|Descrição|
|---|---|---|---|
|`codigo_estudante`|string|Sim|Código público do estudante.|
|`campo`|string|Sim|Chave exata do documento dentro do mapa `documentos` do estudante ou tipo do documento quando combinado com `nivel` e `ano_academico`.|

**Query params opcionais para documentos acadêmicos:**

|Campo|Tipo|Obrigatório|Descrição|
|---|---|---|---|
|`nivel`|string|Não|Escopo acadêmico do documento (`fundamental`, `medio`, `superior` ou `escopo_desconhecido`).|
|`ano_academico`|string|Não|Ano acadêmico esperado, por exemplo `3_ano_medio`. Quando informado, evita que uma rota com `campo=declaracao` baixe a declaração de outro ano.|

**Response 200:** `application/pdf`, com `Content-Disposition: inline; filename="{campo}.pdf"`.

**Erros:**

|Status|Quando ocorre|
|---|---|
|`401`|Token ausente ou inválido.|
|`403`|Usuário autenticado não tem permissão para o estudante informado.|
|`404`|Estudante, campo de documento ou arquivo remoto não encontrado.|
|`503`|Storage indisponível ou falha de leitura no provider configurado.|

### GET /documentos/solicitacoes-matricula/{codigo_solicitacao}/{campo}/download

Faz stream inline de um documento persistido no mapa `documentos` da projeção da solicitação de matrícula. Assim como nos documentos de estudante, documentos acadêmicos normalizados podem ser baixados pela chave exata `nivel.ano_academico.tipo` ou por `campo`/`tipo` com os query params opcionais `nivel` e `ano_academico`.

**Escopo da rota**: global autenticado (`protected`), fora dos prefixos `/academia` e `/dominis`, para que a URL salva em `download_url` seja consumida pelo front end em telas administrativas e de academia sem depender do provider externo.

**Proteção**: autenticado. Permitido para admin e para a academia dona da solicitação. Estudantes, usuários públicos e academias de outro código recebem `403 Forbidden`.

**Parâmetros de path:**

|Campo|Tipo|Obrigatório|Descrição|
|---|---|---|---|
|`codigo_solicitacao`|string|Sim|Código público da solicitação de matrícula.|
|`campo`|string|Sim|Chave do documento dentro do mapa `documentos` da solicitação.|

**Response 200:** `application/pdf`, com `Content-Disposition: inline; filename="{campo}.pdf"`.

**Erros:**

|Status|Quando ocorre|
|---|---|
|`401`|Token ausente ou inválido.|
|`403`|Usuário autenticado não tem permissão para a solicitação informada.|
|`404`|Solicitação, campo de documento ou arquivo remoto não encontrado.|
|`503`|Storage indisponível ou falha de leitura no provider configurado.|

### GET /estudante/solicitacoes-edicao/{codigo_solicitacao}/documento/download

Faz stream inline do PDF comprovativo anexado a uma solicitação de edição de dados sensíveis do estudante autenticado. Esse endpoint é o valor de `documento.download_url` retornado por `GET /estudante/solicitacoes-edicao`.

**Proteção**: autenticado + estudante. O estudante só pode baixar documentos de solicitações associadas ao seu próprio `codigo_estudante`.

**Parâmetros de path:**

|Campo|Tipo|Obrigatório|Descrição|
|---|---|---|---|
|`codigo_solicitacao`|string|Sim|Código público da solicitação de edição.|

**Response 200:** `application/pdf`, com `Content-Disposition: inline; filename="documento_{campo}.pdf"`.

**Erros:**

|Status|Quando ocorre|
|---|---|
|`401`|Token ausente ou inválido.|
|`403`|Usuário autenticado não é o estudante dono da solicitação.|
|`404`|Solicitação, metadado de documento ou arquivo remoto não encontrado.|
|`503`|Storage indisponível ou falha de leitura no provider configurado.|

### GET /academia/documentos/solicitacoes-edicao-estudante/{codigo_solicitacao}/documento/download

Faz stream inline do PDF comprovativo anexado a uma solicitação de edição de dados sensíveis em escopo de academia. Esse endpoint é o valor de `documento.download_url` retornado por `GET /academia/solicitacoes-edicao-estudante`.

**Proteção**: academia ativa ou admin. Academias só podem baixar documentos de solicitações vinculadas ao próprio `codigo_academia`; admins podem baixar qualquer solicitação.

**Parâmetros de path:**

|Campo|Tipo|Obrigatório|Descrição|
|---|---|---|---|
|`codigo_solicitacao`|string|Sim|Código público da solicitação de edição.|

**Response 200:** `application/pdf`, com `Content-Disposition: inline; filename="documento_{campo}.pdf"`.

**Erros:**

|Status|Quando ocorre|
|---|---|
|`401`|Token ausente ou inválido.|
|`403`|Academia autenticada não é dona da solicitação informada.|
|`404`|Solicitação, metadado de documento ou arquivo remoto não encontrado.|
|`503`|Storage indisponível ou falha de leitura no provider configurado.|

### GET /dominis/storage/quota

Retorna a distribuição conhecida dos arquivos gerenciados pelo provider ativo. Com `STORAGE_PROVIDER=local`, o backend contabiliza os arquivos dentro de `MEGA_LOCAL_ROOT` (padrão `data/mega_storage`) usando a mesma regra dos caminhos lógicos de academia. Com `STORAGE_PROVIDER=mega`, a implementação não simula quota detalhada quando o MEGAcmd não oferece dados suficientes por diretório; nesse caso, a operação retorna erro normalizado de operação não suportada/indisponível em vez de informar números incorretos.

Quando a configuração do Mega ou da quota estiver incompleta ou inválida, a rota retorna `503 Service Unavailable` com mensagem sanitizada. Exemplos de mensagens:

- `configuração de storage inválida: MEGA_EMAIL e MEGA_PASSWORD são obrigatórios quando STORAGE_PROVIDER=mega`
- `configuração de storage inválida: MEGAcmd não encontrado no PATH`
- `operação de storage não suportada`

**Proteção**: autenticado + admin


**Request:** sem payload
**Response 200:**

```json
{
  "provider": "mega",
  "total_bytes": 108003328,
  "used_bytes": 108003328,
  "available_bytes": 0,
  "managed_bytes": 104857600,
  "outside_academias_bytes": 3145728,
  "unmanaged_bytes": 0,
  "total_human": "103.00 MB",
  "used_human": "103.00 MB",
  "available_human": "0 B",
  "managed_human": "100.00 MB",
  "outside_academias_human": "3.00 MB",
  "unmanaged_human": "0 B",
  "academias": [
    {
      "codigo_academia": "ACA001",
      "used_bytes": 104857600,
      "used_human": "100.00 MB"
    }
  ],
  "account_files": [
    {
      "path": "ACA001/matriculas/matricula_2026_0001/documento.pdf",
      "name": "documento.pdf",
      "size_bytes": 1048576,
      "size_human": "1.00 MB",
      "managed": true
    }
  ]
}
```

**Response 503:**

```json
{
  "error": "SERVICE_UNAVAILABLE",
  "message": "operação de storage não suportada",
  "request_id": "8c7e6a5d-9b9f-4fd2-a2d0-3a989a8c2d8b"
}
```


---

## 21. Integrações Externas / Ziett (Teste)

Esta seção documenta a rota isolada para validar conectividade com a API externa da Ziett por envio de SMS. A rota não grava eventos, não alimenta projeções, não usa `spuri_ledger` e não participa de fluxos de matrícula, estudante, academia ou financeiro.

### 21.1 `POST /integracoes/ziett/mensagens/teste`

Envia uma mensagem de teste através do endpoint `POST /messages` da Ziett, com `channel_type` sempre fixo em `SMS` no backend. Dependendo da API Key configurada, o disparo pode ser simulado (`zk_test_`) ou real e com custo (`zk_live_`).

**Proteção**: `Authorization: Bearer <token>` de admin FPP. Requisições sem token retornam `401`; tokens autenticados sem nível FPP retornam `403`.

**Variável obrigatória**: `ZIETT_API_KEY`. Se ausente ou vazia, a API retorna `503 Service Unavailable` sem contactar a Ziett.

**Request JSON:**

```json
{
  "remitter_id": "550e8400-e29b-41d4-a716-446655440000",
  "target_e164": "923456789",
  "content": "Mensagem de teste Spuri via Ziett"
}
```

| Campo | Tipo | Obrigatório | Validação |
| --- | --- | --- | --- |
| `remitter_id` | string | Sim | UUID válido do Sender ID cadastrado na Ziett. |
| `target_e164` | string | Sim | Número móvel angolano nacional com 9 dígitos, iniciado por `9`, sem `0` inicial e sem `+244`. O backend aceita defensivamente `0`, `244` ou `+244` recebidos por engano e envia à Ziett no formato `+244XXXXXXXXX`. |
| `content` | string | Sim | Não vazio; máximo de 1600 caracteres. |

> `channel_type` não é aceito no payload de entrada. O backend sempre envia `SMS` para a Ziett.

**Response 202:**

```json
{
  "message": "mensagem de teste enviada à Ziett com sucesso",
  "message_id": "msg_123",
  "target_e164": "+244923456789",
  "channel_type": "SMS"
}
```

**Erros:**

| Status | Quando ocorre | Observações |
| --- | --- | --- |
| `400` | JSON malformado, `remitter_id` ausente/não UUID, `target_e164` inválido, `content` vazio ou acima de 1600 caracteres. | Usa o envelope global `{error, message, request_id, details?}`. |
| `401` | Token ausente, inválido ou expirado. | Exige autenticação. |
| `403` | Usuário autenticado não é admin FPP. | A rota pode disparar SMS real com custo. |
| `503` | `ZIETT_API_KEY` não configurada. | A Ziett não é contactada. |
| `401`, `402`, `422`, `429` ou outro status da Ziett | A própria Ziett rejeitou o envio. | O envelope inclui `ziett_code`, `ziett_trace_id`, `ziett_status`, `ziett_message`, `ziett_service` e, quando enviado pela Ziett, `ziett_fields`. |
| `500` | Falha de rede/timeout ao contactar a Ziett. | Mensagem sanitizada, sem vazar detalhes internos de rede nem API Key. |

**Exemplo de erro repassado da Ziett:**

```json
{
  "error": "UNAUTHORIZED",
  "message": "A Ziett rejeitou o envio da mensagem de teste.",
  "request_id": "8c7e6a5d-9b9f-4fd2-a2d0-3a989a8c2d8b",
  "ziett_code": "AUTH_INVALID_API_KEY",
  "ziett_trace_id": "trace-1",
  "ziett_status": 401,
  "ziett_message": "The provided API key is invalid or has been revoked.",
  "ziett_service": "core"
}
```
