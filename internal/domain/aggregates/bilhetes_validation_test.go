package aggregates

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEstudanteCriarComVinculoRejeitaBilhetesIguais(t *testing.T) {
	bi := " 001LA001 "
	tel := "923000000"
	biResp := "001la001"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Aluno Teste",
		"ABC1234",
		strings.Repeat("a", 60),
		nil,
		&tel,
		nil,
		&bi,
		&biResp,
		"masculino",
		time.Now().AddDate(-10, 0, 0),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"ACA2026",
	)
	if err == nil || !strings.Contains(err.Error(), "não podem ser iguais") {
		t.Fatalf("esperava erro de bilhetes iguais, recebeu %v", err)
	}
}

func TestEstudanteAtualizarDadosPessoaisRejeitaBilhetesEfetivosIguais(t *testing.T) {
	bi := "001LA001"
	estudante := NewEstudante()
	estudante.BilheteIdentidade = &bi
	tel := "923000000"
	estudante.Telefone = &tel

	biResp := " 001la001 "
	err := estudante.AtualizarDadosPessoais(nil, nil, nil, nil, nil, &biResp, nil)
	if err == nil || !strings.Contains(err.Error(), "não podem ser iguais") {
		t.Fatalf("esperava erro de bilhetes iguais na atualização, recebeu %v", err)
	}
}

func TestSolicitacaoMatriculaCriarRejeitaBilhetesIguais(t *testing.T) {
	bi := "001LA001"
	biResp := " 001la001 "
	solicitacao := NewSolicitacaoMatricula()

	err := solicitacao.Criar(
		"SOL2026001",
		"ACA2026",
		"Aluno Teste",
		"feminino",
		time.Now().AddDate(-8, 0, 0),
		nil,
		nil,
		nil,
		&bi,
		&biResp,
		nil,
		nil,
		nil,
		nil,
		nil,
		map[string]DocumentoMatricula{"bi_responsavel": {Path: "bi.pdf"}},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "não podem ser iguais") {
		t.Fatalf("esperava erro de bilhetes iguais na solicitação, recebeu %v", err)
	}
}

func TestEstudanteCriarComVinculoAceitaPrimeiroFundamentalSemComprovativoAcademico(t *testing.T) {
	bi := "001LA002"
	biResp := "001LA003"
	tel := "923000000"
	telResp := "924000000"
	anoFundamental := "1_ano_fundamental"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Aluno Teste",
		"ABC1235",
		strings.Repeat("a", 60),
		nil,
		&tel,
		&telResp,
		&bi,
		&biResp,
		"masculino",
		time.Now().AddDate(-10, 0, 0),
		&anoFundamental,
		nil,
		nil,
		nil,
		nil,
		nil,
		"ACA2026",
		map[string]DocumentoMatricula{
			"bi_responsavel":                {Path: "bi-responsavel.pdf"},
			"bi_estudante":                  {Path: "bi-estudante.pdf"},
			"certificado_6_ano_fundamental": {},
		},
	)
	if err != nil {
		t.Fatalf("não esperava erro de comprovativo académico no 1_ano_fundamental, recebeu %v", err)
	}
}

func TestEstudanteCriarComVinculoAceitaDocumentoEscolarComDownloadURL(t *testing.T) {
	bi := "001LA004"
	biResp := "001LA005"
	tel := "923000000"
	telResp := "924000000"
	anoFundamental := "1_ano_fundamental"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Aluno Teste",
		"ABC1236",
		strings.Repeat("a", 60),
		nil,
		&tel,
		&telResp,
		&bi,
		&biResp,
		"feminino",
		time.Now().AddDate(-10, 0, 0),
		&anoFundamental,
		nil,
		nil,
		nil,
		nil,
		nil,
		"ACA2026",
		map[string]DocumentoMatricula{
			"bi_responsavel": {DownloadURL: "https://example.com/bi-responsavel.pdf"},
			"bi_estudante":   {FileURL: "https://example.com/bi-estudante.pdf"},
			"declaracao":     {Path: "declaracao.pdf", AnoAcademico: "9_ano_fundamental"},
		},
	)
	if err != nil {
		t.Fatalf("não esperava erro com documentos válidos, recebeu %v", err)
	}
}

