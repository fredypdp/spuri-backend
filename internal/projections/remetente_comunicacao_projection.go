package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

type RemetenteComunicacaoProjection struct{ client *db.Client }

func NewRemetenteComunicacaoProjection(c *db.Client) *RemetenteComunicacaoProjection {
	return &RemetenteComunicacaoProjection{c}
}
func (p *RemetenteComunicacaoProjection) Name() string { return "remetentes_comunicacao" }
func (p *RemetenteComunicacaoProjection) GetLastProcessedEventID() (int64, error) {
	var v int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
func (p *RemetenteComunicacaoProjection) UpdateCheckpoint(id int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT(projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return err
}
func (p *RemetenteComunicacaoProjection) Handle(e db.Event) error {
	if e.AggregateType != "RemetenteComunicacao" {
		return nil
	}
	switch e.EventType {
	case "RemetenteComunicacaoConfigurado":
		return p.configurado(e)
	}
	return nil
}
func (p *RemetenteComunicacaoProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE projection_remetentes_comunicacao CASCADE`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='RemetenteComunicacao' ORDER BY id`)
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

// RemetenteComunicacaoDTO é a representação PÚBLICA (usada nas respostas
// HTTP de "listar remetentes"). Nunca inclui o token de API — apenas o
// indicador booleano TokenConfigurado.
type RemetenteComunicacaoDTO struct {
	ID                 uuid.UUID `json:"id"`
	Provedor           string    `json:"provedor"`
	Identificador      string    `json:"identificador"`
	TokenConfigurado   bool      `json:"token_configurado"`
	ConfiguradoPor     uuid.UUID `json:"configurado_por"`
	ConfiguradoPorTipo string    `json:"configurado_por_tipo"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// RemetenteComunicacaoCredenciais é a representação INTERNA usada apenas
// pelo fluxo de envio de mensagem (comunicacao_handlers.go), para decifrar
// o token e chamar o provedor. Nunca é serializada numa resposta HTTP.
type RemetenteComunicacaoCredenciais struct {
	Identificador   string
	TokenAPICifrado string
}

func (p *RemetenteComunicacaoProjection) configurado(e db.Event) error {
	var x struct {
		Provedor           string
		Identificador      string
		TokenAPICifrado    string
		ConfiguradoPor     uuid.UUID
		ConfiguradoPorTipo string
		ConfiguradoEm      time.Time
	}
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_remetentes_comunicacao (id, provedor, identificador, token_api_cifrado, configurado_por, configurado_por_tipo, created_at, updated_at, version, last_event_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET
			identificador = EXCLUDED.identificador,
			token_api_cifrado = EXCLUDED.token_api_cifrado,
			configurado_por = EXCLUDED.configurado_por,
			configurado_por_tipo = EXCLUDED.configurado_por_tipo,
			updated_at = EXCLUDED.updated_at,
			version = EXCLUDED.version,
			last_event_id = EXCLUDED.last_event_id
	`, e.AggregateID, x.Provedor, x.Identificador, x.TokenAPICifrado, x.ConfiguradoPor, x.ConfiguradoPorTipo, x.ConfiguradoEm, e.EventVersion, e.EventID)
	return err
}

const remetenteComunicacaoCols = `id,provedor,identificador,configurado_por,configurado_por_tipo,created_at,updated_at`

func (p *RemetenteComunicacaoProjection) scan(row interface{ Scan(...interface{}) error }) (*RemetenteComunicacaoDTO, error) {
	var d RemetenteComunicacaoDTO
	if err := row.Scan(&d.ID, &d.Provedor, &d.Identificador, &d.ConfiguradoPor, &d.ConfiguradoPorTipo, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}
	d.TokenConfigurado = true
	return &d, nil
}

// GetByProvedor devolve a representação pública (sem token) do remetente
// configurado para o provedor, ou nil se ainda não houver nenhum.
func (p *RemetenteComunicacaoProjection) GetByProvedor(provedor string) (*RemetenteComunicacaoDTO, error) {
	d, err := p.scan(p.client.DB().QueryRow(`SELECT `+remetenteComunicacaoCols+` FROM projection_remetentes_comunicacao WHERE provedor=$1`, provedor))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// GetCredenciaisByProvedor devolve o identificador e o token AINDA
// CIFRADO do remetente configurado para o provedor, ou nil se ainda não
// houver nenhum. Uso exclusivo do fluxo de envio — nunca expor em resposta
// HTTP.
func (p *RemetenteComunicacaoProjection) GetCredenciaisByProvedor(provedor string) (*RemetenteComunicacaoCredenciais, error) {
	var c RemetenteComunicacaoCredenciais
	err := p.client.DB().QueryRow(`SELECT identificador, token_api_cifrado FROM projection_remetentes_comunicacao WHERE provedor=$1`, provedor).Scan(&c.Identificador, &c.TokenAPICifrado)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List devolve todos os remetentes configurados (no máximo dois: um por
// provedor), ordenados por provedor.
func (p *RemetenteComunicacaoProjection) List() ([]RemetenteComunicacaoDTO, error) {
	rows, err := p.client.DB().Query(`SELECT ` + remetenteComunicacaoCols + ` FROM projection_remetentes_comunicacao ORDER BY provedor`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RemetenteComunicacaoDTO{}
	for rows.Next() {
		d, err := p.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar remetentes de comunicação: %w", err)
	}
	return out, nil
}
