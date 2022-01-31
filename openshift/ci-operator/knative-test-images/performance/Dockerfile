# Do not edit! This file was generated via Makefile
FROM openshift/origin-base
USER 65532

COPY ./test/test_images/performance/kodata /var/run/ko
ADD performance /usr/bin/performance
ENTRYPOINT ["/usr/bin/performance"]
