# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder

# Install build dependencies for CGO (required by go-sqlite3)
RUN apk add --no-cache gcc musl-dev sqlite-dev

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum to leverage Docker's layer caching.
# This will only re-download dependencies if go.mod or go.sum has changed.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application.
# CGO_ENABLED=1 is required for the sqlite driver.
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /app/main .

# Stage 2: Create the final, minimal image
FROM alpine:latest

# Install runtime dependencies.
# - sqlite is needed by the application.
# - ca-certificates is needed for making HTTPS requests (e.g., to Discord).
# - procps provides 'pgrep' for the healthcheck.
RUN apk add --no-cache sqlite ca-certificates procps

# Set the working directory
WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /app/main .

# The application creates a sqlite database in the 'data' directory.
RUN mkdir data

# Set the command to run the application
CMD ["./main"]
