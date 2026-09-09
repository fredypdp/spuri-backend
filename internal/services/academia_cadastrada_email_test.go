package services

import "testing"

func TestSendAcademiaCadastradaEmailDisabledDoesNotError(t *testing.T) {
	t.Setenv("EMAILJS_SERVICE_ID", "")
	t.Setenv("EMAILJS_TEMPLATE_VERIFICATION", "")
	t.Setenv("EMAILJS_TEMPLATE_RESET", "")
	t.Setenv("EMAILJS_PUBLIC_KEY", "")
	t.Setenv("EMAILJS_PRIVATE_KEY", "")
	t.Setenv("EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA", "")

	svc := NewEmailService(nil)
	if svc.IsEnabled() {
		t.Fatal("esperava serviço desabilitado sem EMAILJS_* configurado")
	}

	err := svc.SendAcademiaCadastradaEmail("admin@example.com", "Admin Teste", AcademiaCadastradaInfo{
		Nome:           "Academia Teste",
		CodigoAcademia: "LDA2026A001",
		NIF:            "5417845812",
		Nivel:          "escola",
		Type:           "private",
		Provincia:      "LDA",
	})
	if err != nil {
		t.Fatalf("com serviço desabilitado deveria retornar nil (não bloqueia o cadastro), obteve: %v", err)
	}
}

func TestSendAcademiaCadastradaEmailEmptyRecipientErrors(t *testing.T) {
	t.Setenv("EMAILJS_SERVICE_ID", "svc_test")
	t.Setenv("EMAILJS_TEMPLATE_VERIFICATION", "tpl_verify")
	t.Setenv("EMAILJS_TEMPLATE_RESET", "tpl_reset")
	t.Setenv("EMAILJS_PUBLIC_KEY", "pub_test")
	t.Setenv("EMAILJS_PRIVATE_KEY", "")
	t.Setenv("EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA", "tpl_academia")

	svc := NewEmailService(nil)
	if !svc.IsEnabled() {
		t.Fatal("esperava serviço habilitado com EMAILJS_* mínimos configurados")
	}

	err := svc.SendAcademiaCadastradaEmail("", "Admin Teste", AcademiaCadastradaInfo{Nome: "Academia Teste"})
	if err == nil {
		t.Fatal("esperava erro para destinatário vazio")
	}
}

func TestSendAcademiaCadastradaEmailMissingTemplateDoesNotError(t *testing.T) {
	t.Setenv("EMAILJS_SERVICE_ID", "svc_test")
	t.Setenv("EMAILJS_TEMPLATE_VERIFICATION", "tpl_verify")
	t.Setenv("EMAILJS_TEMPLATE_RESET", "tpl_reset")
	t.Setenv("EMAILJS_PUBLIC_KEY", "pub_test")
	t.Setenv("EMAILJS_PRIVATE_KEY", "")
	t.Setenv("EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA", "") // deliberadamente não configurado

	svc := NewEmailService(nil)
	if !svc.IsEnabled() {
		t.Fatal("esperava serviço habilitado com EMAILJS_* mínimos configurados")
	}

	err := svc.SendAcademiaCadastradaEmail("admin@example.com", "Admin Teste", AcademiaCadastradaInfo{Nome: "Academia Teste"})
	if err != nil {
		t.Fatalf("sem EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA configurado deveria retornar nil (sem fallback de template), obteve: %v", err)
	}
}
