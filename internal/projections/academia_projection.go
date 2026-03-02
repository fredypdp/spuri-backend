package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Projection
// ============================================================================

type AcademiaProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewAcademiaProjection(client *db.Client) *AcademiaProjection {
	return &AcademiaProjection{client: client, ctx: context.Background()}
}

func (p *AcademiaProjection) Name() string { return "academias" }

// Handle processa eventos do ledger.
//
// CORREÇÃO [BUG #2]: O guard original `if event.AggregateType != "Academia"`
// bloqueava o evento "EstudanteInscrito" (emitido pelo aggregate Estudante),
// impedindo o incremento de total_inscricoes_pendentes.
//
// Agora aceitamos eventos de "Estudante" para tratar EstudanteInscrito,
// e mantemos o filtro para todos os demais tipos desconhecidos.
func (p *AcademiaProjection) Handle(event db.Event) error {
	// Eventos do aggregate Academia
	if event.AggregateType == "Academia" {
		academiaHandlers := map[string]func(db.Event) error{
			"AcademiaCriada":           p.handleAcademiaCriada,
			"AcademiaAtivada":          p.handleStatusChange("ativo"),
			"AcademiaDesativada":       p.handleStatusChange("inativo"),
			"CursosAtualizados":        p.handleCursosAtualizados,
			"InscricaoAprovada":        p.handleInscricaoAprovada,
			"InscricaoReprovada":       p.handleInscricaoReprovada,
			"AcademiaDadosAtualizados": p.handleAcademiaDadosAtualizados,
			"EmailVerificado":          p.handleEmailVerificado,
			// CategoriaNotaAdicionada é tratado pela CategoriasNotaProjection dedicada.
			// A AcademiaProjection ignora este evento intencionalmente.
		}

		if handler, ok := academiaHandlers[event.EventType]; ok {
			log.Printf("[DEBUG] [academias] Processando %s para %s", event.EventType, event.AggregateID)
			return handler(event)
		}
		return nil
	}

	// Eventos do aggregate Estudante relevantes para a AcademiaProjection
	// CORREÇÃO [BUG #2]: EstudanteInscrito incrementa total_inscricoes_pendentes
	if event.AggregateType == "Estudante" && event.EventType == "EstudanteInscrito" {
		log.Printf("[DEBUG] [academias] Processando EstudanteInscrito para incrementar pendentes")
		return p.handleEstudanteInscrito(event)
	}

	return nil
}

// ============================================================================
// Rebuild
// ============================================================================

// Rebuild reconstrói a projeção do zero a partir do ledger.
//
// CORREÇÃO [BUG #1]: O scan de previous_hash agora usa sql.NullString.
// O campo é nullable no banco (o primeiro evento de qualquer aggregate
// não tem hash anterior), e escanear NULL diretamente para *string causava
// erro de runtime que abortava o rebuild inteiro.
//
// CORREÇÃO [BUG #2]: A query inclui eventos do aggregate "Estudante"
// (especificamente EstudanteInscrito) para reconstruir corretamente
// o campo total_inscricoes_pendentes.
func (p *AcademiaProjection) Rebuild() error {
	log.Printf("[DEBUG] [academias] Rebuild iniciado")

	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar projection_academias: %w", err)
	}

	// Busca tanto eventos de Academia quanto EstudanteInscrito de Estudante
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Academia'
		   OR (aggregate_type = 'Estudante' AND event_type = 'EstudanteInscrito')
		ORDER BY id ASC
	`)
	if err != nil {
		return fmt.Errorf("erro ao buscar eventos para rebuild: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var event db.Event
		// CORREÇÃO [BUG #1]: usa sql.NullString para previous_hash (campo nullable)
		var prevHash sql.NullString

		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		); err != nil {
			return fmt.Errorf("erro ao escanear evento %d: %w", count, err)
		}

		// Atribui apenas se não-NULL
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}

		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d (type=%s): %w", event.ID, event.EventType, err)
		}
		count++
	}

	log.Printf("[DEBUG] [academias] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

// ============================================================================
// Checkpoint — CORREÇÃO [INCONSISTÊNCIA #3]: prepared statements
// ============================================================================

func (p *AcademiaProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	err := p.client.DB().QueryRow(
		`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = $1`,
		p.Name(),
	).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *AcademiaProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

func (p *AcademiaProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_academias CASCADE`)
	return err
}

// ============================================================================
// Event handlers — CORREÇÃO [INCONSISTÊNCIA #3]: prepared statements em todos
// ============================================================================

