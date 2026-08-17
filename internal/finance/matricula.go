package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)

type MatriculaConfiguracaoInput struct {
	CodigoAcademia   string   `json:"codigo_academia"`
	Nivel            string   `json:"nivel"`
	AnoAcademico     string   `json:"ano_academico"`
	CursoID          *string  `json:"curso_id,omitempty"`
	Valor            float64  `json:"valor"`
	MetodosPagamento []string `json:"metodos_pagamento"`
}
type MatriculaConfiguracaoView struct {
	CodigoAcademia   string     `json:"codigo_academia"`
	Nivel            string     `json:"nivel"`
	AnoAcademico     string     `json:"ano_academico"`
	CursoID          *uuid.UUID `json:"curso_id,omitempty"`
	Valor            float64    `json:"valor"`
	MetodosPagamento []string   `json:"metodos_pagamento"`
	VigenteEm        time.Time  `json:"vigente_em"`
}
type MatriculaPagamentoInput struct {
	CodigoSolicitacao string `json:"-"`
	MetodoPagamento   string `json:"metodo_pagamento"`
	Telefone          string `json:"telefone,omitempty"`
}
type MatriculaPagamentoView struct {
	// Charge é QRCodeResult pelo mesmo motivo documentado em
	// MensalidadePagamentoView (internal/finance/mensalidade.go).
	Charge QRCodeResult `json:"cobranca"`
}

