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

type AcademiaProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewAcademiaProjection(client *db.Client) *AcademiaProjection {
	return &AcademiaProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *AcademiaProjection) Name() string {
	return "academias"
}

func (p *AcademiaProjection) Handle(event db.Event) error {
	log.Printf("[ACADEMIA_PROJECTION] Recebendo evento: type=%s, aggregate_id=%s, event_id=%s", 
		event.EventType, event.AggregateID, event.EventID)
	
	if event.AggregateType != "Academia" {
		log.Printf("[ACADEMIA_PROJECTION] Ignorando evento de tipo %s", event.AggregateType)
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
	case "AcademiaDadosAtualizados":
		return p.handleAcademiaDadosAtualizados(event)
	default:
		log.Printf("[ACADEMIA_PROJECTION] Tipo de evento desconhecido: %s", event.EventType)
		return nil
	}
}

func (p *AcademiaProjection) Rebuild() error {
	log.Printf("[ACADEMIA_PROJECTION] Iniciando rebuild da projeção")
	
	if err := p.clear(); err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao limpar projeção: %v", err)
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Academia'
		ORDER BY id ASC
	`
	
	log.Printf("[ACADEMIA_PROJECTION] Executando query de rebuild")
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao executar query de rebuild: %v", err)
		return err
	}
	defer rows.Close()

	eventCount := 0
	for rows.Next() {
		var event db.Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			log.Printf("[ACADEMIA_PROJECTION] Erro ao fazer scan do evento: %v", err)
			return err
		}

		if err := p.Handle(event); err != nil {
			log.Printf("[ACADEMIA_PROJECTION] Erro ao processar evento %d: %v", event.ID, err)
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
		eventCount++
	}

	log.Printf("[ACADEMIA_PROJECTION] Rebuild concluído. %d eventos processados", eventCount)
	return rows.Err()
}

func (p *AcademiaProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(p.Name())
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	log.Printf("[ACADEMIA_PROJECTION] Buscando último evento processado: %s", query)
	
	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	
	if err == sql.ErrNoRows {
		log.Printf("[ACADEMIA_PROJECTION] Nenhum checkpoint encontrado, retornando 0")
		return 0, nil
	}
	
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao buscar checkpoint: %v", err)
	} else {
		log.Printf("[ACADEMIA_PROJECTION] Último evento processado: %d", lastID)
	}
	
	return lastID, err
}

func (p *AcademiaProjection) UpdateCheckpoint(eventID int64) error {
	safeName := db.SafeString(p.Name())
	eventID = int64(db.ValidateOffset(int(eventID)))
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed
		) VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, safeName, eventID, eventID)
	
	log.Printf("[ACADEMIA_PROJECTION] Atualizando checkpoint para event_id=%d", eventID)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao atualizar checkpoint: %v", err)
	}
	return err
}

func (p *AcademiaProjection) clear() error {
	log.Printf("[ACADEMIA_PROJECTION] Limpando tabela projection_academias")
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_academias CASCADE`)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao limpar tabela: %v", err)
	}
	return err
}

