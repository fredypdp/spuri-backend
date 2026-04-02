package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

type batchResponseWriter struct {
	*httptest.ResponseRecorder
	statusWritten bool
}

func newBatchResponseWriter() *batchResponseWriter {
	rec := httptest.NewRecorder()
	rec.Code = http.StatusOK
	return &batchResponseWriter{ResponseRecorder: rec}
}

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

// CloseNotify implementa gin.ResponseWriter — retorna canal que nunca fecha
// (contexto sintético de batch não tem conexão real).
func (w *batchResponseWriter) CloseNotify() <-chan bool {
	return make(chan bool)
}

func newFakeContext(parent *gin.Context) *gin.Context {
	w := newBatchResponseWriter()

	req := parent.Request.Clone(parent.Request.Context())
	req.Body = http.NoBody
	req.ContentLength = 0

	c, _ := gin.CreateTestContext(w)
	c.Request = req

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

func setJSONBody(c *gin.Context, v interface{}) {
	b, _ := json.Marshal(v)
	c.Request.Body = io.NopCloser(bytes.NewReader(b))
	if c.Request.Header == nil {
		c.Request.Header = make(http.Header)
	}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.ContentLength = int64(len(b))
}

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