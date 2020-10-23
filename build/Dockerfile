FROM golang:1.13 as antrea-operator-build

WORKDIR /antrea-operator

COPY go.mod /antrea-operator/go.mod

RUN go mod download

COPY . /antrea-operator

RUN make bin

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

LABEL name="antrea-operator-for-kubernetes"
LABEL maintainer="Antrea Team"
LABEL summary="A cluster operator to deploy Antrea CNI plugin"
LABEL version="0.0.1"
LABEL release="1"
LABEL description="Manage deployments, daemonsets, and config maps for Antrea"
LABEL vendor="VMware"

COPY LICENSE /licenses/

ENV OPERATOR=/usr/local/bin/antrea-operator \
    USER_UID=1001 \
    USER_NAME=antrea-operator


COPY build/bin /usr/local/bin
# install operator binary
COPY --from=antrea-operator-build /antrea-operator/build/_output/bin/antrea-operator ${OPERATOR}
COPY manifest /manifest
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
