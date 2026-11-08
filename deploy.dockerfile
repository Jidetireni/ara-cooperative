FROM golang:1.25.2-alpine AS builder

RUN apk add --no-cache git
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.26.0

COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -o /out/api ./cmd/api

FROM alpine:3

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /out/api /app/api
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY --from=builder /app/migrations/schema /app/migrations/schema

ENV GOOSE_DRIVER=postgres \
    GOOSE_MIGRATION_DIR=/app/migrations/schema

EXPOSE 5001

# Database credentials are supplied at runtime. Failed migrations stop startup.
ENTRYPOINT ["sh", "-c", "goose up && exec /app/api"]
