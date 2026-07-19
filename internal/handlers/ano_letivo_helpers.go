package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"spuri/internal/db"
)

type anoLetivoPartes struct{ Inicio, Fim int }

const (
	periodoLetivoEscolar  = "09_07"
	periodoLetivoSuperior = "10_07"
)

func periodoFixoPorTipoAnoLetivo(tipo string) (string, error) {
	t, err := normalizarTipoAnoLetivo(tipo)
	if err != nil {
		return "", err
	}
	switch t {
	case "escolar":
		return periodoLetivoEscolar, nil
	case "superior":
		return periodoLetivoSuperior, nil
	default:
		return "", fmt.Errorf("type deve ser 'escolar' ou 'superior'")
	}
}

func normalizarTipoAnoLetivo(tipo string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(tipo))
	if t != "escolar" && t != "superior" {
		return "", fmt.Errorf("type deve ser 'escolar' ou 'superior'")
	}
	return t, nil
}

func parseAnoLetivo(v string) (anoLetivoPartes, error) {
	v = strings.TrimSpace(v)
	var p anoLetivoPartes
	if len(v) != 9 || v[4] != '_' {
		return p, fmt.Errorf("ano_letivo deve usar formato YYYY_YYYY")
	}
	if _, err := fmt.Sscanf(v, "%4d_%4d", &p.Inicio, &p.Fim); err != nil || p.Fim != p.Inicio+1 {
		return p, fmt.Errorf("ano_letivo deve ter segundo ano igual ao primeiro + 1")
	}
	return p, nil
}

