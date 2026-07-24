package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var removedMatriculaFields = map[string]string{
	"bilhete_identidade_responsavel":  "bilhete_identidade_encarregado",
	"telefone_responsavel":            "telefone_encarregado",
	"telefone_responsavel_verificado": "telefone_encarregado_verificado",
	"bi_responsavel":                  "bi_encarregado",
}

func removedFieldMessage(oldField, newField string) string {
	return fmt.Sprintf("o campo '%s' não existe mais neste contrato; use '%s'", oldField, newField)
}

func respondRemovedField(c *gin.Context, oldField, newField string) {
	msg := removedFieldMessage(oldField, newField)
	c.JSON(http.StatusBadRequest, gin.H{"error": "VALIDATION_ERROR", "message": msg, "details": []gin.H{{"field": oldField, "code": "campo_removido", "message": msg}}})
}

func rejectRemovedJSONFields(c *gin.Context) bool {
	if c.Request == nil || c.Request.Body == nil || !strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "json") {
		return false
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	var raw any
	if json.Unmarshal(body, &raw) != nil {
		return false
	}
	if old, newf, ok := findRemovedJSONField(raw); ok {
		respondRemovedField(c, old, newf)
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return false
}

func findRemovedJSONField(v any) (string, string, bool) {
	switch x := v.(type) {
	case map[string]any:
		for old, newf := range removedMatriculaFields {
			if _, ok := x[old]; ok {
				return old, newf, true
			}
		}
		for _, child := range x {
			if old, newf, ok := findRemovedJSONField(child); ok {
				return old, newf, true
			}
		}
	case []any:
		for _, child := range x {
			if old, newf, ok := findRemovedJSONField(child); ok {
				return old, newf, true
			}
		}
	}
	return "", "", false
}

func findRemovedJSONFieldString(raw string) (string, string, bool) {
	var decoded any
	if json.Unmarshal([]byte(raw), &decoded) != nil {
		return "", "", false
	}
	return findRemovedJSONField(decoded)
}

var immutableStudentPersonalFields = map[string]struct{}{
	"nome":                           {},
	"bilhete_identidade":             {},
	"bilhete_identidade_encarregado": {},
	"data_nascimento":                {},
}

func rejectStudentPersonalImmutableFields(c *gin.Context) bool {
	if c.Request == nil || c.Request.Body == nil || !strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "json") {
		return false
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return false
	}
	for field := range immutableStudentPersonalFields {
		if _, ok := raw[field]; ok {
			log.Printf("[estudante-dados-debug] tentativa bloqueada de editar campo pessoal imutável: field=%s route=%s method=%s", field, c.FullPath(), c.Request.Method)
			msg := fmt.Sprintf("%s não pode ser alterado após o cadastro do estudante", field)
			c.JSON(http.StatusBadRequest, gin.H{"error": "VALIDATION_ERROR", "message": msg, "details": []gin.H{{"field": field, "code": "campo_imutavel", "message": msg}}})
			return true
		}
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return false
}

func rejectRemovedMultipartFields(c *gin.Context) bool {
	form := c.Request.MultipartForm
	if form == nil {
		return false
	}
	for old, newf := range removedMatriculaFields {
		if _, ok := form.Value[old]; ok {
			respondRemovedField(c, old, newf)
			return true
		}
		if _, ok := form.File[old]; ok {
			respondRemovedField(c, old, newf)
			return true
		}
	}
	for field := range form.File {
		if strings.HasSuffix(field, ".bi_responsavel") {
			respondRemovedField(c, "bi_responsavel", "bi_encarregado")
			return true
		}
	}
	return false
}
