package utils

import "fmt"

func ValidateNIF(nif string) error {
	if len(nif) != 10 {
		return fmt.Errorf("nif deve ser string com exatamente 10 dígitos")
	}
	for _, r := range nif {
		if r < '0' || r > '9' {
			return fmt.Errorf("nif deve conter somente dígitos")
		}
	}
	return nil
}
