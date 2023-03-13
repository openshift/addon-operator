FROM registry.access.redhat.com/ubi8/ubi-minimal@sha256:ab03679e683010d485ef0399e056b09a38d7843ba4a36ee7dec337dd0037f7a7
# registry.access.redhat.com/ubi8/ubi-minimal:8.7-1085

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

USER "noroot"

ENTRYPOINT ["/usr/local/bin/addon-operator-webhook"]
