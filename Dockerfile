# Multi-stage PathWeaver image (controller + static UI)
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/pathweaver-controller ./cmd/pathweaver-controller \
 && CGO_ENABLED=0 go build -o /out/pathweaver-agent ./cmd/pathweaver-agent \
 && CGO_ENABLED=0 go build -o /out/pathweaver-netd ./cmd/pathweaver-netd \
 && CGO_ENABLED=0 go build -o /out/pathweaver-updater ./cmd/pathweaver-updater

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wireguard-tools iproute2 nftables \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /opt/pathweaver
COPY --from=build /out/ /opt/pathweaver/bin/
COPY --from=web /web/dist/ /opt/pathweaver/web/
ENV PW_STATIC_DIR=/opt/pathweaver/web \
    PW_DATA_DIR=/opt/pathweaver/data \
    PW_DB_PATH=/opt/pathweaver/data/pathweaver.db \
    PW_WEB_PORT=8443
EXPOSE 8443 8444
VOLUME ["/opt/pathweaver/data"]
CMD ["/opt/pathweaver/bin/pathweaver-controller"]