func TestValidarDocumentosMatriculaRegrasAcademicasPorAno(t *testing.T) {
	bi := "001LA010"
	biResp := "001LA011"
	docsIdentificacao := map[string]DocumentoMatricula{
		"bi_responsavel": {Path: "bi-responsavel.pdf"},
		"bi_estudante":   {Path: "bi-estudante.pdf"},
	}

	anoPrimeiroFundamental := "1_ano_fundamental"
	if err := ValidarDocumentosMatricula(&bi, &biResp, &anoPrimeiroFundamental, nil, nil, docsIdentificacao); err != nil {
		t.Fatalf("1_ano_fundamental não deve exigir declaração/certificado: %v", err)
	}

	anoSetimo := "7_ano_fundamental"
	if err := ValidarDocumentosMatricula(&bi, &biResp, &anoSetimo, nil, nil, docsIdentificacao); err == nil || !strings.Contains(err.Error(), "certificado do 6.º ano fundamental") {
		t.Fatalf("7_ano_fundamental deve exigir certificado do 6.º ano ou declaração, recebeu %v", err)
	}
	docsSetimo := map[string]DocumentoMatricula{
		"bi_responsavel":                {Path: "bi-responsavel.pdf"},
		"bi_estudante":                  {Path: "bi-estudante.pdf"},
		"certificado_6_ano_fundamental": {Path: "certificado.pdf"},
	}
	if err := ValidarDocumentosMatricula(&bi, &biResp, &anoSetimo, nil, nil, docsSetimo); err != nil {
		t.Fatalf("7_ano_fundamental deve aceitar certificado do 6.º ano: %v", err)
	}

	anoMedio := "1_ano_medio"
	docsDeclaracao := map[string]DocumentoMatricula{
		"bi_responsavel": {Path: "bi-responsavel.pdf"},
		"bi_estudante":   {Path: "bi-estudante.pdf"},
		"declaracao":     {Path: "declaracao.pdf", AnoAcademico: "9_ano_fundamental"},
	}
	if err := ValidarDocumentosMatricula(&bi, &biResp, nil, &anoMedio, nil, docsDeclaracao); err != nil {
		t.Fatalf("1_ano_medio deve aceitar declaração alternativa: %v", err)
	}
}

func TestValidarDocumentosMatriculaDeclaracaoAnoAnteriorObrigatorio(t *testing.T) {
	bi := "001LA040"
	biResp := "001LA041"
	anoSegundo := "2_ano_fundamental"
	base := map[string]DocumentoMatricula{
		"bi_responsavel": {Path: "bi-responsavel.pdf"},
		"bi_estudante":   {Path: "bi-estudante.pdf"},
	}

	docsValidos := map[string]DocumentoMatricula{
		"bi_responsavel": {Path: "bi-responsavel.pdf"},
		"bi_estudante":   {Path: "bi-estudante.pdf"},
		"declaracao":     {Path: "declaracao.pdf", AnoAcademico: "1_ano_fundamental"},
	}
	if err := ValidarDocumentosMatricula(&bi, &biResp, &anoSegundo, nil, nil, docsValidos); err != nil {
		t.Fatalf("2_ano_fundamental deve aceitar declaração do 1_ano_fundamental: %v", err)
	}
	if err := ValidarDocumentosMatricula(&bi, &biResp, &anoSegundo, nil, nil, base); err == nil || !strings.Contains(err.Error(), "1_ano_fundamental") {
		t.Fatalf("2_ano_fundamental deve exigir declaração do ano anterior, recebeu %v", err)
	}

	for nome, anoDeclaracao := range map[string]string{
		"mesmo ano":             "2_ano_fundamental",
		"posterior":             "3_ano_fundamental",
		"anterior não imediato": "9_ano_fundamental",
		"sem ano":               "",
	} {
		docs := map[string]DocumentoMatricula{
			"bi_responsavel": {Path: "bi-responsavel.pdf"},
			"bi_estudante":   {Path: "bi-estudante.pdf"},
			"declaracao":     {Path: "declaracao.pdf", AnoAcademico: anoDeclaracao},
		}
		if err := ValidarDocumentosMatricula(&bi, &biResp, &anoSegundo, nil, nil, docs); err == nil || !strings.Contains(err.Error(), "1_ano_fundamental") {
			t.Fatalf("declaração com %s deve ser rejeitada e informar ano esperado, recebeu %v", nome, err)
		}
	}
}

