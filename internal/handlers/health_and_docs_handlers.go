// ============================================================================
// ARQUIVO: internal/handlers/health_and_docs_handlers.go (NOVO)
// Handlers para health check e documentação da API
// ============================================================================

package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthCheck verifica saúde da aplicação
func HealthCheck(c *gin.Context) {
	client := getDbClient(c)

	// Testar conexão com banco
	dbStatus := "ok"
	if err := client.Health(); err != nil {
		dbStatus = "error: " + err.Error()
	}

	// Estatísticas do banco
	stats := client.Stats()

	response := gin.H{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "Spuri Event Sourcing API",
		"version":   "3.0.0",
		"database": gin.H{
			"status":            dbStatus,
			"open_connections":  stats.OpenConnections,
			"in_use":            stats.InUse,
			"idle":              stats.Idle,
			"max_open_allowed":  stats.MaxOpenConnections,
		},
		"encoding": "UTF-8",
	}

	c.JSON(http.StatusOK, response)
}

// APIDocumentation retorna documentação da API
func APIDocumentation(c *gin.Context) {
	docs := gin.H{
		"service": "Spuri Event Sourcing API",
		"version": "3.0.0",
		"description": "Sistema de gestão acadêmica com Event Sourcing e CQRS",
		"encoding": "UTF-8",
		"features": []string{
			"Event Sourcing com PostgreSQL",
			"CQRS com Projeções",
			"Auditoria completa via Ledger",
			"Rate Limiting",
			"JWT Authentication",
			"Verificação de Email",
			"Recuperação de Senha",
		},
		
		"endpoints": gin.H{
			"health": gin.H{
				"GET /health": "Verificar saúde da aplicação",
			},
			
			"authentication": gin.H{
				"POST /login":                        "Login estudante/academia",
				"POST /admin/login":                  "Login administrador",
				"POST /estudante/register":           "Registrar estudante",
				"POST /academia/register":            "Registrar academia",
				"POST /bootstrap/admin-fpp":          "Criar primeiro admin (apenas se não existir nenhum)",
			},
			
			"email": gin.H{
				"GET  /verificar-email/:token":       "Verificar email com token",
				"POST /recuperar-senha/solicitar":    "Solicitar recuperação de senha",
				"POST /recuperar-senha/:token":       "Resetar senha com token",
				"PUT  /alterar-senha":                "Alterar senha (autenticado)",
			},
			
			"profile": gin.H{
				"GET /meu-perfil": "Ver perfil do usuário logado",
			},
			
			"academias": gin.H{
				"GET /academias":                     "Listar todas academias (público)",
				"GET /consultar-academia/:codigo":    "Consultar academia por código",
			},
			
			"estudantes": gin.H{
				"GET /estudantes":                    "Listar estudantes (academia: próprios / admin: todos)",
				"GET /consultar-estudante/:codigo":   "Consultar estudante por código",
			},
			
			"inscricoes": gin.H{
				"GET /inscricoes":                    "Listar inscrições (filtradas por tipo de usuário)",
				"GET /inscricoes-pendentes":          "Listar inscrições pendentes",
			},
			
			"notas_e_faltas": gin.H{
				"GET /notas-estudante/:codigo":      "Ver notas de estudante",
				"GET /faltas-estudante/:codigo":     "Ver faltas de estudante",
				"GET /historico-estudante/:codigo":  "Ver histórico completo",
			},
			
			"event_sourcing": gin.H{
				"GET /eventos-estudante/:codigo":    "Ver eventos do estudante (Event Sourcing)",
				"GET /verificar-integridade/:codigo": "Verificar integridade do ledger",
			},
			
			"estudante_actions": gin.H{
				"GET  /estudante/minhas-inscricoes":        "Ver minhas inscrições",
				"GET  /estudante/meu-historico":            "Ver meu histórico",
				"GET  /estudante/inscricoes-aprovadas":     "Ver inscrições aprovadas não usadas",
				"POST /estudante/inscricao-escola":         "Solicitar inscrição em escola",
				"POST /estudante/inscricao-universidade":   "Solicitar inscrição em universidade",
				"POST /estudante/vincular-academia":        "Vincular-se a academia (usar inscrição aprovada)",
				"PUT  /estudante/status-escolar":           "Atualizar status escolar",
				"PUT  /estudante/status-superior":          "Atualizar status superior",
				"PUT  /estudante/dados-pessoais":           "Atualizar dados pessoais",
				"PUT  /estudante/dados-academicos":         "Atualizar dados acadêmicos",
			},
			
			"academia_actions": gin.H{
				"POST /academia/notas-aluno":               "Registrar notas de aluno",
				"POST /academia/faltas-aluno":              "Registrar faltas de aluno",
				"PUT  /academia/inscricao/:id/aprovar":     "Aprovar inscrição",
				"PUT  /academia/inscricao/:id/reprovar":    "Reprovar inscrição",
				"POST /academia/cursos":                    "Criar curso",
				"GET  /academia/cursos":                    "Listar cursos",
				"PUT  /academia/cursos/:id":                "Atualizar curso",
				"PUT  /academia/cursos/:id/ativar":         "Ativar curso",
				"PUT  /academia/cursos/:id/desativar":      "Desativar curso",
				"POST /academia/materias":                  "Criar matéria",
				"GET  /academia/materias":                  "Listar matérias",
				"PUT  /academia/materias/:id":              "Atualizar matéria",
				"PUT  /academia/dados":                     "Atualizar dados da academia",
			},
			
			"admin_queries": gin.H{
				"GET /admin/todos-registros":               "Listar todos registros (notas/faltas)",
				"GET /admin/registros/estudante/:codigo":   "Registros de um estudante",
				"GET /admin/registros/academia/:codigo":    "Registros de uma academia",
				"GET /admin/consultar-admin/:email":        "Consultar admin por email",
				"GET /admin/buscar-usuario":                "Buscar usuário (query params: tipo, id)",
			},
			
			"admin_management": gin.H{
				"POST /admin/register":                     "Criar admin (ADM+)",
				"GET  /admin/admins":                       "Listar admins (ADM+)",
				"PUT  /admin/dados/:id":                    "Atualizar dados admin",
				"PUT  /admin/admin/:id/ativar":             "Ativar admin (FPP)",
				"PUT  /admin/admin/:id/desativar":          "Desativar admin (FPP)",
				"PUT  /admin/role/:id":                     "Atualizar role admin (FPP)",
				"PUT  /admin/academia/:codigo/ativar":      "Ativar academia (GERENTE+)",
				"PUT  /admin/academia/:codigo/desativar":   "Desativar academia (GERENTE+)",
			},
			
			"admin_projections": gin.H{
				"POST /admin/rebuild-projection/:name":     "Reconstruir projeção",
				"GET  /admin/projection-status/:name":      "Status de projeção",
				"GET  /admin/projections-status":           "Status de todas projeções",
				"GET  /admin/ledger-stats":                 "Estatísticas do ledger",
				"GET  /admin/verify-all-integrity":         "Verificar integridade geral",
			},
		},
		
		"authentication": gin.H{
			"type": "JWT Bearer Token",
			"header": "Authorization: Bearer <token>",
			"how_to_get_token": gin.H{
				"estudante": "POST /login com {usuario, senha, type: 'estudante'}",
				"academia":  "POST /login com {usuario, senha, type: 'academia'}",
				"admin":     "POST /admin/login com {email, senha}",
			},
		},
		
		"user_types": gin.H{
			"estudante": gin.H{
				"description": "Estudante do sistema",
				"identifier":  "codigo_estudante (7 caracteres: AAA1234)",
				"default_password": "Mesmo que codigo_estudante",
			},
			"academia": gin.H{
				"description": "Escola ou Universidade",
				"identifier":  "codigo_academia (gerado automaticamente)",
				"default_password": "Mesmo que codigo_academia",
				"status": "Inativa ao criar - precisa aprovação de admin GERENTE+",
			},
			"admin": gin.H{
				"description": "Administrador do sistema",
				"roles": gin.H{
					"fpp":     "Fundação Pedro Pires - Máxima permissão",
					"adm":     "Administrador - Pode criar outros admins",
					"gerente": "Gerente - Pode ativar/desativar academias",
				},
				"hierarchy": "fpp > adm > gerente",
			},
		},
		
		"event_sourcing": gin.H{
			"description": "Sistema usa Event Sourcing completo",
			"benefits": []string{
				"Auditoria completa de todas operações",
				"Histórico imutável de eventos",
				"Possibilidade de rebuild de projeções",
				"Integridade verificável via hash chain",
				"Rastreamento de mudanças ao longo do tempo",
			},
			"projections": []string{
				"estudantes", "academias", "admins",
				"notas", "faltas", "inscricoes",
				"cursos", "materias",
			},
		},
		
		"email_service": gin.H{
			"status": "Opcional - funciona sem SMTP configurado",
			"features": []string{
				"Verificação de email (se SMTP configurado)",
				"Recuperação de senha",
			},
			"note": "Se email não configurado, sistema usa senha padrão na recuperação",
		},
		
		"rate_limiting": gin.H{
			"global":   "100 req/min por IP",
			"login":    "5 req/min por IP",
			"email":    "2 req/hora por IP",
		},
		
		"contact": gin.H{
			"organization": "Fundação Pedro Pires",
			"country":      "Angola",
		},
	}

	c.JSON(http.StatusOK, docs)
}