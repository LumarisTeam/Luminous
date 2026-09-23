#!/usr/bin/env bash
#
# 服务器单例部署：从镜像仓库拉取预构建镜像，复用本目录的 compose 定义起一个容器。
#
# 为什么是"拉"而不是"构建"：本项目的 Dockerfile 要在服务器上跑一次完整的 Go 构建
# （拉 golang 基础镜像 + 编译全部依赖）；而 CI（.github/workflows/build-image.yml）
# 已经在 ubuntu-latest 上构建并推送了镜像。本脚本只负责拉取与重启。
#
# 与仓库根目录 build.sh 是两条并行路径，别混着用：
#   本脚本     从镜像仓库拉 CI 构建好的不可变镜像，用 compose 起。部署默认走这条。
#   根目录     从源码构建本地镜像并起容器（走 prod.env），供服务器上没有 registry 凭据时应急。
#
# 两条路径的容器名都是 luminous，互相不能叠加：根目录脚本起的是 docker run 直接创建的
# 容器，不带 compose 标签，之后再用本脚本会因容器名冲突而失败，需要先
# `docker rm -f luminous`（脚本末尾会把这条命令打出来）。
#
# 用法：
#   ./build_from_ghcr.sh
#   ./build_from_ghcr.sh ccr.ccs.tencentyun.com/lumaris/luminous:<commit-sha>   # 指定版本，也是回滚方式
#   APP_PORT=8080 ./build_from_ghcr.sh
#   IMAGE=... APP_PORT=... COMPOSE_PROJECT_NAME=... ./build_from_ghcr.sh
#   USE_GHCR=1 ./build_from_ghcr.sh       # 改从 ghcr.io 拉（国内一般拉不动）
#   TAKE_OVER=1 ./build_from_ghcr.sh     # 同名容器是 docker run 起的时，先删掉它再起
#   sh build_from_ghcr.sh                 # /bin/sh 是 dash 时同样可用（脚本会自己切到 bash）
#
# 同目录必须有：
#   docker-compose.yml 或 docker-compose.production.yml   （两种名字都认）
#   .env                                                   （见仓库根的 .env.example）

# 服务器上最常见的调用方式是 `sh build_from_ghcr.sh`，而 Debian/Ubuntu 的 /bin/sh 是 dash：
# 它既没有 pipefail，也没有 [[ ]] / (( )) / $SECONDS / $BASH_SOURCE，会在下面那行 set
# 直接以 "Illegal option -o pipefail" 退出，连参数校验都轮不到。
# 检测到当前不是 bash 就用 bash 重新执行自己，让 `sh x.sh` 与 `./x.sh` 完全等价。
if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi

set -euo pipefail

CONTAINER_NAME="luminous"
SERVICE_NAME="luminous"

# 镜像来源。CI 把同一批 tag 双推两份：ghcr 作归档，腾讯云 TCR 供国内服务器拉取
# （ghcr 的镜像层走 pkg-containers.githubusercontent.com，在国内基本拉不动）。
# 默认走 TCR；要用 ghcr 就 USE_GHCR=1，或用 IMAGE 直接给完整镜像名。
if [[ "${USE_GHCR:-0}" == "1" ]]; then
  DEFAULT_IMAGE="ghcr.io/lumaristeam/luminous:latest"
else
  DEFAULT_IMAGE="ccr.ccs.tencentyun.com/lumaris/luminous:latest"
fi
IMAGE="${IMAGE:-${1:-$DEFAULT_IMAGE}}"

# 镜像仓库域名直接从镜像名里取：登录、登出都用它，换 registry 时不必再改别处。
REGISTRY_HOST="${IMAGE%%/*}"

APP_PORT="${APP_PORT:-23467}"
READY_TIMEOUT="${READY_TIMEOUT:-60}"

# compose 的 project 名默认取目录名。这里显式固定，否则固定 container_name 会与旧 project 撞名。
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-luminous}"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 显式导出：compose 插值优先级是 shell 环境 > 同目录 .env
export IMAGE APP_PORT COMPOSE_PROJECT_NAME

