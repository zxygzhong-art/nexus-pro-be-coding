# syntax=docker/dockerfile:1

FROM golang:1.26 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/nexus-pro-api ./cmd/api

# distroless static 會以非 root 使用者執行 binary，且不包含 shell 或 package manager。
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

# Production images must never inherit development startup defaults.
ENV APP_ENV=production

COPY --from=builder /out/nexus-pro-api /app/nexus-pro-api

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/app/nexus-pro-api"]