func (s *Service) ConfigureMatricula(ctx context.Context, in MatriculaConfiguracaoInput, actorID, actorType, ip string) (MatriculaConfiguracaoView, error) {
	if err := s.validateConfiguracaoMatricula(ctx, &in); err != nil {
		return MatriculaConfiguracaoView{}, err
	}
	in.Valor = roundAmount(in.Valor)
	payload := map[string]any{"codigo_academia": in.CodigoAcademia, "nivel": in.Nivel, "ano_academico": in.AnoAcademico, "curso_id": optionalString(in.CursoID), "valor": in.Valor, "metodos_pagamento": in.MetodosPagamento}
	if err := s.recordMensalidade(ctx, in.CodigoAcademia, aggregates.MatriculaConfigurada, payload, actorID, actorType, ip); err != nil {
		return MatriculaConfiguracaoView{}, err
	}
	return s.ResolveMatriculaConfiguracao(ctx, in.CodigoAcademia, in.Nivel, in.AnoAcademico, in.CursoID)
}
func (s *Service) ListMatriculaConfiguracoes(ctx context.Context, academia string) ([]MatriculaConfiguracaoView, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT ON (nivel,ano_academico,curso_id) nivel,ano_academico,curso_id,valor::float8,metodos_pagamento,vigente_em FROM financeiro_matricula_configuracoes WHERE codigo_academia=$1 ORDER BY nivel,ano_academico,curso_id,vigente_em DESC,event_id DESC`, strings.TrimSpace(academia))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MatriculaConfiguracaoView{}
	for rows.Next() {
		var v MatriculaConfiguracaoView
		var curso sql.NullString
		v.CodigoAcademia = academia
		if err := rows.Scan(&v.Nivel, &v.AnoAcademico, &curso, &v.Valor, pq.Array(&v.MetodosPagamento), &v.VigenteEm); err != nil {
			return nil, err
		}
		if curso.Valid {
			id, err := uuid.Parse(curso.String)
			if err != nil {
				return nil, err
			}
			v.CursoID = &id
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ResolveMatriculaConfiguracao returns ErrNotFound when enrollment is free.
func (s *Service) ResolveMatriculaConfiguracao(ctx context.Context, academia, nivel, ano string, curso *string) (MatriculaConfiguracaoView, error) {
	var v MatriculaConfiguracaoView
	var raw sql.NullString
	err := s.client.DB().QueryRowContext(ctx, `SELECT valor::float8,metodos_pagamento,vigente_em,curso_id FROM financeiro_matricula_configuracoes WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM NULLIF($4,'')::uuid ORDER BY vigente_em DESC,event_id DESC LIMIT 1`, strings.TrimSpace(academia), strings.ToLower(strings.TrimSpace(nivel)), strings.TrimSpace(ano), optionalString(curso)).Scan(&v.Valor, pq.Array(&v.MetodosPagamento), &v.VigenteEm, &raw)
	if err == sql.ErrNoRows {
		return MatriculaConfiguracaoView{}, ErrNotFound
	}
	if err != nil {
		return MatriculaConfiguracaoView{}, err
	}
	v.CodigoAcademia, v.Nivel, v.AnoAcademico = academia, nivel, ano
	if raw.Valid {
		id, err := uuid.Parse(raw.String)
		if err != nil {
			return MatriculaConfiguracaoView{}, err
		}
		v.CursoID = &id
	}
	return v, nil
}
func (s *Service) validateConfiguracaoMatricula(ctx context.Context, in *MatriculaConfiguracaoInput) error {
	in.CodigoAcademia, in.Nivel, in.AnoAcademico = strings.TrimSpace(in.CodigoAcademia), strings.ToLower(strings.TrimSpace(in.Nivel)), strings.TrimSpace(in.AnoAcademico)
	if in.CodigoAcademia == "" || !nivelValido(in.Nivel) || in.AnoAcademico == "" {
		return errors.New("codigo_academia, nivel e ano_academico são obrigatórios")
	}
	if in.Valor <= 0 || !amountsEqual(roundAmount(in.Valor), in.Valor) {
		return errors.New("valor deve ser maior que zero e ter no máximo duas casas decimais")
	}
	if len(in.MetodosPagamento) == 0 {
		return errors.New("ao menos um método de pagamento é obrigatório para matrícula")
	}
	seen := map[string]bool{}
	for i, m := range in.MetodosPagamento {
		m = strings.ToUpper(strings.TrimSpace(m))
		if m != "GPO" && m != "REF" && m != "GPO_QR" {
			return errors.New("metodos_pagamento aceita apenas GPO, REF ou GPO_QR")
		}
		if seen[m] {
			return errors.New("metodos_pagamento não pode conter duplicados")
		}
		seen[m] = true
		in.MetodosPagamento[i] = m
	}
	if _, err := s.loadCredential(ctx, ContextoAcademia, in.CodigoAcademia); err != nil {
		return errors.New("não é possível habilitar matrícula sem credenciais AppyPay da academia")
	}
	var anosRaw []byte
	if err := s.client.DB().QueryRowContext(ctx, `SELECT anos_academicos FROM projection_academias WHERE codigo_academia=$1`, in.CodigoAcademia).Scan(&anosRaw); err == sql.ErrNoRows {
		return fmt.Errorf("%w: academia", ErrNotFound)
	} else if err != nil {
		return err
	}
	if in.Nivel == NivelFundamental {
		if in.CursoID != nil && strings.TrimSpace(*in.CursoID) != "" {
			return errors.New("curso_id não é permitido para o " + utils.RotuloEnsinoFundamentalGenerico)
		}
		var anos []string
		if json.Unmarshal(anosRaw, &anos) != nil || !contains(anos, in.AnoAcademico) {
			return errors.New("ano acadêmico do " + utils.RotuloEnsinoFundamentalGenerico + " não é oferecido pela academia")
		}
		return nil
	}
	if in.CursoID == nil || strings.TrimSpace(*in.CursoID) == "" {
		return errors.New("curso_id é obrigatório para ensino médio e superior")
	}
	id, err := uuid.Parse(*in.CursoID)
	if err != nil {
		return errors.New("curso_id inválido")
	}
	var tipo, cod string
	var anosCurso []byte
	if err = s.client.DB().QueryRowContext(ctx, `SELECT type,codigo_academia,anos_academicos FROM projection_cursos WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&tipo, &cod, &anosCurso); err == sql.ErrNoRows {
		return errors.New("curso não encontrado")
	} else if err != nil {
		return err
	}
	var anos []string
	if cod != in.CodigoAcademia || tipo != in.Nivel || json.Unmarshal(anosCurso, &anos) != nil || !contains(anos, in.AnoAcademico) {
		return errors.New("curso ou ano_academico não é oferecido pela academia")
	}
	return nil
}
func (s *Service) IniciarPagamentoMatricula(ctx context.Context, in MatriculaPagamentoInput, ip string) (MatriculaPagamentoView, error) {
	in.CodigoSolicitacao, in.MetodoPagamento = strings.TrimSpace(in.CodigoSolicitacao), strings.ToUpper(strings.TrimSpace(in.MetodoPagamento))
	if in.CodigoSolicitacao == "" || in.MetodoPagamento == "" {
		return MatriculaPagamentoView{}, errors.New("solicitação e método de pagamento são obrigatórios")
	}
	var academia, status string
	var valor sql.NullFloat64
	var metodos []string
	err := s.client.DB().QueryRowContext(ctx, `SELECT codigo_academia,status,valor_matricula::float8,metodos_pagamento_matricula FROM projection_solicitacoes_matricula WHERE codigo_solicitacao=$1`, in.CodigoSolicitacao).Scan(&academia, &status, &valor, pq.Array(&metodos))
	if err != nil || status != "aprovada_pendente_pagamento_matricula" || !valor.Valid {
		return MatriculaPagamentoView{}, errors.New("solicitação não disponível para pagamento de matrícula")
	}
	if !contains(metodos, in.MetodoPagamento) {
		return MatriculaPagamentoView{}, errors.New("método de pagamento não está habilitado para matrícula nesta academia")
	}
	open, err := s.matriculaTemCobrancaAberta(ctx, in.CodigoSolicitacao)
	if err != nil {
		return MatriculaPagamentoView{}, err
	}
	if open {
		return MatriculaPagamentoView{}, errors.New("solicitação já possui cobrança de matrícula em aberto")
	}
	desc := "Taxa de matrícula " + academia
	if in.MetodoPagamento == "GPO_QR" {
		qr, err := s.CreateGPOQRCode(ctx, QRCodeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: academia, CodigoSolicitacao: in.CodigoSolicitacao, Amount: valor.Float64, Currency: "AOA", Description: desc, MerchantTransactionID: merchantID()}, "solicitacao:"+in.CodigoSolicitacao, "solicitante", ip)
		if err != nil {
			return MatriculaPagamentoView{}, err
		}
		return MatriculaPagamentoView{Charge: qr}, nil
	}
	info := map[string]any{}
	if in.MetodoPagamento == "GPO" {
		info["phoneNumber"] = strings.TrimSpace(in.Telefone)
	}
	charge, err := s.CreateCharge(ctx, ChargeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: academia, CodigoSolicitacao: in.CodigoSolicitacao, Amount: valor.Float64, Currency: "AOA", Description: desc, MerchantTransactionID: merchantID(), PaymentMethod: in.MetodoPagamento, PaymentInfo: info}, "solicitacao:"+in.CodigoSolicitacao, "solicitante", ip)
	if err != nil {
		return MatriculaPagamentoView{}, err
	}
	return MatriculaPagamentoView{Charge: QRCodeResult{ChargeResult: charge}}, nil
}
func (s *Service) matriculaTemCobrancaAberta(ctx context.Context, codigo string) (bool, error) {
	var ok bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_cobrancas WHERE payload->>'codigo_solicitacao'=$1 AND COALESCE(payload->>'status','') NOT IN ('Success','success','cancelada','falhada'))`, codigo).Scan(&ok)
	return ok, err
}

// CodigoSolicitacaoDaCobranca identifies an enrollment charge without exposing
// applicant details to AppyPay-facing code.
func (s *Service) CodigoSolicitacaoDaCobranca(ctx context.Context, identifier string) (string, error) {
	row, err := s.loadCharge(ctx, identifier)
	if err != nil {
		return "", err
	}
	codigo, _ := row.Payload["codigo_solicitacao"].(string)
	return strings.TrimSpace(codigo), nil
}
func (s *Service) CancelarCobrancaMatriculaAberta(ctx context.Context, codigo, motivo, actorID, actorType, ip string) error {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id::text,codigo_academia FROM financeiro_cobrancas WHERE payload->>'codigo_solicitacao'=$1 AND COALESCE(payload->>'status','') NOT IN ('Success','success','cancelada','falhada')`, codigo)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, academia string
		if err := rows.Scan(&id, &academia); err != nil {
			return err
		}
		if _, err := s.CancelCharge(ctx, ContextoAcademia, academia, id, motivo, actorID, actorType, ip); err != nil {
			return err
		}
	}
	return rows.Err()
}
