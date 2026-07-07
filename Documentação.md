---
modificado: 28-06-2026 17:10
criado: 05-04-2026 13:01
---
Versão atual: 2.1.1
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

O formato padronizado usado pelas rotas que chamam `utils.RespondWithError`/`RespondWithDetailedError` é:

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

Alguns middlewares e handlers legados retornam erro simples, sem `message`, `request_id` ou `details`:

```json
{
  "error": "mensagem do erro"
}
```

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
type StatusGeralEstudante = 'inativo' | 'ativo' | 'arquivado'
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
interface MateriasChaveCursoAnoDTO {
  ano_academico: string      // deve pertencer a CursoDTO.anos_academicos
  materias_chave: string[]   // UUIDs de matérias médias ativas do mesmo curso/ano
}

interface CursoDTO {
  id: string
  nome: string
  type: CursoType            // preenchido automaticamente pelo backend e imutável
  modelo?: ModeloCursoMedio  // obrigatório e exposto apenas em cursos médios
  anos_academicos: string[]  // ex: ['1_ano_medio', '2_ano_medio', '3_ano_medio']
  periodos?: string[]        // ex: ['1_semestre', '2_semestre'] — apenas para superior
  materias_chave?: MateriasChaveCursoAnoDTO[] // apenas médio; estado configurado por rota própria após a criação
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
  categoria: string           // código de categoria cadastrada pela academia
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
  anos_academicos: string[]
  adicionado_por?: string  // UUID
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

### Segurança e Autenticação

### Consulta pública de academias

As rotas `GET /academias` e `GET /consultar-academia/:codigo` são públicas com autenticação opcional. Usuários não autenticados podem consultar a lista de academias ou uma academia específica pelo código, mas a resposta expõe somente os campos públicos: `nivel`, `type`, `nome`, `codigo_academia`, `provincia`, `endereco`, `nivel_escolar` e `anos_academicos`. Para escolas fundamentais ou mistas, `anos_academicos` informa os anos acadêmicos ofertados sem exigir sessão.

As rotas `GET /academia/cursos?codigo_academia=...` e `GET /academia/curso/:id` também são públicas com autenticação opcional para consulta dos cursos e dos anos desses cursos em escolas do médio e academias do nível superior. Academias autenticadas continuam consultando os próprios cursos sem informar `codigo_academia`; admins autenticados continuam informando `codigo_academia` na listagem.

Quando a requisição envia `Authorization: Bearer <jwt_token>` válido, a API preserva o contrato autenticado anterior, retornando também campos operacionais para usuários autenticados e campos administrativos adicionais para admins. Tokens enviados em formato inválido, expirados ou pertencentes a contas inativas devem ser rejeitados com `401`.

### 8.1 JWT

- Algoritmo: HS256
- Expiração configurável via `JWT_EXPIRY_HOURS` (padrão: 24h)
- Secret configurável via `JWT_SECRET` (obrigatório em produção)
- Payload: `user_id` (UUID) e `user_type` (string)

### 8.2 Senhas

- Algoritmo: bcrypt com custo padrão
- Senhas de admins: geradas com `crypto/rand` (segurança criptográfica), nunca hardcoded
- Senhas de academia/estudante: código da entidade como senha inicial
- Todas as alterações de senha passam pelo ledger (evento)

### 8.3 Hash Chain do Ledger

Cada evento no ledger tem um hash SHA256 que inclui o hash do evento anterior:

```
hash(evento_N) = SHA256(conteúdo_N + hash(evento_N-1))
```

Qualquer adulteração de um evento invalida toda a cadeia a partir daquele ponto, tornando a adulteração detectável.

### 8.4 Whitelist de Eventos

Apenas eventos previamente autorizados podem ser gravados no ledger (`safe_queries.go`). Qualquer evento desconhecido é rejeitado antes de chegar ao banco.

### 8.5 Segurança das Queries

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

### 5.1 Cadastro de Academia

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

### 6.1 Regras de Academia

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

**Nota**: se o email for alterado, `email_verificado` volta para `false`; se o telefone for alterado, `telefone_verificado` volta para `false`.

---

### Regras automáticas de documentos de matrícula

A obrigatoriedade dos documentos não é mais configurada por academia. O backend aplica automaticamente as regras abaixo no `POST /solicitacao-matricula` e na aprovação da solicitação. No cadastro direto `POST /academia/estudante/register`, os anexos são opcionais e somente os PDFs enviados são validados tecnicamente:

- Para estudantes escolares/fundamental/médio, `bi_responsavel` e `bilhete_identidade_responsavel` são obrigatórios na solicitação pública e na aprovação; no cadastro direto, apenas o `bilhete_identidade_responsavel` textual permanece obrigatório.
- `bilhete_identidade` e `bilhete_identidade_responsavel`, quando ambos informados para o mesmo estudante, não podem ser iguais.
- Na solicitação pública, `bi_estudante` é obrigatório quando `bilhete_identidade` for informado; `cedula_estudante` é obrigatória quando o estudante não tiver BI próprio.
- Para estudantes escolares/fundamental/médio, o BI do responsável não pode coincidir com o BI principal de outro estudante escolar/fundamental/médio; ele pode repetir como BI de responsável de outros estudantes.
- `certificado_6_ano_fundamental` é o certificado aplicável somente para `7_ano_fundamental`, `8_ano_fundamental` e `9_ano_fundamental`.
- `certificado_9_ano_fundamental` é o certificado aplicável somente para anos do ensino médio.
- `certificado_ensino_medio` é o certificado aplicável somente para anos do ensino superior.
- Na solicitação pública, `declaracao` é obrigatória quando o certificado aplicável não for enviado ou quando não existir certificado aplicável ao ano académico informado.


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

### 5.3 Configuração do Ano Letivo

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

### 5.2 Cadastro de Estudante

**Quem faz**: Academia (status ativo)

1. Academia envia os dados do estudante em `multipart/form-data`, com ou sem anexos.
2. Sistema mantém obrigatórias as validações cadastrais e acadêmicas, mas não bloqueia o cadastro direto pela ausência de PDFs.
3. Sistema valida que todos os arquivos enviados são PDF, respeitam o limite de 5MB e possuem assinatura `%PDF`.
4. Sistema gera código único (`AAA1234`), verificando ledger e projeção.
5. Quando enviados, os documentos são enviados ao storage definitivo em `{codigo_academia}/estudantes/{codigo_estudante}/documentos/`.
6. Senha padrão = código do estudante (ex: `ABC1234`).
7. Estudante é criado com **status `ativo`**, vinculado à academia e com o mapa `documentos` gravado no evento `EstudanteCriadoComVinculo` e na projeção.
8. Se qualquer validação ou persistência falhar após upload parcial, o diretório de documentos do estudante é removido para evitar ficheiros órfãos.

**Regras de validação:**

- `genero` obrigatório: `masculino` ou `feminino`
- `data_nascimento` obrigatório: deve ser anterior à data atual
- JSON puro não é aceito no cadastro direto; o fluxo deve usar `multipart/form-data`, mesmo quando nenhum anexo for enviado
- `bilhete_identidade_responsavel` continua obrigatório para estudantes escolares/fundamental/médio; o PDF `bi_responsavel` é opcional no cadastro direto
- `bilhete_identidade` e `bilhete_identidade_responsavel`, quando ambos informados, não podem ser iguais após normalização
- `bi_estudante` e `cedula_estudante` são opcionais no cadastro direto; quando enviados, precisam ser PDFs válidos
- o BI do responsável não pode coincidir com o BI principal de outro estudante escolar/fundamental/médio, mas pode repetir como BI de responsável de irmãos/outros estudantes
- Certificados acadêmicos e `declaracao` são opcionais no cadastro direto; quando enviados, precisam ser PDFs válidos
- `ano_escolar_fundamental` deve seguir o formato canônico para o tipo de ensino
- Se informar `curso_medio_id`, o curso deve existir, estar ativo, pertencer à academia e ser do tipo `medio`
- Se informar `curso_superior_id`, o curso deve existir, estar ativo, pertencer à academia e ser do tipo `superior`
- Status inicial padrão para fundamental: `em_andamento`
- Status inicial padrão para médio e superior: `inativo` até eventos específicos de matrícula/curso
### Regras de Negócio — Estudante

### 6.2 Regras de Estudante

| Regra                                                         | Detalhe                                 |
| ------------------------------------------------------------- | --------------------------------------- |
| Apenas academia pode cadastrar estudantes                     | Não existe auto-cadastro                |
| `genero` e `data_nascimento` são obrigatórios                 | Não podem ser omitidos                  |
| `data_nascimento` deve ser anterior a hoje                    | Validação no aggregate                  |
| Senha padrão = código do estudante                            | Ex: `ABC1234` acede com senha `ABC1234` |
| Status superior exige fundamntal e médio finalizados/inativos | Progressão lógica do ensino             |
| Deleção de nota/falta exige motivo                            | Para auditoria                          |


### POST /academia/estudante/register

Cadastra um novo estudante vinculado à academia autenticada. O cadastro direto usa `multipart/form-data`, mas os anexos documentais são opcionais neste fluxo: a academia pode criar o estudante sem PDFs e anexá-los apenas quando estiverem disponíveis. JSON puro não é aceito. Para estudantes escolares/fundamental/médio, a criação mantém as validações cadastrais, incluindo BI textual do responsável e a regra de que esse BI não pode coincidir com o BI principal de outro estudante escolar.

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
| `bi_responsavel` | Opcional; quando enviado, deve ser PDF válido. |
| `bi_estudante` | Opcional; pode ser enviado quando `bilhete_identidade` do estudante for informado. |
| `cedula_estudante` | Opcional; pode ser enviada quando o estudante ainda não tiver BI próprio. |
| `declaracao` | Opcional; pode ser enviada como documento escolar provisório. |
| `certificado_6_ano_fundamental` | Opcional; aplicável a `7_ano_fundamental`, `8_ano_fundamental` e `9_ano_fundamental`. |
| `certificado_9_ano_fundamental` | Opcional; aplicável ao ensino médio. |
| `certificado_ensino_medio` | Opcional; aplicável ao ensino superior. |

Quando enviados, todos os ficheiros devem ter `Content-Type: application/pdf`, extensão `.pdf`, assinatura `%PDF` e tamanho máximo de 5MB. Os documentos são armazenados em `{codigo_academia}/estudantes/{codigo_estudante}/documentos/` e gravados no evento `EstudanteCriadoComVinculo` e na projeção do estudante como `documentos.<campo>.path`, `documentos.<campo>.file_url` e `documentos.<campo>.download_url`. Se a criação falhar após upload parcial, o backend remove o diretório definitivo do estudante para evitar ficheiros órfãos.

**Request — multipart/form-data (sem documentos):**

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

**Request — multipart/form-data (com documentos opcionais):**

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
declaracao=@./declaracao.pdf;type=application/pdf
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
  -F "ano_escolar_fundamental=7_ano_fundamental"
```

**Exemplo cURL com documentos opcionais:**

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
  -F "declaracao=@./declaracao.pdf;type=application/pdf"
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
  "tipo_ensino": "fundamental"
}
```

**Request — reingresso no médio:**

```json
{
  "tipo_ensino": "medio",
  "curso_medio_id": "uuid-do-curso-medio"
}
```

`curso_medio_id` é opcional no reingresso do médio. Quando omitido, o backend considera que o curso não foi alterado, usa o `curso_medio_id` anterior do estudante e mantém o mesmo nível/progressão em que ele estava.

**Request — reingresso no superior:**

```json
{
  "tipo_ensino": "superior",
  "curso_superior_id": "uuid-do-curso-superior"
}
```

`curso_superior_id` é opcional no reingresso do superior. Quando omitido, o backend considera que o curso não foi alterado, usa o `curso_superior_id` anterior do estudante e mantém o mesmo nível/progressão em que ele estava.

No reingresso/revinculação o cliente não informa ano nem semestre. O backend determina a progressão acadêmica consolidada a partir do histórico do estudante quando o curso informado é o mesmo já registrado ou quando o curso do nível médio/superior é omitido. Assim, superior retorna ao mesmo `semestre_atual`/`ano_superior`, médio retorna ao mesmo `ano_escolar_medio` e fundamental reutiliza o `ano_escolar_fundamental` anterior. Apenas mudança real de curso reinicia o vínculo atual: no superior para `semestre_atual = 1` e `ano_superior = "1_ano_superior"`; no médio para `ano_escolar_medio = "1_ano_medio"`. Eventos de trancamento, interrupção, desvinculação e reativação não apagam notas, faltas, avaliações, turmas ou demais registros históricos.


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

### Processos e Regras de Negócio — Solicitação de Matrícula

### Entidade `SolicitacaoMatricula`

A entidade representa o pedido feito pelo estudante para se matricular numa academia. Ela possui código único de 11 caracteres (`codigo_solicitacao`), dados pessoais/académicos, mapa de documentos enviados, status (`pendente`, `aprovada`, `reprovada`) e campos de decisão (`codigo_estudante_gerado`, `motivo_reprovacao`, `aprovada_por`, `reprovada_por`). Cada documento enviado é salvo como objeto com `path`, `file_url` e `download_url`, permitindo que as rotas GET retornem tanto o caminho interno quanto as URLs do arquivo e de download.

Eventos do ledger:

- `SolicitacaoMatriculaCriada`
- `SolicitacaoMatriculaAprovada`
- `SolicitacaoMatriculaReprovada`

### Processo de negócio

1. O estudante envia `POST /solicitacao-matricula` com formulário multipart e PDFs.
2. O backend valida bilhete de identidade do responsável, cédula do estudante quando necessário, data de nascimento, academia ativa, assinatura/extensão PDF, limite máximo de 5MB por ficheiro e as regras automáticas de declaração/certificados.
3. Os documentos são enviados ao storage em `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/`.
4. Para cada PDF, o storage devolve o caminho interno, a URL do arquivo (`file_url`) e a URL de download (`download_url`); esses dados são gravados no evento de criação e na projeção.
5. O aggregate `SolicitacaoMatricula` valida que `bilhete_identidade` e `bilhete_identidade_responsavel`, quando ambos informados, não sejam iguais.
6. Para estudantes escolares/fundamental/médio, o handler e o aggregate exigem `bilhete_identidade_responsavel`, `bi_responsavel`, `bi_estudante` quando houver BI próprio, `cedula_estudante` quando não houver BI próprio, e certificado aplicável ou `declaracao`.
7. Antes de criar ou aprovar a solicitação escolar, o handler confirma que o BI do responsável não pertence como BI principal a outro estudante escolar/fundamental/médio já existente.
8. O aggregate `SolicitacaoMatricula` grava o evento de criação.
9. A academia lista/consulta solicitações e aprova ou reprova.
10. Na aprovação, o sistema reutiliza o aggregate `Estudante`, revalida os documentos e conflitos atuais, e emite `EstudanteCriadoComVinculo`.
11. Na reprovação, grava o evento de reprovação e remove o diretório dos documentos.

### Regras de negócio

- O bilhete de identidade do responsável é obrigatório para estudantes escolares/fundamental/médio.
- A cédula do estudante é obrigatória quando o bilhete de identidade do estudante não for enviado.
- Certificado do 6.º ano fundamental só é aplicável para matrículas do 7.º ao 9.º ano fundamental.
- Certificado do 9.º ano fundamental só é aplicável para matrículas do ensino médio.
- Certificado do ensino médio só é aplicável para matrículas do ensino superior.
- A declaração escolar é obrigatória quando o certificado aplicável não for enviado ou quando não houver certificado aplicável ao ano académico informado.
- Arquivos devem ser PDFs (`Content-Type`, extensão e assinatura `%PDF`).
- Apenas a academia dona pode aprovar/reprovar.
- Solicitação decidida não volta para pendente.
- Rebuild inclui `solicitacoes_matricula` após as projeções principais.


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

### Processos de Negócio — Gestão de Cursos

### 5.8 Gestão de Cursos e Matérias

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

Em cursos médios, `materias_chave` é parte da configuração curricular do curso e é mantido pela rota específica `PUT /academia/curso/:id/materias-chave`, após a criação das matérias do curso. A criação e a edição cadastral do curso não aceitam esse campo. Quando a avaliação final do Médio precisa de matérias-chave e a configuração do curso/ano ainda não existe, o backend retorna erro claro de configuração ausente.

**Ciclo de vida da matéria superior:**

```
Criada (INATIVO) → Período Definido → Ativada (ATIVO) → Desativada → Deletada
```

Matérias fundamental e médio são criadas já **ativas**.

**Validação de período**: o período da matéria deve existir na lista de períodos do curso vinculado.
### Regras de Negócio — Curso

### 6.8 Regras de Curso

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

Cursos médios exigem `modelo` com valor exatamente `liceu` ou `tecnico`; cursos superiores não aceitam `modelo`, `anos_academicos` nem `materias_chave` no payload; cursos médios não aceitam `anos_academicos`, `periodos` numérico nem `materias_chave` na criação. Para curso médio, o backend deriva `anos_academicos`: `liceu` gera `1_ano_medio` a `3_ano_medio`, e `tecnico` gera `1_ano_medio` a `4_ano_medio`. Configure `materias_chave` depois de criar as matérias do curso, pela rota `PUT /academia/curso/:id/materias-chave`.


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
    "materias_chave": []
  }
}
```

**Erros:**

- `400` — nome ausente, `type` incompatível com a academia, `modelo` ausente/inválido para curso médio, `anos_academicos` enviado para curso médio, ou `materias_chave` enviado na criação
- `400` — curso superior sem `periodos`, com `periodos <= 0`, decimal, string, array, nulo, com `anos_academicos` enviado ou com `materias_chave` enviado
- `400` — curso médio com `periodos` numérico enviado
- `403` — academia inativa não pode criar cursos

---

### GET /academia/cursos

Lista todos os cursos da academia, incluindo `anos_academicos` de cada curso e, quando o curso é médio, a configuração `materias_chave` por ano acadêmico.

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

Retorna um curso específico, incluindo seus `anos_academicos` e, quando o curso é médio, `materias_chave` por ano acadêmico.

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

Atualiza apenas dados cadastrais de um curso. O `type` é imutável e esta rota não manipula anos acadêmicos, períodos, semestres nem `materias_chave`.

**Proteção**: autenticado + academia ativa

**Validações de integridade:**

- O payload aceito para esta rota permite apenas dados cadastrais como `nome`; `materias_chave` deve ser configurado pela rota própria.
- Campos acadêmicos como `anos_academicos`, `anosAcademicos`, `periodos`, `semestres`, `quantidade_semestres` e `anos` são rejeitados com erro de validação, sem mutação parcial.
- Anos de curso médio não podem ser adicionados ou removidos manualmente; eles são derivados de `modelo`. Mantenha `materias_chave` coerente com os anos derivados pela rota específica.
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
  "materias_chave": [
    {
      "ano_academico": "1_ano_medio",
      "materias_chave": ["uuid-materia-portugues"]
    },
    {
      "ano_academico": "2_ano_medio",
      "materias_chave": ["uuid-materia-fisica"]
    }
  ]
}
```

