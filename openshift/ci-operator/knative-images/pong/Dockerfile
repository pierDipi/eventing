# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./cmd/pong/kodata /var/run/ko
ADD pong /usr/bin/pong
ENTRYPOINT ["/usr/bin/pong"]
