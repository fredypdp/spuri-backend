package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	
	// Carregar .env
	if err := godotenv.Load(); err != nil {
		log.Println("[WARN] Arquivo .env não encontrado")
	}

	// Configurações do Gmail
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("FROM_EMAIL")
	testTo := os.Getenv("TEST_EMAIL") // Email de destino para teste

	if host == "" || user == "" || pass == "" {
		log.Fatal("[ERROR] Configure SMTP_HOST, SMTP_USER, SMTP_PASSWORD no .env")
	}

	if testTo == "" {
		testTo = user // Envia para si mesmo se não especificado
	}

	if from == "" {
		from = user
	}

	if port == "" {
		port = "587"
	}

	log.Printf("[CONFIG] Host: %s", host)
	log.Printf("[CONFIG] Port: %s", port)
	log.Printf("[CONFIG] User: %s", user)
	log.Printf("[CONFIG] From: %s", from)
	log.Printf("[CONFIG] To: %s", testTo)
	log.Println("\n=== INICIANDO TESTES ===\n")

	// Teste 1: Conectividade básica
	if err := testConnection(host, port); err != nil {
		log.Printf("[FAIL] Teste de conexão: %v\n", err)
	} else {
		log.Println("[PASS] Teste de conexão\n")
	}

	// Teste 2: TLS Handshake
	if err := testTLS(host, port); err != nil {
		log.Printf("[FAIL] Teste TLS: %v\n", err)
	} else {
		log.Println("[PASS] Teste TLS\n")
	}

	// Teste 3: Autenticação
	if err := testAuth(host, port, user, pass); err != nil {
		log.Printf("[FAIL] Teste de autenticação: %v\n", err)
	} else {
		log.Println("[PASS] Teste de autenticação\n")
	}

	// Teste 4: Envio completo (método STARTTLS - porta 587)
	if port == "587" {
		log.Println("\n=== TESTE ENVIO STARTTLS (porta 587) ===")
		if err := sendEmailSTARTTLS(host, port, user, pass, from, testTo); err != nil {
			log.Printf("[FAIL] Envio STARTTLS: %v\n", err)
		} else {
			log.Println("[PASS] Email enviado com STARTTLS!\n")
		}
	}

	// Teste 5: Envio completo (método SSL - porta 465)
	if port == "465" {
		log.Println("\n=== TESTE ENVIO SSL (porta 465) ===")
		if err := sendEmailSSL(host, port, user, pass, from, testTo); err != nil {
			log.Printf("[FAIL] Envio SSL: %v\n", err)
		} else {
			log.Println("[PASS] Email enviado com SSL!\n")
		}
	}

	log.Println("\n=== TESTES FINALIZADOS ===")
}

func testConnection(host, port string) error {
	log.Printf("[TEST] Testando conexão TCP com %s:%s...", host, port)
	
	addr := fmt.Sprintf("%s:%s", host, port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("falha ao conectar: %w", err)
	}
	defer conn.Close()
	
	log.Println("[TEST] ✓ Conexão TCP estabelecida")
	return nil
}

func testTLS(host, port string) error {
	log.Printf("[TEST] Testando TLS handshake com %s:%s...", host, port)
	
	addr := fmt.Sprintf("%s:%s", host, port)
	
	if port == "465" {
		// SSL direto
		config := &tls.Config{
			ServerName: host,
		}
		conn, err := tls.Dial("tcp", addr, config)
		if err != nil {
			return fmt.Errorf("falha no TLS direto: %w", err)
		}
		defer conn.Close()
		log.Printf("[TEST] ✓ TLS direto (SSL) estabelecido - versão: %s", tlsVersion(conn.ConnectionState().Version))
		
	} else {
		// STARTTLS
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("falha ao conectar: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("falha ao criar cliente SMTP: %w", err)
		}
		defer client.Quit()

		if ok, _ := client.Extension("STARTTLS"); ok {
			config := &tls.Config{
				ServerName: host,
			}
			if err := client.StartTLS(config); err != nil {
				return fmt.Errorf("falha no STARTTLS: %w", err)
			}
			log.Println("[TEST] ✓ STARTTLS estabelecido")
		} else {
			return fmt.Errorf("STARTTLS não disponível")
		}
	}
	
	return nil
}

