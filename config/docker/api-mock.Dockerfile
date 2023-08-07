FROM registry.access.redhat.com/ubi8/ubi-minimal@sha256:3e1adcc31c6073d010b8043b070bd089d7bf37ee2c397c110211a6273453433f
# registry.access.redhat.com/ubi8/ubi-minimal:8.7-1107

USER 65532:65532

WORKDIR /

COPY api-mock /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/api-mock"]
