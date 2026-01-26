# ---- build stage ----
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY ./vendor ./vendor
COPY go.mod go.sum ./
COPY ./cmd ./cmd
COPY ./internal ./internal
COPY ./docs ./docs
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app ./cmd/api/main.go


# ---- runtime stage ----
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /app/app /app/app

EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/app/app"]