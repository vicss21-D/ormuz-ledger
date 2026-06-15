# Estágio de Build
FROM golang:1.23.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Compilando o Broker
RUN go build -o broker_bin ./cmd/broker

# Estágio de Execução
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/broker_bin .

EXPOSE 9000/udp 8080/tcp

CMD ["/root/broker_bin"]