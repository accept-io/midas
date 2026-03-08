# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /midas ./cmd/midas

# Runtime stage
FROM gcr.io/distroless/base-debian12

COPY --from=builder /midas /midas

EXPOSE 8080

ENTRYPOINT ["/midas"]