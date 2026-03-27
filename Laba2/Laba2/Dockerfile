FROM golang:1.26

WORKDIR /app

COPY . .

RUN go mod tidy
RUN go build -o main .

CMD ["./main"]