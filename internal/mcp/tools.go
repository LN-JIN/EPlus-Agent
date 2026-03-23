// MCP 工具封装模块
// 对 EnergyPlus-Agent-try2 MCP Server 的各工具提供具名 Go 方法封装，
// 隐藏 JSON-RPC 的底层细节，提供类型安全、语义清晰的调用接口。
// 当前封装 workflow 工具集（加载/验证/摘要/导出）。

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// toolResult Python MCP Server 返回的业务层结果结构
type toolResult struct {
	Result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	} `json:"result"`
}

// checkSuccess 解析响应 JSON，若 success=false 则返回错误
func checkSuccess(raw string) error {
	var r toolResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		// 无法解析时不视为错误，保持兼容
		return nil
	}
	if !r.Result.Success {
		return fmt.Errorf("%s", r.Result.Message)
	}
	return nil
}

// ============================================================
// Workflow 工具集封装
// 对应 Python MCP Server 中的 workflow 工具
// ============================================================

// LoadYAML 加载 YAML 配置文件到 MCP Server 的 ConfigState
func (c *Client) LoadYAML(ctx context.Context, yamlPath string) error {
	slog.Info("[MCP] 加载 YAML 配置", "path", yamlPath)

	result, err := c.CallTool(ctx, "load_yaml", map[string]any{
		"input_path": yamlPath,
	})
	if err != nil {
		return fmt.Errorf("load_yaml 失败: %w", err)
	}
	if err := checkSuccess(result); err != nil {
		return fmt.Errorf("load_yaml 失败: %w", err)
	}

	slog.Info("[MCP] YAML 加载成功", "result", result)
	return nil
}

// ValidateConfig 验证当前 MCP Server 中已加载的配置
func (c *Client) ValidateConfig(ctx context.Context) (string, error) {
	slog.Info("[MCP] 验证配置")

	result, err := c.CallTool(ctx, "validate_config", nil)
	if err != nil {
		return "", fmt.Errorf("validate_config 失败: %w", err)
	}
	if err := checkSuccess(result); err != nil {
		return "", fmt.Errorf("validate_config 失败: %w", err)
	}

	slog.Info("[MCP] 配置验证完成", "result_len", len(result))
	return result, nil
}

// GetSummary 获取当前已加载配置的摘要信息
func (c *Client) GetSummary(ctx context.Context) (string, error) {
	slog.Info("[MCP] 获取配置摘要")

	result, err := c.CallTool(ctx, "get_summary", nil)
	if err != nil {
		return "", fmt.Errorf("get_summary 失败: %w", err)
	}

	slog.Info("[MCP] 获取摘要成功", "result_len", len(result))
	return result, nil
}

// ExportYAML 将当前 ConfigState 导出为 YAML 文件
func (c *Client) ExportYAML(ctx context.Context, outputPath string) error {
	slog.Info("[MCP] 导出 YAML", "output_path", outputPath)

	result, err := c.CallTool(ctx, "export_yaml", map[string]any{
		"output_path": outputPath,
	})
	if err != nil {
		return fmt.Errorf("export_yaml 失败: %w", err)
	}
	if err := checkSuccess(result); err != nil {
		return fmt.Errorf("export_yaml 失败: %w", err)
	}

	slog.Info("[MCP] YAML 导出成功", "result", result)
	return nil
}

// ClearAll 清空 MCP Server 的 ConfigState
func (c *Client) ClearAll(ctx context.Context) error {
	slog.Info("[MCP] 清空配置状态")

	result, err := c.CallTool(ctx, "clear_all", nil)
	if err != nil {
		slog.Warn("[MCP] 清空状态失败（可忽略）", "err", err)
		return nil
	}

	slog.Info("[MCP] 状态已清空", "result", result)
	return nil
}

// RunSimulation 执行 EnergyPlus 仿真，生成 IDF 并运行
//
// Deprecated: 因未知原因通过 MCP 调用仿真会产生异常，请改用 eplusrun.Runner.RunSimulation()
// 直接通过命令行调用 EPlus-MCP main.py。若将来 MCP 问题修复，可恢复使用此方法。
//
// epwPath: EPW 气象文件路径；outputDir: 输出目录
func (c *Client) RunSimulation(ctx context.Context, epwPath, outputDir string) (string, error) {
	slog.Info("[MCP] 运行仿真", "epw", epwPath, "output_dir", outputDir)

	result, err := c.CallTool(ctx, "run_simulation", map[string]any{
		"epw_path":   epwPath,
		"output_dir": outputDir,
	})
	if err != nil {
		return "", fmt.Errorf("run_simulation 失败: %w", err)
	}
	if err := checkSuccess(result); err != nil {
		return "", fmt.Errorf("run_simulation 失败: %w", err)
	}

	slog.Info("[MCP] 仿真完成", "result", result)
	return result, nil
}

// Ping 测试 MCP Server 连通性（通过 Initialize）
func (c *Client) Ping(ctx context.Context) error {
	err := c.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("MCP Server 连通性测试失败 [%s]: %w", c.baseURL, err)
	}
	return nil
}
