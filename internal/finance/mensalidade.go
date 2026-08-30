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
	"github.com/lib/pq"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)

const (
	NivelFundamental = "fundamental"
	NivelMedio       = "medio"
	NivelSuperior    = "superior"

	EstadoPendente = "pendente"
	EstadoPago     = "pago"
	EstadoAnulado  = "anulado"

	// EstadoCobrancaAguardandoPagamento é o estado canônico de uma cobrança
	// REAL (financeiro_cobrancas) que já foi gerada/tentada junto à AppyPay
	// mas ainda não foi resolvida — ver normalizeChargeStatus em appypay.go
	// para a tradução completa (cobre os estados locais intermediários
	// "solicitada"/"criada" e os estados brutos "Requested"/"Pending" que a
	// própria AppyPay devolve nesta fase, ambos documentados em
	// docs/Parceiros e integrações/AppyPay Documentação.md).
	//
	// Deliberadamente DISTINTO de EstadoPendente ("pendente"): EstadoPendente
	// é reservado exclusivamente para uma OBRIGAÇÃO de mensalidade (ou uma
	// pendência sintética em PagamentoResumo, ver pagamentos_unificado.go)
	// que NUNCA teve nenhuma cobrança gerada nem tentada — uma cobrança real
	// nunca usa "pendente" como seu status; assim que qualquer cobrança é
	// gerada (mesmo antes de qualquer resposta do provedor), o status passa
	// a ser EstadoCobrancaAguardandoPagamento até resolver para um estado
	// terminal (pago, falhado, cancelado ou expirado). Essa separação é o
	// que permite ao status, sozinho, dizer se existe ou não uma cobrança
	// real por trás de um item da lista unificada de pagamentos — sem
	// precisar de nenhum campo booleano adicional.
	EstadoCobrancaAguardandoPagamento = "aguardando_pagamento"
)

