# ── Stage 1: Build ────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Download dependencies first (cached layer — only re-runs if go.mod changes)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o site .

# ── Stage 2: Runtime ───────────────────────────────────────────────────
# Distroless is ~2MB vs alpine's ~8MB, and has no shell (smaller attack surface)
FROM gcr.io/distroless/static-debian12

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/site .

# Cloud Run sets the PORT env var — our app reads it
EXPOSE 8080

CMD ["/app/site"]
