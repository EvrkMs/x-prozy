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

WORKDIR /app

COPY --from=build /out/node /app/node

RUN mkdir -p /app/data

ENV PANEL_GRPC_ADDR=127.0.0.1:9090
ENV CLUSTER_TOKEN=
ENV NODE_NAME=
ENV SECRET_FILE=/app/data/reconnect.secret

CMD ["/app/node"]