**Erros:**

- `400` — nenhum campo para atualizar
- `400` — `materias_chave` enviado nesta rota; use `PUT /academia/curso/:id/materias-chave`
- `400` — `type` enviado na edição, pois o tipo do curso é imutável
- `400` — campo acadêmico enviado (`anos_academicos`, `anosAcademicos`, `periodos`, `semestres`, `quantidade_semestres`, `anos` ou equivalente)

---

### PUT /academia/curso/:id/materias-chave

Configura, substitui ou remove a configuração de `materias_chave` de um curso médio já criado. Use esta rota somente depois de criar as matérias médias vinculadas ao curso.

**Proteção**: autenticado + academia ativa

**Request:**

```json
{
  "materias_chave": [
    {
      "ano_academico": "1_ano_medio",
      "materias_chave": ["uuid-materia-portugues"]
    },
    {
      "ano_academico": "2_ano_medio",
      "materias_chave": ["uuid-materia-fisica"]
    }
  ]
}
```

**Validações:** curso existente, ativo e da academia autenticada; curso com `type="medio"`; matérias existentes, ativas, da mesma academia, do mesmo curso médio e do ano acadêmico informado; sem anos duplicados; sem IDs duplicados no mesmo ano; sem ano vazio; sem lista vazia; todos os anos do curso médio com configuração.

