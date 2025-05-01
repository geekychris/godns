FROM golang:1.18-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the applications
RUN go build -o bin/godns cmd/godns/main.go
RUN go build -o bin/godns-client cmd/godns-client/main.go

# Final stage
FROM alpine:latest

WORKDIR /app

# Install CA certificates for HTTPS connections
RUN apk --no-cache add ca-certificates

# Copy the built binaries from the builder stage
COPY --from=builder /app/bin/godns /app/bin/godns
COPY --from=builder /app/bin/godns-client /app/bin/godns-client

# Create a directory for config files
RUN mkdir -p /app/config

# Expose DNS port (UDP)
EXPOSE 53/udp

# Expose REST API port (TCP)
EXPOSE 8080/tcp

# Set the entrypoint to run the DNS server
ENTRYPOINT ["/app/bin/godns", "-config", "/app/config.yaml"]

