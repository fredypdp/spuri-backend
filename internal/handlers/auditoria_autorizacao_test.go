package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/projections"
)

func TestPodeAuditarEstudanteRestringeEstudanteAoProprioAggregate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	estudanteID := uuid.New()
	estudante := &projections.EstudanteDTO{ID: estudanteID}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("user_type", "estudante")
	ctx.Set("user_id", estudanteID)
	if !podeAuditarEstudante(ctx, estudante) {
		t.Fatal("estudante deve auditar o proprio aggregate")
	}

	ctx.Set("user_id", uuid.New())
	if podeAuditarEstudante(ctx, estudante) {
		t.Fatal("estudante nao deve auditar aggregate de outro estudante")
	}
}

func TestPodeAuditarEstudantePermiteAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("user_type", "admin")
	ctx.Set("user_id", uuid.New())
	if !podeAuditarEstudante(ctx, &projections.EstudanteDTO{ID: uuid.New()}) {
		t.Fatal("admin deve auditar qualquer aggregate de estudante")
	}
}
