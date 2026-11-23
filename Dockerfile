# syntax=docker/dockerfile:1
FROM docker.io/library/golang:1.27.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/ib_exporter .

# Debian slim, not Alpine: the infiniband-diags tools (perfquery, smpquery,
# ibnetdiscover, sminfo) are glibc binaries that link libibmad/libibumad.
FROM docker.io/library/debian:trixie-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends infiniband-diags curl ca-certificates && \
    apt-cache policy infiniband-diags && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd --gid 10001 ib && \
    useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin ib

COPY --from=build /out/ib_exporter /ib_exporter
COPY config.yaml /etc/ib_exporter/config.yaml

EXPOSE 9449

# /livez never depends on fabric reachability or the collection cycle.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:9449/livez || exit 1

USER ib

ENTRYPOINT ["/ib_exporter"]
CMD ["--config", "/etc/ib_exporter/config.yaml"]
