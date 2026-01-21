#./Dockerfile

# Stage 1: Build
FROM golang:1.25 AS builder

WORKDIR /app

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o crudder-go

# Stage 2: Runtime (distroless, nonroot)
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /app/crudder-go ./crudder-go

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["./crudder-go"]
