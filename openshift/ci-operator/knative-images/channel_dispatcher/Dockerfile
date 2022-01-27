# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./cmd/in_memory/channel_dispatcher/kodata /var/run/ko
ADD channel_dispatcher /usr/bin/channel_dispatcher
ENTRYPOINT ["/usr/bin/channel_dispatcher"]
