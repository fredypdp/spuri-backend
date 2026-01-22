# =============================================================================
# Dockerfile - Spuri Event Sourcing
# Otimizado para Railway/Render - SEM init_db.sh
# =============================================================================

# =============================================================================
# Build Stage
# =============================================================================
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o spuri \
    cmd/server/main.go

RUN chmod +x spuri

# =============================================================================
# Runtime Stage
# =============================================================================
FROM alpine:latest

RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

RUN addgroup -g 1000 spuri && \
    adduser -D -u 1000 -G spuri spuri

WORKDIR /home/spuri

# Copiar binário
COPY --from=builder --chown=spuri:spuri /app/spuri .

# Copiar migrations (serão executadas pela aplicação Go)
COPY --from=builder --chown=spuri:spuri /app/migrations ./migrations

USER spuri

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT:-8080}/health || exit 1

# 🔥 INICIAR DIRETAMENTE - Migrations rodadas pelo Go
CMD ["./spuri"]

LABEL maintainer="Spuri Team"
LABEL description="Spuri Event Sourcing API"
LABEL version="2.0.0"