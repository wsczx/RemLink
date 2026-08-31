# RemLink Dockerfile
# 需要先编译前端（bash scripts/build_web.sh）再执行 docker build

# 编译阶段
FROM golang:1.26-alpine3.22 AS builder

ARG CN="no"
ARG appVer="appVer"
ARG commitId="commitId"

ENV TZ=Asia/Shanghai

WORKDIR /server
COPY docker/init_build.sh /tmp/
COPY server/ /server/
COPY web/ui  /server/ui

RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build sh /tmp/init_build.sh

# 运行阶段
FROM alpine:3.22
LABEL maintainer="github.com/wsczx"

ARG CN="no"

ENV TZ=Asia/Shanghai
ENV REMLINK_IN_CONTAINER="on"

WORKDIR /app
COPY docker/init_release.sh /tmp/
COPY --from=builder /server/remlink /app/
COPY docker/docker_entrypoint.sh server/bridge-init.sh /app/

RUN sh /tmp/init_release.sh

EXPOSE 443 8800 443/udp

ENTRYPOINT ["/app/docker_entrypoint.sh"]
