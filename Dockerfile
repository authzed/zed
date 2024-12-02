# NOTE: we use the chainguard image here rather
# than the golang image because the golang image
# uses musl as its libc, and chainguard no longer provides
# a musl-dynamic container. - 2024-12-02 Tanner Stirrat
FROM cgr.dev/chainguard/go:latest AS zed-builder
WORKDIR /go/src/app
RUN apk update && apk add --no-cache git
COPY . .
RUN go build -v ./cmd/zed/

FROM cgr.dev/chainguard/glibc-dynamic:latest
COPY --from=zed-builder /go/src/app/zed /usr/local/bin/zed
ENTRYPOINT ["zed"]
