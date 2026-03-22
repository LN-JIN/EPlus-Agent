// 配置加载模块
// 负责从 configs/config.yaml 读取应用配置，并支持通过环境变量覆盖敏感字段（如 API Key）。
// 所有模块共用同一个 Config 实例，由 main.go 初始化后注入。

package config

import (
	"fmt"
	"os"

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

// MCPConfig MCP Server 连接配置
type MCPConfig struct {
	BaseURL        string `yaml:"base_url"`
	TimeoutSec     int    `yaml:"timeout_sec"`
	InitTimeoutSec int    `yaml:"init_timeout_sec"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level   string `yaml:"level"`
	File    string `yaml:"file"`
	Console bool   `yaml:"console"`
}

// SessionConfig 会话运行参数
type SessionConfig struct {
	OutputDir    string `yaml:"output_dir"`
	MaxReactIter int    `yaml:"max_react_iter"`
	MaxHealIter  int    `yaml:"max_heal_iter"`
}

// Config 全局配置根节点
type Config struct {
	LLM     LLMConfig     `yaml:"llm"`
	MCP     MCPConfig     `yaml:"mcp"`
	Log     LogConfig     `yaml:"log"`
	Session SessionConfig `yaml:"session"`
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
	if cfg.Session.MaxReactIter <= 0 {
		cfg.Session.MaxReactIter = 15
	}
	if cfg.Session.MaxHealIter <= 0 {
		cfg.Session.MaxHealIter = 5
	}
	if cfg.Session.OutputDir == "" {
		cfg.Session.OutputDir = "output"
	}
	if cfg.Log.File == "" {
		cfg.Log.File = "logs/agent.log"
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
