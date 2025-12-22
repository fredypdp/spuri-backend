package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/domain"

	"github.com/google/uuid"
)

// GetAcademiaByCodigoOrEmail obtém uma academia por código ou email
func GetAcademiaByCodigoOrEmail(identifier string) (*domain.Academia, error) {
	query := `
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
		       numero_telefone, email, website, nivel_escolar, status, 
		       COALESCE(cursos, '[]'::jsonb) as cursos, created_at
		FROM escolas_universidades
		WHERE codigo_academia = $1 OR email = $1
		LIMIT 1
	`

	var academia domain.Academia
	var cursosJSON []byte

	err := DB.QueryRow(query, identifier).Scan(
		&academia.ID,
		&academia.Type,
		&academia.Nome,
		&academia.CodigoAcademia,
		&academia.SenhaHash,
		&academia.Provincia,
		&academia.Endereco,
		&academia.NumeroTelefone,
		&academia.Email,
		&academia.Website,
		&academia.NivelEscolar,
		&academia.Status,
		&cursosJSON,
		&academia.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao buscar academia: %w", err)
	}

	// Deserializar JSONB de cursos
	if err := json.Unmarshal(cursosJSON, &academia.Cursos); err != nil {
		academia.Cursos = []string{} // Se falhar, usa array vazio
	}

	return &academia, nil
}

// GetAcademiaByID obtém uma academia por ID
func GetAcademiaByID(id uuid.UUID) (*domain.Academia, error) {
	query := `
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
		       numero_telefone, email, website, nivel_escolar, status,
		       COALESCE(cursos, '[]'::jsonb) as cursos, created_at
		FROM escolas_universidades
		WHERE id = $1
	`

	var academia domain.Academia
	var cursosJSON []byte

	err := DB.QueryRow(query, id).Scan(
		&academia.ID,
		&academia.Type,
		&academia.Nome,
		&academia.CodigoAcademia,
		&academia.SenhaHash,
		&academia.Provincia,
		&academia.Endereco,
		&academia.NumeroTelefone,
		&academia.Email,
		&academia.Website,
		&academia.NivelEscolar,
		&academia.Status,
		&cursosJSON,
		&academia.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao buscar academia: %w", err)
	}

	// Deserializar JSONB de cursos
	if err := json.Unmarshal(cursosJSON, &academia.Cursos); err != nil {
		academia.Cursos = []string{} // Se falhar, usa array vazio
	}

	return &academia, nil
}

// CreateAcademia cria uma nova academia
func CreateAcademia(academia *domain.Academia) error {
	query := `
		INSERT INTO escolas_universidades (
			type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, cursos
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`

	// Converter cursos para JSONB
	cursosJSON, err := json.Marshal(academia.Cursos)
	if err != nil {
		return fmt.Errorf("erro ao converter cursos: %w", err)
	}

	err = DB.QueryRow(
		query,
		academia.Type,
		academia.Nome,
		academia.CodigoAcademia,
		academia.SenhaHash,
		academia.Provincia,
		academia.Endereco,
		academia.NumeroTelefone,
		academia.Email,
		academia.Website,
		academia.NivelEscolar,
		cursosJSON,
	).Scan(&academia.ID, &academia.CreatedAt)

	if err != nil {
		return fmt.Errorf("erro ao criar academia: %w", err)
	}

	return nil
}

// GetEstudanteByBilhete obtém um estudante por bilhete
func GetEstudanteByBilhete(bilhete string) (*domain.Estudante, error) {
	query := `
		SELECT id, nome, senha_hash, bilhete_identidade, 
		       bilhete_identidade_responsavel, id_academia,
		       ano_escolar, ano_superior, curso_medio, curso_superior,
		       status_escolar, status_superior, created_at
		FROM estudantes
		WHERE bilhete_identidade = $1 OR bilhete_identidade_responsavel = $1
		LIMIT 1
	`

	var estudante domain.Estudante
	err := DB.Get(&estudante, query, bilhete)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao buscar estudante: %w", err)
	}

	return &estudante, nil
}

// GetEstudanteByID obtém um estudante por ID
func GetEstudanteByID(id uuid.UUID) (*domain.Estudante, error) {
	query := `
		SELECT id, nome, senha_hash, bilhete_identidade, 
		       bilhete_identidade_responsavel, id_academia,
		       ano_escolar, ano_superior, curso_medio, curso_superior,
		       status_escolar, status_superior, created_at
		FROM estudantes
		WHERE id = $1
	`

	var estudante domain.Estudante
	err := DB.Get(&estudante, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao buscar estudante: %w", err)
	}

	return &estudante, nil
}

// CreateEstudante cria um novo estudante
func CreateEstudante(estudante *domain.Estudante) error {
	query := `
		INSERT INTO estudantes (
			nome, senha_hash, bilhete_identidade, bilhete_identidade_responsavel,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			status_escolar, status_superior
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`

	err := DB.QueryRow(
		query,
		estudante.Nome,
		estudante.SenhaHash,
		estudante.BilheteIdentidade,
		estudante.BilheteIdentidadeResp,
		estudante.AnoEscolar,
		estudante.AnoSuperior,
		estudante.CursoMedio,
		estudante.CursoSuperior,
		estudante.StatusEscolar,
		estudante.StatusSuperior,
	).Scan(&estudante.ID, &estudante.CreatedAt)

	if err != nil {
		return fmt.Errorf("erro ao criar estudante: %w", err)
	}

	return nil
}

