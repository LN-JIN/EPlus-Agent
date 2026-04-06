# ── Stage 1: 编译 Go 二进制 ──────────────────────────────────────
FROM golang:1.22-bookworm AS go-builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o eplus-agent ./cmd/main.go

# ── Stage 2: 运行时镜像（EnergyPlus 25.1 + Python + Go）─────────
FROM nrel/energyplus:25.1.0

# 安装 uv（直接从官方镜像复制，最快且无网络依赖）
COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /bin/

WORKDIR /app

ENV UV_CACHE_DIR=/app/.cache/uv \
    UV_PYTHON_INSTALL_DIR=/app/.local/share/uv/python

# Energy+.idd 直接从 EnergyPlus 镜像内复制，不进构建上下文（节省 4 MB）
RUN mkdir -p pytools/data/dependencies && \
    cp /EnergyPlus-*/Energy+.idd pytools/data/dependencies/

# 先复制 pyproject.toml，利用 Docker layer 缓存：依赖不变时跳过 uv sync
COPY pytools/pyproject.toml ./pytools/
RUN cd pytools && uv sync

# Python 源码（改动频率更高，放后面避免破坏依赖层缓存）
COPY pytools/main.py ./pytools/
COPY pytools/src ./pytools/src

# Go 二进制
COPY --from=go-builder /build/eplus-agent ./

# Agent 配置、RAG 索引与技能定义
COPY configs ./configs
COPY data ./data
COPY skills ./skills

# 运行时输出目录
RUN mkdir -p output logs

# ── 容器内固定路径（覆盖 config.yaml 中的 Windows 路径）──────────
ENV SESSION_SIMULATION_SCRIPT=/app/pytools/main.py \
    SESSION_PYTHON_PATH=/app/pytools/.venv/bin/python3
# SESSION_EPW_PATH 由外部 .env / docker-compose env_file 提供，保持灵活

ENTRYPOINT ["/app/eplus-agent", "-config", "/app/configs/config.yaml"]
