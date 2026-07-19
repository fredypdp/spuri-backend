package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReadAndValidateJSONArrayBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		maxItems   int
		wantCount  int
		wantErr    bool
		errMessage string
	}{
		{
			name:      "array válido com dois itens",
			body:      `[{"a":1},{"b":2}]`,
			maxItems:  5,
			wantCount: 2,
		},
		{
			name:       "body não é array",
			body:       `{"a":1}`,
			maxItems:   5,
			wantErr:    true,
			errMessage: "array JSON",
		},
		{
			name:       "ultrapassa limite",
			body:       `[1,2,3]`,
			maxItems:   2,
			wantErr:    true,
			errMessage: "máximo de 2 itens",
		},
		{
			name:       "lixo após array",
			body:       `[{"ok":true}] {"extra":true}`,
			maxItems:   5,
			wantErr:    true,
			errMessage: "apenas um array JSON",
		},
		{
			name:       "body vazio",
			body:       ``,
			maxItems:   5,
			wantErr:    true,
			errMessage: "array JSON",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodPost, "/academia/notas-aluno/async", strings.NewReader(tt.body))
			c.Request = req

			payload, gotCount, err := readAndValidateJSONArrayBody(c, tt.maxItems)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("esperava erro, mas não houve")
				}
				if tt.errMessage != "" && !strings.Contains(err.Error(), tt.errMessage) {
					t.Fatalf("erro esperado conter %q, obtido %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("não esperava erro, obtido: %v", err)
			}
			if gotCount != tt.wantCount {
				t.Fatalf("count esperado=%d obtido=%d", tt.wantCount, gotCount)
			}
			if strings.TrimSpace(string(payload)) != strings.TrimSpace(tt.body) {
				t.Fatalf("payload retornado não preserva body original")
			}
		})
	}
}

func TestCadastroEstudanteJSONItemToCadastroRequest(t *testing.T) {
	t.Parallel()

	item := cadastroEstudanteJSONItem{
		Nome:                   "  Ana Maria  ",
		Genero:                 "feminino",
		DataNascimento:         "2005-04-03",
		Email:                  " ana@example.com ",
		Telefone:               " 923 000 111 ",
		TelefoneEncarregado:    " 924 000 111 ",
		BilheteIdentidade:      " 001LA000000 ",
		BilheteEncarregado:     " 002LA000000 ",
		AnoEscolar:             " 1_ano_fundamental ",
		DeclaracaoAnoAcademico: " 1_ano_fundamental ",
	}

	req, declaracaoAnoAcademico, err := item.toCadastroRequest()
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if req.Nome != "Ana Maria" || req.Email != "ana@example.com" || req.AnoEscolar != "1_ano_fundamental" {
		t.Fatalf("campos textuais não foram normalizados: %+v", req)
	}
	if declaracaoAnoAcademico != "1_ano_fundamental" {
		t.Fatalf("declaracaoAnoAcademico inesperado: %q", declaracaoAnoAcademico)
	}
	if got := req.DataNascimento.Format("2006-01-02"); got != "2005-04-03" {
		t.Fatalf("data_nascimento inesperada: %s", got)
	}
}

func TestCadastroEstudanteJSONItemToCadastroRequestRejectsInvalidDate(t *testing.T) {
	t.Parallel()

	_, _, err := (cadastroEstudanteJSONItem{DataNascimento: "03/04/2005"}).toCadastroRequest()
	if err == nil || !strings.Contains(err.Error(), "data_nascimento deve ser YYYY-MM-DD") {
		t.Fatalf("erro esperado de data inválida, obtido: %v", err)
	}
}
