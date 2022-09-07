#!/bin/bash

set -eo pipefail

function echoerr {
    >&2 echo "$@"
}

_usage="Usage: $0 [--version <antrea version>] [--platform[openshift|kubernetes]] [--help|-h]
Generate a YAML manifest for Antrea using Kustomize and print it to stdout.
        --version                     Antrea version for use for manifest generation
        --platform                    Target platform for operator yamls
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

ANTREA_VERSION=${ANTREA_VERSION:-"main"}
ANTREA_PLATFORM=${ANTREA_PLATFORM:-"kubernetes"}

while [[ $# -gt 0 ]]
do
key="$1"

case $key in
    --version)
    ANTREA_VERSION="$2"
    shift 2
    ;;
    --platform)
    ANTREA_PLATFORM="$2"
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

if [ "$ANTREA_PLATFORM" != "openshift" ] && [ "$ANTREA_PLATFORM" != "kubernetes" ]; then
    print_usage
fi

THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

source $THIS_DIR/verify-kustomize.sh

if [ -z "$KUSTOMIZE" ]; then
    KUSTOMIZE="$(verify_kustomize)"
elif ! $KUSTOMIZE version > /dev/null 2>&1; then
    echoerr "$KUSTOMIZE does not appear to be a valid kustomize binary"
    print_help
    exit 1
fi

### Get Antrea repository
ANTREA_DIR=$(mktemp -d /tmp/antrea.XXXXXXX)

if [ "$ANTREA_VERSION" == "main" ]; then
    ANTREA_URL="https://github.com/antrea-io/antrea/archive/refs/heads/main.tar.gz"
    ANTREA_ROOT=$ANTREA_DIR/antrea-main
else
    ANTREA_URL="https://github.com/antrea-io/antrea/archive/refs/tags/v${ANTREA_VERSION}.tar.gz"
    ANTREA_ROOT=$ANTREA_DIR/antrea-$ANTREA_VERSION
fi
curl -sL $ANTREA_URL | tar xz -C $ANTREA_DIR

### Generate antrea-manifest/antrea.yml

KUSTOMIZATION_DIR=$ANTREA_ROOT/build/yamls

TMP_DIR=$(mktemp -d /tmp/overlays.XXXXXXXX)

mkdir $TMP_DIR/base
pushd $TMP_DIR/base > /dev/null

# Use antrea.yml from Antrea repo as base
cp $ANTREA_ROOT/build/yamls/antrea.yml .
touch kustomization.yml
$KUSTOMIZE edit add resource antrea.yml

# do all ConfigMap edits
mkdir $TMP_DIR/configMap && cd $TMP_DIR/configMap

BASE=../base
cp $THIS_DIR/../build/yamls/configMap/* .
$KUSTOMIZE edit add base $BASE

BASE=../configMap
cd $TMP_DIR
mkdir osmft && cd osmft
cp $THIS_DIR/../build/yamls/patches/*.yml .
touch kustomization.yml
$KUSTOMIZE edit add base $BASE
$KUSTOMIZE edit add patch --path agentOcpRelease.yml
$KUSTOMIZE edit add patch --path agentImage.yml
$KUSTOMIZE edit add patch --path ovsImage.yml
$KUSTOMIZE edit add patch --path installCniImage.yml
$KUSTOMIZE edit add patch --path installCniConfDir.yml
$KUSTOMIZE edit add patch --path installCniBinDir.yml
$KUSTOMIZE edit add patch --path controllerOsRelease.yml
$KUSTOMIZE edit add patch --path controllerImage.yml
$KUSTOMIZE edit add patch --path agentImagePullPolicy.yml
$KUSTOMIZE edit add patch --path controllerImagePullPolicy.yml

$KUSTOMIZE build | sed 's/^\s*{{/{{/; s/\\"\({{.*}}\)\\"/"\1"/; '"s/'\({{.*}}\)'/\1/" > $THIS_DIR/../antrea-manifest/antrea.yml

popd > /dev/null

### Generate config/rbac/role.yaml
# generate-role-yaml.py requires PyYAML, install it just in case that it's missing
pip3 -q install PyYAML

ROLE_FILES="$THIS_DIR/../config/rbac/role_base.yaml $ANTREA_ROOT/build/yamls/antrea.yml"

if [ "$ANTREA_PLATFORM" == "openshift" ]; then
    ROLE_FILES+=" $THIS_DIR/../config/rbac/role_base_ocp.yaml"
fi

$THIS_DIR/generate-role-yaml.py $ROLE_FILES  > $THIS_DIR/../config/rbac/role.yaml

### Generate config/samples/operator_v1_antreainstall.yaml
$THIS_DIR/generate-antrea-samples.py --platform $ANTREA_PLATFORM --version $ANTREA_VERSION \
    $ANTREA_ROOT/build/yamls/antrea.yml > $THIS_DIR/../config/samples/operator_v1_antreainstall.yaml

rm -rf $TMP_DIR $ANTREA_DIR

exit 0
