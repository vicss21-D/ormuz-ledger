# Estágio 1: Build
FROM golang:1.23.5-alpine AS builder

WORKDIR /app
# Copia os arquivos de módulo e baixa dependências
COPY go.mod ./
# Se tiver go.sum, descomente a linha abaixo:
# COPY go.sum ./ 
RUN go mod download

# Copia todo o código fonte
COPY . .

# Compila o binário otimizado para não depender do Linux C-Library (CGO_ENABLED=0)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o sensor_bin ./cmd/sensor/main.go

# Estágio 2: Runner (Imagem super leve)
FROM alpine:latest

WORKDIR /root/
# Traz apenas o binário compilado do estágio anterior
COPY --from=builder /app/sensor_bin .

# Define o fuso horário (opcional, bom para logs)
RUN apk add --no-cache tzdata
ENV TZ=America/Sao_Paulo

# Executa
CMD ["./sensor_bin"]