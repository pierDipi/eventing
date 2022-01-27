# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./cmd/webhook/kodata /var/run/ko
ADD webhook /usr/bin/webhook
ENTRYPOINT ["/usr/bin/webhook"]
