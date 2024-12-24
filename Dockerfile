FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o uptimer ./cmd/main.go

FROM alpine:latest

ENV TZ=America/New_York APP_NAME=uptimer

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/uptimer .

EXPOSE 8080

ENTRYPOINT ["./uptimer"]