func TestValidarDocumentosMatriculaCertificadoComDeclaracaoAlternativaAnoAnterior(t *testing.T) {
	bi := "001LA050"
	biResp := "001LA051"
	anoSetimo := "7_ano_fundamental"
	docsDeclaracao := map[string]DocumentoMatricula{
		"bi_responsavel": {Path: "bi-responsavel.pdf"},
		"bi_estudante":   {Path: "bi-estudante.pdf"},
		"declaracao":     {Path: "declaracao.pdf", AnoAcademico: "6_ano_fundamental"},
	}
	if err := ValidarDocumentosMatricula(&bi, &biResp, &anoSetimo, nil, nil, docsDeclaracao); err != nil {
		t.Fatalf("7_ano_fundamental deve aceitar declaração alternativa do 6_ano_fundamental: %v", err)
	}
	docsIncorretos := map[string]DocumentoMatricula{
		"bi_responsavel": {Path: "bi-responsavel.pdf"},
		"bi_estudante":   {Path: "bi-estudante.pdf"},
		"declaracao":     {Path: "declaracao.pdf", AnoAcademico: "5_ano_fundamental"},
	}
	if err := ValidarDocumentosMatricula(&bi, &biResp, &anoSetimo, nil, nil, docsIncorretos); err == nil || !strings.Contains(err.Error(), "6_ano_fundamental") {
		t.Fatalf("7_ano_fundamental deve rejeitar declaração alternativa de ano incorreto, recebeu %v", err)
	}
}

func TestValidarDocumentosMatriculaSuperiorExigeBIEstudanteENaoResponsavel(t *testing.T) {
	bi := "001LA020"
	anoSuperior := "1_ano_superior"
	docs := map[string]DocumentoMatricula{
		"bi_estudante":             {Path: "bi-estudante.pdf"},
		"certificado_ensino_medio": {Path: "certificado-medio.pdf"},
	}
	if err := ValidarDocumentosMatricula(&bi, nil, nil, nil, &anoSuperior, docs); err != nil {
		t.Fatalf("ensino superior deve aceitar BI do estudante sem BI do responsável: %v", err)
	}
	if err := ValidarDocumentosMatricula(nil, nil, nil, nil, &anoSuperior, docs); err == nil || !strings.Contains(err.Error(), "bilhete_identidade é obrigatório") {
		t.Fatalf("ensino superior deve rejeitar ausência do campo BI do estudante, recebeu %v", err)
	}
	if err := ValidarDocumentosMatricula(&bi, nil, nil, nil, &anoSuperior, map[string]DocumentoMatricula{"certificado_ensino_medio": {Path: "certificado.pdf"}}); err == nil || !strings.Contains(err.Error(), "bi_estudante") {
		t.Fatalf("ensino superior deve rejeitar ausência do documento BI do estudante, recebeu %v", err)
	}
	biResp := "001LA021"
	if err := ValidarDocumentosMatricula(&bi, &biResp, nil, nil, &anoSuperior, docs); err == nil || !strings.Contains(err.Error(), "bi_responsavel") {
		t.Fatalf("ensino superior deve rejeitar BI do responsável textual sem documento correspondente, recebeu %v", err)
	}
	docsComResponsavel := map[string]DocumentoMatricula{
		"bi_estudante":             {Path: "bi-estudante.pdf"},
		"bi_responsavel":           {Path: "bi-responsavel.pdf"},
		"certificado_ensino_medio": {Path: "certificado-medio.pdf"},
	}
	if err := ValidarDocumentosMatricula(&bi, &biResp, nil, nil, &anoSuperior, docsComResponsavel); err != nil {
		t.Fatalf("ensino superior deve aceitar BI do responsável quando documento correspondente é anexado: %v", err)
	}
}

func TestValidarDocumentosMatriculaEscolarExigeResponsavelEIdentificacaoDoEstudante(t *testing.T) {
	bi := "001LA030"
	biResp := "001LA031"
	ano := "1_ano_fundamental"
	if err := ValidarDocumentosMatricula(&bi, nil, &ano, nil, nil, map[string]DocumentoMatricula{"bi_estudante": {Path: "bi.pdf"}}); err == nil || !strings.Contains(err.Error(), "bilhete_identidade_responsavel") {
		t.Fatalf("escolar deve rejeitar ausência do campo BI do responsável, recebeu %v", err)
	}
	if err := ValidarDocumentosMatricula(&bi, &biResp, &ano, nil, nil, map[string]DocumentoMatricula{"bi_estudante": {Path: "bi.pdf"}}); err == nil || !strings.Contains(err.Error(), "bi_responsavel") {
		t.Fatalf("escolar deve rejeitar ausência do documento BI do responsável, recebeu %v", err)
	}
	if err := ValidarDocumentosMatricula(nil, &biResp, &ano, nil, nil, map[string]DocumentoMatricula{"bi_responsavel": {Path: "resp.pdf"}, "cedula_estudante": {Path: "cedula.pdf"}}); err != nil {
		t.Fatalf("escolar deve aceitar cédula válida sem BI próprio: %v", err)
	}
	if err := ValidarDocumentosMatricula(nil, &biResp, &ano, nil, nil, map[string]DocumentoMatricula{"bi_responsavel": {Path: "resp.pdf"}}); err == nil || !strings.Contains(err.Error(), "cedula_estudante") {
		t.Fatalf("escolar deve rejeitar estudante sem BI próprio e sem cédula, recebeu %v", err)
	}
}

