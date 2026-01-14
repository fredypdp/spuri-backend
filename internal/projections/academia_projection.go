package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/genesisdb"
	"time"

	"github.com/google/uuid"
)

// AcademiaProjection projeÃ§Ã£o de academias
type AcademiaProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

// NewAcademiaProjection cria nova projeÃ§Ã£o de academia
func NewAcademiaProjection(client *genesisdb.Client) *AcademiaProjection {
	return &AcademiaProjection{
		client: client,
		ctx:    context.Background(),
	}
}

// Name implementa Projection
func (p *AcademiaProjection) Name() string {
	return "academias"
}

// Handle processa um evento
func (p *AcademiaProjection) Handle(event genesisdb.Event) error {
	// Processar apenas eventos de Academia
	if event.AggregateType != "Academia" {
		return nil
	}

	switch event.EventType {
	case "AcademiaCriada":
		return p.handleAcademiaCriada(event)
	case "AcademiaAtivada":
		return p.handleAcademiaAtivada(event)
	case "AcademiaDesativada":
		return p.handleAcademiaDesativada(event)
	case "CursosAtualizados":
		return p.handleCursosAtualizados(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "InscricaoReprovada":
		return p.handleInscricaoReprovada(event)
	default:
		return nil
	}
}

// Rebuild reconstrÃ³i a projeÃ§Ã£o do zero
func (p *AcademiaProjection) Rebuild() error {
	// 1. Limpar projeÃ§Ã£o existente
	if err := p.clear(); err != nil {
		return err
	}

	// 2. Buscar todos os eventos de Academia
	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE aggregate_type = 'Academia'
		ORDER BY id ASC
	`

	var events []genesisdb.Event
	if err := p.client.DB().Select(&events, query); err != nil {
		return err
	}

	// 3. Processar todos os eventos
	for _, event := range events {
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}

	return nil
}

// GetLastProcessedEventID implementa Projection
func (p *AcademiaProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().GetContext(p.ctx, &lastID, query, p.Name())
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

// UpdateCheckpoint implementa Projection
func (p *AcademiaProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		UPDATE projection_checkpoints
		SET 
			last_processed_event_id = $1,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = events_processed + 1
		WHERE projection_name = $2
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, eventID, p.Name())
	return err
}

// clear limpa a projeÃ§Ã£o
func (p *AcademiaProjection) clear() error {
	query := `TRUNCATE TABLE projection_academias CASCADE`
	_, err := p.client.DB().ExecContext(p.ctx, query)
	return err
}

// Event Handlers

func (p *AcademiaProjection) handleAcademiaCriada(event genesisdb.Event) error {
	log.Printf("ðŸ”µ [PROJEÃ‡ÃƒO ACADEMIA] Iniciando processamento de AcademiaCriada")
	log.Printf("   Event ID: %s", event.EventID)
	log.Printf("   Aggregate ID: %s", event.AggregateID)
	
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
		Cursos         []string  `json:"Cursos"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("âŒ [PROJEÃ‡ÃƒO ACADEMIA] Erro ao parsear payload: %v", err)
		log.Printf("   Payload raw: %s", string(event.Payload))
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("ðŸ“Š [PROJEÃ‡ÃƒO ACADEMIA] Dados parseados:")
	log.Printf("   Type: %s", payload.Type)
	log.Printf("   Nome: %s", payload.Nome)
	log.Printf("   CÃ³digo: %s", payload.CodigoAcademia)
	log.Printf("   ProvÃ­ncia: %s", payload.Provincia)
	log.Printf("   SenhaHash existe: %v (length: %d)", payload.SenhaHash != "", len(payload.SenhaHash))

	// Verificar se senha estÃ¡ no payload
	if payload.SenhaHash == "" {
		log.Printf("âŒ [PROJEÃ‡ÃƒO ACADEMIA] SenhaHash vazio no evento!")
		return fmt.Errorf("SenhaHash vazio no evento")
	}

	// Converter cursos para JSONB
	cursosJSON, err := json.Marshal(payload.Cursos)
	if err != nil {
		log.Printf("âŒ [PROJEÃ‡ÃƒO ACADEMIA] Erro ao converter cursos: %v", err)
		return err
	}

	query := `
		INSERT INTO projection_academias (
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, version, created_at, updated_at, last_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (id) DO UPDATE SET
			type = EXCLUDED.type,
			nome = EXCLUDED.nome,
			codigo_academia = EXCLUDED.codigo_academia,
			senha_hash = EXCLUDED.senha_hash,
			provincia = EXCLUDED.provincia,
			endereco = EXCLUDED.endereco,
			numero_telefone = EXCLUDED.numero_telefone,
			email = EXCLUDED.email,
			website = EXCLUDED.website,
			nivel_escolar = EXCLUDED.nivel_escolar,
			cursos = EXCLUDED.cursos,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			last_event_id = EXCLUDED.last_event_id
	`

	log.Printf("ðŸ”„ [PROJEÃ‡ÃƒO ACADEMIA] Executando INSERT na tabela...")
	result, err := p.client.DB().ExecContext(
		p.ctx, query,
		event.AggregateID,
		payload.Type,
		payload.Nome,
		payload.CodigoAcademia,
		payload.SenhaHash,
		payload.Provincia,
		payload.Endereco,
		payload.NumeroTelefone,
		payload.Email,
		payload.Website,
		payload.NivelEscolar,
		"ativo",
		cursosJSON,
		event.EventVersion,
		payload.CreatedAt,
		time.Now(),
		event.EventID,
	)

	if err != nil {
		log.Printf("âŒ [PROJEÃ‡ÃƒO ACADEMIA] Erro ao executar INSERT: %v", err)
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("âœ… [PROJEÃ‡ÃƒO ACADEMIA] Academia salva com sucesso! (rows affected: %d)", rowsAffected)
	
	// Verificar se realmente salvou
	var count int
	checkQuery := `SELECT COUNT(*) FROM projection_academias WHERE id = $1`
	p.client.DB().GetContext(p.ctx, &count, checkQuery, event.AggregateID)
	log.Printf("ðŸ” [PROJEÃ‡ÃƒO ACADEMIA] VerificaÃ§Ã£o: %d registro(s) encontrado(s) com ID %s", count, event.AggregateID)

	return nil
}

func (p *AcademiaProjection) handleAcademiaAtivada(event genesisdb.Event) error {
	query := `
		UPDATE projection_academias
		SET 
			status = 'ativo',
			version = $1,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $2
		WHERE id = $3
	`

	_, err := p.client.DB().ExecContext(
		p.ctx, query,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

func (p *AcademiaProjection) handleAcademiaDesativada(event genesisdb.Event) error {
	query := `
		UPDATE projection_academias
		SET 
			status = 'inativo',
			version = $1,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $2
		WHERE id = $3
	`

	_, err := p.client.DB().ExecContext(
		p.ctx, query,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

func (p *AcademiaProjection) handleCursosAtualizados(event genesisdb.Event) error {
	var payload struct {
		NovoCursos []string `json:"NovoCursos"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	cursosJSON, err := json.Marshal(payload.NovoCursos)
	if err != nil {
		return err
	}

	query := `
		UPDATE projection_academias
		SET 
			cursos = $1,
			version = $2,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $3
		WHERE id = $4
	`

	_, err = p.client.DB().ExecContext(
		p.ctx, query,
		cursosJSON,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

func (p *AcademiaProjection) handleInscricaoAprovada(event genesisdb.Event) error {
	query := `
		UPDATE projection_academias
		SET 
			total_estudantes = total_estudantes + 1,
			total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, event.AggregateID)
	return err
}

func (p *AcademiaProjection) handleInscricaoReprovada(event genesisdb.Event) error {
	query := `
		UPDATE projection_academias
		SET 
			total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, event.AggregateID)
	return err
}

// Query methods

// GetByID busca academia por ID na projeÃ§Ã£o
func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	query := `
		SELECT 
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE id = $1
	`

	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRowContext(p.ctx, query, id).Scan(
		&dto.ID,
		&dto.Type,
		&dto.Nome,
		&dto.CodigoAcademia,
		&dto.SenhaHash,
		&dto.Provincia,
		&dto.Endereco,
		&dto.NumeroTelefone,
		&dto.Email,
		&dto.Website,
		&dto.NivelEscolar,
		&dto.Status,
		&cursosJSON,
		&dto.CreatedAt,
		&dto.UpdatedAt,
		&dto.TotalEstudantes,
		&dto.TotalInscricoesPendentes,
		&dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Deserializar cursos
	if err := json.Unmarshal(cursosJSON, &dto.Cursos); err != nil {
		dto.Cursos = []string{}
	}

	return &dto, nil
}

// GetByCodigoOrEmail busca academia por cÃ³digo ou email
func (p *AcademiaProjection) GetByCodigoOrEmail(identifier string) (*AcademiaDTO, error) {
	log.Printf("ðŸ” [PROJEÃ‡ÃƒO ACADEMIA] GetByCodigoOrEmail: %s", identifier)
	
	query := `
		SELECT 
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = $1 OR email = $1
		LIMIT 1
	`

	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRowContext(p.ctx, query, identifier).Scan(
		&dto.ID,
		&dto.Type,
		&dto.Nome,
		&dto.CodigoAcademia,
		&dto.SenhaHash,
		&dto.Provincia,
		&dto.Endereco,
		&dto.NumeroTelefone,
		&dto.Email,
		&dto.Website,
		&dto.NivelEscolar,
		&dto.Status,
		&cursosJSON,
		&dto.CreatedAt,
		&dto.UpdatedAt,
		&dto.TotalEstudantes,
		&dto.TotalInscricoesPendentes,
		&dto.Version,
	)

	if err == sql.ErrNoRows {
		log.Printf("âŒ [PROJEÃ‡ÃƒO ACADEMIA] Nenhum registro encontrado")
		return nil, nil
	}
	if err != nil {
		log.Printf("âŒ [PROJEÃ‡ÃƒO ACADEMIA] Erro na query: %v", err)
		return nil, err
	}

	// Deserializar cursos
	if err := json.Unmarshal(cursosJSON, &dto.Cursos); err != nil {
		dto.Cursos = []string{}
	}

	log.Printf("âœ… [PROJEÃ‡ÃƒO ACADEMIA] Academia encontrada: %s", dto.Nome)
	return &dto, nil
}

// AcademiaDTO DTO da projeÃ§Ã£o
type AcademiaDTO struct {
	ID                       uuid.UUID  `json:"id"`
	Type                     string     `json:"type"`
	Nome                     string     `json:"nome"`
	CodigoAcademia           string     `json:"codigo_academia"`
	SenhaHash                string     `json:"-"`
	Provincia                string     `json:"provincia"`
	Endereco                 string     `json:"endereco"`
	NumeroTelefone           *string    `json:"numero_telefone,omitempty"`
	Email                    *string    `json:"email,omitempty"`
	Website                  *string    `json:"website,omitempty"`
	NivelEscolar             *string    `json:"nivel_escolar,omitempty"`
	Status                   string     `json:"status"`
	Cursos                   []string   `json:"cursos"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	TotalEstudantes          int        `json:"total_estudantes"`
	TotalInscricoesPendentes int        `json:"total_inscricoes_pendentes"`
	Version                  int        `json:"version"`
}

// ðŸ”¥ NOVO: GetByCodigo busca academia por cÃ³digo
func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	log.Printf("ðŸ” [PROJEÃ‡ÃƒO ACADEMIA] GetByCodigo: %s", codigo)
	
	query := `
		SELECT 
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = $1
		LIMIT 1
	`

	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRowContext(p.ctx, query, codigo).Scan(
		&dto.ID,
		&dto.Type,
		&dto.Nome,
		&dto.CodigoAcademia,
		&dto.SenhaHash,
		&dto.Provincia,
		&dto.Endereco,
		&dto.NumeroTelefone,
		&dto.Email,
		&dto.Website,
		&dto.NivelEscolar,
		&dto.Status,
		&cursosJSON,
		&dto.CreatedAt,
		&dto.UpdatedAt,
		&dto.TotalEstudantes,
		&dto.TotalInscricoesPendentes,
		&dto.Version,
	)

	if err == sql.ErrNoRows {
		log.Printf("âŒ [PROJEÃ‡ÃƒO ACADEMIA] Nenhum registro encontrado com cÃ³digo: %s", codigo)
		return nil, nil
	}
	if err != nil {
		log.Printf("âŒ [PROJEÃ‡ÃƒO ACADEMIA] Erro na query: %v", err)
		return nil, err
	}

	// Deserializar cursos
	if err := json.Unmarshal(cursosJSON, &dto.Cursos); err != nil {
		dto.Cursos = []string{}
	}

	log.Printf("âœ… [PROJEÃ‡ÃƒO ACADEMIA] Academia encontrada: %s (CÃ³digo: %s)", dto.Nome, dto.CodigoAcademia)
	return &dto, nil
}