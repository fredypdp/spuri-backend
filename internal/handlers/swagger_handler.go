package handlers

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

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

func SwaggerYAML(c *gin.Context) {
	// Verifica se arquivo existe
	if _, err := os.Stat("./docs/swagger.yaml"); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "swagger.yaml não encontrado",
			"path":  "./docs/swagger.yaml",
		})
		return
	}

	// Define Content-Type correto para YAML
	c.Header("Content-Type", "application/x-yaml; charset=utf-8")
	c.Header("Access-Control-Allow-Origin", "*")
	c.File("./docs/swagger.yaml")
}