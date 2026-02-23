FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /emusync .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -h /data emusync
COPY --from=builder /emusync /usr/local/bin/emusync
USER emusync
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["emusync", "server"]
