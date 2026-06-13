FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY . .

RUN go mod download
RUN go mod verify

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

# 🔥 INICIAR DIRETAMENTE - Migrations rodadas pelo Go
CMD ["./spuri"]

LABEL maintainer="Spuri Team"
LABEL description="Spuri Event Sourcing API"
LABEL version="2.0.2"
