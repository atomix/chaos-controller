FROM docker

RUN apk upgrade --update --no-cache

RUN apk add iptables bash

USER nobody

ADD build/_output/bin/chaos-worker /usr/local/bin/chaos-worker