type MensalidadeConfiguracaoInput struct {
	CodigoAcademia   string   `json:"codigo_academia"`
	Nivel            string   `json:"nivel"`
	AnoAcademico     string   `json:"ano_academico"`
	CursoID          *string  `json:"curso_id,omitempty"`
	Valor            float64  `json:"valor"`
	MesFimCobranca   int      `json:"mes_fim_cobranca"`
	MetodosPagamento []string `json:"metodos_pagamento"`
	ModoVigencia     string   `json:"modo_vigencia"`
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
	ModoVigencia     string     `json:"modo_vigencia,omitempty"`
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
	// Charge é QRCodeResult (não ChargeResult) para que o campo QRCodeArr (o
	// conteúdo do QR Code) chegue nesta resposta quando
	// metodo_pagamento = "GPO_QR". Para qualquer outro método, QRCodeArr
	// fica vazio e é omitido do JSON (omitempty). Ver Problema 2 em
	// docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md.
	Charge QRCodeResult            `json:"cobranca"`
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
	payload := map[string]any{"codigo_academia": in.CodigoAcademia, "nivel": in.Nivel, "ano_academico": in.AnoAcademico, "curso_id": optionalString(in.CursoID), "valor": in.Valor, "mes_fim_cobranca": in.MesFimCobranca, "metodos_pagamento": in.MetodosPagamento, "modo_vigencia": in.ModoVigencia}
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
	rows, err := s.client.DB().QueryContext(ctx, `SELECT nivel,ano_academico,curso_id,valor::float8,mes_fim_cobranca,metodos_pagamento,vigente_em,modo_vigencia FROM financeiro_mensalidade_configuracoes_atual WHERE codigo_academia=$1 ORDER BY nivel,ano_academico,curso_id`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MensalidadeConfiguracaoView
	for rows.Next() {
		var v MensalidadeConfiguracaoView
		var curso sql.NullString
		v.CodigoAcademia = codigoAcademia
		if err := rows.Scan(&v.Nivel, &v.AnoAcademico, &curso, &v.Valor, &v.MesFimCobranca, pq.Array(&v.MetodosPagamento), &v.VigenteEm, &v.ModoVigencia); err != nil {
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

// RemoveMensalidadeConfiguracao remove a configuração de mensalidade
// (preço + métodos de pagamento) atualmente vigente para um escopo
// (academia+nível+ano acadêmico+curso). É registrada como um novo evento
// imutável (MensalidadeConfiguracaoRemovida) — nenhuma linha de
// financeiro_mensalidade_configuracoes é apagada ou reescrita, então o
// preço histórico de meses já cobrados antes da remoção continua
// resolvendo exatamente igual (ver resolveConfiguracao). A partir deste
// comando, o escopo passa a não ter configuração ativa: novas tentativas
// de pagamento para meses de referência a partir de agora falham, e
// metodosPagamentoMensalidade/ListMensalidadeConfiguracoes deixam de
// listar este escopo.
func (s *Service) RemoveMensalidadeConfiguracao(ctx context.Context, codigoAcademia, nivel, anoAcademico string, cursoID *uuid.UUID, actorID, actorType, ip string) error {
	if s.client == nil {
		return errors.New("serviço financeiro não inicializado")
	}
	codigoAcademia, nivel, anoAcademico = strings.TrimSpace(codigoAcademia), strings.TrimSpace(nivel), strings.TrimSpace(anoAcademico)
	if codigoAcademia == "" || nivel == "" || anoAcademico == "" {
		return errors.New("academia, nível e ano acadêmico são obrigatórios")
	}
	if _, err := s.resolveConfiguracao(ctx, codigoAcademia, nivel, anoAcademico, cursoID, time.Now().UTC()); err != nil {
		return fmt.Errorf("%w: nenhuma configuração de mensalidade ativa para este escopo", ErrNotFound)
	}
	payload := map[string]any{"codigo_academia": codigoAcademia, "nivel": nivel, "ano_academico": anoAcademico, "curso_id": optionalUUID(cursoID)}
	return s.recordMensalidade(ctx, codigoAcademia, aggregates.MensalidadeConfiguracaoRemovida, payload, actorID, actorType, ip)
}

// RemoveMesInicioCobranca remove a redefinição de mês de início de
// cobrança para um ano letivo, fazendo o sistema voltar a usar o mês
// natural padrão (início do ano letivo) como se DefinirMesInicioCobranca
// nunca tivesse sido chamado para este ano letivo. O evento
// MesInicioCobrancaDefinido original permanece intacto no ledger.
func (s *Service) RemoveMesInicioCobranca(ctx context.Context, codigoAcademia, anoLetivo, actorID, actorType, ip string) error {
	if s.client == nil {
		return errors.New("serviço financeiro não inicializado")
	}
	codigoAcademia, anoLetivo = strings.TrimSpace(codigoAcademia), strings.TrimSpace(anoLetivo)
	if codigoAcademia == "" || !anoLetivoValido(anoLetivo) {
		return errors.New("academia e ano_letivo válido são obrigatórios")
	}
	var existe bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_mensalidade_inicio_cobranca_atual WHERE codigo_academia=$1 AND ano_letivo=$2)`, codigoAcademia, anoLetivo).Scan(&existe)
	if err != nil {
		return err
	}
	if !existe {
		return fmt.Errorf("%w: nenhum mês de início de cobrança definido para este ano letivo", ErrNotFound)
	}
	payload := map[string]any{"codigo_academia": codigoAcademia, "ano_letivo": anoLetivo}
	return s.recordMensalidade(ctx, codigoAcademia, aggregates.MesInicioCobrancaRemovido, payload, actorID, actorType, ip)
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
	description := fmt.Sprintf("Propinas %s: %d mensalidade(s)", in.CodigoAcademia, len(in.Meses))
	result, err := s.gerarCobranca(ctx, gerarCobrancaInput{
		CodigoAcademia:        in.CodigoAcademia,
		MetodoPagamento:       in.MetodoPagamento,
		Amount:                total,
		Description:           description,
		MerchantTransactionID: merchantID(),
		Telefone:              in.Telefone,
		CodigoEstudante:       in.CodigoEstudante,
		Mensalidades:          in.Meses,
	}, actorID, actorType, ip)
	if err != nil {
		return MensalidadePagamentoView{}, err
	}
	return MensalidadePagamentoView{Charge: result, Meses: in.Meses}, nil
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
		natural := mesNaturalInicioAnoLetivo(v.Nivel)
		inicioPos := posicaoNoAnoLetivo(inicio, natural)
		for _, ref := range mesesAnoLetivo(v.AnoLetivo, v.Nivel) {
			if posicaoNoAnoLetivo(ref.Month, natural) < inicioPos {
				continue
			}
			state, audit, err := s.estadoObrigacao(ctx, codigoEstudante, v.CodigoAcademia, v.AnoLetivo, ref.Month)
			if err != nil {
				return nil, err
			}
			cfg, err := s.resolveConfiguracaoEfetiva(ctx, v.CodigoAcademia, v.Nivel, v.AnoAcademico, v.CursoID, ref.Data, state == EstadoPendente)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if posicaoNoAnoLetivo(ref.Month, natural) > posicaoNoAnoLetivo(cfg.MesFimCobranca, natural) {
				continue
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
		return result[i].DataReferencia.Before(result[j].DataReferencia)
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
	if !modoVigenciaValido(in.ModoVigencia) {
		return errors.New(`modo_vigencia é obrigatório: informe "cobrancas_pendentes" ou "a_partir_da_atualizacao"`)
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
			return errors.New("curso_id não é permitido para o " + utils.RotuloEnsinoFundamentalGenerico)
		}
		var anos []string
		if err := jsonUnmarshal(anosRaw, &anos); err != nil || !contains(anos, in.AnoAcademico) {
			return errors.New("ano acadêmico do " + utils.RotuloEnsinoFundamentalGenerico + " não é oferecido pela academia")
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
	natural := mesNaturalInicioAnoLetivo(nivel)
	var menor sql.NullInt64
	err = s.client.DB().QueryRowContext(ctx, `SELECT MIN(mes_fim_cobranca) FROM financeiro_mensalidade_configuracoes_atual WHERE codigo_academia=$1`, in.CodigoAcademia).Scan(&menor)
	if err != nil {
		return err
	}
	if menor.Valid && posicaoNoAnoLetivo(in.MesInicio, natural) > posicaoNoAnoLetivo(int(menor.Int64), natural) {
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
	err := s.client.DB().QueryRowContext(ctx, `SELECT curso_id,valor::float8,mes_fim_cobranca,vigente_em FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4 ORDER BY CASE WHEN vigente_em <= $5 THEN 0 ELSE 1 END, CASE WHEN vigente_em <= $5 THEN vigente_em END DESC, CASE WHEN vigente_em > $5 THEN vigente_em END ASC, sequencia DESC LIMIT 1`, academia, nivel, ano, nullableUUID(curso), referencia.UTC()).Scan(&cursoText, &out.Valor, &out.MesFimCobranca, &out.VigenteEm)
	if err == sql.ErrNoRows {
		return out, fmt.Errorf("%w: configuração de mensalidade", ErrNotFound)
	}
	if err != nil {
		return out, err
	}
	// A configuração resolvida acima é a versão que estava (ou passará a
	// estar) vigente. Se ela já foi removida — isto é, existe um evento
	// MensalidadeConfiguracaoRemovida cujo removido_em cai entre o
	// vigente_em dessa versão e a referência que estamos resolvendo — o
	// escopo está sem configuração ativa NAQUELA data, mesmo que uma
	// versão mais antiga já tenha existido um dia. Isso nunca reescreve
	// preços já resolvidos para referências ANTERIORES à remoção: o
	// filtro removido_em <= referencia garante que meses já cobrados
	// antes da remoção continuam resolvendo normalmente.
	if !out.VigenteEm.After(referencia.UTC()) {
		var removidoEm time.Time
		errRem := s.client.DB().QueryRowContext(ctx, `SELECT removido_em FROM financeiro_mensalidade_configuracoes_remocoes WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4 AND removido_em >= $5 AND removido_em <= $6 ORDER BY removido_em DESC LIMIT 1`, academia, nivel, ano, nullableUUID(curso), out.VigenteEm, referencia.UTC()).Scan(&removidoEm)
		if errRem == nil {
			return MensalidadeConfiguracaoView{}, fmt.Errorf("%w: configuração de mensalidade removida", ErrNotFound)
		}
		if errRem != sql.ErrNoRows {
			return out, errRem
		}
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
	natural := mesNaturalInicioAnoLetivo(nivel)
	var mes int
	err := s.client.DB().QueryRowContext(ctx, `SELECT mes_inicio FROM financeiro_mensalidade_inicio_cobranca_atual WHERE codigo_academia=$1 AND ano_letivo=$2`, academia, anoLetivo).Scan(&mes)
	if err == sql.ErrNoRows {
		return natural, nil
	}
	if err != nil {
		return 0, err
	}
	if !mesValido(mes) {
		return 0, errors.New("configuração de mes_inicio inconsistente")
	}
	return mes, nil
}

// mesNaturalInicioAnoLetivo devolve o mês de calendário em que o ano letivo
// começa para o nível informado. Fundamental e médio começam em setembro;
// superior começa em outubro.
func mesNaturalInicioAnoLetivo(nivel string) int {
	if nivel == NivelSuperior || nivel == "superior" {
		return 10
	}
	return 9
}

// posicaoNoAnoLetivo devolve a posição ordinal do mês dentro do ano letivo
// (1 = primeiro mês cobrável, crescente), para que meses de calendário de
// anos civis diferentes dentro do mesmo ano letivo sejam comparáveis.
// natural é o mês de calendário em que o ano letivo começa (9 para os
// níveis fundamental/médio, 10 para superior).
func posicaoNoAnoLetivo(mes, natural int) int {
	if mes >= natural {
		return mes - natural + 1
	}
	return mes + (12 - natural) + 1
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
	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT unnest(metodos_pagamento) FROM financeiro_mensalidade_configuracoes_atual WHERE codigo_academia=$1`, academia)
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

// chargeAbertaStatusExcluidos é a lista (em minúsculas) de todo status
// TERMINAL que uma cobrança real pode ter — usada para excluir cobranças
// "em aberto" nas consultas SQL diretas abaixo e em matriculaTemCobrancaAberta/
// CancelarCobrancaMatriculaAberta (matricula.go). Precisa ficar em sincronia
// manual com isTerminalChargeStatus (appypay.go): as duas listam exatamente
// os mesmos estados terminais, mas isTerminalChargeStatus não pode ser
// chamada aqui porque estas são consultas SQL, não Go, sobre linhas que
// ainda não foram carregadas em memória. Cobre tanto os estados locais
// ("cancelada", "falhada") quanto os quatro estados terminais que a própria
// AppyPay documenta e devolve verbatim (Success, Failed, Cancelled, Expired
// — ver docs/Parceiros e integrações/AppyPay Documentação.md): antes desta
// correção, uma cobrança com status bruto "Failed"/"Cancelled"/"Expired" da
// AppyPay nunca entrava nesta lista e por isso ficava "presa" como em
// aberto para sempre — bloqueando indefinidamente uma nova tentativa de
// pagamento do mesmo mês/matrícula mesmo depois de a cobrança anterior já
// ter definitivamente falhado no provedor.
const chargeAbertaStatusExcluidos = `'success','cancelada','falhada','failed','cancelled','expired'`

func (s *Service) mensalidadeTemCobrancaAberta(ctx context.Context, estudante, academia, ano string, mes int) (bool, error) {
	var exists bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM financeiro_mensalidade_cobrancas m JOIN financeiro_cobrancas c ON c.id=m.charge_id
		WHERE m.codigo_estudante=$1 AND m.codigo_academia=$2 AND m.ano_letivo=$3 AND m.mes=$4
		AND lower(COALESCE(c.payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)
	)`, estudante, academia, ano, mes).Scan(&exists)
	return exists, err
}

func (s *Service) cancelOpenMensalidadeCharges(ctx context.Context, estudante, academia, ano string, mes int, actorID, actorType, ip string) error {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT c.id::text FROM financeiro_mensalidade_cobrancas m JOIN financeiro_cobrancas c ON c.id=m.charge_id
		WHERE m.codigo_estudante=$1 AND m.codigo_academia=$2 AND m.ano_letivo=$3 AND m.mes=$4
		AND lower(COALESCE(c.payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)`, estudante, academia, ano, mes)
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
	// Student identity is stored in the structured selection payload (set by
	// CreateCharge/CreateGPOQRCode from the validated ChargeRequest), never
	// inferred from payment_info, which only carries provider-specific
	// parameters (e.g. GPO's phoneNumber) and never a student code.
	estudante, _ := row.Payload["codigo_estudante"].(string)
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