**Response 200:** retorna o estado atualizado do curso.

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

### 5.8 Gestão de Cursos e Matérias

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

Em cursos médios, `materias_chave` é parte da configuração curricular do curso e é mantido pela rota específica `PUT /academia/curso/:id/materias-chave`, após a criação das matérias do curso. A criação e a edição cadastral do curso não aceitam esse campo. Quando a avaliação final do Médio precisa de matérias-chave e a configuração do curso/ano ainda não existe, o backend retorna erro claro de configuração ausente.

**Ciclo de vida da matéria superior:**

```
Criada (INATIVO) → Período Definido → Ativada (ATIVO) → Desativada → Deletada
```

Matérias fundamental e médio são criadas já **ativas**.

**Validação de período**: o período da matéria deve existir na lista de períodos do curso vinculado.
### Regras de Negócio — Matéria Disciplinar

### 6.7 Regras de Matéria Disciplinar

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

### 5.7 Gestão de Turmas

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

### 6.6 Regras de Turma

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

### 5.4 Registro de Notas

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

**Correção de nota**: `observacao` é **obrigatória** (justificativa da correção)

**Deleção de nota**: `motivo` é **obrigatório**; soft delete (permanece no ledger)

**Proteção contra duplicatas**: o aggregate mantém um mapa em memória (`NotasRegistradasPorChave`). Se a mesma combinação de academia/ano/período/matéria/tipo/categoria já existir, o comando é rejeitado com erro de negócio claro antes de tocar o banco.
### Regras de Negócio — Notas