# ------------------------------------------------------------------ 前置检查

command -v docker >/dev/null 2>&1 || { echo "错误：未找到 docker。" >&2; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "错误：需要 docker compose v2 插件（docker compose，而非旧版 docker-compose）。" >&2; exit 1; }

if [[ ! "$APP_PORT" =~ ^[0-9]+$ ]] || (( APP_PORT < 1 || APP_PORT > 65535 )); then
  echo "错误：APP_PORT 必须是 1 到 65535 之间的端口号，收到：'$APP_PORT'" >&2
  exit 1
fi

COMPOSE_FILE=""
for candidate in docker-compose.yml docker-compose.yaml docker-compose.production.yml; do
  if [[ -f "$candidate" ]]; then COMPOSE_FILE="$candidate"; break; fi
done
if [[ -z "$COMPOSE_FILE" ]]; then
  echo "错误：$SCRIPT_DIR 下找不到 docker-compose.yml 或 docker-compose.production.yml。" >&2
  exit 1
fi

if [[ ! -f .env ]]; then
  cat >&2 <<'ENV_HELP'
错误：未找到 .env。compose 的 env_file 指向它，缺失时 docker compose 会直接失败。

请在当前目录创建 .env，至少包含：
  LUMINOUS_DATABASE_DSN=postgresql://user:pass@host:port/db   # 必需，缺失或连不上时进程直接退出
  LUMINOUS_AUTH_ADMIN_TOKEN=                                  # /api/v1/admin 的鉴权令牌
  LUMINOUS_SERVER_MODE=release

完整说明见仓库根目录的 .env.example。注意 .env 不要提交进 git（.dockerignore 已排除它）。
ENV_HELP
  exit 1
fi

# 固定 container_name 被既有容器占用时，compose 只抛一句
# "Conflict. The container name ... is already in use"，既不说原因也不说怎么办，
# 所以这里提前把两种情形分开讲清楚：
#   有 compose 标签但 project 不同 —— compose 不会接管别的 project 的容器
#   完全没有 compose 标签          —— 是 `docker run` 起的（仓库根目录 build.sh 那条源码构建路径）
if docker container inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
  owner="$(docker container inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$CONTAINER_NAME" 2>/dev/null || true)"
  [[ "$owner" == "<no value>" ]] && owner=""

  if [[ -z "$owner" ]]; then
    if [[ "${TAKE_OVER:-0}" == "1" ]]; then
      echo "==> 移除既有容器 ${CONTAINER_NAME}（docker run 创建，不受 compose 管理）"
      docker rm -f "$CONTAINER_NAME" >/dev/null
    else
      cat >&2 <<TAKEOVER
错误：容器 $CONTAINER_NAME 已存在，但它不带 compose 标签，也就是由 docker run 直接创建的
      ——多半来自仓库根目录 build.sh 那条源码构建路径。

compose 不会接管这种容器，直接 up 就会报：
  Conflict. The container name "/$CONTAINER_NAME" is already in use

删它之前先确认它确实是可停的旧容器：
  docker inspect -f '{{.Config.Image}}' $CONTAINER_NAME
  docker ps --filter name=$CONTAINER_NAME

确认无误后二选一：
  docker rm -f $CONTAINER_NAME     # 然后重跑本脚本
  TAKE_OVER=1 $0                   # 让本脚本替你删掉再起
TAKEOVER
      exit 1
    fi
  elif [[ "$owner" != "$COMPOSE_PROJECT_NAME" ]]; then
    cat >&2 <<CONFLICT
错误：容器 $CONTAINER_NAME 已存在，但属于另一个 compose project「${owner}」。

compose 不会接管别的 project 的容器。二选一：
  docker rm -f $CONTAINER_NAME          # 让本脚本接管
  COMPOSE_PROJECT_NAME=$owner $0   # 沿用那个 project
CONFLICT
    exit 1
  fi
fi

# ------------------------------------------------------------------ 镜像仓库登录

