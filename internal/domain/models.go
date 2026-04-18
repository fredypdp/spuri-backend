package domain

import (
	"time"

	"github.com/google/uuid"
)

type Academia struct {
	ID             uuid.UUID `json:"id" db:"id"`
	Nivel          string    `json:"nivel" db:"nivel"`
	Type           string    `json:"type" db:"type"`
	Nome           string    `json:"nome" db:"nome"`
	CodigoAcademia string    `json:"codigo_academia" db:"codigo_academia"`
	SenhaHash      string    `json:"-" db:"senha_hash"`
	Provincia      string    `json:"provincia" db:"provincia"`
	Endereco       string    `json:"endereco" db:"endereco"`
	NumeroTelefone *string   `json:"numero_telefone,omitempty" db:"numero_telefone"`
	Email          *string   `json:"email,omitempty" db:"email"`
	Website        *string   `json:"website,omitempty" db:"website"`
	NivelEscolar   *string   `json:"nivel_escolar,omitempty" db:"nivel_escolar"`
	Status         string    `json:"status" db:"status"`
	Cursos         []string  `json:"cursos" db:"cursos"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// Estudante — genero e data_nascimento são sempre preenchidos (obrigatórios).
type Estudante struct {
	ID                       uuid.UUID  `json:"id" db:"id"`
	Nome                     string     `json:"nome" db:"nome"`
	CodigoEstudante          string     `json:"codigo_estudante" db:"codigo_estudante"`
	SenhaHash                string     `json:"-" db:"senha_hash"`
	BilheteIdentidade        *string    `json:"bilhete_identidade,omitempty" db:"bilhete_identidade"`
	BilheteIdentidadeResp    *string    `json:"bilhete_identidade_responsavel,omitempty" db:"bilhete_identidade_responsavel"`
	Genero                   string     `json:"genero" db:"genero"`
	DataNascimento           time.Time  `json:"data_nascimento" db:"data_nascimento"`
	CodigoAcademia           *string    `json:"codigo_academia,omitempty" db:"codigo_academia"`
	AnoEscolar               *string    `json:"ano_escolar,omitempty" db:"ano_escolar"`
	AnoEscolarMedio          *string    `json:"ano_escolar_medio,omitempty" db:"ano_escolar_medio"`
	AnoSuperior              *string    `json:"ano_superior,omitempty" db:"ano_superior"`
	CursoMedioID             *uuid.UUID `json:"curso_medio_id,omitempty" db:"curso_medio_id"`
	CursoSuperiorID          *uuid.UUID `json:"curso_superior_id,omitempty" db:"curso_superior_id"`
	StatusEscolarFundamental *string    `json:"status_escolar_fundamental,omitempty" db:"status_escolar_fundamental"`
	StatusEscolarMedio       *string    `json:"status_escolar_medio,omitempty" db:"status_escolar_medio"`
	StatusSuperior           *string    `json:"status_superior,omitempty" db:"status_superior"`
	CreatedAt                time.Time  `json:"created_at" db:"created_at"`
}

type Curso struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Nome      string    `json:"nome" db:"nome"`
	Type      string    `json:"type" db:"type"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type RegistroNotas struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	CodigoEstudante string     `json:"codigo_estudante" db:"codigo_estudante"`
	CodigoAcademia  string     `json:"codigo_academia" db:"codigo_academia"`
	AnoLectivo      string     `json:"ano_lectivo" db:"ano_lectivo"`
	Periodo         string     `json:"periodo" db:"periodo"`
	Materias        []Materia  `json:"materias" db:"materias"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	EventID         *uuid.UUID `json:"event_id,omitempty" db:"event_id"`
}

type RegistroFaltas struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	CodigoEstudante string          `json:"codigo_estudante" db:"codigo_estudante"`
	CodigoAcademia  string          `json:"codigo_academia" db:"codigo_academia"`
	AnoLectivo      string          `json:"ano_lectivo" db:"ano_lectivo"`
	Periodo         string          `json:"periodo" db:"periodo"`
	Materias        []MateriaFaltas `json:"materias" db:"materias"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	EventID         *uuid.UUID      `json:"event_id,omitempty" db:"event_id"`
}

type Materia struct {
	Nome string  `json:"nome"`
	Nota float64 `json:"nota"`
}

type MateriaFaltas struct {
	Nome   string `json:"nome"`
	Faltas int    `json:"faltas"`
}

type LoginRequest struct {
	Usuario string `json:"usuario" binding:"required"`
	Senha   string `json:"senha" binding:"required"`
	Type    string `json:"type" binding:"required"`
}

type LoginResponse struct {
	Token  string `json:"token"`
	Codigo string `json:"codigo"`
	Nome   string `json:"nome"`
	Type   string `json:"type"`
}

type RegisterAcademiaRequest struct {
	Nivel          string   `json:"nivel" binding:"required"`
	Type           string   `json:"type" binding:"required"`
	Senha          string   `json:"senha" binding:"required"`
	Nome           string   `json:"nome" binding:"required"`
	Provincia      string   `json:"provincia" binding:"required"`
	Endereco       string   `json:"endereco" binding:"required"`
	NumeroTelefone *string  `json:"numero_telefone"`
	Email          *string  `json:"email"`
	Website        *string  `json:"website"`
	NivelEscolar   *string  `json:"nivel_escolar"`
	Cursos         []string `json:"cursos"`
}

// RegisterEstudanteRequest — genero e data_nascimento são obrigatórios.
type RegisterEstudanteRequest struct {
	Senha                    string     `json:"senha"            binding:"required"`
	Nome                     string     `json:"nome"             binding:"required"`
	Genero                   string     `json:"genero"           binding:"required"`
	DataNascimento           time.Time  `json:"data_nascimento"  binding:"required"`
	BilheteIdentidade        *string    `json:"bilhete_identidade"`
	BilheteIdentidadeResp    *string    `json:"bilhete_identidade_responsavel"`
	AnoEscolar               *string    `json:"ano_escolar"`
	AnoEscolarMedio          *string    `json:"ano_escolar_medio"`
	AnoSuperior              *string    `json:"ano_superior"`
	CursoMedioID             *uuid.UUID `json:"curso_medio_id"`
	CursoSuperiorID          *uuid.UUID `json:"curso_superior_id"`
	StatusEscolarFundamental *string    `json:"status_escolar_fundamental"`
	StatusEscolarMedio       *string    `json:"status_escolar_medio"`
	StatusSuperior           *string    `json:"status_superior"`
}

type RegistrarNotasRequest struct {
	CodigoEstudante string    `json:"codigo_estudante" binding:"required"`
	AnoLectivo      string    `json:"ano_lectivo"      binding:"required"`
	Periodo         string    `json:"periodo"          binding:"required"`
	Materias        []Materia `json:"materias"         binding:"required"`
}

type RegistrarFaltasRequest struct {
	CodigoEstudante string          `json:"codigo_estudante" binding:"required"`
	AnoLectivo      string          `json:"ano_lectivo"      binding:"required"`
	Periodo         string          `json:"periodo"          binding:"required"`
	Materias        []MateriaFaltas `json:"materias"         binding:"required"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
