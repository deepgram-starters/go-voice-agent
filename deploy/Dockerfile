# Stage 1: Build custom Caddy with rate limiting module
FROM golang:1.23-alpine AS caddy-builder
WORKDIR /build
RUN go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
RUN xcaddy build --with github.com/mholt/caddy-ratelimit
# Binary output: /build/caddy

# Stage 2: Build Go backend
FROM golang:1.23-alpine AS backend-builder
WORKDIR /build
COPY go.mod go.sum* ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server main.go

# Stage 3: Build frontend
FROM node:24-slim AS frontend-builder
RUN corepack enable
WORKDIR /build
COPY frontend/package.json frontend/pnpm-lock.yaml ./frontend/
RUN cd frontend && pnpm install --frozen-lockfile
COPY frontend/ ./frontend/
RUN cd frontend && pnpm build
# Inject Caddy template directive for subpath base path
RUN sed -i 's|<head>|<head>\n  <base href="{{ .Req.Header.Get "X-Base-Path" }}">|' ./frontend/dist/index.html

# Stage 4: Runtime
FROM alpine:3.21

# Copy custom Caddy binary
COPY --from=caddy-builder /build/caddy /usr/bin/caddy

WORKDIR /app

# Copy Go binary
COPY --from=backend-builder /build/server ./server

# Copy config files
COPY *.toml sample.env ./

# Copy built frontend
COPY --from=frontend-builder /build/frontend/dist/ ./frontend/dist/

# Copy shared deployment files
COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY deploy/start.sh ./start.sh
RUN chmod +x ./start.sh

ENV PORT=8081
ENV HOST=0.0.0.0
EXPOSE 8080

ENV BACKEND_CMD="./server"
CMD ["./start.sh"]
