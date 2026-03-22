# EnergyPlus Agent — 项目概览

## 项目简介

EnergyPlus Agent 是一个基于 Go 的智能建筑能耗仿真配置助手。用户通过自然语言对话描述建筑，Agent 自动生成符合 EnergyPlus 规范的 IDF 配置文件，并通过 MCP Server 完成校验。

---

## 技术栈

| 层次 | 技术 |
|------|------|
| 语言 | Go 1.22，极少外部依赖（仅 `gopkg.in/yaml.v3`） |
| LLM | OpenAI 兼容 API（Alibaba DashScope / Qwen） |
| Agent 范式 | ReAct（Reasoning + Acting） |
| 后端通信 | MCP 协议（JSON-RPC 2.0 over HTTP） |
| 配置 | YAML + 环境变量覆盖 |
| 日志 | `log/slog` 双通道：彩色终端 + JSON 文件 |

---

## 目录结构

```
energyplus-agent/
├── cmd/
│   ├── main.go                    # 主入口
│   ├── mcp_test/main.go           # MCP 工具调试
│   ├── run_simulation_demo/       # EnergyPlus 直接仿真演示
│   └── expand_objects_demo/       # HVACTemplate 展开演示
├── internal/
│   ├── config/          # 配置加载与校验
│   ├── orchestrator/    # 四阶段流水线调度 + 状态机
│   ├── intent/          # 意图采集、YAML 生成、提示词
│   ├── react/           # ReAct 循环核心
│   ├── llm/             # LLM HTTP 客户端（含 SSE 流式）
│   ├── mcp/             # MCP 协议客户端 + 工具封装
│   ├── tools/           # 工具注册表与分发
│   ├── ui/              # CLI 输入 + 表格输出
│   ├── logger/          # 结构化日志
│   └── rag/             # RAG 接口（占位）
├── configs/config.yaml   # 运行时配置
├── output/               # 生成的 YAML/IDF 及仿真结果
├── logs/                 # 会话日志
└── doc/                  # 文档
```

---

## 核心流水线

```
用户输入
   │
   ▼
[Stage 1] 意图采集（Intent Collection）
   多轮对话 → 填充 BuildingIntent 结构体
   │
   ▼
[Stage 2] YAML 生成（YAML Generation）
   LLM 合成完整 EnergyPlus 配置 → 自愈校错循环（最多 5 次）
   │
   ▼
[Stage 3] MCP 校验（IDF Converting）
   调用 Python MCP Server：LoadYAML → ValidateConfig → GetSummary
   │
   ▼
[Stage 4] 结果展示（IDF Ready）
   终端表格 + YAML 预览
```

---

## 已完成功能

- **意图采集**：多轮对话收集 21 个建筑参数字段；支持用户二次修改确认
- **YAML 生成**：LLM 合成建筑围护结构、几何、HVAC、时间表等完整配置；自动从 U 值反推构造层厚度
- **自愈循环**：YAML 语法错误时自动让 LLM 修正，最多重试 5 次
- **MCP 集成**：完整实现 JSON-RPC 2.0 握手、工具调用、SSE 响应解析
- **状态机**：`IntentCollection → YAMLGenerating → IDFConverting → IDFReady → Done` 全阶段快照记录
- **工具注册表**：统一注册/分发 `ask_user`、`write_yaml`、`load_yaml`、`validate_config` 等工具
- **ReAct 循环**：思考 → 工具选择 → 执行 → 观察，支持最多 15 轮
- **CLI 界面**：彩色输出、阶段横幅、意图表格展示
- **日志系统**：双通道结构化日志，Debug 级别记录所有 LLM 思考过程
- **配置管理**：YAML 文件 + 环境变量（`LLM_API_KEY`、`LLM_BASE_URL` 等）

---

## 待完善 / 可优化点

### 高优先级

| 方向 | 现状 | 建议 |
|------|------|------|
| **RAG 检索增强** | `rag/interface.go` 仅有 `NoopRetriever` 占位 | 接入向量数据库，检索 EnergyPlus IDD 文档、典型案例 |
| **仿真执行集成** | `run_simulation_demo` 独立存在，未接入主流水线 | 作为 Stage 5 可选阶段，运行后直接返回能耗结果 |
| **IDF 展开集成** | `expand_objects_demo` 未接入主流水线 | 将 HVACTemplate → 显式 IDF 对象的展开纳入 Stage 3 |
| **校验错误定位** | 校验失败时错误信息缺少行号和上下文 | MCP Server 返回结构化错误位置；LLM 定点修复 |

### 中优先级

| 方向 | 现状 | 建议 |
|------|------|------|
| **复杂建筑几何** | 仅支持矩形平面 | 扩展为 L 型、多翼、中庭等复杂平面 |
| **内外区分区** | 当前每层一个 Zone | 按内区/外区分区建模，提高仿真精度 |
| **气候数据** | 提示词硬编码 5 个气候区 + 30 城市 | 外接 ASHRAE/IDD 数据库动态查询 |
| **断点续传** | 状态机支持快照但无恢复机制 | 序列化 Session 到磁盘，崩溃后可从断点继续 |
| **单元测试** | 无测试文件 | 对 YAML 生成、MCP 工具、ReAct 循环补充测试 |

### 低优先级（锦上添花）

| 方向 | 建议 |
|------|------|
| **批量处理** | 从 CSV 读取多栋建筑描述，批量生成 |
| **Web 界面** | 为非技术用户提供表单化输入替代 CLI |
| **结果可视化** | 生成建筑几何预览图、能耗平衡图 |
| **多版本 IDF** | 支持导出不同 EnergyPlus 版本格式 |

---

## 关键配置参数

```yaml
# configs/config.yaml 主要参数
llm:
  temperature: 0.2        # 低温度保证 YAML 确定性
  timeout_sec: 600        # LLM 单次最大 10 分钟
  max_react_iter: 15      # ReAct 最大轮数
  max_heal_iter: 5        # 自愈最大重试次数
mcp:
  timeout_sec: 60         # MCP 工具调用超时
```

---

## 项目优势与局限

**优势**
- 极少外部依赖，部署简单
- ReAct + 自愈双重保障，对 LLM 输出容错性强
- 领域知识内化于 1400+ 行系统提示词，无需外部知识库即可运行
- 模块边界清晰，易于扩展

**局限**
- 依赖 LLM 预训练知识，缺乏 RAG 支撑时对生僻工况泛化能力有限
- 无持久化存储，中断即丢失当前会话
- 仅支持交互式单次运行，暂不支持批量/自动化场景
- 建筑几何过于简化，不适用于异形或复杂平面建筑
