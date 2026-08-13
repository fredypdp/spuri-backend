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
	CodigoAcademia string  `json:"codigo_academia"`
	Nivel          string  `json:"nivel"`
	AnoAcademico   string  `json:"ano_academico"`
	CursoID        *string `json:"curso_id,omitempty"`
	Valor          float64 `json:"valor"`
	MesFimCobranca int     `json:"mes_fim_cobranca"`
}

type MensalidadeConfiguracaoView struct {
	CodigoAcademia string     `json:"codigo_academia"`
	Nivel          string     `json:"nivel"`
	AnoAcademico   string     `json:"ano_academico"`
	CursoID        *uuid.UUID `json:"curso_id,omitempty"`
	Valor          float64    `json:"valor"`
	MesFimCobranca int        `json:"mes_fim_cobranca"`
	VigenteEm      time.Time  `json:"vigente_em"`
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
		return MensalidadeConfiguracaoView{}, errors.New("serviÃ§o financeiro nÃ£o inicializado")
	}
	if err := s.validateConfiguracaoMensalidade(ctx, &in); err != nil {
		return MensalidadeConfiguracaoView{}, err
	}
	in.Valor = roundAmount(in.Valor)
	payload := map[string]any{"codigo_academia": in.CodigoAcademia, "nivel": in.Nivel, "ano_academico": in.AnoAcademico, "curso_id": optionalString(in.CursoID), "valor": in.Valor, "mes_fim_cobranca": in.MesFimCobranca}
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
	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT ON (nivel,ano_academico,curso_id) nivel,ano_academico,curso_id,valor::float8,mes_fim_cobranca,vigente_em FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1 ORDER BY nivel,ano_academico,curso_id,vigente_em DESC,event_id DESC`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MensalidadeConfiguracaoView
	for rows.Next() {
		var v MensalidadeConfiguracaoView
		var curso sql.NullString
		v.CodigoAcademia = codigoAcademia
		if err := rows.Scan(&v.Nivel, &v.AnoAcademico, &curso, &v.Valor, &v.MesFimCobranca, &v.VigenteEm); err != nil {
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
		return errors.New("estudante, academia, ano_letivo vÃ¡lido e meses sÃ£o obrigatÃ³rios")
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
			return errors.New("nÃ£o Ã© possÃ­vel anular uma mensalidade jÃ¡ paga")
		}
		if eventType == aggregates.ObrigacaoMensalidadeReativada && state != EstadoAnulado {
			return errors.New("sÃ³ Ã© possÃ­vel reativar uma mensalidade anulada e nÃ£o paga")
		}
	}
	for _, mes := range in.Meses {
		if err := s.recordMensalidade(ctx, in.CodigoAcademia, eventType, map[string]any{"codigo_estudante": in.CodigoEstudante, "codigo_academia": in.CodigoAcademia, "ano_letivo": in.AnoLetivo, "mes": mes, "motivo": strings.TrimSpace(in.Motivo)}, actorID, actorType, ip); err != nil {
			return err
		}
	}
	return nil
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
		return MensalidadeMesView{}, errors.New("mÃªs ou ano_letivo invÃ¡lido")
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
	return MensalidadeMesView{}, errors.New("mÃªs fora do perÃ­odo de mensalidade configurado")
}

func (s *Service) validateConfiguracaoMensalidade(ctx context.Context, in *MensalidadeConfiguracaoInput) error {
	in.CodigoAcademia, in.Nivel, in.AnoAcademico = strings.TrimSpace(in.CodigoAcademia), strings.ToLower(strings.TrimSpace(in.Nivel)), strings.TrimSpace(in.AnoAcademico)
	if in.CodigoAcademia == "" || !nivelValido(in.Nivel) || in.AnoAcademico == "" {
		return errors.New("codigo_academia, nivel e ano_academico sÃ£o obrigatÃ³rios")
	}
	if in.Valor <= 0 || roundAmount(in.Valor) != in.Valor {
		return errors.New("valor deve ser maior que zero e ter no mÃ¡ximo duas casas decimais")
	}
	if in.MesFimCobranca != 6 && in.MesFimCobranca != 7 {
		return errors.New("mes_fim_cobranca deve ser 6 ou 7")
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
		return errors.New("mensalidade sÃ³ pode ser configurada por academia privada")
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
			return errors.New("curso_id nÃ£o Ã© permitido para ensino fundamental")
		}
		var anos []string
		if err := jsonUnmarshal(anosRaw, &anos); err != nil || !contains(anos, in.AnoAcademico) {
			return errors.New("ano_academico fundamental nÃ£o Ã© oferecido pela academia")
		}
		return nil
	}
	if in.CursoID == nil || strings.TrimSpace(*in.CursoID) == "" {
		return errors.New("curso_id Ã© obrigatÃ³rio para ensino mÃ©dio e superior")
	}
	cursoID, err := uuid.Parse(*in.CursoID)
	if err != nil {
		return errors.New("curso_id invÃ¡lido")
	}
	var cursoTipo, codigoCurso string
	var anosRawCurso []byte
	err = s.client.DB().QueryRowContext(ctx, `SELECT type,codigo_academia,anos_academicos FROM projection_cursos WHERE id=$1 AND deleted_at IS NULL`, cursoID).Scan(&cursoTipo, &codigoCurso, &anosRawCurso)
	if err == sql.ErrNoRows {
		return errors.New("curso nÃ£o encontrado")
	}
	if err != nil {
		return err
	}
	var anos []string
	if codigoCurso != in.CodigoAcademia || cursoTipo != in.Nivel || jsonUnmarshal(anosRawCurso, &anos) != nil || !contains(anos, in.AnoAcademico) {
		return errors.New("curso ou ano_academico nÃ£o Ã© oferecido pela academia")
	}
	return nil
}

func (s *Service) validateMesInicioCobranca(ctx context.Context, in *MesInicioCobrancaInput) error {
	in.CodigoAcademia, in.AnoLetivo = strings.TrimSpace(in.CodigoAcademia), strings.TrimSpace(in.AnoLetivo)
	if in.CodigoAcademia == "" || !anoLetivoValido(in.AnoLetivo) || !mesValido(in.MesInicio) {
		return errors.New("codigo_academia, ano_letivo vÃ¡lido e mes_inicio sÃ£o obrigatÃ³rios")
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
		return errors.New("mensalidade sÃ³ pode ser configurada por academia privada")
	}
	natural := 9
	if nivel == "superior" {
		natural = 10
	}
	if in.MesInicio < natural {
		return fmt.Errorf("mes_inicio nÃ£o pode ser anterior a %02d", natural)
	}
	var menor sql.NullInt64
	err = s.client.DB().QueryRowContext(ctx, `SELECT MIN(mes_fim_cobranca) FROM (SELECT DISTINCT ON (nivel,ano_academico,curso_id) mes_fim_cobranca FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1 ORDER BY nivel,ano_academico,curso_id,vigente_em DESC,event_id DESC) c`, in.CodigoAcademia).Scan(&menor)
	if err != nil {
		return err
	}
	if menor.Valid && in.MesInicio > int(menor.Int64) {
		return errors.New("mes_inicio nÃ£o pode ser posterior ao mes_fim_cobranca configurado")
	}
	return nil
}

func (s *Service) recordMensalidade(ctx context.Context, codigoAcademia, event string, payload map[string]any, userID, userType, ip string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("autor do evento financeiro Ã© obrigatÃ³rio")
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
	err := s.client.DB().QueryRowContext(ctx, `SELECT curso_id,valor::float8,mes_fim_cobranca,vigente_em FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4 AND vigente_em <= $5 ORDER BY vigente_em DESC,event_id DESC LIMIT 1`, academia, nivel, ano, nullableUUID(curso), referencia.UTC()).Scan(&cursoText, &out.Valor, &out.MesFimCobranca, &out.VigenteEm)
	if err == sql.ErrNoRows {
		return out, fmt.Errorf("%w: configuraÃ§Ã£o de mensalidade", ErrNotFound)
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
		return nil, errors.New("codigo_estudante Ã© obrigatÃ³rio")
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
		return 0, errors.New("configuraÃ§Ã£o de mes_inicio inconsistente")
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
func jsonUnmarshal(raw []byte, target any) error { return json.Unmarshal(raw, target) }
