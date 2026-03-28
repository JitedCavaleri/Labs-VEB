FROM golang:1.26-alpine

WORKDIR /app

# Копируем только go.mod и go.sum сначала
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь код
COPY . .

# Собираем приложение
RUN go build -o main .

CMD ["./main"]