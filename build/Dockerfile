FROM docker

RUN apk upgrade --update --no-cache

RUN apk add iptables

USER nobody

ADD build/_output/bin/chaos-controller /usr/local/bin/chaos-controller
