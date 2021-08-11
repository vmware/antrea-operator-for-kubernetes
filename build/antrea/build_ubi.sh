#! /bin/bash

set -e

_usage="Usage: $0 [--tag <ImageTag>] [--antrea-version <AntreaVersion>] [--ovs-version <Version>] 
                  [--cni-version <Version> ]
Generate Antrea UBI image.
       --tag                                                   Specify the image tag.
       --antrea-version                                        Specify Antrea version.
       --ovs-version                                           Specify the OVS version.
       --cni-version                                           Specify the CNI version.
       --help, -h                                              Print usage message.
"

function print_usage {
    echo "$_usage"
}

while [[ $# -gt 0 ]]
do
key="$1"
case $key in
    --tag)
    IMAGE_TAG="$2"
    shift 2
    ;;
    --antrea-version)
    ANTREA_VERSION="$2"
    shift 2
    ;;
    --ovs-version)
    OVS_VERSION="$2"
    shift 2
    ;;
    --cni-version)
    CNI_VERSION="$2"
    shift 2
    ;;
    -h|--help)
    print_usage
    exit 0
    ;;
    *)    # unknown option
    echo "Unknown option $1, try '$0 --help' for more information."
    exit 1
    ;;
esac
done

if [ -z "${IMAGE_TAG}" ]; then
  IMAGE_TAG="latest"
  echo "Using default image tag ${IMAGE_TAG}."
fi

if [ -z "${ANTREA_VERSION}" ]; then
  ANTREA_VERSION="1.2.1"
  echo "Using default Antrea version ${ANTREA_VERSION}."
fi

if [ -z "${OVS_VERSION}" ]; then
  OVS_VERSION="2.14.2"
  echo "Using default OVS version ${OVS_VERSION}."
fi

if [ -z "${CNI_VERSION}" ]; then
  CNI_VERSION="V0.8.7"
  echo "Using default CNI version ${CNI_VERSION}."
fi

wget https://github.com/antrea-io/antrea/archive/refs/tags/v${ANTREA_VERSION}.tar.gz && 
tar -xzvf v${ANTREA_VERSION}.tar.gz

docker build -t antrea/ovs-ubi:${OVS_VERSION} --build-arg OVS_VERSION=${OVS_VERSION} -f ./ovs/Dockerfile.ubi ./ovs
docker build -t antrea/base-ubi:${OVS_VERSION} --build-arg OVS_VERSION=${OVS_VERSION} --build-arg CNI_BINARIES_VERSION=${CNI_VERSION} -f ./base/Dockerfile.ubi ./base
docker build -t antrea/antrea-ubi:${IMAGE_TAG} --build-arg OVS_VERSION=${OVS_VERSION} -f ./Dockerfile.ubi ./antrea-${ANTREA_VERSION}
