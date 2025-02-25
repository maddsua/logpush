FROM golang:1.23-alpine3.21 AS builder

WORKDIR /app

COPY . .

RUN go mod download
RUN go build -v -o logpush-service

FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/logpush-service ./

ENTRYPOINT ["./logpush-service"]
