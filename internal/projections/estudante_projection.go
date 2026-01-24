package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

type EstudanteProjection struct {
	client *db.Client
}

func NewEstudanteProjection(client *db.Client) *EstudanteProjection {
	return &EstudanteProjection{client: client}
}

func (p *EstudanteProjection) Name() string {
	return "estudantes"
}

func (p *EstudanteProjection) Handle(event db.Event) error {
	log.Printf("[ESTUDANTE_PROJECTION] Recebendo evento: type=%s, aggregate_id=%s, event_id=%s", 
		event.EventType, event.AggregateID, event.EventID)
	
	if event.AggregateType != "Estudante" {
		log.Printf("[ESTUDANTE_PROJECTION] Ignorando evento de tipo %s", event.AggregateType)
		return nil
	}

	switch event.EventType {
	case "EstudanteCriado":
		return p.handleEstudanteCriado(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "EstudanteVinculado":
		return p.handleEstudanteVinculado(event)
	case "StatusEscolarAtualizado":
		return p.handleStatusEscolarAtualizado(event)
	case "StatusSuperiorAtualizado":
		return p.handleStatusSuperiorAtualizado(event)
	case "DadosPessoaisAtualizados":
		return p.handleDadosPessoaisAtualizados(event)
	case "DadosAcademicosAtualizados":
		return p.handleDadosAcademicosAtualizados(event)
	default:
		log.Printf("[ESTUDANTE_PROJECTION] Tipo de evento desconhecido: %s", event.EventType)
		return nil
	}
}

func (p *EstudanteProjection) Rebuild() error {
	log.Printf("[ESTUDANTE_PROJECTION] Iniciando rebuild da projeção")
	
	if err := p.clear(); err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao limpar projeção: %v", err)
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Estudante'
		ORDER BY id ASC
	`
	
	log.Printf("[ESTUDANTE_PROJECTION] Executando query de rebuild")
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao executar query de rebuild: %v", err)
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
			log.Printf("[ESTUDANTE_PROJECTION] Erro ao fazer scan do evento: %v", err)
			return err
		}

		if err := p.Handle(event); err != nil {
			log.Printf("[ESTUDANTE_PROJECTION] Erro ao processar evento %d: %v", event.ID, err)
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
		eventCount++
	}

	log.Printf("[ESTUDANTE_PROJECTION] Rebuild concluído. %d eventos processados", eventCount)
	return rows.Err()
}

func (p *EstudanteProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(p.Name())
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	log.Printf("[ESTUDANTE_PROJECTION] Buscando último evento processado: %s", query)
	
	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	
	if err == sql.ErrNoRows {
		log.Printf("[ESTUDANTE_PROJECTION] Nenhum checkpoint encontrado, retornando 0")
		return 0, nil
	}
	
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao buscar checkpoint: %v", err)
	} else {
		log.Printf("[ESTUDANTE_PROJECTION] Último evento processado: %d", lastID)
	}
	
	return lastID, err
}

func (p *EstudanteProjection) UpdateCheckpoint(eventID int64) error {
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
	
	log.Printf("[ESTUDANTE_PROJECTION] Atualizando checkpoint para event_id=%d", eventID)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao atualizar checkpoint: %v", err)
	}
	return err
}

func (p *EstudanteProjection) clear() error {
	log.Printf("[ESTUDANTE_PROJECTION] Limpando tabela projection_estudantes")
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_estudantes CASCADE`)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao limpar tabela: %v", err)
	}
	return err
}

