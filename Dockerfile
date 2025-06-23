FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the wallet
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o wallet ./cmd/wallet

FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary
COPY --from=builder /app/wallet .

# Create data directories
RUN mkdir -p /app/wallets /app/data

# Create startup script
COPY docker-entrypoint.sh /app/
RUN chmod +x /app/docker-entrypoint.sh

CMD ["/app/docker-entrypoint.sh"]
