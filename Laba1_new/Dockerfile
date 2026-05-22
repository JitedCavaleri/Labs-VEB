# Этап сборки
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod .

COPY cmd ./cmd

RUN go build -o app ./cmd/main.go

# Финальный контейнер
FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/app .

EXPOSE 4200

CMD ["./app"]