# Used for local dev only
# TODO: Deprecate post Boilerplate adoption
FROM registry.access.redhat.com/ubi8/ubi:8.8-1032

WORKDIR /

COPY addon-operator-webhook /usr/local/bin/

USER 1001

ENV CGO_ENABLED=1

ENTRYPOINT ["/usr/local/bin/addon-operator-webhook"]
