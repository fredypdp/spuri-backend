package finance

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestIntegrationRemoveMatriculaConfiguracaoFluxoDeComando(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	service := NewService(client)

	if _, _, err := service.ConfigureCredential(context.Background(), nil, CredentialInput{
		ContextoTipo: ContextoAcademia, CodigoAcademia: academia,
		ClientID: "client-" + uuid.NewString(), ClientSecret: "secret-" + uuid.NewString(),
		GPOPaymentMethod: "GPO_QR_TEST", REFPaymentMethod: "REF_TEST",
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureCredential não deveria falhar: %v", err)
	}
	if _, err := service.ConfigureMatricula(context.Background(), MatriculaConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "7_ano_fundamental",
		Valor: 15000, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureMatricula falhou: %v", err)
	}

	if _, err := service.ResolveMatriculaConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil); err != nil {
		t.Fatalf("ResolveMatriculaConfiguracao antes da remoção não deveria falhar: %v", err)
	}

	if err := service.RemoveMatriculaConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("RemoveMatriculaConfiguracao não deveria falhar: %v", err)
	}

	// Sem configuração ativa: ResolveMatriculaConfiguracao volta a
	// significar "matrícula gratuita" (ErrNotFound), exatamente como se
	// nunca tivesse sido configurada.
	if _, err := service.ResolveMatriculaConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperava ErrNotFound após remoção, obteve: %v", err)
	}

	configs, err := service.ListMatriculaConfiguracoes(context.Background(), academia)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range configs {
		if c.Nivel == NivelFundamental && c.AnoAcademico == "7_ano_fundamental" {
			t.Fatalf("configuração removida ainda aparece em ListMatriculaConfiguracoes: %#v", c)
		}
	}

	// O evento original permanece no ledger, imutável.
	aggID := mensalidadeAggregateID(academia)
	var configurados, removidos int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='MatriculaConfigurada'`, aggID).Scan(&configurados); err != nil {
		t.Fatal(err)
	}
	if configurados != 1 {
		t.Fatalf("esperava 1 evento MatriculaConfigurada preservado, obteve %d", configurados)
	}
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='MatriculaConfiguracaoRemovida'`, aggID).Scan(&removidos); err != nil {
		t.Fatal(err)
	}
	if removidos != 1 {
		t.Fatalf("esperava 1 evento MatriculaConfiguracaoRemovida, obteve %d", removidos)
	}

	// Remover de novo (nada ativo) falha.
	if err := service.RemoveMatriculaConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, uuid.NewString(), "academia", "127.0.0.1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("segunda remoção deveria falhar com ErrNotFound, obteve: %v", err)
	}

	// Reconfigurar depois de remover funciona.
	if _, err := service.ConfigureMatricula(context.Background(), MatriculaConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "7_ano_fundamental",
		Valor: 20000, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("reconfiguração após remoção não deveria falhar: %v", err)
	}
	v, err := service.ResolveMatriculaConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil)
	if err != nil || v.Valor != 20000 {
		t.Fatalf("esperava 20000 após reconfiguração, obteve valor=%v err=%v", v.Valor, err)
	}

	// Rebuild() a partir do ledger reproduz o mesmo estado final.
	if err := service.projection.Rebuild(); err != nil {
		t.Fatalf("Rebuild não deveria falhar: %v", err)
	}
	vPosRebuild, err := service.ResolveMatriculaConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil)
	if err != nil || vPosRebuild.Valor != 20000 {
		t.Fatalf("após Rebuild, esperava 20000, obteve valor=%v err=%v", vPosRebuild.Valor, err)
	}
}
