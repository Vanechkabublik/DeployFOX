FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download || go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 go build -trimpath -o /out/agent ./cmd/agent

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 deployfox
COPY --from=build /out/api /out/worker /usr/local/bin/
USER deployfox
EXPOSE 8080
CMD ["api"]

FROM docker:29-cli AS agent
RUN apk add --no-cache ca-certificates git
ARG TARGETARCH
RUN set -eu; \
    case "$TARGETARCH" in \
      amd64) arch=x86_64; sum=248a8f60f36854aabed6124113631078a802de3461660855fc4de9c26065c1fb ;; \
      arm64) arch=arm64; sum=732ece14eb4f4676a43ebba19431f954edf7b130acb76159ef0f2ff86ac42ac9 ;; \
      *) exit 1 ;; \
    esac; \
    wget -q "https://github.com/railwayapp/railpack/releases/download/v0.43.0/railpack-v0.43.0-${arch}-unknown-linux-musl.tar.gz" -O /tmp/railpack.tgz; \
    echo "$sum  /tmp/railpack.tgz" | sha256sum -c -; \
    tar xzf /tmp/railpack.tgz -C /usr/local/bin railpack; \
    rm /tmp/railpack.tgz
COPY --from=build /out/agent /usr/local/bin/agent
RUN mkdir -p /work /routes
EXPOSE 8090
CMD ["agent"]
