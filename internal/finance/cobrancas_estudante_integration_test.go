package finance

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestIntegrationListCobrancasEstudanteIncluiMensalidadeEMatricula cobre o
// requisito de que um estudante deve conseguir consultar TODOS os pagamentos
// que já teve, em qualquer estado — incluindo a cobrança da matrícula
// original (associada por codigo_solicitacao, não por codigo_estudante,
// porque é anterior ao registo do estudante) e cobranças de mensalidade em
// qualquer estado (não só "pago").
func TestIntegrationListCobrancasEstudanteIncluiMensalidadeEMatricula(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	codigoSolicitacao := seedMatriculaPendente(t, client, academia, 750)
	codigoEstudante := "EST" + codigoSolicitacao[3:7] // mesmo cálculo de seedMatriculaPendente

	insert := func(status, estudante, solicitacao string) {
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
			uuid.New(), integrationMerchant("EST"), academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert("Success", "", codigoSolicitacao) // cobrança da matrícula original (sem codigo_estudante)
	insert("Success", codigoEstudante, "")   // mensalidade paga
	insert("Failed", codigoEstudante, "")    // mensalidade falhada — deve aparecer também (todos os estados)
	insert("Success", "OUTRO12", "")         // de outro estudante — não deve aparecer

	res, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 {
		t.Fatalf("esperava 3 cobranças do estudante (1 matrícula + 2 mensalidade), obteve %d: %#v", res.Total, res.Cobrancas)
	}
	var temMatricula, temFalhada bool
	for _, cobranca := range res.Cobrancas {
		if cobranca.Origem == "matricula" && cobranca.CodigoSolicitacao == codigoSolicitacao {
			temMatricula = true
		}
		if cobranca.Status == "Failed" {
			temFalhada = true
		}
	}
	if !temMatricula {
		t.Fatalf("cobrança de matrícula original não apareceu na listagem: %#v", res.Cobrancas)
	}
	if !temFalhada {
		t.Fatalf("cobrança falhada não apareceu (listagem deveria incluir todos os estados por padrão): %#v", res.Cobrancas)
	}

	pagas, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, []string{"Success"}, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if pagas.Total != 2 {
		t.Fatalf("filtro por estado=Success deveria devolver 2 cobranças (matrícula + mensalidade paga), obteve %d", pagas.Total)
	}

	// tarefa 49: o próprio estudante também precisa conseguir filtrar por
	// tipo de cobrança (mensalidade/matrícula/avulsa), mesmo mecanismo que
	// ListCobrancas já oferece à academia/admin.
	somenteMensalidade, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"mensalidade"}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if somenteMensalidade.Total != 2 {
		t.Fatalf("filtro por tipo=mensalidade deveria devolver 2 cobranças, obteve %d: %#v", somenteMensalidade.Total, somenteMensalidade.Cobrancas)
	}
	for _, cobranca := range somenteMensalidade.Cobrancas {
		if cobranca.Origem != "mensalidade" {
			t.Fatalf("filtro por tipo=mensalidade devolveu cobrança de origem %q: %#v", cobranca.Origem, cobranca)
		}
	}

	somenteMatricula, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"matricula"}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if somenteMatricula.Total != 1 || somenteMatricula.Cobrancas[0].CodigoSolicitacao != codigoSolicitacao {
		t.Fatalf("filtro por tipo=matricula deveria devolver só a cobrança da matrícula original, obteve %#v", somenteMatricula.Cobrancas)
	}

	if _, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"invalido"}, 50, 0); err == nil {
		t.Fatal("esperava erro para tipo de cobrança inválido")
	}
}

// TestIntegrationListCobrancasEstudanteSomenteAcademiaIsolaOutraAcademia
// cobre o isolamento por academia: quando somenteAcademia é passado (caso da
// academia autenticada, via ConsultarCobrancasEstudante), cobranças que o
// mesmo estudante fez a OUTRA academia não devem aparecer.
func TestIntegrationListCobrancasEstudanteSomenteAcademiaIsolaOutraAcademia(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academiaA := mensalidadeCodigo()
	academiaB := mensalidadeCodigo()
	estudante := "EST" + uuid.NewString()[:4]

	insert := func(academia string) {
		payload := map[string]any{"status": "Success", "amount": 500.0, "currency": "AOA", "description": "teste", "payment_method": "REF", "codigo_estudante": estudante}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), integrationMerchant("ISO"), academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert(academiaA)
	insert(academiaB)

	semRestricao, err := service.ListCobrancasEstudante(ctx, estudante, nil, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if semRestricao.Total != 2 {
		t.Fatalf("sem somenteAcademia deveria ver as 2 cobranças (histórico completo), obteve %d", semRestricao.Total)
	}

	comRestricao, err := service.ListCobrancasEstudante(ctx, estudante, &academiaA, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comRestricao.Total != 1 || comRestricao.Cobrancas[0].CodigoAcademia != academiaA {
		t.Fatalf("com somenteAcademia=%s deveria ver só a cobrança dessa academia, obteve %#v", academiaA, comRestricao.Cobrancas)
	}
}
