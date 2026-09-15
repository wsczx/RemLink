#!/bin/sh
set -ex

if [ "$CN" = "yes" ]; then
  sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories
  export GOPROXY=https://goproxy.cn
fi

apk add tzdata upx

cd /server
go mod tidy

ldflags="-s -w -X main.appVer=$appVer -X main.commitId=$commitId -X main.buildDate=$(date -Iseconds)"

export CGO_ENABLED=0
go build -v -o remlink -trimpath -ldflags "$ldflags"

upx --best remlink

ls -lh /server/
/server/remlink -v
