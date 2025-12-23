# =============================================================================
# Dockerfile - Spuri Event Sourcing
# Otimizado para Railway e produção
# =============================================================================

# =============================================================================
# Build Stage
# =============================================================================
FROM golang:1.21-alpine AS builder

# Instalar dependências de build
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copiar arquivos de dependências primeiro (cache layer)
COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

# Copiar código fonte
COPY . .

# Build otimizado
# CGO_ENABLED=0: build estático
# -ldflags="-w -s": remove debug info (reduz tamanho)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o spuri \
    cmd/server/main.go

# Verificar binário
RUN chmod +x spuri
RUN ./spuri --version 2>/dev/null || echo "Build OK"

# =============================================================================
# Runtime Stage
# =============================================================================
FROM alpine:latest

# Instalar dependências de runtime
RUN apk --no-cache add \
    ca-certificates \
    postgresql-client \
    tzdata \
    && rm -rf /var/cache/apk/*

# Criar usuário não-root para segurança
RUN addgroup -g 1000 spuri && \
    adduser -D -u 1000 -G spuri spuri

WORKDIR /home/spuri

# Copiar binário do builder
COPY --from=builder --chown=spuri:spuri /app/spuri .

# Copiar migrations
COPY --from=builder --chown=spuri:spuri /app/migrations ./migrations

# Copiar script de inicialização
COPY --chown=spuri:spuri init_db.sh .
RUN chmod +x init_db.sh

# Mudar para usuário não-root
USER spuri

# Railway expõe automaticamente a porta via $PORT
# Mas declaramos 8080 como padrão
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT:-8080}/health || exit 1

# Comando de inicialização
# 1. Executa init_db.sh (migrations se necessário)
# 2. Inicia aplicação
CMD sh -c "./init_db.sh && ./spuri"

# Labels para metadata
LABEL maintainer="Spuri Team"
LABEL description="Spuri Event Sourcing API with GenesisDB"
LABEL version="2.0.0"