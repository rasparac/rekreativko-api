ARG GO_VERSION=1.26-alpine

# ---- build stage ----
FROM golang:${GO_VERSION} AS builder

ARG SERVICE

WORKDIR /app

# Copy dependency manifests first for better layer caching
COPY go.mod go.sum ./

# Download dependencies into the cache mount (not the image layer)
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download -x

# Copy source after deps are cached
COPY ./cmd/${SERVICE} ./cmd
COPY ./internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o service \
    ./cmd/main.go

# ---- runtime stage ----
FROM gcr.io/distroless/base-debian12

WORKDIR /app

COPY --from=builder /app/service /app/service

EXPOSE 8080

USER 65532:65532

ENTRYPOINT ["/app/service"]