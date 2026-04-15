FROM quay.io/redhat-services-prod/openshift/boilerplate:image-v8.3.4 AS builder

WORKDIR /workdir

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY . .

RUN make go-build
RUN GOOS=linux CGO_ENABLED=1 GOARCH=amd64 GOFLAGS="" go build -o build/_output/bin/addon-operator-webhook ./cmd/addon-operator-webhook

###
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.7-1773939694

ENV USER_UID=1001 \
    USER_NAME=addon-operator

COPY --from=builder /workdir/build/_output/bin/* /usr/local/bin/

COPY build/bin /usr/local/bin
RUN /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}

LABEL io.openshift.managed.name="addon-operator" \
    io.openshift.managed.description="Addon Operator"