// GetNotasByEstudante obtém todas as notas de um estudante
func GetNotasByEstudante(estudanteID uuid.UUID) ([]domain.RegistroNotas, error) {
	query := `
		SELECT id, estudante_id, id_academia, ano_lectivo, periodo, 
		       materias, created_at, event_id
		FROM registro_notas
		WHERE estudante_id = $1
		ORDER BY created_at DESC
	`

	rows, err := DB.Queryx(query, estudanteID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar notas: %w", err)
	}
	defer rows.Close()

	var notas []domain.RegistroNotas
	for rows.Next() {
		var nota domain.RegistroNotas
		var materiasJSON []byte

		err := rows.Scan(
			&nota.ID,
			&nota.EstudanteID,
			&nota.IDAcademia,
			&nota.AnoLectivo,
			&nota.Periodo,
			&materiasJSON,
			&nota.CreatedAt,
			&nota.EventID,
		)
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear nota: %w", err)
		}

		// Deserializar JSONB de materias
		if err := json.Unmarshal(materiasJSON, &nota.Materias); err != nil {
			return nil, fmt.Errorf("erro ao deserializar materias: %w", err)
		}

		notas = append(notas, nota)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar sobre notas: %w", err)
	}

	return notas, nil
}

// GetFaltasByEstudante obtém todas as faltas de um estudante
func GetFaltasByEstudante(estudanteID uuid.UUID) ([]domain.RegistroFaltas, error) {
	query := `
		SELECT id, estudante_id, id_academia, ano_lectivo, periodo,
		       materias, created_at, event_id
		FROM registro_faltas
		WHERE estudante_id = $1
		ORDER BY created_at DESC
	`

	rows, err := DB.Queryx(query, estudanteID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar faltas: %w", err)
	}
	defer rows.Close()

	var faltas []domain.RegistroFaltas
	for rows.Next() {
		var falta domain.RegistroFaltas
		var materiasJSON []byte

		err := rows.Scan(
			&falta.ID,
			&falta.EstudanteID,
			&falta.IDAcademia,
			&falta.AnoLectivo,
			&falta.Periodo,
			&materiasJSON,
			&falta.CreatedAt,
			&falta.EventID,
		)
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear falta: %w", err)
		}

		// Deserializar JSONB de materias
		if err := json.Unmarshal(materiasJSON, &falta.Materias); err != nil {
			return nil, fmt.Errorf("erro ao deserializar materias: %w", err)
		}

		faltas = append(faltas, falta)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar sobre faltas: %w", err)
	}

	return faltas, nil
}

// CreateInscricao cria uma nova inscrição
func CreateInscricao(inscricao *domain.Inscricao) error {
	query := `
		INSERT INTO inscricoes (
			estudante_id, academia_id, tipo, ano_inscricao, curso, status
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	err := DB.QueryRow(
		query,
		inscricao.EstudanteID,
		inscricao.AcademiaID,
		inscricao.Tipo,
		inscricao.AnoInscricao,
		inscricao.Curso,
		inscricao.Status,
	).Scan(&inscricao.ID, &inscricao.CreatedAt, &inscricao.UpdatedAt)

	if err != nil {
		return fmt.Errorf("erro ao criar inscrição: %w", err)
	}

	return nil
}

// GetInscricoesByEstudante obtém inscrições de um estudante
func GetInscricoesByEstudante(estudanteID uuid.UUID) ([]domain.Inscricao, error) {
	query := `
		SELECT id, estudante_id, academia_id, tipo, ano_inscricao, 
		       curso, status, created_at, updated_at
		FROM inscricoes
		WHERE estudante_id = $1
		ORDER BY created_at DESC
	`

	var inscricoes []domain.Inscricao
	err := DB.Select(&inscricoes, query, estudanteID)
	if err != nil {
		if err == sql.ErrNoRows {
			return []domain.Inscricao{}, nil
		}
		return nil, fmt.Errorf("erro ao buscar inscrições: %w", err)
	}

	return inscricoes, nil
}

// GetInscricoesByAcademia obtém inscrições de uma academia
func GetInscricoesByAcademia(academiaID uuid.UUID, status string) ([]domain.Inscricao, error) {
	query := `
		SELECT id, estudante_id, academia_id, tipo, ano_inscricao,
		       curso, status, created_at, updated_at
		FROM inscricoes
		WHERE academia_id = $1 AND status = $2
		ORDER BY created_at DESC
	`

	var inscricoes []domain.Inscricao
	err := DB.Select(&inscricoes, query, academiaID, status)
	if err != nil {
		if err == sql.ErrNoRows {
			return []domain.Inscricao{}, nil
		}
		return nil, fmt.Errorf("erro ao buscar inscrições: %w", err)
	}

	return inscricoes, nil
}

// UpdateInscricaoStatus atualiza o status de uma inscrição
func UpdateInscricaoStatus(inscricaoID uuid.UUID, status string) error {
	query := `
		UPDATE inscricoes
		SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	result, err := DB.Exec(query, status, inscricaoID)
	if err != nil {
		return fmt.Errorf("erro ao atualizar status da inscrição: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("erro ao verificar linhas afetadas: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("inscrição não encontrada")
	}

	return nil
}

// VincularEstudanteAcademia vincula um estudante a uma academia
func VincularEstudanteAcademia(estudanteID, academiaID uuid.UUID) error {
	query := `
		UPDATE estudantes
		SET id_academia = $1
		WHERE id = $2
	`

	result, err := DB.Exec(query, academiaID, estudanteID)
	if err != nil {
		return fmt.Errorf("erro ao vincular estudante: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("erro ao verificar linhas afetadas: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("estudante não encontrado")
	}

	log.Printf("✅ Estudante %s vinculado à academia %s", estudanteID, academiaID)
	return nil
}