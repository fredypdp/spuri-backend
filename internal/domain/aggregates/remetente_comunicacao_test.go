package aggregates

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRemetenteComunicacaoConfigurar(t *testing.T) {
	actor := uuid.New()
	validZiettID := uuid.NewString()

	tests := []struct {
		name          string
		provedor      string
		identificador string
		token         string
		wantErr       string
	}{
		{name: "accepts GoSMS sender", provedor: ProvedorComunicacaoGoSMS, identificador: "spuri123", token: "ciphertext"},
		{name: "rejects long GoSMS sender", provedor: ProvedorComunicacaoGoSMS, identificador: "ABCDEFGHIJKL", token: "ciphertext", wantErr: "1 a 11"},
		{name: "rejects non alphanumeric GoSMS sender", provedor: ProvedorComunicacaoGoSMS, identificador: "SPURI-1", token: "ciphertext", wantErr: "alfanuméricos"},
		{name: "accepts Ziett UUID", provedor: ProvedorComunicacaoZiett, identificador: validZiettID, token: "ciphertext"},
		{name: "rejects invalid Ziett ID", provedor: ProvedorComunicacaoZiett, identificador: "not-a-uuid", token: "ciphertext", wantErr: "UUID"},
		{name: "rejects unknown provider", provedor: "OTHER", identificador: "sender", token: "ciphertext", wantErr: "provedor inválido"},
		{name: "rejects empty identifier", provedor: ProvedorComunicacaoGoSMS, identificador: "", token: "ciphertext", wantErr: "identificador é obrigatório"},
		{name: "rejects empty token", provedor: ProvedorComunicacaoGoSMS, identificador: "SPURI", token: "", wantErr: "token de API é obrigatório"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRemetenteComunicacao()
			err := r.Configurar(tt.provedor, tt.identificador, tt.token, actor)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Configurar() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Configurar() unexpected error: %v", err)
			}
			if r.Provedor != tt.provedor || r.TokenAPICifrado != tt.token || r.ConfiguradoPor != actor {
				t.Fatalf("Configurar() did not apply expected fields: %+v", r)
			}
		})
	}
}

func TestMensagemComunicacaoRegistrar(t *testing.T) {
	m := NewMensagemComunicacao()
	actor := uuid.New()
	detalhes := []TentativaEnvioComunicacao{{Provedor: ProvedorComunicacaoGoSMS, Sucesso: true, MensagemExternaID: "external-id"}}

	if err := m.Registrar("923456789", "Olá", ProvedorComunicacaoGoSMS, "", ProvedorComunicacaoGoSMS, StatusMensagemComunicacaoEnviada, "external-id", detalhes, actor, "admin", ""); err != nil {
		t.Fatalf("Registrar() unexpected error: %v", err)
	}
	if m.ProvedorTentado2 != "" || m.ProvedorUtilizado != ProvedorComunicacaoGoSMS || m.Status != StatusMensagemComunicacaoEnviada {
		t.Fatalf("Registrar() fields = %+v", m)
	}
	if len(m.DetalhesTentativas) != 1 || m.DetalhesTentativas[0].MensagemExternaID != "external-id" {
		t.Fatalf("Registrar() details = %+v", m.DetalhesTentativas)
	}
}
