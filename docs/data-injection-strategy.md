# 数据注入策略：建筑节能规范 → EnergyPlus Agent

## 背景

`data/` 目录下有来自《建筑节能与可再生能源利用通用规范》（GB 55015-2021）的数据：
- **C.0.6 条款**（运行参数）：空调供暖运行时间、照明功率密度、人均面积、换气次数、设备功率密度
- **C.0.1 条款**（围护结构）：各气候区传热系数 K 值、SHGC、窗墙比限值

未来数据规模：10~50 个文件，包含更多建筑节能规范 + EnergyPlus 技术文档。

---

## 方案选型辨析

### 为什么不用 Go map

将规范数据硬编码为 Go struct/map：
- 新增一张表 → 需要修改 Go 文件 → 需要重新编译部署
- 规范值更新 → 同上
- **数据不应该活在业务逻辑代码里**

### 为什么不用 RAG（针对规范表格）

RAG（向量检索）适合语义相似度查询。规范表格是多维精确查找表，RAG 无法保证查询精确性，且为 12.5 KB 数据搭建向量数据库基础设施成本过高。RAG 应留给大体量非结构化技术文档（EnergyPlus Input-Output Reference 等）。

### 为什么不静态全量注入

启动时读取所有数据文件注入 System Prompt：当前 ~3000 tokens 可接受，但未来 50 张表后 token 膨胀不可控。更重要的是，LLM 始终携带大量可能不相关的规范数据。

---

## 采用方案：文件树 + Skill（On-Demand 查询）

### 核心原则

1. 数据文件放在 skill 的 `references/` 子目录下，按**表格名**命名
2. LLM 在推理过程中**遇到不确定时主动查阅**
3. 使用 **grep/glob/read 工具**精确搜索 reference 文件

### 目录结构

```
skills/
└── query_standard/
    ├── SKILL.md                    ← skill 定义：指令 + 工具使用说明
    └── references/                 ← 参考数据（每张表一个文件，按表格名命名）
        ├── 围护结构传热系数基本要求.md
        ├── 公共建筑透光围护结构传热系数和太阳得热系数.md
        ├── 居住建筑和工业建筑透光围护结构太阳得热系数基本要求.md
        ├── 严寒和寒冷地区居住建筑窗墙面积比基本要求.md
        ├── 夏热冬冷和夏热冬暖地区窗墙面积比及对应外窗传热系数.md
        ├── 空气调节和供暖系统的日运行时间.md
        ├── 照明功率密度值.md
        ├── 不同类型房间人均占有的建筑面积.md
        ├── 居住建筑的换气次数.md
        ├── 工业建筑的换气次数.md
        └── 不同类型房间电器设备功率密度.md
```

### 工作机制

**启动时**（轻量加载）：
```
SkillLoader 扫描 skills/ 目录
    → 解析 SKILL.md 的 frontmatter（name, description, phase）
    → 将 SKILL.md 正文（查询指令）注入对应阶段的 System Prompt
    → 告知 LLM references 目录路径
    （不读取 reference 文件内容，基础开销极低）
```

**推理时**（按需查阅）：
```
LLM 推理围护结构 U 值时不确定
    → 调用 search_standard("严寒.*外墙", "skills/query_standard/references/")
        → 返回匹配行（文件名 + 内容）
    → 或调用 list_references() 查看所有可用表格
    → 或调用 read_reference("围护结构传热系数基本要求.md") 读取完整表格
    → LLM 提取精确数值
```

### 与其他方案对比

| 方面 | Go map | 静态全量注入 | On-Demand 查询（本方案）|
|------|--------|------------|------------------------|
| 新增表格 | 改代码 | 加文件 | **加文件** |
| 更新规范 | 改代码 | 改文件 | **改文件** |
| 基础 token 开销 | 无 | 全量 | **极低（~200 tokens）** |
| 数据精确度 | 高（人工录入） | 中（LLM 读表） | **中（工具返回精确文本）** |
| 可扩展性 | 差 | 一般 | **好** |

