FROM golang:1.23.5-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o station_bin ./cmd/station/main.go ./cmd/station/http_server.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/station_bin .
EXPOSE 8081
CMD ["./station_bin"]