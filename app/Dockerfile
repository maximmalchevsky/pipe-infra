# Multi-stage Dockerfile for notifier-api, notifier-worker, and loadgen
FROM --platform=$BUILDPLATFORM golang:1.26 AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o /out/notifier-api ./cmd/notifier-api && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o /out/notifier-worker ./cmd/notifier-worker && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o /out/loadgen ./cmd/loadgen

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/notifier-api /usr/local/bin/notifier-api
COPY --from=build /out/notifier-worker /usr/local/bin/notifier-worker
COPY --from=build /out/loadgen /usr/local/bin/loadgen

# Numeric user so kubelet can verify runAsNonRoot
USER 65532:65532

EXPOSE 8080 9090

ENTRYPOINT ["/usr/local/bin/notifier-api"]
