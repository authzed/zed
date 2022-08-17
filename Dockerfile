FROM golang:1.18-alpine3.15 AS build

RUN apk update
RUN apk add git

WORKDIR /go/src/zed
COPY . /go/src/zed
RUN go mod download
RUN go install ./cmd/zed

FROM distroless.dev/static
COPY --from=build /go/bin/* /usr/local/bin/
ENTRYPOINT ["zed"]
