FROM docker

RUN apk upgrade --update --no-cache

RUN apk install iptables

USER nobody

ADD build/_output/bin/chaos-controller /usr/local/bin/chaos-controller
