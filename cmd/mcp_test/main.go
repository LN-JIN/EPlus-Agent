// MCP 工具手动测试程序
// 依次调用 MCP server 的各个 workflow 工具，打印每步的输入输出。
// 用法：go run cmd/mcp_test/main.go

package main

import (
	"context"
	"fmt"
	"time"

	"energyplus-agent/internal/mcp"
)

const (
	mcpBaseURL = "http://127.0.0.1:8000"
	yamlPath   = `D:\TryAgent\energyplus-agent\output\building_20260320_180357.yaml`
	// yamlPath   = `D:\TryAgent\EnergyPlus-Agent-try2\data\schemas\building_schema.yaml`
	exportPath = `D:\TryAgent\energyplus-agent\output\mcp_test_export.yaml`
	epwPath    = `D:\TryAgent\EnergyPlus-Agent-try2\data\weather\Shenzhen.epw`
	simOutDir  = `D:\TryAgent\energyplus-agent\output\simulation`
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	client := mcp.NewClient(mcpBaseURL, 300)

	// ── 1. Initialize ────────────────────────────────────────
	section("1. Initialize（握手）")
	fmt.Printf("  → POST %s/mcp  method=initialize\n", mcpBaseURL)
	if err := client.Initialize(ctx); err != nil {
		fail("Initialize", err)
		return
	}
	ok("Initialize 成功")

	// ── 2. ClearAll ──────────────────────────────────────────
	section("2. ClearAll（清空旧状态）")
	fmt.Println("  → 工具: clear_all  参数: 无")
	if err := client.ClearAll(ctx); err != nil {
		fail("ClearAll", err)
	} else {
		ok("ClearAll 成功")
	}

	// ── 3. LoadYAML ──────────────────────────────────────────
	section("3. LoadYAML（加载 YAML）")
	fmt.Printf("  → 工具: load_yaml  参数: input_path=%s\n", yamlPath)
	if err := client.LoadYAML(ctx, yamlPath); err != nil {
		fail("LoadYAML", err)
		return
	}
	ok("LoadYAML 成功")

	// ── 4. GetSummary ────────────────────────────────────────
	section("4. GetSummary（配置摘要）")
	fmt.Println("  → 工具: get_summary  参数: 无")
	summary, err := client.GetSummary(ctx)
	if err != nil {
		fail("GetSummary", err)
	} else {
		ok("GetSummary 成功")
		fmt.Println("  ← 返回内容:")
		indent(summary)
	}

	// ── 5. ValidateConfig ────────────────────────────────────
	section("5. ValidateConfig（验证配置）")
	fmt.Println("  → 工具: validate_config  参数: 无")
	result, err := client.ValidateConfig(ctx)
	if err != nil {
		fail("ValidateConfig", err)
		return
	}
	ok("ValidateConfig 成功")
	fmt.Println("  ← 返回内容:")
	indent(result)

	// ── 6. RunSimulation ─────────────────────────────────────
	section("6. RunSimulation（生成 IDF 并运行仿真）")
	fmt.Printf("  → 工具: run_simulation\n")
	fmt.Printf("       epw_path=%s\n", epwPath)
	fmt.Printf("       output_dir=%s\n", simOutDir)
	simResult, err := client.RunSimulation(ctx, epwPath, simOutDir)
	if err != nil {
		fail("RunSimulation", err)
	} else {
		ok("RunSimulation 成功")
		fmt.Println("  ← 返回内容:")
		indent(simResult)
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════")
	fmt.Println("  全部测试完成")
	fmt.Println("══════════════════════════════════════════")
}

func section(title string) {
	fmt.Println()
	fmt.Println("──────────────────────────────────────────")
	fmt.Printf("  %s\n", title)
	fmt.Println("──────────────────────────────────────────")
}

func ok(msg string) {
	fmt.Printf("  ✓ %s\n", msg)
}

func fail(name string, err error) {
	fmt.Printf("  ✗ %s 失败: %v\n", name, err)
}

func indent(s string) {
	if len(s) > 800 {
		fmt.Printf("    %s\n    ... (共 %d 字节，已截断)\n", s[:800], len(s))
		return
	}
	fmt.Printf("    %s\n", s)
}
