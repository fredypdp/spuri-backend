---
modificado: 28-06-2026 17:10
criado: 05-04-2026 13:01
---
Versão atual: 2.2.0
## Índice

1. [[#1. Convenções Globais]]
2. [[#2. Estruturas de Dados]]
3. [[#3. Autenticação]]
4. [[#4. Perfil e Conta]]
5. [[#5. Email]]
6. [[#6. Academias]]
7. [[#7. Ano Letivo]]
8. [[#8. Estudantes]]
9. [[#9. Solicitação de Matrícula]]
10. [[#10. Cursos]]
11. [[#11. Matérias]]
12. [[#12. Turmas]]
13. [[#13. Notas]]
14. [[#14. Sumários/Aulas]]
15. [[#15. Faltas]]
16. [[#16. Avaliações Finais]]
17. [[#17. Admins]]
18. [[#18. Jobs Assíncronos]]
19. [[#19. Batch Assíncrono]]
20. [[#20. Armazenamento]]

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

O formato legado de erro simples não é contrato suportado.

> `details` é opcional. Ele aparece quando a rota consegue apontar exatamente
> o campo, o código interno do problema e uma explicação acionável para o
> cliente corrigir a requisição. Em `/academia/anos-academicos`, `details`
> também pode aparecer em `409 Conflict` quando a alteração é bloqueada por
> estudantes ativos vinculados ao ano/período removido.

---

## 2. Estruturas de Dados

### 2.1 Tipos Base

```typescript
type UserType   = 'academia' | 'estudante' | 'admin'
type AdminRole  = 'fpp' | 'adm' | 'gerente'
type AcademiaNivel = 'escola' | 'superior'
type AcademiaType = 'public' | 'private'
type NivelEscolar = 'fundamental' | 'medio' | 'misto'
type StatusGeralEstudante = 'inativo' | 'ativo' | 'arquivado' | 'pendente_documentos'
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
type SolicitacaoMatriculaStatus = 'pendente' | 'aprovada' | 'reprovada'
```

**Períodos de nota:**

- Escolar (fixos): `1_trimestre`, `2_trimestre`, `3_trimestre`
- Superior (dinâmicos): `1_semestre`, `2_semestre`, ..., `N_semestre`

**Categorias de nota:**

- Escolas (`nivel="escola"`) usam categorias fixas do sistema por ano acadêmico. Elas são retornadas na listagem como `source="system"`, `fixed=true`, `readonly=true` e não podem ser criadas/removidas pela academia.
- Categorias escolares regulares: `nota_professor` e `prova_trimestral`.
- Anos com exame (`6_ano_fundamental`, `9_ano_fundamental`, `3_ano_medio`) também aceitam `exame_final` e `exame_recurso`.
- O `4_ano_medio` de curso médio `tecnico` usa apenas `nota_pap` (`Prova de Aptidão Profissional`).
- Academias superiores (`nivel="superior"`) continuam usando categorias configuráveis; toda categoria usada para lançar nota, montar fórmula ou validar regra superior deve ser cadastrada explicitamente pela academia.

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
  telefone_responsavel?: string  // 9 dígitos, sem DDI
  telefone_responsavel_verificado: boolean // reservado
  email_verificado: boolean
  bilhete_identidade?: string
  bilhete_identidade_responsavel?: string
  genero: Genero
  data_nascimento: string         // 'YYYY-MM-DD'
  codigo_academia?: string
  status: string                  // 'ativo' | 'inativo'
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
  bilhete_identidade?: string
  bilhete_identidade_responsavel?: string
  ano_escolar_fundamental?: string
  ano_escolar_medio?: string
  curso_medio_id?: string
  ano_superior?: string
  curso_superior_id?: string
  status: SolicitacaoMatriculaStatus
  motivo_reprovacao?: string
  documentos: Record<string, SolicitacaoMatriculaDocumentoDTO>
  codigo_estudante_gerado?: string
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
  anos_academicos?: string[]  // ex: ['2_ano_fundamental'] ou ['1_ano_medio']
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

### 2.10 Falta

```typescript
interface FaltaDTO {
  id: string
  codigo_estudante: string
  codigo_academia: string
  ano_lectivo: string
  ano_academico: string
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

### 2.11 Registro de Nota (consulta global)

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

### 2.12 Registro de Falta (consulta global)

```typescript
interface FaltaRegistroDTO {
  id: string
  codigo_estudante: string
  estudante_nome: string
  codigo_academia: string
  academia_nome: string
  ano_lectivo: string
  ano_academico: string
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

### 2.14 Categoria de Nota

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

As rotas `GET /academia/cursos?codigo_academia=...` e `GET /academia/curso/:id` também são públicas com autenticação opcional para consulta dos cursos e dos anos desses cursos em escolas do médio e academias do nível superior. Academias autenticadas continuam consultando os próprios cursos sem informar `codigo_academia`; admins autenticados continuam informando `codigo_academia` na listagem.

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
    "bilhete_identidade_responsavel": "string",
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

Gera e retorna o token de verificação ao frontend, que fica responsável por enviar o email.

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
	  { nome: 'CUANDO CUBANGO', codigo: 'CND' },
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
  "type": "private",
  "provincia": "luanda",
  "endereco": "string",
  "telefone": "string",
  "email": "string",
  "website": "string",
  "nivel_escolar": "fundamental",
  "anos_academicos": ["1_ano_fundamental"],
  "cursos": ["Curso A"]
}
```

**Response 200:**

```json
{
  "message": "dados atualizados com sucesso"
}
```

**Nota**: se o email for alterado, `email_verificado` volta para `false`; se o telefone for alterado, `telefone_verificado` volta para `false`.

---

### Regras automáticas de documentos de matrícula

A obrigatoriedade dos documentos e as validações cadastrais comuns são aplicadas por uma política única compartilhada por `POST /solicitacao-matricula`, pela aprovação da solicitação e pelo cadastro direto `POST /academia/estudante/register`. As duas rotas normalizam os mesmos campos de estudante, responsável, nível de ensino, telefones, bilhetes e documentos antes de qualquer gravação no ledger. A validação considera simultaneamente os campos textuais do request e os PDFs anexados:

- `1_ano_fundamental` não exige `declaracao` nem certificado acadêmico anterior.
- Todo ano escolar sequencial com ano anterior exige `declaracao` acompanhada do campo textual `declaracao_ano_academico`, e esse valor deve ser exatamente o ano acadêmico imediatamente anterior ao ano pretendido.
- `7_ano_fundamental` exige `certificado_6_ano_fundamental` ou `declaracao` com `declaracao_ano_academico=6_ano_fundamental`.
- `1_ano_medio` exige `certificado_9_ano_fundamental` ou `declaracao` com `declaracao_ano_academico=9_ano_fundamental`.
- `1_ano_superior` exige `certificado_ensino_medio` ou `declaracao` com `declaracao_ano_academico=3_ano_medio`.
- Declaração sem `declaracao_ano_academico`, do mesmo ano, de ano posterior ou de ano anterior não imediato é rejeitada quando ela é necessária para cumprir a regra acadêmica.
- Na persistência, a declaração deixa de ser documento acadêmico genérico: uploads e documentos informados no corpo JSON são normalizados para `tipo=declaracao_<ano_academico>` e chave `nivel.ano_academico.declaracao_<ano_academico>`, por exemplo `medio.3_ano_medio.declaracao_3_ano_medio`.
- Quando uma declaração opcional é enviada sem ano acadêmico e não participa da validação obrigatória, ela não é gravada como chave raiz `declaracao`; o backend isola o registro em `escopo_desconhecido.declaracao` até que o escopo possa ser corrigido.
- No ensino superior, `bilhete_identidade` do estudante e PDF `bi_estudante` são obrigatórios; `bilhete_identidade_responsavel` e PDF `bi_responsavel` são opcionais.
- No nível escolar/fundamental/médio, `bilhete_identidade_responsavel` e PDF `bi_responsavel` são sempre obrigatórios.
- No nível escolar/fundamental/médio, o estudante deve ter `bilhete_identidade` + PDF `bi_estudante`, ou PDF `cedula_estudante` quando não tiver BI próprio.
- `bilhete_identidade` e `bilhete_identidade_responsavel`, quando ambos informados para o mesmo estudante, não podem ser iguais.
- Para estudantes escolares/fundamental/médio, o BI do responsável não pode coincidir com o BI principal de outro estudante escolar/fundamental/médio; ele pode repetir como BI de responsável de outros estudantes.

**Telefone por nível de ensino:**

- No nível escolar/fundamental/médio, `telefone_responsavel` é obrigatório; `telefone` do estudante é opcional e não substitui o telefone do responsável.
- No ensino superior, `telefone` do estudante é obrigatório; `telefone_responsavel` é opcional.
- Quando `telefone` e `telefone_responsavel` forem enviados, ambos devem ter formato válido de 9 dígitos locais e não podem ser iguais.

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
  "limit": 50,
  "offset": 0
}
```

**Nota**: usuários autenticados veem os campos operacionais do `AcademiaDTO`, incluindo `documentos.alvara.download_url`; admins veem campos extras (`email`, `total_estudantes`, `version`). O backend nunca retorna mais de 100 academias por página, mesmo que o cliente envie `limit` maior.

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
| `fundamental` | Academia autenticada (`projection_academias.anos_academicos`) | `type`, `anos_academicos` | `type`, `anos_academicos` | Une os anos enviados com os anos fundamentais já ativos. |
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

**Importante:** `PATCH /academia/anos-academicos` foi removido do roteamento e do contrato público. Clientes devem usar `POST` para adicionar e `DELETE` para remover escopos específicos, sem fallback para substituição de lista.

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
  "message": "O campo 'type' recebeu '', mas só aceita: 'fundamental', 'medio' ou 'superior'. Use 'fundamental' para anos do ensino fundamental, 'medio' para cursos médios e 'superior' para cursos superiores.",
  "request_id": "uuid-da-requisicao",
  "details": [
    {
      "field": "type",
      "code": "valor_invalido",
      "message": "O campo 'type' recebeu '', mas só aceita: 'fundamental', 'medio' ou 'superior'. Use 'fundamental' para anos do ensino fundamental, 'medio' para cursos médios e 'superior' para cursos superiores."
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

Retorna o ano letivo ativo da academia autenticada.

**Proteção**: autenticado + academia ativa **ou** admin

**Query params:**

- `codigo_academia` (opcional para academia, obrigatório para admin): código da academia alvo.
  - Se o usuário for `academia`, o backend ignora o parâmetro e retorna o próprio ano letivo.
  - Se o usuário for `admin`, deve informar `?codigo_academia=...`.


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
- `404` — academia não encontrada (incluindo admin sem `codigo_academia`)

---

### GET /academia/anos-letivos-lista

Retorna a lista histórica de anos letivos definidos pela academia alvo.

**Proteção**: autenticado + academia ativa **ou** admin

**Query params:**

- `codigo_academia` (opcional para academia, obrigatório para admin): código da academia alvo.
  - Se o usuário for `academia`, o backend ignora o parâmetro e retorna a própria lista.
  - Se o usuário for `admin`, deve informar `?codigo_academia=...`.


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

- `404` — academia não encontrada (incluindo admin sem `codigo_academia`)

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

A academia autenticada finaliza o ano letivo ativo no próprio escopo e, na mesma operação, avança automaticamente para o ano letivo seguinte. O cliente não envia `academia_id`; o backend obtém a academia pelo token, normaliza `type`, valida que o `ano_letivo` informado, quando presente, corresponde ao ano letivo ativo da academia, valida o formato `YYYY_YYYY` com segundo ano igual ao primeiro + 1, valida a janela mensal de finalização pelo `periodo` fixo derivado do tipo e grava um evento auditável `AnoLetivoAcademiaFinalizado`. A janela mensal é inclusiva no mês final e exclusiva no mês inicial: o mês atual precisa ser maior ou igual ao mês de fim do período letivo e menor que o mês de início do período letivo. Exemplo: se `periodo=10_07`, a finalização é permitida somente em julho, agosto e setembro; em outubro o próximo período já começou, e de novembro a junho o período vigente ainda não chegou ao mês de encerramento.

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

A operação é auditada por `(academia_id, type, ano_letivo_finalizado)` e avança a academia para o próximo `YYYY_YYYY`. Se, após esse avanço, todas as academias ativas do mesmo tipo estiverem no mesmo ano letivo, o backend atualiza automaticamente o ano letivo global daquele tipo para esse ano. Fora da janela mensal permitida, o backend retorna `400` e não grava novo evento.

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
8. Se qualquer validação ou persistência falhar após upload parcial, o diretório de documentos do estudante é removido para evitar ficheiros órfãos.

**Regras de validação:**

- `genero` obrigatório: `masculino` ou `feminino`
- `data_nascimento` obrigatório: deve ser anterior à data atual
- JSON puro não é aceito no cadastro direto; o fluxo deve usar `multipart/form-data`, mesmo quando nenhum anexo for enviado
- `bilhete_identidade_responsavel` e PDF `bi_responsavel` são obrigatórios para estudantes escolares/fundamental/médio; no ensino superior, o responsável é opcional
- `bilhete_identidade` e `bilhete_identidade_responsavel`, quando ambos informados, não podem ser iguais após normalização
- `bi_estudante` é obrigatório no ensino superior e quando o estudante escolar informa BI próprio; sem BI próprio no escolar, `cedula_estudante` é obrigatória
- o BI do responsável não pode coincidir com o BI principal de outro estudante escolar/fundamental/médio, mas pode repetir como BI de responsável de irmãos/outros estudantes
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

Cadastra um novo estudante vinculado à academia autenticada. O cadastro direto usa `multipart/form-data` e aplica a mesma política documental da solicitação de matrícula para campos textuais e PDFs. JSON puro não é aceito. Para estudantes escolares/fundamental/médio, exige BI textual/PDF do responsável e BI textual/PDF do estudante ou cédula; para ensino superior, exige BI textual/PDF do estudante e mantém o responsável opcional.

**Proteção**: autenticado + academia ativa

**Content-Type:** `multipart/form-data`

**Campos de texto:**

| Campo | Obrigatório | Observações |
| --- | --- | --- |
| `nome` | sim | Nome completo válido. |
| `genero` | sim | `masculino` ou `feminino`. |
| `data_nascimento` | sim | Data simples `YYYY-MM-DD`, anterior à data atual. |
| `email` | não | Validado quando informado. |
| `telefone` | condicional | Obrigatório no ensino superior. Opcional para escolar/fundamental/médio. Quando enviado, deve ter 9 dígitos locais e não pode ser igual a `telefone_responsavel`. |
| `telefone_responsavel` | condicional | Obrigatório para escolar/fundamental/médio. Opcional no ensino superior. Quando enviado, deve ter 9 dígitos locais e não pode ser igual a `telefone`. |
| `bilhete_identidade` | condicional | Obrigatório no ensino superior; para escolar/fundamental/médio é obrigatório quando o estudante usa BI próprio em vez de cédula. Deve ser único entre estudantes. |
| `bilhete_identidade_responsavel` | condicional | Obrigatório para estudante escolar/fundamental/médio; opcional no ensino superior. Não pode ser igual ao BI do estudante após normalização nem coincidir com o BI principal de outro estudante escolar/fundamental/médio. |
| `ano_escolar_fundamental` | condicional | Ano fundamental canônico, quando aplicável. |
| `ano_escolar_medio` | condicional | Ano médio canônico, quando aplicável. |
| `curso_medio_id` | condicional | UUID de curso médio ativo da academia, quando o ano médio for informado. |
| `ano_superior` | condicional | Ano superior canônico, quando aplicável. |
| `curso_superior_id` | condicional | UUID de curso superior ativo da academia, quando o ano superior for informado. |

**Ficheiros PDF aceitos:**

| Campo de arquivo | Regra |
| --- | --- |
| `bi_responsavel` | Obrigatório para escolar/fundamental/médio; opcional no ensino superior. |
| `bi_estudante` | Obrigatório no ensino superior e obrigatório no escolar quando `bilhete_identidade` do estudante for informado. |
| `cedula_estudante` | Obrigatória para estudante escolar/fundamental/médio sem BI próprio. |
| `declaracao` | PDF da declaração acadêmica. Obrigatória nos anos escolares com ano anterior quando não houver certificado substitutivo válido; exige o campo textual `declaracao_ano_academico` com o ano imediatamente anterior. |
| `declaracao_ano_academico` | Campo textual obrigatório quando `declaracao` for enviada para cumprir comprovativo acadêmico; exemplos: `1_ano_fundamental` para ingresso no `2_ano_fundamental`, `6_ano_fundamental` para ingresso no `7_ano_fundamental`, `9_ano_fundamental` para ingresso no `1_ano_medio`, `3_ano_medio` para ingresso no `1_ano_superior`. O valor também define o `tipo`, `nivel`, `ano_academico`, chave lógica e path do documento persistido. |
| `certificado_6_ano_fundamental` | Exigido como alternativa à declaração somente para `7_ano_fundamental`. |
| `certificado_9_ano_fundamental` | Exigido como alternativa à declaração somente para `1_ano_medio`. |
| `certificado_ensino_medio` | Exigido como alternativa à declaração somente para `1_ano_superior`. |

Quando enviados, todos os ficheiros devem ter `Content-Type: application/pdf`, extensão `.pdf`, assinatura `%PDF` e tamanho máximo de 10MB. O cadastro direto usa a mesma validação compartilhada de matrícula aplicada por `POST /solicitacao-matricula` para dados comuns e documentos. Os documentos obrigatórios são validados e enviados para `{codigo_academia}/estudantes/{codigo_estudante}/documentos/` antes de qualquer gravação no ledger; somente após sucesso total dos uploads o evento `EstudanteCriadoComVinculo` é persistido com metadados normalizados em `documentos.<chave>.documento_id`, `documentos.<chave>.tipo`, `documentos.<chave>.nivel`, `documentos.<chave>.ano_academico`, `documentos.<chave>.versao`, `documentos.<chave>.path`, `documentos.<chave>.file_url` e `documentos.<chave>.download_url`. Para identificação, a chave continua sendo o tipo do arquivo (`bi_estudante`, `bi_responsavel`, `cedula_estudante`); para documentos acadêmicos, a chave segue `nivel.ano_academico.tipo`, como `medio.3_ano_medio.declaracao_3_ano_medio`. Se validação/upload falhar, nenhum estudante/vínculo é gravado; se a criação falhar após upload parcial, o backend remove o diretório definitivo do estudante para evitar ficheiros órfãos.

**Request — campos de texto principais (exemplo escolar; requer anexar os PDFs obrigatórios do quadro acima):**

```text
nome=João Silva
genero=masculino
data_nascimento=2010-05-20
telefone=923000000
telefone_responsavel=924000000
bilhete_identidade=001234567LA089
bilhete_identidade_responsavel=009876543LA089
ano_escolar_fundamental=7_ano_fundamental
```

**Request — multipart/form-data com documentos obrigatórios:**

```text
nome=João Silva
genero=masculino
data_nascimento=2010-05-20
telefone=923000000
telefone_responsavel=924000000
bilhete_identidade=001234567LA089
bilhete_identidade_responsavel=009876543LA089
ano_escolar_fundamental=7_ano_fundamental
bi_estudante=@./bi_estudante.pdf;type=application/pdf
bi_responsavel=@./bi_responsavel.pdf;type=application/pdf
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
  -F "telefone_responsavel=924000000" \
  -F "bilhete_identidade=001234567LA089" \
  -F "bilhete_identidade_responsavel=009876543LA089" \
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
  -F "telefone_responsavel=924000000" \
  -F "bilhete_identidade=001234567LA089" \
  -F "bilhete_identidade_responsavel=009876543LA089" \
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
    "documentos": {
      "bi_responsavel": {
        "path": "LDA20261/estudantes/ABC1234/documentos/bi_responsavel_ABC1234.pdf",
        "file_url": "https://...",
        "download_url": "https://..."
      }
    }
  }
}
```

**Erros:**

- `400` — `Content-Type` diferente de `multipart/form-data`
- `400` — genero inválido, data_nascimento inválida ou no futuro
- `400` — ano académico em formato incorreto ou incompatível com a academia/curso
- `400` — ficheiro não PDF, sem assinatura `%PDF`, com extensão diferente de `.pdf` ou acima de 10MB
- `400` — BI do estudante igual ao BI do responsável, ou BI do estudante já cadastrado

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
      "telefone_responsavel": "924000000",
      "bilhete_identidade_responsavel": "009876543LA089",
      "ano_escolar_fundamental": "1_ano_fundamental"
    }
  ]
}
```

Neste modo são validados somente os campos textuais pelas mesmas regras de `POST /academia/estudante/register`, sem cobrança de PDFs. Cada estudante é criado com `status = "pendente_documentos"` e não deve ser tratado como ativo até concluir a documentação pela rota posterior. Envio de arquivos com `com_arquivo: false` ou `com_arquivo` ausente/inválido é rejeitado.

**Modo com arquivos (`multipart/form-data`)**

Campos:

- `com_arquivo=true`;
- `estudantes`: JSON array com os mesmos campos textuais e um `codigo_temporario` único por estudante;
- arquivos nomeados como `<codigo_temporario>.<campo_documental>`, por exemplo `tmp-1.bi_estudante` e `tmp-1.bi_responsavel`.

```bash
curl -X POST https://api.exemplo.ao/academia/estudante/register/async \
  -H "Authorization: Bearer <jwt_academia>" \
  -F 'com_arquivo=true' \
  -F 'estudantes=[{"codigo_temporario":"tmp-1","nome":"João Silva","genero":"masculino","data_nascimento":"2010-05-20","telefone_responsavel":"924000000","bilhete_identidade_responsavel":"009876543LA089","ano_escolar_fundamental":"1_ano_fundamental"}]' \
  -F 'tmp-1.bi_responsavel=@./bi_responsavel.pdf;type=application/pdf' \
  -F 'tmp-1.cedula_estudante=@./cedula.pdf;type=application/pdf'
```

Arquivos órfãos, `codigo_temporario` duplicado, campos documentais desconhecidos, documentos ausentes obrigatórios e PDFs inválidos seguem as mesmas validações documentais do cadastro singular/solicitação de matrícula.

**Response:** segue o envelope de lote `{total, sucesso, falhas, items[]}`.

### POST /academia/estudante/{codigo_estudante}/documentos

Carrega posteriormente os documentos de estudante cadastrado em lote JSON com `status = "pendente_documentos"`. Aceita apenas `multipart/form-data` com os mesmos campos de arquivo de `POST /academia/estudante/register`. A rota valida documentos com a política compartilhada de matrícula/cadastro direto, armazena em `{codigo_academia}/estudantes/{codigo_estudante}/documentos/` e só grava o evento de conclusão quando todos os documentos obrigatórios estiverem válidos.

A cobrança de Bilhete de Identidade respeita os dados textuais já cadastrados: se houver somente BI textual do responsável, exige somente `bi_responsavel`; se houver somente BI textual do estudante, exige somente `bi_estudante`; se ambos existirem, exige ambos; outras obrigatoriedades condicionais existentes continuam aplicáveis. Estudantes ativos, arquivados, inexistentes ou de outra academia são rejeitados.

```bash
curl -X POST https://api.exemplo.ao/academia/estudante/ABC1234/documentos \
  -H "Authorization: Bearer <jwt_academia>" \
  -F 'bi_responsavel=@./bi_responsavel.pdf;type=application/pdf' \
  -F 'cedula_estudante=@./cedula.pdf;type=application/pdf'
```

**Response 200:** retorna `codigo_estudante`, `status: "ativo"` e o mapa `documentos`.

---

### GET /estudantes

Lista estudantes. Retorna apenas os da academia (para academia) ou todos (para admin).

**Proteção**: autenticado + academia ou admin

**Query Params:**

- `genero` — filtro por gênero (`masculino`, `feminino`). Aceita múltiplos valores (`?genero=masculino,feminino` ou repetindo o parâmetro).
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
> - `GET /estudantes?genero=feminino&idade_min=12&idade_max=15&turno=manha`
> - `GET /estudantes?status_escolar_medio=em_andamento&codigo_turma=TURMA-10A&com_turma=true`
> - `GET /estudantes?codigo_academia=LDA20261&semestre_atual=1,2&curso_id=550e8400-e29b-41d4-a716-446655440000`


**Request:** sem payload
**Response 200:**

```json
{
  "estudantes": [EstudanteDTO],
  "total": 50,
  "tipo_usuario": "academia",
  "codigo_academia": "LDA20261",
  "nome_academia": "string",
  "limit": 50,
  "offset": 0
}
```

**Erros de validação (400):**

- `com_turma` inválido (deve ser `true` ou `false`).
- `semestre_atual` inválido (deve ser inteiro >= 1).
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

### PUT /estudante/dados-pessoais

Atualiza os dados pessoais do estudante autenticado.

**Proteção**: autenticado + estudante

**Request:** (todos os campos opcionais)

```json
{
  "nome": "string",
  "email": "string",
  "telefone": "string",
  "bilhete_identidade": "string",
  "bilhete_identidade_responsavel": "string",
  "data_nascimento": "2010-05-20"
}
```

**Nota**: `genero` não pode ser alterado. `data_nascimento` deve ser anterior à data atual.

**Response 200:**

```json
{
  "message": "dados pessoais atualizados com sucesso"
}
```

---

### Endpoints de acontecimentos que alteram status do estudante

Os status do estudante não devem ser editados diretamente por payloads genéricos. Nesta API, eles são derivados de acontecimentos reais do domínio acadêmico e gravados no ledger do estudante, preservando auditoria, hash chain e histórico acadêmico. Use estas rotas quando a academia precisar registrar um fato operacional que altera a situação do vínculo ou da etapa acadêmica do estudante.

**Proteção de todas as rotas deste escopo:** autenticado + academia + academia ativa. O estudante informado em `:codigo` precisa existir e pertencer à academia autenticada; caso contrário, a API retorna erro de permissão ou de não encontrado.

**Regras gerais do escopo:**

- Cada operação registra um evento de domínio no ledger do estudante, com o usuário academia que executou a ação e o IP da requisição.
- Trancamentos, interrupções, desvinculações e reintegrações não apagam notas, faltas, avaliações, turmas, documentos ou demais registros históricos.
- `motivo` é obrigatório para interrupção, trancamento e desvinculação, e não pode ser vazio.
- `curso_id`, `curso_medio_id` e `curso_superior_id`, quando enviados, precisam ser UUIDs válidos de cursos existentes e do tipo correto (`medio` ou `superior`).
- Os anos escolares aceitos são validados pelo backend: fundamental usa `1_ano_fundamental` até `9_ano_fundamental`; médio usa o formato numérico `[n]_ano_medio`, como `1_ano_medio`.

#### POST /academia/estudante/:codigo/matricula/fundamental

Registra o acontecimento de matrícula ou retomada do estudante no ensino fundamental. Deve ser usado quando a academia confirma que o estudante vai cursar um ano específico do fundamental dentro da instituição.

**Efeito no estudante:** grava o evento `MatriculaFundamentalEfetivada`, define `status_escolar_fundamental = "em_andamento"` e atualiza `ano_escolar_fundamental` com o ano informado.

**Regras de negócio:**

- O estudante não pode estar com `status = "arquivado"`; estudantes arquivados devem passar por revinculação antes de nova matrícula.
- `ano_escolar_fundamental` é obrigatório e deve estar entre `1_ano_fundamental` e `9_ano_fundamental`.
- A operação deve representar uma matrícula efetiva, não uma simples correção cadastral.

**Processo de negócio recomendado:**

1. Validar documentos e elegibilidade do estudante no processo interno da academia.
2. Confirmar o ano fundamental que será cursado.
3. Chamar esta rota para registrar a matrícula no ledger e ativar a etapa fundamental.

**Request:**

```json
{
  "ano_escolar_fundamental": "1_ano_fundamental"
}
```

**Response 200:**

```json
{
  "message": "matrícula fundamental efetivada",
  "status_escolar_fundamental": "em_andamento"
}
```

#### POST /academia/estudante/:codigo/matricula/medio

Registra o acontecimento de matrícula no ensino médio. Deve ser usado quando a academia confirma o ingresso do estudante no médio e precisa associá-lo a um curso médio da instituição.

**Efeito no estudante:** grava o evento `MatriculaMedioEfetivada`, define `status_escolar_medio = "em_andamento"`, atualiza `ano_escolar_medio` e vincula `curso_medio_id`.

**Regras de negócio:**

- O ensino fundamental do estudante deve estar com `status_escolar_fundamental = "finalizado"`.
- `ano_escolar_medio` é obrigatório e deve seguir o formato `[n]_ano_medio`, por exemplo `1_ano_medio`.
- `curso_id` é obrigatório, precisa existir e precisa ser de um curso do tipo `medio`.
- Use esta rota para efetivar matrícula no médio; alterações posteriores de curso devem seguir o fluxo próprio de alteração de curso, quando aplicável.

**Processo de negócio recomendado:**

1. Confirmar que o fundamental foi concluído ou reconhecido por equivalência no sistema.
2. Selecionar o curso médio correto.
3. Registrar a matrícula no ano médio aplicável.

**Request:**

```json
{
  "ano_escolar_medio": "1_ano_medio",
  "curso_id": "uuid-do-curso-medio"
}
```

**Response 200:**

```json
{
  "message": "matrícula no médio efetivada",
  "status_escolar_medio": "em_andamento"
}
```

#### POST /academia/estudante/:codigo/matricula/superior

Registra o acontecimento de matrícula no ensino superior. Deve ser usado quando a academia confirma o ingresso ou retorno do estudante a um curso superior da instituição.

**Efeito no estudante:** grava o evento `MatriculaSuperiorEfetivada`, define `status_superior = "em_andamento"` e vincula `curso_superior_id`. Quando o curso é novo ou diferente do curso superior já registrado, a progressão começa em `ano_superior = "1_ano_superior"` e `semestre_atual = 1`. Quando o curso informado é o mesmo já registrado, o backend preserva o `ano_superior` e o `semestre_atual` anteriores, se existirem.

**Regras de negócio:**

- `curso_id` é obrigatório, precisa existir e precisa ser de um curso do tipo `superior`.
- O fundamental deve estar `finalizado` ou `inativo`.
- O médio deve estar `finalizado` ou `inativo`.
- A rota não recebe ano nem semestre; esses campos são calculados pelo backend conforme histórico e curso informado.

**Processo de negócio recomendado:**

1. Validar que o estudante atende aos critérios de ingresso no superior ou possui equivalência/dispensa registrada quando necessário.
2. Selecionar o curso superior correto.
3. Chamar a rota para registrar a matrícula e iniciar ou retomar a progressão superior.

**Request:**

```json
{
  "curso_id": "uuid-do-curso-superior"
}
```

**Response 200:**

```json
{
  "message": "matrícula superior efetivada",
  "status_superior": "em_andamento",
  "ano_superior": "1_ano_superior",
  "semestre_atual": 1
}
```

#### POST /academia/estudante/:codigo/interrupcao/fundamental

Registra a interrupção do percurso do estudante no ensino fundamental. Deve ser usado quando o estudante deixa temporariamente de cursar o fundamental na academia, sem remover seu histórico.

**Efeito no estudante:** grava o evento `FundamentalInterrompido` e define `status_escolar_fundamental = "inativo"`.

**Regras de negócio:**

- Só é permitido interromper quando `status_escolar_fundamental = "em_andamento"`.
- `motivo` é obrigatório e deve explicar o fato operacional, como mudança de residência, pausa familiar ou impedimento temporário.
- A interrupção não finaliza o fundamental e não apaga histórico acadêmico.

**Processo de negócio recomendado:**

1. Receber a solicitação ou decisão administrativa de interrupção.
2. Registrar o motivo de forma objetiva.
3. Chamar esta rota para inativar a etapa fundamental mantendo a trilha de auditoria.

**Request:**

```json
{ "motivo": "mudança de residência" }
```

**Response 200:**

```json
{
  "message": "fundamental interrompido",
  "status_escolar_fundamental": "inativo"
}
```

#### POST /academia/estudante/:codigo/interrupcao/medio

Registra a interrupção do percurso do estudante no ensino médio. Deve ser usado quando o estudante deixa temporariamente de cursar o médio na academia, preservando curso, ano e histórico já lançados.

**Efeito no estudante:** grava o evento `MedioInterrompido` e define `status_escolar_medio = "inativo"`.

**Regras de negócio:**

- Só é permitido interromper quando `status_escolar_medio = "em_andamento"`.
- `motivo` é obrigatório e não pode ser vazio.
- A interrupção não conclui o médio, não altera o curso médio e não remove notas, faltas ou avaliações.

**Processo de negócio recomendado:**

1. Confirmar a interrupção junto ao estudante/responsável ou setor acadêmico.
2. Informar o motivo administrativo.
3. Registrar o acontecimento para manter o histórico auditável.

**Request:**

```json
{ "motivo": "pausa solicitada" }
```

**Response 200:**

```json
{
  "message": "médio interrompido",
  "status_escolar_medio": "inativo"
}
```

#### POST /academia/estudante/:codigo/trancamento/superior

Registra o trancamento do curso superior. Deve ser usado quando o estudante suspende formalmente o vínculo acadêmico no superior sem cancelar seu histórico ou sua progressão anterior.

**Efeito no estudante:** grava o evento `SuperiorTrancado` e define `status_superior = "inativo"`.

**Regras de negócio:**

- Só é permitido trancar quando `status_superior = "em_andamento"`.
- `motivo` é obrigatório e deve representar a justificativa do trancamento formal.
- O trancamento preserva `curso_superior_id`, `ano_superior`, `semestre_atual` e todos os registros acadêmicos anteriores.

**Processo de negócio recomendado:**

1. Homologar o pedido de trancamento conforme regras internas da academia.
2. Registrar o motivo do trancamento.
3. Chamar a rota para inativar o superior mantendo a possibilidade de retorno posterior.

**Request:**

```json
{ "motivo": "trancamento formal" }
```

**Response 200:**

```json
{
  "message": "superior trancado",
  "status_superior": "inativo"
}
```

#### POST /academia/estudante/:codigo/desvincular

Registra a saída do estudante da academia. Deve ser usado quando a academia encerra o vínculo institucional do estudante, por transferência, cancelamento, desligamento administrativo ou outro motivo formal, preservando todo o histórico para consulta e eventual retorno.

**Efeito no estudante:** grava o evento `EstudanteDesvinculadoDaAcademia`, define `status = "arquivado"` e registra no evento `codigo_academia`, `codigo_estudante`, `motivo` e o nível acadêmico atual calculado pelo backend. O nível pode indicar a etapa em andamento, como `fundamental:1_ano_fundamental`, `medio:1_ano_medio`, `superior:1_ano_superior:semestre_1`, ou `sem_etapa_em_andamento`.

**Regras de negócio:**

- O estudante precisa pertencer à academia autenticada.
- Apenas estudante com `status = "ativo"` pode ser desvinculado.
- `motivo` é obrigatório e não pode ser vazio.
- A desvinculação não remove o vínculo histórico nem apaga dados acadêmicos; ela arquiva o estudante para impedir operações de matrícula direta sem reintegração.

**Processo de negócio recomendado:**

1. Confirmar o encerramento do vínculo institucional.
2. Registrar o motivo da saída.
3. Chamar esta rota para arquivar o estudante e manter o histórico auditável.
4. Em caso de retorno futuro, usar `/academia/estudante/:codigo/revincular`.

**Request:**

```json
{ "motivo": "transferência para outra instituição" }
```

**Response 200:**

```json
{
  "message": "estudante desvinculado da academia",
  "status": "arquivado"
}
```

#### POST /academia/estudante/:codigo/revincular

Registra a reintegração de um estudante arquivado à academia. Deve ser usado quando um estudante anteriormente desvinculado retorna para continuar ou reiniciar uma etapa acadêmica dentro da mesma academia.

**Efeito no estudante:** grava o evento `EstudanteReintegrado`, define `status = "ativo"` e reativa a etapa indicada em `tipo_ensino` com `status_escolar_fundamental`, `status_escolar_medio` ou `status_superior` em `em_andamento`, conforme o caso. A progressão é calculada pelo backend a partir do histórico e do curso informado.

**Regras de negócio:**

- Apenas estudante com `status = "arquivado"` pode ser reintegrado.
- `tipo_ensino` é obrigatório e deve ser `fundamental`, `medio` ou `superior`.
- No reingresso no fundamental, o backend reutiliza o `ano_escolar_fundamental` anterior; se não conseguir determiná-lo, a operação é rejeitada.
- No reingresso no médio, `curso_medio_id` é opcional. Quando omitido, o backend reutiliza o curso médio anterior; se não houver curso anterior, a operação é rejeitada.
- Se o `curso_medio_id` informado for o mesmo curso já registrado, o backend preserva `ano_escolar_medio`; se for um curso diferente, reinicia em `1_ano_medio`.
- No reingresso no superior, `curso_superior_id` é opcional. Quando omitido, o backend reutiliza o curso superior anterior; se não houver curso anterior, a operação é rejeitada.
- Se o `curso_superior_id` informado for o mesmo curso já registrado, o backend preserva `ano_superior` e `semestre_atual`; se for um curso diferente, reinicia em `1_ano_superior` e semestre `1`.
- Cursos informados precisam existir e ter o tipo compatível com a etapa (`medio` ou `superior`).
- A reintegração não recebe ano nem semestre no payload; esses campos são derivados pelo backend.

**Processo de negócio recomendado:**

1. Confirmar que o estudante está arquivado por desvinculação anterior.
2. Definir a etapa de retorno (`fundamental`, `medio` ou `superior`).
3. Para médio ou superior, informar um novo curso somente quando houver mudança real de curso; omitir o curso para continuar no curso anterior.
4. Chamar a rota para reativar o vínculo e registrar o retorno no ledger.

**Request — reingresso no fundamental:**

```json
{
  "tipo_ensino": "fundamental"
}
```

**Request — reingresso no médio mantendo curso anterior:**

```json
{
  "tipo_ensino": "medio"
}
```

**Request — reingresso no médio com mudança de curso:**

```json
{
  "tipo_ensino": "medio",
  "curso_medio_id": "uuid-do-curso-medio"
}
```

**Request — reingresso no superior mantendo curso anterior:**

```json
{
  "tipo_ensino": "superior"
}
```

**Request — reingresso no superior com mudança de curso:**

```json
{
  "tipo_ensino": "superior",
  "curso_superior_id": "uuid-do-curso-superior"
}
```

**Response 200:**

```json
{
  "message": "estudante reintegrado",
  "status": "ativo",
  "tipo_ensino": "superior"
}
```

### GET /eventos-estudante/:codigo

Retorna todos os eventos do ledger de um estudante (trilha de auditoria completa).

**Proteção**: autenticado + admin (qualquer role)

**Path Params:**

- `codigo` — código do estudante


**Request:** sem payload
**Response 200:**

```json
{
  "codigo_estudante": "ABC1234",
  "eventos": [
    {
      "id": 1,
      "event_id": "uuid",
      "aggregate_id": "uuid",
      "aggregate_type": "Estudante",
      "event_type": "EstudanteCriadoComVinculo",
      "event_version": 1,
      "payload": {},
      "metadata": {},
      "occurred_at": "2025-01-01T08:00:00Z",
      "recorded_at": "2025-01-01T08:00:01Z",
      "ledger_hash": "sha256hex",
      "previous_hash": "sha256hex"
    }
  ],
  "total": 15
}
```

---

### GET /verificar-integridade/:codigo

Verifica a integridade da hash chain do ledger para um estudante.

**Proteção**: autenticado (qualquer tipo)

**Nota de acesso:**

- Estudante: apenas o próprio
- Academia: apenas estudantes da própria academia
- Admin: qualquer estudante


**Request:** sem payload
**Response 200:**

```json
{
  "codigo_estudante": "ABC1234",
  "nome": "João Silva",
  "integro": true,
  "message": "✅ Cadeia de hashes íntegra. Eventos não foram alterados."
}
```

---

### GET /estudante/minhas-avaliacoes

Retorna as avaliações finais do estudante autenticado.

**Proteção**: autenticado + estudante

**Query Params:**

- `limit` — quantidade máxima de itens retornados (padrão: 50; máximo: 100)
- `offset` — deslocamento inicial para paginação (padrão: 0)

**Request:** sem payload
**Response 200:**

```json
{
  "avaliacoes": [AvaliacaoFinalDTO],
  "total": 2,
  "limit": 50,
  "offset": 0
}
```

---

---

## 9. Solicitação de Matrícula

### Processos e Regras de Negócio — Solicitação de Matrícula

### Entidade `SolicitacaoMatricula`

A entidade representa o pedido feito pelo estudante para se matricular numa academia. Ela possui código único de 11 caracteres (`codigo_solicitacao`), dados pessoais/académicos, mapa de documentos enviados, status (`pendente`, `aprovada`, `reprovada`) e campos de decisão (`codigo_estudante_gerado`, `motivo_reprovacao`, `aprovada_por`, `reprovada_por`). Cada documento enviado é salvo como objeto com `path`, `file_url` e `download_url`, permitindo que as rotas GET retornem tanto o caminho interno quanto as URLs do arquivo e de download.

Eventos do ledger:

- `SolicitacaoMatriculaCriada`
- `SolicitacaoMatriculaAprovada`
- `SolicitacaoMatriculaReprovada`

### Processo de negócio

1. O estudante envia `POST /solicitacao-matricula` com formulário multipart e PDFs.
2. O backend usa a mesma validação compartilhada do cadastro direto para validar dados comuns, nível de ensino, telefones, bilhetes, academia ativa, assinatura/extensão PDF, limite máximo de 10MB por ficheiro e as regras automáticas de declaração/certificados.
3. A política compartilhada exige, para escolar/fundamental/médio, `telefone_responsavel`, `bilhete_identidade_responsavel`, `bi_responsavel` e BI do estudante com `bi_estudante` ou `cedula_estudante`; para superior, exige `telefone`, `bilhete_identidade` e `bi_estudante`, mantendo dados do responsável opcionais.
4. Antes de criar ou aprovar a solicitação escolar, o handler confirma que o BI do responsável não pertence como BI principal a outro estudante escolar/fundamental/médio já existente.
5. Os documentos são enviados ao storage em `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/`.
6. Para cada PDF, o storage devolve o caminho interno, a URL do arquivo (`file_url`) e a URL de download (`download_url`).
7. Somente após todos os uploads obrigatórios concluírem com sucesso, o aggregate `SolicitacaoMatricula` grava o evento de criação no ledger com os metadados documentais.
8. Se validação ou upload falhar, nenhum evento `SolicitacaoMatriculaCriada` é gravado; se ocorrer falha posterior após upload parcial, o backend tenta remover o diretório da solicitação.
9. A academia lista/consulta solicitações e aprova ou reprova.
10. Na aprovação, o sistema reutiliza o aggregate `Estudante`, revalida os documentos e conflitos atuais, e emite `EstudanteCriadoComVinculo`.
11. Na reprovação, grava o evento de reprovação e remove o diretório dos documentos.

### Regras de negócio

- `telefone_responsavel` é obrigatório para estudantes escolares/fundamental/médio; `telefone` do estudante é opcional nesse nível e não substitui o responsável.
- `telefone` do estudante é obrigatório no ensino superior; `telefone_responsavel` é opcional nesse nível.
- O bilhete de identidade do responsável e o PDF `bi_responsavel` são obrigatórios para estudantes escolares/fundamental/médio e opcionais para ensino superior.
- O bilhete de identidade do estudante e o PDF `bi_estudante` são obrigatórios no ensino superior.
- No escolar/fundamental/médio, a cédula do estudante é obrigatória quando o bilhete de identidade do estudante não for enviado; quando o BI do estudante for enviado, o PDF `bi_estudante` também é obrigatório.
- `1_ano_fundamental` não exige declaração nem certificado.
- Anos escolares sequenciais exigem `declaracao` do ano imediatamente anterior, indicado em `declaracao_ano_academico`; ao persistir, ela é indexada como `nivel.ano_academico.declaracao_<ano_academico>`.
- `7_ano_fundamental` exige certificado do 6.º ano fundamental ou declaração com `declaracao_ano_academico=6_ano_fundamental`.
- `1_ano_medio` exige certificado do 9.º ano fundamental ou declaração com `declaracao_ano_academico=9_ano_fundamental`.
- `1_ano_superior` exige certificado do ensino médio ou declaração com `declaracao_ano_academico=3_ano_medio`.
- Arquivos devem ser PDFs (`Content-Type`, extensão e assinatura `%PDF`).
- Apenas a academia dona pode aprovar/reprovar.
- Solicitação decidida não volta para pendente.
- Rebuild inclui `solicitacoes_matricula` após as projeções principais.


### POST /solicitacao-matricula

Cria uma solicitação pública de matrícula via `multipart/form-data`. O backend gera `codigo_solicitacao`, valida dados e PDFs pelo mesmo validador compartilhado usado no cadastro direto, envia documentos para o armazenamento no caminho `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/` e só então grava `SolicitacaoMatriculaCriada` no ledger. Para cada arquivo enviado, o evento e a projeção salvam metadados compatíveis (`documento_id`, `tipo`, `nivel`, `ano_academico`, `versao`, `path`, `file_url`, `download_url`); o download para leitura deve ser feito pelas rotas autenticadas do backend, sem expor credenciais ou IDs internos do Mega. Documentos acadêmicos usam chave `nivel.ano_academico.tipo` e path com o mesmo escopo, por exemplo `medio/3_ano_medio/declaracao_3_ano_medio/<documento_id>.pdf`.

**Proteção**: pública

**Campos**: `codigo_academia`, `nome`, `genero`, `data_nascimento`, `email`, `telefone`, `telefone_responsavel`, `bilhete_identidade`, `bilhete_identidade_responsavel`, `ano_escolar_fundamental`, `ano_escolar_medio`, `curso_medio_id`, `ano_superior`, `curso_superior_id`. `telefone_responsavel` é obrigatório para escolar/fundamental/médio; `telefone` é obrigatório para ensino superior. Quando `bilhete_identidade` e `bilhete_identidade_responsavel` forem enviados juntos, eles não podem ser iguais (comparação sem espaços nas extremidades e sem diferenciar maiúsculas/minúsculas).

**Ficheiros PDF**: `bi_estudante`, `bi_responsavel`, `cedula_estudante`, `declaracao`, `certificado_6_ano_fundamental`, `certificado_9_ano_fundamental`, `certificado_ensino_medio`. Todos os documentos obrigatórios são validados e enviados ao storage antes da gravação no ledger; falha de upload impede a criação da solicitação. Cada ficheiro deve ser PDF válido e ter no máximo 10MB. Para estudantes escolares/fundamental/médio, `bi_responsavel` é obrigatório e o estudante deve enviar `bi_estudante` com `bilhete_identidade` ou `cedula_estudante` sem BI próprio. Para ensino superior, `bi_estudante` é obrigatório e `bi_responsavel` é opcional. `1_ano_fundamental` não exige comprovativo acadêmico; os demais anos escolares exigem `declaracao` do ano imediatamente anterior informada por `declaracao_ano_academico`, salvo quando um certificado específico válido substituir a declaração em `7_ano_fundamental`, `1_ano_medio` ou `1_ano_superior`. Na resposta e projeção, declarações são retornadas com `tipo=declaracao_<ano_academico>` e chave acadêmica normalizada; certificados ficam vinculados ao ano concluído correspondente.

**Request:** `multipart/form-data` com os campos e ficheiros listados acima.

**Response 201:**

```json
{
  "message": "solicitação de matrícula criada com sucesso",
  "codigo_solicitacao": "A3F9K2BPQ7X",
  "codigo_academia": "LDA20261",
  "status": "pendente"
}
```

### GET /academia/solicitacoes-matricula

Lista solicitações da academia autenticada em ordem decrescente de criação. Para cada documento retornado, `download_url` aponta para a rota autenticada do backend no escopo global (`/documentos/solicitacoes-matricula/{codigo_solicitacao}/{campo}/download`), garantindo download do PDF pelo cliente sem expor credenciais ou IDs internos do storage.

**Proteção**: autenticado + academia

**Query params**:

- `status`: filtro repetível por status (`pendente`, `aprovada`, `reprovada`). Ex.: `?status=pendente&status=reprovada`.
- `limit`: quantidade máxima de registros. Padrão `50`, mínimo `1`, máximo `1000`.
- `offset`: deslocamento de paginação. Padrão `0`.


**Request:** sem payload
**Response 200:**

```json
{
  "solicitacoes": [
    {
      "id": "0d0f5f7d-2f80-4e2d-9b48-b016f8d8f2ab",
      "codigo_solicitacao": "A3F9K2BPQ7X",
      "codigo_academia": "LDA20261",
      "nome": "Maria da Silva",
      "genero": "feminino",
      "data_nascimento": "2010-05-12T00:00:00Z",
      "status": "pendente",
      "documentos": {
        "bi_responsavel": {
          "path": "LDA20261/matriculas/matricula_A3F9K2BPQ7X/bi_responsavel_A3F9K2BPQ7X.pdf",
          "file_url": "LDA20261/matriculas/matricula_A3F9K2BPQ7X/bi_responsavel_A3F9K2BPQ7X.pdf",
          "download_url": "/documentos/solicitacoes-matricula/A3F9K2BPQ7X/bi_responsavel/download"
        }
      },
      "created_at": "2026-06-14T10:00:00Z",
      "updated_at": "2026-06-14T10:00:00Z",
      "version": 1
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /academia/solicitacao-matricula/:codigo

Consulta uma solicitação da academia autenticada pelo `codigo_solicitacao`. Retorna `404` se não existir e `403` se pertencer a outra academia. Os documentos retornados também recebem `download_url` de rota autenticada do backend para download do PDF.

**Proteção**: autenticado + academia dona


**Request:** sem payload
**Response 200:**

```json
{
  "solicitacao": {
    "id": "0d0f5f7d-2f80-4e2d-9b48-b016f8d8f2ab",
    "codigo_solicitacao": "A3F9K2BPQ7X",
    "codigo_academia": "LDA20261",
    "nome": "Maria da Silva",
    "genero": "feminino",
    "data_nascimento": "2010-05-12T00:00:00Z",
    "status": "pendente",
    "documentos": {
      "declaracao": {
        "path": "LDA20261/matriculas/matricula_A3F9K2BPQ7X/declaracao_A3F9K2BPQ7X.pdf",
        "file_url": "LDA20261/matriculas/matricula_A3F9K2BPQ7X/declaracao_A3F9K2BPQ7X.pdf",
        "download_url": "/documentos/solicitacoes-matricula/A3F9K2BPQ7X/declaracao/download"
      }
    },
    "created_at": "2026-06-14T10:00:00Z",
    "updated_at": "2026-06-14T10:00:00Z",
    "version": 1
  }
}
```

### PUT /academia/solicitacao-matricula/:codigo/aprovar

Aprova uma solicitação pendente e cria automaticamente o estudante com o aggregate `Estudante`.


**Request:** sem payload
**Response 200:**

```json
{
  "message": "solicitação aprovada e estudante registado com sucesso",
  "codigo_solicitacao": "A3F9K2BPQ7X",
  "codigo_estudante_gerado": "ABC1234"
}
```

### PUT /academia/solicitacao-matricula/:codigo/reprovar

Reprova uma solicitação pendente, grava `SolicitacaoMatriculaReprovada` e remove o diretório de documentos.

**Request:**

```json
{ "motivo_reprovacao": "Documentos ilegíveis." }
```


**Response 200:**

```json
{
  "message": "solicitação reprovada com sucesso",
  "codigo_solicitacao": "A3F9K2BPQ7X",
  "status": "reprovada"
}
```

### GET /solicitacoes-matricula

Lista todas as solicitações do sistema para admin em ordem decrescente de criação. Retorna o mesmo formato de `GET /academia/solicitacoes-matricula`, incluindo `documentos.<campo>.path`, `documentos.<campo>.file_url` e `documentos.<campo>.download_url` para cada arquivo enviado.

**Proteção**: autenticado + admin

**Query params**: `status` repetível, `codigo_academia` repetível, `limit` e `offset`.

**Request:** sem payload

**Response 200:** mesmo formato de `GET /academia/solicitacoes-matricula`.

---

---

## 10. Cursos

### Processos de Negócio — Gestão de Cursos

### 10.1 Gestão de Cursos e Matérias

**Ciclo de vida do curso:**

```
Criado → Ativado → Desativado → Deletado
       ↑__________|
```

Para deletar um curso:

1. Deve estar inativo
2. Não pode ter estudantes matriculados
3. Não pode ter matérias ativas (desativar todas primeiro)
4. Matérias inativas são deletadas em cascata automaticamente
5. Turmas inativas vinculadas são deletadas em cascata automaticamente


**Ciclo de vida da matéria superior:**

```
Criada (INATIVO) → Período Definido → Ativada (ATIVO) → Desativada → Deletada
```

Matérias fundamental e médio são criadas já **ativas**.

**Validação de período**: o período da matéria deve existir na lista de períodos do curso vinculado.
### Regras de Negócio — Curso

### 10.2 Regras de Curso

| Regra                                                           | Detalhe                                                                                                                                                                                    |
| --------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Tipo imutável após criação e compatível com o nível da academia | Preenchido automaticamente no back end. Quando a academia é do nível escola e `NivelEscolar` = "medio", o tipo será `medio`. Quando a academia é do nível superior o tipo será `superior`. |
| Curso superior exige períodos                                   | Ao menos um semestre                                                                                                                                                                       |
| Curso superior limita semestres por ano                         | `total_semestres >= total_anos` e `total_semestres <= total_anos * 2`                                                                                                                     |
| Curso médio exige modelo                                     | `modelo` obrigatório e exatamente `liceu` ou `tecnico`; cursos superiores não aceitam `modelo`                                                                                          |
| Curso médio não deve ter períodos                               | Trimestres são fixos do sistema                                                                                                                                                            |
| Edição de anos acadêmicos bloqueia remoções em uso              | Não é permitido remover de `anos_academicos` um ano com estudantes ativos matriculados no curso                                                                                            |
| Edição de períodos do superior bloqueia remoções em uso          | Não é permitido remover de `periodos` um semestre com estudantes ativos no curso superior usando o `semestre_atual` correspondente                                                         |
| Deleção exige inatividade                                       | Desativar primeiro                                                                                                                                                                         |
| Deleção exige sem estudantes matriculados                       | Verificação antes de deletar                                                                                                                                                               |
| Matérias ativas bloqueiam deleção                               | Desativar todas as matérias antes                                                                                                                                                          |
| Cascata na deleção                                              | Matérias e turmas inativas e sem estudantes são deletadas automaticamente                                                                                                                  |


### POST /academia/curso

Cria um novo curso para a academia. O tipo efetivo do curso é inferido pelo backend a partir da academia autenticada (`medio` para escola do médio e `superior` para academia superior). O campo `type` pode ser enviado para explicitar a intenção, mas deve corresponder ao tipo permitido para a academia.

**Proteção**: autenticado + academia ativa

**Request para curso médio:**

```json
{
  "nome": "Ciências e Tecnologia",
  "type": "medio",
  "modelo": "liceu"
}
```

**Request para curso superior:**

```json
{
  "nome": "Engenharia Informática",
  "type": "superior",
  "periodos": 8
}
```

Para cursos superiores, `periodos` é um **número inteiro positivo** que representa o total de semestres. O backend persiste internamente os semestres sequenciais (`1_semestre` até `N_semestre`) e calcula `anos_academicos` automaticamente com `ceil(periodos / 2)`. Ex.: `periodos = 3` gera `periodos = ["1_semestre", "2_semestre", "3_semestre"]` e `anos_academicos = ["1_ano_superior", "2_ano_superior"]`.



**Exemplo 400 — curso médio com anos_academicos manual:**

```json
{
  "message": "anos_academicos não é aceito para cursos médios; os anos são fixos e derivados de modelo",
  "error": "anos_academicos não é aceito para cursos médios; os anos são fixos e derivados de modelo"
}
```

**Response 201:**

```json
{
  "message": "curso criado com sucesso",
  "data": {
    "id": "uuid",
    "nome": "string",
    "type": "medio",
    "periodos": [],
  }
}
```

**Erros:**

- `400` — curso médio com `periodos` numérico enviado
- `403` — academia inativa não pode criar cursos

---

### GET /academia/cursos


**Proteção**: pública com autenticação opcional.

- Sem `Authorization`, permite consultar cursos de escolas do médio e academias do nível superior por `codigo_academia`.
- Com `Authorization: Bearer <jwt_token>` válido, mantém o contrato anterior para academias e admins.
- Tokens enviados em formato inválido, expirados ou pertencentes a contas inativas retornam `401`.

**Query params:**

- `codigo_academia` — obrigatório para usuários sem sessão e para admins; ignorado para academias autenticadas, que consultam os próprios cursos.


**Request:** sem payload
**Response 200:**

```json
{
  "cursos": [CursoDTO],
  "total": 3
}
```

---

### GET /academia/curso/:id


**Proteção**: pública com autenticação opcional.

- Sem `Authorization`, permite consultar os anos acadêmicos de cursos de escolas do médio e academias do nível superior pelo ID do curso.
- Academias autenticadas só podem consultar os próprios cursos.
- Admins autenticados podem consultar qualquer curso.


**Request:** sem payload
**Response 200:** `CursoDTO`

---

### PUT /academia/curso/:id/ativar

Ativa um curso inativo.

**Proteção**: autenticado + academia ativa


**Request:** sem payload
**Response 200:**

```json
{
  "message": "curso ativado com sucesso",
  "nome": "string"
}
```

---

### PUT /academia/curso/:id/desativar

Desativa um curso ativo.

**Proteção**: autenticado + academia ativa


**Request:** sem payload
**Response 200:**

```json
{
  "message": "curso desativado com sucesso",
  "nome": "string"
}
```

---

### PUT /academia/curso/:id/dados


**Proteção**: autenticado + academia ativa

**Validações de integridade:**

- Campos acadêmicos como `anos_academicos`, `anosAcademicos`, `periodos`, `semestres`, `quantidade_semestres` e `anos` são rejeitados com erro de validação, sem mutação parcial.
- Cursos superiores não aceitam adição/remoção direta de anos acadêmicos, períodos ou semestres por esta rota nem por `/academia/anos-academicos`; qualquer fluxo futuro de períodos deve ser explícito e separado dos dados cadastrais do curso.

**Request:**

```json
{
  "nome": "Ciências e Tecnologia"
}
```

**Response 200:**

```json
{
  "message": "curso atualizado com sucesso",
  "nome": "string",
  "type": "medio",
  "modelo": "liceu",
  "anos_academicos": ["1_ano_medio", "2_ano_medio"],
  "periodos": [],
    {
      "ano_academico": "1_ano_medio",
    },
    {
      "ano_academico": "2_ano_medio",
    }
  ]
}
```

**Erros:**

- `400` — nenhum campo para atualizar
- `400` — `type` enviado na edição, pois o tipo do curso é imutável
- `400` — campo acadêmico enviado (`anos_academicos`, `anosAcademicos`, `periodos`, `semestres`, `quantidade_semestres`, `anos` ou equivalente)

---


### DELETE /academia/curso/:id

Deleta um curso (soft delete com cascata).

**Proteção**: autenticado + academia ativa

**Request:** (opcional)

```json
{
  "motivo": "string"
}
```

**Response 200:**

```json
{
  "message": "curso deletado com sucesso",
  "curso_id": "uuid",
  "nome": "string",
  "materias_deletadas": ["Matemática", "Física"],
  "turmas_deletadas": ["T1A"],
  "auditavel": true
}
```

**Erros:**

- `400` — curso está ativo (desativar primeiro)
- `400` — curso tem estudantes matriculados
- `400` — curso tem matérias ativas (desativar todas primeiro)
- `400` — curso tem turmas ativas (desativar todas primeiro)

---

---

## 11. Matérias

### Processos de Negócio — Gestão de Matérias

### 11.1 Gestão de Cursos e Matérias

**Ciclo de vida do curso:**

```
Criado → Ativado → Desativado → Deletado
       ↑__________|
```

Para deletar um curso:

1. Deve estar inativo
2. Não pode ter estudantes matriculados
3. Não pode ter matérias ativas (desativar todas primeiro)
4. Matérias inativas são deletadas em cascata automaticamente
5. Turmas inativas vinculadas são deletadas em cascata automaticamente


**Ciclo de vida da matéria superior:**

```
Criada (INATIVO) → Período Definido → Ativada (ATIVO) → Desativada → Deletada
```

Matérias fundamental e médio são criadas já **ativas**.

**Validação de período**: o período da matéria deve existir na lista de períodos do curso vinculado.
### Regras de Negócio — Matéria Disciplinar

### 11.2 Regras de Matéria Disciplinar

| Regra                                                                                                | Detalhe                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Edição restrita à academia dona da matéria                                                           |                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Anos acadêmicos deve ser compatível aos anos acadêmicos da academia ou do curso                      | Ao criar ou editar anos_academicos ele deve ser compatível com os anos acadêmicos da academia (para matéria do tipo fundamental), ou com os anos acadêmicos do curso (matéria do tipo medio ou superior)                                                                                                                                                                                                                                                              |
| Tipo compativel com o nivel da academia                                                              | - Quando a academia é do nível escola e `NivelEscolar` = "fundamental", o tipo será `fundamental`.<br>- Quando a academia é do nível escola e `NivelEscolar` = "medio", o tipo será `medio`.<br>- Quando a academia é do nível superior o tipo será `superior`.<br><br>MateriaType será preenchido automaticamente, apenas quando a academia é do nível escola e `NivelEscolar` = "misto", a academia terá que enviar o tipo definindo se é `fundamental` ou `medio`. |
| Período só pode ser definido para matéria do tipo `superior`. E deve ser compatível com o seu curso. | Matérias `fundamental` e `medio` não aceitam definição de período. E o período da matéria do tipo superior deve ser compatível com um dos períodos do seu curso                                                                                                                                                                                                                                                                                                       |
| Quando a matéria é do tipo `superior` período não pode ser vazio                                     |
| Matérias dependentes são exclusivas do superior                                                      | Configuração de dependências/pendências por matéria é bloqueada para o ensino médio escolar; médio escolar usa o padrão fixo escolar |                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Deleção exige inatividade                                                                            | Matéria com status `ativo` é rejeitada; é obrigatório desativar antes de deletar                                                                                                                                                                                                                                                                                                                                                                                      |


### POST /academia/materia

Cria uma nova matéria disciplinar.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "nome": "Matemática",
  "type": "fundamental",
  "anos_academicos": ["3_ano_fundamental", "4_ano_fundamental"],
  "curso_id": null
}
```

**Para médio/superior:**

```json
{
  "nome": "Álgebra Linear",
  "type": "superior",
  "anos_academicos": ["1_ano_superior"],
  "curso_id": "uuid",  // obrigatório para medio e superior
  "periodo": "1_semestre",  // obrigatório para superior e deve existir nos períodos do curso
  "pendencia_permitida": true,  // apenas superior
  "pendencia_nivel_conclusao": "2_semestre"  // apenas superior
}
```

**Response 201:**

```json
{
  "message": "materia criada com sucesso",
  "data": {
    "id": "uuid",
    "nome": "string",
    "type": "superior",
    "status": "inativo",
    "pendencia_permitida": true,
    "pendencia_nivel_conclusao": "2_semestre",
    "periodo": "1_semestre"
  }
}
```

**Notas:**

- Matérias `superior` nascem **inativas**, exigem `periodo` no `POST /academia/materia` e não permitem edição posterior do período
- `pendencia_permitida` é um booleano disponível apenas para matérias `superior`; quando `true`, indica que o estudante pode avançar com essa matéria pendente para aprovação futura antes de concluir o ciclo
- `pendencia_nivel_conclusao` é uma string disponível apenas para matérias `superior`; deve ser um semestre superior (`N_semestre`) válido do curso e define o último nível em que o estudante poderá chegar com pendências desta matéria
- `curso_id` obrigatório para `medio` e `superior`
- Para `fundamental`: `anos_academicos` com 1 a 9 itens no formato correto
- Para `medio`/`superior`: exatamente 1 item no formato correto
- `periodo` não é aceito em `PUT /academia/materia/:id/dados`; para matérias superiores ele deve ser escolhido somente na criação

---

### GET /academia/materias

Lista todas as matérias da academia.

**Proteção**: autenticado + (`academia` ativa **ou** `admin` **ou** `estudante`)

**Query params (quando `admin`)**:

- `codigo_academia` (obrigatório)


**Request:** sem payload
**Response 200:**

```json
{
  "materias": [MateriaDTO],
  "total": 10
}
```

---

### GET /academia/materia/:id

Retorna uma matéria específica.

**Proteção**: autenticado + (`academia` ativa **ou** `admin` **ou** `estudante`)


**Request:** sem payload
**Response 200:** `MateriaDTO`

---

### PUT /academia/materia/:id/ativar

Ativa uma matéria inativa. Matérias superiores sem período definido não podem ser ativadas.

**Proteção**: autenticado + academia ativa


**Request:** sem payload
**Response 200:**

```json
{
  "message": "materia ativada com sucesso",
  "nome": "string"
}
```

---

### PUT /academia/materia/:id/desativar

Desativa uma matéria ativa.

**Proteção**: autenticado + academia ativa


**Request:** sem payload
**Response 200:**

```json
{
  "message": "materia desativada com sucesso",
  "nome": "string"
}
```

---

### PUT /academia/materia/:id/dados

Atualiza os dados cadastrais de uma matéria. Em matérias superiores, também pode atualizar `pendencia_permitida` e `pendencia_nivel_conclusao`. O campo `periodo` não pode ser editado.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "nome": "string",
  "pendencia_permitida": true,
  "pendencia_nivel_conclusao": "2_semestre"
}
```

**Response 200:**

```json
{
  "message": "matéria atualizada com sucesso",
  "nome": "string",
  "pendencia_permitida": true,
  "pendencia_nivel_conclusao": "2_semestre"
}
```

**Erros:**

- `400` — `periodo` informado na edição; o período só pode ser definido no `POST /academia/materia`
- `400` — `pendencia_permitida=true` informado para matéria do tipo `fundamental` ou `medio`
- `400` — `pendencia_nivel_conclusao` informado para matéria do tipo `fundamental` ou `medio`
- `400` — `pendencia_nivel_conclusao` não corresponde a um semestre superior (`N_semestre`) válido do curso

---

### DELETE /academia/materia/:id

Deleta uma matéria (soft delete). Deve estar inativa.

**Proteção**: autenticado + academia ativa


**Request:** sem payload
**Response 200:**

```json
{
  "message": "materia deletada com sucesso",
  "nome": "string"
}
```

---

---

## 12. Turmas

### Processos de Negócio — Gestão de Turmas

### 12.1 Gestão de Turmas

**Quem faz**: Academia

**Ciclo de vida de uma turma:**

```
Criada (ativo) → Desativada (inativo) → Deletada (deletado)
              ↑_________|
         (pode ativar novamente)
```

**Para deletar**: a turma deve estar inativa e sem estudantes vinculados.

**Adição de estudantes**: o estudante deve pertencer à academia. Apenas estudantes do superior podem estar em múltiplas turmas simultaneamente.

**Remoção automática**: para avaliações finais escolares, removida. Agora há progressão de turma na aprovação e permanência na turma na reprovação.

**Histórico por ano letivo**: cada turma mantém `historico_estudantes_ano_letivo` (mapa `ano_letivo -> [codigo_estudante]`) com os estudantes que já fizeram parte dela em cada ano letivo.
### Regras de Negócio — Turma

### 12.2 Regras de Turma

| Regra                                                                        | Detalhe                                                                                                                                                                                                                                                                                    |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Código único por academia                                                    | Não pode repetir dentro da mesma academia                                                                                                                                                                                                                                                  |
| Turno: `manha`, `tarde` ou `noite`                                           | Valores fixos                                                                                                                                                                                                                                                                              |
| Edição restrita à academia dona da turma                                     |                                                                                                                                                                                                                                                                                            |
| Edição aceita apenas `nivel`, `curso_id` e `turno`                           |                                                                                                                                                                                                                                                                                            |
| Mudança de nível/curso enquanto a turma tem estudantes exige compatibilidade | Se a turma já tiver estudantes vinculados, qualquer alteração de `nivel` e/ou `curso_id` dispara revalidação de compatibilidade antes de persistir. Onde os estudantes devem ter o ano acadêmico igual à esse novo nível, ou o curso_medio_id ou curso_superior_id igual à esse novo curso |
| Deleção exige inatividade                                                    | Desativar antes de deletar                                                                                                                                                                                                                                                                 |
| Deleção exige sem estudantes                                                 | Remover todos os estudantes primeiro                                                                                                                                                                                                                                                       |


### POST /academia/turma

Cria uma nova turma.

O campo `codigo_turma` é normalizado antes de persistir e validar duplicidade: espaços antes/depois são descartados, somente espaços internos entre textos viram `_` (ex.: ` Turma 10 A ` vira `Turma_10_A`) e caracteres especiais diferentes de `_` são rejeitados. O código aceita letras, números, espaços e `_`.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "codigo_turma": "T1A",
  "nivel": "3_ano_fundamental",
  "turno": "manha",
  "curso_id": null  // opcional, para turmas de médio/superior
}
```

**Response 201:**

```json
{
  "message": "turma criada com sucesso",
  "id": "uuid",
  "codigo_turma": "T1A"
}
```

**Erros:**

- `400` — turno inválido (deve ser `manha`, `tarde` ou `noite`) ou `codigo_turma` com caracteres especiais inválidos
- `409` — código de turma já existe nesta academia

---

### GET /academia/turmas

Lista todas as turmas da academia.

**Proteção**: autenticado + (`academia` ativa **ou** `admin` **ou** `estudante`)

**Query params (quando `admin`)**:

- `codigo_academia` (obrigatório)


**Request:** sem payload
**Response 200:**

```json
{
  "turmas": [TurmaDTO]
}
```

---

### GET /academia/turma/:codigo

Retorna uma turma pelo código.

**Proteção**: autenticado + (`academia` ativa **ou** `admin` **ou** `estudante`)

**Query params (quando `admin`)**:

- `codigo_academia` (obrigatório, porque o código da turma é contextual por academia)


**Request:** sem payload
**Response 200:** `TurmaDTO`

---

### PUT /academia/turma/:codigo/ativar

Ativa uma turma inativa.

**Proteção**: autenticado + academia ativa


**Request:** sem payload
**Response 200:**

```json
{
  "message": "turma ativada com sucesso",
  "codigo_turma": "T1A"
}
```

---

### PUT /academia/turma/:codigo/desativar

Desativa uma turma ativa. Pré-requisito para deletar.

**Proteção**: autenticado + academia ativa


**Request:** sem payload
**Response 200:**

```json
{
  "message": "turma desativada com sucesso",
  "codigo_turma": "T1A"
}
```

---

### PUT /academia/turma/:codigo/dados

Atualiza dados de uma turma.

**Proteção**: autenticado + academia ativa

**Regra de compatibilidade (novo)**:
- Ao alterar `nivel` e/ou `curso_id`, o backend valida todos os estudantes já vinculados.
- Se pelo menos um estudante ficar incompatível com os novos dados, a atualização é bloqueada com `400`.

**Request:** (todos opcionais)

```json
{
  "nivel": "string",
  "turno": "tarde",
  "curso_id": "uuid"
}
```

**Response 200:**

```json
{
  "message": "turma atualizada com sucesso"
}
```

**Erros comuns:**
- `400` — estudante vinculado ficaria incompatível com o novo `nivel` e/ou `curso_id`

---

### DELETE /academia/turma/:codigo

Deleta uma turma (soft delete). Deve estar inativa e sem estudantes.

**Proteção**: autenticado + academia ativa

**Request:** (opcional)

```json
{
  "motivo": "string"
}
```

**Response 200:**

```json
{
  "message": "turma deletada com sucesso",
  "codigo_turma": "T1A",
  "auditavel": true
}
```

---

### POST /academia/turma/:codigo/estudante

Adiciona um estudante à turma.

**Proteção**: autenticado + academia ativa

**Path Params:**

- `codigo` — código da turma

**Request:**

```json
{
  "codigo_estudante": "ABC1234"
}
```

**Response 200:**

```json
{
  "message": "estudante adicionado à turma com sucesso",
  "codigo_turma": "T1A",
  "codigo_estudante": "ABC1234"
}
```

**Erros:**

- `403` — estudante não pertence à academia
- `404` — estudante ou turma não encontrados
- `409` — estudante já está na turma

---

### DELETE /academia/turma/:codigo/estudantes/:codigo_estudante

Remove um estudante da turma.

**Proteção**: autenticado + academia ativa


**Request:** sem payload
**Response 200:**

```json
{
  "message": "estudante removido da turma com sucesso",
  "codigo_turma": "T1A",
  "codigo_estudante": "ABC1234"
}
```

---

### GET /turmas-estudante/:codigo

Retorna as turmas de um estudante com autorização por perfil na mesma rota.

**Proteção**: autenticado (qualquer tipo)

**Path Params:**

- `codigo` — código do estudante

**Regras de autorização:**

- `estudante`: só pode consultar as próprias turmas
- `academia`: pode consultar qualquer estudante da sua academia
- `admin`: pode consultar qualquer estudante


**Request:** sem payload
**Response 200:**

```json
{
  "codigo_estudante": "ABC1234",
  "nome": "João Silva",
  "turmas": [TurmaDTO],
  "total": 2
}
```

**Erros:**

- `403` — estudante tentando consultar outro estudante
- `403` — academia tentando consultar estudante de outra academia
- `404` — estudante não encontrado

---

---

## 13. Notas

### Processos de Negócio — Registro de Notas

### 13.1 Registro de Notas

**Quem faz**: Academia (status ativo, com ano letivo configurado)

**Processo:**

1. Academia envia: código do estudante, período, matéria, tipo, categoria, valor
2. Sistema valida o ano letivo ativo da academia
3. Sistema verifica que estudante pertence à academia
4. Sistema verifica que matéria pertence à academia
5. Sistema infere o `ano_academico`:
    - Se estudante tem `ano_escolar_fundamental` preenchido (fundamental) → usa esse valor **somente se** esse ano existir em `anos_academicos` da matéria
    - Se não existir, o registro é bloqueado com erro de validação (incompatibilidade estudante × matéria)
    - Caso contrário → usa `anos_academicos[0]` da matéria
6. Sistema valida a `categoria`: em escolas, ela precisa pertencer ao catálogo fixo do `ano_academico` inferido e do modelo do curso médio; no Superior, ela precisa estar configurada na academia com `anos_academicos` contendo o ano/período inferido. Sem catálogo/correspondência, o registro é bloqueado
7. Sistema verifica idempotência (chave: `codigoAcademia_anoLectivo_periodo_materiaID_tipo_categoria`)
8. Se não for duplicata, emite `NotasRegistradas` no ledger do estudante

**Tipos de nota:**

|Tipo|Academia|Categorias fixas|Períodos|
|---|---|---|---|
|`escolar`|`escola`|Categorias fixas do sistema por ano acadêmico|`1_trimestre`, `2_trimestre`, `3_trimestre`|
|`superior`|`superior`|Categorias cadastradas explicitamente pela academia|Semestres do curso|

No ensino superior, academias criam explicitamente todas as categorias de nota que pretendem usar. Toda categoria superior possui `anos_academicos`; apenas os anos presentes nessa lista aceitam registros. Se a categoria não tiver anos definidos, nenhuma nota pode ser registrada nela. O `codigo` da categoria é normalizado antes de persistir: espaços antes/depois são descartados, somente espaços internos entre textos viram `_`, letras maiúsculas viram minúsculas e caracteres especiais diferentes de `_` são rejeitados.

Nas escolas, a academia não cria nem remove categorias. O backend seleciona automaticamente as categorias fixas pelo `ano_academico` inferido da nota e, no médio técnico, pelo `modelo` do curso. Categorias escolares legadas ou configuráveis eventualmente presentes em projeções não substituem esse catálogo fixo para lançamento de notas nem para avaliação final. Isso garante um padrão avaliativo único entre escolas e evita divergência operacional entre academias.

**Valor**: escala validada por ano acadêmico (`0–10` no 1.º ao 6.º fundamental; `0–20` no 7.º ao 9.º fundamental, médio e superior).

**Imutabilidade de nota**: notas só podem ser criadas e consultadas. Não existe endpoint público, administrativo, batch ou assíncrono para editar, eliminar, restaurar ou ocultar notas por soft delete.

**Proteção contra duplicatas**: o aggregate mantém um mapa em memória (`NotasRegistradasPorChave`). Se a mesma combinação de academia/ano/período/matéria/tipo/categoria já existir, o comando é rejeitado com erro de negócio claro antes de tocar o banco.
### Regras de Negócio — Notas

### 13.2 Regras de Notas

| Regra                                       | Detalhe                                                       |
| ------------------------------------------- | ------------------------------------------------------------- |
| Nota deve respeitar a escala do ano acadêmico | `0–10` para `1_ano_fundamental` a `6_ano_fundamental`; `0–20` para demais anos fundamentais, médio e superior |
| Academia escola só registra notas `escolar` | Academia superior só registra `superior`                      |
| Categorias escolares são fixas | Escolas usam apenas categorias do catálogo do ano: regulares, exames nos anos previstos e `nota_pap` no `4_ano_medio` técnico |
| Categorias configuráveis são do superior | Criação/remoção de categorias é bloqueada para escolas |
| Ano do estudante deve pertencer à matéria   | Se `ano_escolar_fundamental` não estiver em `anos_academicos`, bloqueia   |
| Imutabilidade após criação                  | Notas só podem ser criadas e consultadas; não há edição, deleção, restauração ou soft delete operacional |
| Duplicata bloqueada no aggregate            | Mesma combinação ano/período/matéria/tipo/categoria rejeitada |


### POST /academia/categorias-nota

Cria uma categoria de nota explícita para a academia **somente quando a academia é de ensino superior**. Escolas não podem criar categorias: o padrão avaliativo escolar é fixo do sistema e é exposto apenas pela listagem.

O campo `codigo` é normalizado antes de persistir: espaços antes/depois são descartados, somente espaços internos entre textos viram `_` (ex.: ` Prova profesor ` vira `prova_profesor`) e caracteres especiais diferentes de `_` são rejeitados. O código aceita letras minúsculas, números, espaços e `_`; letras maiúsculas são convertidas para minúsculas.

**Proteção**: autenticado + academia ativa + `nivel="superior"`

**Request:**

```json
{
  "codigo": "prova_profesor",
  "nome": "Prova do professor",
  "descricao": "string",
  "anos_academicos": ["3_ano_fundamental", "4_ano_fundamental"]
}
```

**Response 201:**

```json
{
  "message": "categoria criada com sucesso",
  "categoria": "prova_profesor"
}
```

**Erros:**

- `400` — academia escolar tentando criar categoria, codigo/nome/anos_academicos ausente/vazio, ou codigo com caracteres especiais inválidos
- `409` — categoria já existe nesta academia

---

### GET /academia/categorias-nota

Lista todas as categorias de nota da academia alvo. Para escolas, a resposta soma eventuais categorias legadas da projeção com as categorias escolares fixas do sistema marcadas como `source`, `fixed` e `readonly`; no Médio, os anos vêm dos cursos médios ativos da academia, não de `academia.anos_academicos`, e o `4_ano_medio` só expõe `nota_pap` quando o curso médio é `modelo="tecnico"`. Para superior, lista as categorias configuráveis da academia.

**Proteção**: autenticado + (`academia` ativa **ou** `admin` **ou** `estudante`)

**Query params (quando `admin`)**:

- `codigo_academia` (obrigatório)


**Request:** sem payload
**Response 200:**

```json
{
  "categorias": [
    {
      "id": "uuid",
      "codigo_academia": "ACAD20251",
      "codigo": "prova_profesor",
      "nome": "Prova do professor",
      "descricao": "string",
      "anos_academicos": ["3_ano_fundamental"],
      "created_at": "2026-06-13T00:00:00Z",
      "version": 1
    },
    {
      "codigo_academia": "ACAD20251",
      "codigo": "nota_professor",
      "nome": "Nota do professor/Avaliação contínua",
      "anos_academicos": ["3_ano_fundamental"],
      "source": "system",
      "fixed": true,
      "readonly": true,
      "status": "ativo"
    }
  ],
  "total": 2
}
```

**Erros:**

- `404` — academia não encontrada (incluindo admin sem `codigo_academia`)

---

### DELETE /academia/categorias-nota/:codigo

Inativa (remove logicamente) uma categoria de nota adicional da academia **somente no ensino superior**. Escolas não podem remover categorias porque o catálogo escolar é fixo do sistema.

**Proteção**: autenticado + academia ativa

**Path Params:**

- `codigo` — código da categoria adicional a remover

**Request:** sem payload

**Response 200:**

```json
{
  "message": "categoria removida com sucesso",
  "categoria": "prova_profesor"
}
```

**Erros:**

- `400` — academia escolar tentando remover categoria, codigo ausente/vazio ou codigo com caracteres especiais inválidos no path
- `400` — categoria não existe nesta academia

---

### POST /academia/notas-aluno

Registra uma nota para um estudante.

**Proteção**: autenticado + academia ativa (com ano letivo configurado)

**Request:**

```json
{
  "codigo_estudante": "ABC1234",
  "periodo": "1_trimestre",
  "materia_disciplinar_id": "uuid",
  "tipo": "escolar",
  "categoria": "nota_professor",
  "nota": 8.5,
  "observacao": "string"  // opcional
}
```

**Restrições:**

- Academia `escola` só pode usar tipo `escolar`
- Academia `superior` só pode usar tipo `superior`
- `nota` deve estar dentro da escala do ano acadêmico: `0–10` para `1_ano_fundamental` a `6_ano_fundamental`; `0–20` para `7_ano_fundamental`, `8_ano_fundamental`, `9_ano_fundamental`, todos os anos médios e superior
- `periodo` deve ser válido para o tipo (`1_trimestre`/`2_trimestre`/`3_trimestre` para escolar; semestres do curso para superior)
- Para `tipo=superior`, o `periodo` precisa coincidir com o `periodo` definido na matéria (além de existir na lista de períodos do curso)
- Se o estudante tiver `ano_escolar_fundamental` ou `ano_escolar_medio`, esse ano deve existir em `anos_academicos` da matéria; caso contrário, o registro é bloqueado
- Para escolas, `categoria` deve estar no catálogo fixo do ano acadêmico inferido: `nota_professor`/`prova_trimestral` nos anos regulares; + `exame_final`/`exame_recurso` em `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`; apenas `nota_pap` no `4_ano_medio` técnico
- Para superior, `categoria` deve estar configurada em `POST /academia/categorias-nota` com `anos_academicos` contendo o ano/período acadêmico aplicável; sem anos definidos ou sem correspondência, nenhuma nota pode ser registrada nessa categoria
- O endpoint `POST /academia/notas-aluno/async` reaproveita exatamente as mesmas validações deste endpoint por item do lote
- Notas são imutáveis após criação: não há endpoint público ou assíncrono para editar ou eliminar notas

**Response 201:**

```json
{
  "message": "nota registrada com sucesso",
  "estudante": "ABC1234",
  "materia": "Matemática",
  "tipo": "escolar",
  "categoria": "nota_professor",
  "nota": 8.5,
  "ano_academico": "3_ano_fundamental",
  "periodo": "1_trimestre",
  "periodos_validos": ["1_trimestre", "2_trimestre", "3_trimestre"]
}
```

**Erros:**

- `400` — nota fora da escala do ano acadêmico, período inválido, categoria inválida/não configurada para o ano acadêmico, duplicata, ou incompatibilidade entre `ano_escolar_fundamental`/`ano_escolar_medio` do estudante e `anos_academicos` da matéria
- `403` — estudante ou matéria não pertencem à academia
- `400` — academia sem ano letivo configurado

---

### GET /notas-estudante/:codigo

Retorna as notas de um estudante.

**Proteção**: autenticado

**Nota de acesso:**

- Estudante: apenas o próprio código (`:codigo` deve ser o do estudante autenticado)
- Academia: apenas estudantes da própria academia
- Admin: qualquer estudante

**Query Params (opcionais):**

- `ano_letivo` — aceita múltiplos valores
- `ano_academico` — aceita múltiplos valores
- `curso_id` — aceita múltiplos valores
- `periodo` — filtra o período registado da nota (aceita múltiplos valores)
- `materia_disciplinar_id` — aceita múltiplos valores
- `categoria` — filtra por categoria da nota (aceita múltiplos valores)
- `codigo_academia` — aceita múltiplos valores

**Formato de múltiplos valores:**

- chave repetida: `?ano_letivo=2024_2025&ano_letivo=2025_2026`
- CSV na mesma chave: `?ano_letivo=2024_2025,2025_2026`
- também é possível combinar os dois formatos na mesma chamada


**Request:** sem payload
**Response 200:**

```json
{
  "codigo_estudante": "ABC1234",
  "nome": "string",
  "notas": [NotaDTO],
  "total": 12
}
```

---

### GET /notas

Lista registros de notas com escopo por perfil.

**Proteção**: autenticado (`admin` ou `academia`)

**Regras de escopo:**

- `admin`: lista todas as notas registradas no sistema
- `academia`: lista apenas notas com `codigo_academia` da academia autenticada

**Query Params:**

- `limit` — quantidade máxima por página (padrão: 50, teto fixo: 100)
- `offset` — deslocamento de paginação (padrão: 0)
- `ano_letivo` — filtra por ano letivo (aceita múltiplos valores)
- `ano_academico` — filtra por ano académico (aceita múltiplos valores)
- `curso_id` — filtra por curso (nível médio ou superior) (aceita múltiplos valores)
- `codigo_turma` — filtra por turma (requer `codigo_academia` em consultas admin) (aceita múltiplos valores)
- `periodo` — filtra por período (`1_trimestre`, `2_trimestre`, `3_trimestre`, `1_semestre`, `2_semestre`) (aceita múltiplos valores)
- `materia_disciplinar_id` — filtra por matéria disciplinar (aceita múltiplos valores)
- `categoria` — filtra por categoria da nota (aceita múltiplos valores)
- `codigo_academia` — filtro de academia (admin); para academia autenticada, este filtro é sempre forçado ao seu próprio código

**Formato de múltiplos valores (todos os filtros acima):**

- chave repetida: `?ano_letivo=2024_2025&ano_letivo=2025_2026`
- CSV na mesma chave: `?ano_letivo=2024_2025,2025_2026`
- também é possível combinar os dois formatos na mesma chamada


**Request:** sem payload
**Response 200:**

```json
{
  "notas": [NotaRegistroDTO],
  "total": 30,
  "total_geral": 5000,
  "limit": 50,
  "offset": 0
}
```

**Observação sobre paginação e tipo retornado:**

- `total`: quantidade de itens retornados no array `notas` nesta página.
- `total_geral`: quantidade total de registros no escopo do usuário (ignorando `limit/offset`).
- os itens em `notas` seguem `NotaRegistroDTO` (seção 2.10).
- o backend nunca retorna mais de 100 notas por página, mesmo que o cliente envie `limit` maior.

---

---

---

## 14. Sumários/Aulas

O recurso de sumários/aulas foi removido do contrato público da API. Não há endpoints para criar, listar, consultar, atualizar ou remover sumários, e faltas não aceitam nem retornam vínculo com sumário.

## 15. Faltas

### Processos de Negócio — Registro de Faltas

### 15.1 Registro de Faltas

**Quem faz**: Academia (status ativo, com ano letivo configurado)

**Processo:**

1. Academia envia: código do estudante, data, matéria, quantidade
2. Sistema valida ano letivo ativo
3. Sistema verifica pertencimento do estudante e da matéria à academia
4. Sistema infere o `ano_academico` (mesma lógica das notas)
5. Sistema verifica idempotência (chave: `data+codigo_estudante+materia_disciplinar_id`; no aggregate ela é resolvida no contexto de academia + ano letivo)

**Quantidade**: deve ser positiva (≥ 1)

**Data**: formato `AAAA-MM-DD` (date-only, sem componente de hora)

**Regra de registro**: faltas mantêm unicidade por combinação de `data + codigo_estudante + materia_disciplinar_id` (equivalente à unicidade técnica em projeção: estudante + academia + data + matéria).

**Quantidade por registro**: não possui teto máximo (apenas deve ser `>= 1`).

**Imutabilidade**: faltas são imutáveis após criação; não há endpoint público ou assíncrono para editar ou eliminar faltas.
### Regras de Negócio — Faltas

### 15.2 Regras de Faltas

| Regra                                            | Detalhe                                                                                 |
| ------------------------------------------------ | --------------------------------------------------------------------------------------- |
| Quantidade deve ser 1 ou mais                    | Validação no handler e no aggregate                                                     |
| Data no formato date (`AAAA-MM-DD`)              | Campo date-only em faltas (sem hora)                                                    |
| Ano do estudante deve pertencer à matéria        | Se `ano_escolar_fundamental` do estudante não existir em `anos_academicos` da matéria, bloqueia    |
| Data dentro do período letivo aplicável          | A matéria define o tipo letivo; faltas escolares usam a janela escolar fixa da academia e faltas superiores usam a janela superior fixa da academia |
| Duplicata bloqueada                              | Mesma combinação `data + codigo_estudante + materia_disciplinar_id` é rejeitada         |
| Sem vínculo de sumário                           | Faltas são independentes e não aceitam `sumario_id` ou `sumario_titulo` |

### 15.3 Remoção de Sumários/Aulas

O sistema não possui mais a entidade sumário/aula. As faltas devem ser lançadas e consultadas sem `sumario_id`, `sumario_titulo` ou qualquer vínculo equivalente.

### POST /academia/faltas-aluno

Registra falta(s) para um estudante.

**Proteção**: autenticado + academia ativa (com ano letivo configurado)

**Request:**

```json
{
  "codigo_estudante": "ABC1234",
  "data": "2025-03-15",              // formato AAAA-MM-DD
  "materia_disciplinar_id": "uuid",
  "quantidade": 2,                    // mínimo 1 (sem teto máximo)
  "observacao": "string"              // opcional
}
```

**Restrições:**

- `quantidade` deve ser maior ou igual a 1
- `data` é tratada como **date-only** (sem hora), em formato `AAAA-MM-DD`
- Se o estudante tiver `ano_escolar_fundamental` ou `ano_escolar_medio`, esse ano deve existir em `anos_academicos` da matéria; caso contrário, o registro é bloqueado
- Idempotência (duplicata bloqueada): combinação `data + codigo_estudante + materia_disciplinar_id`
- Payloads de falta não aceitam `sumario_id`, `sumario_titulo` ou campos equivalentes de sumário.
- A data da falta deve estar dentro do intervalo inclusivo do ano letivo aplicável: escolar para matérias escolares e superior para matérias superiores.
- O endpoint `POST /academia/faltas-aluno/async` reaproveita exatamente as mesmas validações deste endpoint por item do lote
- Faltas são imutáveis após criação: não há endpoint público ou assíncrono para editar ou eliminar faltas

**Response 201:**

```json
{
  "message": "faltas registradas com sucesso",
  "estudante": "ABC1234",
  "materia": "Matemática",
  "quantidade": 2,
  "ano_academico": "3_ano_fundamental"
}
```

**Erros:**

- `400` — quantidade inválida (deve ser ≥ 1), data inválida ou fora do período letivo aplicável, academia sem ano letivo configurado, ou incompatibilidade entre `ano_escolar_fundamental`/`ano_escolar_medio` do estudante e `anos_academicos` da matéria
- `403` — estudante ou matéria não pertencem à academia

---

### GET /faltas-estudante/:codigo

Retorna as faltas de um estudante.

**Proteção**: autenticado

**Nota de acesso:**

- Estudante: apenas o próprio código (`:codigo` deve ser o do estudante autenticado)
- Academia: apenas estudantes da própria academia
- Admin: qualquer estudante

**Query Params (opcionais):**

- `ano_letivo` — aceita múltiplos valores
- `ano_academico` — aceita múltiplos valores
- `curso_id` — aceita múltiplos valores
- `periodo` — filtra pelo período configurado na matéria (aceita múltiplos valores)
- `materia_disciplinar_id` — aceita múltiplos valores
- `codigo_academia` — aceita múltiplos valores

> Nesta rota, o filtro `codigo_turma` não é utilizado.

**Formato de múltiplos valores:**

- chave repetida: `?periodo=1_trimestre&periodo=2_trimestre`
- CSV na mesma chave: `?periodo=1_trimestre,2_trimestre`
- também é possível combinar os dois formatos na mesma chamada


**Request:** sem payload
**Response 200:**

```json
{
  "codigo_estudante": "ABC1234",
  "nome": "string",
  "faltas": [FaltaDTO],
  "total": 5
}
```

---

### GET /faltas

Lista registros de faltas com escopo por perfil.

**Proteção**: autenticado (`admin` ou `academia`)

**Regras de escopo:**

- `admin`: lista todas as faltas registradas no sistema
- `academia`: lista apenas faltas com `codigo_academia` da academia autenticada

**Query Params:**

- `limit` — quantidade máxima por página (padrão: 50, teto fixo: 100)
- `offset` — deslocamento de paginação (padrão: 0)
- `ano_letivo` — filtra por ano letivo (aceita múltiplos valores)
- `ano_academico` — filtra por ano académico (aceita múltiplos valores)
- `curso_id` — filtra por curso (nível médio ou superior) (aceita múltiplos valores)
- `codigo_turma` — filtra por turma (requer `codigo_academia` em consultas admin) (aceita múltiplos valores)
- `periodo` — filtra por período da matéria (`1_trimestre`, `2_trimestre`, `3_trimestre`, `1_semestre`, `2_semestre`) (aceita múltiplos valores)
- `materia_disciplinar_id` — filtra por matéria disciplinar (aceita múltiplos valores)
- `codigo_academia` — filtro de academia (admin); para academia autenticada, este filtro é sempre forçado ao seu próprio código

**Formato de múltiplos valores (todos os filtros acima):**

- chave repetida: `?periodo=1_trimestre&periodo=2_trimestre`
- CSV na mesma chave: `?periodo=1_trimestre,2_trimestre`
- também é possível combinar os dois formatos na mesma chamada


**Request:** sem payload
**Response 200:**

```json
{
  "faltas": [FaltaRegistroDTO],
  "total": 20,
  "total_geral": 3000,
  "limit": 50,
  "offset": 0
}
```

**Observação sobre paginação e tipo retornado:**

- `total`: quantidade de itens retornados no array `faltas` nesta página.
- `total_geral`: quantidade total de registros no escopo do usuário (ignorando `limit/offset`).
- os itens em `faltas` seguem `FaltaRegistroDTO` (seção 2.11).
- o backend nunca retorna mais de 100 faltas por página, mesmo que o cliente envie `limit` maior.

---

---

## 16. Avaliações Finais

### Processos de Negócio — Avaliação Final de Ano Académico

### 16.1 Avaliação Final de Ano Académico

**Quem faz**: Academia ativa, com ano letivo configurado, por meio da configuração de regras e do lançamento de notas. A academia **não envia manualmente** a nota final calculada nem decide aprovação/reprovação no payload de execução.

A avaliação final é automática, auditável e orientada por regras. Ela é disparada pelo fluxo de lançamento de notas quando o backend identifica que existem regras ativas e notas suficientes para calcular a etapa aplicável. O modelo atual **não é uma média global única do estudante**: o backend calcula uma `nota_final` independente para cada matéria disciplinar aplicável, registra resultados por matéria e deriva a decisão geral do conjunto de resultados, da cadeia de regras e, apenas para Superior, das regras de pendência.

#### 16.1.1 Conceitos funcionais

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

#### 16.1.2 Montagem e criação de regras de avaliação final

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
| `anos_academicos` | Obrigatório e não vazio; array simples de anos fundamentais | Obrigatório; lista de objetos `{curso_id, anos_academicos}` por curso médio | Rejeitado |
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

#### 16.1.3 Fórmulas por nível

A regra usa `formula` como texto declarativo. O parser valida referências, operadores, parênteses, constantes, categorias e períodos antes de persistir ou calcular. Erro de fórmula é erro de validação, não falha operacional inesperada.

| Nível | Formato da referência | Exemplo válido | Exemplo inválido |
|---|---|---|---|
| Fundamental | `[categoria,periodo]` | `([prova_trimestral,1_trimestre]+[prova_trimestral,2_trimestre]+[prova_trimestral,3_trimestre])/3` | `[prova_trimestral]` |
| Médio | `[categoria,periodo]` | `[prova_trimestral,1_trimestre]*0.4+[exame_final,3_trimestre]*0.6` | `[exame_final]` |
| Superior | `[categoria]`; período inferido pela matéria/semestre avaliado | `([prova_parcelar_1]+[prova_parcelar_2])/2` | `[prova_parcelar_1,1_semestre]` |

No Superior, o backend preenche o período no momento do cálculo usando a matéria/escopo avaliado (`periodo` da matéria e `semestre_atual` do estudante). Assim, a mesma regra superior pode calcular as matérias do semestre atual sem expor período explícito no payload da regra.

Quando a nota recém-lançada dispara a avaliação final para uma matéria, qualquer referência da fórmula dessa mesma matéria que ainda não tenha nota registrada é calculada como `0` naquele momento. A substituição por zero fica restrita à matéria que recebeu a nota-gatilho (`nota_despertadora` no Superior ou o gatilho escolar fixo equivalente) e é registrada no snapshot de `resultados_materias.notas_substituidas_zero`; matérias que ainda não receberam o próprio gatilho não são forçadas a avaliar. A fórmula sempre lê notas do ano letivo atual, da mesma academia, do mesmo estudante, da matéria avaliada e de categorias extraídas da própria fórmula.

#### 16.1.4 Execução automática por lançamento de notas

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

#### 16.1.5 Modelos escolares fixos de avaliação final por ano

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

#### 16.1.6 Fundamental

- O escopo é `ano_escolar_fundamental` atual do estudante (`1_ano_fundamental` a `9_ano_fundamental`).
- O catálogo avaliativo fundamental é fixo do sistema; rotas configuráveis de regras não aceitam criação/edição/remoção para Fundamental. Regras superiores não aceitam `anos_academicos`.
- O backend avalia cada matéria fundamental ativa aplicável ao ano do estudante, respeitando `materias_aplicaveis` se configurado.
- Cada matéria recebe `nota_final` própria; aprovação direta exige que todas as matérias avaliadas atinjam a mínima.
- Uma ou mais matérias abaixo da mínima reprovam a etapa e podem acionar regra descendente por matéria reprovada.
- Fundamental não permite aprovação com pendência: regra fundamental não tem `limite_materias_pendentes` e matérias fundamentais não aceitam `pendencia_permitida`/`pendencia_nivel_conclusao`.
- Aprovado em ano intermediário progride para o próximo ano fundamental. Se a academia não oferta o próximo ano, o evento registra o motivo `academia_sem_oferta_do_proximo_ano_academico_fundamental`, mantém o ciclo em andamento e não adiciona turma automaticamente.
- Aprovado no `9_ano_fundamental` finaliza o ciclo fundamental. Reprovado permanece no mesmo ano.

#### 16.1.7 Médio

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

#### 16.1.8 Superior

- O escopo é o curso superior ativo e o `semestre_atual` do estudante, convertido para `1_semestre`, `2_semestre`, etc.
- O backend avalia matérias superiores ativas do curso cujo `periodo` corresponde ao semestre atual.
- Fórmulas superiores não declaram período; o período é preenchido automaticamente para cada matéria avaliada.
- Aprovação direta exige todas as matérias avaliadas com nota final maior ou igual à mínima.
- Reprovação em matéria aciona descendentes aplicáveis; descendentes também trabalham por matéria e podem ser restringidas por `materias_aplicaveis`.
- Após esgotar a cadeia superior, o estudante pode aprovar com pendência se o total de reprovações couber em `limite_materias_pendentes` e todas as matérias reprovadas permitirem pendência.
- Reprovação por limite excedido ou matéria sem pendência permitida mantém o estudante no mesmo `semestre_atual` e não altera o status superior.
- Aprovação em semestre intermediário incrementa `semestre_atual` e recalcula `ano_superior`; aprovação no último semestre finaliza o ciclo superior.
- Pendência de curso anterior permanece histórica e não bloqueia o curso atual.

#### 16.1.9 Regras descendentes por matéria

Regra descendente é qualquer regra com `aplica_se_reprovado_em_type`. Ela representa uma etapa posterior da cadeia e só roda quando a etapa ascendente indicada reprovou. A descendente herda a lógica por matéria: calcula notas para matérias aplicáveis, compara cada resultado com a mínima e grava `type`/regra/fórmula usados naquela etapa.

Pontos importantes:

- A descendente não é uma média global do estudante; ela recalcula matérias no escopo da regra.
- `materias_aplicaveis` funciona como filtro: matéria fora da lista não é recalculada naquela etapa.
- A cadeia termina quando não há descendente ativa aplicável, quando a etapa anterior aprovou ou quando faltam notas para calcular a próxima etapa.
- Ao final da última etapa reprovada, apenas o Superior avalia se a reprovação vira pendência; Fundamental e Médio escolar permanecem reprovados quando não atendem ao padrão fixo.
- Exemplo: raiz `avaliacao_final` reprova Matemática e Física; descendente `avaliacao_final_com_exame` com `materias_aplicaveis=[Matemática]` recalcula somente Matemática. Física continua com o resultado anterior para a decisão final/pendência.

#### 16.1.10 Resultados por matéria, eventos, projeções e auditoria

Cada avaliação final gravada deve ser explicada pelos itens de `resultados_materias`, não por média global única. Cada item contém, no mínimo, `materia_id`, `nota_final`, `aprovado`, `type`, `formula_snapshot`, `regra_avaliacao_final_id`, `pendencia_permitida` e, quando aplicável, `notas_substituidas_zero` com as referências calculadas como zero por ausência de lançamento no momento do gatilho. A projeção também mantém `nota_final` agregada como média dos itens calculados para compatibilidade/consulta resumida, mas a decisão funcional é por matéria.

Eventos `AvaliacaoFinalEscolar` e `AvaliacaoFinalSuperior` preservam snapshots de regra, fórmula, notas calculadas, progressão e pendências geradas. Alterações posteriores de regra, matéria ou nota não reescrevem silenciosamente decisões já registradas; ajustes exigem fluxo operacional próprio/rebuild controlado.

#### 16.1.11 Pendências de matérias

Pendências existem apenas para o Superior. Elas são consideradas depois de reprovação na cadeia aplicável e só são criadas quando a decisão final superior é aprovação com pendência. Se o estudante reprova totalmente, nenhuma nova pendência é criada.

A pendência carrega funcionalmente: estudante, matéria, academia, curso, `nivel`, ano letivo, escopo acadêmico (`periodo_superior`), regra/evento de origem, status `pendente`, dados de origem/snapshot e timestamps. Há proteção contra duplicidade aberta no mesmo estudante, matéria, curso, nível, ano letivo e escopo. A estrutura também possui campos de baixa (`baixada_por_event_id`, `updated_at`) para histórico, mas a documentação funcional reconhece uma limitação atual: **não há rota pública consolidada de regularização/baixa de pendência exposta nesta documentação de API**. Portanto, o sistema já persiste e consulta a base de pendências abertas/históricas, mas a regularização operacional precisa ser implementada ou conduzida por fluxo administrativo/evento específico antes de ser tratada como rotina pública.

#### 16.1.12 Bloqueio por `pendencia_nivel_conclusao` e regularização

`pendencia_nivel_conclusao` pertence à matéria e deve ser usado para identificar pendências bloqueantes do curso atual. Funcionalmente:

- No Superior, pendência aberta cujo limite coincide com semestre/período conclusivo bloqueia conclusão automática até baixa.
- Aprovação com pendência pode permitir progressão intermediária, mas não deve permitir conclusão com pendência bloqueante do curso atual.
- Pendências não bloqueantes permitem progressão conforme regra de avaliação, desde que pertençam a escopo anterior e dentro do limite funcional definido.
- Pendências de curso anterior são históricas e não bloqueiam o curso atual.
- Regularização de pendência é diferente de avaliação final normal: deve avaliar a matéria pendente, registrar evento próprio auditável, baixar a pendência se aprovada e manter aberta se reprovada. Como limitação atual, esse fluxo ainda não está exposto como endpoint público completo; ao ser implementado, deve reutilizar os dados de origem da pendência e retomar progressão/conclusão quando não restarem pendências relevantes abertas.

#### 16.1.13 Cenários de erro e validação

Devem falhar com erro de validação ou bloqueio funcional:

- Payload de regra com `tipo_ensino`; use `nivel`.
- Academia mista criando regra sem `nivel` ou tentando criar regra `superior`.
- Academia não mista criando regra de nível incompatível com sua configuração.
- `anos_academicos` ausente em regra fundamental ou presente em Médio/Superior.
- `limite_materias_pendentes` enviado em regra escolar, ausente em regra superior ou negativo.
- `materias_aplicaveis` fora do escopo do curso/ano/período aplicável deve ser tratada como configuração inválida ou ineficaz operacionalmente; QA deve validar esse cenário contra a base de matérias da academia.
- Descendente órfã, descendente que aponta para si mesma, ciclo de dependências ou escopo fundamental diferente da raiz.
- Fórmula Fundamental/Médio sem período explícito (`[categoria]`).
- Fórmula Superior com período explícito (`[categoria,periodo]`).
- Fórmula com categoria inexistente, período inválido, divisão por zero, caracteres fora da gramática ou categorias enviadas que não batem com a fórmula.
- Tentativa de criar pendência em matéria fundamental.
- Tentativa de criar duplicidade de pendência aberta no mesmo escopo.
- Tentativa de concluir/progredir em desacordo com pendência bloqueante do curso atual.

#### 16.1.14 Consultas

- `GET /avaliacoes` → registros de avaliação final, com filtros por `nivel`, ano letivo, ano/período acadêmico atual, turma, academia e `type`. O filtro legado `tipo_ensino` é rejeitado no handler atual.
- `GET /aprovacoes` → apenas aprovados (`aprovado = TRUE`) com os mesmos filtros.
- `GET /reprovacoes` → reprovações definitivas; reprovações intermediárias com descendente ativa posterior não aparecem como definitivas até a cadeia terminar.
- `GET /academia/avaliacao-final/regras` → lista regras da academia autenticada.
- `PUT /academia/avaliacao-final/regras/:id` → edita apenas campos seguros de apresentação/cálculo (`nome`, `descricao`, `nota_minima_aprovacao`, `formula`).
- `DELETE /academia/avaliacao-final/regras/:id` → inativa a regra e suas dependentes em cascata.

**Escopo por academia:** usuário autenticado como academia só consulta/gerencia dados da própria academia. Admin pode consultar de forma ampla com filtros.
### Regras de Negócio — Avaliação Final


#### 16.1.15 Decisão futura para correção de notas após avaliação final

Na versão atual do sistema, notas permanecem imutáveis: elas podem ser criadas e consultadas, mas não há endpoint público, administrativo, batch ou assíncrono para editar, eliminar, restaurar ou substituir notas já registradas. Portanto, esta seção é uma decisão de produto para uma funcionalidade futura de correção de notas; ela não descreve um comportamento já implementado.

Quando uma funcionalidade de correção/edição de notas for especificada no futuro, ela deve verificar se o ano letivo ativo da academia ainda é o mesmo ano letivo da nota corrigida e da avaliação final já registrada para aquele estudante, matéria e escopo. Se for o mesmo ano letivo, a avaliação daquela matéria deve ser recalculada com a nota corrigida, e o resultado deve ser persistido como um novo evento auditável de reavaliação, distinto do evento original de avaliação final. O evento original deve permanecer preservado para auditoria. Se o ano letivo ativo já tiver avançado, a reavaliação automática não deve ocorrer; esse cenário deve ser definido junto com a própria funcionalidade de edição de notas. Qualquer implementação futura de edição/correção de notas deve reutilizar esta regra em vez de introduzir comportamento divergente sem revisão de produto.

### 16.2 Regras de Avaliação Final

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
- Se alguma nota exigida pela fórmula ainda estiver ausente, aquela regra é ignorada naquele momento e poderá ser calculada quando novas notas forem registradas.
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
  "limit": 50,
  "offset": 0
}
```

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
  "limit": 50,
  "offset": 0
}
```

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
  "limit": 50,
  "offset": 0
}
```

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
  "limit": 50,
  "offset": 0
}
```

---

---

## 17. Admins

### Processos de Negócio — Administração e Integridade

### 17.1 Verificação de Integridade do Ledger

O sistema suporta verificação da cadeia de hashes do ledger para qualquer estudante:

```
GET /verificar-integridade/:codigo
```

A função SQL `verify_hash_chain` verifica se todos os hashes encadeados são válidos. Se qualquer evento foi adulterado, a verificação retorna `integro = false` indicando a versão onde a cadeia foi quebrada.

---

### 17.2 Rebuild de Projeções

Admins com role `fpp` podem reconstruir projeções:

```
POST /dominis/projections/rebuild/:name
```

Para evitar timeout em rebuilds longos (ex.: projeções com alto volume de eventos no ledger), use a versão assíncrona:

```
POST /dominis/projections/rebuild/:name/async
```

Esse endpoint retorna `202 Accepted` com `job_id`, `poll_url` e `sse_url`; o cliente pode acompanhar em `GET /jobs/:id` e/ou receber eventos em `GET /jobs/stream`.

**Concorrência de rebuild**: o manager permite apenas **1 rebuild por vez** (lock global). Se outro rebuild já estiver em execução, o endpoint síncrono retorna `409 Conflict`.

**Antes de reconstruir**, o sistema verifica a integridade completa do ledger. Se qualquer aggregate estiver com hash chain inválida, o rebuild é abortado.

**Ordem de rebuild recomendada** (respeita dependências):

1. `admins`
2. `academias`
3. `cursos`, `materias`, `categorias_nota`
4. `estudantes`, `turmas`
5. `notas`, `faltas`
6. `avaliacao_final`
### Regras de Negócio — Admin

### 17.3 Regras de Admin

| Regra                                    | Detalhe                                                                                            |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Hierarquia estrita                       | Só pode gerenciar roles estritamente inferiores                                                    |
| Email verificado obrigatório para operar | Sem verificação, acesso ao painel é bloqueado                                                      |
| Apenas FPP altera roles                  | Regra de negócio deliberada                                                                        |
| Não pode desativar a si próprio          | Prevenção de bloqueio acidental                                                                    |
| Bootstrap único                          | Primeiro FPP criado via endpoint especial com advisory lock                                        |
| Senha gerada automaticamente             | Senha segura gerada com `crypto/rand` e enviada por email (apenas depois do e-mail ser verificado) |


### POST /dominis/register

Cria um novo admin. A senha é gerada automaticamente e enviada por email.

**Proteção**: autenticado + admin (qualquer role)

**Regra de hierarquia**: o admin criador deve ter role estritamente superior ao do novo admin.

**Request:**

```json
{
  "nome": "string",
  "email": "string",
  "role": "gerente"  // 'fpp' | 'adm' | 'gerente'
}
```

**Response 201:**

```json
{
  "message": "administrador criado com sucesso. A senha temporária foi enviada por email.",
  "data": {
    "id": "uuid",
    "nome": "string",
    "email": "string",
    "role": "gerente"
  }
}
```

---

### GET /dominis/admin-lista

Lista todos os admins.

**Proteção**: autenticado + admin (qualquer role)


**Request:** sem payload
**Response 200:**

```json
{
  "admins": [AdminDTO],
  "total": 5
}
```

---

### GET /dominis/consultar-admin/:email

Busca um admin pelo email.

**Proteção**: autenticado + admin role `adm` ou `fpp`


**Request:** sem payload
**Response 200:**

```json
{
  "admin": AdminDTO
}
```

---

### PUT /dominis/admin/:id/ativar

Ativa um admin inativo.

**Proteção**: autenticado + admin role `adm` ou `fpp`


**Request:** sem payload
**Response 200:**

```json
{
  "message": "administrador ativado com sucesso",
  "email": "string"
}
```

---

### PUT /dominis/admin/:id/desativar

Desativa um admin ativo.

**Proteção**: autenticado + admin role `adm` ou `fpp`

**Request:**

```json
{
  "motivo": "string"  // obrigatório
}
```

**Response 200:**

```json
{
  "message": "administrador desativado com sucesso",
  "email": "string"
}
```

---

### PUT /dominis/admin/:id/role

Altera o role de um admin. Apenas FPP pode fazer isso.

**Proteção**: autenticado + admin role `fpp`

**Request:**

```json
{
  "novo_role": "adm"  // 'fpp' | 'adm' | 'gerente'
}
```

**Response 200:**

```json
{
  "message": "role atualizado com sucesso",
  "role_anterior": "gerente",
  "novo_role": "adm"
}
```

---

### PUT /dominis/admin/:id/dados

Atualiza nome e/ou email de um admin.

**Proteção**: autenticado + admin (qualquer role)

**Request:** (pelo menos um campo obrigatório)

```json
{
  "nome": "string",
  "email": "string"
}
```

**Response 200:**

```json
{
  "message": "dados atualizados com sucesso"
}
```

---

### GET /dominis/metrics

Retorna métricas do sistema (requisições, erros, latência por endpoint).

**Proteção**: autenticado + admin (qualquer role)


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

---

### POST /dominis/projections/rebuild/:name

Reconstrói uma projeção do zero a partir do ledger.

**Proteção**: autenticado + admin role `fpp`

**Path Params:**

- `name` — nome da projeção (ex: `estudantes`, `academias`, `notas`)


**Request:** sem payload
**Response 200:**

```json
{
  "message": "projeção reconstruída com sucesso",
  "projection": "estudantes"
}
```

**Erros:**

- `409` — já existe outro rebuild em andamento
- `404` — projeção não encontrada
- `500` — integridade do ledger comprometida (rebuild abortado), com motivo detalhado no campo `message`

---

### POST /dominis/projections/rebuild/:name/async

Enfileira o rebuild de uma projeção para execução em background (job assíncrono).

Use este endpoint quando o rebuild puder demorar vários minutos.

**Proteção**: autenticado + admin role `fpp`

**Path Params:**

- `name` — nome da projeção (ex: `admins`, `estudantes`, `notas`)


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

---

## 18. Jobs Assíncronos

### Processos de Negócio — Sistema de Jobs Assíncronos

### 18.1 Sistema de Jobs Assíncronos

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

## 19. Batch Assíncrono

### Processos de Negócio — Operações em Lote

### 19.1 Batch Assíncrono

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

> Exceção: `POST /academia/estudante/register/async` usa o contrato específico de cadastro em massa com `com_arquivo` descrito na seção da rota, retorna resposta de lote e não cria job de background.

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
|`POST /academia/estudante/register/async`|`{com_arquivo:false, estudantes:[...]}` ou `multipart/form-data` com `com_arquivo=true`|resposta de lote (`200`/`207`)|100|
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

Os documentos de matrícula continuam sendo gravados em `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/`; documentos formais seguem `{codigo_academia}/Documentação formal/`; documentos de estudantes seguem `{codigo_academia}/Estudantes/{codigo_estudante}/`. `EnsureDir` cria a hierarquia de pastas de forma idempotente, `Upload` envia o conteúdo para o caminho lógico solicitado e retorna metadados internos do projeto (`path`, `file_url`, `download_url`). Nas respostas de consulta, o backend normaliza `download_url` para uma rota autenticada própria do escopo consultado, mesmo quando o metadado persistido contém link legado do storage. O front end deve baixar documentos pelas rotas autenticadas de download do backend (`/documentos/academias/{codigo_academia}/alvara/download`, `/documentos/estudantes/{codigo_estudante}/{campo}/download`, `/documentos/solicitacoes-matricula/{codigo_solicitacao}/{campo}/download`, `/estudante/documentos/{campo}/download` ou `/academia/documentos/...`), e não por credenciais, links privados ou IDs internos do Mega. `Read` faz o download para arquivo temporário e entrega um stream fechado pelo handler; `Delete`, `Move` e `Rename` normalizam paths e erros externos. `GetQuota` é suportado no provider local; no Mega real, limitações do MEGAcmd para quota detalhada por diretório são expostas como operação não suportada em vez de simular sucesso.

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

Faz stream inline de um documento persistido no mapa `documentos` da projeção do estudante. Para documentos acadêmicos normalizados, o backend procura primeiro pela chave exata `nivel.ano_academico.tipo` e também aceita localizar por `campo`/`tipo` quando os query params `nivel` e `ano_academico` forem enviados. Exemplos de chaves atuais: `bi_estudante`, `bi_responsavel`, `cedula_estudante`, `medio.3_ano_medio.declaracao_3_ano_medio`, `fundamental.9_ano_fundamental.certificado_9_ano_fundamental` ou `medio.3_ano_medio.certificado_ensino_medio`.

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
