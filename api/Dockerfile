# syntax=docker/dockerfile:1.7
# Build context: the api/ directory.
#   docker build -t simhook/api api

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/simhook ./cmd/simhook

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -H -u 10001 simhook
COPY --from=build /out/simhook /usr/local/bin/simhook
USER simhook
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
# `serve` runs migrations on start, then the HTTP server and the job workers.
ENTRYPOINT ["simhook"]
CMD ["serve"]
