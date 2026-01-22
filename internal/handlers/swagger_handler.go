package handlers

import (
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func SwaggerUI(c *gin.Context) {
	// Ler o arquivo swagger.yaml
	swaggerPath := "docs/swagger.yaml"
	absPath, _ := filepath.Abs(swaggerPath)
	
	yamlContent, err := ioutil.ReadFile(absPath)
	if err != nil {
		log.Printf("[ERROR] Não conseguiu ler swagger.yaml: %v", err)
		c.String(http.StatusInternalServerError, "Erro ao carregar documentação")
		return
	}

	html := `
<!DOCTYPE html>
<html lang="pt">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Spuri API - Documentação</title>
    <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.10.5/swagger-ui.min.css">
    <style>
        body { margin: 0; padding: 0; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.10.5/swagger-ui-bundle.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.10.5/swagger-ui-standalone-preset.min.js"></script>
    <script>
        window.onload = function() {
            const spec = ` + "`" + string(yamlContent) + "`" + `;
            SwaggerUIBundle({
                spec: jsyaml.load(spec),
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
    <script src="https://cdnjs.cloudflare.com/ajax/libs/js-yaml/4.1.0/js-yaml.min.js"></script>
</body>
</html>
`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

func SwaggerYAML(c *gin.Context) {
	swaggerPath := "docs/swagger.yaml"
	absPath, _ := filepath.Abs(swaggerPath)
	
	c.Header("Content-Type", "application/x-yaml; charset=utf-8")
	c.Header("Access-Control-Allow-Origin", "*")
	c.File(absPath)
}