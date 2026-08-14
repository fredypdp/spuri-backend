package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
)

const (
	NivelFundamental = "fundamental"
	NivelMedio       = "medio"
	NivelSuperior    = "superior"

	EstadoPendente = "pendente"
	EstadoPago     = "pago"
	EstadoAnulado  = "anulado"
)

type MensalidadeConfiguracaoInput struct {
	CodigoAcademia   string   `json:"codigo_academia"`
	Nivel            string   `json:"nivel"`
	AnoAcademico     string   `json:"ano_academico"`
	CursoID          *string  `json:"curso_id,omitempty"`
	Valor            float64  `json:"valor"`
	MesFimCobranca   int      `json:"mes_fim_cobranca"`
	MetodosPagamento []string `json:"metodos_pagamento"`
}

type MensalidadeConfiguracaoView struct {
	CodigoAcademia   string     `json:"codigo_academia"`
	Nivel            string     `json:"nivel"`
	AnoAcademico     string     `json:"ano_academico"`
	CursoID          *uuid.UUID `json:"curso_id,omitempty"`
	Valor            float64    `json:"valor"`
	MesFimCobranca   int        `json:"mes_fim_cobranca"`
	MetodosPagamento []string   `json:"metodos_pagamento"`
	VigenteEm        time.Time  `json:"vigente_em"`
}

type MesInicioCobrancaInput struct {
	CodigoAcademia string `json:"codigo_academia"`
	AnoLetivo      string `json:"ano_letivo"`
	MesInicio      int    `json:"mes_inicio"`
}

type ObrigacaoMensalidadeInput struct {
	CodigoEstudante string `json:"codigo_estudante"`
	CodigoAcademia  string `json:"codigo_academia"`
	AnoLetivo       string `json:"ano_letivo"`
	Meses           []int  `json:"meses"`
	Motivo          string `json:"motivo,omitempty"`
}

type MensalidadeMesView struct {
	CodigoEstudante  string      `json:"codigo_estudante"`
	CodigoAcademia   string      `json:"codigo_academia"`
	AnoLetivo        string      `json:"ano_letivo"`
	Mes              int         `json:"mes"`
	DataReferencia   time.Time   `json:"data_referencia"`
	Nivel            string      `json:"nivel"`
	AnoAcademico     string      `json:"ano_academico"`
	CursoID          *uuid.UUID  `json:"curso_id,omitempty"`
	Valor            float64     `json:"valor"`
	MesFimCobranca   int         `json:"mes_fim_cobranca"`
	Estado           string      `json:"estado"`
	EventosAuditoria []uuid.UUID `json:"eventos_auditoria,omitempty"`
}

type MensalidadeSelecaoMes struct {
	AnoLetivo string `json:"ano_letivo"`
	Mes       int    `json:"mes"`
}

type MensalidadePagamentoInput struct {
	CodigoEstudante string                  `json:"-"`
	CodigoAcademia  string                  `json:"codigo_academia"`
	Meses           []MensalidadeSelecaoMes `json:"meses"`
	MetodoPagamento string                  `json:"metodo_pagamento"`
	Telefone        string                  `json:"telefone,omitempty"`
}

type MensalidadePagamentoView struct {
	Charge ChargeResult            `json:"cobranca"`
	Meses  []MensalidadeSelecaoMes `json:"meses"`
}

type mensalidadeVinculo struct {
	CodigoAcademia string
	AnoLetivo      string
	Nivel          string
	AnoAcademico   string
	CursoID        *uuid.UUID
}

// ConfigureMensalidade appends a new version; it never updates an existing
// price. Historical prices are resolved later by the reference month.
func (s *Service) ConfigureMensalidade(ctx context.Context, in MensalidadeConfiguracaoInput, actorID, actorType, ip string) (MensalidadeConfiguracaoView, error) {
	if s.client == nil {
		return MensalidadeConfiguracaoView{}, errors.New("serviço financeiro não inicializado")
	}
	if err := s.validateConfiguracaoMensalidade(ctx, &in); err != nil {
		return MensalidadeConfiguracaoView{}, err
	}
	in.Valor = roundAmount(in.Valor)
	payload := map[string]any{"codigo_academia": in.CodigoAcademia, "nivel": in.Nivel, "ano_academico": in.AnoAcademico, "curso_id": optionalString(in.CursoID), "valor": in.Valor, "mes_fim_cobranca": in.MesFimCobranca, "metodos_pagamento": in.MetodosPagamento}
	if err := s.recordMensalidade(ctx, in.CodigoAcademia, aggregates.MensalidadeConfigurada, payload, actorID, actorType, ip); err != nil {
		return MensalidadeConfiguracaoView{}, err
	}
	var cursoID *uuid.UUID
	if in.CursoID != nil && strings.TrimSpace(*in.CursoID) != "" {
		id, err := uuid.Parse(*in.CursoID)
		if err != nil {
			return MensalidadeConfiguracaoView{}, err
		}
		cursoID = &id
	}
	return s.resolveConfiguracao(ctx, in.CodigoAcademia, in.Nivel, in.AnoAcademico, cursoID, time.Now().UTC())
}

