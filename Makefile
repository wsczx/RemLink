.PHONY: all web build docker local local-noupx run test clean release release-l

VERSION := $(shell cat version)

## 构建前端（Docker node:16-alpine）
web:
	bash scripts/build_web.sh

## 构建前端并构建 Docker 镜像
build: web
	bash scripts/build_docker.sh

## 构建 Docker 镜像
docker:
	bash scripts/build_docker.sh

## 本地编译二进制（musl 静态链接 + UPX）
local:
	bash scripts/build_local.sh

## 本地编译二进制（不压缩）
local-noupx:
	bash scripts/build_local.sh noupx

## 编译并运行（需先 make web）
run: local-noupx
	cd server && sudo ./remlink

## 运行测试
test:
	cd server && go test ./...

## 构建 Go 后端（不依赖 Docker，需先 make web）
go:
	cd server && go build -o remlink -trimpath

## 清理构建产物
clean:
	rm -rf server/ui web/ui server/remlink remlink-deploy remlink-deploy-* artifact-dist

## 本地发布（绕过 GitHub Actions，需 gh 登录 + docker 登录）
##   ARCH=linux/amd64 make release        仅 amd64
##   make release-l                       同时打 latest 镜像标签
release:
	bash scripts/local-release.sh $(ARGS)

## 本地发布并打 latest 镜像标签
release-l:
	bash scripts/local-release.sh -l

## 查看帮助
help:
	@echo "RemLink 构建命令:"
	@echo ""
	@echo "  make web        构建前端"
	@echo "  make build      构建前端 + Docker 镜像"
	@echo "  make docker     构建 Docker 镜像"
	@echo "  make local      本地编译二进制（musl + UPX）"
	@echo "  make local-noupx 本地编译二进制（不压缩）"
	@echo "  make go         快速编译 Go 后端（需先 make web）"
	@echo "  make run        编译并运行"
	@echo "  make test       运行测试"
	@echo "  make clean      清理构建产物"
	@echo "  make release    本地发布（需 gh + docker 登录，ARCH/ARGS 可传参）"
	@echo "  make release-l  本地发布并打 latest 镜像标签"
	@echo "  make help       显示帮助"
	@echo ""
	@echo "当前版本: $(VERSION)"

all: build
