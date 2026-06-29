# Single-stage multi-target build — TARGET=bot (default).
# docker build --build-arg TARGET=bot -t watchtower-bot .

FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGET=bot
RUN go build -o /bin/watchtower ./cmd/${TARGET}/

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/watchtower /bin/watchtower
CMD ["/bin/watchtower"]
