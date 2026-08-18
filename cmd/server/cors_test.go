package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCORSPreflightAllowsAllRegisteredHTTPMethods garante que o preflight
// OPTIONS tratado por corsMiddleware sempre libera, em Access-Control-Allow-Methods,
// todo método HTTP que exista em pelo menos uma rota registrada em setupRouter().
//
// Este teste existe porque Access-Control-Allow-Methods em corsMiddleware
// (cmd/server/main.go) é uma string fixa, mantida manualmente, separada da
// lista real de rotas. Quando uma rota nova passa a usar um método HTTP que
// ainda não constava nessa string (foi o caso de PATCH em
// /academia/notas-aluno/:id e /academia/faltas-aluno/:id), a chamada direta
// via curl/Postman continua funcionando normalmente — só o navegador, que faz
// preflight OPTIONS antes de métodos não "simples", passa a bloquear a
// requisição real com o erro:
//
//	"Method PATCH is not allowed by Access-Control-Allow-Methods in preflight response"
//
// Isso faz o bug ficar invisível em testes manuais de terminal e só aparecer
// em produção, no navegador. Ao invés de fixar apenas "PATCH" no teste, ele
// itera dinamicamente sobre router.Routes(), então qualquer método novo
// adicionado no futuro (ex.: um DELETE ou HEAD em alguma rota nova) que não
// for espelhado em Access-Control-Allow-Methods derruba este teste
// automaticamente, sem precisar lembrar de atualizá-lo.
func TestCORSPreflightAllowsAllRegisteredHTTPMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	registeredMethods := map[string]bool{}
	for _, route := range router.Routes() {
		registeredMethods[route.Method] = true
	}
	if len(registeredMethods) == 0 {
		t.Fatal("nenhuma rota registrada em setupRouter(); não é possível validar o CORS")
	}

	req := httptest.NewRequest(http.MethodOptions, "/academia/faltas-aluno/00000000-0000-0000-0000-000000000000", nil)
	req.Header.Set("Origin", "https://spuri-teste.vercel.app")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("esperado 204 no preflight OPTIONS, recebeu %d (body=%s)", w.Code, w.Body.String())
	}

	allowMethods := w.Header().Get("Access-Control-Allow-Methods")
	if allowMethods == "" {
		t.Fatal("Access-Control-Allow-Methods ausente na resposta de preflight")
	}

	for method := range registeredMethods {
		if method == http.MethodOptions {
			continue
		}
		if !strings.Contains(allowMethods, method) {
			t.Errorf(
				"método %s está registrado em pelo menos uma rota mas ausente em Access-Control-Allow-Methods (%q); "+
					"navegadores bloquearão o preflight para esse método em produção",
				method, allowMethods,
			)
		}
	}
}

// TestCORSPreflightFaltasAlunoAndNotasAlunoAllowPatch cobre especificamente os
// dois endpoints que geraram o bug em produção (correção de nota e de falta),
// reproduzindo o preflight real feito pelo front-end (mesmo Origin usado pelo
// spuripainel de teste), com asserção explícita de que PATCH está liberado.
func TestCORSPreflightFaltasAlunoAndNotasAlunoAllowPatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	paths := []string{
		"/academia/notas-aluno/00000000-0000-0000-0000-000000000000",
		"/academia/faltas-aluno/00000000-0000-0000-0000-000000000000",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodOptions, path, nil)
		req.Header.Set("Origin", "https://spuri-teste.vercel.app")
		req.Header.Set("Access-Control-Request-Method", "PATCH")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("%s: esperado 204 no preflight OPTIONS, recebeu %d", path, w.Code)
		}

		allowMethods := w.Header().Get("Access-Control-Allow-Methods")
		if !strings.Contains(allowMethods, "PATCH") {
			t.Fatalf("%s: PATCH ausente em Access-Control-Allow-Methods (%q)", path, allowMethods)
		}

		allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "https://spuri-teste.vercel.app" {
			t.Fatalf("%s: Access-Control-Allow-Origin inesperado (%q)", path, allowOrigin)
		}
	}
}
