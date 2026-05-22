FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# Копируем только go.mod (go.sum может отсутствовать — это нормально).
COPY go.mod ./
COPY go.sum* ./

# Подтягиваем зависимости и формируем актуальный go.sum.
RUN go mod tidy

# Устанавливаем swag CLI (генератор OpenAPI спецификации из аннотаций Go-кода).
# Версия зафиксирована для воспроизводимости сборки.
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.3

COPY . .

# Генерируем актуальную OpenAPI спецификацию из аннотаций кода.
# Файлы появляются в ./docs (docs.go, swagger.json, swagger.yaml) и перезаписывают stub.
# Флаги:
#   -g main.go              — стартовая точка с глобальными @-аннотациями
#   --parseDependency       — парсить типы из зависимых пакетов (dto, models)
#   --parseInternal         — включать internal-пакеты
#   -o ./docs               — куда складывать сгенерированные файлы
RUN /go/bin/swag init -g main.go --parseDependency --parseInternal -o ./docs

# Ещё один tidy на случай новых импортов после COPY, затем сборка.
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o server .

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 4200
CMD ["./server"]
