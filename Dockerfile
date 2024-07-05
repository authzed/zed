FROM golang:1.22-alpine3.20 AS zed-builder
WORKDIR /go/src/app
RUN apk update && apk add --no-cache git
COPY . .
RUN go build -v ./cmd/zed/

FROM cgr.dev/chainguard/musl-dynamic:latest
COPY --from=zed-builder /go/src/app/zed /usr/local/bin/zed
ENTRYPOINT ["zed"]
