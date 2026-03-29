FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .
RUN go build -o telegram_service .

FROM alpine:3.21

WORKDIR /app
COPY --from=builder /app/telegram_service .

VOLUME ["/telegram_service"]

CMD ["./telegram_service"]
