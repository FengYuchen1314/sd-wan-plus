FROM rust:1.83-slim-bookworm AS builder

RUN apt-get update && apt-get install -y \
    pkg-config libssl-dev protobuf-compiler \
    nodejs npm curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY . .

RUN cd web && npm install && npm run build && cd ..

RUN cargo build --release -p pathweaver-controller

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    ca-certificates wireguard-tools nftables sqlite3 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/target/release/pathweaver-controller /usr/local/bin/
COPY --from=builder /build/web/dist /opt/pathweaver/web

RUN mkdir -p /opt/pathweaver/data
ENV PW_STATIC_DIR=/opt/pathweaver/web
ENV RUST_LOG=info

EXPOSE 8443 8444 30000-30999/udp

VOLUME ["/opt/pathweaver/data"]

ENTRYPOINT ["pathweaver-controller"]
