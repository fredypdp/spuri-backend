---
modificado: 28-06-2026 17:10
criado: 05-04-2026 13:01
---
Versão atual: 2.0.7
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
14. [[#14. Faltas]]
15. [[#15. Avaliações Finais]]
16. [[#16. Admins]]
17. [[#17. Jobs Assíncronos]]
18. [[#18. Batch Assíncrono]]
19. [[#19. Armazenamento]]

---

### Telefones nativos

O conceito de telefone extra foi removido. Estudantes, academias e admins possuem campos nativos de telefone. Todos os telefones são normalizados removendo espaços, hifens e parênteses, e devem ser salvos como string local de exatamente 9 dígitos, sem DDI.

A verificação de telefone ainda não está implementada: `telefone_verificado` e `telefone_responsavel_verificado` existem apenas para compatibilidade futura e nenhum endpoint de verificação deve ser consumido ou documentado. Um número já verificado por outro usuário não poderá ser reaproveitado; números não verificados podem coincidir entre entidades, exceto nas regras específicas de estudante.

Para estudantes, pelo menos um entre `telefone` e `telefone_responsavel` deve ser informado. Os dois campos não podem ser iguais, e o `telefone` de um estudante não pode ser usado como `telefone_responsavel` de outro estudante. Para estudantes de ensino superior, `telefone_responsavel` é opcional desde que `telefone` esteja preenchido.

---

## 1. Convenções Globais

### Autenticação

Todas as rotas protegidas exigem o header:

```
Authorization: Bearer <jwt_token>
```

### Content-Type

```
Content-Type: application/json
```

### Datas

- Datas completas em UTC no formato RFC3339: `2025-03-15T10:30:00Z`
- Datas simples: `2025-03-15` (formato ISO 8601)

### Convenção de Payload e Resposta

- Toda rota desta documentação declara explicitamente o formato de entrada e saída.
- Quando uma rota não tiver corpo de entrada, ela trará `Request: sem payload` ou será um `GET` sem body.
- Nos endpoints batch (`/async`), o payload é sempre um array JSON; cada item segue o contrato da rota síncrona equivalente.

### Regras de Negócio por Rota

- Cada rota documenta explicitamente: permissões, pré-condições, validações de domínio e bloqueios de negócio.
- As regras de negócio vêm da documentação do sistema (`Spuri - Documentação.md`) e devem ser consideradas fonte principal.
- Em caso de diferença entre exemplo de payload e regra de negócio, prevalece a regra de negócio.

### Envelope de Erro

Todas as respostas de erro seguem o formato:

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

> `details` é opcional e normalmente aparece em `400` quando a validação de payload
> falha no bind/validator. Campos sem erro podem omitir essa chave.

### Códigos HTTP

|Código|Situação|
|---|---|
|`200`|Sucesso|
|`201`|Criado com sucesso|
|`202`|Aceito (job assíncrono criado)|
|`204`|Sem conteúdo|
|`400`|Erro de validação de entrada|
|`401`|Token ausente, inválido ou expirado|
|`403`|Sem permissão ou entidade inativa|
|`404`|Recurso não encontrado|
|`409`|Conflito (duplicidade ou estado incompatível)|
|`422`|Lote inteiramente inválido|
|`500`|Erro interno do servidor|

---

## 2. Estruturas de Dados

### 2.1 Tipos Base

```typescript
type UserType   = 'academia' | 'estudante' | 'admin'
type AdminRole  = 'fpp' | 'adm' | 'gerente'
type AcademiaNivel = 'escola' | 'superior'
type AcademiaType = 'public' | 'private'
type NivelEscolar = 'fundamental' | 'medio' | 'misto'
type StatusGeralEstudante = 'inativo' | 'ativo' | 'arquivado'
type StatusEscolar = 'inativo' | 'em_andamento' | 'finalizado'
type TipoEnsino = 'fundamental' | 'medio' | 'superior'
type Turno = 'manha' | 'tarde' | 'noite'
type CursoType = 'medio' | 'superior'
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

**Categorias de nota fixas:**

- Escolar: `nota_escola`, `nota_professor`
- Superior: `nota_pp1`, `nota_pp2`, `nota_exame`
- Adicionais: qualquer nome cadastrado pela academia

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
  updated_at?: string      // RFC3339
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
  created_at: string
  updated_at: string
  version: number
}
```

---


### 2.x SolicitacaoMatricula

```typescript
interface SolicitacaoMatriculaDocumentoDTO {
  path: string
  file_url: string
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

### 2.5 Curso

```typescript
interface CursoDTO {
  id: string
  nome: string
  type: CursoType            // preenchido automaticamente pelo backend e imutável
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

### 2.6 Matéria

```typescript
interface MateriaDTO {
  id: string
  nome: string
  type: MateriaType          // preenchido automaticamente (exceto escola mista, que informa no create)
  anos_academicos?: string[]  // ex: ['2_ano_fundamental'] ou ['1_ano_medio']
  periodo?: string            // ex: '1_semestre' — obrigatório para superior
  codigo_academia: string
  curso_id?: string           // UUID — obrigatório para medio e superior
  status: string              // 'ativo' | 'inativo' | 'deletado'
  created_at: string
  updated_at: string
  version: number
}
```

---

### 2.7 Turma

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

### 2.8 Nota

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
  categoria: string           // ex: 'nota_escola', 'nota_exame'
  nota: number                // >= 0
  observacao?: string
  registered_at: string
  event_id: string
  version: number
}
```

---

### 2.9 Falta

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

### 2.10 Registro de Nota (consulta global)

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

### 2.11 Registro de Falta (consulta global)

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

### 2.12 Avaliação Final

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

### 2.13 Categoria de Nota

```typescript
interface CategoriaNotaDTO {
  id: string
  codigo_academia: string
  codigo: string
  nome: string
  descricao?: string
  adicionado_por?: string  // UUID
  status: string           // 'ativo' | 'inativo'
  created_at: string
  version: number
}
```

---

### 2.14 Job

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

### 2.15 Resposta de Criação de Job Batch Assíncrono

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
    "curso_superior": null
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

### POST /dominis/academia/register

Registra uma nova academia. Criada com status `inativo`.

**Proteção**: autenticado + admin (qualquer role)

**Request — Escola:**

```json
{
  "nivel": "escola",
  "type": "public",
  "nome": "Escola Primária Ngola Kiluanje",
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
    "provincia": "LDA",
    "codigo_academia": "LDA20261"
  }
}
```

**Erros:**

- `400` — `nivel` inválido, `type` inválido (`public`/`private`) ou ausente, campos obrigatórios ausentes, anos_academicos inválidos
- `409` — academia já existe

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

**Nota**: se o email for alterado, `email_verificado` volta para `false`.

---

### Regras automáticas de documentos de matrícula

A obrigatoriedade dos documentos não é mais configurada por academia. O backend aplica automaticamente as mesmas regras abaixo no `POST /solicitacao-matricula` e no cadastro direto `POST /academia/estudante/register`:

- Para estudantes escolares/fundamental/médio, `bi_responsavel` e `bilhete_identidade_responsavel` são obrigatórios em todos os fluxos de criação e aprovação.
- `bilhete_identidade` e `bilhete_identidade_responsavel`, quando ambos informados para o mesmo estudante, não podem ser iguais.
- Para estudantes escolares/fundamental/médio, `bi_estudante` é obrigatório quando `bilhete_identidade` for informado; `cedula_estudante` é obrigatória quando o estudante não tiver BI próprio.
- Para estudantes escolares/fundamental/médio, o BI do responsável não pode coincidir com o BI principal de outro estudante escolar/fundamental/médio; ele pode repetir como BI de responsável de outros estudantes.
- `certificado_6_ano_fundamental` é o certificado aplicável somente para `7_ano_fundamental`, `8_ano_fundamental` e `9_ano_fundamental`.
- `certificado_9_ano_fundamental` é o certificado aplicável somente para anos do ensino médio.
- `certificado_ensino_medio` é o certificado aplicável somente para anos do ensino superior.
- `declaracao` é obrigatória quando o certificado aplicável não for enviado ou quando não existir certificado aplicável ao ano académico informado.


---

### GET /academias

Lista todas as academias com paginação e filtro de status.

**Proteção**: pública com autenticação opcional.

- Sem `Authorization`, a rota retorna apenas dados públicos de cada academia.
- Com `Authorization: Bearer <jwt_token>` válido, a rota mantém o contrato autenticado anterior.
- Se um header `Authorization` for enviado, ele deve ser um Bearer token válido; tokens inválidos/expirados retornam `401`.

**Query Params:**

- `limit` — quantidade máxima (padrão sem limit: 1000, teto: 1000)
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
  "limit": 1000,
  "offset": 0
}
```

**Campos públicos por academia:** `nivel`, `type`, `nome`, `codigo_academia`, `provincia`, `endereco`, `nivel_escolar`, `anos_academicos`. Para escolas fundamentais ou mistas, `anos_academicos` permite que usuários sem sessão recebam os anos acadêmicos ofertados.

**Response 200 — usuário autenticado:**

```json
{
  "academias": [AcademiaDTO],
  "total": 25,
  "limit": 1000,
  "offset": 0
}
```

**Nota**: usuários autenticados veem os campos operacionais do `AcademiaDTO`; admins veem campos extras (`email`, `total_estudantes`, `version`).

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
  "anos_letivos_lista": ["2026"]
}
```

**Campos públicos para usuário não autenticado:** `nivel`, `type`, `nome`, `codigo_academia`, `provincia`, `endereco`, `nivel_escolar`, `anos_academicos`.

**Nota**: admins veem também `email` e `motivo_desativacao`.

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
| `400` | Admin não enviou `codigo_academia`. | `{ "error": "VALIDATION_ERROR", "message": "codigo_academia é obrigatório para admin" }` |
| `401` | Token ausente ou inválido. | `{ "error": "UNAUTHORIZED", "message": "..." }` |
| `403` | Usuário não é academia nem admin autorizado, ou academia está inativa. | `{ "error": "FORBIDDEN", "message": "..." }` |
| `404` | Academia do token ou `codigo_academia` não encontrada. | `{ "error": "NOT_FOUND", "message": "academia não encontrado" }` |

### POST /academia/anos-academicos

Adiciona/habilita novos escopos acadêmicos sem remover os escopos existentes. Use esta rota para expandir a oferta da academia ou do curso.

**Proteção**: autenticado + academia ativa. Admins não escrevem por esta rota.

**Funcionamento por `type`:**

| `type` | Onde altera | Campos aceitos | Campos obrigatórios | Resultado |
| --- | --- | --- | --- | --- |
| `fundamental` | Academia autenticada (`projection_academias.anos_academicos`) | `type`, `anos_academicos` | `type`, `anos_academicos` | Une os anos enviados com os anos fundamentais já ativos. |
| `medio` | Curso médio da academia (`projection_cursos.anos_academicos`) | `type`, `curso_id`, `anos_academicos` | `type`, `curso_id`, `anos_academicos` | Une os anos enviados com os anos médios já ativos no curso. |
| `superior` | Curso superior da academia (`projection_cursos.periodos` e anos derivados) | `type`, `curso_id`, `periodos` | `type`, `curso_id`, `periodos` | Define a quantidade total de semestres informada e deriva os anos superiores por `ceil(periodos/2)`. |

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

**Request — médio:**

```json
{
  "type": "medio",
  "curso_id": "uuid-do-curso-medio",
  "anos_academicos": ["4_ano_medio"]
}
```

**Response 200 — médio:**

```json
{
  "message": "anos acadêmicos atualizados com sucesso",
  "type": "medio",
  "curso_id": "uuid-do-curso-medio",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"]
}
```

**Request — superior:**

```json
{
  "type": "superior",
  "curso_id": "uuid-do-curso-superior",
  "periodos": 8
}
```

**Response 200 — superior:**

```json
{
  "message": "anos acadêmicos atualizados com sucesso",
  "type": "superior",
  "curso_id": "uuid-do-curso-superior",
  "anos_academicos": ["1_ano_superior", "2_ano_superior", "3_ano_superior", "4_ano_superior"],
  "periodos": ["1_semestre", "2_semestre", "3_semestre", "4_semestre", "5_semestre", "6_semestre", "7_semestre", "8_semestre"]
}
```

### PATCH /academia/anos-academicos

Substitui completamente o conjunto habilitado do escopo informado. Use esta rota quando a lista final desejada já é conhecida.

**Proteção**: autenticado + academia ativa. Admins não escrevem por esta rota.

**Funcionamento:**

- Para `fundamental`, `anos_academicos` substitui a lista de anos fundamentais da academia.
- Para `medio`, `anos_academicos` substitui a lista de anos acadêmicos do curso médio informado em `curso_id`.
- Para `superior`, `periodos` substitui a quantidade total de semestres do curso superior; o backend recalcula `periodos` (`1_semestre` até `n_semestre`) e `anos_academicos` superiores automaticamente.
- Qualquer ano/semestre que exista hoje e deixe de existir após a substituição é tratado como redução e passa pelas validações de estudantes ativos.

**Request — fundamental/misto:**

```json
{
  "type": "fundamental",
  "anos_academicos": ["1_ano_fundamental", "2_ano_fundamental", "3_ano_fundamental"]
}
```

**Request — médio:**

```json
{
  "type": "medio",
  "curso_id": "uuid-do-curso-medio",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"]
}
```

**Request — superior:**

```json
{
  "type": "superior",
  "curso_id": "uuid-do-curso-superior",
  "periodos": 8
}
```

**Response 200 — fundamental/médio:**

```json
{
  "message": "anos acadêmicos atualizados com sucesso",
  "type": "medio",
  "curso_id": "uuid-do-curso-medio",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"]
}
```

**Response 200 — superior:**

```json
{
  "message": "anos acadêmicos atualizados com sucesso",
  "type": "superior",
  "curso_id": "uuid-do-curso-superior",
  "anos_academicos": ["1_ano_superior", "2_ano_superior", "3_ano_superior", "4_ano_superior"],
  "periodos": ["1_semestre", "2_semestre", "3_semestre", "4_semestre", "5_semestre", "6_semestre", "7_semestre", "8_semestre"]
}
```

### DELETE /academia/anos-academicos

Desabilita/remover logicamente escopos acadêmicos da oferta futura, preservando histórico. Use esta rota para reduzir a oferta sem apagar dados já registrados.

**Proteção**: autenticado + academia ativa. Admins não escrevem por esta rota.

**Funcionamento por `type`:**

- `fundamental`: remove do cadastro da academia somente os anos enviados em `anos_academicos`.
- `medio`: remove do curso médio informado somente os anos enviados em `anos_academicos`.
- `superior`: não recebe uma lista de semestres a remover; recebe `periodos` com a nova quantidade total de semestres que deve permanecer ativa no curso. Exemplo: se o curso tem 8 semestres e o payload envia `periodos: 6`, os semestres `7_semestre` e `8_semestre` deixam de estar disponíveis para novos vínculos.
- A remoção é lógica/prospectiva: o backend não apaga eventos, ledger, estudantes, turmas, matérias, notas, faltas, avaliações finais ou sumários já registrados.

**Request — fundamental/misto:**

```json
{
  "type": "fundamental",
  "anos_academicos": ["4_ano_fundamental"]
}
```

**Request — médio:**

```json
{
  "type": "medio",
  "curso_id": "uuid-do-curso-medio",
  "anos_academicos": ["4_ano_medio"]
}
```

**Request — superior:**

```json
{
  "type": "superior",
  "curso_id": "uuid-do-curso-superior",
  "periodos": 6
}
```

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

### Validações e erros de `POST`, `PATCH` e `DELETE /academia/anos-academicos`

**Validações comuns:**

- `type` é obrigatório e deve ser `fundamental`, `medio` ou `superior`.
- A academia só altera o próprio escopo.
- `curso_id` é obrigatório para `medio` e `superior` e precisa pertencer à academia autenticada.
- O `type` do payload precisa corresponder ao `type` do curso informado.
- `fundamental` só é permitido para academias escolares com `nivel_escolar` igual a `fundamental` ou `misto`.
- `fundamental` aceita somente códigos canônicos `[1-9]_ano_fundamental`.
- `medio` aceita somente anos médios compatíveis com o curso.
- `superior` aceita somente `periodos` numérico; `anos_academicos` superiores enviados pelo cliente não são usados como fonte manual.
- Academias fundamental/misto devem manter ao menos um ano acadêmico ativo após a operação.
- Reduções em `PATCH` e `DELETE` são bloqueadas com `409 Conflict` quando existem estudantes ativos no ano/semestre removido (`status_escolar_fundamental`, `status_escolar_medio` ou `status_superior` em andamento conforme o escopo operacional).

**Erros esperados:**

| Status | Quando ocorre | Response |
| --- | --- | --- |
| `400` | Payload JSON inválido. | `{ "error": "VALIDATION_ERROR", "message": "payload inválido" }` |
| `400` | `type` ausente ou diferente de `fundamental`, `medio` e `superior`. | `{ "error": "VALIDATION_ERROR", "message": "type deve ser fundamental, medio ou superior" }` |
| `400` | `curso_id` ausente para médio/superior. | `{ "error": "VALIDATION_ERROR", "message": "curso_id é obrigatório para type medio" }` |
| `400` | Curso inexistente, de outra academia ou com `type` incompatível. | `{ "error": "VALIDATION_ERROR", "message": "curso não encontrado" }` |
| `400` | Academia não pode gerenciar o fundamental, ou lista final ficaria vazia. | `{ "error": "VALIDATION_ERROR", "message": "academia não pode gerenciar anos do fundamental" }` |
| `409` | Redução afetaria estudantes ativos. | `{ "error": "CONFLICT", "message": "não é possível desativar anos_academicos [...]: existem N estudante(s) ativo(s) vinculados" }` |
| `401` | Token ausente ou inválido. | `{ "error": "UNAUTHORIZED", "message": "..." }` |
| `403` | Usuário não é academia ativa. | `{ "error": "FORBIDDEN", "message": "..." }` |
| `404` | Academia autenticada não encontrada. | `{ "error": "NOT_FOUND", "message": "academia não encontrado" }` |

---

---

## 7. Ano Letivo

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
  "ano_letivo": "2026_2027"
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

**Request:**

```json
{
  "tipo": "escola"
}
```

**Response 200:**

```json
{
  "message": "ano letivo definido com sucesso",
  "ano_letivo": "2026_2027",
  "tipo": "escola"
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
  "tipo": "escola",
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
      "tipo": "escola",
      "definido_por": "11111111-1111-1111-1111-111111111111",
      "definido_em": "2024-09-01T08:00:00Z"
    },
    {
      "ano_letivo": "2025_2026",
      "tipo": "escola",
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
2. **Período fixo por tipo** (`periodo`, exemplo `09_07`) — configuração global estável mantida pelo Admin FPP para calcular o intervalo real de datas aceitas.

Cada tipo canônico de ano letivo possui exatamente um período fixo global:

- `escolar` — usado para fundamental e médio. O alias legado `escola` não é mais aceito para `type` de ano letivo.
- `superior` — usado para ensino superior.

O `periodo` usa o formato `MM_MM`, em que o primeiro mês pertence ao ano inicial de `ano_letivo` e o segundo mês pertence ao ano final. Exemplo: `ano_letivo=2025_2026` com `periodo=10_07` permite datas de `2025-10-01` a `2026-07-31`. O cliente não precisa calcular esse intervalo para validar segurança; o backend recalcula e valida em operações sensíveis, especialmente faltas. A definição do ano letivo seguinte também respeita a mesma janela operacional da finalização: enquanto o mês atual ainda estiver dentro do ano letivo em curso delimitado pelo período, o avanço para o próximo ano letivo é bloqueado.

#### GET `/anos-letivos/configuracoes`

Lista as configurações vigentes.

Request: não possui body.

Response:

```json
{
  "configuracoes": [
    {
      "type": "escolar",
      "periodo": "09_07",
      "updated_at": "2026-06-26T10:30:00Z",
      "updated_by": "uuid-do-admin-fpp"
    },
    {
      "type": "superior",
      "periodo": "10_07",
      "updated_at": "2026-06-26T10:35:00Z",
      "updated_by": "uuid-do-admin-fpp"
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
      "updated_at": "2026-06-26T10:30:00Z",
      "updated_by": "uuid-do-admin-fpp"
    },
    {
      "type": "superior",
      "periodo": "10_07",
      "updated_at": "2026-06-26T10:35:00Z",
      "updated_by": "uuid-do-admin-fpp"
    }
  ]
}
```

#### PUT `/admin/sistema/anos-letivos/configuracoes/:type`

Apenas Admin FPP. Atualiza o período fixo do tipo informado. O parâmetro `:type` aceita somente `escolar` ou `superior`.

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

Response:

```json
{
  "message": "configuração de ano letivo atualizada com sucesso",
  "type": "escolar",
  "periodo": "09_07"
}
```

### Validação de faltas pelo período letivo

O registro e a atualização de faltas validam a data no backend usando o tipo inferido da matéria (`superior` ou `escolar` para fundamental/médio), o `ano_letivo` ativo da academia e o `periodo` configurado para o tipo. Datas fora do intervalo retornam `400` com mensagem indicando o intervalo permitido.

### Finalização de ano letivo por academia

#### POST `/academia/anos-letivos/finalizar`

A academia autenticada finaliza o ano letivo ativo no próprio escopo e, na mesma operação, avança automaticamente para o ano letivo seguinte. O cliente não envia `academia_id`; o backend obtém a academia pelo token, normaliza `type`, valida que o `ano_letivo` informado, quando presente, corresponde ao ano letivo ativo da academia, valida o formato `YYYY_YYYY` com segundo ano igual ao primeiro + 1, valida a janela mensal de finalização pelo `periodo` configurado para o tipo e grava um evento auditável `AnoLetivoAcademiaFinalizado`. A janela mensal é inclusiva no mês final e exclusiva no mês inicial: o mês atual precisa ser maior ou igual ao mês de fim do período letivo e menor que o mês de início do período letivo. Exemplo: se `periodo=10_07`, a finalização é permitida somente em julho, agosto e setembro; em outubro o próximo período já começou, e de novembro a junho o período vigente ainda não chegou ao mês de encerramento.

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

#### GET `/admin/academias/anos-letivos/finalizacoes?type=escolar&ano_letivo=2025_2026`

Apenas Admin FPP. Consulta finalizações por academia, com filtros opcionais.

Request: não possui body.

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

1. Admin FPP consulta/ajusta `GET|PUT /admin/sistema/anos-letivos/configuracoes/:type` para confirmar o `periodo` por tipo.
2. Admin FPP define inicialmente o ano global com `POST /admin/definir-ano-letivo-geral`, enviando `type` e `ano_letivo`, desde que não exista academia ativa com ano letivo definido.
3. Cada academia sem ano letivo define o próprio ano letivo com `POST /academia/definir-ano-letivo` usando o ano global atual.
4. Ao encerrar notas/faltas/avaliações de um ciclo, a academia chama `POST /academia/anos-letivos/finalizar`; essa chamada finaliza o ano ativo e já avança para o seguinte.
5. Telas administrativas podem usar `GET /admin/sistema/anos-letivos/finalizacao-limites` para mostrar o marco finalizado por todas as academias e o mínimo global permitido antes de tentar avançar ou corrigir o global.

---

## 8. Estudantes

### POST /academia/estudante/register

Cadastra um novo estudante vinculado à academia autenticada. A partir da versão 2.0.4, o cadastro direto usa `multipart/form-data` e exige a mesma matriz documental do `POST /solicitacao-matricula`; JSON puro não é mais aceito para evitar cadastro sem documentação obrigatória. Para estudantes escolares/fundamental/médio, a criação valida também que o BI do responsável não coincide com o BI principal de outro estudante escolar.

**Proteção**: autenticado + academia ativa

**Content-Type:** `multipart/form-data`

**Campos de texto:**

| Campo | Obrigatório | Observações |
| --- | --- | --- |
| `nome` | sim | Nome completo válido. |
| `genero` | sim | `masculino` ou `feminino`. |
| `data_nascimento` | sim | Data simples `YYYY-MM-DD`, anterior à data atual. |
| `email` | não | Validado quando informado. |
| `telefone` | condicional | Pelo menos um entre `telefone` e `telefone_responsavel`; para superior, `telefone_responsavel` pode ficar ausente. |
| `telefone_responsavel` | condicional | Não pode ser igual a `telefone`. |
| `bilhete_identidade` | não | Quando informado, deve ser único entre estudantes. |
| `bilhete_identidade_responsavel` | sim | Obrigatório para estudante escolar/fundamental/médio; não pode ser igual ao BI do estudante após normalização nem coincidir com o BI principal de outro estudante escolar/fundamental/médio. |
| `ano_escolar_fundamental` | condicional | Ano fundamental canônico, quando aplicável. |
| `ano_escolar_medio` | condicional | Ano médio canônico, quando aplicável. |
| `curso_medio_id` | condicional | UUID de curso médio ativo da academia, quando o ano médio for informado. |
| `ano_superior` | condicional | Ano superior canônico, quando aplicável. |
| `curso_superior_id` | condicional | UUID de curso superior ativo da academia, quando o ano superior for informado. |

**Ficheiros PDF aceitos:**

| Campo de arquivo | Regra |
| --- | --- |
| `bi_responsavel` | Obrigatório. |
| `bi_estudante` | Obrigatório quando `bilhete_identidade` do estudante for informado. |
| `cedula_estudante` | Obrigatória quando `bilhete_identidade` do estudante não for informado. |
| `declaracao` | Obrigatória quando o certificado aplicável não for enviado ou quando não existir certificado aplicável. |
| `certificado_6_ano_fundamental` | Aplicável a `7_ano_fundamental`, `8_ano_fundamental` e `9_ano_fundamental`; pode ser substituído por `declaracao`. |
| `certificado_9_ano_fundamental` | Aplicável ao ensino médio; pode ser substituído por `declaracao`. |
| `certificado_ensino_medio` | Aplicável ao ensino superior; pode ser substituído por `declaracao`. |

Todos os ficheiros devem ter `Content-Type: application/pdf`, extensão `.pdf`, assinatura `%PDF` e tamanho máximo de 5MB. Os documentos são armazenados em `{codigo_academia}/estudantes/{codigo_estudante}/documentos/` e gravados no evento `EstudanteCriadoComVinculo` e na projeção do estudante como `documentos.<campo>.path`, `documentos.<campo>.file_url` e `documentos.<campo>.download_url`. Se a criação falhar após upload parcial, o backend remove o diretório definitivo do estudante para evitar ficheiros órfãos.

**Exemplo cURL:**

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
  -F "bi_responsavel=@./bi_responsavel.pdf;type=application/pdf" \
  -F "certificado_6_ano_fundamental=@./certificado_6.pdf;type=application/pdf"
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
- `400` — `bi_responsavel` ausente, `cedula_estudante` ausente sem `bi_estudante`, ou certificado/declaração ausente
- `400` — ficheiro não PDF, sem assinatura `%PDF`, com extensão diferente de `.pdf` ou acima de 5MB
- `400` — BI do estudante igual ao BI do responsável, ou BI do estudante já cadastrado

---
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
  "total": 120,
  "tipo_usuario": "academia",
  "codigo_academia": "LDA20261",
  "nome_academia": "string"
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

Consulta um estudante por código.

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
    "curso_superior": null
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

Os status do estudante não são alterados diretamente. Eles mudam como consequência dos acontecimentos de domínio abaixo.

#### POST /academia/estudante/:codigo/matricula/fundamental

Efetiva matrícula no fundamental e muda `status_escolar_fundamental` para `em_andamento`.

**Request:**

```json
{
  "ano_escolar_fundamental": "1_ano_fundamental"
}
```


**Response 200:**

```json
{
  "message": "operação registrada com sucesso",
  "codigo_estudante": "ABC1234"
}
```
#### POST /academia/estudante/:codigo/matricula/medio

Efetiva matrícula no médio e muda `status_escolar_medio` para `em_andamento`. Exige fundamental `finalizado`.

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
  "message": "operação registrada com sucesso",
  "codigo_estudante": "ABC1234"
}
```
#### POST /academia/estudante/:codigo/matricula/superior

Efetiva matrícula no superior, muda `status_superior` para `em_andamento` e define `ano_superior = "1_ano_superior"` e `semestre_atual = 1`.

**Request:**

```json
{
  "curso_id": "uuid-do-curso-superior"
}
```


**Response 200:**

```json
{
  "message": "operação registrada com sucesso",
  "codigo_estudante": "ABC1234"
}
```
#### POST /academia/estudante/:codigo/interrupcao/fundamental

Registra interrupção do fundamental e muda `status_escolar_fundamental` para `inativo`.

**Request:**

```json
{ "motivo": "mudança de residência" }
```


**Response 200:**

```json
{
  "message": "operação registrada com sucesso",
  "codigo_estudante": "ABC1234"
}
```
#### POST /academia/estudante/:codigo/interrupcao/medio

Registra interrupção do médio e muda `status_escolar_medio` para `inativo`.

**Request:**

```json
{ "motivo": "pausa solicitada" }
```


**Response 200:**

```json
{
  "message": "operação registrada com sucesso",
  "codigo_estudante": "ABC1234"
}
```
#### POST /academia/estudante/:codigo/trancamento/superior

Registra trancamento do superior e muda `status_superior` para `inativo`.

**Request:**

```json
{ "motivo": "trancamento formal" }
```


**Response 200:**

```json
{
  "message": "operação registrada com sucesso",
  "codigo_estudante": "ABC1234"
}
```
#### POST /academia/estudante/:codigo/desvincular

Desvincula o estudante da academia preservando histórico e muda o status geral para `arquivado`. O evento registra `codigo_academia`, `codigo_estudante`, `motivo` e o nível acadêmico em que o estudante estava.

**Request:**

```json
{ "motivo": "transferência para outra instituição" }
```


**Response 200:**

```json
{
  "message": "operação registrada com sucesso",
  "codigo_estudante": "ABC1234"
}
```
#### POST /academia/estudante/:codigo/revincular

Reintegra estudante arquivado à academia e muda o status geral para `ativo`.

**Request — reingresso no fundamental:**

```json
{
  "tipo_ensino": "fundamental",
  "ano_escolar_fundamental": "4_ano_fundamental"
}
```

**Request — reingresso no médio:**

```json
{
  "tipo_ensino": "medio",
  "ano_escolar_medio": "2_ano_medio",
  "curso_medio_id": "uuid-do-curso-medio"
}
```

**Request — reingresso no superior:**

```json
{
  "tipo_ensino": "superior",
  "curso_superior_id": "uuid-do-curso-superior"
}
```

No reingresso superior o sistema define `ano_superior = "1_ano_superior"` e `semestre_atual = 1`.


**Response 200:**

```json
{
  "message": "operação registrada com sucesso",
  "codigo_estudante": "ABC1234"
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


**Request:** sem payload
**Response 200:**

```json
{
  "avaliacoes": [AvaliacaoFinalDTO],
  "total": 2
}
```

---

---

## 9. Solicitação de Matrícula

### POST /solicitacao-matricula

Cria uma solicitação pública de matrícula via `multipart/form-data`. O backend gera `codigo_solicitacao`, valida dados e PDFs, envia documentos para o armazenamento no caminho `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/` e grava `SolicitacaoMatriculaCriada` no ledger. Para cada arquivo enviado, o evento e a projeção salvam `path`, `file_url` (URL de visualização/arquivo no Drive) e `download_url` (URL direta de download quando o Google Drive disponibilizar `webContentLink`).

**Proteção**: pública

**Campos**: `codigo_academia`, `nome`, `genero`, `data_nascimento`, `email`, `telefone`, `bilhete_identidade`, `bilhete_identidade_responsavel`, `ano_escolar_fundamental`, `ano_escolar_medio`, `curso_medio_id`, `ano_superior`, `curso_superior_id`. Quando `bilhete_identidade` e `bilhete_identidade_responsavel` forem enviados juntos, eles não podem ser iguais (comparação sem espaços nas extremidades e sem diferenciar maiúsculas/minúsculas).

**Ficheiros PDF**: `bi_estudante`, `bi_responsavel`, `cedula_estudante`, `declaracao`, `certificado_6_ano_fundamental`, `certificado_9_ano_fundamental`, `certificado_ensino_medio`. Cada ficheiro deve ser PDF válido e ter no máximo 5MB. Para estudantes escolares/fundamental/médio, `bi_responsavel` é obrigatório; `bi_estudante` é obrigatório quando `bilhete_identidade` for informado; `cedula_estudante` é obrigatória quando o estudante não tiver BI próprio; `declaracao` é obrigatória quando o certificado aplicável ao ano académico não for enviado.

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

Lista solicitações da academia autenticada em ordem decrescente de criação.

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
          "file_url": "https://drive.google.com/file/d/FILE_ID/view?usp=drivesdk",
          "download_url": "https://drive.google.com/uc?id=FILE_ID&export=download"
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

Consulta uma solicitação da academia autenticada pelo `codigo_solicitacao`. Retorna `404` se não existir e `403` se pertencer a outra academia.

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
        "file_url": "https://drive.google.com/file/d/FILE_ID/view?usp=drivesdk",
        "download_url": "https://drive.google.com/uc?id=FILE_ID&export=download"
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

### POST /academia/curso

Cria um novo curso para a academia. O tipo efetivo do curso é inferido pelo backend a partir da academia autenticada (`medio` para escola do médio e `superior` para academia superior). O campo `type` pode ser enviado para explicitar a intenção, mas deve corresponder ao tipo permitido para a academia.

**Proteção**: autenticado + academia ativa

**Request para curso médio:**

```json
{
  "nome": "Ciências e Tecnologia",
  "type": "medio",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"]
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

Cursos superiores não aceitam `anos_academicos` no payload; cursos médios não aceitam `periodos` numérico.

**Response 201:**

```json
{
  "message": "curso criado com sucesso",
  "data": {
    "id": "uuid",
    "nome": "string",
    "type": "superior",
    "periodos": ["1_semestre", "2_semestre", "3_semestre", "4_semestre"]
  }
}
```

**Erros:**

- `400` — nome ausente, `type` incompatível com a academia ou anos_academicos inválidos para curso médio
- `400` — curso superior sem `periodos`, com `periodos <= 0`, decimal, string, array, nulo ou com `anos_academicos` enviado
- `400` — curso médio com `periodos` numérico enviado
- `403` — academia inativa não pode criar cursos

---

### GET /academia/cursos

Lista todos os cursos da academia, incluindo `anos_academicos` de cada curso.

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

Retorna um curso específico, incluindo seus `anos_academicos`.

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

Atualiza dados de um curso. O `type` é imutável.

**Proteção**: autenticado + academia ativa

**Validações de integridade:**

- Para cursos médios, `anos_academicos` pode ser enviado e a atualização é rejeitada se remover algum ano que ainda possua estudante ativo matriculado no curso.
- Para cursos superiores, `anos_academicos` não pode ser enviado. Quando `periodos` numérico é enviado, o backend recalcula semestres e anos acadêmicos derivados; se houver redução, a atualização é rejeitada ao remover semestre usado por estudante ativo em `semestre_atual` ou ano superior ainda usado por estudante ativo.
- `periodos` numérico é aceito apenas para cursos superiores e deve ser inteiro positivo.

**Request para curso médio:** (todos opcionais)

```json
{
  "nome": "string",
  "anos_academicos": ["1_ano_medio", "2_ano_medio"]
}
```

**Request para curso superior:** (todos opcionais)

```json
{
  "nome": "string",
  "periodos": 10
}
```

**Response 200:**

```json
{
  "message": "curso atualizado com sucesso",
  "nome": "string",
  "type": "superior",
  "anos_academicos": ["1_ano_superior", "2_ano_superior", "3_ano_superior", "4_ano_superior", "5_ano_superior"],
  "periodos": ["1_semestre", "2_semestre", "3_semestre", "4_semestre", "5_semestre", "6_semestre", "7_semestre", "8_semestre", "9_semestre", "10_semestre"]
}
```

**Erros:**

- `400` — nenhum campo para atualizar
- `400` — tentativa de remover `anos_academicos` ou `periodos` ainda usados por estudantes ativos
- `400` — curso superior com `anos_academicos` enviado ou `periodos` inválido
- `400` — curso médio com `periodos` numérico enviado

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
  "curso_id": "uuid"  // obrigatório para medio e superior
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
    "proximo_passo": "defina o periodo via PUT /academia/materias/uuid/periodo antes de ativar"
  }
}
```

**Notas:**

- Matérias `superior` nascem **inativas** e exigem período antes de ativar
- `curso_id` obrigatório para `medio` e `superior`
- Para `fundamental`: `anos_academicos` com 1 a 9 itens no formato correto
- Para `medio`/`superior`: exatamente 1 item no formato correto

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

### PUT /academia/materia/:id/periodo

Define o período de uma matéria do tipo `superior`. Pré-requisito para ativar a matéria.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "periodo": "1_semestre"  // deve existir nos períodos do curso vinculado
}
```

**Response 200:**

```json
{
  "message": "periodo definido com sucesso",
  "nome": "string",
  "periodo": "1_semestre"
}
```

**Erros:**

- `400` — matéria não é do tipo superior
- `400` — período não pertence ao curso vinculado

---

### PUT /academia/materia/:id/dados

Atualiza o nome de uma matéria.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "nome": "string"
}
```

**Response 200:**

```json
{
  "message": "matéria atualizada com sucesso",
  "nome": "string"
}
```

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

### DELETE /academia/turma/:codigo/estudantes/:codigoEstudante

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

### POST /academia/categorias-nota

Cria ou configura uma categoria de nota para a academia. O mesmo endpoint é usado para categorias adicionais e para definir os anos acadêmicos das categorias fixas/obrigatórias (`nota_escola`, `nota_professor`, `nota_pp1`, `nota_pp2`, `nota_exame`).

O campo `codigo` é normalizado antes de persistir: espaços antes/depois são descartados, somente espaços internos entre textos viram `_` (ex.: ` Prova profesor ` vira `prova_profesor`) e caracteres especiais diferentes de `_` são rejeitados. O código aceita letras minúsculas, números, espaços e `_`; letras maiúsculas são convertidas para minúsculas.

**Proteção**: autenticado + academia ativa

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

- `400` — codigo, nome ou anos_academicos ausente/vazio, ou codigo com caracteres especiais inválidos
- `409` — categoria já existe nesta academia

---

### GET /academia/categorias-nota

Lista todas as categorias de nota da academia alvo.

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
      "status": "ativo",
      "created_at": "2026-06-13T00:00:00Z",
      "version": 1
    }
  ],
  "total": 2
}
```

**Erros:**

- `404` — academia não encontrada (incluindo admin sem `codigo_academia`)

---

### DELETE /academia/categorias-nota/:codigo

Inativa (remove logicamente) uma categoria de nota adicional da academia.

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

- `400` — codigo, nome ou anos_academicos ausente/vazio, ou codigo com caracteres especiais inválidos no path
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
  "categoria": "nota_escola",
  "nota": 15.5,
  "observacao": "string"  // opcional
}
```

**Restrições:**

- Academia `escola` só pode usar tipo `escolar`
- Academia `superior` só pode usar tipo `superior`
- `nota` deve ser maior ou igual a 0 (`>= 0`)
- `periodo` deve ser válido para o tipo (`1_trimestre`/`2_trimestre`/`3_trimestre` para escolar; semestres do curso para superior)
- Para `tipo=superior`, o `periodo` precisa coincidir com o `periodo` definido na matéria (além de existir na lista de períodos do curso)
- Se o estudante tiver `ano_escolar_fundamental`, esse ano deve existir em `anos_academicos` da matéria; caso contrário, o registro é bloqueado
- `categoria` deve estar configurada em `POST /academia/categorias-nota` com `anos_academicos` contendo o `ano_academico` inferido da nota; sem anos definidos ou sem correspondência com o ano, nenhuma nota pode ser registrada nessa categoria
- O endpoint `POST /academia/notas-aluno/async` reaproveita exatamente as mesmas validações deste endpoint por item do lote

**Response 201:**

```json
{
  "message": "nota registrada com sucesso",
  "estudante": "ABC1234",
  "materia": "Matemática",
  "tipo": "escolar",
  "categoria": "nota_escola",
  "nota": 15.5,
  "ano_academico": "3_ano_fundamental",
  "periodo": "1_trimestre",
  "periodos_validos": ["1_trimestre", "2_trimestre", "3_trimestre"]
}
```

**Erros:**

- `400` — nota negativa, período inválido, categoria inválida/não configurada para o ano acadêmico, duplicata, ou incompatibilidade entre `ano_escolar_fundamental` do estudante e `anos_academicos` da matéria
- `403` — estudante ou matéria não pertencem à academia
- `400` — academia sem ano letivo configurado

---

### PUT /academia/atualizar-nota

Corrige uma nota já registada.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "id": "uuid",               // ID da nota na projeção
  "nota_nova": 16.0,          // obrigatório (usar 0.0 para zerar, não omitir)
  "observacao": "string"      // OBRIGATÓRIO (justificativa da correção)
}
```

**Response 200:**

```json
{
  "message": "nota atualizada com sucesso",
  "nota_anterior": 15.5,
  "nota_nova": 16.0,
  "observacao": "string"
}
```

**Erros:**

- `400` — nota_nova omitida ou negativa
- `400` — observacao ausente
- `403` — nota não pertence à academia
- `404` — nota não encontrada

---

### DELETE /academia/nota/:id

Remove uma nota (soft delete — permanece no ledger para auditoria).

**Proteção**: autenticado + academia ativa

**Path Params:**

- `id` — UUID da nota

**Request:**

```json
{
  "motivo": "string"  // OBRIGATÓRIO
}
```

**Response 200:**

```json
{
  "message": "nota deletada com sucesso"
}
```

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

- `limit` — padrão 50, máximo 1000
- `offset` — padrão 0
- `ano_letivo` — filtra por ano letivo (aceita múltiplos valores)
- `ano_academico` — filtra por ano académico (aceita múltiplos valores)
- `curso_id` — filtra por curso (nível médio ou superior) (aceita múltiplos valores)
- `codigo_turma` — filtra por turma (requer `codigo_academia` em consultas admin) (aceita múltiplos valores)
- `periodo` — filtra por período (`1_trimestre`, `2_trimestre`, `3_trimestre`, `1_semestre`, `2_semestre`) (aceita múltiplos valores)
- `materia_disciplinar_id` — filtra por matéria disciplinar (aceita múltiplos valores)
- `categoria` — filtra por categoria da nota (aceita múltiplos valores)
- `codigo_academia` — filtro de academia (admin); para academia autenticada, este filtro é sempre forçado ao seu próprio código
- `type` — filtra o tipo de avaliação final (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.) (aceita múltiplos valores)

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

---

---

---

## 13.1 Sumários/Aulas

Os sumários representam a aula ou conteúdo ministrado por uma academia e podem ser vinculados opcionalmente às faltas. O backend nunca confia em `academia_id`, `nivel` ou `type` enviados pelo cliente: a academia vem do token, enquanto `nivel` e `type` são inferidos a partir da matéria/curso validado.

### POST /academia/sumarios

Cria um sumário/aula.

**Request:**

```json
{
  "sumario_titulo": "Introdução às equações do 2º grau",
  "descricao": "Conteúdo detalhado opcional",
  "periodo": "1_trimestre",
  "ano_academico": 9,
  "curso_id": "uuid-do-curso",
  "materia_id": "uuid-da-materia"
}
```

Regras principais:

- `sumario_titulo` é obrigatório e deve ter entre 3 e 200 caracteres.
- `materia_id` é obrigatório e deve pertencer à academia autenticada.
- `curso_id` é obrigatório para matérias de `type=medio` e `type=superior`; quando a matéria já possui curso, o backend usa esse curso como fonte de verdade.
- Matérias superiores aceitam apenas períodos `N_semestre` e, se a matéria tiver `periodo` definido, ele deve coincidir com o período do sumário.
- Matérias escolares/médio aceitam períodos `N_trimestre`.
- `ano_academico` deve existir em `anos_academicos` da matéria.

**Response 201:**

```json
{
  "message": "sumário criado com sucesso",
  "sumario": {
    "id": "uuid",
    "sumario_titulo": "Introdução às equações do 2º grau",
    "materia_id": "uuid-da-materia"
  }
}
```
### GET /academia/sumarios

Lista sumários da academia autenticada. Admin pode consultar para suporte informando `codigo_academia`. Filtros opcionais: `periodo`, `ano_academico`, `curso_id`, `materia_id`.


**Request:** sem payload

**Response 200:**

```json
{
  "sumarios": [],
  "total": 0
}
```
### GET /academia/sumarios/:id

Retorna um sumário, desde que pertença à academia autenticada.


**Request:** sem payload

**Response 200:**

```json
{
  "sumario": {
    "id": "uuid",
    "sumario_titulo": "Introdução às equações do 2º grau"
  }
}
```
### PUT /academia/sumarios/:id

Atualiza título, descrição ou contexto acadêmico do sumário. A atualização reexecuta as mesmas validações de escopo da criação.

**Request:** (todos os campos opcionais; envie pelo menos um campo para alteração)

```json
{
  "sumario_titulo": "Introdução às equações do 2º grau",
  "descricao": "Conteúdo detalhado atualizado",
  "periodo": "1_trimestre",
  "ano_academico": 9,
  "curso_id": "uuid-do-curso",
  "materia_id": "uuid-da-materia"
}
```

**Response 200:**

```json
{
  "message": "sumário atualizado com sucesso"
}
```
### DELETE /academia/sumarios/:id

Remove logicamente o sumário (`deleted_at`), preservando faltas já vinculadas e seus snapshots históricos.

**Request:** sem payload

**Response 200:**

```json
{
  "message": "sumário deletado com sucesso"
}
```

## 14. Faltas
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
  "sumario_id": "uuid",               // opcional; cliente não envia sumario_titulo
  "observacao": "string"              // opcional
}
```

**Restrições:**

- `quantidade` deve ser maior ou igual a 1
- `data` é tratada como **date-only** (sem hora), em formato `AAAA-MM-DD`
- Se o estudante tiver `ano_escolar_fundamental`, esse ano deve existir em `anos_academicos` da matéria; caso contrário, o registro é bloqueado
- Idempotência (duplicata bloqueada): combinação `data + codigo_estudante + materia_disciplinar_id`
- `sumario_id` é opcional; quando informado, o backend busca o sumário, valida academia/matéria/ano acadêmico e grava `sumario_titulo` como snapshot histórico na falta
- O endpoint `POST /academia/faltas-aluno/async` reaproveita exatamente as mesmas validações deste endpoint por item do lote

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

- `400` — quantidade inválida (deve ser ≥ 1), data inválida, ou incompatibilidade entre `ano_escolar_fundamental` do estudante e `anos_academicos` da matéria
- `403` — estudante ou matéria não pertencem à academia

---

### PUT /academia/atualizar-falta

Corrige uma falta registada.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "id": "uuid",                         // obrigatório
  "data": "2025-03-16",                 // opcional
  "materia_disciplinar_id": "uuid",     // opcional
  "quantidade": 3,                      // opcional, mínimo 1
  "sumario_id": "uuid",                 // opcional; troca o vínculo e atualiza o snapshot
  "observacao": "string"                // OBRIGATÓRIO (justificativa da correção)
}
```

**Pelo menos um campo além do `id` deve ser informado.**

**Restrições:**

- `observacao` é obrigatória e não pode ser vazia
- `quantidade`, quando enviada, deve ser `>= 1`
- `data`, quando enviada, deve estar em `AAAA-MM-DD`
- Se `materia_disciplinar_id` for alterada (ou mantida), continua valendo a regra de compatibilidade com `ano_escolar_fundamental` do estudante
- A atualização também bloqueia duplicata pela combinação `data + codigo_estudante + materia_disciplinar_id`

**Response 200:**

```json
{
  "message": "falta atualizada com sucesso",
  "id": "uuid",
  "codigo_estudante": "ABC1234"
}
```

---

### DELETE /academia/falta/:id

Remove uma falta (soft delete).

**Proteção**: autenticado + academia ativa

**Path Params:**

- `id` — UUID da falta

**Request:**

```json
{
  "motivo": "string"  // OBRIGATÓRIO
}
```

**Response 200:**

```json
{
  "message": "falta deletada com sucesso"
}
```

**Restrições:**

- `motivo` é obrigatório e não pode ser vazio

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

- `limit` — padrão 50, máximo 1000
- `offset` — padrão 0
- `ano_letivo` — filtra por ano letivo (aceita múltiplos valores)
- `ano_academico` — filtra por ano académico (aceita múltiplos valores)
- `curso_id` — filtra por curso (nível médio ou superior) (aceita múltiplos valores)
- `codigo_turma` — filtra por turma (requer `codigo_academia` em consultas admin) (aceita múltiplos valores)
- `periodo` — filtra por período da matéria (`1_trimestre`, `2_trimestre`, `3_trimestre`, `1_semestre`, `2_semestre`) (aceita múltiplos valores)
- `materia_disciplinar_id` — filtra por matéria disciplinar (aceita múltiplos valores)
- `codigo_academia` — filtro de academia (admin); para academia autenticada, este filtro é sempre forçado ao seu próprio código
- `type` — filtra o tipo de avaliação final (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.) (aceita múltiplos valores)

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

---

---

## 15. Avaliações Finais

### Progressão semestral do ensino superior na avaliação final

Para `tipo_ensino = "superior"`, a avaliação final automática usa `semestre_atual` como unidade de progressão. O backend converte o inteiro armazenado no estudante para o período `[n]_semestre` (por exemplo, `semestre_atual = 3` vira `3_semestre`) e esse período deve existir em `curso.periodos`.

Regras superiores devem ser configuradas em `anos_academicos` com valores semestrais (`1_semestre`, `2_semestre`, ...). Fundamental e médio continuam usando anos acadêmicos (`[n]_ano_fundamental` e `[n]_ano_medio`). A unicidade da avaliação final superior considera estudante, academia, ano letivo, `tipo_ensino`, semestre avaliado e `type`, portanto uma avaliação de `1_semestre` não bloqueia a posterior avaliação de `2_semestre` no mesmo ano letivo.

Na aprovação superior, o backend incrementa `semestre_atual` quando ainda existe próximo semestre no curso e recalcula `ano_superior = ceil(semestre_atual / 2)`. Assim, `1_semestre → semestre_atual = 2` mantém `1_ano_superior`, enquanto `2_semestre → semestre_atual = 3` muda para `2_ano_superior`. Na aprovação no último semestre, `status_superior` passa para `finalizado`; na reprovação, `semestre_atual`, `ano_superior` e `status_superior` permanecem inalterados.

O cliente não envia `proximo_ano_academico`, `proximo_semestre_atual` nem resultado de aprovação: a fórmula da regra calcula `nota_final`, compara com `nota_minima_aprovacao` e emite o evento auditável com `semestre_atual`, `proximo_semestre_atual`, `ano_superior_antes` e `ano_superior_depois` para rebuild determinístico.

### Execução automática da avaliação final

Não existe rota pública/registrada para executar avaliação final manualmente. Em `cmd/server/main.go`, a academia só registra notas (`POST /academia/notas-aluno`) e configura/lista regras (`POST /academia/avaliacao-final/regras`, `GET /academia/avaliacao-final/regras`); a avaliação final é disparada automaticamente pelo backend quando uma nota é registrada.

**Por que o cliente não envia `type` para executar avaliação final:**

- O `type` da avaliação final executada (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.) vem da regra aplicável, não do payload de uma requisição manual.
- Ao registrar/atualizar notas, o backend identifica o estudante, infere o `tipo_ensino`, descobre o ano acadêmico atual e busca todas as regras ativas aplicáveis àquele ano.
- A cadeia precisa ter exatamente uma regra raiz, isto é, a regra sem `aplica_se_reprovado_em_type`. O processamento começa sempre nessa raiz.
- Cada regra dependente é alcançada pelo campo `aplica_se_reprovado_em_type`: por exemplo, `avaliacao_final_com_recurso` pode depender de reprovação em `avaliacao_final`, e `avaliacao_final_com_exame` pode depender de reprovação em `avaliacao_final_com_recurso`.
- O backend só executa uma dependente quando encontra reprovação no `type` pré-requisito. Se o pré-requisito aprovou, a dependente é encerrada e não executa. Se o pré-requisito ainda não existe, a dependente aguarda.
- Portanto, a ordem correta não é decidida pelo cliente nem pela categoria da nota recém-registrada; ela é calculada a partir da cadeia de regras configurada até a raiz.

**Regras de execução automática:**

- Se não houver regra ativa aplicável, nenhuma avaliação final é registrada.
- Se a cadeia aplicável não tiver exatamente uma raiz, o backend retorna erro para evitar ambiguidade.
- O backend evita duplicidade por `codigo_estudante`, `codigo_academia`, `ano_lectivo`, `tipo_ensino`, `ano_academico_atual` e `type`.
- Se alguma nota exigida pela fórmula ainda estiver ausente, aquela regra é ignorada naquele momento e poderá ser calculada quando novas notas forem registradas.
- Quando uma regra é executada, o backend calcula `nota_final`, define `aprovado = nota_final >= nota_minima_aprovacao`, calcula o próximo ano acadêmico e persiste o evento com snapshot da regra.
- O registro de nota retorna o campo `avaliacoes_finais_automaticas` com os resultados automáticos disparados naquele request.

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
      "turmas_removidas": ["T1A"]
    }
  ]
}
```

---
### POST /academia/avaliacao-final/regras

Cria uma regra ativa de avaliação final para a academia autenticada.

**Proteção**: academia autenticada.

**Request:**

```json
{
  "type": "avaliacao_final",
  "nome": "Avaliação final",
  "descricao": "Média dos três trimestres",
  "tipo_ensino": "fundamental",
  "anos_academicos": ["3_ano_fundamental"],
  "nota_minima_aprovacao": 10,
  "formula": "([nota_escola,1_trimestre]+[nota_professor,1_trimestre]+[nota_escola,2_trimestre]+[nota_professor,2_trimestre]+[nota_escola,3_trimestre]+[nota_professor,3_trimestre])/3",
  "aplica_se_reprovado_em_type": null
}
```

**Campos e validações:**

- `type` — obrigatório. Identifica a etapa pública (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.). Aceita apenas letras, números, espaços e `_`; espaços são normalizados para `_` antes de persistir (ex.: `exame final` vira `exame_final`), e outros caracteres são rejeitados.
- `nome` — obrigatório. Exemplos: `Avaliação final`, `Avaliação final (com exame)` ou `Avaliação final (com recurso)`.
- `descricao` — opcional.
- `tipo_ensino` — obrigatório; apenas `fundamental`, `medio` ou `superior`.
- `anos_academicos` — obrigatório e não vazio; não pode conter string vazia.
- `nota_minima_aprovacao` — obrigatório e maior que zero.
- `categorias_envolvidas` — opcional. O backend extrai automaticamente as categorias usadas em `formula`. Se enviado, deve corresponder exatamente às categorias extraídas da fórmula, sem duplicatas, sobras ou omissões, e todas precisam estar ativas/configuradas pela academia para os anos da regra.
- `formula` — obrigatório; deve ser uma string textual no modelo `formula_textual_v1`. O formato JSON em árvore antigo foi removido e não é aceito.
- `aplica_se_reprovado_em_type` — opcional para regra raiz; obrigatório para regras dependentes. Quando informado, passa pela mesma normalização de `type`, deve apontar para regra ativa existente na mesma academia/tipo de ensino, não pode ser igual ao próprio `type`, não pode criar ciclo e obriga a regra dependente a usar exatamente os mesmos `anos_academicos` da regra raiz da cadeia. Uma regra dependente inativa não pode ser ativada enquanto a regra da qual ela depende estiver inativa.

**Unicidade e cadeia:**

- Não pode existir outra regra ativa com o mesmo `type`, `tipo_ensino` e ano acadêmico sobreposto para a mesma academia. Ao criar ou editar uma regra, é permitido definir um `type` igual ao de uma regra inativa; porém essa regra inativa não poderá ser reativada enquanto existir uma regra ativa com o mesmo `type`, `tipo_ensino` e ano acadêmico sobreposto.
- Para cada academia, tipo de ensino e ano acadêmico, só pode haver uma regra raiz ativa. Regra raiz é a regra sem `aplica_se_reprovado_em_type`.
- Regras dependentes formam uma cadeia de novas chances; elas precisam ter os mesmos `anos_academicos` da raiz e só executam depois de reprovação no `type` apontado.
- A regra é criada pelo backend com `status = "ativo"` e `version = 1`; esses campos não são enviados na criação.

**Fórmula textual (`formula_textual_v1`):**

A fórmula é uma expressão declarativa interpretada por parser próprio do backend, sem `eval`, sem JavaScript e sem execução dinâmica. O resultado numérico da expressão vira `nota_final`.

- Referência de nota: `[categoria,periodo]`, por exemplo `[nota_escola,1_trimestre]` ou `[nota_exame,2_semestre]`.
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
      "tipo_ensino": "fundamental",
      "anos_academicos": ["3_ano_fundamental"],
      "nota_minima_aprovacao": 10,
      "categorias_envolvidas": ["nota_escola"],
      "formula": "([nota_escola,1_trimestre]+[nota_escola,2_trimestre]+[nota_escola,3_trimestre])/3",
      "aplica_se_reprovado_em_type": null,
      "status": "ativo",
      "version": 1
    }
  ],
  "total": 1
}
```

---


### PUT /academia/avaliacao-final/regras/:id

Edita uma regra ativa de avaliação final da academia autenticada. Por segurança, a edição é limitada aos campos que não mudam o desenho da cadeia: `nome`, `descricao`, `nota_minima_aprovacao` e `formula`. O backend recalcula `categorias_envolvidas` a partir da nova fórmula.

**Proteção**: academia autenticada.

**Request:**

```json
{
  "nome": "Avaliação final atualizada",
  "descricao": "Média ponderada atualizada",
  "nota_minima_aprovacao": 10,
  "formula": "([nota_escola,1_trimestre]*0.3)+([nota_escola,2_trimestre]*0.3)+([nota_exame,3_trimestre]*0.4)"
}
```

**Validações de segurança:**

- O `id` precisa ser UUID válido e pertencer à academia autenticada.
- A regra precisa estar `ativo`; regras inativas não são editadas.
- Não é permitido editar `type`, `tipo_ensino`, `anos_academicos`, `aplica_se_reprovado_em_type`, `status` nem `version` via payload, para não quebrar a cadeia já configurada. Caso uma versão futura permita editar `type`, a validação deve seguir a mesma regra da criação: o `type` pode coincidir com regra inativa, mas a regra inativa permanecerá bloqueada para ativação enquanto houver regra ativa conflitante.
- `nome` é obrigatório e não pode ser vazio.
- `nota_minima_aprovacao` precisa ser maior que zero.
- `formula` passa pelo mesmo parser seguro da criação; categorias são extraídas da fórmula e precisam estar ativas/configuradas para os anos da regra.
- Se `categorias_envolvidas` for enviado por compatibilidade, deve bater exatamente com as categorias da fórmula.
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

Inativa uma regra ativa de avaliação final da academia autenticada. A deleção é **lógica** (`status = "inativo"`), não física, para preservar histórico, auditoria e snapshots de avaliações já calculadas.

**Proteção**: academia autenticada.

**Comportamento em cadeia:**

- Se a regra tiver dependentes, o backend inativa também todas as dependentes diretas e indiretas.
- Essa cascata evita deixar regras órfãs apontando para um `type` inativo.
- Depois da inativação em cascata, uma regra dependente não pode ser ativada se a regra indicada em `aplica_se_reprovado_em_type` continuar inativa.
- Regra inativa cujo `type` conflite com outra regra ativa no mesmo `tipo_ensino` e ano acadêmico sobreposto não pode ser ativada até que o conflito seja removido.
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


**Request:** sem payload
**Response 200:**

```json
{
  "avaliacoes": [AvaliacaoFinalDTO],
  "total": 50
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


**Request:** sem payload
**Response 200:**

```json
{
  "aprovacoes": [AvaliacaoFinalDTO],
  "total": 35
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


**Request:** sem payload
**Response 200:**

```json
{
  "reprovacoes": [AvaliacaoFinalDTO],
  "total": 15
}
```

---


### GET /avaliacoes-estudante/:codigo

Retorna avaliações finais de um estudante específico.

**Proteção**: autenticado + academia ou admin


**Request:** sem payload
**Response 200:**

```json
{
  "codigo_estudante": "ABC1234",
  "nome": "string",
  "avaliacoes": [AvaliacaoFinalDTO],
  "total": 3
}
```

---

---

## 16. Admins

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

## 17. Jobs Assíncronos

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

|Endpoint|Payload por item|Resposta|Limite|
|---|---|---|---|
|`POST /academia/estudante/register/async`|igual ao `POST /academia/estudante/register`|`202` (job criado)|1000|
|`POST /academia/notas-aluno/async`|igual ao `POST /academia/notas-aluno`|`202` (job criado)|2000|
|`PUT /academia/atualizar-nota/async`|igual ao `PUT /academia/atualizar-nota`|`202` (job criado)|2000|
|`DELETE /academia/nota/async`|igual ao `DELETE /academia/nota/:id` (sem `:id`, enviado no item)|`202` (job criado)|2000|
|`POST /academia/faltas-aluno/async`|igual ao `POST /academia/faltas-aluno`|`202` (job criado)|2000|
|`PUT /academia/atualizar-falta/async`|igual ao `PUT /academia/atualizar-falta`|`202` (job criado)|2000|
|`DELETE /academia/falta/async`|igual ao `DELETE /academia/falta/:id` (sem `:id`, enviado no item)|`202` (job criado)|2000|
|`POST /academia/curso/async`|igual ao `POST /academia/curso`|`202` (job criado)|200|
|`POST /academia/materia/async`|igual ao `POST /academia/materia`|`202` (job criado)|500|
|`POST /academia/turma/async`|igual ao `POST /academia/turma`|`202` (job criado)|200|
|`POST /academia/turma/estudante/async`|igual ao `POST /academia/turma/:codigo/estudante`|`202` (job criado)|1000|
|`PUT /academia/dados/async`|igual ao `PUT /academia/dados`|`202` (job criado)|200|
|`POST /academia/categorias-nota/async`|igual ao `POST /academia/categorias-nota`|`202` (job criado)|500|
|`DELETE /academia/categorias-nota/async`|igual ao `DELETE /academia/categorias-nota/:nome` (`nome` vai no item)|`202` (job criado)|500|
|`PUT /academia/curso/ativar/async`|igual ao `PUT /academia/curso/:id/ativar` (`id` vai no item)|`202` (job criado)|500|
|`PUT /academia/curso/desativar/async`|igual ao `PUT /academia/curso/:id/desativar` (`id` vai no item)|`202` (job criado)|500|
|`PUT /academia/curso/dados/async`|igual ao `PUT /academia/curso/:id/dados` (`id` vai no item)|`202` (job criado)|500|
|`DELETE /academia/curso/async`|igual ao `DELETE /academia/curso/:id` (`id` vai no item)|`202` (job criado)|500|
|`PUT /academia/materia/ativar/async`|igual ao `PUT /academia/materia/:id/ativar` (`id` vai no item)|`202` (job criado)|1000|
|`PUT /academia/materia/desativar/async`|igual ao `PUT /academia/materia/:id/desativar` (`id` vai no item)|`202` (job criado)|1000|
|`PUT /academia/materia/periodo/async`|igual ao `PUT /academia/materia/:id/periodo` (`id` vai no item)|`202` (job criado)|1000|
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

## 19. Armazenamento

### GET /dominis/storage/quota

Retorna a distribuição dos arquivos existentes dentro da pasta raiz compartilhada/gerenciada pelo Spuri no Google Drive. Em produção, o backend deve estar configurado com `GOOGLE_DRIVE_CREDENTIALS_PATH` ou `GOOGLE_DRIVE_CREDENTIALS_JSON`, além de `GOOGLE_DRIVE_ROOT_FOLDER_ID`; nessa configuração, o backend lista recursivamente apenas a pasta raiz configurada. `total_bytes` e `used_bytes` são a soma dos arquivos existentes nessa pasta raiz, `managed_bytes` e `academias` detalham arquivos dentro dos diretórios de academia, e `outside_academias_bytes` detalha arquivos da raiz que não estão dentro de diretórios de academia. O backend não consulta nem estima consumo de arquivos fora da pasta raiz compartilhada; `unmanaged_bytes` permanece apenas por compatibilidade e não representa mais uso externo da conta.

Sem credenciais de produção, o backend só permite estimativa local quando `GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true`, contabilizando apenas os arquivos dentro de `GOOGLE_DRIVE_LOCAL_ROOT` (padrão `data/google_drive_storage`) com a mesma regra relativa à pasta raiz.

Quando a configuração do Google Drive ou da quota estiver incompleta ou inválida, a rota retorna `503 Service Unavailable` com a mensagem operacional gerada pelo storage. Exemplos de mensagens:

- `configuração Google Drive incompleta: GOOGLE_DRIVE_ROOT_FOLDER_ID é obrigatório`
- `configuração Google Drive incompleta: nenhuma credencial configurada (defina GOOGLE_DRIVE_CREDENTIALS_PATH ou GOOGLE_DRIVE_CREDENTIALS_JSON)`
- `credencial Google Drive inválida: JSON malformado ou não é uma service account`
- `quota do Google Drive indisponível: configure credenciais e GOOGLE_DRIVE_ROOT_FOLDER_ID; para ambiente local, defina GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true`

**Proteção**: autenticado + admin


**Request:** sem payload
**Response 200:**

```json
{
  "provider": "google_drive",
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
  "message": "quota do Google Drive indisponível: configure credenciais e GOOGLE_DRIVE_ROOT_FOLDER_ID; para ambiente local, defina GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true",
  "request_id": "8c7e6a5d-9b9f-4fd2-a2d0-3a989a8c2d8b"
}
```

---