func testAuth(host, port, user, pass string) error {
	log.Printf("[TEST] Testando autenticação SMTP...")
	
	addr := fmt.Sprintf("%s:%s", host, port)
	auth := smtp.PlainAuth("", user, pass, host)
	
	if port == "465" {
		// SSL
		config := &tls.Config{ServerName: host}
		conn, err := tls.Dial("tcp", addr, config)
		if err != nil {
			return fmt.Errorf("falha ao conectar SSL: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("falha ao criar cliente: %w", err)
		}
		defer client.Quit()

		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("falha na autenticação: %w", err)
		}
		
	} else {
		// STARTTLS
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("falha ao conectar: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("falha ao criar cliente: %w", err)
		}
		defer client.Quit()

		if ok, _ := client.Extension("STARTTLS"); ok {
			config := &tls.Config{ServerName: host}
			if err := client.StartTLS(config); err != nil {
				return fmt.Errorf("falha no STARTTLS: %w", err)
			}
		}

		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("falha na autenticação: %w", err)
		}
	}
	
	log.Println("[TEST] ✓ Autenticação bem-sucedida")
	return nil
}

func sendEmailSTARTTLS(host, port, user, pass, from, to string) error {
	log.Printf("[SEND] Enviando email de %s para %s via STARTTLS...", from, to)
	
	addr := fmt.Sprintf("%s:%s", host, port)
	auth := smtp.PlainAuth("", user, pass, host)

	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: Teste SMTP - STARTTLS\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"<h1>Teste de Email via STARTTLS</h1>\r\n"+
		"<p>Este é um email de teste enviado em %s</p>\r\n",
		from, to, time.Now().Format("2006-01-02 15:04:05")))

	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return fmt.Errorf("falha ao conectar: %w", err)
	}
	defer conn.Close()
	log.Println("[SEND] ✓ Conectado")

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("falha ao criar cliente: %w", err)
	}
	defer client.Quit()
	log.Println("[SEND] ✓ Cliente criado")

	if ok, _ := client.Extension("STARTTLS"); ok {
		config := &tls.Config{ServerName: host}
		if err = client.StartTLS(config); err != nil {
			return fmt.Errorf("falha no STARTTLS: %w", err)
		}
		log.Println("[SEND] ✓ STARTTLS iniciado")
	}

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("falha na autenticação: %w", err)
	}
	log.Println("[SEND] ✓ Autenticado")

	if err = client.Mail(from); err != nil {
		return fmt.Errorf("falha ao definir remetente: %w", err)
	}
	log.Println("[SEND] ✓ Remetente definido")

	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("falha ao definir destinatário: %w", err)
	}
	log.Println("[SEND] ✓ Destinatário definido")

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("falha ao abrir data: %w", err)
	}
	log.Println("[SEND] ✓ Data aberto")

	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("falha ao escrever mensagem: %w", err)
	}
	log.Println("[SEND] ✓ Mensagem escrita")

	if err = w.Close(); err != nil {
		return fmt.Errorf("falha ao fechar data: %w", err)
	}
	log.Println("[SEND] ✓ Data fechado")

	log.Println("[SEND] ✓✓✓ EMAIL ENVIADO COM SUCESSO ✓✓✓")
	return nil
}

func sendEmailSSL(host, port, user, pass, from, to string) error {
	log.Printf("[SEND] Enviando email de %s para %s via SSL...", from, to)
	
	addr := fmt.Sprintf("%s:%s", host, port)
	auth := smtp.PlainAuth("", user, pass, host)

	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: Teste SMTP - SSL\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"<h1>Teste de Email via SSL</h1>\r\n"+
		"<p>Este é um email de teste enviado em %s</p>\r\n",
		from, to, time.Now().Format("2006-01-02 15:04:05")))

	config := &tls.Config{ServerName: host}
	
	conn, err := tls.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("falha ao conectar SSL: %w", err)
	}
	defer conn.Close()
	log.Println("[SEND] ✓ Conectado via SSL")

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("falha ao criar cliente: %w", err)
	}
	defer client.Quit()
	log.Println("[SEND] ✓ Cliente criado")

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("falha na autenticação: %w", err)
	}
	log.Println("[SEND] ✓ Autenticado")

	if err = client.Mail(from); err != nil {
		return fmt.Errorf("falha ao definir remetente: %w", err)
	}
	log.Println("[SEND] ✓ Remetente definido")

	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("falha ao definir destinatário: %w", err)
	}
	log.Println("[SEND] ✓ Destinatário definido")

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("falha ao abrir data: %w", err)
	}
	log.Println("[SEND] ✓ Data aberto")

	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("falha ao escrever mensagem: %w", err)
	}
	log.Println("[SEND] ✓ Mensagem escrita")

	if err = w.Close(); err != nil {
		return fmt.Errorf("falha ao fechar data: %w", err)
	}
	log.Println("[SEND] ✓ Data fechado")

	log.Println("[SEND] ✓✓✓ EMAIL ENVIADO COM SUCESSO ✓✓✓")
	return nil
}

func tlsVersion(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (%d)", version)
	}
}