func (p *AcademiaProjection) handleAcademiaCriada(event db.Event) error {
	log.Printf("[ACADEMIA_PROJECTION] Processando AcademiaCriada: event_id=%s", event.EventID)
	
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
		log.Printf("[ACADEMIA_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[ACADEMIA_PROJECTION] Dados da academia: nome=%s, codigo=%s, type=%s, provincia=%s", 
		payload.Nome, payload.CodigoAcademia, payload.Type, payload.Provincia)

	if payload.SenhaHash == "" {
		log.Printf("[ACADEMIA_PROJECTION] SenhaHash vazio no evento")
		return fmt.Errorf("SenhaHash vazio no evento")
	}

	cursosJSON, err := json.Marshal(payload.Cursos)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao serializar cursos: %v", err)
		return err
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ACADEMIA_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeType := db.SafeString(payload.Type)
	safeNome := db.SafeString(payload.Nome)
	safeCodigo := db.SafeString(payload.CodigoAcademia)
	safeHash := db.SafeString(payload.SenhaHash)
	safeProv := db.SafeString(payload.Provincia)
	safeEnd := db.SafeString(payload.Endereco)
	safeCursos := db.SafeString(string(cursosJSON))

	var telStr, emailStr, webStr, nivelStr string
	if payload.NumeroTelefone != nil {
		telStr = fmt.Sprintf("'%s'", db.SafeString(*payload.NumeroTelefone))
		log.Printf("[ACADEMIA_PROJECTION] Telefone: %s", *payload.NumeroTelefone)
	} else {
		telStr = "NULL"
	}
	if payload.Email != nil {
		emailStr = fmt.Sprintf("'%s'", db.SafeString(*payload.Email))
		log.Printf("[ACADEMIA_PROJECTION] Email: %s", *payload.Email)
	} else {
		emailStr = "NULL"
	}
	if payload.Website != nil {
		webStr = fmt.Sprintf("'%s'", db.SafeString(*payload.Website))
		log.Printf("[ACADEMIA_PROJECTION] Website: %s", *payload.Website)
	} else {
		webStr = "NULL"
	}
	if payload.NivelEscolar != nil {
		nivelStr = fmt.Sprintf("'%s'", db.SafeString(*payload.NivelEscolar))
		log.Printf("[ACADEMIA_PROJECTION] Nível escolar: %s", *payload.NivelEscolar)
	} else {
		nivelStr = "NULL"
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_academias (
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, version, created_at, updated_at, last_event_id
		) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', %s, %s, %s, %s, 'inativo', '%s', %d, '%s', CURRENT_TIMESTAMP, '%s')
		ON CONFLICT (id) DO UPDATE SET
			type = EXCLUDED.type, nome = EXCLUDED.nome,
			codigo_academia = EXCLUDED.codigo_academia, senha_hash = EXCLUDED.senha_hash,
			provincia = EXCLUDED.provincia, endereco = EXCLUDED.endereco,
			numero_telefone = EXCLUDED.numero_telefone, email = EXCLUDED.email,
			website = EXCLUDED.website, nivel_escolar = EXCLUDED.nivel_escolar,
			cursos = EXCLUDED.cursos, version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at, last_event_id = EXCLUDED.last_event_id
	`, aggID, safeType, safeNome, safeCodigo, safeHash, safeProv, safeEnd,
		telStr, emailStr, webStr, nivelStr, safeCursos,
		event.EventVersion, payload.CreatedAt.Format(time.RFC3339), event.EventID)
	
	log.Printf("[ACADEMIA_PROJECTION] Executando insert/update: %s", query)
	
	_, err = p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao processar AcademiaCriada (event_id: %s): %v", event.EventID, err)
	} else {
		log.Printf("[ACADEMIA_PROJECTION] AcademiaCriada processada com sucesso: id=%s", aggID)
	}
	return err
}

func (p *AcademiaProjection) handleAcademiaAtivada(event db.Event) error {
	log.Printf("[ACADEMIA_PROJECTION] Processando AcademiaAtivada: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ACADEMIA_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET status = 'ativo', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ACADEMIA_PROJECTION] Executando update: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao ativar academia %s: %v", aggID, err)
	} else {
		log.Printf("[ACADEMIA_PROJECTION] Academia %s ativada com sucesso", aggID)
	}
	return err
}

func (p *AcademiaProjection) handleAcademiaDesativada(event db.Event) error {
	log.Printf("[ACADEMIA_PROJECTION] Processando AcademiaDesativada: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ACADEMIA_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET status = 'inativo', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ACADEMIA_PROJECTION] Executando update: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao desativar academia %s: %v", aggID, err)
	} else {
		log.Printf("[ACADEMIA_PROJECTION] Academia %s desativada com sucesso", aggID)
	}
	return err
}

func (p *AcademiaProjection) handleCursosAtualizados(event db.Event) error {
	log.Printf("[ACADEMIA_PROJECTION] Processando CursosAtualizados: event_id=%s", event.EventID)
	
	var payload struct {
		NovoCursos []string `json:"NovoCursos"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[ACADEMIA_PROJECTION] Novos cursos: %v", payload.NovoCursos)

	cursosJSON, err := json.Marshal(payload.NovoCursos)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao serializar cursos: %v", err)
		return err
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ACADEMIA_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeCursos := db.SafeString(string(cursosJSON))

	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET cursos = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, safeCursos, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ACADEMIA_PROJECTION] Executando update: %s", query)
	
	_, err = p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao atualizar cursos: %v", err)
	} else {
		log.Printf("[ACADEMIA_PROJECTION] Cursos atualizados com sucesso para academia %s", aggID)
	}
	return err
}

func (p *AcademiaProjection) handleInscricaoAprovada(event db.Event) error {
	log.Printf("[ACADEMIA_PROJECTION] Processando InscricaoAprovada: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ACADEMIA_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET total_estudantes = total_estudantes + 1,
			total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, aggID)
	
	log.Printf("[ACADEMIA_PROJECTION] Incrementando total_estudantes para academia %s", aggID)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao processar inscrição aprovada: %v", err)
	} else {
		log.Printf("[ACADEMIA_PROJECTION] Inscrição aprovada processada com sucesso")
	}
	return err
}

