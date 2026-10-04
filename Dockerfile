FROM golang:1.25.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOMAXPROCS=2 go build -p=1 -trimpath -o /homeproxy ./cmd/homeproxy
FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 proxy && mkdir /run/homeproxy && chown proxy /run/homeproxy
COPY --from=build /homeproxy /usr/local/bin/homeproxy
USER proxy
ENTRYPOINT ["homeproxy"]
CMD ["server", "-quic", "0.0.0.0:4433", "-socks", "0.0.0.0:1080", "-cert", "/secrets/cert.pem", "-key", "/secrets/key.pem"]
