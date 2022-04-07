# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./test/test_images/event-library/kodata /var/run/ko
ADD event-library /usr/bin/event-library
ENTRYPOINT ["/usr/bin/event-library"]
