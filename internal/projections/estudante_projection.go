package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"strings"
	"time"

	"github.com/google/uuid"
)

type EstudanteProjection struct {
	client *db.Client
}

func NewEstudanteProjection(client *db.Client) *EstudanteProjection {
	return &EstudanteProjection{client: client}
}

func (p *EstudanteProjection) Name() string { return "estudantes" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *EstudanteProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *EstudanteProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *EstudanteProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}
	switch event.EventType {
	case "EstudanteCriadoComVinculo":
		return p.handleEstudanteCriadoComVinculo(event)
	case "StatusEscolarFundamentalAtualizado":
		return p.handleStatusEscolarFundamental(event)
	case "StatusEscolarMedioAtualizado":
		return p.handleStatusEscolarMedio(event)
	case "StatusEscolarAtualizado":
		return p.handleStatusEscolarAtualizado(event)
	case "StatusSuperiorAtualizado":
		return p.handleStatusSuperiorAtualizado(event)
	case "MatriculaFundamentalEfetivada", "FundamentalRetomado":
		return p.handleStatusIndireto(event, "status_escolar_fundamental", "em_andamento")
	case "FundamentalInterrompido":
		return p.handleStatusIndireto(event, "status_escolar_fundamental", "inativo")
	case "MatriculaMedioEfetivada", "MedioRetomado":
		return p.handleStatusIndireto(event, "status_escolar_medio", "em_andamento")
	case "MedioInterrompido":
		return p.handleStatusIndireto(event, "status_escolar_medio", "inativo")
	case "MatriculaSuperiorEfetivada", "SuperiorReaberto":
		return p.handleStatusIndireto(event, "status_superior", "em_andamento")
	case "SuperiorTrancado":
		return p.handleStatusIndireto(event, "status_superior", "inativo")
	case "EstudanteArquivado":
		return p.handleStatusIndireto(event, "status", "inativo")
	case "EstudanteReativado":
		return p.handleStatusIndireto(event, "status", "ativo")
	case "DadosPessoaisAtualizados":
		return p.handleDadosPessoaisAtualizados(event)
	case "DadosAcademicosAtualizados":
		return p.handleDadosAcademicosAtualizados(event)
	case "EmailVerificadoEstudante":
		return p.handleEmailVerificadoEstudante(event)
	case "CursoAlterado":
		return p.handleCursoAlterado(event)
	case "SenhaAlterada":
		return p.handleSenhaAlterada(event)
	case "AvaliacaoFinalEscolar", "AvaliacaoFinalSuperior":
		return p.handleAvaliacaoFinalAnoAcademico(event)
	case "NotasRegistradas", "FaltasRegistradas":
		return p.handleVersionOnly(event)
	}
	return nil
}

// ============================================================================
// Rebuild
// ============================================================================

