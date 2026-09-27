# ---------- 前端构建 ----------
FROM node:20-alpine AS fe
WORKDIR /fe
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---------- 后端构建 ----------
FROM golang:1.22-alpine AS be
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /be
COPY backend/ ./
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /kegtrace .

# ---------- 运行时 ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget \
    && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=be /kegtrace /app/kegtrace
COPY --from=fe /fe/dist /app/static
COPY --chown=appuser:appuser demo-uploads/ /app/data/uploads/
ENV PORT=8080 \
    STATIC_DIR=/app/static \
    UPLOAD_DIR=/app/data/uploads
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
CMD ["/app/kegtrace"]
