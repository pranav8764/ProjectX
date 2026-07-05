# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy modules and download dependencies
COPY go.mod ./
# If go.sum is not generated yet, we run go mod tidy inside, but simple go mod download works
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server cmd/server/main.go

# Production stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/server .

ENV PORT=8080
EXPOSE 8080

CMD ["./server"]
