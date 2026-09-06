# Production image recipe for termond.
#
# Run with a persistent volume on /data so the SQLite database AND the SSH
# host key survive restarts — regenerating the host key changes the server
# identity every client sees. Example:
#
#   docker build -f Containerfile -t termond .
#   docker run -v termon-data:/data -p 22:2222 termond
#
# Metrics stay on loopback inside the container network namespace
# (127.0.0.1:9090); scrape them from a sidecar/proxy sharing that namespace,
# or see docs/operations.md before changing the bind address.

FROM golang:1.27-bookworm AS build

WORKDIR /src
ARG TERMON_VERSION=dev
COPY go.mod go.sum ./
COPY third_party/ultraviolet ./third_party/ultraviolet
RUN go mod download
COPY . .
# modernc.org/sqlite is pure Go (no cgo), so a fully static binary builds fine.
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-X main.appVersion=${TERMON_VERSION}" -o /out/termond ./cmd/termond

# Pre-create /data in the build stage (the runtime image has no shell)
# so it can be copied in owned by the non-root user; a fresh named volume
# then inherits writable ownership.
RUN mkdir -p /data && chown 65532:65532 /data

# distroless/static is the production-grade stand-in for the scratch runtime
# the other Dockerfiles use: no shell, no package manager, plus CA certs and
# a pre-created non-root user (uid 65532).
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/termond /app/termond
COPY third_party/ultraviolet/LICENSE /app/licenses/ultraviolet-LICENSE
COPY content /app/content
COPY --from=build --chown=65532:65532 /data /data

USER 65532:65532
EXPOSE 2222

# The binary itself probes the loopback-only readiness endpoint, so the
# distroless image does not need curl or a shell.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD ["/app/termond", "-healthcheck-url", "http://127.0.0.1:9090/readyz"]

# Mount a volume here: termon.db (plus its WAL/SHM sidecars) and the host key.
VOLUME ["/data"]

ENTRYPOINT ["/app/termond"]
CMD ["-content", "/app/content", \
     "-database", "/data/termon.db", \
     "-host-key", "/data/host-key", \
     "-listen", "0.0.0.0:2222"]
