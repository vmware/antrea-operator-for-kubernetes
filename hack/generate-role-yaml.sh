#!/bin/bash

set -eo pipefail

function echoerr {
    >&2 echo "$@"
}

_usage="Usage: $0 [--version <antrea version>] [--help|-h]
Generate a YAML manifest for Antrea using Kustomize and print it to stdout.
        --version                     Antrea version for use for manifest generation
        --help, -h                    Print this message and exit

This tool uses kustomize (https://github.com/kubernetes-sigs/kustomize) to generate manifests for
Antrea. You can set the KUSTOMIZE environment variable to the path of the kustomize binary you want
us to use. Otherwise we will download the appropriate version of the kustomize binary and use
it (this is the recommended approach since different versions of kustomize may create different
output YAMLs)."

function print_usage {
    echoerr "$_usage"
}

function print_help {
    echoerr "Try '$0 --help' for more information."
}

ANTREA_VERSION="main"

while [[ $# -gt 0 ]]
do
key="$1"

case $key in
    --version)
    ANTREA_VERSION="$2"
    shift 2
    ;;
    -h|--help)
    print_usage
    exit 0
    ;;
    *)    # unknown option
    echoerr "Unknown option $1"
    exit 1
    ;;
esac
done

THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

# Get Antrea repository
ANTREA_DIR=$(mktemp -d /tmp/antrea.XXXXXXX)

if [ "$ANTREA_VERSION" == "main" ]; then
    ANTREA_URL="https://github.com/antrea-io/antrea/archive/refs/heads/main.tar.gz"
    ANTREA_ROOT=$ANTREA_DIR/antrea-main
else
    ANTREA_URL="https://github.com/antrea-io/antrea/archive/refs/tags/v${ANTREA_VERSION}.tar.gz"
    ANTREA_ROOT=$ANTREA_DIR/antrea-$ANTREA_VERSION
fi
curl -sL $ANTREA_URL | tar xz -C $ANTREA_DIR

# generate-role-yaml.py requires PyYAML, install it just in case that it's missing
pip3 -q install PyYAML

$THIS_DIR/generate-role-yaml.py $THIS_DIR/../config/rbac/role.yaml \
                                $ANTREA_ROOT/build/yamls/base/agent-rbac.yml \
                                $ANTREA_ROOT/build/yamls/base/controller-rbac.yml \
                                $ANTREA_ROOT/build/yamls/base/antctl.yml \
                                $ANTREA_ROOT/build/yamls/base/crds-rbac.yml

rm -rf $TMP_DIR $ANTREA_DIR
