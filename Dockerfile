# syntax=docker/dockerfile:1

FROM golang:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o forwardme main.go

FROM alpine:latest

# Create a non-root user and group
RUN addgroup -S appgroup && adduser -S -G appgroup appuser

WORKDIR /app

# Create the /app/data directory
RUN mkdir -p /app/data && chown appuser:appgroup /app/data

# Copy the executable and data with the correct permissions
COPY --chown=appuser:appgroup --from=builder /app/forwardme /app/forwardme
COPY --chown=appuser:appgroup --from=builder /app/data /app/data

# Set the user for the container
USER appuser

CMD ["/app/forwardme"]
