FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the applications
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o phase1 cmd/phase1/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o phase2 cmd/phase2/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

# Copy the binaries from builder
COPY --from=builder /app/phase1 .
COPY --from=builder /app/phase2 .

# Default to phase1
CMD ["./phase1"] 