package domain

import (
	"time"

	"github.com/google/uuid"
)

// Academia representa uma escola ou universidade
type Academia struct {
	ID             uuid.UUID `json:"id" db:"id"`
	Type           string    `json:"type" db:"type"`
	Nome           string    `json:"nome" db:"nome"`
	CodigoAcademia string    `json:"codigo_academia" db:"codigo_academia"`
	SenhaHash      string    `json:"-" db:"senha_hash"`
	Endereco       string    `json:"endereco" db:"endereco"`
	NumeroTelefone *string   `json:"numero_telefone,omitempty" db:"numero_telefone"`
	Email          *string   `json:"email,omitempty" db:"email"`
	Website        *string   `json:"website,omitempty" db:"website"`
	NivelEscolar   *string   `json:"nivel_escolar,omitempty" db:"nivel_escolar"`
	Status         string    `json:"status" db:"status"`
	Cursos         []string  `json:"cursos" db:"cursos"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// Estudante representa um estudante
type Estudante struct {
	ID                         uuid.UUID  `json:"id" db:"id"`
	Nome                       string     `json:"nome" db:"nome"`
	SenhaHash                  string     `json:"-" db:"senha_hash"`
	BilheteIdentidade          *string    `json:"bilhete_identidade,omitempty" db:"bilhete_identidade"`
	BilheteIdentidadeResp      *string    `json:"bilhete_identidade_responsavel,omitempty" db:"bilhete_identidade_responsavel"`
	IDAcademia                 *uuid.UUID `json:"id_academia,omitempty" db:"id_academia"`
	AnoEscolar                 *string    `json:"ano_escolar,omitempty" db:"ano_escolar"`
	AnoSuperior                *string    `json:"ano_superior,omitempty" db:"ano_superior"`
	CursoMedio                 *string    `json:"curso_medio,omitempty" db:"curso_medio"`
	CursoSuperior              *string    `json:"curso_superior,omitempty" db:"curso_superior"`
	StatusEscolar              *string    `json:"status_escolar,omitempty" db:"status_escolar"`
	StatusSuperior             *string    `json:"status_superior,omitempty" db:"status_superior"`
	CreatedAt                  time.Time  `json:"created_at" db:"created_at"`
}

// Curso representa um curso
type Curso struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Nome      string    `json:"nome" db:"nome"`
	Type      string    `json:"type" db:"type"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Inscricao representa uma solicitação de inscrição
type Inscricao struct {
	ID           uuid.UUID `json:"id" db:"id"`
	EstudanteID  uuid.UUID `json:"estudante_id" db:"estudante_id"`
	AcademiaID   uuid.UUID `json:"academia_id" db:"academia_id"`
	Tipo         string    `json:"tipo" db:"tipo"`
	AnoInscricao string    `json:"ano_inscricao" db:"ano_inscricao"`
	Curso        *string   `json:"curso,omitempty" db:"curso"`
	Status       string    `json:"status" db:"status"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// RegistroNotas representa o registro de notas de um estudante
type RegistroNotas struct {
	ID          uuid.UUID              `json:"id" db:"id"`
	EstudanteID uuid.UUID              `json:"estudante_id" db:"estudante_id"`
	IDAcademia  uuid.UUID              `json:"id_academia" db:"id_academia"`
	AnoLectivo  string                 `json:"ano_lectivo" db:"ano_lectivo"`
	Periodo     string                 `json:"periodo" db:"periodo"`
	Materias    []Materia              `json:"materias" db:"materias"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	EventID     *uuid.UUID             `json:"event_id,omitempty" db:"event_id"`
}

// RegistroFaltas representa o registro de faltas de um estudante
type RegistroFaltas struct {
	ID          uuid.UUID              `json:"id" db:"id"`
	EstudanteID uuid.UUID              `json:"estudante_id" db:"estudante_id"`
	IDAcademia  uuid.UUID              `json:"id_academia" db:"id_academia"`
	AnoLectivo  string                 `json:"ano_lectivo" db:"ano_lectivo"`
	Periodo     string                 `json:"periodo" db:"periodo"`
	Materias    []MateriaFaltas        `json:"materias" db:"materias"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	EventID     *uuid.UUID             `json:"event_id,omitempty" db:"event_id"`
}

// Materia representa uma matéria com nota
type Materia struct {
	Nome string  `json:"nome"`
	Nota float64 `json:"nota"`
}

// MateriaFaltas representa uma matéria com faltas
type MateriaFaltas struct {
	Nome   string `json:"nome"`
	Faltas int    `json:"faltas"`
}

// LoginRequest representa uma requisição de login
type LoginRequest struct {
	Usuario string `json:"usuario" binding:"required"`
	Senha   string `json:"senha" binding:"required"`
	Type    string `json:"type" binding:"required"`
}

// LoginResponse representa a resposta de login
type LoginResponse struct {
	Token string    `json:"token"`
	ID    uuid.UUID `json:"id"`
	Nome  string    `json:"nome"`
	Type  string    `json:"type"`
}

// RegisterAcademiaRequest representa uma requisição de registro de academia
type RegisterAcademiaRequest struct {
	Type           string   `json:"type" binding:"required"`
	Senha          string   `json:"senha" binding:"required"`
	Nome           string   `json:"nome" binding:"required"`
	Endereco       string   `json:"endereco" binding:"required"`
	NumeroTelefone *string  `json:"numero_telefone"`
	Email          *string  `json:"email"`
	Website        *string  `json:"website"`
	NivelEscolar   *string  `json:"nivel_escolar"`
	Cursos         []string `json:"cursos"`
}

// RegisterEstudanteRequest representa uma requisição de registro de estudante
type RegisterEstudanteRequest struct {
	Senha                 string  `json:"senha" binding:"required"`
	Nome                  string  `json:"nome" binding:"required"`
	BilheteIdentidade     *string `json:"bilhete_identidade"`
	BilheteIdentidadeResp *string `json:"bilhete_identidade_responsavel"`
	AnoEscolar            *string `json:"ano_escolar"`
	AnoSuperior           *string `json:"ano_superior"`
	CursoMedio            *string `json:"curso_medio"`
	CursoSuperior         *string `json:"curso_superior"`
	StatusEscolar         *string `json:"status_escolar"`
	StatusSuperior        *string `json:"status_superior"`
}

// InscricaoEscolaRequest representa uma solicitação de inscrição em escola
type InscricaoEscolaRequest struct {
	IDAcademia           uuid.UUID `json:"id_academia" binding:"required"`
	AnoEscolarInscricao  string    `json:"ano_escolar_inscricao" binding:"required"`
	CursoMedio           *string   `json:"curso_medio"`
}

// InscricaoUniversidadeRequest representa uma solicitação de inscrição em universidade
type InscricaoUniversidadeRequest struct {
	IDAcademia            uuid.UUID `json:"id_academia" binding:"required"`
	AnoSuperiorInscricao  string    `json:"ano_superior_inscricao" binding:"required"`
	CursoSuperior         string    `json:"curso_superior" binding:"required"`
}

// RegistrarNotasRequest representa uma requisição de registro de notas
type RegistrarNotasRequest struct {
	EstudanteID uuid.UUID `json:"estudante_id" binding:"required"`
	AnoLectivo  string    `json:"ano_lectivo" binding:"required"`
	Periodo     string    `json:"periodo" binding:"required"`
	Materias    []Materia `json:"materias" binding:"required"`
}

// RegistrarFaltasRequest representa uma requisição de registro de faltas
type RegistrarFaltasRequest struct {
	EstudanteID uuid.UUID       `json:"estudante_id" binding:"required"`
	AnoLectivo  string          `json:"ano_lectivo" binding:"required"`
	Periodo     string          `json:"periodo" binding:"required"`
	Materias    []MateriaFaltas `json:"materias" binding:"required"`
}

// ErrorResponse representa uma resposta de erro
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse representa uma resposta de sucesso genérica
type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}