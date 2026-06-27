package handlers

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"spuri/internal/db"
)

type anoLetivoPartes struct{ Inicio, Fim int }

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

func periodoConfigurado(client *db.Client, tipo string) (string, error) {
	var p sql.NullString
	err := client.DB().QueryRow(`SELECT periodo FROM projection_anos_letivos_configuracoes WHERE type = $1`, tipo).Scan(&p)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("período letivo %s não configurado", tipo)
	}
	if err != nil {
		return "", err
	}
	return p.String, nil
}

func mesPermiteFinalizacaoAnoLetivo(mesAtual, mesFim, mesInicio int) bool {
	return mesAtual >= mesFim && mesAtual < mesInicio
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
