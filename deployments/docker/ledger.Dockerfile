# deployments/docker/ledger.Dockerfile
FROM golang:1.23.5-alpine AS builder

WORKDIR /app

# Otimização de cache para os módulos
COPY go.mod go.sum ./
RUN go mod download

# Copia o código fonte
COPY . .

# Compila estaticamente o Ledger App
RUN CGO_ENABLED=0 GOOS=linux go build -o ledger_bin ./cmd/ledger/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/ledger_bin .

# Expondo a porta padrão do ABCI
EXPOSE 26658

CMD ["./ledger_bin", "-addr=tcp://0.0.0.0:26658"]