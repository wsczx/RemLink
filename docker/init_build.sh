#!/bin/sh
set -ex

if [ "$CN" = "yes" ]; then
  sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories
  export GOPROXY=https://goproxy.cn
fi

apk add build-base tzdata upx

cd /server
go mod tidy

ldflags="-s -w -X main.appVer=$appVer -X main.commitId=$commitId -X main.buildDate=$(date -Iseconds) -extldflags \"-static\" "

export CGO_ENABLED=1
go build -v -o remlink -trimpath -ldflags "$ldflags"

upx --best remlink

ls -lh /server/
/server/remlink -v