func (p *AcademiaProjection) handleInscricaoReprovada(event db.Event) error {
	log.Printf("[ACADEMIA_PROJECTION] Processando InscricaoReprovada: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ACADEMIA_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, aggID)
	
	log.Printf("[ACADEMIA_PROJECTION] Decrementando total_inscricoes_pendentes para academia %s", aggID)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao processar inscrição reprovada: %v", err)
	} else {
		log.Printf("[ACADEMIA_PROJECTION] Inscrição reprovada processada com sucesso")
	}
	return err
}

func (p *AcademiaProjection) handleAcademiaDadosAtualizados(event db.Event) error {
	log.Printf("[ACADEMIA_PROJECTION] Processando AcademiaDadosAtualizados: event_id=%s", event.EventID)
	
	var payload struct {
		Nome           *string  `json:"Nome"`
		Provincia      *string  `json:"Provincia"`
		Endereco       *string  `json:"Endereco"`
		NumeroTelefone *string  `json:"NumeroTelefone"`
		Email          *string  `json:"Email"`
		Website        *string  `json:"Website"`
		NivelEscolar   *string  `json:"NivelEscolar"`
		Cursos         []string `json:"Cursos"`
		EmailAlterado  bool     `json:"EmailAlterado"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ACADEMIA_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	if payload.Nome != nil {
		safe := db.SafeString(*payload.Nome)
		query := fmt.Sprintf(`UPDATE projection_academias SET nome = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ACADEMIA_PROJECTION] Atualizando nome: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.Provincia != nil {
		safe := db.SafeString(*payload.Provincia)
		query := fmt.Sprintf(`UPDATE projection_academias SET provincia = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ACADEMIA_PROJECTION] Atualizando provincia: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.Endereco != nil {
		safe := db.SafeString(*payload.Endereco)
		query := fmt.Sprintf(`UPDATE projection_academias SET endereco = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ACADEMIA_PROJECTION] Atualizando endereco: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.NumeroTelefone != nil {
		safe := db.SafeString(*payload.NumeroTelefone)
		query := fmt.Sprintf(`UPDATE projection_academias SET numero_telefone = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ACADEMIA_PROJECTION] Atualizando telefone: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.Email != nil {
		safe := db.SafeString(*payload.Email)
		if payload.EmailAlterado {
			query := fmt.Sprintf(`UPDATE projection_academias SET email = '%s', email_verificado = FALSE WHERE id = '%s'`, safe, aggID)
			log.Printf("[ACADEMIA_PROJECTION] Atualizando email (resetando verificação): %s", query)
			p.client.DB().Exec(query)
		} else {
			query := fmt.Sprintf(`UPDATE projection_academias SET email = '%s' WHERE id = '%s'`, safe, aggID)
			log.Printf("[ACADEMIA_PROJECTION] Atualizando email: %s", query)
			p.client.DB().Exec(query)
		}
	}
	if payload.Website != nil {
		safe := db.SafeString(*payload.Website)
		query := fmt.Sprintf(`UPDATE projection_academias SET website = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ACADEMIA_PROJECTION] Atualizando website: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.NivelEscolar != nil {
		safe := db.SafeString(*payload.NivelEscolar)
		query := fmt.Sprintf(`UPDATE projection_academias SET nivel_escolar = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ACADEMIA_PROJECTION] Atualizando nivel_escolar: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.Cursos != nil {
		cursosJSON, _ := json.Marshal(payload.Cursos)
		safe := db.SafeString(string(cursosJSON))
		query := fmt.Sprintf(`UPDATE projection_academias SET cursos = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ACADEMIA_PROJECTION] Atualizando cursos: %s", query)
		p.client.DB().Exec(query)
	}

	query := fmt.Sprintf(`
		UPDATE projection_academias SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s' WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ACADEMIA_PROJECTION] Atualizando version e timestamp: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao atualizar dados: %v", err)
	} else {
		log.Printf("[ACADEMIA_PROJECTION] Dados da academia %s atualizados com sucesso", aggID)
	}
	return err
}

func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	log.Printf("[ACADEMIA_PROJECTION] Buscando academia por ID: %s", id)
	
	if id == uuid.Nil {
		log.Printf("[ACADEMIA_PROJECTION] UUID inválido fornecido")
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias WHERE id = '%s'`, id)
	
	log.Printf("[ACADEMIA_PROJECTION] Executando query: %s", query)
	
	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia, &dto.SenhaHash, &dto.Provincia,
		&dto.Endereco, &dto.NumeroTelefone, &dto.Email, &dto.Website, &dto.NivelEscolar,
		&dto.Status, &cursosJSON, &dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		log.Printf("[ACADEMIA_PROJECTION] Academia não encontrada: %s", id)
		return nil, nil
	}
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao buscar academia: %v", err)
		return nil, err
	}

	json.Unmarshal(cursosJSON, &dto.Cursos)
	
	log.Printf("[ACADEMIA_PROJECTION] Academia encontrada: %s (%s)", dto.Nome, dto.CodigoAcademia)
	return &dto, nil
}

