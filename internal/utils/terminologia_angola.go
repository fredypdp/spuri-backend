package utils

import "strings"

// RotuloEnsinoFundamentalGenerico é o rótulo a usar sempre que uma mensagem
// citar o "Ensino Fundamental" de forma genérica, sem apontar um ano
// específico (Regra 1).
const RotuloEnsinoFundamentalGenerico = "Ensino Primário e Iº Ciclo"

// RotuloEnsinoMisto é o rótulo a usar em referências que abrangem, em
// conjunto, o ensino fundamental e o ensino médio — contexto de
// academias/escolas mistas (Regra 3).
const RotuloEnsinoMisto = "Ensino Primário ao Médio"

// RotuloClasseFundamental converte o código interno de um ano do ensino
// fundamental (formato "N_ano_fundamental") no rótulo de exibição usado em
// Angola: "Nª Classe" (Regra 2).
//
// O valor interno "N_ano_fundamental" NUNCA é alterado em lógica/validação/
// persistência — esta função serve apenas para gerar texto amigável em
// mensagens devolvidas ao cliente.
//
// Caso a entrada não esteja no formato esperado, a função devolve o valor
// original sem modificação (fallback seguro, nunca quebra uma mensagem).
func RotuloClasseFundamental(anoFundamental string) string {
	trimmed := strings.TrimSpace(anoFundamental)
	numero := strings.TrimSuffix(trimmed, "_ano_fundamental")
	if numero == "" || numero == trimmed {
		return trimmed
	}
	for _, r := range numero {
		if r < '0' || r > '9' {
			return trimmed
		}
	}
	return numero + "ª Classe"
}
