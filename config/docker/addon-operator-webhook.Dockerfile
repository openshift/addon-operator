FROM registry.access.redhat.com/ubi8/ubi-minimal@sha256:3e1adcc31c6073d010b8043b070bd089d7bf37ee2c397c110211a6273453433f
# registry.access.redhat.com/ubi8/ubi-minimal:8.7-1107

# shadow-utils contains adduser and groupadd binaries
RUN microdnf install shadow-utils \
	&& groupadd --gid 1000 noroot \
	&& adduser \
		--no-create-home \
		--no-user-group \
		--uid 1000 \
		--gid 1000 \
		noroot

WORKDIR /

COPY addon-operator-webhook /usr/local/bin/

USER 1001

ENTRYPOINT ["/usr/local/bin/addon-operator-webhook"]
