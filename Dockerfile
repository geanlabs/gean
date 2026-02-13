FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /build/gean ./cmd/gean

FROM alpine:3.21

RUN apk add --no-cache ca-certificates
COPY --from=builder /build/gean /usr/local/bin/gean

ENTRYPOINT ["gean"]
