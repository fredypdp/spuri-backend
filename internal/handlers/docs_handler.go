// ============================================================================
// ARQUIVO: internal/handlers/docs_handler.go
// Handler para documentação OpenAPI
// ============================================================================

package handlers

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// GetOpenAPISpec retorna a especificação OpenAPI 3.0
func GetOpenAPISpec(c *gin.Context) {
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "Spuri Event Sourcing API",
			"version":     "3.0.0",
			"description": "Sistema de gestão acadêmica com Event Sourcing, CQRS e auditoria completa",
			"contact": map[string]string{
				"name": "Fundação Pedro Pires",
				"url":  "https://fpp.ao",
			},
		},
		"servers": []map[string]string{
			{"url": "http://localhost:8080", "description": "Desenvolvimento"},
			{"url": "https://api.spuri.ao", "description": "Produção"},
		},
		"tags": []map[string]string{
			{"name": "Health", "description": "Saúde e status"},
			{"name": "Auth", "description": "Autenticação"},
			{"name": "Email", "description": "Verificação de email"},
			{"name": "Estudante", "description": "Operações de estudantes"},
			{"name": "Academia", "description": "Operações de academias"},
			{"name": "Admin", "description": "Operações administrativas"},
			{"name": "Consultas", "description": "Consultas públicas"},
		},
		"paths": map[string]interface{}{
			"/health": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Health"},
					"summary":     "Health check básico",
					"description": "Verifica saúde da aplicação (público)",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Sistema operacional",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"status":   map[string]string{"type": "string"},
											"database": map[string]string{"type": "string"},
											"service":  map[string]string{"type": "string"},
											"version":  map[string]string{"type": "string"},
										},
									},
								},
							},
						},
					},
				},
			},
			"/login": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Auth"},
					"summary":     "Login estudante/academia",
					"description": "Autentica estudante ou academia e retorna JWT",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"required": []string{"usuario", "senha", "type"},
									"properties": map[string]interface{}{
										"usuario": map[string]string{
											"type":        "string",
											"description": "codigo_estudante ou codigo_academia",
										},
										"senha": map[string]string{"type": "string"},
										"type": map[string]interface{}{
											"type": "string",
											"enum": []string{"estudante", "academia"},
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Login bem-sucedido",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/LoginResponse",
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Credenciais inválidas"},
					},
				},
			},
			"/admin/login": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":    []string{"Auth"},
					"summary": "Login administrador",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"email", "senha"},
									"properties": map[string]interface{}{
										"email": map[string]string{"type": "string"},
										"senha": map[string]string{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Login bem-sucedido"},
						"401": map[string]interface{}{"description": "Credenciais inválidas"},
					},
				},
			},
			"/estudante/register": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":    []string{"Auth"},
					"summary": "Registrar estudante",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/RegisterEstudanteRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Estudante criado"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
					},
				},
			},
			"/academia/register": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":    []string{"Auth"},
					"summary": "Registrar academia",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/RegisterAcademiaRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Academia criada"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
					},
				},
			},
			"/meu-perfil": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":     []string{"Consultas"},
					"summary":  "Ver perfil do usuário logado",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Dados do perfil"},
						"401": map[string]interface{}{"description": "Não autenticado"},
					},
				},
			},
			"/academias": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Consultas"},
					"summary":     "Listar todas academias",
					"description": "Lista academias (pública)",
					"parameters": []map[string]interface{}{
						{
							"name":        "limit",
							"in":          "query",
							"schema":      map[string]string{"type": "integer"},
							"description": "Limite de resultados (padrão: 50)",
						},
						{
							"name":        "offset",
							"in":          "query",
							"schema":      map[string]string{"type": "integer"},
							"description": "Offset para paginação",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Lista de academias"},
					},
				},
			},
			"/estudante/inscricao-escola": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Estudante"},
					"summary":  "Solicitar inscrição em escola",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"codigo_academia", "ano_escolar_inscricao"},
									"properties": map[string]interface{}{
										"codigo_academia": map[string]string{
											"type": "string",
										},
										"ano_escolar_inscricao": map[string]string{
											"type": "string",
										},
										"curso_medio": map[string]string{
											"type": "string",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Inscrição criada"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
						"401": map[string]interface{}{"description": "Não autenticado"},
					},
				},
			},
			"/academia/notas-aluno": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Registrar nota individual",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"required": []string{
										"codigo_estudante",
										"ano_lectivo",
										"periodo",
										"materia_disciplinar_id",
										"nota",
									},
									"properties": map[string]interface{}{
										"codigo_estudante": map[string]string{
											"type": "string",
										},
										"ano_lectivo": map[string]string{
											"type": "string",
										},
										"periodo": map[string]interface{}{
											"type": "string",
											"enum": []string{
												"1_trimestre",
												"2_trimestre",
												"3_trimestre",
												"1_semestre",
												"2_semestre",
											},
										},
										"materia_disciplinar_id": map[string]string{
											"type": "string",
										},
										"nota": map[string]interface{}{
											"type":    "number",
											"minimum": 0,
											"maximum": 20,
										},
										"observacao": map[string]string{
											"type": "string",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Nota registrada"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
						"403": map[string]interface{}{"description": "Academia inativa"},
					},
				},
			},
			"/academia/cursos": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Criar curso",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"nome", "type", "nivel"},
									"properties": map[string]interface{}{
										"nome": map[string]string{"type": "string"},
										"type": map[string]interface{}{
											"type": "string",
											"enum": []string{"medio", "superior"},
										},
										"nivel": map[string]interface{}{
											"type":  "array",
											"items": map[string]string{"type": "string"},
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Curso criado"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
					},
				},
				"get": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Listar cursos da academia",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Lista de cursos"},
					},
				},
			},
			"/admin/todos-registros": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":     []string{"Admin"},
					"summary":  "Listar todos registros (notas/faltas)",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"parameters": []map[string]interface{}{
						{
							"name":   "tipo",
							"in":     "query",
							"schema": map[string]string{"type": "string"},
							"description": "Filtrar por tipo: notas, faltas (opcional)",
						},
						{
							"name":   "limit",
							"in":     "query",
							"schema": map[string]string{"type": "integer"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Lista de registros"},
						"403": map[string]interface{}{"description": "Acesso negado"},
					},
				},
			},
			"/admin/rebuild-projection/{name}": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Admin"},
					"summary":  "Reconstruir projeção",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"parameters": []map[string]interface{}{
						{
							"name":     "name",
							"in":       "path",
							"required": true,
							"schema":   map[string]string{"type": "string"},
							"description": "Nome da projeção (estudantes, academias, all)",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Projeção reconstruída"},
						"400": map[string]interface{}{"description": "Nome inválido"},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"BearerAuth": map[string]string{
					"type":   "http",
					"scheme": "bearer",
					"bearerFormat": "JWT",
				},
			},
			"schemas": map[string]interface{}{
				"LoginResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"token": map[string]string{"type": "string"},
						"codigo": map[string]string{"type": "string"},
						"nome": map[string]string{"type": "string"},
						"type": map[string]string{"type": "string"},
					},
				},
				"RegisterEstudanteRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"nome", "senha"},
					"properties": map[string]interface{}{
						"nome":  map[string]string{"type": "string"},
						"senha": map[string]string{"type": "string"},
						"email": map[string]string{"type": "string"},
						"telefone": map[string]string{"type": "string"},
						"bilhete_identidade": map[string]string{"type": "string"},
						"bilhete_identidade_responsavel": map[string]string{"type": "string"},
					},
				},
				"RegisterAcademiaRequest": map[string]interface{}{
					"type": "object",
					"required": []string{"type", "nome", "senha", "provincia", "endereco"},
					"properties": map[string]interface{}{
						"type": map[string]interface{}{
							"type": "string",
							"enum": []string{"escola", "superior"},
						},
						"nome":      map[string]string{"type": "string"},
						"senha":     map[string]string{"type": "string"},
						"provincia": map[string]string{"type": "string"},
						"endereco":  map[string]string{"type": "string"},
						"email":     map[string]string{"type": "string"},
						"nivel_escolar": map[string]interface{}{
							"type": "string",
							"enum": []string{"fundamental", "medio", "misto"},
						},
					},
				},
			},
		},
	}

	c.JSON(http.StatusOK, spec)
}