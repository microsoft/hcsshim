#!/bin/bash

# Requires https://github.com/vbatts/git-validation. This is run in the hcsshim CI to validate that
# commits are signed off, a reasonable subject length is adhered to, and there's no dangling whitespace
# in the changes.

echo "DCO checks"

set -x
if [ -z "${GITHUB_COMMIT_URL}" ]; then
    DCO_RANGE=$(jq -r '.after + "..HEAD"' ${GITHUB_EVENT_PATH})
else
    DCO_RANGE=$(curl ${GITHUB_COMMIT_URL} | jq -r '.[0].parents[0].sha + "..HEAD"')
fi

range=
commit_range="${DCO_RANGE-}"
[ ! -z "${commit_range}" ] && {
    range="-range ${commit_range}"
}

git-validation -v ${range} -run DCO,short-subject,dangling-whitespace