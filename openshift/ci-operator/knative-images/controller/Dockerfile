# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./cmd/controller/kodata /var/run/ko
ADD controller /usr/bin/controller
ENTRYPOINT ["/usr/bin/controller"]
