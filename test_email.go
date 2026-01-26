package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gopkg.in/gomail.v2"
)

func main() {
	// Carregar .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env não encontrado, usando variáveis do sistema")
	}

	host := os.Getenv("SMTP_HOST")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASSWORD")
	port := 587

	if host == "" || user == "" || pass == "" {
		log.Fatal("❌ Configure SMTP_HOST, SMTP_USER e SMTP_PASSWORD")
	}

	fmt.Printf("🔧 Testando conexão Gmail...\n")
	fmt.Printf("   Host: %s:%d\n", host, port)
	fmt.Printf("   User: %s\n", user)
	fmt.Printf("   Pass: %s...\n\n", pass[:4])

	// Criar mensagem de teste
	m := gomail.NewMessage()
	m.SetHeader("From", user)
	m.SetHeader("To", "fredrodrigues795@gmail.com") // Enviar para si mesmo
	m.SetHeader("Subject", "🧪 Teste Spuri Email Service")
	m.SetBody("text/html", `
		<h2>✅ Teste de Email Bem-Sucedido!</h2>
		<p>Se você está lendo isso, o serviço de email está funcionando perfeitamente.</p>
		<p><strong>Configuração:</strong></p>
		<ul>
			<li>SMTP: `+host+`</li>
			<li>Porta: 587 (STARTTLS)</li>
			<li>Biblioteca: gomail.v2</li>
		</ul>
	`)

	// Criar dialer
	d := gomail.NewDialer(host, port, user, pass)
	d.SSL = false // Porta 587 usa STARTTLS, não SSL direto

	// Tentar enviar
	fmt.Println("📧 Enviando email de teste...")
	if err := d.DialAndSend(m); err != nil {
		log.Fatalf("❌ Falha: %v", err)
	}

	fmt.Println("✅ Email enviado com sucesso!")
	fmt.Printf("📬 Verifique a caixa de entrada de: %s\n", user)
}