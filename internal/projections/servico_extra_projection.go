package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/db"
	"time"
)

type ServicoExtraProjection struct{ client *db.Client }

func NewServicoExtraProjection(c *db.Client) *ServicoExtraProjection {
	return &ServicoExtraProjection{c}
}
func (p *ServicoExtraProjection) Name() string { return "servicos_extras" }
func (p *ServicoExtraProjection) GetLastProcessedEventID() (int64, error) {
	var v int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
func (p *ServicoExtraProjection) UpdateCheckpoint(id int64) error {
	_, e := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT(projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return e
}
func (p *ServicoExtraProjection) Handle(e db.Event) error {
	if e.AggregateType != "ServicoExtra" {
		return nil
	}
	switch e.EventType {
	case "ServicoExtraCriado":
		return p.created(e)
	case "ServicoExtraAtualizado":
		return p.updated(e)
	case "ServicoExtraDesativado":
		return p.active(e, false)
	case "ServicoExtraReativado":
		return p.active(e, true)
	}
	return nil
}
func (p *ServicoExtraProjection) Rebuild() error {
	if _, e := p.client.DB().Exec(`TRUNCATE projection_servicos_extras CASCADE`); e != nil {
		return e
	}
	rows, e := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='ServicoExtra' ORDER BY id`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var x db.Event
		var prev sql.NullString
		if e = rows.Scan(&x.ID, &x.EventID, &x.AggregateID, &x.AggregateType, &x.EventType, &x.EventVersion, &x.Payload, &x.Metadata, &x.OccurredAt, &x.RecordedAt, &x.LedgerHash, &prev); e != nil {
			return e
		}
		if prev.Valid {
			x.PreviousHash = &prev.String
		}
		if e = p.Handle(x); e != nil {
			return e
		}
	}
	return rows.Err()
}

type ServicoExtraDTO struct {
	ID                            uuid.UUID              `json:"id"`
	CodigoAcademia                string                 `json:"codigo_academia"`
	Nome                          string                 `json:"nome"`
	Descricao                     string                 `json:"descricao"`
	Categoria                     string                 `json:"categoria"`
	Pago                          bool                   `json:"pago"`
	Preco                         *float64               `json:"preco"`
	TipoCobranca                  *string                `json:"tipo_cobranca"`
	MetodosPagamento              []string               `json:"metodos_pagamento"`
	TemTaxaInscricao              bool                   `json:"tem_taxa_inscricao"`
	ValorTaxaInscricao            *float64               `json:"valor_taxa_inscricao"`
	MetodosPagamentoTaxaInscricao []string               `json:"metodos_pagamento_taxa_inscricao"`
	AnosAcademicosDisponiveis     []string               `json:"anos_academicos_disponiveis"`
	CursosDisponiveis             []string               `json:"cursos_disponiveis"`
	DocumentoObrigatorio          bool                   `json:"documento_obrigatorio"`
	DocumentoInstrucoes           string                 `json:"documento_instrucoes"`
	DetalhesPersonalizados        map[string]interface{} `json:"detalhes_personalizados"`
	Ativo                         bool                   `json:"ativo"`
	CreatedAt                     time.Time              `json:"created_at"`
	UpdatedAt                     time.Time              `json:"updated_at"`
}

func (p *ServicoExtraProjection) created(e db.Event) error {
	var x struct {
		CodigoAcademia, Nome, Descricao, Categoria                                  string
		Pago                                                                        bool
		Preco                                                                       float64
		TipoCobranca                                                                string
		MetodosPagamento                                                            []string
		TemTaxaInscricao                                                            bool
		ValorTaxaInscricao                                                          float64
		MetodosPagamentoTaxaInscricao, AnosAcademicosDisponiveis, CursosDisponiveis []string
		DocumentoObrigatorio                                                        bool
		DocumentoInstrucoes                                                         string
		DetalhesPersonalizados                                                      map[string]interface{}
		CriadoPor                                                                   uuid.UUID
		CreatedAt                                                                   time.Time
	}
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	d, _ := json.Marshal(x.DetalhesPersonalizados)
	var preco, taxa interface{}
	var tipo interface{}
	if x.Pago {
		preco = x.Preco
		tipo = x.TipoCobranca
	}
	if x.TemTaxaInscricao {
		taxa = x.ValorTaxaInscricao
	}
	_, err := p.client.DB().Exec(`INSERT INTO projection_servicos_extras(id,codigo_academia,nome,descricao,categoria,pago,preco,tipo_cobranca,metodos_pagamento,tem_taxa_inscricao,valor_taxa_inscricao,metodos_pagamento_taxa_inscricao,anos_academicos_disponiveis,cursos_disponiveis,documento_obrigatorio,documento_instrucoes,detalhes_personalizados,ativo,criado_por,created_at,updated_at,version,last_event_id) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,NULLIF($16,''),$17,true,$18,$19,$19,$20,$21) ON CONFLICT(id) DO NOTHING`, e.AggregateID, x.CodigoAcademia, x.Nome, x.Descricao, x.Categoria, x.Pago, preco, tipo, pq.Array(x.MetodosPagamento), x.TemTaxaInscricao, taxa, pq.Array(x.MetodosPagamentoTaxaInscricao), pq.Array(x.AnosAcademicosDisponiveis), pq.Array(x.CursosDisponiveis), x.DocumentoObrigatorio, x.DocumentoInstrucoes, string(d), x.CriadoPor, x.CreatedAt, e.EventVersion, e.EventID)
	return err
}
func (p *ServicoExtraProjection) updated(e db.Event) error {
	var x map[string]json.RawMessage
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	fields := map[string]string{"Nome": "nome", "Descricao": "descricao", "Categoria": "categoria", "Pago": "pago", "Preco": "preco", "TipoCobranca": "tipo_cobranca", "MetodosPagamento": "metodos_pagamento", "TemTaxaInscricao": "tem_taxa_inscricao", "ValorTaxaInscricao": "valor_taxa_inscricao", "MetodosPagamentoTaxaInscricao": "metodos_pagamento_taxa_inscricao", "AnosAcademicosDisponiveis": "anos_academicos_disponiveis", "CursosDisponiveis": "cursos_disponiveis", "DocumentoObrigatorio": "documento_obrigatorio", "DocumentoInstrucoes": "documento_instrucoes", "DetalhesPersonalizados": "detalhes_personalizados"}
	for k, col := range fields {
		if v, ok := x[k]; ok && string(v) != "null" {
			var val interface{}
			if err := json.Unmarshal(v, &val); err != nil {
				return err
			}
			if _, err := p.client.DB().Exec(fmt.Sprintf("UPDATE projection_servicos_extras SET %s=$1 WHERE id=$2", col), val, e.AggregateID); err != nil {
				return err
			}
		}
	}
	_, err := p.client.DB().Exec(`UPDATE projection_servicos_extras SET preco=CASE WHEN pago THEN preco ELSE NULL END,tipo_cobranca=CASE WHEN pago THEN tipo_cobranca ELSE NULL END,metodos_pagamento=CASE WHEN pago THEN metodos_pagamento ELSE ARRAY[]::TEXT[] END,valor_taxa_inscricao=CASE WHEN tem_taxa_inscricao THEN valor_taxa_inscricao ELSE NULL END,metodos_pagamento_taxa_inscricao=CASE WHEN tem_taxa_inscricao THEN metodos_pagamento_taxa_inscricao ELSE ARRAY[]::TEXT[] END,version=$1,last_event_id=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$3`, e.EventVersion, e.EventID, e.AggregateID)
	return err
}
func (p *ServicoExtraProjection) active(e db.Event, v bool) error {
	_, err := p.client.DB().Exec(`UPDATE projection_servicos_extras SET ativo=$1,version=$2,last_event_id=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$4`, v, e.EventVersion, e.EventID, e.AggregateID)
	return err
}

func (p *ServicoExtraProjection) scan(row interface{ Scan(...interface{}) error }) (*ServicoExtraDTO, error) {
	var d ServicoExtraDTO
	var detalhes []byte
	err := row.Scan(&d.ID, &d.CodigoAcademia, &d.Nome, &d.Descricao, &d.Categoria, &d.Pago, &d.Preco, &d.TipoCobranca, pq.Array(&d.MetodosPagamento), &d.TemTaxaInscricao, &d.ValorTaxaInscricao, pq.Array(&d.MetodosPagamentoTaxaInscricao), pq.Array(&d.AnosAcademicosDisponiveis), pq.Array(&d.CursosDisponiveis), &d.DocumentoObrigatorio, &d.DocumentoInstrucoes, &detalhes, &d.Ativo, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(detalhes, &d.DetalhesPersonalizados)
	return &d, nil
}

const servicoCols = `id,codigo_academia,nome,COALESCE(descricao,''),COALESCE(categoria,''),pago,preco,tipo_cobranca,metodos_pagamento,tem_taxa_inscricao,valor_taxa_inscricao,metodos_pagamento_taxa_inscricao,anos_academicos_disponiveis,cursos_disponiveis,documento_obrigatorio,COALESCE(documento_instrucoes,''),detalhes_personalizados,ativo,created_at,updated_at`

func (p *ServicoExtraProjection) GetByID(id uuid.UUID) (*ServicoExtraDTO, error) {
	d, e := p.scan(p.client.DB().QueryRow(`SELECT `+servicoCols+` FROM projection_servicos_extras WHERE id=$1`, id))
	if e == sql.ErrNoRows {
		return nil, nil
	}
	return d, e
}
func (p *ServicoExtraProjection) GetByAcademia(codigo string, ativosOnly bool) ([]ServicoExtraDTO, error) {
	q := `SELECT ` + servicoCols + ` FROM projection_servicos_extras WHERE codigo_academia=$1`
	if ativosOnly {
		q += ` AND ativo=true`
	}
	q += ` ORDER BY nome`
	rows, e := p.client.DB().Query(q, codigo)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ServicoExtraDTO{}
	for rows.Next() {
		d, e := p.scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}
