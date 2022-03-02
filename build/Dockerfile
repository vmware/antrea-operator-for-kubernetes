FROM golang:1.16 as antrea-operator-build

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY antrea_import/ antrea_import/
COPY api/ api/
COPY controllers/ controllers/
COPY version/ version/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go


FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

LABEL name="antrea-operator-for-kubernetes"
LABEL maintainer="Antrea <projectantrea-dev@googlegroups.com>"
LABEL vendor="Project Antrea"
LABEL summary="A cluster operator to deploy Antrea CNI plugin"
LABEL version="0.0.1"
LABEL release="1"
LABEL description="Manage deployments, daemonsets, and config maps for Antrea"

COPY LICENSE /licenses/

ENV OPERATOR=/usr/local/bin/antrea-operator \
    USER_UID=1001 \
    USER_NAME=antrea-operator


COPY build/bin /usr/local/bin
# install operator binary
COPY --from=antrea-operator-build /workspace/manager ${OPERATOR}
COPY antrea-manifest /antrea-manifest
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
