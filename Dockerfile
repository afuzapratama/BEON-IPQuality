# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build API server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/api ./cmd/api

# Build Ingestor
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/ingestor ./cmd/ingestor

# Build Compiler
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/compiler ./cmd/compiler

# Build Judge
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/judge ./cmd/judge

# Final stage - API
FROM alpine:3.19 AS api

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary
COPY --from=builder /bin/api /app/api
COPY --from=builder /app/configs /app/configs

# Create data directory
RUN mkdir -p /app/data/mmdb /app/logs

EXPOSE 8080

CMD ["/app/api", "-config", "/app/configs/config.yaml"]

# Final stage - Ingestor
FROM alpine:3.19 AS ingestor

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/ingestor /app/ingestor
COPY --from=builder /app/configs /app/configs

RUN mkdir -p /app/data /app/logs

CMD ["/app/ingestor", "-config", "/app/configs/config.yaml", "-feeds", "/app/configs/feeds.yaml"]

# Final stage - Compiler
FROM alpine:3.19 AS compiler

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/compiler /app/compiler
COPY --from=builder /app/configs /app/configs

RUN mkdir -p /app/data/mmdb /app/logs

CMD ["/app/compiler", "-config", "/app/configs/config.yaml"]

# Final stage - Judge
FROM alpine:3.19 AS judge

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/judge /app/judge
COPY --from=builder /app/configs /app/configs

RUN mkdir -p /app/data/mmdb /app/logs

EXPOSE 8081

CMD ["/app/judge", "-config", "/app/configs/config.yaml"]
