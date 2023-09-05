FROM registry.access.redhat.com/ubi8/ubi-minimal:8.8-1037

WORKDIR /

COPY prometheus-remote-storage-mock /usr/local/bin/

USER 1001

ENV CGO_ENABLED=1

# force the binary to behave as if FIPS mode were enabled.
ENV OPENSSL_FORCE_FIPS_MODE=1

ENTRYPOINT ["/usr/local/bin/prometheus-remote-storage-mock"]
