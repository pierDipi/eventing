# DO NOT EDIT! Generated Dockerfile for cmd/apiserver_receive_adapter.
FROM registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.22-openshift-4.17 as builder

COPY . .

RUN mkdir -p /var/run/ko && \
    mkdir -p cmd/apiserver_receive_adapter/kodata && \
    go build -o /usr/bin/main ./cmd/apiserver_receive_adapter && \
    cp -r cmd/apiserver_receive_adapter/kodata /var/run/ko

FROM registry.access.redhat.com/ubi8/ubi-minimal

# install the missing zoneinfo to ubi-minimal
RUN microdnf install tzdata

USER 65532

COPY --from=builder /usr/bin/main /usr/bin/main
COPY --from=builder /var/run/ko /var/run/ko
ENTRYPOINT ["/usr/bin/main"]