### 6.3 Regras de Notas

| Regra                                       | Detalhe                                                       |
| ------------------------------------------- | ------------------------------------------------------------- |
| Nota deve respeitar a escala do ano acadêmico | `0–10` para `1_ano_fundamental` a `6_ano_fundamental`; `0–20` para demais anos fundamentais, médio e superior |
| Academia escola só registra notas `escolar` | Academia superior só registra `superior`                      |
| Categorias escolares são fixas | Escolas usam apenas categorias do catálogo do ano: regulares, exames nos anos previstos e `nota_pap` no `4_ano_medio` técnico |
| Categorias configuráveis são do superior | Criação/remoção de categorias é bloqueada para escolas |
| Ano do estudante deve pertencer à matéria   | Se `ano_escolar_fundamental` não estiver em `anos_academicos`, bloqueia   |
| Observação obrigatória na correção          | Justificativa da alteração                                    |
| Motivo obrigatório na deleção               | Para auditoria                                                |
| Duplicata bloqueada no aggregate            | Mesma combinação ano/período/matéria/tipo/categoria rejeitada |
| Nota deletada não pode ser re-registada     | Mapa de chaves não remove entradas deletadas                  |


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

O recurso de sumários/aulas foi removido do contrato público da API. Não há endpoints para criar, listar, consultar, atualizar ou remover sumários, e faltas não aceitam nem retornam vínculo com sumário.

## 14. Faltas

### Processos de Negócio — Registro de Faltas

### 5.5 Registro de Faltas

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

**Correção de falta**: `observacao` é **obrigatória** (justificativa da correção).

**Deleção de falta**: `motivo` é **obrigatório**; soft delete
### Regras de Negócio — Faltas

### 6.4 Regras de Faltas

| Regra                                            | Detalhe                                                                                 |
| ------------------------------------------------ | --------------------------------------------------------------------------------------- |
| Quantidade deve ser 1 ou mais                    | Validação no handler e no aggregate                                                     |
| Data no formato date (`AAAA-MM-DD`)              | Campo date-only em faltas (sem hora)                                                    |
| Ano do estudante deve pertencer à matéria        | Se `ano_escolar_fundamental` do estudante não existir em `anos_academicos` da matéria, bloqueia    |
| Observação obrigatória na correção               | Justificativa da alteração em `PUT /academia/atualizar-falta`                           |
| Motivo obrigatório na deleção                    | Para auditoria no ledger e na projeção                                                  |
| Duplicata bloqueada                              | Mesma combinação `data + codigo_estudante + materia_disciplinar_id` é rejeitada         |
| Sem vínculo de sumário                           | Faltas são independentes e não aceitam `sumario_id` ou `sumario_titulo` |

### 6.4.1 Remoção de Sumários/Aulas

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

### Processos de Negócio — Avaliação Final de Ano Académico

### 5.6 Avaliação Final de Ano Académico

**Quem faz**: Academia ativa, com ano letivo configurado, por meio da configuração de regras e do lançamento de notas. A academia **não envia manualmente** a nota final calculada nem decide aprovação/reprovação no payload de execução.

A avaliação final é automática, auditável e orientada por regras. Ela é disparada pelo fluxo de lançamento de notas quando o backend identifica que existem regras ativas e notas suficientes para calcular a etapa aplicável. O modelo atual **não é uma média global única do estudante**: o backend calcula uma `nota_final` independente para cada matéria disciplinar aplicável, registra resultados por matéria e deriva a decisão geral do conjunto de resultados, da cadeia de regras e, apenas para Superior, das regras de pendência.