// handleAcademiaCriada insere um novo registro na projeção.
// CORREÇÃO: usa prepared statement com $1..$N em vez de fmt.Sprintf + SafeString.
func (p *AcademiaProjection) handleAcademiaCriada(event db.Event) error {
	var payload struct {
		Type           string    `json:"Type"`
		Nome           string    `json:"Nome"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		SenhaHash      string    `json:"SenhaHash"`
		Provincia      string    `json:"Provincia"`
		Endereco       string    `json:"Endereco"`
		NumeroTelefone *string   `json:"NumeroTelefone"`
		Email          *string   `json:"Email"`
		Website        *string   `json:"Website"`
		NivelEscolar   *string   `json:"NivelEscolar"`
		AnosAcademicos []string  `json:"AnosAcademicos"`
		Cursos         []string  `json:"Cursos"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAcademiaCriada: parse error: %w", err)
	}
	if event.AggregateID == uuid.Nil || payload.SenhaHash == "" {
		return fmt.Errorf("handleAcademiaCriada: dados inválidos (uuid ou senhaHash vazios)")
	}

	cursosJSON, _ := json.Marshal(payload.Cursos)

	var anosJSON interface{}
	if len(payload.AnosAcademicos) > 0 {
		b, _ := json.Marshal(payload.AnosAcademicos)
		anosJSON = string(b)
	} // nil → NULL no banco

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_academias (
			id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, version, created_at, updated_at, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			'inativo', $13, FALSE, $14, $15, CURRENT_TIMESTAMP, $16
		)
		ON CONFLICT (id) DO UPDATE SET
			type            = EXCLUDED.type,
			nome            = EXCLUDED.nome,
			codigo_academia = EXCLUDED.codigo_academia,
			senha_hash      = EXCLUDED.senha_hash,
			provincia       = EXCLUDED.provincia,
			endereco        = EXCLUDED.endereco,
			numero_telefone = EXCLUDED.numero_telefone,
			email           = EXCLUDED.email,
			website         = EXCLUDED.website,
			nivel_escolar   = EXCLUDED.nivel_escolar,
			anos_academicos = EXCLUDED.anos_academicos,
			cursos          = EXCLUDED.cursos,
			version         = EXCLUDED.version,
			updated_at      = EXCLUDED.updated_at,
			last_event_id   = EXCLUDED.last_event_id
	`,
		event.AggregateID,
		payload.Type, payload.Nome, payload.CodigoAcademia, payload.SenhaHash,
		payload.Provincia, payload.Endereco,
		payload.NumeroTelefone, payload.Email, payload.Website, payload.NivelEscolar,
		anosJSON,
		string(cursosJSON),
		event.EventVersion,
		payload.CreatedAt,
		event.EventID,
	)
	return err
}

// handleStatusChange retorna um handler que atualiza o status da academia.
// CORREÇÃO: usa prepared statement.
func (p *AcademiaProjection) handleStatusChange(status string) func(db.Event) error {
	return func(event db.Event) error {
		if event.AggregateID == uuid.Nil {
			return fmt.Errorf("handleStatusChange: UUID inválido")
		}
		_, err := p.client.DB().Exec(`
			UPDATE projection_academias
			SET status       = $1,
			    version      = $2,
			    updated_at   = CURRENT_TIMESTAMP,
			    last_event_id = $3
			WHERE id = $4
		`, status, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}
}

// handleCursosAtualizados atualiza a lista de cursos da academia.
// CORREÇÃO: usa prepared statement.
func (p *AcademiaProjection) handleCursosAtualizados(event db.Event) error {
	var payload struct {
		NovoCursos []string `json:"NovoCursos"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCursosAtualizados: parse error: %w", err)
	}

	cursosJSON, _ := json.Marshal(payload.NovoCursos)

	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET cursos        = $1,
		    version       = $2,
		    updated_at    = CURRENT_TIMESTAMP,
		    last_event_id = $3
		WHERE id = $4
	`, string(cursosJSON), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleInscricaoAprovada incrementa total_estudantes e decrementa
// total_inscricoes_pendentes quando uma inscrição é aprovada.
// CORREÇÃO: usa prepared statement.
func (p *AcademiaProjection) handleInscricaoAprovada(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET total_estudantes           = total_estudantes + 1,
		    total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
		    version                    = $1,
		    updated_at                 = CURRENT_TIMESTAMP,
		    last_event_id              = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleInscricaoReprovada decrementa total_inscricoes_pendentes quando
// uma inscrição é reprovada.
// CORREÇÃO: usa prepared statement.
func (p *AcademiaProjection) handleInscricaoReprovada(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
		    version                    = $1,
		    updated_at                 = CURRENT_TIMESTAMP,
		    last_event_id              = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleEstudanteInscrito incrementa total_inscricoes_pendentes quando um
// estudante solicita inscrição em uma academia.
//
// CORREÇÃO [BUG #2]: este handler é novo. O evento EstudanteInscrito é emitido
// pelo aggregate Estudante e carrega o AcademiaID no payload. Usamos esse ID
// para encontrar a academia correta e incrementar o contador.
func (p *AcademiaProjection) handleEstudanteInscrito(event db.Event) error {
	var payload struct {
		AcademiaID uuid.UUID `json:"AcademiaID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteInscrito: parse error: %w", err)
	}
	if payload.AcademiaID == uuid.Nil {
		return fmt.Errorf("handleEstudanteInscrito: AcademiaID ausente no payload")
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1,
		    updated_at                 = CURRENT_TIMESTAMP
		WHERE id = $1
	`, payload.AcademiaID)
	return err
}

// handleAcademiaDadosAtualizados atualiza campos opcionais da academia.
// CORREÇÃO: usa prepared statement para os campos escalar; mantém lógica
// de SET dinâmico mas com binding seguro para valores.
func (p *AcademiaProjection) handleAcademiaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome           *string  `json:"Nome"`
		Provincia      *string  `json:"Provincia"`
		Endereco       *string  `json:"Endereco"`
		NumeroTelefone *string  `json:"NumeroTelefone"`
		Email          *string  `json:"Email"`
		Website        *string  `json:"Website"`
		NivelEscolar   *string  `json:"NivelEscolar"`
		AnosAcademicos []string `json:"AnosAcademicos"`
		Cursos         []string `json:"Cursos"`
		EmailAlterado  bool     `json:"EmailAlterado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAcademiaDadosAtualizados: parse error: %w", err)
	}

	// Monta os campos escalares com SET nome_col = $N e acumula args
	args := []interface{}{}
	setClauses := []string{}
	paramIdx := 1

	addParam := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", clause, paramIdx))
		args = append(args, val)
		paramIdx++
	}

	if payload.Nome != nil {
		addParam("nome", *payload.Nome)
	}
	if payload.Provincia != nil {
		addParam("provincia", *payload.Provincia)
	}
	if payload.Endereco != nil {
		addParam("endereco", *payload.Endereco)
	}
	if payload.NumeroTelefone != nil {
		addParam("numero_telefone", *payload.NumeroTelefone)
	}
	if payload.Website != nil {
		addParam("website", *payload.Website)
	}
	if payload.NivelEscolar != nil {
		addParam("nivel_escolar", *payload.NivelEscolar)
	}
	if payload.Email != nil {
		addParam("email", *payload.Email)
		if payload.EmailAlterado {
			setClauses = append(setClauses, "email_verificado = FALSE")
		}
	}

	// anos_academicos: nil = não altera; len==0 = limpa; len>0 = atualiza
	if payload.AnosAcademicos != nil {
		if len(payload.AnosAcademicos) > 0 {
			b, _ := json.Marshal(payload.AnosAcademicos)
			addParam("anos_academicos", string(b))
		} else {
			setClauses = append(setClauses, "anos_academicos = NULL")
		}
	}

	if payload.Cursos != nil {
		b, _ := json.Marshal(payload.Cursos)
		addParam("cursos", string(b))
	}

	if len(setClauses) == 0 {
		// Nenhum campo alterado — garante version/updated_at mesmo assim
		_, err := p.client.DB().Exec(`
			UPDATE projection_academias
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	// Adiciona campos de auditoria ao SET
	setClauses = append(setClauses,
		fmt.Sprintf("version = $%d", paramIdx),
		fmt.Sprintf("updated_at = CURRENT_TIMESTAMP"),
		fmt.Sprintf("last_event_id = $%d", paramIdx+1),
	)
	args = append(args, event.EventVersion, event.EventID)
	paramIdx += 2

	// WHERE id
	args = append(args, event.AggregateID)
	whereClause := fmt.Sprintf("WHERE id = $%d", paramIdx)

	query := fmt.Sprintf(
		"UPDATE projection_academias SET %s %s",
		joinClauses(setClauses),
		whereClause,
	)

	_, err := p.client.DB().Exec(query, args...)
	return err
}

// handleEmailVerificado marca email_verificado = TRUE.
// CORREÇÃO: usa prepared statement.
func (p *AcademiaProjection) handleEmailVerificado(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET email_verificado = TRUE,
		    version          = $1,
		    updated_at       = CURRENT_TIMESTAMP,
		    last_event_id    = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Query / DTO
// ============================================================================

type AcademiaDTO struct {
	ID                       uuid.UUID `json:"id"`
	Type                     string    `json:"type"`
	Nome                     string    `json:"nome"`
	CodigoAcademia           string    `json:"codigo_academia"`
	SenhaHash                string    `json:"-"`
	Provincia                string    `json:"provincia"`
	Endereco                 string    `json:"endereco"`
	NumeroTelefone           *string   `json:"numero_telefone,omitempty"`
	Email                    *string   `json:"email,omitempty"`
	Website                  *string   `json:"website,omitempty"`
	NivelEscolar             *string   `json:"nivel_escolar,omitempty"`
	// AnosAcademicos — anos do ensino fundamental oferecidos (nil para superior/médio)
	AnosAcademicos           []string  `json:"anos_academicos,omitempty"`
	Status                   string    `json:"status"`
	Cursos                   []string  `json:"cursos"`
	EmailVerificado          bool      `json:"email_verificado"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
	TotalEstudantes          int       `json:"total_estudantes"`
	TotalInscricoesPendentes int       `json:"total_inscricoes_pendentes"`
	Version                  int       `json:"version"`
}

// GetByID busca academia pelo UUID.
// CORREÇÃO: usa prepared statement.
func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	rows, err := p.client.DB().Query(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE id = $1
		LIMIT 1
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return p.scanAcademia(rows)
}

// GetByCodigoOrEmail busca academia por codigo_academia OU email.
// Usado no login onde o utilizador pode inserir qualquer um dos dois.
// CORREÇÃO: usa prepared statement.
func (p *AcademiaProjection) GetByCodigoOrEmail(usuario string) (*AcademiaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = $1 OR email = $1
		LIMIT 1
	`, usuario)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return p.scanAcademia(rows)
}

// GetByCodigo busca academia pelo código único.
// CORREÇÃO: usa prepared statement.
func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = $1
		LIMIT 1
	`, codigo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return p.scanAcademia(rows)
}

// ListarTodas retorna todas as academias ordenadas por nome.
func (p *AcademiaProjection) ListarTodas() ([]AcademiaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		ORDER BY nome ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var academias []AcademiaDTO
	for rows.Next() {
		dto, err := p.scanAcademia(rows)
		if err != nil {
			log.Printf("[WARN] [academias] Erro ao escanear academia: %v", err)
			continue
		}
		academias = append(academias, *dto)
	}
	return academias, rows.Err()
}

// ============================================================================
// Scan helper
// ============================================================================

type scannable interface {
	Scan(dest ...interface{}) error
}

func (p *AcademiaProjection) scanAcademia(row scannable) (*AcademiaDTO, error) {
	var dto AcademiaDTO
	var cursosJSON []byte
	var anosJSON []byte
	var numTel, email, website, nivelEscolar sql.NullString

	err := row.Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia, &dto.SenhaHash,
		&dto.Provincia, &dto.Endereco,
		&numTel, &email, &website, &nivelEscolar, &anosJSON,
		&dto.Status, &cursosJSON, &dto.EmailVerificado,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes, &dto.Version,
	)
	if err != nil {
		return nil, err
	}

	if numTel.Valid {
		dto.NumeroTelefone = &numTel.String
	}
	if email.Valid {
		dto.Email = &email.String
	}
	if website.Valid {
		dto.Website = &website.String
	}
	if nivelEscolar.Valid {
		dto.NivelEscolar = &nivelEscolar.String
	}

	if cursosJSON != nil {
		json.Unmarshal(cursosJSON, &dto.Cursos) //nolint:errcheck
	}
	if dto.Cursos == nil {
		dto.Cursos = []string{}
	}

	if anosJSON != nil {
		json.Unmarshal(anosJSON, &dto.AnosAcademicos) //nolint:errcheck
	}

	return &dto, nil
}

// ============================================================================
// Helper interno
// ============================================================================

// joinClauses une cláusulas SET com vírgula.
func joinClauses(clauses []string) string {
	result := ""
	for i, c := range clauses {
		if i > 0 {
			result += ", "
		}
		result += c
	}
	return result
}