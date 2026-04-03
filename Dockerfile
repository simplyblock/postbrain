# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25
ARG GOPLS_VERSION=v0.21.1
ARG MARKITDOWN_VERSION=0.1.5

FROM golang:${GO_VERSION}-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /out/postbrain ./cmd/postbrain

FROM golang:${GO_VERSION}-bookworm AS gopls
ARG GOPLS_VERSION
RUN GOBIN=/out go install golang.org/x/tools/gopls@${GOPLS_VERSION}

FROM python:3.12-slim AS runtime
ARG MARKITDOWN_VERSION

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tini \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir --upgrade pip \
    && pip install --no-cache-dir "markitdown[all]==${MARKITDOWN_VERSION}"

COPY --from=builder /out/postbrain /usr/local/bin/postbrain
COPY --from=gopls /out/gopls /usr/local/bin/gopls

RUN mkdir -p /etc/postbrain
COPY config.example.yaml /etc/postbrain/config.yaml

EXPOSE 7433

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["postbrain", "serve", "--config", "/etc/postbrain/config.yaml"]