#### 5.6.1 Conceitos funcionais

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
| `materias_chave` | Configuração curricular do curso médio, por `ano_academico`; não pertence à regra. Na raiz do Médio, o backend resolve a lista pelo `curso_medio_id` do estudante e pelo `ano_escolar_medio` atual. Reprovação em matéria não-chave na raiz não impede aprovação direta; reprovação em matéria-chave impede e pode acionar cadeia/pendência. |
| `materias_aplicaveis` | Lista opcional de matérias que restringe a execução da regra. É especialmente útil em descendentes para recalcular apenas matérias de recuperação/exame/recurso. |
| `limite_materias_pendentes` | Campo configurável apenas no Superior. Em escolas, regras/limites finais são fixos do sistema e não são cadastrados pela academia. |
| `pendencia_permitida` | Campo exclusivo de matéria do Superior. Somente matérias superiores com esse campo verdadeiro podem gerar aprovação com pendência. |
| `pendencia_nivel_conclusao` | Campo exclusivo de matéria do Superior que indica o semestre de conclusão usado para bloqueio funcional de progressão/conclusão quando há pendência aberta. |
| Matéria pendente | Registro persistente em `projection_materias_pendentes`, criado quando uma avaliação do Superior aprova com pendência. Mantém histórico aberto/baixado por estudante, matéria, curso, ano letivo e escopo. |

A avaliação registrada é idempotente no escopo suportado: o sistema evita gravar duas avaliações com o mesmo estudante, academia, ano letivo, nível interno da avaliação, ano/período acadêmico atual e `type`. Eventos e projeções preservam snapshots de fórmula, regra, matérias e pendências suficientes para auditoria.

#### 5.6.2 Montagem e criação de regras de avaliação final

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
| `materias_chave` | Rejeitado | Rejeitado na regra; configurado no curso médio por rota específica após a criação das matérias | Rejeitado |
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
- Na edição, não é permitido alterar `type`, `nivel`, `anos_academicos`, dependência, `materias_chave`, `materias_aplicaveis`, limite, status ou version; para mudar escopo/cadeia, cria-se nova regra. Se `materias_chave` for enviado em criação ou edição de regra, o backend rejeita e orienta configurar as matérias-chave no curso médio, por `ano_academico`.

**Exemplos funcionais de configuração:**

| Cenário | Configuração típica |
|---|---|
| Raiz do Fundamental | Não configurável por endpoint na versão 2.1.0; o padrão escolar fundamental é fixo do sistema e exposto como regra `source="system"`, `fixed=true`, `readonly=true`. |
| Recuperação do Fundamental | Não configurável por endpoint; o sistema fornece as etapas fixas com `exame_final` e `exame_recurso` para os anos oficiais. |
| Raiz do Médio | Não configurável por endpoint na versão 2.1.0; o padrão escolar médio é fixo do sistema. Matérias-chave continuam sendo configuração curricular do curso médio/ano, não da regra. |
| Descendente do Médio | Não configurável por endpoint; o sistema fornece `exame_recurso` fixo para `6_ano_fundamental`, `9_ano_fundamental` e `3_ano_medio`, limitado às matérias reprovadas na avaliação final. |
| Raiz do Superior | `nivel=superior`, sem `anos_academicos`, fórmula com referências `[categoria]`, `limite_materias_pendentes` definido. |
| Superior com pendência | Mesma regra superior, matérias com `pendencia_permitida=true` e, quando aplicável, `pendencia_nivel_conclusao` indicando o semestre-limite. |

#### 5.6.3 Fórmulas por nível

A regra usa `formula` como texto declarativo. O parser valida referências, operadores, parênteses, constantes, categorias e períodos antes de persistir ou calcular. Erro de fórmula é erro de validação, não falha operacional inesperada.

| Nível | Formato da referência | Exemplo válido | Exemplo inválido |
|---|---|---|---|
| Fundamental | `[categoria,periodo]` | `([prova_trimestral,1_trimestre]+[prova_trimestral,2_trimestre]+[prova_trimestral,3_trimestre])/3` | `[prova_trimestral]` |
| Médio | `[categoria,periodo]` | `[prova_trimestral,1_trimestre]*0.4+[exame_final,3_trimestre]*0.6` | `[exame_final]` |
| Superior | `[categoria]`; período inferido pela matéria/semestre avaliado | `([prova_parcelar_1]+[prova_parcelar_2])/2` | `[prova_parcelar_1,1_semestre]` |

No Superior, o backend preenche o período no momento do cálculo usando a matéria/escopo avaliado (`periodo` da matéria e `semestre_atual` do estudante). Assim, a mesma regra superior pode calcular as matérias do semestre atual sem expor período explícito no payload da regra.

Se a fórmula exigir nota que ainda não existe para determinada matéria, categoria e período, aquela execução não fecha a avaliação naquele momento; o lançamento de novas notas tentará novamente. A fórmula sempre lê notas do ano letivo atual, da mesma academia, do mesmo estudante, da matéria avaliada e de categorias extraídas da própria fórmula.

#### 5.6.4 Execução automática por lançamento de notas

1. A academia registra/atualiza nota; o backend valida ano letivo, estudante, matéria, categoria, período, escala numérica e pertencimento ao `ano_escolar_fundamental` ou `ano_escolar_medio` atual do estudante.
2. O backend infere o nível acadêmico do estudante para execução: Superior tem prioridade quando há vínculo/status superior; depois Médio; caso contrário Fundamental.
3. Para Superior, o backend transforma `semestre_atual` em `[n]_semestre` e valida esse período contra o curso.
4. O backend busca regras aplicáveis à academia, ao `nivel` e ao escopo acadêmico atual. Para `fundamental` e `medio`, essa busca é sempre resolvida pelo catálogo fixo do sistema; se a categoria lançada não despertar uma regra fixa, a execução termina sem consultar regras configuráveis/legadas. Para `superior`, a busca usa as regras configuráveis ativas da academia.
5. A execução automática da raiz continua quando a categoria da nota registrada/atualizada é a `nota_despertadora` da raiz. No padrão escolar fixo, isso significa `prova_trimestral` no 3º trimestre para anos regulares, `exame_final` para anos com exame e `nota_pap` no `4_ano_medio` técnico.
6. Descendentes só são consideradas se a etapa anterior reprovou. A descendente escolar fixa `exame_recurso` também pode ser despertada diretamente por lançamento/atualização de `exame_recurso`, mas somente para matérias reprovadas na avaliação final anterior e quando todas as notas de recurso exigidas estiverem completas.
7. Para cada regra executável, o backend resolve as matérias aplicáveis e calcula `nota_final` individual por matéria.
8. O resultado de cada matéria compara `nota_final` com `nota_minima_aprovacao`.
9. No modelo superior configurável legado não escolar, o backend pode resolver `materias_chave` do curso médio quando a regra não é fixa. No padrão escolar fixo atual, o Médio é decidido pelas regras oficiais por matéria e pelo `exame_recurso`, sem pendência escolar.
10. A decisão geral é derivada dos resultados por matéria e, no Superior, das condições de pendência.
11. O evento grava snapshots: regra, `type`, fórmula usada, nota mínima, curso usado, matérias-chave resolvidas, resultados por matéria, pendências geradas, turma atual/turmas removidas quando aplicável e dados de progressão.
12. Se a avaliação já existir no escopo idempotente, o backend não duplica o registro.

