#!/usr/bin/env bash

# Copyright 2021 Antrea Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# Usage: VERSION=v1.0.0 ./prepare-assets.sh <output dir>

set -eo pipefail

function echoerr {
    >&2 echo "$@"
    exit 1
}

if [ -z "$VERSION" ]; then
    echoerr "Environment variable VERSION must be set"
fi

if [ -z "$1" ]; then
    echoerr "Argument required: output directory for assets"
fi

THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
pushd $THIS_DIR/.. > /dev/null

source ./hack/get-kustomize.sh
kustomize=$(check_or_install_kustomize)

mkdir -p "$1"
OUTPUT_DIR=$(cd "$1" && pwd)

OPERATOR_IMG_NAME="antrea/antrea-operator"
# Prepare tarball with deployment yaml
# Each platform specified here must match a subdirector of the 'deploy' directory
OPERATOR_PLATFORMS=(
    "ocp4"
)

for platform in "${OPERATOR_PLATFORMS[@]}"; do
    mkdir -p ${OUTPUT_DIR}/${platform}
    cp deploy/${platform}/*.yaml ${OUTPUT_DIR}/${platform}
    pushd ${OUTPUT_DIR} > /dev/null
    pushd ${platform} > /dev/null
    # erase anything that might already be in the kustomization file
    echo "" > kustomization.yaml
    $kustomize edit add base operator.yaml
    $kustomize edit set image ${OPERATOR_IMG_NAME}:${VERSION}
    $kustomize build > operator_tmp.yaml
    mv operator_tmp.yaml operator.yaml
    rm kustomization.yaml
    popd > /dev/null
    tar czf ${platform}.tar.gz ${platform}/*.yaml
    rm -rf ${platform}
    popd > /dev/null
done

# Prepare the bundle image
make bundle-build
docker save antrea/antrea-operator-bundle > ${OUTPUT_DIR}/bundle-image.tar
gzip ${OUTPUT_DIR}/bundle-image.tar

ls "$OUTPUT_DIR" | cat
