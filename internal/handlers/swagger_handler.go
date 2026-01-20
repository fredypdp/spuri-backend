// ============================================================================
// ARQUIVO: internal/handlers/swagger_handler.go
// Handler para servir documentação Swagger
// ============================================================================

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SwaggerUI serve a interface Swagger
func SwaggerUI(c *gin.Context) {
	html := `
<!DOCTYPE html>
<html lang="pt">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Spuri API - Documentação</title>
    <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.10.5/swagger-ui.min.css">
    <style>
        body {
            margin: 0;
            padding: 0;
        }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.10.5/swagger-ui-bundle.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.10.5/swagger-ui-standalone-preset.min.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: '/api-docs/swagger.yaml',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
            });
        };
    </script>
</body>
</html>
`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

// SwaggerYAML serve o arquivo YAML
func SwaggerYAML(c *gin.Context) {
	c.File("./docs/swagger.yaml")
}