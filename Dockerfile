# Dockerfile para corrigir a versão do Go em go.mod
# Stage 1: Build the Go application
FROM golang:1.23 AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Corrigir a versão do Go no go.mod para uma versão correta
RUN sed -i 's/go 1.23.1/go 1.23/' go.mod

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the Go app
RUN go build -o crudder-go

# Stage 2: Run the application
FROM debian:12

WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/crudder-go .

COPY --from=builder /app/.env ./

EXPOSE 8080

CMD ["./crudder-go"]
