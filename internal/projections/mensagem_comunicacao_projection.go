package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

type MensagemComunicacaoProjection struct{ client *db.Client }

func NewMensagemComunicacaoProjection(c *db.Client) *MensagemComunicacaoProjection {
	return &MensagemComunicacaoProjection{c}
}
func (p *MensagemComunicacaoProjection) Name() string { return "mensagens_comunicacao" }
func (p *MensagemComunicacaoProjection) GetLastProcessedEventID() (int64, error) {
	var v int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
func (p *MensagemComunicacaoProjection) UpdateCheckpoint(id int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT(projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return err
}
func (p *MensagemComunicacaoProjection) Handle(e db.Event) error {
	if e.AggregateType != "MensagemComunicacao" {
		return nil
	}
	switch e.EventType {
	case "MensagemComunicacaoRegistrada":
		return p.registrada(e)
	}
	return nil
}
func (p *MensagemComunicacaoProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE projection_mensagens_comunicacao CASCADE`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='MensagemComunicacao' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e db.Event
		var prev sql.NullString
		if err = rows.Scan(&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.EventVersion, &e.Payload, &e.Metadata, &e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &prev); err != nil {
			return err
		}
		if prev.Valid {
			e.PreviousHash = &prev.String
		}
		if err = p.Handle(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

type MensagemComunicacaoDTO struct {
	ID                 uuid.UUID                             `json:"id"`
	Destinatario       string                                `json:"destinatario"`
	Conteudo           string                                `json:"conteudo"`
	ProvedorTentado1   string                                `json:"provedor_tentado_1"`
	ProvedorTentado2   string                                `json:"provedor_tentado_2,omitempty"`
	ProvedorUtilizado  string                                `json:"provedor_utilizado,omitempty"`
	Status             string                                `json:"status"`
	MensagemExternaID  string                                `json:"mensagem_externa_id,omitempty"`
	DetalhesTentativas []aggregatesTentativaEnvioComunicacao `json:"detalhes_tentativas"`
	EnviadoPor         uuid.UUID                             `json:"enviado_por"`
	EnviadoPorTipo     string                                `json:"enviado_por_tipo"`
	CodigoAcademia     string                                `json:"codigo_academia,omitempty"`
	CreatedAt          time.Time                             `json:"created_at"`
}

// aggregatesTentativaEnvioComunicacao espelha
// aggregates.TentativaEnvioComunicacao (mesmos nomes/tags de JSON) — este
// pacote (projections) não importa internal/domain/aggregates para não
// criar uma dependência circular com internal/handlers, então o payload é
// decodificado para este tipo espelhado local em vez do tipo do agregado.
type aggregatesTentativaEnvioComunicacao struct {
	Provedor          string `json:"provedor"`
	Sucesso           bool   `json:"sucesso"`
	MensagemExternaID string `json:"mensagem_externa_id,omitempty"`
	ErroCodigo        string `json:"erro_codigo,omitempty"`
	ErroMensagem      string `json:"erro_mensagem,omitempty"`
}

func (p *MensagemComunicacaoProjection) registrada(e db.Event) error {
	var x struct {
		Destinatario       string
		Conteudo           string
		ProvedorTentado1   string
		ProvedorTentado2   string
		ProvedorUtilizado  string
		Status             string
		MensagemExternaID  string
		DetalhesTentativas []aggregatesTentativaEnvioComunicacao
		EnviadoPor         uuid.UUID
		EnviadoPorTipo     string
		CodigoAcademia     string
		CreatedAt          time.Time
	}
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	detalhesJSON, err := json.Marshal(x.DetalhesTentativas)
	if err != nil {
		return err
	}
	var provedorTentado2, provedorUtilizado, mensagemExternaID, codigoAcademia interface{}
	if x.ProvedorTentado2 != "" {
		provedorTentado2 = x.ProvedorTentado2
	}
	if x.ProvedorUtilizado != "" {
		provedorUtilizado = x.ProvedorUtilizado
	}
	if x.MensagemExternaID != "" {
		mensagemExternaID = x.MensagemExternaID
	}
	if x.CodigoAcademia != "" {
		codigoAcademia = x.CodigoAcademia
	}
	_, err = p.client.DB().Exec(`
		INSERT INTO projection_mensagens_comunicacao (
			id, destinatario, conteudo, provedor_tentado_1, provedor_tentado_2, provedor_utilizado,
			status, mensagem_externa_id, detalhes_tentativas, enviado_por, enviado_por_tipo, codigo_academia,
			created_at, updated_at, version, last_event_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13,$14,$15)
		ON CONFLICT (id) DO NOTHING
	`, e.AggregateID, x.Destinatario, x.Conteudo, x.ProvedorTentado1, provedorTentado2, provedorUtilizado,
		x.Status, mensagemExternaID, detalhesJSON, x.EnviadoPor, x.EnviadoPorTipo, codigoAcademia,
		x.CreatedAt, e.EventVersion, e.EventID)
	return err
}

const mensagemComunicacaoCols = `id,destinatario,conteudo,provedor_tentado_1,coalesce(provedor_tentado_2,''),coalesce(provedor_utilizado,''),status,coalesce(mensagem_externa_id,''),detalhes_tentativas,enviado_por,enviado_por_tipo,coalesce(codigo_academia,''),created_at`

func (p *MensagemComunicacaoProjection) scan(row interface{ Scan(...interface{}) error }) (*MensagemComunicacaoDTO, error) {
	var d MensagemComunicacaoDTO
	var detalhesRaw []byte
	if err := row.Scan(&d.ID, &d.Destinatario, &d.Conteudo, &d.ProvedorTentado1, &d.ProvedorTentado2, &d.ProvedorUtilizado,
		&d.Status, &d.MensagemExternaID, &detalhesRaw, &d.EnviadoPor, &d.EnviadoPorTipo, &d.CodigoAcademia, &d.CreatedAt); err != nil {
		return nil, err
	}
	if len(detalhesRaw) > 0 {
		if err := json.Unmarshal(detalhesRaw, &d.DetalhesTentativas); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

// ListFiltro controla a listagem: CodigoAcademia (quando preenchido)
// restringe às mensagens enviadas por essa academia; quando vazio E
// SomenteAcademia for false, lista de todas as origens (uso exclusivo de
// administradores).
type MensagemComunicacaoListFiltro struct {
	CodigoAcademia string
	Limit          int
	Offset         int
}

// List devolve as mensagens mais recentes primeiro. Quando filtro.CodigoAcademia
// estiver preenchido, restringe às mensagens enviadas por essa academia —
// usado tanto para "uma academia só vê as suas" quanto para o filtro
// opcional de um admin.
func (p *MensagemComunicacaoProjection) List(filtro MensagemComunicacaoListFiltro) ([]MensagemComunicacaoDTO, int, error) {
	where := ""
	args := []interface{}{}
	if filtro.CodigoAcademia != "" {
		where = "WHERE codigo_academia = $1"
		args = append(args, filtro.CodigoAcademia)
	}
	var total int
	countQuery := "SELECT COUNT(*) FROM projection_mensagens_comunicacao " + where
	if err := p.client.DB().QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("contar mensagens de comunicação: %w", err)
	}
	args = append(args, filtro.Limit, filtro.Offset)
	limitPos := len(args) - 1
	offsetPos := len(args)
	query := fmt.Sprintf(`SELECT %s FROM projection_mensagens_comunicacao %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		mensagemComunicacaoCols, where, limitPos, offsetPos)
	rows, err := p.client.DB().Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listar mensagens de comunicação: %w", err)
	}
	defer rows.Close()
	out := []MensagemComunicacaoDTO{}
	for rows.Next() {
		d, err := p.scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("listar mensagens de comunicação: %w", err)
	}
	return out, total, nil
}
