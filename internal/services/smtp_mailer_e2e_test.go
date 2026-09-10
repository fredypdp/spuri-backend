package services

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockSMTPServer struct {
	listener net.Listener
	mu       sync.Mutex
	rawData  string
	authSeen bool
}

func startMockSMTPServer(t *testing.T) *mockSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &mockSMTPServer{listener: ln}
	go s.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}
func (s *mockSMTPServer) addr() string { return s.listener.Addr().String() }
func (s *mockSMTPServer) serve() {
	for {
		c, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}
func (s *mockSMTPServer) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	write := func(f string, a ...interface{}) { _, _ = fmt.Fprintf(c, f+"\r\n", a...) }
	write("220 mock.local ESMTP ready")
	inData := false
	var data strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				inData = false
				s.mu.Lock()
				s.rawData = data.String()
				s.mu.Unlock()
				write("250 OK: queued")
				continue
			}
			data.WriteString(line + "\r\n")
			continue
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250-mock.local greets you")
			write("250 AUTH PLAIN")
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			s.mu.Lock()
			s.authSeen = true
			s.mu.Unlock()
			write("235 Authentication successful")
		case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
			write("250 OK")
		case upper == "DATA":
			inData = true
			write("354 Send message content; end with <CRLF>.<CRLF>")
		case upper == "QUIT":
			write("221 Bye")
			return
		default:
			write("500 unrecognized command")
		}
	}
}
func (s *mockSMTPServer) received() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rawData, s.authSeen
}
func waitMail(t *testing.T, s *mockSMTPServer) string {
	t.Helper()
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		raw, _ := s.received()
		if raw != "" {
			return raw
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ""
}
func TestSendSMTPEmailAgainstRealServer(t *testing.T) {
	s := startMockSMTPServer(t)
	host, port, err := net.SplitHostPort(s.addr())
	if err != nil {
		t.Fatal(err)
	}
	err = sendSMTPEmail(SMTPConfig{Host: host, Port: port, User: "spuri@example.com", Pass: "senha", From: "spuri@example.com"}, "admin@example.com", "Admin Teste", "Nova instituição cadastrada: Academia Teste", "pendente de analise", "<p><strong>Academia Teste</strong></p>")
	if err != nil {
		t.Fatal(err)
	}
	raw := waitMail(t, s)
	_, auth := s.received()
	if !auth || raw == "" {
		t.Fatal("handshake SMTP ou DATA ausente")
	}
	for _, want := range []string{"Content-Type: multipart/alternative", "admin@example.com", "pendente de analise", "<strong>Academia Teste</strong>"} {
		if !strings.Contains(raw, want) {
			t.Errorf("DATA não contém %q", want)
		}
	}
}
func TestSendAcademiaCadastradaEmailSMTPAgainstRealServer(t *testing.T) {
	s := startMockSMTPServer(t)
	host, port, _ := net.SplitHostPort(s.addr())
	t.Setenv("EMAIL_HOST", host)
	t.Setenv("EMAIL_PORT", port)
	t.Setenv("EMAIL_USER", "spuri@example.com")
	t.Setenv("EMAIL_PASS", "senha")
	t.Setenv("FRONTEND_URL", "https://painel.exemplo.com")
	err := NewEmailService(nil).SendAcademiaCadastradaEmailSMTP("admin@example.com", "Admin Teste", AcademiaCadastradaInfo{Nome: "Academia Teste", CodigoAcademia: "LDA2026A001", NIF: "5417845812"})
	if err != nil {
		t.Fatal(err)
	}
	raw := waitMail(t, s)
	for _, want := range []string{"LDA2026A001", "5417845812", "https://painel.exemplo.com/academias"} {
		if !strings.Contains(raw, want) {
			t.Errorf("DATA não contém %q", want)
		}
	}
}
