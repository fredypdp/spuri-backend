package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type SolicitacaoMatriculaProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewSolicitacaoMatriculaProjection(client *db.Client) *SolicitacaoMatriculaProjection {
	return &SolicitacaoMatriculaProjection{client: client, ctx: context.Background()}
}

func (p *SolicitacaoMatriculaProjection) Name() string { return "solicitacoes_matricula" }

func (p *SolicitacaoMatriculaProjection) Handle(event db.Event) error {
	if event.AggregateType != "SolicitacaoMatricula" {
		return nil
	}
	switch event.EventType {
	case "SolicitacaoMatriculaCriada":
		return p.handleCriada(event)
	case "SolicitacaoMatriculaAprovada":
		return p.handleAprovada(event)
	case "SolicitacaoMatriculaReprovada":
		return p.handleReprovada(event)
	default:
		return nil
	}
}
func (p *SolicitacaoMatriculaProjection) GetLastProcessedEventID() (int64, error) {
	return NewBaseProjection(p.client).GetLastProcessedEventIDByName(p.Name())
}
func (p *SolicitacaoMatriculaProjection) UpdateCheckpoint(eventID int64) error {
	return NewBaseProjection(p.client).UpdateCheckpointByName(p.Name(), eventID)
}
func (p *SolicitacaoMatriculaProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id, event_id, aggregate_id, aggregate_type, event_type, event_version, payload, metadata, occurred_at, recorded_at FROM spuri_ledger WHERE aggregate_type = 'SolicitacaoMatricula' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var ev db.Event
		if err := rows.Scan(&ev.ID, &ev.EventID, &ev.AggregateID, &ev.AggregateType, &ev.EventType, &ev.EventVersion, &ev.Payload, &ev.Metadata, &ev.OccurredAt, &ev.RecordedAt); err != nil {
			return err
		}
		if err := p.Handle(ev); err != nil {
			return err
		}
		last = ev.ID
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return p.UpdateCheckpoint(last)
}
func (p *SolicitacaoMatriculaProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_solicitacoes_matricula`)
	return err
}

func (p *SolicitacaoMatriculaProjection) handleCriada(event db.Event) error {
	var payload struct {
		CodigoSolicitacao            string            `json:"CodigoSolicitacao"`
		CodigoAcademia               string            `json:"CodigoAcademia"`
		Nome                         string            `json:"Nome"`
		Genero                       string            `json:"Genero"`
		DataNascimento               time.Time         `json:"DataNascimento"`
		Email                        *string           `json:"Email"`
		Telefone                     *string           `json:"Telefone"`
		BilheteIdentidade            *string           `json:"BilheteIdentidade"`
		BilheteIdentidadeResponsavel *string           `json:"BilheteIdentidadeResponsavel"`
		AnoEscolarFundamental        *string           `json:"AnoEscolarFundamental"`
		AnoEscolarMedio              *string           `json:"AnoEscolarMedio"`
		CursoMedioID                 *uuid.UUID        `json:"CursoMedioID"`
		AnoSuperior                  *string           `json:"AnoSuperior"`
		CursoSuperiorID              *uuid.UUID        `json:"CursoSuperiorID"`
		Status                       string            `json:"Status"`
		Documentos                   map[string]string `json:"Documentos"`
		CreatedAt                    time.Time         `json:"CreatedAt"`
		UpdatedAt                    time.Time         `json:"UpdatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	docs, _ := json.Marshal(payload.Documentos)
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_solicitacoes_matricula (
			id, codigo_solicitacao, codigo_academia, nome, genero, data_nascimento,
			email, telefone, bilhete_identidade, bilhete_identidade_responsavel,
			ano_escolar_fundamental, ano_escolar_medio, curso_medio_id, ano_superior, curso_superior_id,
			status, documentos, created_at, updated_at, version, last_event_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		ON CONFLICT (id) DO UPDATE SET
			codigo_solicitacao=EXCLUDED.codigo_solicitacao, codigo_academia=EXCLUDED.codigo_academia,
			nome=EXCLUDED.nome, genero=EXCLUDED.genero, data_nascimento=EXCLUDED.data_nascimento,
			email=EXCLUDED.email, telefone=EXCLUDED.telefone, bilhete_identidade=EXCLUDED.bilhete_identidade,
			bilhete_identidade_responsavel=EXCLUDED.bilhete_identidade_responsavel,
			ano_escolar_fundamental=EXCLUDED.ano_escolar_fundamental, ano_escolar_medio=EXCLUDED.ano_escolar_medio,
			curso_medio_id=EXCLUDED.curso_medio_id, ano_superior=EXCLUDED.ano_superior, curso_superior_id=EXCLUDED.curso_superior_id,
			status=EXCLUDED.status, documentos=EXCLUDED.documentos, updated_at=EXCLUDED.updated_at,
			version=EXCLUDED.version, last_event_id=EXCLUDED.last_event_id
	`, event.AggregateID, payload.CodigoSolicitacao, payload.CodigoAcademia, payload.Nome, payload.Genero, payload.DataNascimento,
		payload.Email, payload.Telefone, payload.BilheteIdentidade, payload.BilheteIdentidadeResponsavel,
		payload.AnoEscolarFundamental, payload.AnoEscolarMedio, nullOrUUID(payload.CursoMedioID), payload.AnoSuperior, nullOrUUID(payload.CursoSuperiorID),
		payload.Status, docs, payload.CreatedAt, payload.UpdatedAt, event.EventVersion, event.EventID)
	return err
}
func (p *SolicitacaoMatriculaProjection) handleAprovada(event db.Event) error {
	var payload struct {
		CodigoEstudanteGerado string    `json:"CodigoEstudanteGerado"`
		AprovadaPor           uuid.UUID `json:"AprovadaPor"`
		ApprovedAt            time.Time `json:"ApprovedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_matricula SET status='aprovada', codigo_estudante_gerado=$1, aprovada_por=$2, updated_at=$3, version=$4, last_event_id=$5 WHERE id=$6`, payload.CodigoEstudanteGerado, payload.AprovadaPor, payload.ApprovedAt, event.EventVersion, event.EventID, event.AggregateID)
	return err
}
func (p *SolicitacaoMatriculaProjection) handleReprovada(event db.Event) error {
	var payload struct {
		MotivoReprovacao string    `json:"MotivoReprovacao"`
		ReprovadaPor     uuid.UUID `json:"ReprovadaPor"`
		RejectedAt       time.Time `json:"RejectedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_matricula SET status='reprovada', motivo_reprovacao=$1, reprovada_por=$2, updated_at=$3, version=$4, last_event_id=$5 WHERE id=$6`, payload.MotivoReprovacao, payload.ReprovadaPor, payload.RejectedAt, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

type SolicitacaoMatriculaDTO struct {
	ID                           uuid.UUID         `json:"id"`
	CodigoSolicitacao            string            `json:"codigo_solicitacao"`
	CodigoAcademia               string            `json:"codigo_academia"`
	Nome                         string            `json:"nome"`
	Genero                       string            `json:"genero"`
	DataNascimento               time.Time         `json:"data_nascimento"`
	Email                        *string           `json:"email,omitempty"`
	Telefone                     *string           `json:"telefone,omitempty"`
	BilheteIdentidade            *string           `json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResponsavel *string           `json:"bilhete_identidade_responsavel,omitempty"`
	AnoEscolarFundamental        *string           `json:"ano_escolar_fundamental,omitempty"`
	AnoEscolarMedio              *string           `json:"ano_escolar_medio,omitempty"`
	CursoMedioID                 *uuid.UUID        `json:"curso_medio_id,omitempty"`
	AnoSuperior                  *string           `json:"ano_superior,omitempty"`
	CursoSuperiorID              *uuid.UUID        `json:"curso_superior_id,omitempty"`
	Status                       string            `json:"status"`
	MotivoReprovacao             *string           `json:"motivo_reprovacao,omitempty"`
	Documentos                   map[string]string `json:"documentos"`
	CodigoEstudanteGerado        *string           `json:"codigo_estudante_gerado,omitempty"`
	AprovadaPor                  *uuid.UUID        `json:"aprovada_por,omitempty"`
	ReprovadaPor                 *uuid.UUID        `json:"reprovada_por,omitempty"`
	CreatedAt                    time.Time         `json:"created_at"`
	UpdatedAt                    time.Time         `json:"updated_at"`
	Version                      int               `json:"version"`
}

type SolicitacaoListResult struct {
	Solicitacoes []SolicitacaoMatriculaDTO
	Total        int
}

func (p *SolicitacaoMatriculaProjection) GetByCodigo(codigo string) (*SolicitacaoMatriculaDTO, error) {
	rows, err := p.query(`WHERE codigo_solicitacao = $1`, codigo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		dto, err := scanSolicitacao(rows)
		return dto, err
	}
	return nil, rows.Err()
}
func (p *SolicitacaoMatriculaProjection) List(status []string, codigosAcademia []string, limit, offset int) (*SolicitacaoListResult, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	i := 1
	if len(status) > 0 {
		where += fmt.Sprintf(" AND status = ANY($%d)", i)
		args = append(args, pq.Array(status))
		i++
	}
	if len(codigosAcademia) > 0 {
		where += fmt.Sprintf(" AND codigo_academia = ANY($%d)", i)
		args = append(args, pq.Array(codigosAcademia))
		i++
	}
	var total int
	if err := p.client.DB().QueryRow("SELECT COUNT(*) FROM projection_solicitacoes_matricula "+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`SELECT id, codigo_solicitacao, codigo_academia, nome, genero, data_nascimento, email, telefone, bilhete_identidade, bilhete_identidade_responsavel, ano_escolar_fundamental, ano_escolar_medio, curso_medio_id, ano_superior, curso_superior_id, status, motivo_reprovacao, documentos, codigo_estudante_gerado, aprovada_por, reprovada_por, created_at, updated_at, version FROM projection_solicitacoes_matricula %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, where, i, i+1)
	args = append(args, limit, offset)
	rows, err := p.client.DB().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SolicitacaoMatriculaDTO{}
	for rows.Next() {
		dto, err := scanSolicitacao(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *dto)
	}
	return &SolicitacaoListResult{Solicitacoes: out, Total: total}, rows.Err()
}
func (p *SolicitacaoMatriculaProjection) query(where string, args ...interface{}) (*sql.Rows, error) {
	return p.client.DB().Query(`SELECT id, codigo_solicitacao, codigo_academia, nome, genero, data_nascimento, email, telefone, bilhete_identidade, bilhete_identidade_responsavel, ano_escolar_fundamental, ano_escolar_medio, curso_medio_id, ano_superior, curso_superior_id, status, motivo_reprovacao, documentos, codigo_estudante_gerado, aprovada_por, reprovada_por, created_at, updated_at, version FROM projection_solicitacoes_matricula `+where, args...)
}

func scanSolicitacao(row interface{ Scan(...interface{}) error }) (*SolicitacaoMatriculaDTO, error) {
	var dto SolicitacaoMatriculaDTO
	var docs []byte
	var email, telefone, bi, biResp, anoFund, anoMedio, anoSup, motivo, codEst sql.NullString
	var cursoMedio, cursoSuperior, aprovadaPor, reprovadaPor uuid.NullUUID
	err := row.Scan(&dto.ID, &dto.CodigoSolicitacao, &dto.CodigoAcademia, &dto.Nome, &dto.Genero, &dto.DataNascimento, &email, &telefone, &bi, &biResp, &anoFund, &anoMedio, &cursoMedio, &anoSup, &cursoSuperior, &dto.Status, &motivo, &docs, &codEst, &aprovadaPor, &reprovadaPor, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
	if err != nil {
		return nil, err
	}
	if email.Valid {
		dto.Email = &email.String
	}
	if telefone.Valid {
		dto.Telefone = &telefone.String
	}
	if bi.Valid {
		dto.BilheteIdentidade = &bi.String
	}
	if biResp.Valid {
		dto.BilheteIdentidadeResponsavel = &biResp.String
	}
	if anoFund.Valid {
		dto.AnoEscolarFundamental = &anoFund.String
	}
	if anoMedio.Valid {
		dto.AnoEscolarMedio = &anoMedio.String
	}
	if anoSup.Valid {
		dto.AnoSuperior = &anoSup.String
	}
	if motivo.Valid {
		dto.MotivoReprovacao = &motivo.String
	}
	if codEst.Valid {
		dto.CodigoEstudanteGerado = &codEst.String
	}
	if cursoMedio.Valid {
		dto.CursoMedioID = &cursoMedio.UUID
	}
	if cursoSuperior.Valid {
		dto.CursoSuperiorID = &cursoSuperior.UUID
	}
	if aprovadaPor.Valid {
		dto.AprovadaPor = &aprovadaPor.UUID
	}
	if reprovadaPor.Valid {
		dto.ReprovadaPor = &reprovadaPor.UUID
	}
	dto.Documentos = map[string]string{}
	_ = json.Unmarshal(docs, &dto.Documentos)
	return &dto, nil
}
