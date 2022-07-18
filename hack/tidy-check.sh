#!/bin/bash

# Copyright 2019 Antrea Authors
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

set +e
trap cleanup EXIT

THIS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

pushd "$(dirname "$THIS_DIR")" >/dev/null || exit

PROJECT_RELATIVE_DIR=${1:-.}
PROJECT_DIR=$(dirname "$THIS_DIR")/$PROJECT_RELATIVE_DIR

MOD_FILE="$PROJECT_DIR/go.mod"
SUM_FILE="$PROJECT_DIR/go.sum"
TMP_DIR="$THIS_DIR/.tmp.tidy-check"
TMP_MOD_FILE="$TMP_DIR/go.mod"
TMP_SUM_FILE="$TMP_DIR/go.sum"

# if Go environment variable is set, use it as it is, otherwise default to "go"
: "${GO:=go}"
: "${TARGET_GO_VERSION:=1.16}"

TIDY_COMMAND="cd $PROJECT_RELATIVE_DIR && $GO mod tidy >> /dev/null 2>&1"

function echoerr {
    echo >&2 "$@"
}

function general_help {
    echoerr "Please run the following command to generate a new go.mod & go.sum:"
    echoerr "  \$ make tidy"
}

function precheck {
    if [ ! -r "$MOD_FILE" ]; then
        echoerr "no go.mod found"
        general_help
        exit 1
    fi
    if [ ! -r "$SUM_FILE" ]; then
        echoerr "no go.sum found"
        general_help
        exit 1
    fi
    mkdir -p "$TMP_DIR"
}

function tidy {
    cp "$MOD_FILE" "$TMP_MOD_FILE"
    mv "$SUM_FILE" "$TMP_SUM_FILE"

    if [ -n "$GO" ]; then
        /bin/bash -c "$TIDY_COMMAND"
    else
        docker run --rm -u "$(id -u):$(id -g)" \
            -e "GOCACHE=/tmp/gocache" \
            -e "GOPATH=/tmp/gopath" \
            -w /usr/src/vmware/antrea-operator-for-kubernetes \
            -v "$(dirname "$THIS_DIR"):/usr/src/vmware/antrea-operator-for-kubernetes" \
            golang:$TARGET_GO_VERSION bash -c "$TIDY_COMMAND"
    fi
}

function cleanup {
    if [ -f "$TMP_MOD_FILE" ]; then
        mv "$TMP_MOD_FILE" "$MOD_FILE"
    fi
    if [ -f "$TMP_SUM_FILE" ]; then
        mv "$TMP_SUM_FILE" "$SUM_FILE"
    fi
    if [ -d "$TMP_DIR" ]; then
        rm -fr "$TMP_DIR"
    fi
}

function failed {
    echoerr "'go mod tidy' failed, there are errors in dependencies"
    general_help
    exit 1
}

function check {
    MOD_DIFF=$(diff "$MOD_FILE" "$TMP_MOD_FILE")
    SUM_DIFF=$(diff "$SUM_FILE" "$TMP_SUM_FILE")
    if [ -n "$MOD_DIFF" ] || [ -n "$SUM_DIFF" ]; then
        echo "=== go.mod diff ==="
        echo $MOD_DIFF
        echo "=== go.sum diff ==="
        echo $SUM_DIFF

        echoerr "dependencies are not tidy"
        general_help
        exit 1
    fi
}

precheck
if tidy; then
    check
else
    failed
fi

popd >/dev/null || exit