func (p *EstudanteProjection) handleEstudanteCriado(event db.Event) error {
	log.Printf("[ESTUDANTE_PROJECTION] Processando EstudanteCriado: event_id=%s", event.EventID)
	
	var payload struct {
		Nome                  string    `json:"Nome"`
		CodigoEstudante       string    `json:"CodigoEstudante"`
		SenhaHash             string    `json:"SenhaHash"`
		Email                 *string   `json:"Email"`
		Telefone              *string   `json:"Telefone"`
		BilheteIdentidade     *string   `json:"BilheteIdentidade"`
		BilheteIdentidadeResp *string   `json:"BilheteIdentidadeResp"`
		AnoEscolar            *string   `json:"AnoEscolar"`
		AnoSuperior           *string   `json:"AnoSuperior"`
		CursoMedio            *string   `json:"CursoMedio"`
		CursoSuperior         *string   `json:"CursoSuperior"`
		StatusEscolar         string    `json:"StatusEscolar"`
		StatusSuperior        string    `json:"StatusSuperior"`
		CreatedAt             time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[ESTUDANTE_PROJECTION] Dados do estudante: nome=%s, codigo=%s, status_escolar=%s, status_superior=%s", 
		payload.Nome, payload.CodigoEstudante, payload.StatusEscolar, payload.StatusSuperior)

	if payload.SenhaHash == "" || payload.CodigoEstudante == "" {
		log.Printf("[ESTUDANTE_PROJECTION] Dados obrigatórios vazios no evento")
		return fmt.Errorf("dados obrigatórios vazios no evento")
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ESTUDANTE_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeNome := db.SafeString(payload.Nome)
	safeCodigo := db.SafeString(payload.CodigoEstudante)
	safeHash := db.SafeString(payload.SenhaHash)
	safeStatusEsc := db.SafeString(payload.StatusEscolar)
	safeStatusSup := db.SafeString(payload.StatusSuperior)

	var emailStr string
	if payload.Email != nil {
		emailStr = fmt.Sprintf("'%s'", db.SafeString(*payload.Email))
		log.Printf("[ESTUDANTE_PROJECTION] Email: %s", *payload.Email)
	} else {
		emailStr = "NULL"
	}

	var telefoneStr string
	if payload.Telefone != nil {
		telefoneStr = fmt.Sprintf("'%s'", db.SafeString(*payload.Telefone))
		log.Printf("[ESTUDANTE_PROJECTION] Telefone: %s", *payload.Telefone)
	} else {
		telefoneStr = "NULL"
	}

	var bilheteStr, bilheteRespStr string
	if payload.BilheteIdentidade != nil {
		bilheteStr = fmt.Sprintf("'%s'", db.SafeString(*payload.BilheteIdentidade))
		log.Printf("[ESTUDANTE_PROJECTION] BI: %s", *payload.BilheteIdentidade)
	} else {
		bilheteStr = "NULL"
	}
	if payload.BilheteIdentidadeResp != nil {
		bilheteRespStr = fmt.Sprintf("'%s'", db.SafeString(*payload.BilheteIdentidadeResp))
		log.Printf("[ESTUDANTE_PROJECTION] BI Responsável: %s", *payload.BilheteIdentidadeResp)
	} else {
		bilheteRespStr = "NULL"
	}

	var anoEscStr, anoSupStr, cursoMedStr, cursoSupStr string
	if payload.AnoEscolar != nil {
		anoEscStr = fmt.Sprintf("'%s'", db.SafeString(*payload.AnoEscolar))
	} else {
		anoEscStr = "NULL"
	}
	if payload.AnoSuperior != nil {
		anoSupStr = fmt.Sprintf("'%s'", db.SafeString(*payload.AnoSuperior))
	} else {
		anoSupStr = "NULL"
	}
	if payload.CursoMedio != nil {
		cursoMedStr = fmt.Sprintf("'%s'", db.SafeString(*payload.CursoMedio))
	} else {
		cursoMedStr = "NULL"
	}
	if payload.CursoSuperior != nil {
		cursoSupStr = fmt.Sprintf("'%s'", db.SafeString(*payload.CursoSuperior))
	} else {
		cursoSupStr = "NULL"
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, 
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			version, created_at, updated_at, last_event_id
		) VALUES (
			'%s', '%s', '%s', '%s', %s, %s, FALSE, %s, %s, NULL,
			'inativo', '%s', '%s', %s, %s, %s, %s,
			%d, '%s', CURRENT_TIMESTAMP, '%s'
		)
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome, codigo_estudante = EXCLUDED.codigo_estudante,
			senha_hash = EXCLUDED.senha_hash, email = EXCLUDED.email,
			telefone = EXCLUDED.telefone, bilhete_identidade = EXCLUDED.bilhete_identidade,
			bilhete_identidade_responsavel = EXCLUDED.bilhete_identidade_responsavel,
			status = EXCLUDED.status, status_escolar = EXCLUDED.status_escolar,
			status_superior = EXCLUDED.status_superior, ano_escolar = EXCLUDED.ano_escolar,
			ano_superior = EXCLUDED.ano_superior, curso_medio = EXCLUDED.curso_medio,
			curso_superior = EXCLUDED.curso_superior, version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at, last_event_id = EXCLUDED.last_event_id
	`, aggID, safeNome, safeCodigo, safeHash, emailStr, telefoneStr, 
		bilheteStr, bilheteRespStr, safeStatusEsc, safeStatusSup,
		anoEscStr, anoSupStr, cursoMedStr, cursoSupStr,
		event.EventVersion, payload.CreatedAt.Format(time.RFC3339), event.EventID)

	log.Printf("[ESTUDANTE_PROJECTION] Executando insert/update: %s", query)

	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao processar EstudanteCriado (event_id: %s): %v", event.EventID, err)
		return err
	}

	log.Printf("[ESTUDANTE_PROJECTION] EstudanteCriado processado com sucesso: id=%s", aggID)
	return nil
}

func (p *EstudanteProjection) handleInscricaoAprovada(event db.Event) error {
	log.Printf("[ESTUDANTE_PROJECTION] Processando InscricaoAprovada: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ESTUDANTE_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s',
			total_inscricoes = total_inscricoes + 1
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ESTUDANTE_PROJECTION] Incrementando total_inscricoes: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao processar inscrição aprovada: %v", err)
	} else {
		log.Printf("[ESTUDANTE_PROJECTION] Inscrição aprovada processada com sucesso")
	}
	return err
}

func (p *EstudanteProjection) handleEstudanteVinculado(event db.Event) error {
	log.Printf("[ESTUDANTE_PROJECTION] Processando EstudanteVinculado: event_id=%s", event.EventID)
	
	var payload struct {
		CodigoAcademia string `json:"CodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[ESTUDANTE_PROJECTION] Vinculando à academia: %s", payload.CodigoAcademia)

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ESTUDANTE_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeCodigo := db.SafeString(payload.CodigoAcademia)

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET codigo_academia = '%s', status = 'ativo', version = %d,
			updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, safeCodigo, event.EventVersion, event.EventID, aggID)

	log.Printf("[ESTUDANTE_PROJECTION] Executando update: %s", query)

	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao vincular estudante: %v", err)
	} else {
		log.Printf("[ESTUDANTE_PROJECTION] Estudante %s vinculado com sucesso", aggID)
	}
	return err
}