# 镜像若为私有，需要凭据。优先用传入的 token（用完即登出）；否则沿用本机已有的
# docker 凭据（此前手动 docker login <registry> 过就行）。
if [[ -n "${PULL_TOKEN:-}" ]]; then
  : "${PULL_USERNAME:?设置了 PULL_TOKEN 就必须同时设置 PULL_USERNAME}"
  trap 'docker logout "$REGISTRY_HOST" >/dev/null 2>&1 || true' EXIT
  printf '%s' "$PULL_TOKEN" | docker login "$REGISTRY_HOST" --username "$PULL_USERNAME" --password-stdin
fi

# ------------------------------------------------------------------ 拉取与启动

# 先记下当前在跑的镜像，末尾用它给出准确的回滚命令
previous_image="$(docker container inspect -f '{{.Config.Image}}' "$CONTAINER_NAME" 2>/dev/null || true)"

echo "==> 拉取镜像 $IMAGE"
docker compose -f "$COMPOSE_FILE" pull "$SERVICE_NAME"

echo "==> 启动容器（project=${COMPOSE_PROJECT_NAME}）"
docker compose -f "$COMPOSE_FILE" up -d --no-build --remove-orphans "$SERVICE_NAME"

# ------------------------------------------------------------------ 就绪等待

# 最常见的启动失败是 .env 里 LUMINOUS_DATABASE_DSN 缺失或库连不上：
# NewPGSchoolRepository 会 ping 并在失败时直接 os.Exit(1)，容器随即退出。
# 所以不只看容器在不在跑，还要等它真的监听起来。
echo "==> 等待服务就绪（最多 ${READY_TIMEOUT}s）"
deadline=$(( SECONDS + READY_TIMEOUT ))
ready=0
while (( SECONDS < deadline )); do
  running="$(docker container inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null || echo false)"
  if [[ "$running" != "true" ]]; then
    echo "错误：容器 $CONTAINER_NAME 已退出。日志：" >&2
    docker logs --tail 60 "$CONTAINER_NAME" >&2 || true
    exit 1
  fi

  # 用 bash 字符串匹配而不是管道 grep：pipefail 下 grep -q 提前退出会让 docker logs 吃到 SIGPIPE。
  # "Server listening" 同时也覆盖了走 TLS 时那条 "Server listening with TLS"。
  logs="$(docker logs "$CONTAINER_NAME" 2>&1 || true)"
  if [[ "$logs" == *"Server listening"* ]]; then
    ready=1
    break
  fi
  sleep 1
done

if (( ! ready )); then
  echo "错误：${READY_TIMEOUT}s 内没等到启动完成。最近日志：" >&2
  docker logs --tail 60 "$CONTAINER_NAME" >&2 || true
  exit 1
fi

# ------------------------------------------------------------------ 收尾

# 只清 dangling 镜像（不带 -a，不会动还有 tag 的镜像）。
docker image prune --force >/dev/null

digest="$(docker image inspect --format '{{index .RepoDigests 0}}' "$IMAGE" 2>/dev/null || true)"
health="$(docker container inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}无{{end}}' "$CONTAINER_NAME" 2>/dev/null || true)"

echo
echo "==> 部署完成"
echo "    容器 : $CONTAINER_NAME"
echo "    镜像 : $IMAGE"
if [[ -n "$digest" ]]; then echo "    摘要 : $digest"; fi
echo "    端口 : 宿主机 ${APP_PORT} -> 容器 8080"
echo "    健康 : ${health:-未知}（镜像自带 HEALTHCHECK，探针是 /healthz）"
echo
echo "    自检 : curl -fsS http://localhost:${APP_PORT}/healthz"
echo "    排障 : docker logs -f $CONTAINER_NAME"
echo "           docker exec -it $CONTAINER_NAME sh（基础镜像是 alpine，有 shell）"
if [[ -n "$previous_image" && "$previous_image" != "$IMAGE" ]]; then
  echo "    回滚 : $0 $previous_image"
fi
