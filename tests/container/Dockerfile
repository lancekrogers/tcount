FROM golang:1.25.6-bookworm

RUN apt-get update \
    && apt-get install --no-install-recommends --yes time \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY tokenizer ./tokenizer
COPY tests ./tests

RUN mkdir -p /workspace/bin \
    && go build -o /workspace/bin/tcount ./cmd/tcount \
    && go build -o /workspace/bin/cache-validation-bench ./tests/cachefs/validatebench \
    && go build -o /workspace/bin/cache-manifest-bench ./tests/cachefs/manifestbench \
    && chmod +x /workspace/tests/container/run.sh

ENTRYPOINT ["/workspace/tests/container/run.sh"]
