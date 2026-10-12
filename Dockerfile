# Stage 1: Build
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o homesocks ./cmd/homesocks
FROM alpine:latest
RUN apk add --no-cache iproute2 iptables openvpn curl tzdata ca-certificates

WORKDIR /app
COPY --from=builder /app/homesocks /usr/local/bin/homesocks
RUN chmod +x /usr/local/bin/homesocks

VOLUME ["/etc/homesocks"]

EXPOSE 8899 1080

ENTRYPOINT ["/usr/local/bin/homesocks", "-c", "/etc/homesocks/config.json"]