#!/bin/bash
set -eo pipefail

function cleanup {
    if [ -v CONTAINER_TOOL ] && [ -v IMAGE_ID ]; then
        $CONTAINER_TOOL image rm -f $IMAGE_ID
    fi
    $CONTAINER_TOOL image rm -f quay.io/opdev/preflight:stable
}

trap cleanup EXIT

CONTAINER_TOOL=${CONTAINER_TOOL:-docker}
CONTAINER_REGISTRY=${CONTAINER_REGISTRY:-quay.io}
AUTH_FILE='$HOME/.docker/config.json'

if [ $CONTAINER_TOOL == 'podman' ]; then
    AUTH_FILE_SETTING="--authfile $AUTH_FILE"
fi
$CONTAINER_TOOL login $AUTH_FILE_SETTING -u $REGISTRY_LOGIN_USERNAME -p $REGISTRY_LOGIN_PASSWORD $CONTAINER_REGISTRY

$CONTAINER_TOOL pull antrea/antrea-operator:$VERSION
IMAGE_ID=$($CONTAINER_TOOL image ls | awk '/antrea-operator/{print $3}')

$CONTAINER_TOOL tag $IMAGE_ID $CONTAINER_REGISTRY/$OCP_PROJECT_NAMESPACE/$PFLT_CERTIFICATION_PROJECT_ID:$VERSION
$CONTAINER_TOOL push $CONTAINER_REGISTRY/$OCP_PROJECT_NAMESPACE/$PFLT_CERTIFICATION_PROJECT_ID:$VERSION

$CONTAINER_TOOL run \
  --rm \
  --security-opt=label=disable \
  --env PFLT_LOGLEVEL=trace \
  --env PFLT_CERTIFICATION_PROJECT_ID=$PFLT_CERTIFICATION_PROJECT_ID \
  --env PFLT_PYXIS_API_TOKEN=$PFLT_PYXIS_API_TOKEN \
  -v $HOME/.docker:/docker \
  quay.io/opdev/preflight:stable check container -s --docker-config /docker/config.json $CONTAINER_REGISTRY/$OCP_PROJECT_NAMESPACE/$PFLT_CERTIFICATION_PROJECT_ID:$VERSION

exit 0
