FROM golang:1.18-alpine3.15 AS build

WORKDIR /go/src/zed
COPY . /go/src/zed
RUN go mod download
RUN go install ./cmd/zed

FROM gcr.io/distroless/base
COPY --from=build /go/bin/* /usr/local/bin/
ENTRYPOINT ["zed"]