func (p *EstudanteProjection) Rebuild() error {
	log.Printf("[DEBUG] [estudantes] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar projection_estudantes: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Estudante'
		ORDER BY id ASC
	`)
	if err != nil {
		return fmt.Errorf("erro ao buscar eventos para rebuild: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var event db.Event
		var prevHash sql.NullString
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		); err != nil {
			return fmt.Errorf("erro ao escanear evento %d: %w", count, err)
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d (type=%s): %w", event.ID, event.EventType, err)
		}
		count++
	}

	log.Printf("[DEBUG] [estudantes] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

func (p *EstudanteProjection) clear() error {
	_, err := p.client.DB().Exec(`DELETE FROM projection_estudantes`)
	return err
}

// ============================================================================
// Handlers de eventos
// ============================================================================

func (p *EstudanteProjection) handleEstudanteCriadoComVinculo(event db.Event) error {
	var payload struct {
		Nome                     string     `json:"Nome"`
		CodigoEstudante          string     `json:"CodigoEstudante"`
		SenhaHash                string     `json:"SenhaHash"`
		Email                    *string    `json:"Email"`
		Telefone                 *string    `json:"Telefone"`
		BilheteIdentidade        *string    `json:"BilheteIdentidade"`
		BilheteIdentidadeResp    *string    `json:"BilheteIdentidadeResp"`
		Genero                   string     `json:"Genero"`
		DataNascimento           time.Time  `json:"DataNascimento"`
		StatusEscolarFundamental string     `json:"StatusEscolarFundamental"`
		StatusEscolarMedio       string     `json:"StatusEscolarMedio"`
		StatusSuperior           string     `json:"StatusSuperior"`
		AnoEscolar               *string    `json:"AnoEscolar"`
		AnoEscolarMedio          *string    `json:"AnoEscolarMedio"`
		AnoSuperior              *string    `json:"AnoSuperior"`
		SemestreAtual            *int       `json:"SemestreAtual"`
		CursoMedioID             *uuid.UUID `json:"CursoMedioID"`
		CursoSuperiorID          *uuid.UUID `json:"CursoSuperiorID"`
		CodigoAcademia           string     `json:"CodigoAcademia"`
		CreatedAt                time.Time  `json:"CreatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteCriadoComVinculo: parse error: %w", err)
	}

	// FIX BUG #1: as variáveis cursoMedioIDStr e cursoSuperiorIDStr são *string
	// para o banco. O código original usava `:=` em resolveExistingCursoIDs, o
	// que sombreava as variáveis locais sem efeito visível — mas o problema real
	// era outro: a FK de codigo_academia pode falhar se a projeção de academias
	// ainda não processou o evento AcademiaCriada (race entre projeções).
	// Solução: verificar existência de academia antes do INSERT e retornar nil
	// (não erro) para não travar o pipeline — o evento será reprocessado no
	// próximo ciclo de polling.
	var cursoMedioIDStr, cursoSuperiorIDStr *string
	if payload.CursoMedioID != nil {
		s := payload.CursoMedioID.String()
		cursoMedioIDStr = &s
	}
	if payload.CursoSuperiorID != nil {
		s := payload.CursoSuperiorID.String()
		cursoSuperiorIDStr = &s
	}

	// FIX BUG #2 (crítico): verificar se a academia já foi projetada.
	// O manager processa projeções de forma independente e paralela.
	// É possível que o evento EstudanteCriadoComVinculo chegue para processamento
	// ANTES do evento AcademiaCriada ter sido processado pela projeção de academias.
	// Se projection_estudantes tiver FK para projection_academias (via codigo_academia),
	// o INSERT falha com FK violation, o handler retorna erro, o manager retenta 3x,
	// e depois PARA de avançar o checkpoint — travando TODOS os eventos seguintes.
	// Solução: verificar a existência e, se não existir ainda, retornar nil
	// (o evento será reprocessado no próximo tick do polling, ~1s depois).
	academiaExists, err := p.academiaExists(payload.CodigoAcademia)
	if err != nil {
		// Erro de banco ao verificar — propagar para retry
		return fmt.Errorf("handleEstudanteCriadoComVinculo: erro ao verificar academia: %w", err)
	}
	if !academiaExists {
		// Academia ainda não foi projetada — não é erro, só timing.
		// Retornar nil faz o checkpoint NÃO avançar (o event ID não muda),
		// então na próxima rodada de polling este evento será reprocessado.
		// IMPORTANTE: para que o checkpoint não avance quando retornamos nil aqui,
		// precisamos que o Manager interprete nil como "processado com sucesso" e
		// avance o checkpoint. Mas não queremos isso — queremos REPROCESSAR.
		// Portanto lançamos um erro temporário que o Manager vai retentar:
		log.Printf("[WARN] [estudantes] academia '%s' ainda não projetada para evento %d — aguardando próximo ciclo",
			payload.CodigoAcademia, event.ID)
		return fmt.Errorf("academia '%s' ainda não disponível na projeção (evento %d) — retry automático",
			payload.CodigoAcademia, event.ID)
	}

	// FIX BUG #3: validar cursos referenciados da mesma forma.
	// Se o curso não existir ainda, não travar — usar nil temporariamente.
	// A diferença aqui é que cursos SÃO opcionais no estudante, então podemos
	// usar nil sem perda de consistência crítica (diferente da academia, que é
	// obrigatória e bloqueia tudo se não existir).
	resolvedCursoMedio, resolvedCursoSuperior := p.resolveCursoIDs(
		cursoMedioIDStr, cursoSuperiorIDStr, event.EventID,
	)

	// FIX BUG #4: o INSERT original verificava RowsAffected == 0 e retornava
	// erro. Com ON CONFLICT (id) DO UPDATE isso nunca deveria ser 0, MAS se
	// houver outra FK violation (ex: codigo_academia não existe no banco sem
	// constraint FK declarada mas com trigger), o Exec pode retornar nil e 0 rows.
	// Adicionamos log detalhado para diagnóstico futuro.
	_, err = p.client.DB().Exec(`
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, genero,
			data_nascimento,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar_fundamental, ano_escolar_medio, ano_superior, semestre_atual, curso_medio_id, curso_superior_id,
			codigo_academia, created_at, updated_at, version, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, FALSE,
			$7, $8, $9,
			$10,
			'ativo', $11, $12, $13,
			$14, $15, $16, $17, $18, $19,
			$20, $21, CURRENT_TIMESTAMP, $22, $23
		)
		ON CONFLICT (id) DO UPDATE SET
			nome                           = EXCLUDED.nome,
			codigo_estudante               = EXCLUDED.codigo_estudante,
			senha_hash                     = EXCLUDED.senha_hash,
			email                          = EXCLUDED.email,
			telefone                       = EXCLUDED.telefone,
			bilhete_identidade             = EXCLUDED.bilhete_identidade,
			bilhete_identidade_responsavel = EXCLUDED.bilhete_identidade_responsavel,
			genero                         = EXCLUDED.genero,
			data_nascimento                = EXCLUDED.data_nascimento,
			status                         = EXCLUDED.status,
			status_escolar_fundamental     = EXCLUDED.status_escolar_fundamental,
			status_escolar_medio           = EXCLUDED.status_escolar_medio,
			status_superior                = EXCLUDED.status_superior,
			ano_escolar_fundamental         = EXCLUDED.ano_escolar_fundamental,
			ano_escolar_medio              = EXCLUDED.ano_escolar_medio,
			ano_superior                   = EXCLUDED.ano_superior,
			semestre_atual                 = EXCLUDED.semestre_atual,
			curso_medio_id                 = EXCLUDED.curso_medio_id,
			curso_superior_id              = EXCLUDED.curso_superior_id,
			codigo_academia                = EXCLUDED.codigo_academia,
			created_at                     = EXCLUDED.created_at,
			updated_at                     = CURRENT_TIMESTAMP,
			version                        = EXCLUDED.version,
			last_event_id                  = EXCLUDED.last_event_id
	`,
		event.AggregateID, payload.Nome, payload.CodigoEstudante, payload.SenhaHash,
		payload.Email, payload.Telefone,
		payload.BilheteIdentidade, payload.BilheteIdentidadeResp, payload.Genero,
		payload.DataNascimento,
		payload.StatusEscolarFundamental, payload.StatusEscolarMedio, payload.StatusSuperior,
		payload.AnoEscolar, payload.AnoEscolarMedio, payload.AnoSuperior, payload.SemestreAtual,
		resolvedCursoMedio, resolvedCursoSuperior,
		payload.CodigoAcademia,
		payload.CreatedAt, event.EventVersion, event.EventID,
	)
	if err != nil {
		return fmt.Errorf("handleEstudanteCriadoComVinculo: exec error (estudante=%s academia=%s): %w",
			payload.CodigoEstudante, payload.CodigoAcademia, err)
	}

	log.Printf("[DEBUG] [estudantes] Estudante %s projetado (evento %d, academia=%s)",
		payload.CodigoEstudante, event.ID, payload.CodigoAcademia)
	return nil
}

