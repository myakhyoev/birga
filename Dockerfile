FROM golang:1.27.1-alpine AS builder

WORKDIR /src

# Get dependencies - cached as long as go.mod/go.sum do not change
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/birga_backend ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 app

ENV TZ=Asia/Tashkent

COPY --from=builder /out/birga_backend /usr/local/bin/birga_backend

USER app

EXPOSE 8080 9090

ENTRYPOINT ["/usr/local/bin/birga_backend"]
