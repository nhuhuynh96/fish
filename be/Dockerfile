# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/api cmd/api/main.go

# Stage 2: Runtime
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Ho_Chi_Minh

COPY --from=builder /app/api /app/api

EXPOSE 8080

CMD ["/app/api"]