// academiaExists verifica se a academia já foi projetada em projection_academias.
func (p *EstudanteProjection) academiaExists(codigoAcademia string) (bool, error) {
	if codigoAcademia == "" {
		// Estudante sem academia — permitir (caso raro mas válido historicamente)
		return true, nil
	}
	var exists bool
	err := p.client.DB().QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM projection_academias WHERE codigo_academia = $1
		)
	`, codigoAcademia).Scan(&exists)
	return exists, err
}

// resolveCursoIDs verifica se os cursos referenciados já existem em projection_cursos.
// Se não existirem ainda (race entre projeções), retorna nil — o vínculo de curso
// é opcional e pode ser atualizado posteriormente por DadosAcademicosAtualizados.
func (p *EstudanteProjection) resolveCursoIDs(
	cursoMedioID *string,
	cursoSuperiorID *string,
	eventID uuid.UUID,
) (*string, *string) {
	if cursoMedioID != nil {
		exists, err := p.cursoExists(*cursoMedioID)
		if err != nil || !exists {
			log.Printf("[WARN] [estudantes] evento=%s: curso_medio_id=%s não encontrado em projection_cursos — inserindo NULL",
				eventID, *cursoMedioID)
			cursoMedioID = nil
		}
	}
	if cursoSuperiorID != nil {
		exists, err := p.cursoExists(*cursoSuperiorID)
		if err != nil || !exists {
			log.Printf("[WARN] [estudantes] evento=%s: curso_superior_id=%s não encontrado em projection_cursos — inserindo NULL",
				eventID, *cursoSuperiorID)
			cursoSuperiorID = nil
		}
	}
	return cursoMedioID, cursoSuperiorID
}

func (p *EstudanteProjection) cursoExists(cursoID string) (bool, error) {
	var exists bool
	err := p.client.DB().QueryRow(`
		SELECT EXISTS (SELECT 1 FROM projection_cursos WHERE id = $1)
	`, cursoID).Scan(&exists)
	return exists, err
}

func (p *EstudanteProjection) handleStatusEscolarFundamental(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusEscolarFundamental: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_escolar_fundamental = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleStatusEscolarMedio(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusEscolarMedio: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_escolar_medio = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleStatusEscolarAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusEscolarAtualizado: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_escolar_fundamental = $1, status_escolar_medio = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleStatusSuperiorAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusSuperiorAtualizado: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_superior = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleStatusIndireto(event db.Event, coluna string, status string) error {
	if !db.SafeString(coluna) {
		return fmt.Errorf("handleStatusIndireto: coluna insegura: %q", coluna)
	}
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, coluna)
	_, err := p.client.DB().Exec(query, status, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleDadosPessoaisAtualizados(event db.Event) error {
	var payload struct {
		Nome                  *string    `json:"Nome"`
		Email                 *string    `json:"Email"`
		Telefone              *string    `json:"Telefone"`
		BilheteIdentidade     *string    `json:"BilheteIdentidade"`
		BilheteIdentidadeResp *string    `json:"BilheteIdentidadeResp"`
		DataNascimento        *time.Time `json:"DataNascimento"`
		EmailAlterado         bool       `json:"EmailAlterado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleDadosPessoaisAtualizados: parse error: %w", err)
	}

	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if payload.Nome != nil {
		setClauses = append(setClauses, fmt.Sprintf("nome = $%d", idx))
		args = append(args, *payload.Nome)
		idx++
	}
	if payload.Email != nil {
		setClauses = append(setClauses, fmt.Sprintf("email = $%d", idx))
		args = append(args, *payload.Email)
		idx++
	}
	if payload.EmailAlterado {
		setClauses = append(setClauses, "email_verificado = FALSE")
	}
	if payload.Telefone != nil {
		setClauses = append(setClauses, fmt.Sprintf("telefone = $%d", idx))
		args = append(args, *payload.Telefone)
		idx++
	}
	if payload.BilheteIdentidade != nil {
		setClauses = append(setClauses, fmt.Sprintf("bilhete_identidade = $%d", idx))
		args = append(args, *payload.BilheteIdentidade)
		idx++
	}
	if payload.BilheteIdentidadeResp != nil {
		setClauses = append(setClauses, fmt.Sprintf("bilhete_identidade_responsavel = $%d", idx))
		args = append(args, *payload.BilheteIdentidadeResp)
		idx++
	}
	if payload.DataNascimento != nil {
		setClauses = append(setClauses, fmt.Sprintf("data_nascimento = $%d", idx))
		args = append(args, *payload.DataNascimento)
		idx++
	}

	if len(setClauses) == 0 {
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	setClauses = append(setClauses,
		fmt.Sprintf("version = $%d", idx),
		fmt.Sprintf("last_event_id = $%d", idx+1),
		"updated_at = CURRENT_TIMESTAMP",
	)
	args = append(args, event.EventVersion, event.EventID, event.AggregateID)
	query := fmt.Sprintf("UPDATE projection_estudantes SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), idx+2)
	_, err := p.client.DB().Exec(query, args...)
	return err
}

func (p *EstudanteProjection) handleDadosAcademicosAtualizados(event db.Event) error {
	var payload struct {
		AnoEscolar      *string    `json:"AnoEscolar"`
		AnoEscolarMedio *string    `json:"AnoEscolarMedio"`
		AnoSuperior     *string    `json:"AnoSuperior"`
		CursoMedioID    *uuid.UUID `json:"CursoMedioID"`
		CursoSuperiorID *uuid.UUID `json:"CursoSuperiorID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleDadosAcademicosAtualizados: parse error: %w", err)
	}

	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if payload.AnoEscolar != nil {
		setClauses = append(setClauses, fmt.Sprintf("ano_escolar_fundamental = $%d", idx))
		args = append(args, *payload.AnoEscolar)
		idx++
	}
	if payload.AnoEscolarMedio != nil {
		setClauses = append(setClauses, fmt.Sprintf("ano_escolar_medio = $%d", idx))
		args = append(args, *payload.AnoEscolarMedio)
		idx++
	}
	if payload.AnoSuperior != nil {
		setClauses = append(setClauses, fmt.Sprintf("ano_superior = $%d", idx))
		args = append(args, *payload.AnoSuperior)
		idx++
	}
	if payload.CursoMedioID != nil {
		setClauses = append(setClauses, fmt.Sprintf("curso_medio_id = $%d", idx))
		args = append(args, payload.CursoMedioID.String())
		idx++
	}
	if payload.CursoSuperiorID != nil {
		setClauses = append(setClauses, fmt.Sprintf("curso_superior_id = $%d", idx))
		args = append(args, payload.CursoSuperiorID.String())
		idx++
	}

	if len(setClauses) == 0 {
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	setClauses = append(setClauses,
		fmt.Sprintf("version = $%d", idx),
		fmt.Sprintf("last_event_id = $%d", idx+1),
		"updated_at = CURRENT_TIMESTAMP",
	)
	args = append(args, event.EventVersion, event.EventID, event.AggregateID)
	query := fmt.Sprintf("UPDATE projection_estudantes SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), idx+2)
	_, err := p.client.DB().Exec(query, args...)
	return err
}

func (p *EstudanteProjection) handleEmailVerificadoEstudante(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET email_verificado = TRUE,
			version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleCursoAlterado(event db.Event) error {
	var payload struct {
		TipoEnsino string    `json:"TipoEnsino"`
		CursoID    uuid.UUID `json:"CursoID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCursoAlterado: parse error: %w", err)
	}
	var col string
	switch payload.TipoEnsino {
	case "medio":
		col = "curso_medio_id"
	case "superior":
		col = "curso_superior_id"
	default:
		return fmt.Errorf("handleCursoAlterado: TipoEnsino inválido: %q", payload.TipoEnsino)
	}
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)
	_, err := p.client.DB().Exec(query, payload.CursoID.String(), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleSenhaAlterada(event db.Event) error {
	var payload struct {
		NovaSenhaHash string `json:"NovaSenhaHash"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleSenhaAlterada: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET senha_hash = $1, version = $2,
			updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovaSenhaHash, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleAvaliacaoFinalAnoAcademico(event db.Event) error {
	payload, err := parseAvaliacaoFinalPayloadEstudante(event.Payload)
	if err != nil {
		return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: parse error: %w", err)
	}
	tipoEnsino := strings.TrimSpace(strings.ToLower(payload.TipoEnsino))
	if tipoEnsino == "" {
		switch event.EventType {
		case "AvaliacaoFinalEscolar":
			inferredTipoEnsino, err := p.inferTipoEnsinoEscolar(event.AggregateID)
			if err != nil {
				return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: falha ao inferir TipoEnsino escolar: %w", err)
			}
			tipoEnsino = inferredTipoEnsino
		case "AvaliacaoFinalSuperior":
			tipoEnsino = "superior"
		}
	}

	if !payload.Aprovado {
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	if payload.ProximoAnoAcademico == nil {
		var statusCol string
		switch tipoEnsino {
		case "fundamental":
			statusCol = "status_escolar_fundamental"
		case "medio":
			statusCol = "status_escolar_medio"
		case "superior":
			statusCol = "status_superior"
		default:
			return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: TipoEnsino inválido: %q", payload.TipoEnsino)
		}
		query := fmt.Sprintf(`
			UPDATE projection_estudantes
			SET %s = 'finalizado', version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, statusCol)
		_, err := p.client.DB().Exec(query, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	var col string
	switch tipoEnsino {
	case "fundamental":
		col = "ano_escolar_fundamental"
	case "medio":
		col = "ano_escolar_medio"
	case "superior":
		col = "ano_superior"
	default:
		return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: TipoEnsino inválido: %q", payload.TipoEnsino)
	}
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)
	_, err = p.client.DB().Exec(query, payload.ProximoAnoAcademico, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

type avaliacaoFinalPayloadEstudante struct {
	TipoEnsino          string
	ProximoAnoAcademico *string
	Aprovado            bool
}

func parseAvaliacaoFinalPayloadEstudante(raw json.RawMessage) (avaliacaoFinalPayloadEstudante, error) {
	var snake struct {
		TipoEnsino          string  `json:"tipo_ensino"`
		ProximoAnoAcademico *string `json:"proximo_ano_academico"`
		Aprovado            bool    `json:"aprovado"`
	}
	if err := json.Unmarshal(raw, &snake); err != nil {
		return avaliacaoFinalPayloadEstudante{}, err
	}

	result := avaliacaoFinalPayloadEstudante{
		TipoEnsino:          snake.TipoEnsino,
		ProximoAnoAcademico: snake.ProximoAnoAcademico,
		Aprovado:            snake.Aprovado,
	}

	if result.TipoEnsino == "" && result.ProximoAnoAcademico == nil {
		var legacy struct {
			TipoEnsino          string  `json:"TipoEnsino"`
			ProximoAnoAcademico *string `json:"ProximoAnoAcademico"`
			Aprovado            bool    `json:"Aprovado"`
		}
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return avaliacaoFinalPayloadEstudante{}, err
		}
		if legacy.TipoEnsino != "" || legacy.ProximoAnoAcademico != nil {
			result.TipoEnsino = legacy.TipoEnsino
			result.ProximoAnoAcademico = legacy.ProximoAnoAcademico
		}
	}

	return result, nil
}

func (p *EstudanteProjection) inferTipoEnsinoEscolar(estudanteID uuid.UUID) (string, error) {
	var anoEscolarMedio, cursoMedioID sql.NullString
	if err := p.client.DB().QueryRow(`
		SELECT ano_escolar_medio, curso_medio_id
		FROM projection_estudantes
		WHERE id = $1
	`, estudanteID).Scan(&anoEscolarMedio, &cursoMedioID); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("estudante %s não encontrado na projeção", estudanteID)
		}
		return "", err
	}

	if (anoEscolarMedio.Valid && strings.TrimSpace(anoEscolarMedio.String) != "") ||
		(cursoMedioID.Valid && strings.TrimSpace(cursoMedioID.String) != "") {
		return "medio", nil
	}

	return "fundamental", nil
}

