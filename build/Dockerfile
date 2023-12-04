FROM golang:1.19 as antrea-operator-build

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy source files
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on make manager


FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

LABEL name="antrea-operator-for-kubernetes"
LABEL maintainer="Antrea <projectantrea-dev@googlegroups.com>"
LABEL vendor="Project Antrea"
LABEL summary="A cluster operator to deploy Antrea CNI plugin"
# set at build time
LABEL version=""
LABEL release="1"
LABEL description="Manage deployments, daemonsets, and config maps for Antrea"

COPY LICENSE /licenses/

ENV OPERATOR=/usr/local/bin/antrea-operator \
    USER_UID=1001 \
    USER_NAME=antrea-operator


COPY build/bin /usr/local/bin
# install operator binary
COPY --from=antrea-operator-build /workspace/bin/manager ${OPERATOR}
COPY antrea-manifest /antrea-manifest
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