func (p *EstudanteProjection) handleStatusEscolarAtualizado(event db.Event) error {
	log.Printf("[ESTUDANTE_PROJECTION] Processando StatusEscolarAtualizado: event_id=%s", event.EventID)
	
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[ESTUDANTE_PROJECTION] Novo status escolar: %s", payload.NovoStatus)

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ESTUDANTE_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeStatus := db.SafeString(payload.NovoStatus)

	if payload.NovoStatus == "inativo" {
		query := fmt.Sprintf(`
			UPDATE projection_estudantes
			SET status_escolar = '%s', status_superior = 'inativo', version = %d,
				updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
			WHERE id = '%s'
		`, safeStatus, event.EventVersion, event.EventID, aggID)
		
		log.Printf("[ESTUDANTE_PROJECTION] Desativando status escolar (também desativa superior): %s", query)
		
		_, err := p.client.DB().Exec(query)
		if err != nil {
			log.Printf("[ESTUDANTE_PROJECTION] Erro ao atualizar status: %v", err)
		}
		return err
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET status_escolar = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, safeStatus, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ESTUDANTE_PROJECTION] Executando update: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao atualizar status escolar: %v", err)
	} else {
		log.Printf("[ESTUDANTE_PROJECTION] Status escolar atualizado com sucesso")
	}
	return err
}