func (p *EstudanteProjection) handleVersionOnly(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// DTO de leitura
// ============================================================================

type EstudanteDTO struct {
	ID                       uuid.UUID `json:"id"`
	Nome                     string    `json:"nome"`
	CodigoEstudante          string    `json:"codigo_estudante"`
	Email                    *string   `json:"email,omitempty"`
	Telefone                 *string   `json:"telefone,omitempty"`
	EmailVerificado          bool      `json:"email_verificado"`
	BilheteIdentidade        *string   `json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp    *string   `json:"bilhete_identidade_responsavel,omitempty"`
	Genero                   string    `json:"genero"`
	DataNascimento           time.Time `json:"data_nascimento"`
	CodigoAcademia           *string   `json:"codigo_academia,omitempty"`
	Status                   string    `json:"status"`
	StatusEscolarFundamental string    `json:"status_escolar_fundamental"`
	StatusEscolarMedio       string    `json:"status_escolar_medio"`
	StatusSuperior           string    `json:"status_superior"`
	AnoEscolar               *string   `json:"ano_escolar_fundamental,omitempty"`
	AnoEscolarMedio          *string   `json:"ano_escolar_medio,omitempty"`
	AnoSuperior              *string   `json:"ano_superior,omitempty"`
	SemestreAtual            *int      `json:"semestre_atual,omitempty"`
	CursoMedioID             *string   `json:"curso_medio_id,omitempty"`
	CursoSuperiorID          *string   `json:"curso_superior_id,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
	Version                  int       `json:"version"`
}

const estudanteCols = `
	id, nome, codigo_estudante, email, telefone, email_verificado,
	bilhete_identidade, bilhete_identidade_responsavel, genero,
	data_nascimento,
	codigo_academia, status, status_escolar_fundamental, status_escolar_medio, status_superior,
	ano_escolar_fundamental, ano_escolar_medio, ano_superior, semestre_atual, curso_medio_id, curso_superior_id,
	created_at, updated_at, version
`

func scanEstudante(row *sql.Row) (*EstudanteDTO, error) {
	var e EstudanteDTO
	err := row.Scan(
		&e.ID, &e.Nome, &e.CodigoEstudante, &e.Email, &e.Telefone, &e.EmailVerificado,
		&e.BilheteIdentidade, &e.BilheteIdentidadeResp, &e.Genero,
		&e.DataNascimento,
		&e.CodigoAcademia, &e.Status, &e.StatusEscolarFundamental, &e.StatusEscolarMedio, &e.StatusSuperior,
		&e.AnoEscolar, &e.AnoEscolarMedio, &e.AnoSuperior, &e.SemestreAtual, &e.CursoMedioID, &e.CursoSuperiorID,
		&e.CreatedAt, &e.UpdatedAt, &e.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func scanEstudanteRows(rows *sql.Rows) ([]EstudanteDTO, error) {
	var estudantes []EstudanteDTO
	for rows.Next() {
		var e EstudanteDTO
		if err := rows.Scan(
			&e.ID, &e.Nome, &e.CodigoEstudante, &e.Email, &e.Telefone, &e.EmailVerificado,
			&e.BilheteIdentidade, &e.BilheteIdentidadeResp, &e.Genero,
			&e.DataNascimento,
			&e.CodigoAcademia, &e.Status, &e.StatusEscolarFundamental, &e.StatusEscolarMedio, &e.StatusSuperior,
			&e.AnoEscolar, &e.AnoEscolarMedio, &e.AnoSuperior, &e.SemestreAtual, &e.CursoMedioID, &e.CursoSuperiorID,
			&e.CreatedAt, &e.UpdatedAt, &e.Version,
		); err != nil {
			return nil, err
		}
		estudantes = append(estudantes, e)
	}
	return estudantes, rows.Err()
}

func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	return scanEstudante(p.client.DB().QueryRow(
		`SELECT `+estudanteCols+` FROM projection_estudantes WHERE id = $1`, id,
	))
}

func (p *EstudanteProjection) GetByCodigo(codigo string) (*EstudanteDTO, error) {
	return scanEstudante(p.client.DB().QueryRow(
		`SELECT `+estudanteCols+` FROM projection_estudantes WHERE codigo_estudante = $1`, codigo,
	))
}

func (p *EstudanteProjection) GetByEmail(email string) (*EstudanteDTO, error) {
	return scanEstudante(p.client.DB().QueryRow(
		`SELECT `+estudanteCols+` FROM projection_estudantes WHERE email = $1`, email,
	))
}

func (p *EstudanteProjection) GetAll() ([]EstudanteDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT ` + estudanteCols + ` FROM projection_estudantes ORDER BY nome ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEstudanteRows(rows)
}

func (p *EstudanteProjection) GetByAcademia(codigoAcademia string) ([]EstudanteDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+estudanteCols+` FROM projection_estudantes WHERE codigo_academia = $1 ORDER BY nome ASC`,
		codigoAcademia,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEstudanteRows(rows)
}

// ============================================================================
// Auth DTO
// ============================================================================

type EstudanteAuthDTO struct {
	ID              uuid.UUID `json:"-"`
	Nome            string    `json:"-"`
	Codigo          string    `json:"-"`
	Status          string    `json:"-"`
	Hash            string    `json:"-"`
	Email           *string   `json:"-"`
	EmailVerificado bool      `json:"-"`
}

func (p *EstudanteProjection) GetAuthByCodigo(codigo string) (*EstudanteAuthDTO, error) {
	var e EstudanteAuthDTO
	err := p.client.DB().QueryRow(
		`SELECT id, nome, codigo_estudante, status, senha_hash
		 FROM projection_estudantes WHERE codigo_estudante = $1`,
		codigo,
	).Scan(&e.ID, &e.Nome, &e.Codigo, &e.Status, &e.Hash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (p *EstudanteProjection) GetAuthByID(id uuid.UUID) (*EstudanteAuthDTO, error) {
	var e EstudanteAuthDTO
	err := p.client.DB().QueryRow(
		`SELECT id, nome, codigo_estudante, status, senha_hash
		 FROM projection_estudantes WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.Nome, &e.Codigo, &e.Status, &e.Hash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (p *EstudanteProjection) GetAuthByIdentificador(identificador string) (*EstudanteAuthDTO, error) {
	var e EstudanteAuthDTO
	var emailNull sql.NullString
	err := p.client.DB().QueryRow(`
		SELECT id, nome, codigo_estudante, status, senha_hash,
		       email, COALESCE(email_verificado, FALSE)
		FROM projection_estudantes
		WHERE codigo_estudante = $1
		   OR email            = $1
		LIMIT 1
	`, identificador).Scan(&e.ID, &e.Nome, &e.Codigo, &e.Status, &e.Hash,
		&emailNull, &e.EmailVerificado)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if emailNull.Valid {
		e.Email = &emailNull.String
	}
	return &e, nil
}

func (p *EstudanteProjection) GetByBilheteIdentidadePrincipal(bilhete string) (*EstudanteDTO, error) {
	return scanEstudante(p.client.DB().QueryRow(
		`SELECT `+estudanteCols+` FROM projection_estudantes
		 WHERE lower(btrim(bilhete_identidade)) = lower(btrim($1))
		 LIMIT 1`,
		bilhete,
	))
}

func (p *EstudanteProjection) GetByBilheteIdentidadePrincipalExcludingID(bilhete string, estudanteID uuid.UUID) (*EstudanteDTO, error) {
	return scanEstudante(p.client.DB().QueryRow(
		`SELECT `+estudanteCols+` FROM projection_estudantes
		 WHERE lower(btrim(bilhete_identidade)) = lower(btrim($1))
		   AND id <> $2
		 LIMIT 1`,
		bilhete,
		estudanteID,
	))
}

func (p *EstudanteProjection) CountByCurso(cursoID uuid.UUID) (int, error) {
	var count int
	err := p.client.DB().QueryRow(
		`SELECT COUNT(*) FROM projection_estudantes
		 WHERE (curso_medio_id = $1 OR curso_superior_id = $1) AND status = 'ativo'`,
		cursoID.String(),
	).Scan(&count)
	return count, err
}
