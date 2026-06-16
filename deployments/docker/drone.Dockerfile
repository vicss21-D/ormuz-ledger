FROM golang:1.23.5-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# O Drone é composto por apenas um ficheiro main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o drone_bin ./cmd/drone/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/drone_bin .
# O Drone não expõe portas, atua apenas como cliente ativo (Light Client)
CMD ["./drone_bin"]