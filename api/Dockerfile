# Stage 1: Build
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
# (ถ้ามี go.sum ให้เอาคอมเมนต์บรรทัดล่างออก)
# COPY go.sum ./ 
RUN go mod download
COPY main.go ./
RUN go build -o main .

# Stage 2: Run
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/main .
EXPOSE 3000
CMD ["./main"]