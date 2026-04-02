/*Uma nota técnica importante sobre batch_context.go:
O batchResponseWriter implementa gin.ResponseWriter (interface). Se o Gin atualizar a interface e adicionar novos métodos, o ficheiro vai falhar em compilação. Para garantir que compila imediatamente, confirma que a versão do Gin em uso tem exatamente os métodos implementados. Se algum método falhar, o erro de compilação vai indicar qual falta — é só adicioná-lo com corpo vazio return nil.
Sobre o Pusher() http.Pusher — se a versão do Gin não incluir esse método na interface, simplesmente remove essa linha de batch_context.go.*/

package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

// batchResponseWriter é um ResponseWriter sintético que captura status e corpo
// sem escrever em nenhuma conexão real de rede. Usado exclusivamente pelos
// batch handlers para chamar handlers individuais de forma sintética.
type batchResponseWriter struct {
	*httptest.ResponseRecorder
	statusWritten bool
}

func newBatchResponseWriter() *batchResponseWriter {
	rec := httptest.NewRecorder()
	rec.Code = http.StatusOK
	return &batchResponseWriter{ResponseRecorder: rec}
}

// Implementação da interface gin.ResponseWriter
func (w *batchResponseWriter) WriteHeader(code int) {
	if !w.statusWritten {
		w.Code = code
		w.statusWritten = true
	}
}

func (w *batchResponseWriter) WriteHeaderNow() {}

func (w *batchResponseWriter) Written() bool {
	return w.statusWritten
}

func (w *batchResponseWriter) Status() int {
	return w.Code
}

func (w *batchResponseWriter) Size() int {
	return w.Body.Len()
}

func (w *batchResponseWriter) Pusher() http.Pusher {
	return nil
}

// newFakeContext cria um *gin.Context filho que herda autenticação e
// dependências injetadas (dbClient, repository, projManager) do contexto pai,
// mas tem seu próprio ResponseWriter e Request isolado — sem afetar a resposta
// HTTP real que está sendo construída pelo batch handler pai.
//
// Os valores propagados do pai são os mesmos injetados por setupRouter:
// user_id, user_type, admin_role, dbClient, repository, projManager, request_id.
func newFakeContext(parent *gin.Context) *gin.Context {
	w := newBatchResponseWriter()

	// Clonar o request do pai para preservar headers (Authorization, Content-Type, etc.)
	req := parent.Request.Clone(parent.Request.Context())
	req.Body = http.NoBody
	req.ContentLength = 0

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Propagar contexto de autenticação e dependências do pai
	for _, key := range []string{
		"user_id", "user_type", "admin_role",
		"dbClient", "repository", "projManager",
		"request_id",
	} {
		if v, exists := parent.Get(key); exists {
			c.Set(key, v)
		}
	}

	return c
}

// setJSONBody serializa v como JSON e define como body do request do contexto fake.
// Deve ser chamado antes do handler que usa c.ShouldBindJSON.
func setJSONBody(c *gin.Context, v interface{}) {
	b, _ := json.Marshal(v)
	c.Request.Body = io.NopCloser(bytes.NewReader(b))
	// Preservar headers existentes e definir Content-Type
	if c.Request.Header == nil {
		c.Request.Header = make(http.Header)
	}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.ContentLength = int64(len(b))
}

// extractResult lê o status HTTP e o corpo JSON do writer sintético
// e converte para BatchItemResult indexado.
func extractResult(c *gin.Context, index int) BatchItemResult {
	w, ok := c.Writer.(*batchResponseWriter)
	if !ok {
		return BatchItemResult{
			Index:   index,
			Sucesso: false,
			Erro:    "erro interno: writer inesperado no contexto fake",
		}
	}

	code := w.Code
	body := w.Body.Bytes()

	if code >= 200 && code < 300 {
		var dados interface{}
		if len(body) > 0 {
			_ = json.Unmarshal(body, &dados)
		}
		return BatchItemResult{Index: index, Sucesso: true, Dados: dados}
	}

	// Extrair mensagem de erro do corpo JSON da resposta
	var errResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &errResp)
	}
	msg := errResp.Message
	if msg == "" {
		msg = errResp.Error
	}
	if msg == "" {
		msg = http.StatusText(code)
	}

	return BatchItemResult{Index: index, Sucesso: false, Erro: msg}
}
