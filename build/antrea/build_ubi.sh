#! /bin/bash

set -e

_usage="Usage: $0 [--tag <ImageTag>] [--antrea-version <AntreaVersion>] 
Generate Antrea UBI image.
       --tag                                                   Specify the ip of vc.
       --antrea-version                                        Specify the password of vc.
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
  ANTREA_VERSION="0.13.1"
  echo "Using default Antrea version ${ANTREA_VERSION}."
fi

wget https://github.com/antrea-io/antrea/archive/refs/tags/v${ANTREA_VERSION}.tar.gz && 
tar -xzvf v${ANTREA_VERSION}.tar.gz

docker build -t antrea/ovs-ubi:2.14.0 -f ./ovs/Dockerfile.ubi ./ovs
docker build -t antrea/base-ubi:2.14.0 -f ./base/Dockerfile.ubi ./base
docker build -t antrea/antrea-ubi:${IMAGE_TAG} -f ./Dockerfile.ubi ./antrea-${ANTREA_VERSION}
