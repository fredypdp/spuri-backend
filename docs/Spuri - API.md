---
modificado: 10-05-2026 10:00
criado: 05-04-2026 13:01
---
Versão atual: 1.5.9
## Índice

1. [[#1. Convenções Globais]]
2. [[#2. Estruturas de Dados]]
3. [[#3. Autenticação]]
4. [[#4. Email]]
5. [[#5. Perfil e Conta]]
6. [[#6. Academias — Gestão pelo Admin]]
7. [[#7. Academia — Operações Próprias]]
8. [[#8. Estudantes]]
9. [[#9. Notas]]
10. [[#10. Faltas]]
11. [[#11. Avaliações Finais]]
12. [[#12. Cursos]]
13. [[#13. Matérias]]
14. [[#14. Turmas]]
15. [[#15. Admins]]
16. [[#16. Consultas Gerais]]
17. [[#17. Jobs Assíncronos]]
18. [[#18. Batch Assíncrono — Academia]]
19. [[#19. Batch Assíncrono — Admin]]

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
type StatusEscolar = 'inativo' | 'em_andamento' | 'finalizado'
type TipoEnsino = 'fundamental' | 'medio' | 'superior'
type Turno = 'manha' | 'tarde' | 'noite'
type CursoType = 'medio' | 'superior'
type MateriaType = 'fundamental' | 'medio' | 'superior'
type Genero = 'masculino' | 'feminino'
type TipoNota = 'escolar' | 'superior'
type JobStatus = 'pending' | 'processing' | 'done' | 'failed'
type JobEventType = 'job_enqueued' | 'job_progress' | 'job_done' | 'job_failed'
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
  numero_telefone?: string
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
  tipo_ano_letivo?: string        // 'escola' | 'superior'
  ano_letivo_ativado_em?: string  // RFC3339
  anos_letivos_lista: AnoLetivoItem[]
  created_at: string
  updated_at?: string
  version: number
}

interface AnoLetivoItem {
  ano_letivo: string              // ex: '2025_2026'
  tipo: 'escola' | 'superior'
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
  telefone?: string
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

## 4. Email

Todos os endpoints de email têm rate limiting ativo.

### POST /email/verificar-email/:token

Verifica o email usando o token recebido no email. Funciona para admin, academia e estudante.

**Proteção**: pública

**Path Params:**

- `token` — token de verificação (hex de 64 caracteres)

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

## 5. Perfil e Conta

### GET /meu-perfil

Retorna os dados do usuário autenticado. O formato da resposta varia por tipo.

**Proteção**: autenticado (qualquer tipo)

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
    "numero_telefone": "string",
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

### POST /adicionar-telefone-extra

Adiciona um número de telefone extra ao usuário autenticado.

**Proteção**: autenticado (qualquer tipo)

**Request:**

```json
{
  "numero_telefone": "string"  // ex: '+244923000000' ou '923000000'
}
```

**Response 201:**

```json
{
  "message": "telefone extra adicionado com sucesso",
  "id": "uuid",
  "numero_telefone": "244923000000",  // normalizado
  "verificado": false
}
```

**Erros:**

- `400` — formato inválido (deve ter 7-15 dígitos)
- `409` — número já verificado por outro usuário
- `409` — você já cadastrou este número

---

## 6. Academias — Gestão pelo Admin

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
  "numero_telefone": "+244923000000",
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

### POST /admin/sistema/ano-letivo

Define ou atualiza o **ano letivo oficial global do sistema**. Esta é a única rota vigente para esta configuração; a rota legada `POST /dominis/sistema/ano-letivo` foi removida.

**Proteção**: autenticado + admin role `fpp`

**Regras de negócio:**

- Apenas `fpp` pode alterar o ano letivo global.
- O formato deve ser `YYYY_YYYY` com segundo ano = primeiro + 1.
- Esse valor torna-se referência obrigatória para a rota `POST /academia/ano-letivo`.

**Request:**

```json
{
  "ano_letivo": "2026_2027"
}
```

**Response 200:**

```json
{
  "message": "ano letivo global definido com sucesso",
  "ano_letivo": "2026_2027"
}
```

**Erros:**

- `400` — formato inválido
- `403` — usuário não é `fpp`

---

### GET /admin/sistema/ano-letivo

Retorna o **ano letivo oficial global atual** da plataforma.

**Proteção**: autenticado + admin

**Response 200:**

```json
{
  "ano_letivo": "2026_2027"
}
```

**Erros:**

- `404` — ano letivo global ainda não definido

---

### GET /admin/sistema/anos-letivos-lista

Retorna a **lista histórica de anos letivos globais** já definidos pelo admin.

**Proteção**: autenticado + admin

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

## 7. Academia — Operações Próprias

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
  "numero_telefone": "string",
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

### POST /academia/ano-letivo

Define ou atualiza o ano letivo ativo da academia.

**Proteção**: autenticado + academia ativa

**Regras de negócio:**

- A academia só altera o próprio ano letivo (não altera o de outras academias).
- `ano_letivo` deve seguir `YYYY_YYYY` com segundo ano = primeiro + 1.
- `ano_letivo` deve ser igual ao ano letivo oficial global do sistema (definido por admin `fpp`).
- Se o ano letivo global ainda não existir, a definição na academia deve ser rejeitada.
- Sem ano letivo ativo válido, operações de notas/faltas/avaliações finais ficam bloqueadas.

**Request:**

```json
{
  "ano_letivo": "2025_2026",  // formato YYYY_YYYY obrigatório
  "tipo": "escola"             // 'escola' | 'superior'
}
```

**Response 200:**

```json
{
  "message": "ano letivo definido com sucesso",
  "ano_letivo": "2025_2026",
  "tipo": "escola"
}
```

**Erros:**

- `400` — formato inválido (o segundo ano deve ser exatamente o primeiro + 1)
- `400` — tipo inválido
- `400` — ano letivo diferente do ano letivo global oficial
- `409` — ano letivo global ainda não definido pelo admin `fpp`

**Regra da lista histórica (`anos_letivos_lista`)**: quando um ano letivo é definido, ele é adicionado na lista apenas se ainda não existir. Se já estiver listado, o backend ignora a duplicação.

---

### GET /academia/ano-letivo

Retorna o ano letivo ativo da academia autenticada.

**Proteção**: autenticado + academia ativa **ou** admin

**Query params:**

- `codigo_academia` (opcional para academia, obrigatório para admin): código da academia alvo.
  - Se o usuário for `academia`, o backend ignora o parâmetro e retorna o próprio ano letivo.
  - Se o usuário for `admin`, deve informar `?codigo_academia=...`.

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

### POST /academia/categorias-nota

Cria uma categoria de nota adicional para a academia.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "codigo": "nota_teste",      // obrigatório, sem espaços
  "nome": "Nota de teste",      // obrigatório (rótulo)
  "descricao": "string"        // opcional
}
```

**Response 201:**

```json
{
  "message": "categoria criada com sucesso",
  "categoria": "nota_teste"
}
```

**Erros:**

- `400` — codigo ou nome ausente
- `409` — categoria já existe nesta academia

---

### GET /academia/categorias-nota

Lista todas as categorias de nota da academia alvo.

**Proteção**: autenticado + (`academia` ativa **ou** `admin` **ou** `estudante`)

**Query params (quando `admin`)**:

- `codigo_academia` (obrigatório)

**Response 200:**

```json
{
  "categorias": [CategoriaNotaDTO],
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
  "categoria": "nota_teste"
}
```

**Erros:**

- `400` — codigo ou nome ausente no path
- `400` — categoria não existe nesta academia

---

## 8. Estudantes

### POST /academia/estudante/register

Cadastra um novo estudante vinculado à academia autenticada.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "nome": "João Silva",
  "genero": "masculino",
  "data_nascimento": "2010-05-20",
  "email": "joao@exemplo.ao",
  "telefone": "+244923000000",
  "bilhete_identidade": "001234567LA089",
  "bilhete_identidade_responsavel": "009876543LA089",
  "ano_escolar_fundamental": "3_ano_fundamental",
  "status_escolar_fundamental": "em_andamento",
  "ano_escolar_medio": null,
  "status_escolar_medio": "inativo",
  "curso_medio_id": null,
  "ano_superior": null,
  "status_superior": "inativo",
  "curso_superior_id": null
}
```

**Campos obrigatórios:** `nome`, `genero`, `data_nascimento`

**Response 201:**

```json
{
  "message": "estudante registrado com sucesso",
  "data": {
    "id": "uuid",
    "codigo_estudante": "ABC1234",
    "codigo_academia": "LDA20261"
  }
}
```

**Erros:**

- `400` — genero inválido, data_nascimento inválida ou no futuro
- `400` — ano_escolar_fundamental em formato incorreto
- `400` — curso_medio_id não encontrado ou tipo errado

---

### GET /estudantes

Lista estudantes. Retorna apenas os da academia (para academia) ou todos (para admin).

**Proteção**: autenticado + academia ou admin

**Query Params:**

- `limit` — máximo de itens (padrão: sem limit = retorna até 1000)
- `offset` — deslocamento para paginação (padrão: 0)

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

---

### GET /consultar-estudante/:codigo

Consulta um estudante por código.

**Proteção**: autenticado + academia (apenas próprios) ou admin

**Path Params:**

- `codigo` — código do estudante (ex: `ABC1234`)

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

### PUT /academia/estudante/:codigo/status-escolar-fundamental

Atualiza o status do ensino fundamental do estudante.

**Proteção**: autenticado + academia ativa

**Path Params:**

- `codigo` — código do estudante

**Request:**

```json
{
  "novo_status": "em_andamento"  // 'inativo' | 'em_andamento' | 'finalizado'
}
```

**Response 200:**

```json
{
  "message": "status_escolar_fundamental atualizado com sucesso",
  "novo_status": "em_andamento"
}
```

---

### PUT /academia/estudante/:codigo/status-escolar-medio

Atualiza o status do ensino médio do estudante.

**Proteção**: autenticado + academia ativa

**Path Params:**

- `codigo` — código do estudante

**Request:**

```json
{
  "novo_status": "em_andamento"  // 'inativo' | 'em_andamento' | 'finalizado'
}
```

**Response 200:**

```json
{
  "message": "status_escolar_medio atualizado com sucesso",
  "novo_status": "em_andamento"
}
```

---

### PUT /academia/estudante/:codigo/status-superior

Atualiza o status do ensino superior do estudante.

**Proteção**: autenticado + academia ativa

**Regra**: só pode avançar se o fundamental e o médio estiverem `finalizado` ou `inativo`.

**Path Params:**

- `codigo` — código do estudante

**Request:**

```json
{
  "novo_status": "em_andamento"  // 'inativo' | 'em_andamento' | 'finalizado'
}
```

**Response 200:**

```json
{
  "message": "status_superior atualizado com sucesso",
  "novo_status": "em_andamento"
}
```

---

### GET /eventos-estudante/:codigo

Retorna todos os eventos do ledger de um estudante (trilha de auditoria completa).

**Proteção**: autenticado + admin (qualquer role)

**Path Params:**

- `codigo` — código do estudante

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

**Response 200:**

```json
{
  "avaliacoes": [AvaliacaoFinalDTO],
  "total": 2
}
```

---

## 9. Notas

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
- `categoria` deve ser uma das fixas ou uma adicional da academia
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

- `400` — nota negativa, período inválido, categoria inválida, duplicata, ou incompatibilidade entre `ano_escolar_fundamental` do estudante e `anos_academicos` da matéria
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

## 10. Faltas

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
- Se o estudante tiver `ano_escolar_fundamental`, esse ano deve existir em `anos_academicos` da matéria; caso contrário, o registro é bloqueado
- Idempotência (duplicata bloqueada): combinação `data + codigo_estudante + materia_disciplinar_id`
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

## 11. Avaliações Finais

### POST /academia/avaliacao-final

Registra a avaliação final de ano para um estudante.

**Proteção**: autenticado + academia ativa (com ano letivo configurado)

**Request:**

```json
{
  "codigo_estudante": "ABC1234",
  "nivel_ano_academico_atual": "3_ano_fundamental",
  "aprovado": true,
  "observacao": "string"  // opcional — se fornecido, bypassa validação de notas
}
```

**Regras:**

- `tipo_ensino` não é enviado no payload; o backend infere automaticamente com base no estudante (sessão + código da academia)
- `nivel_ano_academico_atual` deve seguir o formato canônico do tipo inferido
- `proximo_ano_academico` é calculado automaticamente pelo backend e não deve ser enviado no payload
- Se `aprovado = true`:
  - **escola**: o backend calcula o próximo ano automaticamente e move o estudante da turma atual para uma turma do ano acadêmico seguinte
  - fundamental: sequência fixa `1_ano_fundamental` até `9_ano_fundamental`
  - médio: sequência configurada no curso do estudante
  - superior: avanço sequencial por semestre (`semestre_atual += 1`) até o último semestre configurado do curso
- Se `aprovado = false`:
  - **escola**: o backend mantém o estudante no mesmo nível e na mesma turma
  - demais tipos: sem avanço de nível (sem próximo ano)
- Sem `observacao`: notas de todas as matérias do período são validadas automaticamente

**Response 201:**

```json
{
  "message": "avaliação final registrada com sucesso",
  "resultado": "aprovado → 4_ano_fundamental",
  "turmas_removidas": ["T1A"]
}
```

**Erros:**

- `400` — formato de ano inválido
- `400` — notas obrigatórias faltando (sem observacao para override)
- `400` — `proximo_ano_academico` enviado no payload (campo não permitido)
- `400` — `nivel_ano_academico_atual` inválido para o ciclo (fundamental fora de 1..9, ou fora do curso em médio/superior)
- `409` — avaliação já registrada para este tipo/ano/nível

**Modelos de avaliação final:**
- **Escolar (fundamental/médio):** mantém o comportamento atual por ano acadêmico.
- **Superior:** aprovação sempre avança para o próximo semestre; o avanço de `ano_superior` é automático por `ceil(semestre_atual / 2)`.

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

## 12. Cursos

### POST /academia/curso

Cria um novo curso para a academia.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "nome": "Ciências e Tecnologia",
  "type": "medio",
  "anos_academicos": ["1_ano_medio", "2_ano_medio", "3_ano_medio"],
  "periodos": []  // obrigatório para 'superior', vazio/ausente para 'medio'
}
```

**Para superior:**

```json
{
  "nome": "Engenharia Informática",
  "type": "superior",
  "anos_academicos": ["1_ano_superior", "2_ano_superior", "3_ano_superior", "4_ano_superior"],
  "periodos": ["1_semestre", "2_semestre"]
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
    "periodos": []
  }
}
```

**Erros:**

- `400` — tipo inválido, anos_academicos inválidos
- `400` — periodos ausentes para tipo superior
- `403` — academia do tipo escola não pode criar curso superior (e vice-versa)

---

### GET /academia/cursos

Lista todos os cursos da academia.

**Proteção**: autenticado + (`academia` ativa **ou** `admin` **ou** `estudante`)

**Query params (quando `admin`)**:

- `codigo_academia` (obrigatório)

**Response 200:**

```json
{
  "cursos": [CursoDTO],
  "total": 3
}
```

---

### GET /academia/curso/:id

Retorna um curso específico.

**Proteção**: autenticado + (`academia` ativa **ou** `admin` **ou** `estudante`)

**Response 200:** `CursoDTO`

---

### PUT /academia/curso/:id/ativar

Ativa um curso inativo.

**Proteção**: autenticado + academia ativa

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

**Response 200:**

```json
{
  "message": "curso desativado com sucesso",
  "nome": "string"
}
```

---

### PUT /academia/curso/:id/dados

Atualiza nome, anos_academicos ou periodos de um curso. O `type` é imutável.

**Proteção**: autenticado + academia ativa

**Request:** (todos opcionais)

```json
{
  "nome": "string",
  "anos_academicos": ["1_ano_medio"],
  "periodos": ["1_semestre", "2_semestre"]
}
```

**Response 200:**

```json
{
  "message": "curso atualizado com sucesso",
  "nome": "string",
  "type": "medio",
  "anos_academicos": [],
  "periodos": []
}
```

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

## 13. Matérias

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

**Response 200:** `MateriaDTO`

---

### PUT /academia/materia/:id/ativar

Ativa uma matéria inativa. Matérias superiores sem período definido não podem ser ativadas.

**Proteção**: autenticado + academia ativa

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

**Response 200:**

```json
{
  "message": "materia deletada com sucesso",
  "nome": "string"
}
```

---

## 14. Turmas

### POST /academia/turma

Cria uma nova turma.

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

- `400` — turno inválido (deve ser `manha`, `tarde` ou `noite`)
- `409` — código de turma já existe nesta academia

---

### GET /academia/turmas

Lista todas as turmas da academia.

**Proteção**: autenticado + (`academia` ativa **ou** `admin` **ou** `estudante`)

**Query params (quando `admin`)**:

- `codigo_academia` (obrigatório)

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

**Response 200:** `TurmaDTO`

---

### PUT /academia/turma/:codigo/ativar

Ativa uma turma inativa.

**Proteção**: autenticado + academia ativa

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

**Response 200:**

```json
{
  "message": "estudante removido da turma com sucesso",
  "codigo_turma": "T1A",
  "codigo_estudante": "ABC1234"
}
```

---

## 15. Admins

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

## 16. Consultas Gerais

### GET /academias

Lista todas as academias com paginação e filtro de status.

**Proteção**: autenticado (qualquer tipo)

**Query Params:**

- `limit` — quantidade máxima (padrão sem limit: 1000, teto: 1000)
- `offset` — deslocamento (padrão: 0)
- `status` — `ativo` ou `inativo` (omitir = retorna ambos)

**Response 200:**

```json
{
  "academias": [AcademiaDTO],
  "total": 25,
  "limit": 1000,
  "offset": 0
}
```

**Nota**: admins veem campos extras (`email`, `total_estudantes`, `version`).

---

### GET /consultar-academia/:codigo

Retorna detalhes de uma academia.

**Proteção**: autenticado (qualquer tipo)

**Response 200:**

```json
{
  "id": "uuid",
  "nivel": "escola",
  "type": "public",
  "nome": "string",
  "codigo_academia": "LDA20261",
  "provincia": "LDA",
  "status": "ativo",
  ...
}
```

**Nota**: admins veem também `email` e `motivo_desativacao`.

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
- `codigo_academia` — filtro de academia (admin); para academia autenticada, este filtro é sempre forçado ao seu próprio código (aceita múltiplos valores)

**Formato de múltiplos valores (todos os filtros acima):**

- chave repetida: `?ano_letivo=2024_2025&ano_letivo=2025_2026`
- CSV na mesma chave: `?ano_letivo=2024_2025,2025_2026`
- também é possível combinar os dois formatos na mesma chamada

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
- `codigo_academia` — filtro de academia (admin); para academia autenticada, este filtro é sempre forçado ao seu próprio código (aceita múltiplos valores)

**Formato de múltiplos valores (todos os filtros acima):**

- chave repetida: `?periodo=1_trimestre&periodo=2_trimestre`
- CSV na mesma chave: `?periodo=1_trimestre,2_trimestre`
- também é possível combinar os dois formatos na mesma chamada

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

## 17. Jobs Assíncronos

### GET /jobs

Lista os jobs recentes do usuário autenticado.

**Proteção**: autenticado (qualquer tipo)

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

---

### DELETE /jobs/:id/sse

Oculta um job do stream SSE da academia autenticada.

**Proteção**: autenticado + academia (apenas o dono do job)

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

## 18. Batch Assíncrono — Academia

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
|`POST /academia/avaliacao-final/async`|igual ao `POST /academia/avaliacao-final`|`202` (job criado)|1000|
|`PUT /academia/estudante/status-escolar/async`|igual aos endpoints síncronos de status escolar do estudante|`202` (job criado)|1000|
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

## 19. Batch Assíncrono — Admin

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
