# ./Dockerfile

# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-w -s" -o chatservice ./cmd/server

# Stage 2: Create the final lightweight image
FROM alpine:latest

WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/chatservice .

# Expose application port
EXPOSE 8080

# Run the binary
CMD ["./chatservice"]
