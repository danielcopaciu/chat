FROM golang:1.10-alpine AS build

RUN apk update && apk add make git gcc musl-dev

ARG SERVICE

ADD . /go/src/github.com/danielcopaciu/${SERVICE}

WORKDIR /go/src/github.com/danielcopaciu/${SERVICE}

RUN make clean install
RUN make ${SERVICE}

RUN mv ${SERVICE} /${SERVICE}

FROM alpine:3.8

ARG SERVICE

ENV APP=${SERVICE}
ENV ADDRESS=${ADDRESS}

RUN apk add --no-cache ca-certificates openssl && mkdir /app
COPY --from=build /${SERVICE} /app/${SERVICE}

ENTRYPOINT /app/${APP} server
