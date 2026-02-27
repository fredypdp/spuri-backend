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
			"version":     "3.1.0",
			"description": "Sistema de gestão académica com Event Sourcing, CQRS e auditoria completa via Ledger imutável.\n\n**v3.1 – Atualização de matérias e registro de notas/faltas:**\n- `anos_academicos` em matérias disciplinares: fundamental (1–9 anos), médio/superior (exatamente 1 ano).\n- `ano_academico` nos registros de nota e falta é inferido automaticamente pelo back end.",
			"contact": map[string]string{
				"name": "Fundação Pedro Pires",
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"BearerAuth": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
		},
		"paths": map[string]interface{}{
			// ================================================================
			// NOTAS — ano_academico REMOVIDO do request (inferido pelo back end)
			// ================================================================
			"/academia/notas-aluno": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Registrar nota individual",
					"description": "Registra a nota de um estudante em uma matéria.\n\n**ano_academico é inferido automaticamente:**\n- Estudante no fundamental → usa o ano atual do estudante (`ano_escolar`)\n- Estudante no médio/superior → usa o único `anos_academicos[0]` da matéria",
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
										"tipo",
										"categoria",
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
											"type":        "string",
											"format":      "uuid",
											"example":     "uuid-da-materia",
										},
										"tipo": map[string]interface{}{
											"type":    "string",
											"enum":    []string{"escolar", "superior"},
											"example": "escolar",
										},
										"categoria": map[string]interface{}{
											"type":    "string",
											"example": "nota_escola",
											"description": "escolar: nota_escola | nota_professor. superior: nota_pp1 | nota_pp2 | nota_exame | categorias adicionais",
										},
										"nota": map[string]interface{}{
											"type":    "number",
											"minimum": 0,
											"maximum": 20,
											"example": 15.5,
										},
										"observacao": map[string]interface{}{
											"type":        "string",
											"example":     "Boa participação",
											"description": "Opcional",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Nota registrada",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message":       map[string]string{"type": "string"},
											"ano_academico": map[string]interface{}{
												"type":        "string",
												"description": "Ano académico inferido pelo back end",
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Dados inválidos"},
						"403": map[string]interface{}{"description": "Academia inativa ou estudante de outra academia"},
					},
				},
			},

			// ================================================================
			// FALTAS — ano_academico REMOVIDO do request (inferido pelo back end)
			// ================================================================
			"/academia/faltas-aluno": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Registrar falta individual",
					"description": "Registra falta de um estudante em uma matéria.\n\n**ano_academico é inferido automaticamente** (mesma regra das notas).",
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
											"type":   "string",
											"format": "uuid",
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
						"201": map[string]interface{}{"description": "Falta registrada"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
					},
				},
			},

			// ================================================================
			// CURSOS
			// ================================================================
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
									"required": []string{"nome", "type", "anos_academicos"},
									"properties": map[string]interface{}{
										"nome": map[string]interface{}{"type": "string", "example": "Ciências Exatas"},
										"type": map[string]interface{}{
											"type":    "string",
											"enum":    []string{"medio", "superior"},
											"example": "medio",
										},
										"anos_academicos": map[string]interface{}{
											"type":        "array",
											"items":       map[string]string{"type": "string"},
											"example":     []string{"primeiro_ano", "segundo_ano", "terceiro_ano"},
											"description": "Anos do curso (livres, definidos pela academia)",
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

			// ================================================================
			// MATÉRIAS — ATUALIZAÇÃO 1: campo "anos_academicos", regras por tipo
			// ================================================================
			"/academia/materias": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Criar matéria disciplinar",
					"description": "Cria uma matéria disciplinar.\n\n**Regras para `anos_academicos` (Atualização 1):**\n- `fundamental`: array com **1 a 9** anos (`primeiro_fundamental`…`nono_fundamental`). A matéria pode ser lecionada em múltiplos anos.\n- `medio` ou `superior`: array com **exatamente 1** item — o ano do curso ao qual a matéria pertence. O valor deve existir em `curso.anos_academicos`.",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"nome", "type", "anos_academicos"},
									"properties": map[string]interface{}{
										"nome": map[string]interface{}{
											"type":    "string",
											"example": "Matemática",
										},
										"type": map[string]interface{}{
											"type":    "string",
											"enum":    []string{"fundamental", "medio", "superior"},
											"example": "medio",
										},
										"anos_academicos": map[string]interface{}{
											"type":  "array",
											"items": map[string]string{"type": "string"},
											"description": "Fundamental: 1–9 anos (primeiro_fundamental…nono_fundamental). Médio/Superior: exatamente 1 item (ano do curso).",
											"example": []string{"primeiro_medio"},
										},
										"curso_id": map[string]interface{}{
											"type":        "string",
											"format":      "uuid",
											"description": "Obrigatório para medio/superior. O ano em anos_academicos deve pertencer ao curso.",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Matéria criada"},
						"400": map[string]interface{}{"description": "Dados inválidos (ex.: mais de 1 ano para médio/superior)"},
					},
				},
				"get": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Listar matérias da academia",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Lista de matérias"},
					},
				},
			},

			// ================================================================
			// ATUALIZAR NOTA
			// ================================================================
			"/academia/atualizar-nota": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":     []string{"Academia"},
					"summary":  "Corrigir nota existente",
					"security": []map[string][]string{{"BearerAuth": {}}},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"id", "nota_nova", "observacao"},
									"properties": map[string]interface{}{
										"id":         map[string]interface{}{"type": "string", "format": "uuid"},
										"nota_nova":  map[string]interface{}{"type": "number", "minimum": 0, "maximum": 20},
										"observacao": map[string]interface{}{"type": "string", "description": "Justificativa obrigatória"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Nota atualizada"},
						"400": map[string]interface{}{"description": "Dados inválidos"},
					},
				},
			},

			// ================================================================
			// ADMIN
			// ================================================================
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

			// ================================================================
			// EVENT SOURCING
			// ================================================================
			"/verificar-integridade/{codigo}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Event Sourcing"},
					"summary":     "Verificar integridade do ledger",
					"security":    []map[string][]string{{"BearerAuth": {}}},
					"parameters": []map[string]interface{}{
						{
							"name":        "codigo",
							"in":          "path",
							"required":    true,
							"schema":      map[string]string{"type": "string"},
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
	}

	c.JSON(http.StatusOK, spec)
}