func proximoAnoLetivoValidado(v string) (string, error) {
	p, err := parseAnoLetivo(v)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d_%04d", p.Fim, p.Fim+1), nil
}
func compareAnoLetivo(a, b string) (int, error) {
	pa, err := parseAnoLetivo(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseAnoLetivo(b)
	if err != nil {
		return 0, err
	}
	if pa.Inicio < pb.Inicio {
		return -1, nil
	}
	if pa.Inicio > pb.Inicio {
		return 1, nil
	}
	return 0, nil
}

func validarPeriodoLetivoFixoPayload(tipo, periodo string) (string, error) {
	periodoRecebido, err := normalizarPeriodoLetivo(periodo)
	if err != nil {
		return "", err
	}
	periodoFixo, err := periodoFixoPorTipoAnoLetivo(tipo)
	if err != nil {
		return "", err
	}
	if periodoRecebido != periodoFixo {
		return "", fmt.Errorf("periodo de ano letivo é fixo e imutável para type=%s: use %s", tipo, periodoFixo)
	}
	return periodoFixo, nil
}

func normalizarPeriodoLetivo(periodo string) (string, error) {
	periodo = strings.TrimSpace(periodo)
	partes := strings.Split(periodo, "_")
	if len(partes) != 2 || strings.TrimSpace(partes[0]) == "" || strings.TrimSpace(partes[1]) == "" {
		return "", fmt.Errorf("periodo deve usar formato MM_MM com meses entre 01 e 12")
	}
	ini, errIni := strconv.Atoi(partes[0])
	fim, errFim := strconv.Atoi(partes[1])
	if errIni != nil || errFim != nil || ini < 1 || ini > 12 || fim < 1 || fim > 12 {
		return "", fmt.Errorf("periodo deve usar formato MM_MM com meses entre 01 e 12")
	}
	return fmt.Sprintf("%02d_%02d", ini, fim), nil
}

func mesesPeriodoLetivo(periodo string) (int, int, error) {
	var mi, mf int
	if _, err := fmt.Sscanf(periodo, "%d_%d", &mi, &mf); err != nil || mi < 1 || mi > 12 || mf < 1 || mf > 12 {
		return 0, 0, fmt.Errorf("periodo inválido")
	}
	return mi, mf, nil
}

func intervaloAnoLetivo(anoLetivo, periodo string) (time.Time, time.Time, error) {
	a, err := parseAnoLetivo(anoLetivo)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	mi, mf, err := mesesPeriodoLetivo(periodo)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	inicio := time.Date(a.Inicio, time.Month(mi), 1, 0, 0, 0, 0, time.UTC)
	fim := time.Date(a.Fim, time.Month(mf)+1, 0, 23, 59, 59, 0, time.UTC)
	return inicio, fim, nil
}

func periodoConfigurado(_ *db.Client, tipo string) (string, error) {
	return periodoFixoPorTipoAnoLetivo(tipo)
}

func mesPermiteFinalizacaoAnoLetivo(mesAtual, mesFim, mesInicio int) bool {
	return mesAtual >= mesFim && mesAtual < mesInicio
}

func anoFinalAnoLetivo(anoLetivo string) (int, error) {
	partes, err := parseAnoLetivo(anoLetivo)
	if err != nil {
		return 0, err
	}
	return partes.Fim, nil
}

func mesesPermitidosFinalizacaoAnoLetivo(mesFim, mesInicio int) []time.Month {
	meses := []time.Month{}
	for mes := mesFim; mes < mesInicio; mes++ {
		meses = append(meses, time.Month(mes))
	}
	return meses
}

func nomesMesesPortugues(meses []time.Month) string {
	nomes := map[time.Month]string{
		time.January: "janeiro", time.February: "fevereiro", time.March: "março",
		time.April: "abril", time.May: "maio", time.June: "junho",
		time.July: "julho", time.August: "agosto", time.September: "setembro",
		time.October: "outubro", time.November: "novembro", time.December: "dezembro",
	}
	partes := make([]string, 0, len(meses))
	for _, mes := range meses {
		partes = append(partes, nomes[mes])
	}
	if len(partes) <= 1 {
		return strings.Join(partes, "")
	}
	return strings.Join(partes[:len(partes)-1], ", ") + " e " + partes[len(partes)-1]
}

func validarDataAtualPermiteFinalizacaoAnoLetivo(client *db.Client, tipo, anoLetivo string, agora time.Time) error {
	anoFinal, err := anoFinalAnoLetivo(anoLetivo)
	if err != nil {
		return err
	}
	agoraUTC := agora.UTC()
	anoAtual := agoraUTC.Year()
	if anoAtual != anoFinal {
		return fmt.Errorf("não é possível finalizar o ano letivo %s: o ano atual (%d) não é o ano final do período letivo (%d)", anoLetivo, anoAtual, anoFinal)
	}

	periodo, err := periodoConfigurado(client, tipo)
	if err != nil {
		return err
	}
	mesInicio, mesFim, err := mesesPeriodoLetivo(periodo)
	if err != nil {
		return err
	}
	mesAtual := int(agoraUTC.Month())
	if mesPermiteFinalizacaoAnoLetivo(mesAtual, mesFim, mesInicio) {
		return nil
	}
	mesesPermitidos := mesesPermitidosFinalizacaoAnoLetivo(mesFim, mesInicio)
	return fmt.Errorf(
		"não é possível finalizar o ano letivo %s: fora da janela mensal de finalização; permitido apenas em %s de %d (periodo=%s; mês atual=%02d)",
		anoLetivo, nomesMesesPortugues(mesesPermitidos), anoFinal, periodo, mesAtual,
	)
}

func validarMesAtualPermiteFinalizacaoAnoLetivo(client *db.Client, tipo string, agora time.Time) error {
	periodo, err := periodoConfigurado(client, tipo)
	if err != nil {
		return err
	}
	mesInicio, mesFim, err := mesesPeriodoLetivo(periodo)
	if err != nil {
		return err
	}
	mesAtual := int(agora.UTC().Month())
	if mesPermiteFinalizacaoAnoLetivo(mesAtual, mesFim, mesInicio) {
		return nil
	}
	return fmt.Errorf(
		"ano letivo %s só pode ser finalizado entre o mês de fim do período letivo e o mês anterior ao início do próximo período (periodo=%s; mês atual=%02d; permitido: mês >= %02d e < %02d)",
		tipo, periodo, mesAtual, mesFim, mesInicio,
	)
}

func validarDataNoPeriodoLetivo(client *db.Client, tipo, anoLetivo string, data time.Time) error {
	periodo, err := periodoConfigurado(client, tipo)
	if err != nil {
		return err
	}
	ini, fim, err := intervaloAnoLetivo(anoLetivo, periodo)
	if err != nil {
		return err
	}
	d := data.UTC()
	if d.Before(ini) || d.After(fim) {
		return fmt.Errorf("data da falta fora do período letivo %s %s: permitido de %s até %s", tipo, anoLetivo, ini.Format("2006-01-02"), fim.Format("2006-01-02"))
	}
	return nil
}

func inferirTipoLetivoMateria(materiaType string) (string, error) {
	if strings.TrimSpace(strings.ToLower(materiaType)) == "superior" {
		return "superior", nil
	}
	return "escolar", nil
}
