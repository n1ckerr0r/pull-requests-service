FROM golang:1.23-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /src/cmd/app

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app .

# final stage
FROM alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=builder /app /app
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/app"]