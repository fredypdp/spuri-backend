package handlers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

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
	// Tenta múltiplos caminhos possíveis
	possiblePaths := []string{
		"./docs/swagger.yaml",
		"docs/swagger.yaml",
		"../docs/swagger.yaml",
	}

	// Se existir SWAGGER_PATH, usar primeiro
	if customPath := os.Getenv("SWAGGER_PATH"); customPath != "" {
		possiblePaths = append([]string{customPath}, possiblePaths...)
	}

	var swaggerPath string
	var found bool

	for _, path := range possiblePaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}

		if _, err := os.Stat(absPath); err == nil {
			swaggerPath = absPath
			found = true
			log.Printf("[INFO] Swagger YAML encontrado em: %s", absPath)
			break
		}
	}

	if !found {
		// Log de debug para troubleshooting
		wd, _ := os.Getwd()
		log.Printf("[ERROR] swagger.yaml não encontrado. Working directory: %s", wd)
		
		c.JSON(http.StatusNotFound, gin.H{
			"error":             "swagger.yaml não encontrado",
			"working_directory": wd,
			"paths_tried":       possiblePaths,
		})
		return
	}

	c.Header("Content-Type", "application/x-yaml; charset=utf-8")
	c.Header("Access-Control-Allow-Origin", "*")
	c.File(swaggerPath)
}