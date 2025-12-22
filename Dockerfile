# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copiar arquivos de dependências
COPY go.mod go.sum ./
RUN go mod download

# Copiar todo o código
COPY . .

# Compilar o binário
RUN CGO_ENABLED=0 GOOS=linux go build -o spuri cmd/server/main.go

# Runtime stage
FROM alpine:latest

# Instalar CA certificates (para HTTPS) e PostgreSQL client
RUN apk --no-cache add ca-certificates postgresql-client

WORKDIR /root/

# Copiar binário compilado e migrations
COPY --from=builder /app/spuri .
COPY --from=builder /app/migrations ./migrations

# Expor porta (Railway usa variável PORT)
EXPOSE 8080

# Comando para iniciar
CMD ["./spuri"]