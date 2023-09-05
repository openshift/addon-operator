FROM registry.access.redhat.com/ubi8/ubi:8.8-1032

WORKDIR /

COPY addon-operator-webhook /usr/local/bin/

USER 1001

ENV CGO_ENABLED=1

# force the binary to behave as if FIPS mode were enabled.
ENV OPENSSL_FORCE_FIPS_MODE=1

ENTRYPOINT ["/usr/local/bin/addon-operator-webhook"]
