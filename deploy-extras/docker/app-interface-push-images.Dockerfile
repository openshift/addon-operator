# Used for local dev only
# TODO: Deprecate post Boilerplate adoption
FROM registry.access.redhat.com/ubi8/ubi:8.8-1032

# Install go1.20.6
RUN dnf install -y \
  http://download.eng.bos.redhat.com/brewroot/vol/rhel-8/packages/golang/1.20.6/1.module+el8.9.0+19500+fa91430b/x86_64/golang-1.20.6-1.module+el8.9.0+19500+fa91430b.x86_64.rpm \
  http://download.eng.bos.redhat.com/brewroot/vol/rhel-8/packages/golang/1.20.6/1.module+el8.9.0+19500+fa91430b/x86_64/golang-bin-1.20.6-1.module+el8.9.0+19500+fa91430b.x86_64.rpm \
  http://download.eng.bos.redhat.com/brewroot/vol/rhel-8/packages/golang/1.20.6/1.module+el8.9.0+19500+fa91430b/noarch/golang-src-1.20.6-1.module+el8.9.0+19500+fa91430b.noarch.rpm \
  python3-pip make ncurses git podman gcc && \
  pip3 install pre-commit

ENV PATH="/usr/local/go/bin:${PATH}"

ENV CGO_ENABLED=1

WORKDIR /workdir

COPY . .
