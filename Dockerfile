# ---- build stage ----
FROM golang:1.24 AS builder

WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/app .

# ---- run stage ----
FROM debian:bookworm-slim

ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/app .

# Optional local browser path (only used when not using CDP_URL/REMOTE_BROWSER_URL)
ENV ROD_BROWSER_BIN=/usr/bin/google-chrome

EXPOSE 18060

CMD ["./app"]
