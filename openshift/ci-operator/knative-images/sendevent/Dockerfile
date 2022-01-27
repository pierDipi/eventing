# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./cmd/sendevent/kodata /var/run/ko
ADD sendevent /usr/bin/sendevent
ENTRYPOINT ["/usr/bin/sendevent"]
