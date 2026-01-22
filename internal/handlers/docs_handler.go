// ============================================================================
// ARQUIVO: internal/handlers/docs_handler.go
// Handler para documentação OpenAPI com Swagger UI
// ============================================================================

package handlers

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// GetSwaggerUI retorna interface HTML do Swagger UI
func GetSwaggerUI(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="pt">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Spuri API - Documentação</title>
    <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.10.5/swagger-ui.min.css">
    <style>
        body { margin: 0; padding: 0; }
        .swagger-ui .topbar { display: none; }
        .swagger-ui .information-container { margin: 50px auto; max-width: 1460px; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.10.5/swagger-ui-bundle.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.10.5/swagger-ui-standalone-preset.min.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: '/docs/openapi.json',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                defaultModelsExpandDepth: 1,
                defaultModelExpandDepth: 1,
                docExpansion: "list",
                filter: true,
                showRequestHeaders: true,
                tryItOutEnabled: true
            });
        };
    </script>
</body>
</html>`
	
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

// GetOpenAPISpec retorna a especificação OpenAPI 3.0
func GetOpenAPISpec(c *gin.Context) {
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "Spuri Event Sourcing API",
			"version":     "3.0.0",
			"description": "Sistema de gestão acadêmica com Event Sourcing, CQRS e auditoria completa via Ledger imutável",
			"contact": map[string]string{
				"name": "Fundação Pedro Pires",
				"url":  "https://fpp.ao",
			},
		},
		"servers": []map[string]string{
			{"url": "http://localhost:8080", "description": "Desenvolvimento"},
			{"url": "https://spuri-backend.onrender.com", "description": "Produção"},
		},
		"tags": []map[string]interface{}{
			{"name": "Health", "description": "Saúde e status do sistema"},
			{"name": "Auth", "description": "Autenticação e registro"},
			{"name": "Email", "description": "Verificação de email e recuperação de senha"},
			{"name": "Estudante", "description": "Operações de estudantes"},
			{"name": "Academia", "description": "Operações de academias"},
			{"name": "Admin", "description": "Operações administrativas"},
			{"name": "Consultas", "description": "Consultas públicas e protegidas"},
			{"name": "Event Sourcing", "description": "Verificação de integridade e eventos"},
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
											"status":   map[string]string{"type": "string", "example": "ok"},
											"database": map[string]string{"type": "string", "example": "ok"},
											"service":  map[string]string{"type": "string", "example": "Spuri Event Sourcing API"},
											"version":  map[string]string{"type": "string", "example": "3.0.0"},
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
										"usuario": map[string]interface{}{
											"type":        "string",
											"description": "codigo_estudante (AAA1234) ou codigo_academia",
											"example":     "ABC1234",
										},
										"senha": map[string]interface{}{
											"type":    "string",
											"example": "ABC1234",
										},
										"type": map[string]interface{}{
											"type": "string",
											"enum": []string{"estudante", "academia"},
											"example": "estudante",
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
						"429": map[string]interface{}{"description": "Muitas tentativas (rate limit)"},
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
										"email": map[string]interface{}{
											"type":    "string",
											"example": "admin@spuri.ao",
										},
										"senha": map[string]interface{}{
											"type":    "string",
											"example": "senha123",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Login bem-sucedido"},
						"401": map[string]interface{}{"description": "Credenciais inválidas"},
						"403": map[string]interface{}{"description": "Administrador inativo"},
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
						"201": map[string]interface{}{"description": "Estudante criado com sucesso"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
					},
				},
			},
			"/academia/register": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":    []string{"Auth"},
					"summary": "Registrar academia",
					"description": "Academia inicia inativa - precisa aprovação de admin GERENTE+",
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
						"201": map[string]interface{}{"description": "Academia criada (status: inativa)"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
					},
				},
			},
			"/bootstrap/admin-fpp": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":    []string{"Auth"},
					"summary": "Criar primeiro admin FPP",
					"description": "IMPORTANTE: Só funciona se não existir nenhum admin no sistema",
					"requestBody": map[string]interface{}{
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"nome":  map[string]interface{}{"type": "string", "example": "Admin FPP"},
										"email": map[string]interface{}{"type": "string", "example": "admin@spuri.ao"},
										"senha": map[string]interface{}{"type": "string", "example": "fpp@2025"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Admin FPP criado"},
						"403": map[string]interface{}{"description": "Sistema já possui administradores"},
					},
				},
			},
			"/meu-perfil": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":     []string{"Consultas"},
					"summary":  "Ver perfil do usuário logado",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Dados do perfil (estudante/academia/admin)"},
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
							"schema":      map[string]interface{}{"type": "integer", "default": 50},
							"description": "Limite de resultados",
						},
						{
							"name":        "offset",
							"in":          "query",
							"schema":      map[string]interface{}{"type": "integer", "default": 0},
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
										"codigo_academia": map[string]interface{}{
											"type":    "string",
											"example": "LUA20241234",
										},
										"ano_escolar_inscricao": map[string]interface{}{
											"type":    "string",
											"example": "7ano",
										},
										"curso_medio": map[string]interface{}{
											"type":    "string",
											"example": "Ciências",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Inscrição criada"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
					},
				},
			},
			"/estudante/vincular-academia": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Estudante"},
					"summary":  "Vincular-se a academia (usar inscrição aprovada)",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"inscricao_id"},
									"properties": map[string]interface{}{
										"inscricao_id": map[string]string{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Vinculado com sucesso"},
						"400": map[string]interface{}{"description": "Inscrição não aprovada ou já usada"},
					},
				},
			},
			"/academia/notas-aluno": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Registrar nota individual",
					"description": "Nova estrutura v3.0 - nota por matéria",
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
										"codigo_estudante": map[string]interface{}{
											"type":    "string",
											"example": "ABC1234",
										},
										"ano_lectivo": map[string]interface{}{
											"type":    "string",
											"example": "2024/2025",
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
											"example": "1_trimestre",
										},
										"materia_disciplinar_id": map[string]interface{}{
											"type":    "string",
											"example": "uuid-da-materia",
										},
										"nota": map[string]interface{}{
											"type":    "number",
											"minimum": 0,
											"maximum": 20,
											"example": 15.5,
										},
										"observacao": map[string]interface{}{
											"type":    "string",
											"example": "Boa participação",
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
			"/academia/faltas-aluno": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Registrar falta individual",
					"description": "Nova estrutura v3.0 - falta por data/matéria",
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
										"data",
										"materia_disciplinar_id",
										"quantidade",
									},
									"properties": map[string]interface{}{
										"codigo_estudante": map[string]interface{}{
											"type":    "string",
											"example": "ABC1234",
										},
										"ano_lectivo": map[string]interface{}{
											"type":    "string",
											"example": "2024/2025",
										},
										"data": map[string]interface{}{
											"type":    "string",
											"format":  "date",
											"example": "2025-01-22",
										},
										"materia_disciplinar_id": map[string]interface{}{
											"type":    "string",
											"example": "uuid-da-materia",
										},
										"quantidade": map[string]interface{}{
											"type":    "integer",
											"minimum": 1,
											"example": 1,
										},
										"observacao": map[string]interface{}{
											"type":    "string",
											"example": "Falta justificada",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Falta registrada"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
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
										"nome": map[string]interface{}{"type": "string", "example": "Ciências Exatas"},
										"type": map[string]interface{}{
											"type":    "string",
											"enum":    []string{"medio", "superior"},
											"example": "medio",
										},
										"nivel": map[string]interface{}{
											"type":  "array",
											"items": map[string]string{"type": "string"},
											"example": []string{"primeiro_medio", "segundo_medio", "terceiro_medio"},
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Curso criado"},
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
			"/academia/materias": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Criar matéria",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"nome", "type"},
									"properties": map[string]interface{}{
										"nome": map[string]interface{}{"type": "string", "example": "Matemática"},
										"type": map[string]interface{}{
											"type":    "string",
											"enum":    []string{"fundamental", "medio", "superior"},
											"example": "medio",
										},
										"nivel": map[string]interface{}{
											"type":  "array",
											"items": map[string]string{"type": "string"},
											"description": "Apenas para fundamental",
										},
										"curso_id": map[string]interface{}{
											"type": "string",
											"description": "Obrigatório para medio/superior",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Matéria criada"},
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
							"schema": map[string]interface{}{
								"type": "string",
								"enum": []string{"notas", "faltas"},
							},
							"description": "Filtrar por tipo (opcional)",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Lista de registros"},
					},
				},
			},
			"/admin/academia/{codigo}/ativar": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":     []string{"Admin"},
					"summary":  "Ativar academia (GERENTE+)",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"parameters": []map[string]interface{}{
						{
							"name":     "codigo",
							"in":       "path",
							"required": true,
							"schema":   map[string]string{"type": "string"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Academia ativada"},
						"403": map[string]interface{}{"description": "Permissão negada"},
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
							"schema": map[string]interface{}{
								"type": "string",
								"enum": []string{"estudantes", "academias", "notas", "faltas", "all"},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Projeção reconstruída"},
					},
				},
			},
			"/verificar-integridade/{codigo}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":     []string{"Event Sourcing"},
					"summary":  "Verificar integridade do ledger",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"parameters": []map[string]interface{}{
						{
							"name":     "codigo",
							"in":       "path",
							"required": true,
							"schema":   map[string]string{"type": "string"},
							"description": "codigo_estudante",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Resultado da verificação",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"integro": map[string]string{"type": "boolean"},
											"message": map[string]string{"type": "string"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"BearerAuth": map[string]interface{}{
					"type":   "http",
					"scheme": "bearer",
					"bearerFormat": "JWT",
					"description": "JWT obtido via /login ou /admin/login",
				},
			},
			"schemas": map[string]interface{}{
				"LoginResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"token":  map[string]string{"type": "string"},
						"codigo": map[string]string{"type": "string"},
						"nome":   map[string]string{"type": "string"},
						"type":   map[string]string{"type": "string"},
					},
				},
				"RegisterEstudanteRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"nome", "senha"},
					"properties": map[string]interface{}{
						"nome":  map[string]interface{}{"type": "string", "example": "João Silva"},
						"senha": map[string]interface{}{"type": "string", "example": "senha123"},
						"email": map[string]interface{}{"type": "string", "example": "joao@email.com"},
						"telefone": map[string]interface{}{"type": "string", "example": "+244923456789"},
						"bilhete_identidade": map[string]interface{}{"type": "string", "example": "123456789LA"},
						"bilhete_identidade_responsavel": map[string]string{"type": "string"},
					},
				},
				"RegisterAcademiaRequest": map[string]interface{}{
					"type": "object",
					"required": []string{"type", "nome", "senha", "provincia", "endereco"},
					"properties": map[string]interface{}{
						"type": map[string]interface{}{
							"type":    "string",
							"enum":    []string{"escola", "superior"},
							"example": "escola",
						},
						"nome":      map[string]interface{}{"type": "string", "example": "Escola Primária ABC"},
						"senha":     map[string]interface{}{"type": "string", "example": "senha123"},
						"provincia": map[string]interface{}{"type": "string", "example": "LUA"},
						"endereco":  map[string]interface{}{"type": "string", "example": "Rua Principal, 123"},
						"email":     map[string]interface{}{"type": "string", "example": "escola@email.com"},
						"nivel_escolar": map[string]interface{}{
							"type":    "string",
							"enum":    []string{"fundamental", "medio", "misto"},
							"example": "medio",
							"description": "Obrigatório para type=escola",
						},
					},
				},
			},
		},
	}

	c.JSON(http.StatusOK, spec)
}