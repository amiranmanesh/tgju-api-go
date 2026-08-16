# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Build stage.
#
# The module is vendorless and has a single dependency, so the download layer is
# small and caches well. BuildKit cache mounts keep the module and build caches
# between builds without baking them into the image.
# ---------------------------------------------------------------------------
FROM golang:1.26-alpine AS build

# Reproducibility: build with exactly the toolchain go.mod asks for rather than
# letting the builder fetch a newer one halfway through.
ENV GOTOOLCHAIN=local \
    CGO_ENABLED=0

WORKDIR /src

# Dependencies first: this layer only changes when go.mod or go.sum does.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# TARGETOS and TARGETARCH are provided by buildx; they default to the build
# platform for a plain `docker build`.
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.buildVersion=${VERSION}" \
      -o /out/tgju ./cmd/tgju

# The image has no shell, so the healthcheck cannot be a curl one-liner. A tiny
# Go probe compiled next to the server is the cheapest way to keep one.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./internal/cmd/healthcheck

# ---------------------------------------------------------------------------
# Runtime stage.
#
# scratch plus the CA bundle: the binary is static, it speaks TLS to tgju.org,
# and it needs nothing else. No shell means no shell to exploit.
# ---------------------------------------------------------------------------
FROM scratch AS runtime

ARG VERSION=dev

LABEL org.opencontainers.image.title="tgju-api-go" \
      org.opencontainers.image.description="Live currency, gold and coin prices from tgju.org, as a Go library and a JSON API." \
      org.opencontainers.image.source="https://github.com/amiranmanesh/tgju-api-go" \
      org.opencontainers.image.documentation="https://amiranmanesh.github.io/tgju-api-go/" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/tgju /usr/local/bin/tgju
COPY --from=build /out/healthcheck /usr/local/bin/healthcheck

# 65532 is the conventional "nonroot" uid; the number is used directly because
# scratch has no /etc/passwd to resolve a name against.
USER 65532:65532

ENV TGJU_ADDR=:8080 \
    TGJU_LOG_JSON=true

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthcheck"]

ENTRYPOINT ["/usr/local/bin/tgju"]
CMD ["serve"]
