FROM golang:1.25.2-alpine AS builder

RUN apk add --no-cache git
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.26.0

COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -buildvcs=false -o /out/owner ./cmd/owner

FROM alpine:3

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /out/api /app/api
COPY --from=builder /out/owner /app/owner
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY --from=builder /app/migrations/schema /app/migrations/schema

ENV GOOSE_DRIVER=postgres \
    GOOSE_MIGRATION_DIR=/app/migrations/schema

EXPOSE 5001

# Bootstrap only when both credentials are configured; existing passwords are preserved.
# A migration or bootstrap failure stops startup.
ENTRYPOINT ["sh", "-c", "goose up && if [ -n \"$OWNER_EMAIL\" ] && [ -n \"$OWNER_PASSWORD\" ]; then /app/owner; fi && exec /app/api"]
