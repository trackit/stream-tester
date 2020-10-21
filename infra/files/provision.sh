#!/bin/bash

set -euo pipefail

fileroot="/tmp/stream_test"

dependencies() {
    apt-get update
    apt-get install -y ffmpeg rtmpdump python2-minimal
    curl -o /tmp/golang174.tgz https://storage.googleapis.com/golang/go1.7.4.linux-amd64.tar.gz
    tar -C /usr/local -xzf /tmp/golang174.tgz
    cat >> /etc/profile <<EOF
export PATH=\$PATH:/usr/local/go/bin
EOF
    export PATH=$PATH:/usr/local/go/bin
}

configssh() {
    cat "$fileroot/infra/files/id_rsa.pub" >> /home/ubuntu/.ssh/authorized_keys
    cp "$fileroot/infra/files/id_rsa" /home/ubuntu/.ssh/id_rsa
}

build() {
    rm -rf /tmp/goroot/src/github.com/trackit
    mkdir -p /tmp/goroot/src/github.com/trackit
    cp -r "$fileroot" /tmp/goroot/src/github.com/trackit/stream-tester
    pushd /tmp/goroot/src/github.com/trackit/stream-tester
    pwd
    find .
    GOPATH=/tmp/goroot go get
    GOPATH=/tmp/goroot go build -o stream_test
    mv 640.flv /home/ubuntu/
    mv stream_test /usr/local/bin/stream_test
    mv display.py /usr/local/bin/display.py
    mv infra/files/runtest /usr/local/bin/runtest
    mv infra/files/autoshutdown.sh /usr/local/bin/
    mv infra/files/autoshutdown.service /etc/systemd/system/
    chmod +x /usr/local/bin/display.py\
             /usr/local/bin/runtest\
             /usr/local/bin/autoshutdown.sh
    systemctl enable autoshutdown
    popd
}

dependencies
configssh
build