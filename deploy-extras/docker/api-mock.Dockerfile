# Used for local dev only
FROM registry.access.redhat.com/ubi8/ubi:8.10-1752733233

WORKDIR /

COPY api-mock /usr/local/bin/

USER 1001

ENV CGO_ENABLED=1

# force the binary to behave as if FIPS mode were enabled.
ENV OPENSSL_FORCE_FIPS_MODE=1

ENTRYPOINT ["/usr/local/bin/api-mock"]
