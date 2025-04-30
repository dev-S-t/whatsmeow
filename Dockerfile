FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go get github.com/gin-gonic/gin@latest \
    && go mod tidy

COPY . .

# Build the server binary from cmd/server
RUN CGO_ENABLED=0 GOOS=linux \
    go build -tags netgo -ldflags '-s -w' -o /app ./cmd/server

FROM scratch
COPY --from=builder /app /app
EXPOSE 10000
ENTRYPOINT ["/app"]
