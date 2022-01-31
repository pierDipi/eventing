# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./cmd/sugar_controller/kodata /var/run/ko
ADD sugar_controller /usr/bin/sugar_controller
ENTRYPOINT ["/usr/bin/sugar_controller"]
