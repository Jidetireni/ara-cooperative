# Build stage
FROM golang:1.24.2-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy only dependency files first
COPY go.mod go.sum ./
RUN go mod download

# Install required tools
RUN go install github.com/air-verse/air@latest
# codegen and migrations are handled in the worker container

# Copy the rest of the application
COPY . .

# Final stage
FROM golang:1.23.0-alpine

WORKDIR /app

# Install necessary packages
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    git \
    curl

# Copy the Go binary and tools from builder
COPY --from=builder /go/bin/air /usr/local/bin/air
COPY --from=builder /app ./

EXPOSE 8000

ENTRYPOINT ["air", "--build.cmd", "go build -o tmp/api ./cmd/api", "--build.bin", "./tmp/api", "--build.exclude_dir", "logs"]