func (p *EstudanteProjection) handleStatusSuperiorAtualizado(event db.Event) error {
	log.Printf("[ESTUDANTE_PROJECTION] Processando StatusSuperiorAtualizado: event_id=%s", event.EventID)
	
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[ESTUDANTE_PROJECTION] Novo status superior: %s", payload.NovoStatus)

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ESTUDANTE_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeStatus := db.SafeString(payload.NovoStatus)

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET status_superior = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, safeStatus, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ESTUDANTE_PROJECTION] Executando update: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao atualizar status superior: %v", err)
	} else {
		log.Printf("[ESTUDANTE_PROJECTION] Status superior atualizado com sucesso")
	}
	return err
}

func (p *EstudanteProjection) handleDadosPessoaisAtualizados(event db.Event) error {
	log.Printf("[ESTUDANTE_PROJECTION] Processando DadosPessoaisAtualizados: event_id=%s", event.EventID)
	
	var payload struct {
		Nome                  *string `json:"Nome"`
		Email                 *string `json:"Email"`
		Telefone              *string `json:"Telefone"`
		BilheteIdentidade     *string `json:"BilheteIdentidade"`
		BilheteIdentidadeResp *string `json:"BilheteIdentidadeResp"`
		EmailAlterado         bool    `json:"EmailAlterado"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ESTUDANTE_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	if payload.Nome != nil {
		safeNome := db.SafeString(*payload.Nome)
		query := fmt.Sprintf(`UPDATE projection_estudantes SET nome = '%s' WHERE id = '%s'`, safeNome, aggID)
		log.Printf("[ESTUDANTE_PROJECTION] Atualizando nome: %s", query)
		p.client.DB().Exec(query)
	}
	
	if payload.Email != nil {
		safeEmail := db.SafeString(*payload.Email)
		if payload.EmailAlterado {
			query := fmt.Sprintf(`UPDATE projection_estudantes SET email = '%s', email_verificado = FALSE WHERE id = '%s'`, safeEmail, aggID)
			log.Printf("[ESTUDANTE_PROJECTION] Atualizando email (resetando verificação): %s", query)
			p.client.DB().Exec(query)
		} else {
			query := fmt.Sprintf(`UPDATE projection_estudantes SET email = '%s' WHERE id = '%s'`, safeEmail, aggID)
			log.Printf("[ESTUDANTE_PROJECTION] Atualizando email: %s", query)
			p.client.DB().Exec(query)
		}
	}
	
	if payload.Telefone != nil {
		safeTel := db.SafeString(*payload.Telefone)
		query := fmt.Sprintf(`UPDATE projection_estudantes SET telefone = '%s' WHERE id = '%s'`, safeTel, aggID)
		log.Printf("[ESTUDANTE_PROJECTION] Atualizando telefone: %s", query)
		p.client.DB().Exec(query)
	}
	
	if payload.BilheteIdentidade != nil {
		safeBi := db.SafeString(*payload.BilheteIdentidade)
		query := fmt.Sprintf(`UPDATE projection_estudantes SET bilhete_identidade = '%s' WHERE id = '%s'`, safeBi, aggID)
		log.Printf("[ESTUDANTE_PROJECTION] Atualizando BI: %s", query)
		p.client.DB().Exec(query)
	}
	
	if payload.BilheteIdentidadeResp != nil {
		safeBiResp := db.SafeString(*payload.BilheteIdentidadeResp)
		query := fmt.Sprintf(`UPDATE projection_estudantes SET bilhete_identidade_responsavel = '%s' WHERE id = '%s'`, safeBiResp, aggID)
		log.Printf("[ESTUDANTE_PROJECTION] Atualizando BI responsável: %s", query)
		p.client.DB().Exec(query)
	}

	updateQuery := fmt.Sprintf(`
		UPDATE projection_estudantes SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s' WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ESTUDANTE_PROJECTION] Atualizando version e timestamp: %s", updateQuery)
	
	_, err := p.client.DB().Exec(updateQuery)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao atualizar dados pessoais: %v", err)
	} else {
		log.Printf("[ESTUDANTE_PROJECTION] Dados pessoais atualizados com sucesso")
	}
	return err
}