func (p *AcademiaProjection) GetByCodigoOrEmail(identifier string) (*AcademiaDTO, error) {
	log.Printf("[ACADEMIA_PROJECTION] Buscando academia por código ou email: %s", identifier)
	
	safeId := db.SafeString(identifier)

	query := fmt.Sprintf(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = '%s' OR email = '%s'
		LIMIT 1`, safeId, safeId)
	
	log.Printf("[ACADEMIA_PROJECTION] Executando query: %s", query)
	
	var dto AcademiaDTO
	var cursosJSON []byte
	
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia, &dto.SenhaHash, &dto.Provincia,
		&dto.Endereco, &dto.NumeroTelefone, &dto.Email, &dto.Website, &dto.NivelEscolar,
		&dto.Status, &cursosJSON, &dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		log.Printf("[ACADEMIA_PROJECTION] Academia não encontrada com identificador: %s", identifier)
		return nil, nil
	}
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao buscar academia: %v", err)
		return nil, err
	}

	json.Unmarshal(cursosJSON, &dto.Cursos)

	log.Printf("[ACADEMIA_PROJECTION] Academia encontrada: %s (%s)", dto.Nome, dto.CodigoAcademia)
	return &dto, nil
}

func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	log.Printf("[ACADEMIA_PROJECTION] Buscando academia por código: %s", codigo)
	
	safeCodigo := db.SafeString(codigo)

	query := fmt.Sprintf(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias WHERE codigo_academia = '%s'`, safeCodigo)
	
	log.Printf("[ACADEMIA_PROJECTION] Executando query: %s", query)
	
	var dto AcademiaDTO
	var cursosJSON []byte
	
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia, &dto.SenhaHash, &dto.Provincia,
		&dto.Endereco, &dto.NumeroTelefone, &dto.Email, &dto.Website, &dto.NivelEscolar,
		&dto.Status, &cursosJSON, &dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		log.Printf("[ACADEMIA_PROJECTION] Academia não encontrada com código: %s", codigo)
		return nil, nil
	}
	if err != nil {
		log.Printf("[ACADEMIA_PROJECTION] Erro ao buscar academia: %v", err)
		return nil, err
	}

	json.Unmarshal(cursosJSON, &dto.Cursos)

	log.Printf("[ACADEMIA_PROJECTION] Academia encontrada: %s (%s)", dto.Nome, dto.CodigoAcademia)
	return &dto, nil
}

type AcademiaDTO struct {
	ID                       uuid.UUID `json:"id" db:"id"`
	Type                     string    `json:"type" db:"type"`
	Nome                     string    `json:"nome" db:"nome"`
	CodigoAcademia           string    `json:"codigo_academia" db:"codigo_academia"`
	SenhaHash                string    `json:"-" db:"senha_hash"`
	Provincia                string    `json:"provincia" db:"provincia"`
	Endereco                 string    `json:"endereco" db:"endereco"`
	NumeroTelefone           *string   `json:"numero_telefone,omitempty" db:"numero_telefone"`
	Email                    *string   `json:"email,omitempty" db:"email"`
	Website                  *string   `json:"website,omitempty" db:"website"`
	NivelEscolar             *string   `json:"nivel_escolar,omitempty" db:"nivel_escolar"`
	Status                   string    `json:"status" db:"status"`
	Cursos                   []string  `json:"cursos" db:"cursos"`
	CreatedAt                time.Time `json:"created_at" db:"created_at"`
	UpdatedAt                time.Time `json:"updated_at" db:"updated_at"`
	TotalEstudantes          int       `json:"total_estudantes" db:"total_estudantes"`
	TotalInscricoesPendentes int       `json:"total_inscricoes_pendentes" db:"total_inscricoes_pendentes"`
	Version                  int       `json:"version" db:"version"`
}