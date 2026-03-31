FROM golang:1.26.1-alpine AS build

WORKDIR /src

# Proto module (shared dependency)
COPY proto/gen/go /proto/gen/go

# Node module
COPY node/go.mod node/go.sum /src/
RUN sed -i 's|../proto/gen/go|/proto/gen/go|' go.mod && \
    go mod download

COPY node/cmd ./cmd
COPY node/internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy && go build -o /out/node ./cmd/node

FROM alpine:3.23.3

ARG XRAY_VERSION=v25.3.6

WORKDIR /app

COPY --from=build /out/node /app/node

# Download Xray binary.
RUN apk add --no-cache ca-certificates curl unzip && \
    ARCH=$(uname -m) && \
    case "$ARCH" in \
      x86_64)  XRAY_ARCH="64" ;; \
      aarch64) XRAY_ARCH="arm64-v8a" ;; \
      armv7l)  XRAY_ARCH="arm32-v7a" ;; \
      *)       echo "unsupported arch: $ARCH"; exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/Xray-linux-${XRAY_ARCH}.zip" \
         -o /tmp/xray.zip && \
    unzip -q /tmp/xray.zip -d /tmp/xray && \
    mv /tmp/xray/xray /app/xray && \
    chmod +x /app/xray && \
    rm -rf /tmp/xray /tmp/xray.zip && \
    apk del curl unzip

RUN mkdir -p /app/data /app/data/xray

ENV PANEL_GRPC_ADDR=127.0.0.1:9090
ENV CLUSTER_TOKEN=
ENV NODE_NAME=
ENV SECRET_FILE=/app/data/reconnect.secret
ENV XRAY_BIN=/app/xray
ENV XRAY_CONFIG_DIR=/app/data/xray
ENV XRAY_API_ADDR=127.0.0.1:10085

CMD ["/app/node"]
