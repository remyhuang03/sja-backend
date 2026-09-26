FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server .

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S sja && adduser -S -G sja sja && mkdir -p /data && chown sja:sja /data
WORKDIR /app
COPY --from=build /server /app/server
ENV DATA_DIR=/data BACKEND_PORT=8080
USER sja
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=30s CMD wget -q -O /dev/null http://127.0.0.1:8080/api/readyz || exit 1
CMD ["/app/server"]
