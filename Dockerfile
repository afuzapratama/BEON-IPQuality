# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

ARG VERSION=dev
ARG BUILD_TIME=unknown

WORKDIR /src
RUN apk add --no-cache ca-certificates git tzdata

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
      -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
      -o /out/ingestor ./cmd/ingestor && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/compiler ./cmd/compiler && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/judge ./cmd/judge

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S -g 10001 beon && \
    adduser -S -D -H -u 10001 -G beon beon && \
    mkdir -p /app/configs /app/data/mmdb /app/logs && \
    chown -R beon:beon /app
WORKDIR /app
COPY --from=builder --chown=beon:beon /src/configs /app/configs
USER beon

FROM runtime AS api
COPY --from=builder --chown=beon:beon /out/api /app/api
EXPOSE 8080
ENTRYPOINT ["/app/api"]
CMD ["-config", "/app/configs/config.yaml"]

FROM runtime AS ingestor
COPY --from=builder --chown=beon:beon /out/ingestor /app/ingestor
ENTRYPOINT ["/app/ingestor"]
CMD ["-config", "/app/configs/config.yaml", "-feeds", "/app/configs/feeds.yaml"]

FROM runtime AS compiler
COPY --from=builder --chown=beon:beon /out/compiler /app/compiler
ENTRYPOINT ["/app/compiler"]
CMD ["-config", "/app/configs/config.yaml"]

FROM runtime AS judge
COPY --from=builder --chown=beon:beon /out/judge /app/judge
EXPOSE 8081
ENTRYPOINT ["/app/judge"]
CMD ["-config", "/app/configs/config.yaml"]
