FROM golang:1.19.1-alpine3.16 AS build

RUN apk update
RUN apk add git

WORKDIR /go/src/zed
COPY . /go/src/zed
RUN go mod download
RUN go install ./cmd/zed

FROM distroless.dev/alpine-base
COPY --from=build /go/bin/* /usr/local/bin/
ENTRYPOINT ["zed"]
