#./Dockerfile

# Stage 1: Build
FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o crudder-go

# Stage 2: Runtime
FROM debian:13-slim

WORKDIR /app

# CA certs (HTTPS) + curl (healthcheck/smoke test)
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/crudder-go ./crudder-go

EXPOSE 8080
CMD ["./crudder-go"]
