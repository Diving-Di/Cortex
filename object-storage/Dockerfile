ARG GO_BUILDER_IMAGE=golang:1.26.6-alpine
ARG ALPINE_RUNTIME_IMAGE=alpine:3.23

FROM ${GO_BUILDER_IMAGE} AS source-builder
RUN apk add --no-cache git ca-certificates
ENV CGO_ENABLED=0 GOMAXPROCS=2

# Official RELEASE.2025-07-21T05-28-08Z, pinned to its source commit.
FROM source-builder AS mc-builder
WORKDIR /src/mc
RUN git init && git remote add origin https://github.com/minio/mc.git \
    && git fetch --depth=1 origin 82c5d2530428bea3897490e7030004ee3b011e3a \
    && git checkout --detach FETCH_HEAD \
    && go build -p=2 -trimpath -ldflags="-s -w" -o /out/mc .

# Official RELEASE.2025-07-23T15-54-02Z, pinned to its source commit.
FROM source-builder AS minio-builder
WORKDIR /src/minio
RUN git init && git remote add origin https://github.com/minio/minio.git \
    && git fetch --depth=1 origin ee211f1b5dbd8f4c4d4e3b8be1d4aa37eef9977e \
    && git checkout --detach FETCH_HEAD \
    && go build -p=2 -trimpath -ldflags="-s -w" -o /out/minio .

FROM ${ALPINE_RUNTIME_IMAGE} AS runtime
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates tzdata
COPY --from=mc-builder /out/mc /usr/local/bin/mc
COPY --from=mc-builder /src/mc/LICENSE /usr/share/licenses/mc/LICENSE

FROM runtime AS client
LABEL org.opencontainers.image.source="https://github.com/minio/mc" \
      org.opencontainers.image.revision="82c5d2530428bea3897490e7030004ee3b011e3a" \
      org.opencontainers.image.licenses="AGPL-3.0-only"
ENTRYPOINT ["mc"]

FROM runtime AS server
LABEL org.opencontainers.image.source="https://github.com/minio/minio" \
      org.opencontainers.image.revision="ee211f1b5dbd8f4c4d4e3b8be1d4aa37eef9977e" \
      org.opencontainers.image.licenses="AGPL-3.0-only"
COPY --from=minio-builder /out/minio /usr/local/bin/minio
COPY --from=minio-builder /src/minio/LICENSE /usr/share/licenses/minio/LICENSE
EXPOSE 9000 9001
ENTRYPOINT ["minio"]
CMD ["server", "/data"]
