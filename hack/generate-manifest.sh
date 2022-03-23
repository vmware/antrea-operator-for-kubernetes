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

MODE="dev"
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

source $THIS_DIR/verify-kustomize.sh

if [ -z "$KUSTOMIZE" ]; then
    KUSTOMIZE="$(verify_kustomize)"
elif ! $KUSTOMIZE version > /dev/null 2>&1; then
    echoerr "$KUSTOMIZE does not appear to be a valid kustomize binary"
    print_help
    exit 1
fi

# Get Antrea repository
ANTREA_DIR=$(mktemp -d /tmp/antrea.XXXXXXX)

if [ "$ANTREA_VERSION" == "main" ]; then
    ANTREA_URL="https://github.com/antrea-io/antrea/archive/refs/heads/main.tar.gz"
    ANTREA_ROOT=$ANTREA_DIR/antrea-main
else
    ANTREA_URL="https://github.com/antrea-io/antrea/archive/refs/tags/v${ANTREA_VERSION}.tar.gz"
    ANTREA_ROOT=$ANTREA_DIR/antrea-v$ANTREA_VERSION
fi
curl -sL $ANTREA_URL | tar xz -C $ANTREA_DIR

KUSTOMIZATION_DIR=$ANTREA_ROOT/build/yamls

TMP_DIR=$(mktemp -d $KUSTOMIZATION_DIR/overlays.XXXXXXXX)

pushd $TMP_DIR > /dev/null

BASE=../../base

# do all ConfigMap edits
mkdir configMap && cd configMap

# OpenShift operator implants the configurations so here just add placeholders
echo "{{.AntreaAgentConfig | indent 4}}" > antrea-agent.conf
echo "{{.AntreaControllerConfig | indent 4}}" > antrea-controller.conf
echo "{{.AntreaCNIConfig | indent 4}}" > antrea-cni.conflist

# unfortunately 'kustomize edit add configmap' does not support specifying 'merge' as the behavior,
# which is why we use a template kustomization file.
sed -e "s/<AGENT_CONF_FILE>/antrea-agent.conf/; s/<CONTROLLER_CONF_FILE>/antrea-controller.conf/; s/<CNI_CONFLIST_FILE>/antrea-cni.conflist/" $THIS_DIR/../build/yamls/patches/templates/kustomization.configMap.tpl.yml > kustomization.yml
$KUSTOMIZE edit add base $BASE
BASE=../configMap
cd ..

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
BASE=../osmft
cd ..

mkdir $MODE && cd $MODE
touch kustomization.yml
$KUSTOMIZE edit add base $BASE

find ../../patches/$MODE -name \*.yml -exec cp {} . \;

$KUSTOMIZE edit add patch --path agentImagePullPolicy.yml
$KUSTOMIZE edit add patch --path controllerImagePullPolicy.yml

$KUSTOMIZE build | sed 's/^\s*{{/{{/; s/\\"\({{.*}}\)\\"/"\1"/; '"s/'\({{.*}}\)'/\1/"

popd > /dev/null

rm -rf $TMP_DIR $ANTREA_DIR
