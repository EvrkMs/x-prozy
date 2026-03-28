FROM golang:1.26.1-alpine AS build

WORKDIR /src

# Proto module (shared dependency)
COPY proto/gen/go /proto/gen/go

# Panel module
COPY panel/panel-backend/go.mod panel/panel-backend/go.sum ./
RUN sed -i 's|../../proto/gen/go|/proto/gen/go|' go.mod && \
    go mod download

COPY panel/panel-backend/cmd ./cmd
COPY panel/panel-backend/internal ./internal
COPY panel/panel-backend/web ./web

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy && go build -o /out/panel ./cmd/panel

FROM alpine:3.23.3

WORKDIR /app

COPY --from=build /out/panel /app/panel
COPY --from=build /src/web /app/web

RUN mkdir -p /app/data

EXPOSE 8080
EXPOSE 9090

ENV PANEL_ADDR=0.0.0.0
ENV PANEL_PORT=8080
ENV GRPC_ADDR=0.0.0.0
ENV GRPC_PORT=9090
ENV DB_PATH=/app/data/panel.db
ENV CLUSTER_TOKEN=

CMD ["/app/panel"]
