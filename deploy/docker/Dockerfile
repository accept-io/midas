FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod tidy || true

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /midas ./cmd/midas

FROM alpine:3.20
COPY --from=build /midas /usr/local/bin/midas
EXPOSE 8080
ENTRYPOINT ["midas"]
