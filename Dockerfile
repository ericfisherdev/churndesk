# Stage 1: build
FROM golang:1.25-bookworm AS builder

WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc libc-dev libsqlite3-dev && rm -rf /var/lib/apt/lists/*

ENV CGO_ENABLED=1

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001 && templ generate ./internal/web/templates/...
RUN go build -o /churndesk ./cmd/churndesk

# Stage 2: runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libsqlite3-0 ca-certificates && rm -rf /var/lib/apt/lists/* \
    && groupadd -r churndesk && useradd -r -g churndesk -u 1000 churndesk

COPY --from=builder /churndesk /churndesk
RUN chown churndesk:churndesk /churndesk

USER churndesk
EXPOSE 8080
ENTRYPOINT ["/churndesk"]