func (p *EstudanteProjection) handleDadosAcademicosAtualizados(event db.Event) error {
	log.Printf("[ESTUDANTE_PROJECTION] Processando DadosAcademicosAtualizados: event_id=%s", event.EventID)
	
	var payload struct {
		AnoEscolar    *string `json:"AnoEscolar"`
		AnoSuperior   *string `json:"AnoSuperior"`
		CursoMedio    *string `json:"CursoMedio"`
		CursoSuperior *string `json:"CursoSuperior"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ESTUDANTE_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	if payload.AnoEscolar != nil {
		safe := db.SafeString(*payload.AnoEscolar)
		query := fmt.Sprintf(`UPDATE projection_estudantes SET ano_escolar = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ESTUDANTE_PROJECTION] Atualizando ano_escolar: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.AnoSuperior != nil {
		safe := db.SafeString(*payload.AnoSuperior)
		query := fmt.Sprintf(`UPDATE projection_estudantes SET ano_superior = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ESTUDANTE_PROJECTION] Atualizando ano_superior: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.CursoMedio != nil {
		safe := db.SafeString(*payload.CursoMedio)
		query := fmt.Sprintf(`UPDATE projection_estudantes SET curso_medio = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ESTUDANTE_PROJECTION] Atualizando curso_medio: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.CursoSuperior != nil {
		safe := db.SafeString(*payload.CursoSuperior)
		query := fmt.Sprintf(`UPDATE projection_estudantes SET curso_superior = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ESTUDANTE_PROJECTION] Atualizando curso_superior: %s", query)
		p.client.DB().Exec(query)
	}

	updateQuery := fmt.Sprintf(`
		UPDATE projection_estudantes SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s' WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ESTUDANTE_PROJECTION] Atualizando version e timestamp: %s", updateQuery)
	
	_, err := p.client.DB().Exec(updateQuery)
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao atualizar dados acadêmicos: %v", err)
	} else {
		log.Printf("[ESTUDANTE_PROJECTION] Dados acadêmicos atualizados com sucesso")
	}
	return err
}

func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	log.Printf("[ESTUDANTE_PROJECTION] Buscando estudante por ID: %s", id)
	
	if id == uuid.Nil {
		log.Printf("[ESTUDANTE_PROJECTION] UUID inválido fornecido")
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, nome, codigo_estudante, senha_hash, 
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes WHERE id = '%s'
	`, id)

	log.Printf("[ESTUDANTE_PROJECTION] Executando query: %s", query)

	var dto EstudanteDTO
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.TotalNotas, &dto.TotalFaltas,
		&dto.TotalInscricoes, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		log.Printf("[ESTUDANTE_PROJECTION] Estudante não encontrado: %s", id)
		return nil, nil
	}
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao buscar estudante: %v", err)
		return nil, err
	}
	
	log.Printf("[ESTUDANTE_PROJECTION] Estudante encontrado: %s (%s)", dto.Nome, dto.CodigoEstudante)
	return &dto, err
}

func (p *EstudanteProjection) GetByCodigo(codigo string) (*EstudanteDTO, error) {
	log.Printf("[ESTUDANTE_PROJECTION] Buscando estudante por código: %s", codigo)
	
	safeCodigo := db.SafeString(codigo)

	query := fmt.Sprintf(`
		SELECT id, nome, codigo_estudante, senha_hash, 
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes WHERE codigo_estudante = '%s'
	`, safeCodigo)

	log.Printf("[ESTUDANTE_PROJECTION] Executando query: %s", query)

	var dto EstudanteDTO
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.TotalNotas, &dto.TotalFaltas,
		&dto.TotalInscricoes, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		log.Printf("[ESTUDANTE_PROJECTION] Estudante não encontrado com código: %s", codigo)
		return nil, nil
	}
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao buscar estudante: %v", err)
		return nil, err
	}
	
	log.Printf("[ESTUDANTE_PROJECTION] Estudante encontrado: %s (%s)", dto.Nome, dto.CodigoEstudante)
	return &dto, err
}

func (p *EstudanteProjection) GetByBilhete(bilhete string) (*EstudanteDTO, error) {
	log.Printf("[ESTUDANTE_PROJECTION] Buscando estudante por BI: %s", bilhete)
	
	safeBilhete := db.SafeString(bilhete)

	query := fmt.Sprintf(`
		SELECT id, nome, codigo_estudante, senha_hash, 
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE bilhete_identidade = '%s' OR bilhete_identidade_responsavel = '%s'
		LIMIT 1
	`, safeBilhete, safeBilhete)

	log.Printf("[ESTUDANTE_PROJECTION] Executando query: %s", query)

	var dto EstudanteDTO
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.TotalNotas, &dto.TotalFaltas,
		&dto.TotalInscricoes, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		log.Printf("[ESTUDANTE_PROJECTION] Estudante não encontrado com BI: %s", bilhete)
		return nil, nil
	}
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao buscar estudante: %v", err)
		return nil, err
	}
	
	log.Printf("[ESTUDANTE_PROJECTION] Estudante encontrado: %s (%s)", dto.Nome, dto.CodigoEstudante)
	return &dto, err
}

func (p *EstudanteProjection) GetByBilheteIdentidadePrincipal(bilhete string) (*EstudanteDTO, error) {
	log.Printf("[ESTUDANTE_PROJECTION] Buscando estudante por BI principal: %s", bilhete)
	
	safeBilhete := db.SafeString(bilhete)

	query := fmt.Sprintf(`
		SELECT id, nome, codigo_estudante, senha_hash, 
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE bilhete_identidade = '%s'
		LIMIT 1
	`, safeBilhete)

	log.Printf("[ESTUDANTE_PROJECTION] Executando query: %s", query)

	var dto EstudanteDTO
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.TotalNotas, &dto.TotalFaltas,
		&dto.TotalInscricoes, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		log.Printf("[ESTUDANTE_PROJECTION] Estudante não encontrado com BI principal: %s", bilhete)
		return nil, nil
	}
	if err != nil {
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao buscar estudante: %v", err)
		return nil, err
	}
	
	log.Printf("[ESTUDANTE_PROJECTION] Estudante encontrado: %s (%s)", dto.Nome, dto.CodigoEstudante)
	return &dto, nil
}

type EstudanteDTO struct {
	ID                    uuid.UUID `db:"id" json:"id"`
	Nome                  string    `db:"nome" json:"nome"`
	CodigoEstudante       string    `db:"codigo_estudante" json:"codigo_estudante"`
	SenhaHash             string    `db:"senha_hash" json:"-"`
	Email                 *string   `db:"email" json:"email,omitempty"`
	Telefone              *string   `db:"telefone" json:"telefone,omitempty"`
	EmailVerificado       bool      `db:"email_verificado" json:"email_verificado"`
	BilheteIdentidade     *string   `db:"bilhete_identidade" json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp *string   `db:"bilhete_identidade_responsavel" json:"bilhete_identidade_responsavel,omitempty"`
	CodigoAcademia        *string   `db:"codigo_academia" json:"codigo_academia,omitempty"`
	Status                string    `db:"status" json:"status"`
	StatusEscolar         string    `db:"status_escolar" json:"status_escolar"`
	StatusSuperior        string    `db:"status_superior" json:"status_superior"`
	AnoEscolar            *string   `db:"ano_escolar" json:"ano_escolar,omitempty"`
	AnoSuperior           *string   `db:"ano_superior" json:"ano_superior,omitempty"`
	CursoMedio            *string   `db:"curso_medio" json:"curso_medio,omitempty"`
	CursoSuperior         *string   `db:"curso_superior" json:"curso_superior,omitempty"`
	CreatedAt             time.Time `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time `db:"updated_at" json:"updated_at"`
	TotalNotas            int       `db:"total_notas" json:"total_notas"`
	TotalFaltas           int       `db:"total_faltas" json:"total_faltas"`
	TotalInscricoes       int       `db:"total_inscricoes" json:"total_inscricoes"`
	Version               int       `db:"version" json:"version"`
}