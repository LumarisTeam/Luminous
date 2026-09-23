#!/usr/bin/env bash
#
# 根目录源码构建：git pull -> docker build -> 换掉旧容器。
# 与 deploy/build_from_ghcr.sh 是两条并行路径，别混着用：
#
#   本脚本      本机从源码构建镜像并起容器。适合服务器上没有 ghcr 凭据、或就是要跑当前工作区代码。
#   deploy/     从 ghcr 拉 CI 构建好的不可变镜像，用 compose 起。部署默认走这条。
#
# 两条路径的容器名都是 luminous，互相不能叠加：本脚本起的是 docker run 直接创建的容器，
# 不带 compose 标签，之后再用 deploy/build_from_ghcr.sh 会因容器名冲突而失败，
# 需要先 `docker rm -f luminous`。
#
# 用法：
#   ./build.sh
#   PART=8080 ./build.sh

# 服务器上也可能以 `sh build.sh` 调用，而 Debian/Ubuntu 的 /bin/sh 是 dash：
# 它没有 [[ ]] / (( )) / $SECONDS，会在第一次用到时就报错退出。
# 检测到当前不是 bash 就用 bash 重新执行自己，让 `sh x.sh` 与 `./x.sh` 完全等价。
if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi

PART="${PART:-${1:-23467}}"
if [[ ! "$PART" =~ ^[0-9]+$ ]] || (( PART < 1 || PART > 65535 )); then
  echo "错误：PART 必须是 1 到 65535 之间的端口号。"
  exit 1
fi

git pull
sudo docker stop luminous
sudo docker rm luminous
sudo docker build -t luminous .
if [ ! -f ./prod.env ]; then
  echo "错误：未找到 ./prod.env，请先创建该文件再运行。"
  exit 1
fi
sudo docker run -d   --name luminous   -p "${PART}:8080"   --env-file ./prod.env luminous:latest