func TestDocumentosMatriculaFaltantesSegueNivelEAnoAtual(t *testing.T) {
	bi := "001LA060"
	biResp := "001LA061"
	anoSetimo := "7_ano_fundamental"

	faltantes := DocumentosMatriculaFaltantes(&bi, &biResp, &anoSetimo, nil, nil, map[string]DocumentoMatricula{})
	esperados := []string{"bi_estudante", "bi_responsavel", "certificado_6_ano_fundamental ou declaracao_6_ano_fundamental"}
	if len(faltantes) != len(esperados) {
		t.Fatalf("faltantes = %v, esperados %v", faltantes, esperados)
	}
	for i := range esperados {
		if faltantes[i] != esperados[i] {
			t.Fatalf("faltantes = %v, esperados %v", faltantes, esperados)
		}
	}
}

func TestCompletarDocumentosPendentesNaoAtivaComDocumentosIncompletos(t *testing.T) {
	bi := "001LA070"
	biResp := "001LA071"
	anoSegundo := "2_ano_fundamental"
	academiaID := uuid.New()
	telefoneResp := "923000000"
	estudante := NewEstudante()
	if err := estudante.CriarComVinculoPendenteDocumentos("Aluno", "EST070", "hash", nil, nil, &telefoneResp, &bi, &biResp, "masculino", time.Now().AddDate(-10, 0, 0), &anoSegundo, nil, nil, nil, nil, &academiaID, "ACA", map[string]DocumentoMatricula{}); err != nil {
		t.Fatalf("criar pendente: %v", err)
	}

	err := estudante.CompletarDocumentosPendentes(map[string]DocumentoMatricula{
		"bi_estudante":   {Path: "bi.pdf"},
		"bi_responsavel": {Path: "resp.pdf"},
	}, academiaID)
	if err == nil || !strings.Contains(err.Error(), "declaracao_1_ano_fundamental") {
		t.Fatalf("esperava erro com documento faltante do ano anterior, recebeu %v", err)
	}
	if estudante.Status != "pendente_documentos" {
		t.Fatalf("status deve permanecer pendente_documentos, recebeu %s", estudante.Status)
	}
}

func TestSolicitacaoMatriculaCanceladaNaoPodeSerAprovadaOuReprovada(t *testing.T) {
	bi := "002LA001"
	biResp := "002LA002"
	doc := map[string]DocumentoMatricula{"bi_estudante": {Path: "bi.pdf"}, "bi_responsavel": {Path: "bi_resp.pdf"}}
	solicitacao := NewSolicitacaoMatricula()
	if err := solicitacao.Criar("SOL2026002", "ACA2026", "Aluno Teste", "masculino", time.Now().AddDate(-18, 0, 0), nil, nil, stringPtr("923456789"), &bi, &biResp, stringPtr("1_classe"), nil, nil, nil, nil, doc, []string{"SOL2026001"}); err != nil {
		t.Fatalf("erro inesperado ao criar solicitação: %v", err)
	}
	if err := solicitacao.Cancelar("matricula aprovada em outra instituicao", "SOL2026003", "EST0001"); err != nil {
		t.Fatalf("erro inesperado ao cancelar solicitação: %v", err)
	}
	events := solicitacao.GetUncommittedEvents()
	if err := solicitacao.Apply(events[len(events)-1]); err != nil {
		t.Fatalf("erro inesperado ao aplicar cancelamento: %v", err)
	}
	if err := solicitacao.Aprovar(uuid.New(), "EST0002"); err == nil {
		t.Fatalf("solicitação cancelada não deveria ser aprovada")
	}
	if err := solicitacao.Reprovar(uuid.New(), "documento inválido"); err == nil {
		t.Fatalf("solicitação cancelada não deveria ser reprovada")
	}
}

func stringPtr(v string) *string { return &v }
