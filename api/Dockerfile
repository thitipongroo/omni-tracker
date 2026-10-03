# Stage 1: Build
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY api ./api
WORKDIR /app/api
RUN go mod tidy
RUN go build -o /app/main main.go

# Stage 2: Run
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/main .
COPY dashboard ./dashboard
EXPOSE 3000
CMD ["./main"]