#### 5.6.5 Fundamental

- O escopo é `ano_escolar_fundamental` atual do estudante (`1_ano_fundamental` a `9_ano_fundamental`).
- O catálogo avaliativo fundamental é fixo do sistema; rotas configuráveis de regras não aceitam criação/edição/remoção para Fundamental. Regras superiores não aceitam `anos_academicos`.
- O backend avalia cada matéria fundamental ativa aplicável ao ano do estudante, respeitando `materias_aplicaveis` se configurado.
- Cada matéria recebe `nota_final` própria; aprovação direta exige que todas as matérias avaliadas atinjam a mínima.
- Uma ou mais matérias abaixo da mínima reprovam a etapa e podem acionar regra descendente por matéria reprovada.
- Fundamental não permite aprovação com pendência: regra fundamental não tem `limite_materias_pendentes` e matérias fundamentais não aceitam `pendencia_permitida`/`pendencia_nivel_conclusao`.
- Aprovado em ano intermediário progride para o próximo ano fundamental. Se a academia não oferta o próximo ano, o evento registra o motivo `academia_sem_oferta_do_proximo_ano_academico_fundamental`, mantém o ciclo em andamento e não adiciona turma automaticamente.
- Aprovado no `9_ano_fundamental` finaliza o ciclo fundamental. Reprovado permanece no mesmo ano.

#### 5.6.6 Médio

- O escopo é o `ano_escolar_medio` atual do estudante, validado contra o curso médio ativo vinculado.
- O backend avalia matérias médias ativas do curso e ano atual conforme o padrão fixo escolar.
- A regra escolar fixa de Médio não aceita `materias_chave`; essa configuração curricular do curso médio permanece disponível para outros fluxos/consultas, mas não substitui a decisão oficial por matéria do padrão avaliativo fixo.
- O `exame_recurso` fixo recalcula apenas matérias reprovadas na `avaliacao_final` anterior; matéria aprovada não pode receber recurso.
- Médio escolar não permite matérias dependentes nem aprovação com pendência: `pendencia_permitida` e `pendencia_nivel_conclusao` são exclusivos do Superior.
- O `4_ano_medio` técnico usa apenas `nota_pap` (`Prova de Aptidão Profissional`) como avaliação final, com aprovação por `nota_pap >= 10`, sem trimestres, prova trimestral, exame final ou recurso.

Cenários típicos do Médio:

| Cenário | Resultado funcional |
|---|---|
| Todas as matérias-chave aprovadas | Aprovação direta na raiz, com progressão ou conclusão conforme ano do curso. |
| Matéria não-chave reprovada na raiz | Resultado por matéria fica reprovado, mas a decisão geral da raiz pode permanecer aprovada conforme comportamento atual. |
| Matéria-chave reprovada e aprovada em descendente | A cadeia registra a nova etapa e a aprovação da descendente permite progressão/conclusão. |
| Matéria reprovada sem descendente aplicável | Reprovação no ano/etapa escolar conforme padrão fixo. |
| `4_ano_medio` técnico com `nota_pap >= 10` | Aprovação e conclusão do médio técnico. |
| `4_ano_medio` técnico com `nota_pap < 10` | Reprovação no ano final técnico. |

#### 5.6.7 Superior

- O escopo é o curso superior ativo e o `semestre_atual` do estudante, convertido para `1_semestre`, `2_semestre`, etc.
- O backend avalia matérias superiores ativas do curso cujo `periodo` corresponde ao semestre atual.
- Fórmulas superiores não declaram período; o período é preenchido automaticamente para cada matéria avaliada.
- Aprovação direta exige todas as matérias avaliadas com nota final maior ou igual à mínima.
- Reprovação em matéria aciona descendentes aplicáveis; descendentes também trabalham por matéria e podem ser restringidas por `materias_aplicaveis`.
- Após esgotar a cadeia superior, o estudante pode aprovar com pendência se o total de reprovações couber em `limite_materias_pendentes` e todas as matérias reprovadas permitirem pendência.
- Reprovação por limite excedido ou matéria sem pendência permitida mantém o estudante no mesmo `semestre_atual` e não altera o status superior.
- Aprovação em semestre intermediário incrementa `semestre_atual` e recalcula `ano_superior`; aprovação no último semestre finaliza o ciclo superior.
- Pendência de curso anterior permanece histórica e não bloqueia o curso atual.

#### 5.6.8 Regras descendentes por matéria

Regra descendente é qualquer regra com `aplica_se_reprovado_em_type`. Ela representa uma etapa posterior da cadeia e só roda quando a etapa ascendente indicada reprovou. A descendente herda a lógica por matéria: calcula notas para matérias aplicáveis, compara cada resultado com a mínima e grava `type`/regra/fórmula usados naquela etapa.

Pontos importantes:

- A descendente não é uma média global do estudante; ela recalcula matérias no escopo da regra.
- `materias_aplicaveis` funciona como filtro: matéria fora da lista não é recalculada naquela etapa.
- A cadeia termina quando não há descendente ativa aplicável, quando a etapa anterior aprovou ou quando faltam notas para calcular a próxima etapa.
- Ao final da última etapa reprovada, apenas o Superior avalia se a reprovação vira pendência; Fundamental e Médio escolar permanecem reprovados quando não atendem ao padrão fixo.
- Exemplo: raiz `avaliacao_final` reprova Matemática e Física; descendente `avaliacao_final_com_exame` com `materias_aplicaveis=[Matemática]` recalcula somente Matemática. Física continua com o resultado anterior para a decisão final/pendência.

