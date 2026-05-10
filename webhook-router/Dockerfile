FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/webhook-router ./cmd/webhook-router

FROM alpine:3.22

RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/webhook-router /app/webhook-router
USER app
EXPOSE 18080
ENTRYPOINT ["/app/webhook-router"]
CMD ["--config", "/app/config.yaml"]

