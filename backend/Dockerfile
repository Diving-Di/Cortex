ARG GO_BUILDER_IMAGE=golang:1.26.6-alpine
ARG ALPINE_RUNTIME_IMAGE=alpine:3.23
FROM ${GO_BUILDER_IMAGE} AS builder

WORKDIR /src

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/rag-eval ./cmd/rag-eval
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/blob-migrate ./cmd/blob-migrate

FROM ${ALPINE_RUNTIME_IMAGE}

LABEL org.opencontainers.image.title="Cortex Backend" \
      org.opencontainers.image.description="Personal notes and knowledge service"

RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates chromium tzdata su-exec \
    && addgroup -S cortex \
    && adduser -S -G cortex -h /app cortex

WORKDIR /app
COPY --from=builder /out/server /app/server
COPY --from=builder /out/migrate /app/migrate
COPY --from=builder /out/rag-eval /app/rag-eval
COPY --from=builder /out/blob-migrate /app/blob-migrate
COPY --from=builder /src/testdata/rag /app/testdata/rag
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh

EXPOSE 8000

HEALTHCHECK --interval=10s --timeout=3s --retries=6 \
    CMD wget -q -O - http://127.0.0.1:8000/healthz >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
