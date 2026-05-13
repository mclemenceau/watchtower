# Multi-stage build — pass TARGET=worker or TARGET=server at build time.
# docker build --build-arg TARGET=server -t argus-server .

FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGET=server
RUN go build -o /bin/argus ./cmd/${TARGET}/

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
# web/ is only needed by the server, but including it for both keeps a
# single image definition.
COPY --from=builder /bin/argus /bin/argus
COPY web/ web/
EXPOSE 8080
CMD ["/bin/argus"]
