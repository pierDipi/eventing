# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./cmd/in_memory/channel_controller/kodata /var/run/ko
ADD channel_controller /usr/bin/channel_controller
ENTRYPOINT ["/usr/bin/channel_controller"]
