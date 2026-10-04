# Stage 1: Build Frontend (React/Vite)
FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY dashboard ./
RUN npm install
RUN npm run build

# Stage 2: Build Backend (Go)
FROM golang:1.26-alpine AS backend-builder
WORKDIR /app
COPY api ./api
WORKDIR /app/api
RUN go mod tidy
RUN go build -o /app/main main.go

# Stage 3: Run
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Bangkok
WORKDIR /app
COPY --from=backend-builder /app/main .
COPY --from=frontend-builder /app/dist ./dashboard/dist
EXPOSE 3000
CMD ["./main"]
