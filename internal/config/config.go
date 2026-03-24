// 配置加载模块
// 负责从 configs/config.yaml 读取应用配置，并支持通过环境变量覆盖敏感字段（如 API Key）。
// 所有模块共用同一个 Config 实例，由 main.go 初始化后注入。

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LLMConfig 大语言模型连接配置
type LLMConfig struct {
	BaseURL     string  `yaml:"base_url"`
	APIKey      string  `yaml:"api_key"`
	Model       string  `yaml:"model"`
	TimeoutSec  int     `yaml:"timeout_sec"`
	Temperature float64 `yaml:"temperature"`
}

// MCPConfig MCP Server 连接配置（仅用于 MCP HTTP 协议）
type MCPConfig struct {
	BaseURL        string `yaml:"base_url"`
	TimeoutSec     int    `yaml:"timeout_sec"`
	InitTimeoutSec int    `yaml:"init_timeout_sec"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level       string `yaml:"level"`
	File        string `yaml:"file"`
	Console     bool   `yaml:"console"`
	ReactLog    bool   `yaml:"react_log"`     // 是否记录 ReAct 推理步骤
	ReactLogDir string `yaml:"react_log_dir"` // ReAct 日志目录
}

// SessionConfig 会话运行参数
type SessionConfig struct {
	OutputDir        string `yaml:"output_dir"`
	SimulationScript string `yaml:"simulation_script"` // EPlus-MCP main.py 路径
	EPWPath          string `yaml:"epw_path"`           // 默认 EPW 气象文件路径
	PythonPath       string `yaml:"python_path"`        // Python 解释器路径，留空则自动探测（python → python3）
}

// ModuleConfig 单个模块的运行限制配置
type ModuleConfig struct {
	MaxReactIter    int `yaml:"max_react_iter"`    // ReAct 最大迭代次数
	MaxHealAttempts int `yaml:"max_heal_attempts"` // 自愈最大尝试次数
	MaxFixAttempts  int `yaml:"max_fix_attempts"`  // 修复最大尝试次数
	MaxWorkers      int `yaml:"max_workers"`        // 并行 Worker 最大数量
}

// ModulesConfig 各模块独立配置
type ModulesConfig struct {
	Intent        ModuleConfig `yaml:"intent"`
	YAMLGenerate  ModuleConfig `yaml:"yaml_generate"`
	IDFConvert    ModuleConfig `yaml:"idf_convert"`
	Simulation    ModuleConfig `yaml:"simulation"`
	ParamAnalysis ModuleConfig `yaml:"param_analysis"`
}

// RAGConfig RAG 问答工具配置
type RAGConfig struct {
	IndexPath           string `yaml:"index_path"`            // .idx 文件路径
	EmbeddingModel      string `yaml:"embedding_model"`        // Embedding 模型名（如 text-embedding-v4）
	EmbeddingDim        int    `yaml:"embedding_dim"`          // 向量维度（如 1024）
	TopK                int    `yaml:"top_k"`                  // 检索返回 Parent chunk 数量
	HyDEEnabled         bool   `yaml:"hyde_enabled"`           // 是否开启 HyDE 双路检索
	EmbeddingTimeoutSec int    `yaml:"embedding_timeout_sec"`  // Embedding API 超时秒数
}

// Config 全局配置根节点
type Config struct {
	LLM     LLMConfig     `yaml:"llm"`
	MCP     MCPConfig     `yaml:"mcp"`
	Log     LogConfig     `yaml:"log"`
	Session SessionConfig `yaml:"session"`
	Modules ModulesConfig `yaml:"modules"`
	RAG     RAGConfig     `yaml:"rag"`
}

// Load 从指定路径加载配置文件，并用环境变量覆盖敏感值
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败 [%s]: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 环境变量优先级最高，覆盖配置文件中的值
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("MCP_BASE_URL"); v != "" {
		cfg.MCP.BaseURL = v
	}

	// 设置合理的默认值
	if cfg.LLM.TimeoutSec <= 0 {
		cfg.LLM.TimeoutSec = 120
	}
	if cfg.MCP.TimeoutSec <= 0 {
		cfg.MCP.TimeoutSec = 60
	}
	if cfg.MCP.InitTimeoutSec <= 0 {
		cfg.MCP.InitTimeoutSec = 10
	}
	if cfg.Session.OutputDir == "" {
		cfg.Session.OutputDir = "output"
	}
	// 转为绝对路径，避免子进程（EPlus-MCP）用自己的 workDir 解析相对路径
	if !filepath.IsAbs(cfg.Session.OutputDir) {
		if abs, err := filepath.Abs(cfg.Session.OutputDir); err == nil {
			cfg.Session.OutputDir = abs
		}
	}
	if cfg.Log.File != "" && !filepath.IsAbs(cfg.Log.File) {
		if abs, err := filepath.Abs(cfg.Log.File); err == nil {
			cfg.Log.File = abs
		}
	}
	if cfg.Session.SimulationScript == "" {
		cfg.Session.SimulationScript = `D:\TryAgent\EPlus-MCP\main.py`
	}
	if cfg.Log.File == "" {
		cfg.Log.File = "logs/agent.log"
	}
	if cfg.Log.ReactLogDir == "" {
		cfg.Log.ReactLogDir = "output/logs/react"
	}

	// 模块默认值
	if cfg.Modules.Intent.MaxReactIter <= 0 {
		cfg.Modules.Intent.MaxReactIter = 15
	}
	if cfg.Modules.YAMLGenerate.MaxReactIter <= 0 {
		cfg.Modules.YAMLGenerate.MaxReactIter = 15
	}
	if cfg.Modules.YAMLGenerate.MaxHealAttempts <= 0 {
		cfg.Modules.YAMLGenerate.MaxHealAttempts = 5
	}
	if cfg.Modules.IDFConvert.MaxHealAttempts <= 0 {
		cfg.Modules.IDFConvert.MaxHealAttempts = 8
	}
	if cfg.Modules.Simulation.MaxFixAttempts <= 0 {
		cfg.Modules.Simulation.MaxFixAttempts = 10
	}
	if cfg.Modules.ParamAnalysis.MaxReactIter <= 0 {
		cfg.Modules.ParamAnalysis.MaxReactIter = 12
	}
	if cfg.Modules.ParamAnalysis.MaxWorkers <= 0 {
		cfg.Modules.ParamAnalysis.MaxWorkers = 1
	}
	if cfg.Modules.ParamAnalysis.MaxFixAttempts <= 0 {
		cfg.Modules.ParamAnalysis.MaxFixAttempts = 3
	}

	// RAG 默认值
	if cfg.RAG.EmbeddingModel == "" {
		cfg.RAG.EmbeddingModel = "text-embedding-v4"
	}
	if cfg.RAG.EmbeddingDim <= 0 {
		cfg.RAG.EmbeddingDim = 1024
	}
	if cfg.RAG.TopK <= 0 {
		cfg.RAG.TopK = 5
	}
	if cfg.RAG.EmbeddingTimeoutSec <= 0 {
		cfg.RAG.EmbeddingTimeoutSec = 30
	}
	if cfg.RAG.IndexPath == "" {
		cfg.RAG.IndexPath = "data/index/ior_part.idx"
	}

	return cfg, nil
}

// Validate 校验必填项
func (c *Config) Validate() error {
	if c.LLM.APIKey == "" {
		return fmt.Errorf("LLM API Key 未设置，请在配置文件中设置 llm.api_key 或设置环境变量 LLM_API_KEY")
	}
	if c.LLM.BaseURL == "" {
		return fmt.Errorf("LLM base_url 未设置")
	}
	if c.LLM.Model == "" {
		return fmt.Errorf("LLM model 未设置")
	}
	return nil
}