func (s *Service) ListMensalidadeConfiguracoes(ctx context.Context, codigoAcademia string) ([]MensalidadeConfiguracaoView, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT ON (nivel,ano_academico,curso_id) nivel,ano_academico,curso_id,valor::float8,mes_fim_cobranca,metodos_pagamento,vigente_em FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1 ORDER BY nivel,ano_academico,curso_id,vigente_em DESC,event_id DESC`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MensalidadeConfiguracaoView
	for rows.Next() {
		var v MensalidadeConfiguracaoView
		var curso sql.NullString
		v.CodigoAcademia = codigoAcademia
		if err := rows.Scan(&v.Nivel, &v.AnoAcademico, &curso, &v.Valor, &v.MesFimCobranca, &v.MetodosPagamento, &v.VigenteEm); err != nil {
			return nil, err
		}
		if curso.Valid {
			id, err := uuid.Parse(curso.String)
			if err != nil {
				return nil, err
			}
			v.CursoID = &id
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (s *Service) DefinirMesInicioCobranca(ctx context.Context, in MesInicioCobrancaInput, actorID, actorType, ip string) error {
	if err := s.validateMesInicioCobranca(ctx, &in); err != nil {
		return err
	}
	return s.recordMensalidade(ctx, in.CodigoAcademia, aggregates.MesInicioCobrancaDefinido, map[string]any{"codigo_academia": in.CodigoAcademia, "ano_letivo": in.AnoLetivo, "mes_inicio": in.MesInicio}, actorID, actorType, ip)
}

func (s *Service) AnularObrigacoesMensalidade(ctx context.Context, in ObrigacaoMensalidadeInput, actorID, actorType, ip string) error {
	return s.alterarObrigacoesMensalidade(ctx, in, aggregates.ObrigacaoMensalidadeAnulada, actorID, actorType, ip)
}
func (s *Service) ReativarObrigacoesMensalidade(ctx context.Context, in ObrigacaoMensalidadeInput, actorID, actorType, ip string) error {
	return s.alterarObrigacoesMensalidade(ctx, in, aggregates.ObrigacaoMensalidadeReativada, actorID, actorType, ip)
}

func (s *Service) alterarObrigacoesMensalidade(ctx context.Context, in ObrigacaoMensalidadeInput, eventType, actorID, actorType, ip string) error {
	in.CodigoEstudante, in.CodigoAcademia, in.AnoLetivo = strings.TrimSpace(in.CodigoEstudante), strings.TrimSpace(in.CodigoAcademia), strings.TrimSpace(in.AnoLetivo)
	if in.CodigoEstudante == "" || in.CodigoAcademia == "" || !anoLetivoValido(in.AnoLetivo) || len(in.Meses) == 0 {
		return errors.New("estudante, academia, ano_letivo válido e meses são obrigatórios")
	}
	seen := map[int]bool{}
	for _, mes := range in.Meses {
		if seen[mes] || !mesValido(mes) {
			return errors.New("meses devem ser distintos e estar entre 1 e 12")
		}
		seen[mes] = true
		if _, err := s.mesDevido(ctx, in.CodigoEstudante, in.CodigoAcademia, in.AnoLetivo, mes); err != nil {
			return err
		}
		state, _, err := s.estadoObrigacao(ctx, in.CodigoEstudante, in.CodigoAcademia, in.AnoLetivo, mes)
		if err != nil {
			return err
		}
		if eventType == aggregates.ObrigacaoMensalidadeAnulada && state == EstadoPago {
			return errors.New("não é possível anular uma mensalidade já paga")
		}
		if eventType == aggregates.ObrigacaoMensalidadeReativada && state != EstadoAnulado {
			return errors.New("só é possível reativar uma mensalidade anulada e não paga")
		}
	}
	for _, mes := range in.Meses {
		if err := s.recordMensalidade(ctx, in.CodigoAcademia, eventType, map[string]any{"codigo_estudante": in.CodigoEstudante, "codigo_academia": in.CodigoAcademia, "ano_letivo": in.AnoLetivo, "mes": mes, "motivo": strings.TrimSpace(in.Motivo)}, actorID, actorType, ip); err != nil {
			return err
		}
	}
	if eventType == aggregates.ObrigacaoMensalidadeAnulada {
		for _, mes := range in.Meses {
			// The annulment is already committed. A charge that won the provider
			// race is reconciled by CancelCharge and never invalidates it.
			_ = s.cancelOpenMensalidadeCharges(ctx, in.CodigoEstudante, in.CodigoAcademia, in.AnoLetivo, mes, actorID, actorType, ip)
		}
	}
	return nil
}

// IniciarPagamentoMensalidades is the student-only orchestration layer. It
// never exposes CreateCharge as a student action; it creates one academy-owned
// charge after validating every selected due month locally.
func (s *Service) IniciarPagamentoMensalidades(ctx context.Context, in MensalidadePagamentoInput, actorID, actorType, ip string) (MensalidadePagamentoView, error) {
	in.CodigoEstudante, in.CodigoAcademia = strings.TrimSpace(in.CodigoEstudante), strings.TrimSpace(in.CodigoAcademia)
	in.MetodoPagamento = strings.ToUpper(strings.TrimSpace(in.MetodoPagamento))
	if in.CodigoEstudante == "" || in.CodigoAcademia == "" || len(in.Meses) == 0 || actorType != "estudante" || strings.TrimSpace(actorID) == "" {
		return MensalidadePagamentoView{}, errors.New("somente o estudante pode enviar codigo_academia e pelo menos um mês")
	}
	metodos, err := s.metodosPagamentoMensalidade(ctx, in.CodigoAcademia)
	if err != nil {
		return MensalidadePagamentoView{}, err
	}
	if !contains(metodos, in.MetodoPagamento) {
		return MensalidadePagamentoView{}, errors.New("método de pagamento não está habilitado para propina nesta academia")
	}
	all, err := s.ListMensalidades(ctx, in.CodigoEstudante, &in.CodigoAcademia)
	if err != nil {
		return MensalidadePagamentoView{}, err
	}
	pendentes := make([]MensalidadeMesView, 0)
	for _, v := range all {
		if v.Estado == EstadoPendente {
			pendentes = append(pendentes, v)
		}
	}
	if len(pendentes) == 0 {
		return MensalidadePagamentoView{}, errors.New("não há mensalidades pendentes nesta academia")
	}
	selected, total := map[string]bool{}, 0.0
	for _, m := range in.Meses {
		key := m.AnoLetivo + ":" + strconv.Itoa(m.Mes)
		if selected[key] {
			return MensalidadePagamentoView{}, errors.New("mês selecionado mais de uma vez")
		}
		selected[key] = true
		found := false
		for _, due := range pendentes {
			if due.AnoLetivo == m.AnoLetivo && due.Mes == m.Mes {
				total += due.Valor
				found = true
				break
			}
		}
		if !found {
			return MensalidadePagamentoView{}, fmt.Errorf("mensalidade %s/%02d não está pendente", m.AnoLetivo, m.Mes)
		}
		open, err := s.mensalidadeTemCobrancaAberta(ctx, in.CodigoEstudante, in.CodigoAcademia, m.AnoLetivo, m.Mes)
		if err != nil {
			return MensalidadePagamentoView{}, err
		}
		if open {
			return MensalidadePagamentoView{}, fmt.Errorf("mensalidade %s/%02d já possui cobrança em aberto", m.AnoLetivo, m.Mes)
		}
	}
	oldest := pendentes[0]
	if !selected[oldest.AnoLetivo+":"+strconv.Itoa(oldest.Mes)] {
		return MensalidadePagamentoView{}, fmt.Errorf("a seleção deve incluir a mensalidade pendente mais antiga: %s/%02d", oldest.AnoLetivo, oldest.Mes)
	}
	total = roundAmount(total)
	description, merchant := fmt.Sprintf("Propinas %s: %d mensalidade(s)", in.CodigoAcademia, len(in.Meses)), merchantID()
	if in.MetodoPagamento == "GPO_QR" {
		qr, err := s.CreateGPOQRCode(ctx, QRCodeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: in.CodigoAcademia, CodigoEstudante: in.CodigoEstudante, Amount: total, Currency: "AOA", Description: description, MerchantTransactionID: merchant, Mensalidades: in.Meses}, actorID, actorType, ip)
		if err != nil {
			return MensalidadePagamentoView{}, err
		}
		return MensalidadePagamentoView{Charge: qr.ChargeResult, Meses: in.Meses}, nil
	}
	info := map[string]any{}
	if in.MetodoPagamento == "GPO" {
		info["phoneNumber"] = strings.TrimSpace(in.Telefone)
	}
	charge, err := s.CreateCharge(ctx, ChargeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: in.CodigoAcademia, CodigoEstudante: in.CodigoEstudante, Mensalidades: in.Meses, Amount: total, Currency: "AOA", Description: description, MerchantTransactionID: merchant, PaymentMethod: in.MetodoPagamento, PaymentInfo: info}, actorID, actorType, ip)
	if err != nil {
		return MensalidadePagamentoView{}, err
	}
	return MensalidadePagamentoView{Charge: charge, Meses: in.Meses}, nil
}

// ListMensalidades derives every due month from historical turma membership,
// configuration versions and immutable obligation events. It writes nothing.
func (s *Service) ListMensalidades(ctx context.Context, codigoEstudante string, somenteAcademia *string) ([]MensalidadeMesView, error) {
	vinculos, err := s.vinculosMensalidade(ctx, strings.TrimSpace(codigoEstudante), somenteAcademia)
	if err != nil {
		return nil, err
	}
	result := []MensalidadeMesView{}
	for _, v := range vinculos {
		inicio, err := s.mesInicioEfetivo(ctx, v.CodigoAcademia, v.AnoLetivo, v.Nivel)
		if err != nil {
			return nil, err
		}
		for _, ref := range mesesAnoLetivo(v.AnoLetivo, v.Nivel) {
			if ref.Month < inicio {
				continue
			}
			cfg, err := s.resolveConfiguracao(ctx, v.CodigoAcademia, v.Nivel, v.AnoAcademico, v.CursoID, ref.Data)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if ref.Month > cfg.MesFimCobranca {
				continue
			}
			state, audit, err := s.estadoObrigacao(ctx, codigoEstudante, v.CodigoAcademia, v.AnoLetivo, ref.Month)
			if err != nil {
				return nil, err
			}
			result = append(result, MensalidadeMesView{CodigoEstudante: codigoEstudante, CodigoAcademia: v.CodigoAcademia, AnoLetivo: v.AnoLetivo, Mes: ref.Month, DataReferencia: ref.Data, Nivel: v.Nivel, AnoAcademico: v.AnoAcademico, CursoID: v.CursoID, Valor: cfg.Valor, MesFimCobranca: cfg.MesFimCobranca, Estado: state, EventosAuditoria: audit})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AnoLetivo != result[j].AnoLetivo {
			return result[i].AnoLetivo < result[j].AnoLetivo
		}
		if result[i].CodigoAcademia != result[j].CodigoAcademia {
			return result[i].CodigoAcademia < result[j].CodigoAcademia
		}
		return result[i].Mes < result[j].Mes
	})
	return result, nil
}

// ResolveMensalidade is the public price resolver to be reused by Phase 3.
func (s *Service) ResolveMensalidade(ctx context.Context, codigoEstudante, codigoAcademia, anoLetivo string, mes int) (MensalidadeMesView, error) {
	return s.mesDevido(ctx, codigoEstudante, codigoAcademia, anoLetivo, mes)
}

func (s *Service) mesDevido(ctx context.Context, estudante, academia, anoLetivo string, mes int) (MensalidadeMesView, error) {
	if !mesValido(mes) || !anoLetivoValido(anoLetivo) {
		return MensalidadeMesView{}, errors.New("mês ou ano_letivo inválido")
	}
	all, err := s.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		return MensalidadeMesView{}, err
	}
	for _, v := range all {
		if v.AnoLetivo == anoLetivo && v.Mes == mes {
			return v, nil
		}
	}
	return MensalidadeMesView{}, errors.New("mês fora do período de mensalidade configurado")
}

func (s *Service) validateConfiguracaoMensalidade(ctx context.Context, in *MensalidadeConfiguracaoInput) error {
	in.CodigoAcademia, in.Nivel, in.AnoAcademico = strings.TrimSpace(in.CodigoAcademia), strings.ToLower(strings.TrimSpace(in.Nivel)), strings.TrimSpace(in.AnoAcademico)
	if in.CodigoAcademia == "" || !nivelValido(in.Nivel) || in.AnoAcademico == "" {
		return errors.New("codigo_academia, nivel e ano_academico são obrigatórios")
	}
	if in.Valor <= 0 || roundAmount(in.Valor) != in.Valor {
		return errors.New("valor deve ser maior que zero e ter no máximo duas casas decimais")
	}
	if in.MesFimCobranca != 6 && in.MesFimCobranca != 7 {
		return errors.New("mes_fim_cobranca deve ser 6 ou 7")
	}
	seenMethods := map[string]bool{}
	for i, method := range in.MetodosPagamento {
		method = strings.ToUpper(strings.TrimSpace(method))
		if method != "GPO" && method != "REF" && method != "GPO_QR" {
			return errors.New("metodos_pagamento aceita apenas GPO, REF ou GPO_QR")
		}
		if seenMethods[method] {
			return errors.New("metodos_pagamento não pode conter duplicados")
		}
		seenMethods[method] = true
		in.MetodosPagamento[i] = method
	}
	if len(in.MetodosPagamento) > 0 {
		if _, err := s.loadCredential(ctx, ContextoAcademia, in.CodigoAcademia); err != nil {
			return errors.New("não é possível habilitar propina sem credenciais AppyPay da academia")
		}
	}
	var typ, nivelAcademia string
	var anosRaw []byte
	err := s.client.DB().QueryRowContext(ctx, `SELECT type,nivel,anos_academicos FROM projection_academias WHERE codigo_academia=$1`, in.CodigoAcademia).Scan(&typ, &nivelAcademia, &anosRaw)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: academia", ErrNotFound)
	}
	if err != nil {
		return err
	}
	if typ != "private" {
		return errors.New("mensalidade só pode ser configurada por academia privada")
	}
	// Both immutable teaching periods currently finish in July. Keep the
	// financial limit explicit so it cannot silently outgrow a future period.
	if nivelAcademia != "escola" && nivelAcademia != "superior" {
		return errors.New("tipo de academia invalido para configuracao de mensalidade")
	}
	if in.MesFimCobranca > 7 {
		return errors.New("mes_fim_cobranca nao pode exceder o periodo letivo fixo")
	}
	if in.Nivel == NivelFundamental {
		if in.CursoID != nil && strings.TrimSpace(*in.CursoID) != "" {
			return errors.New("curso_id não é permitido para ensino fundamental")
		}
		var anos []string
		if err := jsonUnmarshal(anosRaw, &anos); err != nil || !contains(anos, in.AnoAcademico) {
			return errors.New("ano_academico fundamental não é oferecido pela academia")
		}
		return nil
	}
	if in.CursoID == nil || strings.TrimSpace(*in.CursoID) == "" {
		return errors.New("curso_id é obrigatório para ensino médio e superior")
	}
	cursoID, err := uuid.Parse(*in.CursoID)
	if err != nil {
		return errors.New("curso_id inválido")
	}
	var cursoTipo, codigoCurso string
	var anosRawCurso []byte
	err = s.client.DB().QueryRowContext(ctx, `SELECT type,codigo_academia,anos_academicos FROM projection_cursos WHERE id=$1 AND deleted_at IS NULL`, cursoID).Scan(&cursoTipo, &codigoCurso, &anosRawCurso)
	if err == sql.ErrNoRows {
		return errors.New("curso não encontrado")
	}
	if err != nil {
		return err
	}
	var anos []string
	if codigoCurso != in.CodigoAcademia || cursoTipo != in.Nivel || jsonUnmarshal(anosRawCurso, &anos) != nil || !contains(anos, in.AnoAcademico) {
		return errors.New("curso ou ano_academico não é oferecido pela academia")
	}
	return nil
}

func (s *Service) validateMesInicioCobranca(ctx context.Context, in *MesInicioCobrancaInput) error {
	in.CodigoAcademia, in.AnoLetivo = strings.TrimSpace(in.CodigoAcademia), strings.TrimSpace(in.AnoLetivo)
	if in.CodigoAcademia == "" || !anoLetivoValido(in.AnoLetivo) || !mesValido(in.MesInicio) {
		return errors.New("codigo_academia, ano_letivo válido e mes_inicio são obrigatórios")
	}
	var typ, nivel string
	err := s.client.DB().QueryRowContext(ctx, `SELECT type,nivel FROM projection_academias WHERE codigo_academia=$1`, in.CodigoAcademia).Scan(&typ, &nivel)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: academia", ErrNotFound)
	}
	if err != nil {
		return err
	}
	if typ != "private" {
		return errors.New("mensalidade só pode ser configurada por academia privada")
	}
	natural := 9
	if nivel == "superior" {
		natural = 10
	}
	if in.MesInicio < natural {
		return fmt.Errorf("mes_inicio não pode ser anterior a %02d", natural)
	}
	var menor sql.NullInt64
	err = s.client.DB().QueryRowContext(ctx, `SELECT MIN(mes_fim_cobranca) FROM (SELECT DISTINCT ON (nivel,ano_academico,curso_id) mes_fim_cobranca FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1 ORDER BY nivel,ano_academico,curso_id,vigente_em DESC,event_id DESC) c`, in.CodigoAcademia).Scan(&menor)
	if err != nil {
		return err
	}
	if menor.Valid && in.MesInicio > int(menor.Int64) {
		return errors.New("mes_inicio não pode ser posterior ao mes_fim_cobranca configurado")
	}
	return nil
}

func (s *Service) recordMensalidade(ctx context.Context, codigoAcademia, event string, payload map[string]any, userID, userType, ip string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("autor do evento financeiro é obrigatório")
	}
	agg := aggregates.NewConfiguracaoMensalidadeWithID(mensalidadeAggregateID(codigoAcademia))
	agg.Registrar(event, payload)
	if err := s.repository.WithContext(ctx).SaveWithAudit(agg, db.AuditContext{UserID: userID, UserType: userType, IP: ip}); err != nil {
		return err
	}
	return s.projection.ApplyLatestForAggregate(agg.ID)
}

func mensalidadeAggregateID(codigoAcademia string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("spuri:mensalidade:"+strings.ToLower(strings.TrimSpace(codigoAcademia))))
}

func (s *Service) resolveConfiguracao(ctx context.Context, academia, nivel, ano string, curso *uuid.UUID, referencia time.Time) (MensalidadeConfiguracaoView, error) {
	var out MensalidadeConfiguracaoView
	var cursoText sql.NullString
	// The first configuration is the best information available for every
	// earlier month of that academic year. Later configurations remain forward
	// only: they win only when their effective date is not after the reference.
	err := s.client.DB().QueryRowContext(ctx, `SELECT curso_id,valor::float8,mes_fim_cobranca,vigente_em FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4 ORDER BY CASE WHEN vigente_em <= $5 THEN 0 ELSE 1 END, CASE WHEN vigente_em <= $5 THEN vigente_em END DESC, CASE WHEN vigente_em > $5 THEN vigente_em END ASC, event_id DESC LIMIT 1`, academia, nivel, ano, nullableUUID(curso), referencia.UTC()).Scan(&cursoText, &out.Valor, &out.MesFimCobranca, &out.VigenteEm)
	if err == sql.ErrNoRows {
		return out, fmt.Errorf("%w: configuração de mensalidade", ErrNotFound)
	}
	if err != nil {
		return out, err
	}
	out.CodigoAcademia, out.Nivel, out.AnoAcademico = academia, nivel, ano
	if cursoText.Valid {
		id, err := uuid.Parse(cursoText.String)
		if err != nil {
			return out, err
		}
		out.CursoID = &id
	}
	return out, nil
}

func (s *Service) vinculosMensalidade(ctx context.Context, estudante string, somenteAcademia *string) ([]mensalidadeVinculo, error) {
	if estudante == "" {
		return nil, errors.New("codigo_estudante é obrigatório")
	}
	args := []any{estudante}
	filter := ""
	if somenteAcademia != nil {
		args = append(args, *somenteAcademia)
		filter = " AND codigo_academia=$2"
	}
	q := `WITH vinculos AS (
		SELECT t.codigo_academia, h.key AS ano_letivo, t.nivel AS ano_academico, t.curso_id, COALESCE(c.type, CASE WHEN t.nivel LIKE '%_ano_fundamental' THEN 'fundamental' END) AS nivel
		FROM projection_turmas t CROSS JOIN LATERAL jsonb_each(t.historico_estudantes_ano_letivo) h LEFT JOIN projection_cursos c ON c.id=t.curso_id JOIN projection_academias a ON a.codigo_academia=t.codigo_academia
		WHERE h.value ? $1 AND a.type='private'
		UNION
		SELECT t.codigo_academia, a.ano_letivo, t.nivel, t.curso_id, COALESCE(c.type, CASE WHEN t.nivel LIKE '%_ano_fundamental' THEN 'fundamental' END)
		FROM projection_turmas t LEFT JOIN projection_cursos c ON c.id=t.curso_id JOIN projection_academias a ON a.codigo_academia=t.codigo_academia
		WHERE t.estudantes ? $1 AND a.type='private' AND a.ano_letivo IS NOT NULL
	) SELECT codigo_academia,ano_letivo,nivel,ano_academico,curso_id FROM vinculos WHERE nivel IS NOT NULL` + filter
	rows, err := s.client.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var result []mensalidadeVinculo
	for rows.Next() {
		var v mensalidadeVinculo
		var curso sql.NullString
		if err := rows.Scan(&v.CodigoAcademia, &v.AnoLetivo, &v.Nivel, &v.AnoAcademico, &curso); err != nil {
			return nil, err
		}
		if !anoLetivoValido(v.AnoLetivo) || !nivelValido(v.Nivel) {
			continue
		}
		if curso.Valid {
			id, err := uuid.Parse(curso.String)
			if err != nil {
				return nil, err
			}
			v.CursoID = &id
		}
		key := v.CodigoAcademia + ":" + v.AnoLetivo + ":" + v.Nivel + ":" + v.AnoAcademico + ":" + optionalUUID(v.CursoID)
		if !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	return result, rows.Err()
}

func (s *Service) mesInicioEfetivo(ctx context.Context, academia, anoLetivo, nivel string) (int, error) {
	natural := 9
	if nivel == NivelSuperior {
		natural = 10
	}
	var mes int
	err := s.client.DB().QueryRowContext(ctx, `SELECT mes_inicio FROM financeiro_mensalidade_inicio_cobranca WHERE codigo_academia=$1 AND ano_letivo=$2 ORDER BY definido_em DESC,event_id DESC LIMIT 1`, academia, anoLetivo).Scan(&mes)
	if err == sql.ErrNoRows {
		return natural, nil
	}
	if err != nil {
		return 0, err
	}
	if mes < natural {
		return 0, errors.New("configuração de mes_inicio inconsistente")
	}
	return mes, nil
}

func (s *Service) estadoObrigacao(ctx context.Context, estudante, academia, anoLetivo string, mes int) (string, []uuid.UUID, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT event_id,tipo FROM financeiro_mensalidade_obrigacoes_eventos WHERE codigo_estudante=$1 AND codigo_academia=$2 AND ano_letivo=$3 AND mes=$4 ORDER BY ocorrido_em,event_id`, estudante, academia, anoLetivo, mes)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var audit []uuid.UUID
	var eventos []string
	for rows.Next() {
		var id uuid.UUID
		var typ string
		if err := rows.Scan(&id, &typ); err != nil {
			return "", nil, err
		}
		audit = append(audit, id)
		eventos = append(eventos, typ)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	return precedenciaEstado(eventos), audit, nil
}

