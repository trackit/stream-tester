#!/bin/bash

set -eou pipefail
pushd $(dirname "$0")/..

revision=$(git rev-parse ${1:-HEAD})
git archive $revision | gzip > /tmp/streamtesterbuild.tgz

pushd infra

nodeips=$(terraform output | grep -Eo '(\d{1,3}[.]){3}\d{1,3}')
nodehosts=$(echo "$nodeips" | sed 's|^|ubuntu@|')

dossh() {
    ssh -o StrictHostKeyChecking=no\
        -o UserKnownHostsFile=/dev/null\
        -i files/id_rsa\
        $1\
        -- "${@:2}"
}


for nodehost in $nodehosts
do
    cat /tmp/streamtesterbuild.tgz | dossh $nodehost dd of=/tmp/stream_test.tgz
    dossh $nodehost mkdir -p /tmp/stream_test
    dossh $nodehost tar -C /tmp/stream_test -xzf /tmp/stream_test.tgz
    dossh $nodehost find /tmp/stream_test
    dossh $nodehost chmod +x /tmp/stream_test/infra/files/provision.sh
    dossh $nodehost sudo /tmp/stream_test/infra/files/provision.sh
done
