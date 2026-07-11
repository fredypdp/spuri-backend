package services

import (
	"fmt"
	"strings"
	"time"

	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)

type MatriculaValidationContext string

const (
	MatriculaContextSolicitacao    MatriculaValidationContext = "solicitacao_matricula"
	MatriculaContextCadastroDireto MatriculaValidationContext = "cadastro_direto_academia"
)

type MatriculaCommonInput struct {
	Contexto                     MatriculaValidationContext
	Nome                         string
	Genero                       string
	DataNascimento               time.Time
	Email                        *string
	TelefoneEstudante            *string
	TelefoneResponsavel          *string
	BilheteIdentidade            *string
	BilheteIdentidadeResponsavel *string
	AnoEscolarFundamental        *string
	AnoEscolarMedio              *string
	AnoSuperior                  *string
	Documentos                   map[string]aggregates.DocumentoMatricula
}

type MatriculaCommonValidated struct {
	Email                        *string
	TelefoneEstudante            *string
	TelefoneResponsavel          *string
	BilheteIdentidade            *string
	BilheteIdentidadeResponsavel *string
	AnoEscolarFundamental        *string
	AnoEscolarMedio              *string
	AnoSuperior                  *string
}

func ValidateMatriculaCommon(in MatriculaCommonInput) (MatriculaCommonValidated, error) {
	out := MatriculaCommonValidated{
		Email:                        trimPtr(in.Email),
		TelefoneEstudante:            normalizePhonePtr(in.TelefoneEstudante),
		TelefoneResponsavel:          normalizePhonePtr(in.TelefoneResponsavel),
		BilheteIdentidade:            trimPtr(in.BilheteIdentidade),
		BilheteIdentidadeResponsavel: trimPtr(in.BilheteIdentidadeResponsavel),
		AnoEscolarFundamental:        trimPtr(in.AnoEscolarFundamental),
		AnoEscolarMedio:              trimPtr(in.AnoEscolarMedio),
		AnoSuperior:                  trimPtr(in.AnoSuperior),
	}
	if err := utils.ValidateNome(strings.TrimSpace(in.Nome)); err != nil {
		return out, err
	}
	if in.Genero != "masculino" && in.Genero != "feminino" {
		return out, fmt.Errorf("genero deve ser 'masculino' ou 'feminino'")
	}
	hoje := time.Now().UTC().Truncate(24 * time.Hour)
	if !in.DataNascimento.UTC().Truncate(24 * time.Hour).Before(hoje) {
		return out, fmt.Errorf("data_nascimento deve ser anterior à data atual")
	}
	if out.Email != nil {
		if err := utils.ValidateEmail(*out.Email); err != nil {
			return out, err
		}
	}
	if out.AnoEscolarFundamental != nil {
		if err := utils.ValidateAnoFundamental(*out.AnoEscolarFundamental); err != nil {
			return out, fmt.Errorf("ano_escolar_fundamental inválido: %w", err)
		}
	}
	if out.AnoEscolarMedio != nil {
		if err := utils.ValidateAnoMedio(*out.AnoEscolarMedio); err != nil {
			return out, fmt.Errorf("ano_escolar_medio inválido: %w", err)
		}
	}
	if out.AnoSuperior != nil {
		if err := utils.ValidateAnoSuperior(*out.AnoSuperior); err != nil {
			return out, fmt.Errorf("ano_superior inválido: %w", err)
		}
	}
	if err := aggregates.ValidarTelefonesMatricula(out.TelefoneEstudante, out.TelefoneResponsavel, out.AnoEscolarFundamental, out.AnoEscolarMedio, out.AnoSuperior); err != nil {
		return out, err
	}
	if err := aggregates.ValidarBilhetesMatricula(out.BilheteIdentidade, out.BilheteIdentidadeResponsavel); err != nil {
		return out, err
	}
	if err := aggregates.ValidarDocumentosMatricula(out.BilheteIdentidade, out.BilheteIdentidadeResponsavel, out.AnoEscolarFundamental, out.AnoEscolarMedio, out.AnoSuperior, in.Documentos); err != nil {
		return out, err
	}
	return out, nil
}

func trimPtr(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}
func normalizePhonePtr(v *string) *string {
	v = trimPtr(v)
	if v == nil {
		return nil
	}
	return utils.NormalizePhonePtr(v)
}