// precedenciaEstado centraliza a regra dos eventos imutáveis de obrigação.
// Um pagamento real prevalece sobre anulação posterior, e uma reativação só
// produz efeito quando a obrigação está anulada.
func precedenciaEstado(eventos []string) string {
	state := EstadoPendente
	for _, typ := range eventos {
		switch typ {
		case "anulada":
			if state != EstadoPago {
				state = EstadoAnulado
			}
		case "reativada":
			if state == EstadoAnulado {
				state = EstadoPendente
			}
		case "paga":
			state = EstadoPago
		}
	}
	return state
}

type mesReferencia struct {
	Month int
	Data  time.Time
}

func mesesAnoLetivo(anoLetivo, nivel string) []mesReferencia {
	ano, _ := strconv.Atoi(anoLetivo[:4])
	inicio := 9
	if nivel == NivelSuperior {
		inicio = 10
	}
	var out []mesReferencia
	for m := inicio; m <= 12; m++ {
		out = append(out, mesReferencia{m, time.Date(ano, time.Month(m), 1, 0, 0, 0, 0, time.UTC)})
	}
	for m := 1; m <= 7; m++ {
		out = append(out, mesReferencia{m, time.Date(ano+1, time.Month(m), 1, 0, 0, 0, 0, time.UTC)})
	}
	return out
}
func anoLetivoValido(v string) bool {
	if len(v) != 9 || v[4] != '_' {
		return false
	}
	a, e1 := strconv.Atoi(v[:4])
	b, e2 := strconv.Atoi(v[5:])
	return e1 == nil && e2 == nil && b == a+1
}
func mesValido(v int) bool { return v >= 1 && v <= 12 }
func nivelValido(v string) bool {
	return v == NivelFundamental || v == NivelMedio || v == NivelSuperior
}
func contains(v []string, wanted string) bool {
	for _, x := range v {
		if x == wanted {
			return true
		}
	}
	return false
}
func optionalString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
func optionalUUID(v *uuid.UUID) string {
	if v == nil {
		return ""
	}
	return v.String()
}
func nullableUUID(v *uuid.UUID) any {
	if v == nil {
		return nil
	}
	return v.String()
}

