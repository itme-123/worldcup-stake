# Stage 1: build frontend
FROM node:24-alpine AS frontend-builder
WORKDIR /app/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build

# Stage 2: build Go binary
FROM golang:1.26-alpine3.23 AS go-builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist

RUN CGO_ENABLED=0 GOOS=linux go build -o /worldcup-stake .

# Stage 3: minimal runtime image
FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=go-builder /worldcup-stake .
COPY data/teams.json data/users.json ./data/

VOLUME ["/data"]

ENV PORT=8080 \
    TZ=Pacific/Auckland \
    GIN_MODE=release

EXPOSE 8080
CMD ["./worldcup-stake"]