#### 5.6.9 Resultados por matéria, eventos, projeções e auditoria

Cada avaliação final gravada deve ser explicada pelos itens de `resultados_materias`, não por média global única. Cada item contém, no mínimo, `materia_id`, `nota_final`, `aprovado`, `type`, `formula_snapshot`, `regra_avaliacao_final_id` e `pendencia_permitida`. A projeção também mantém `nota_final` agregada como média dos itens calculados para compatibilidade/consulta resumida, mas a decisão funcional é por matéria.

Eventos `AvaliacaoFinalEscolar` e `AvaliacaoFinalSuperior` preservam snapshots de regra, fórmula, notas calculadas, progressão e pendências geradas. Alterações posteriores de regra, matéria ou nota não reescrevem silenciosamente decisões já registradas; ajustes exigem fluxo operacional próprio/rebuild controlado.

#### 5.6.10 Pendências de matérias

Pendências existem apenas para o Superior. Elas são consideradas depois de reprovação na cadeia aplicável e só são criadas quando a decisão final superior é aprovação com pendência. Se o estudante reprova totalmente, nenhuma nova pendência é criada.

A pendência carrega funcionalmente: estudante, matéria, academia, curso, `nivel`, ano letivo, escopo acadêmico (`periodo_superior`), regra/evento de origem, status `pendente`, dados de origem/snapshot e timestamps. Há proteção contra duplicidade aberta no mesmo estudante, matéria, curso, nível, ano letivo e escopo. A estrutura também possui campos de baixa (`baixada_por_event_id`, `updated_at`) para histórico, mas a documentação funcional reconhece uma limitação atual: **não há rota pública consolidada de regularização/baixa de pendência exposta nesta documentação de API**. Portanto, o sistema já persiste e consulta a base de pendências abertas/históricas, mas a regularização operacional precisa ser implementada ou conduzida por fluxo administrativo/evento específico antes de ser tratada como rotina pública.

#### 5.6.11 Bloqueio por `pendencia_nivel_conclusao` e regularização

`pendencia_nivel_conclusao` pertence à matéria e deve ser usado para identificar pendências bloqueantes do curso atual. Funcionalmente:

- No Superior, pendência aberta cujo limite coincide com semestre/período conclusivo bloqueia conclusão automática até baixa.
- Aprovação com pendência pode permitir progressão intermediária, mas não deve permitir conclusão com pendência bloqueante do curso atual.
- Pendências não bloqueantes permitem progressão conforme regra de avaliação, desde que pertençam a escopo anterior e dentro do limite funcional definido.
- Pendências de curso anterior são históricas e não bloqueiam o curso atual.
- Regularização de pendência é diferente de avaliação final normal: deve avaliar a matéria pendente, registrar evento próprio auditável, baixar a pendência se aprovada e manter aberta se reprovada. Como limitação atual, esse fluxo ainda não está exposto como endpoint público completo; ao ser implementado, deve reutilizar os dados de origem da pendência e retomar progressão/conclusão quando não restarem pendências relevantes abertas.

#### 5.6.12 Cenários de erro e validação

Devem falhar com erro de validação ou bloqueio funcional:

- Payload de regra com `tipo_ensino`; use `nivel`.
- Academia mista criando regra sem `nivel` ou tentando criar regra `superior`.
- Academia não mista criando regra de nível incompatível com sua configuração.
- `anos_academicos` ausente em regra fundamental ou presente em Médio/Superior.
- `limite_materias_pendentes` enviado em regra escolar, ausente em regra superior ou negativo.
- `materias_chave` enviado em qualquer regra de avaliação final; o campo pertence ao curso médio, não à regra.
- Curso médio sem `materias_chave` para todo ano acadêmico do curso, ou com matérias-chave vazias, duplicadas, inativas, inexistentes, de outra academia, de outro curso, de outro nível ou fora do ano configurado.
- `materias_aplicaveis` fora do escopo do curso/ano/período aplicável deve ser tratada como configuração inválida ou ineficaz operacionalmente; QA deve validar esse cenário contra a base de matérias da academia.
- Descendente órfã, descendente que aponta para si mesma, ciclo de dependências ou escopo fundamental diferente da raiz.
- Fórmula Fundamental/Médio sem período explícito (`[categoria]`).
- Fórmula Superior com período explícito (`[categoria,periodo]`).
- Fórmula com categoria inexistente, período inválido, divisão por zero, caracteres fora da gramática ou categorias enviadas que não batem com a fórmula.
- Tentativa de criar pendência em matéria fundamental.
- Tentativa de criar duplicidade de pendência aberta no mesmo escopo.
- Tentativa de concluir/progredir em desacordo com pendência bloqueante do curso atual.

#### 5.6.13 Consultas

- `GET /avaliacoes` → registros de avaliação final, com filtros por `nivel`, ano letivo, ano/período acadêmico atual, turma, academia e `type`. O filtro legado `tipo_ensino` é rejeitado no handler atual.
- `GET /aprovacoes` → apenas aprovados (`aprovado = TRUE`) com os mesmos filtros.
- `GET /reprovacoes` → reprovações definitivas; reprovações intermediárias com descendente ativa posterior não aparecem como definitivas até a cadeia terminar.
- `GET /academia/avaliacao-final/regras` → lista regras da academia autenticada.
- `PUT /academia/avaliacao-final/regras/:id` → edita apenas campos seguros de apresentação/cálculo (`nome`, `descricao`, `nota_minima_aprovacao`, `formula`).
- `DELETE /academia/avaliacao-final/regras/:id` → inativa a regra e suas dependentes em cascata.

**Escopo por academia:** usuário autenticado como academia só consulta/gerencia dados da própria academia. Admin pode consultar de forma ampla com filtros.
### Regras de Negócio — Avaliação Final