func (s *Service) metodosPagamentoMensalidade(ctx context.Context, academia string) ([]string, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT unnest(metodos_pagamento) FROM (
		SELECT DISTINCT ON (nivel,ano_academico,curso_id) metodos_pagamento
		FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1
		ORDER BY nivel,ano_academico,curso_id,vigente_em DESC,event_id DESC
	) configuracoes`, academia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, strings.ToUpper(v))
	}
	return out, rows.Err()
}

func (s *Service) MetodosPagamentoMensalidade(ctx context.Context, academia string) ([]string, error) {
	return s.metodosPagamentoMensalidade(ctx, academia)
}

func (s *Service) mensalidadeTemCobrancaAberta(ctx context.Context, estudante, academia, ano string, mes int) (bool, error) {
	var exists bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM financeiro_mensalidade_cobrancas m JOIN financeiro_cobrancas c ON c.id=m.charge_id
		WHERE m.codigo_estudante=$1 AND m.codigo_academia=$2 AND m.ano_letivo=$3 AND m.mes=$4
		AND lower(COALESCE(c.payload->>'status','')) NOT IN ('success','cancelada','falhada')
	)`, estudante, academia, ano, mes).Scan(&exists)
	return exists, err
}

func (s *Service) cancelOpenMensalidadeCharges(ctx context.Context, estudante, academia, ano string, mes int, actorID, actorType, ip string) error {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT c.id::text FROM financeiro_mensalidade_cobrancas m JOIN financeiro_cobrancas c ON c.id=m.charge_id
		WHERE m.codigo_estudante=$1 AND m.codigo_academia=$2 AND m.ano_letivo=$3 AND m.mes=$4
		AND lower(COALESCE(c.payload->>'status','')) NOT IN ('success','cancelada','falhada')`, estudante, academia, ano, mes)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		_, _ = s.CancelCharge(ctx, ContextoAcademia, academia, id, "obrigação anulada pela academia", actorID, actorType, ip)
	}
	return rows.Err()
}

// confirmMensalidadeCharge stores one ledger event for the whole charge. The
// projector expands it transactionally into the monthly payment facts, so a
// multi-month confirmation is never visible partially.
func (s *Service) confirmMensalidadeCharge(ctx context.Context, chargeID uuid.UUID, actorID, actorType, ip string) error {
	var exists bool
	if err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_mensalidade_obrigacoes_eventos WHERE charge_id=$1)`, chargeID).Scan(&exists); err != nil || exists {
		return err
	}
	row, err := s.loadCharge(ctx, chargeID.String())
	if err != nil || !isSuccessfulChargeStatus(row.Status) {
		return err
	}
	months := mensalidadesDoPayload(row.Payload)
	if len(months) == 0 {
		return nil
	}
	estudante, _ := row.Payload["codigo_estudante"].(string)
	// Student identity is stored in the structured selection payload, never
	// inferred from an untrusted provider response.
	if info, ok := row.Payload["payment_info"].(map[string]any); ok {
		estudante, _ = info["codigo_estudante"].(string)
	}
	if estudante == "" {
		return errors.New("cobrança de mensalidade sem estudante")
	}
	payload := map[string]any{"charge_id": chargeID.String(), "codigo_estudante": estudante, "codigo_academia": row.Academia, "meses": months}
	return s.record(ctx, chargeID, "MensalidadesCobrancaConfirmada", payload, actorID, actorType, ip)
}

func mensalidadesDoPayload(payload map[string]any) []MensalidadeSelecaoMes {
	var raw any = payload["mensalidades"]
	if raw == nil {
		if info, ok := payload["payment_info"].(map[string]any); ok {
			raw = info["mensalidades"]
		}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var months []MensalidadeSelecaoMes
	if json.Unmarshal(b, &months) != nil {
		return nil
	}
	return months
}
func jsonUnmarshal(raw []byte, target any) error { return json.Unmarshal(raw, target) }
