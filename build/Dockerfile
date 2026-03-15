FROM golang:1.23 as antrea-operator-build

WORKDIR /workspace

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go mod download

# Copy source files
COPY . .

# Build
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target=/root/.cache/go-build/ \
    CGO_ENABLED=0 make manager


FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

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