---

## 实现组件

### SKILL.md 格式

```yaml
---
name: query_standard
description: 查询中国建筑节能规范（GB 55015-2021）设计参数
phase: intent
references_dir: skills/query_standard/references
---

当你对某个建筑参数的标准值不确定时，按以下步骤查阅规范：

1. 调用 `list_references` 查看可用的参考表格清单
2. 调用 `search_standard(pattern="关键词")` 精确搜索（如 "严寒 外墙"）
3. 调用 `read_reference(filename)` 读取完整表格内容

各表格内容索引：
- 围护结构传热系数基本要求.md → 各气候区外墙/屋面/楼板传热系数 K 值
- 公共建筑透光围护结构传热系数和太阳得热系数.md → 窗户 K 值和 SHGC
- ...（其余表格说明）
```

### Go Skill 加载器

```
internal/skills/
├── skill.go       # Skill 结构体（Name, Description, Phase, ReferencesDir, Instructions）
└── loader.go      # 扫描 skills/ 目录，解析 SKILL.md frontmatter，按 phase 分组
```

- 启动时只读取 SKILL.md（轻量）
- References 目录路径通过 frontmatter 声明，不预加载内容

### 注册的文件查询工具（在 internal/intent/collector.go）

| 工具名 | 参数 | 功能 |
|--------|------|------|
| `list_references` | `dir` | 列出 references 目录所有 .md 文件名及首行（表格标题）|
| `search_standard` | `pattern`, `dir` | 在 references 目录中 grep 搜索，返回文件名+匹配行 |
| `read_reference` | `path` | 读取指定文件的完整内容 |

---

## 两类数据的处理策略

| 数据类型 | 规范表格（结构化）| EnergyPlus 技术文档（非结构化，大体量）|
|---------|----------------|---------------------------------------|
| 位置 | `skills/query_standard/references/` | `skills/rag_lookup/references/` |
| 查询机制 | grep/glob/read 精确工具 | RAG 向量检索（实现 `internal/rag/Retriever` 接口）|
| 维护方式 | 编辑 `.md` 文件 | 重新运行 `cmd/index` 建索引 |

---

## 可扩展性

| 操作 | 所需改动 |
|------|---------|
| 新增规范表格 | 在 `references/` 加 `.md` 文件，**无需改代码** |
| 更新规范值 | 编辑对应 `.md` 文件，**无需改代码** |
| 新增整套规范 | 新建 `skills/<name>/references/` + `SKILL.md` |
| 新增 EnergyPlus 技术文档 | 加文件 + 运行 `cmd/index` 重建索引 |

---

## 实施步骤

### Phase 1（当前）

1. 将 `data/建筑节能与可再生能源利用通用规范/Part1.md` 和 `Part2.md` 按表拆分，迁移到 `skills/query_standard/references/`（按表格名命名）
2. 新建 `skills/query_standard/SKILL.md`
3. 新建 `internal/skills/skill.go` 和 `loader.go`
4. 在 `internal/intent/collector.go` 注册 3 个文件查询工具
5. 修改 `internal/intent/prompts.go`：注入 skill 指令，移除硬编码散值
6. 修改 `internal/orchestrator/orchestrator.go`：初始化 SkillLoader

### Phase 2（后续）

7. 新建 `internal/rag/local_retriever.go`：实现 `Retriever` 接口（本地余弦相似度）
8. 新建 `cmd/index/main.go`：离线建索引脚本
9. 新建 `skills/rag_lookup/SKILL.md`：声明使用 RAG 检索，phase=simulation/report

---

## 验证

1. `go build ./...` 编译无误
2. 修改某个 `.md` 文件中的值，重启 Agent，验证 LLM 查到修改后的值（无需重新编译）
3. 在 `references/` 新增一个文件，验证 `list_references` 工具能发现它
4. 输入"北京5层公共建筑"，观察 LLM 是否主动调用 `search_standard` 或 `read_reference` 查阅规范值
