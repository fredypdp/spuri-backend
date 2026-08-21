package finance

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestIntegrationListCobrancasFiltraOrigemEstadoEIsolaPorAcademia cobre o
// Problema 1 documentado em
// "docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md":
// até esta correção não existia nenhuma consulta capaz de listar cobranças
// por academia, com ou sem filtro de estado.
func TestIntegrationListCobrancasFiltraOrigemEstadoEIsolaPorAcademia(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academiaA := mensalidadeCodigo()
	academiaB := mensalidadeCodigo()

	insert := func(academia, status, estudante, solicitacao string) {
		payload := map[string]any{"status": status, "amount": 500.0, "currency": "AOA", "description": "teste", "payment_method": "REF"}
		if estudante != "" {
			payload["codigo_estudante"] = estudante
		}
		if solicitacao != "" {
			payload["codigo_solicitacao"] = solicitacao
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), integrationMerchant("LST"), academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert(academiaA, "criada", "", "")             // avulsa
	insert(academiaA, "Success", "EST-LST-1", "")   // mensalidade paga
	insert(academiaA, "cancelada", "", "SOL-LST-1") // matrícula cancelada
	insert(academiaB, "criada", "", "")             // outra academia, não deve aparecer para A

	todas, err := service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, nil, nil, "", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if todas.Total != 3 {
		t.Fatalf("esperava 3 cobranças da academia A, obteve %d", todas.Total)
	}

	pagas, err := service.ListCobrancas(ctx, ContextoAcademia, academiaA, []string{"Success"}, nil, nil, nil, "", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if pagas.Total != 1 || len(pagas.Cobrancas) != 1 || pagas.Cobrancas[0].Origem != "mensalidade" {
		t.Fatalf("filtro por estado=Success deveria devolver só a cobrança de mensalidade: %#v", pagas)
	}

	matriculas, err := service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, []string{"matricula"}, nil, nil, "", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if matriculas.Total != 1 || matriculas.Cobrancas[0].CodigoSolicitacao != "SOL-LST-1" {
		t.Fatalf("filtro por tipo=matricula incorreto: %#v", matriculas)
	}

	pagina, err := service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, nil, nil, "", "", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina.Cobrancas) != 1 || pagina.Total != 3 {
		t.Fatalf("paginação incorreta: len=%d total=%d", len(pagina.Cobrancas), pagina.Total)
	}

	outraAcademia, err := service.ListCobrancas(ctx, ContextoAcademia, academiaB, nil, nil, nil, nil, "", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if outraAcademia.Total != 1 {
		t.Fatalf("academia B deveria ver só a própria cobrança, obteve %d", outraAcademia.Total)
	}
}
