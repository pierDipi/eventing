# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./cmd/mtping/kodata /var/run/ko
ADD mtping /usr/bin/mtping
ENTRYPOINT ["/usr/bin/mtping"]
