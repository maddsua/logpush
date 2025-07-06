from golang:1.23.2-alpine3.20 as builder

workdir /app

copy . .

run go mod download
run go build -v -o logpush ./cmd

from alpine:latest

copy --from=builder /app/logpush /usr/bin/logpush
copy ./cmd/logpush.yml /etc/mws/logpush/logpush.yml

run apk add --no-cache ca-certificates

entrypoint ["/usr/bin/logpush"]