### 6.5 Regras de Avaliação Final

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
- Ao registrar/atualizar notas, o backend identifica o estudante, infere o `tipo_ensino`, descobre o ano acadêmico atual e busca todas as regras ativas aplicáveis àquele ano.
- A cadeia precisa ter exatamente uma regra raiz, isto é, a regra sem `aplica_se_reprovado_em_type`. A raiz pode declarar `nota_despertadora`, cujo valor é o `codigo` da categoria de nota que autoriza o disparo automático.
- O processamento automático começa na raiz somente quando a categoria da nota registrada/atualizada é igual a `nota_despertadora`. Regras antigas sem esse campo não despertam automaticamente por nota.
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
- `materias_chave` — não é aceito em regras de avaliação final. Para Médio, configure matérias-chave no curso médio, por `ano_academico`. Payloads de criação/edição de regra contendo esse campo retornam erro de validação.
- `materias_aplicaveis` — opcional para Superior; usa itens `{curso_id, ano_academico, materias}`. IDs duplicados no mesmo item e itens duplicados por par curso+ano são inválidos. Escolas não configuram regras por esta rota.
- `limite_materias_pendentes` — obrigatório para `nivel="superior"`; inteiro maior ou igual a zero.
- `nota_minima_aprovacao` — obrigatório e maior que zero.
- `categorias_envolvidas` — opcional. O backend extrai automaticamente as categorias usadas em `formula`. Se enviado, deve corresponder exatamente às categorias extraídas da fórmula, sem duplicatas, sobras ou omissões, e todas precisam estar ativas/configuradas pela academia para os anos da regra.
- `formula` — obrigatório; deve ser uma string textual no modelo `formula_textual_v1`. O formato JSON em árvore antigo foi removido e não é aceito.
- `nota_despertadora` — exclusivo de regra raiz. Quando informado, deve ser o `codigo` de uma categoria de nota ativa, não deletada, pertencente à academia autenticada e configurada para os anos/escopo da regra quando essa segmentação existir. O campo é opcional para compatibilidade: raiz sem `nota_despertadora` não dispara automaticamente por lançamento/atualização de nota.
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
- Não é permitido editar `type`, `nivel`, `anos_academicos`, `aplica_se_reprovado_em_type`, `materias_chave`, `materias_aplicaveis`, `limite_materias_pendentes`, `status` nem `version` via payload, para não quebrar a cadeia já configurada. O campo legado `tipo_ensino` também é rejeitado. Caso uma versão futura permita editar `type`, a validação deve seguir a mesma regra da criação: o `type` pode coincidir com regra inativa, mas a regra inativa permanecerá bloqueada para ativação enquanto houver regra ativa conflitante.
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
- `materias_chave` não pertence à regra. No Médio, o backend resolve as matérias-chave a partir do curso médio do estudante (`curso_medio_id`) e do `ano_escolar_medio` atual; se a configuração do curso/ano estiver ausente, a avaliação falha com erro claro.
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

**Exemplo inválido — `materias_chave` na regra:**

```json
{
  "type": "normal",
  "nome": "Fechamento anual do médio",
  "nivel": "medio",
  "materias_chave": ["b7f7b4d7-5d1e-4d1a-98ea-6a4a7b79b7c0"],
  "limite_materias_pendentes": 2,
  "nota_minima_aprovacao": 10,
  "formula": "([prova,1_trimestre]+[prova,2_trimestre]+[prova,3_trimestre])/3"
}
```

Esse payload é rejeitado; configure `materias_chave` no curso médio, por `ano_academico`.

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

### Processos de Negócio — Administração e Integridade

### 5.9 Verificação de Integridade do Ledger

O sistema suporta verificação da cadeia de hashes do ledger para qualquer estudante:

```
GET /verificar-integridade/:codigo
```

A função SQL `verify_hash_chain` verifica se todos os hashes encadeados são válidos. Se qualquer evento foi adulterado, a verificação retorna `integro = false` indicando a versão onde a cadeia foi quebrada.

---

### 5.10 Rebuild de Projeções

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

### 6.9 Regras de Admin

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

## 17. Jobs Assíncronos

### Processos de Negócio — Sistema de Jobs Assíncronos

### 5.11 Sistema de Jobs Assíncronos

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

### 9.1 Batch Assíncrono

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

## 19. Armazenamento

### Processos e Regras de Negócio — Armazenamento de Arquivos

### Armazenamento de arquivos (Google Drive)

O backend usa a biblioteca oficial `google.golang.org/api/drive/v3` integrada com `golang.org/x/oauth2/google` para autenticar-se como service account. A autenticação é feita a partir do ficheiro JSON da service account (`GOOGLE_DRIVE_CREDENTIALS_PATH` ou `GOOGLE_DRIVE_CREDENTIALS_JSON` em base64), com renovação automática de tokens OAuth — sem necessidade de gestão manual de tokens.

Configuração de produção:

- `GOOGLE_DRIVE_CREDENTIALS_PATH`: caminho para o ficheiro JSON da service account.
- `GOOGLE_DRIVE_CREDENTIALS_JSON`: alternativa em base64 para ambientes sem disco persistente.
- `GOOGLE_DRIVE_ROOT_FOLDER_ID`: ID da pasta raiz no Drive partilhada com a service account.

Remover: `GOOGLE_DRIVE_ACCESS_TOKEN` (deixa de existir).

Configuração local/teste:

- `GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true`: habilita o provider local sem chamar Google Drive.
- `GOOGLE_DRIVE_LOCAL_ROOT`: diretório local usado para simular o Drive (padrão `data/google_drive_storage`).

Os documentos de matrícula continuam sendo gravados em `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/`. `EnsureDir` cria a hierarquia de pastas no Drive verificando cada nível antes de criá-lo, `Upload` envia PDFs para a pasta pai resolvida e retorna metadados do arquivo armazenado. No Google Drive, esses metadados vêm de `webViewLink` (`file_url`) e `webContentLink` (`download_url`), além do `path` interno; no provider local de teste, as URLs usam `file://`. `Delete` remove arquivos ou diretórios resolvendo o caminho dentro da pasta raiz configurada. `GetQuota` não tenta mais estimar consumo fora da pasta raiz compartilhada/gerenciada: ele lista recursivamente apenas essa pasta, define `total_bytes`/`used_bytes` como a soma real dos arquivos existentes nela, preenche `academias`/`managed_bytes` com arquivos dentro dos diretórios de academia e preenche `outside_academias_bytes` com arquivos que estão na raiz ou fora de diretórios de academia. `unmanaged_bytes` fica reservado para compatibilidade e não representa mais consumo externo à pasta raiz. Falhas de configuração retornam mensagens operacionais explícitas para ausência de credenciais, ausência de `GOOGLE_DRIVE_ROOT_FOLDER_ID`, credencial inválida e quota indisponível sem credenciais/estimativa local.
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
