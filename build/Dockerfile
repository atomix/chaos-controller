FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD build/_output/bin/chaos-controller /usr/local/bin/chaos-controller
