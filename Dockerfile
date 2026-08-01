FROM oven/bun:1-alpine AS frontend
WORKDIR /ui
COPY package.json bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache \
    bun install --frozen-lockfile
COPY . .
RUN bun run build

FROM golang:1.26-alpine AS backend
RUN apk add --no-cache build-base
WORKDIR /src
RUN --mount=type=bind,source=.,target=/src \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download
RUN --mount=type=bind,source=.,target=/src \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -o /out/statuspage ./server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend /out/statuspage /app/statuspage
COPY --from=frontend /ui/dist /app/dist
ENV CONFIG=/app/config.local.yaml
VOLUME /app/data.volume
EXPOSE 5000
ENTRYPOINT ["/app/statuspage"]
CMD ["--addr", ":5000"]
