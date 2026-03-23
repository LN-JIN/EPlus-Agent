# EPlus-Agent

> **基于大语言模型的建筑能耗仿真全自动化 Agent**
>
> 用自然语言描述建筑，自动完成 EnergyPlus 建模、仿真与能耗分析报告生成。

[![Go 1.22](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

---
<br>

## 项目背景

建筑能耗仿真是绿色建筑设计、节能改造的核心手段，但传统流程门槛极高：工程师需手动填写数百个 EnergyPlus 参数、编写 IDF 文件、排查仿真错误，往往非常耗时。

**EPlus-Agent** 将这一流程完全自动化——

- 用户只需用自然语言描述建筑（"深圳市 5 层办公楼，建筑面积 2000 m²......"）。
- Agent 自动完成意图理解 → 参数配置文件生成 → IDF 建模 → EnergyPlus 仿真 → 能耗报告撰写 → 参数敏感性分析。遇到错误时 LLM 自动诊断并修复。

<br>

本项目由两部分协作组成：

| 项目 | 语言 | 职责 |
|------|------|------|
| **EPlus-Agent**（本仓库） | Go | LLM 编排、业务流程、会话管理、报告生成 |
| **EPlus-MCP** | Python | EnergyPlus 工具服务（YAML→IDF 转换、仿真执行、IDF 读写）  |

<br>

> [!NOTE]
> 本项目旨在探索 LLM Agent 在建筑能耗仿真领域的一种落地路径，结合 ReAct 推理、多 Agent 协作与自愈修复等技术，提供一套可参考的实现思路。Agent 驱动仿真的方式并不唯一，欢迎基于此思路探索更多变体。

> 当前版本需要继续进行系统性量化验证，引入标准测试集，并开展多 LLM 对比实验，以提供更客观的参考依据。

> EPlus-MCP来自仓库 https://github.com/ITOTI-Y/EnergyPlus-Agent ，该项目提供 MCP 服务，但尚未实现 Agent，期待其下一步实现 。

---
<br>

## 功能亮点

所有 LLM 智能行为均基于统一的 `react.Agent` 底层，以 **ReAct 推理框架**为核心驱动力，将规划、工具调用与自愈能力贯穿仿真全流程。

<br>

<table>
<thead>
<tr>
<th align="center">分类</th>
<th align="center"></th>
<th>特性 / 模式</th>
<th>说明</th>
</tr>
</thead>
<tbody>
<tr>
<td rowspan="3" align="center"><b>Agent<br>推理模式</b></td>
<td align="center">🔄</td>
<td><b>ReAct</b>（思考 → 行动 → 观察）</td>
<td><b>核心模式</b>，贯穿 Phase 1 / 4 / 6；同一推理引擎，工具集按阶段独立配置</td>
</tr>
<tr>
<td align="center">🧩</td>
<td><b>Planner + 并发 Workers</b></td>
<td><b>已实现</b>，用于 Phase 6；Planner 负责规划变体方案，多 Worker 并发执行仿真</td>
</tr>
<tr>
<td align="center">🔀</td>
<td><b>Plan-Execute-Replan</b></td>
<td><b>可接入</b>；执行中动态重规划，适用于方案需在执行时动态调整的复杂场景，例如报告生成场景</td>
</tr>
<tr>
<td rowspan="3" align="center"><b>端到端<br>自动化</b></td>
<td align="center">🏗️</td>
<td><b>自然语言驱动建模</b></td>
<td>以自然语言描述建筑需求，Agent 多轮对话补全参数，自动生成完整 EnergyPlus IDF 文件，无需任何专业软件操作经验</td>
</tr>
<tr>
<td align="center">📋</td>
<td><b>国家规范合规校验</b></td>
<td>内置 GB 55015-2021《建筑节能与可再生能源利用通用规范》知识库，意图收集阶段自动核验围护结构传热系数、窗墙比、照明及设备功率密度等关键参数的合规性</td>
</tr>
<tr>
<td align="center">📊</td>
<td><b>智能能耗报告生成</b></td>
<td>仿真完成后自动解析 CSV/ESO 结果，提取年度能耗指标，LLM 生成包含数据摘要与专业分析解读的 Markdown 报告</td>
</tr>
<tr>
<td rowspan="2" align="center"><b>鲁棒性与<br>自愈能力</b></td>
<td align="center">🔧</td>
<td><b>多层自愈修复</b></td>
<td>YAML 生成、IDF 转换、仿真执行三个阶段均内置独立的 LLM 修复循环，可配置每一阶段自动重试的次数</td>
</tr>
<tr>
<td align="center">🛡️</td>
<td><b>错误分类与死循环防护</b></td>
<td>区分环境错误（不可修复）和内容错误（LLM 可修），避免在无效路径上浪费重试；连续 3 次相同错误自动终止，防止无限循环</td>
</tr>
<tr>
<td rowspan="3" align="center"><b>参数分析与<br>工程设计</b></td>
<td align="center">⚡</td>
<td><b>并发参数敏感性分析</b></td>
<td>Planner Agent 先探查 IDF 真实对象名与字段值，再规划多个参数变体；多 Worker 并发仿真（默认 3 路），汇总输出含对比表格与 AI 解读的分析报告</td>
</tr>
<tr>
<td align="center">🔌</td>
<td><b>灵活入口与断点续传</b></td>
<td>支持从任意阶段切入（UserInput / YAML / IDF / 仿真目录），会话状态逐阶段持久化，通过 <code>-resume</code> 无缝恢复中断会话</td>
</tr>
<tr>
<td align="center">🔍</td>
<td><b>全流程可观测</b></td>
<td>每阶段 ReAct 推理步骤（思考→工具调用→观察）以结构化 Markdown 完整留存，Token 消耗按阶段统计，行为完全可追溯。例如：<a href="output/react_logs/session_20260323_170727_paramanalysis_planner.md">output/react_logs/...planner.md</a></td>
</tr>
</tbody>
</table>

---
<br>

## 系统架构

```
用户自然语言输入
        │
        ▼
┌───────────────────────────────────────────────────────────┐
│                      EPlus-Agent (Go)                      │
│                                                            │
│  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐           │
│  │Phase 1 │→ │Phase 2 │→ │Phase 3 │→ │Phase 4 │→ ...      │
│  │意图收集│  │YAML 生成│  │IDF 转换│  │EPlus 仿真│         │
│  └────────┘  └────────┘  └────────┘  └────────┘           │
│                                                            │
│  ┌────────┐  ┌────────┐                                    │
│  │Phase 5 │→ │Phase 6 │                                    │
│  │报告生成│  │参数分析│                                     │
│  └────────┘  └────────┘                                    │
│                                                            │
│  核心引擎：ReAct Agent ＋ Tools Registry ＋ SessionState    │
└───────────────────────────────────────────────────────────┘
        │                        │
        ▼                        ▼
  LLM API                  EPlus-MCP 
  (OpenAI 兼容)             
                                 │
                                 ▼
                          EnergyPlus 仿真引擎
```

---
<br>


### 系统级管理

| 层面 | 实现机制 | 作用 |
|------|----------|------|
| **流程管理** | `PhaseModule` 接口 + `Orchestrator` | 六阶段统一编排，`shouldRunPhase` 判断跳过/续传，各模块解耦 |
| **上下文管理** | `IntentSummary` 跨阶段注入 System Prompt | Phase 1 生成建筑自然语言摘要后注入 Phase 2～6，LLM 全程持有建筑语义，无需重复询问 |
| **状态管理** | `SessionState` 逐阶段 JSON 持久化 | 单一数据对象贯穿全流程，记录产物路径、Token 消耗、IDF 快照历史，支持跨进程恢复 |

---
<br>

## 已实现功能

### 核心流程
- [x] 自然语言多轮对话建筑意图解析（Phase 1）
- [x] BuildingIntent 结构化数据提取（建筑信息、几何参数、围护结构、窗户、负荷、进度表、仿真设置）
- [x] GB 55015-2021 建筑节能规范动态查询（Skills 知识库：传热系数、窗墙比、照明/设备功率密度、人员密度、换气次数等 11 张参数表）
- [x] LLM 驱动的 EnergyPlus YAML 配置生成（Phase 2）
- [x] YAML 语法错误自愈修复循环（最多 5 次）
- [x] YAML → EnergyPlus IDF 自动转换（Phase 3）
- [x] IDF 转换失败时 LLM 自动修复 YAML 并重试（最多 8 次）
- [x] EnergyPlus 仿真自动执行（Phase 4）
- [x] 仿真失败时 LLM 读取错误日志、修复 IDF 并重试（最多 10 次）
- [x] 仿真结果 CSV/ESO 解析与指标提取（Phase 5）
- [x] LLM 生成 Markdown 能耗分析报告
- [x] 参数变体规划（Planner ReAct Agent）（Phase 6）
- [x] 并发多变体仿真（信号量控制，默认最多 3 Worker）
- [x] LLM 生成多变体对比分析报告

### 工程能力
- [x] 灵活入口：支持从 UserInput / YAML / IDF / SimOutDir 任意阶段切入
- [x] 会话检查点持久化与续传（`-resume <session_id>`）
- [x] 错误分类系统（环境错误 vs. 内容错误 vs. 仿真错误 vs. 瞬时错误）
- [x] `SameErrorGuard` 检测 ReAct 死循环（连续 3 次相同错误即终止）
- [x] 双输出日志（彩色控制台 + JSON 文件）
- [x] ReAct 推理步骤完整 Markdown 记录
- [x] 按阶段统计 LLM Token 消耗
- [x] Ctrl+C 优雅退出（ctx 感知信号量）
- [x] 可跳过可选阶段（`-skip-report`、`-skip-param`）

### 支持的参数化分析类型
- [x] 外墙 / 屋顶材料（Material.Thickness）
- [x] 窗户 U 值（WindowMaterial:SimpleGlazingSystem.UFactor）
- [x] 窗户太阳得热系数 SHGC
- [x] 照明功率密度（Lights.Watts_per_Zone_Floor_Area）
- [x] 设备功率密度（ElectricEquipment.Watts_per_Zone_Floor_Area）

---
<br>

## 未来计划

- [ ] **RAG 检索增强** — 提升报告生成的规范符合度
- [ ] **多气候区天气文件** — 内置主要城市 EPW，支持一键切换气候区对比
- [ ] **建筑类型模板** — 预置办公、住宅、商业、学校等典型建筑基础模板
- [ ] **Web UI** — 提供浏览器端可视化界面，展示仿真进度与报告
- [ ] **报告可视化** — 在 Markdown 报告中嵌入能耗图表（SVG/PNG）
- [ ] **IDF 几何可视化** — 生成建筑三维几何预览


---
<br>

## 快速开始

### 前置条件

| 依赖 | 版本 | 说明 |
|------|------|------|
| Go | 1.22+ | 主程序编译运行 |
| Python | 3.10+ | EPlus-MCP 工具服务 |
| EnergyPlus | 25.1.0 | 仿真引擎（需加入 PATH） |
| LLM API | — | —— |

<br>

## CLI 使用指南

```bash
# 完整流程（交互输入建筑描述）
go run cmd/main.go

# 指定建筑描述（跳过交互询问）
go run cmd/main.go -input "深圳市 3 层住宅，400 平方米"

# 同时指定参数分析目标
go run cmd/main.go -input "北京办公楼" -analysis-goal "对比外墙保温 3/5/8 cm"

# 从已有 YAML 文件开始（跳过 Phase 1-2）
go run cmd/main.go -yaml output/yaml/model.yaml

# 从已有 IDF 文件开始（跳过 Phase 1-3）
go run cmd/main.go -idf output/idf/model.idf

# 从已有仿真结果开始（直接生成报告）
go run cmd/main.go -sim-dir output/model/v1

# 续传中断的会话
go run cmd/main.go -resume session_20260323_141945

# 跳过报告生成
go run cmd/main.go -input "..." -skip-report

# 跳过参数分析
go run cmd/main.go -input "..." -skip-param

# 指定 EPW 气象文件
go run cmd/main.go -input "..." -epw /path/to/Beijing.epw
```

---
<br>

## 输出目录结构

```
output/
├── session/
│   └── <session_id>.json          # 会话状态快照（每阶段后持久化）
├── yaml/
│   └── <building_name>.yaml       # Phase 2 生成的 EnergyPlus 配置
├── idf/
│   └── <building_name>.idf        # Phase 3 生成的 IDF 文件
├── <building_name>/
│   ├── v1/                        # Phase 4 仿真结果（第 1 次尝试）
│   ├── v2/                        # Phase 4 仿真结果（修复后第 2 次）
│   └── ...
├── reports/
│   ├── <building_name>_report.md         # Phase 5 能耗分析报告
│   └── <building_name>_param_analysis.md # Phase 6 参数对比报告
├── param_analysis/
│   └── <session_id>/<timestamp>/
│       ├── baseline/sim/          # 基线仿真结果
│       ├── wall_ins_5cm/sim/      # 变体 1 仿真结果
│       └── wall_ins_8cm/sim/      # 变体 2 仿真结果
└── react_logs/
    ├── <session>_intent.md        # Phase 1 ReAct 推理记录
    ├── <session>_sim_repair.md    # Phase 4 修复推理记录
    └── <session>_paramanalysis_planner.md  # Phase 6 Planner 推理记录
```

---
<br>

## 项目结构

```
EPlus-Agent/
├── cmd/
│   └── main.go                   # CLI 入口，flag 解析
├── configs/
│   ├── config.yaml.example       # 配置模板（安全，可提交）
│   └── config.yaml               # 本地配置（含密钥，已 gitignore）
├── docs/
│   └── ARCHITECTURE.md           # 详细技术架构文档
├── internal/
│   ├── config/                   # 配置加载与环境变量覆盖
│   ├── intent/                   # Phase 1-2：意图收集与 YAML 生成
│   ├── idfconvert/               # Phase 3：YAML → IDF 转换
│   ├── simulation/               # Phase 4：EnergyPlus 仿真与修复
│   ├── report/                   # Phase 5：报告解析与生成
│   ├── paramanalysis/            # Phase 6：参数变体规划与并发仿真
│   ├── react/                    # 通用 ReAct Agent（思考→工具→观察）
│   ├── tools/                    # LLM 工具注册表
│   ├── orchestrator/             # 主流程编排
│   ├── session/                  # 会话状态与 PhaseModule 接口
│   ├── llm/                      # LLM 客户端（OpenAI 兼容 + SSE 流）
│   ├── eplusrun/                 # EPlus-MCP 子进程封装
│   ├── fault/                    # 错误分类与重试策略
│   ├── logger/                   # 双输出结构化日志
│   └── ui/                       # 终端交互与彩色输出
└── output/                       # 生成文件（gitignore）
```


---
<br>

## 依赖说明

### EPlus-MCP

EPlus-MCP 是本项目的 Python 工具后端，提供：
- `convert-idf` — 将 YAML 配置转换为 EnergyPlus IDF 文件
- `run-simulation` — 调用 EnergyPlus 执行仿真
- `edit-idf` / `read-idf` — IDF 对象读写（基于 eppy）
- `validate-idf` — IDF 合法性验证

MCP调用时出现了一些异常（尚未解决），目前EPlus-Agent 不得不通过子进程调用 `python main.py <command>`，以 stdout 传递结果。后续会修正。

### LLM API

本项目使用兼容 OpenAI Chat Completions 接口的 LLM 服务。默认配置为 **阿里云 DashScope（通义千问）**，也支持任何兼容 OpenAI 格式的服务（OpenAI、Azure OpenAI、本地 Ollama 等）。

---
<br>

## License

[MIT License](LICENSE)


