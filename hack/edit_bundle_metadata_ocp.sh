BUNDLE_METADATA=${1:-./bundle/metadata/annotations.yaml}
BUNDLE_DOCKERFILE=${2:-./bundle.Dockerfile}
OCP_VER_ANN_KEY="com.redhat.openshift.versions"
OCP_VER_LABEL_KEY="LABEL "${OCP_VER_ANN_KEY}
OCP_VER_VALUE="v4.9-v4.12"
OCP_DELIVERY_ANN_KEY="com.redhat.delivery.operator.bundle"
OCP_DELIVERY_LABEL_KEY="LABEL "${OCP_DELIVERY_ANN_KEY}
OCP_DELIVERY_VALUE="true"
OCP_BUNDLE_CHANNEL_DEFAULT_LABEL_KEY="operators.operatorframework.io.bundle.channel.default.v1"
OCP_BUNDLE_CHANNEL_DEFAULT_VALUE="alpha"

if [[ ! -f ${BUNDLE_METADATA} ]]; then
    echo "Bundle metadata file "$BUNDLE_METADATA" does not exist"
    exit 1
fi
if [[ ! -f ${BUNDLE_DOCKERFILE} ]]; then
    echo "Bundle Dockerfile "$BUNDLE_DOCKERFILE" does not exist"
    exit 1
fi


function replace_or_append() {
    # the script checks if the annotation already exists
    KEY=$1
    VALUE=$2
    TARGET=$3
    SEP=$4
    INDENT=${5:-0}
    if grep -q "${KEY}" ${TARGET}; then
        if grep -qE "${KEY}.*=.*${VALUE}" ${TARGET}; then
            echo "Nothing to append. Key "${KEY}" already present with value "${VALUE}
        else
            echo "Delete and append "${KEY}
            sed -i "/.*$KEY/d" ${TARGET}
            printf "%*s%s\n" ${INDENT} "" "${KEY}${SEP}${VALUE}" >> $TARGET
        fi
    else
        echo "Append "${KEY}
        printf "%*s%s\n" ${INDENT} "" "${KEY}${SEP}${VALUE}" >> $TARGET
    fi
}

function replace_or_append_ann() {
    replace_or_append "$1" "$2" "$3" ": " "2"
}

function replace_or_append_label(){
    replace_or_append "$1" "$2" "$3" "="
}

# append annotations to bundle metadata
replace_or_append_ann "${OCP_VER_ANN_KEY}" "${OCP_VER_VALUE}" "${BUNDLE_METADATA}"
replace_or_append_ann "${OCP_DELIVERY_ANN_KEY}" "${OCP_DELIVERY_VALUE}" "${BUNDLE_METADATA}"
replace_or_append_ann "${OCP_BUNDLE_CHANNEL_DEFAULT_LABEL_KEY}" "${OCP_BUNDLE_CHANNEL_DEFAULT_VALUE}" "${BUNDLE_METADATA}"
# append annotations to bundle dockerfile
replace_or_append_label "${OCP_VER_LABEL_KEY}" "${OCP_VER_VALUE}" "${BUNDLE_DOCKERFILE}"
replace_or_append_label "${OCP_DELIVERY_LABEL_KEY}" "${OCP_DELIVERY_VALUE}" "${BUNDLE_DOCKERFILE}"
