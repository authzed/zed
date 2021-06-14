FROM golang:1.16-alpine3.13 AS build

WORKDIR /go/src/zed
COPY . /go/src/zed
RUN go mod download
RUN go install ./cmd/zed

FROM alpine:3.13
RUN apk --no-cache add ca-certificates
COPY --from=build /go/bin/* /usr/local/bin/
ENTRYPOINT ["zed"]
