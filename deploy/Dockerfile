# tickd — tick's sync server. Multi-stage: the builder needs the Go toolchain,
# the runtime needs nothing but the static binary (CGO off) and busybox wget
# for the compose healthcheck.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/tickd/ cmd/tickd/
COPY internal/ internal/
ARG VERSION=dev
# tickd imports only the stdlib + internal packages (the Charm deps belong to
# the TUI binary), so this build needs no module downloads — it works even
# where the build machine can't reach proxy.golang.org. GOTOOLCHAIN=local
# fails loudly instead of fetching a toolchain if the base image lags go.mod.
ENV GOTOOLCHAIN=local
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/tickd ./cmd/tickd

FROM alpine:3.22
RUN adduser -D -H -u 10001 tick && mkdir -p /data && chown tick:tick /data
COPY --from=build /out/tickd /usr/local/bin/tickd
USER tick
ENV TICK_DATA_DIR=/data \
    TICK_ADDR=:8080
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["tickd"]
