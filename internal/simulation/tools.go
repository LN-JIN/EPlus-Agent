// Phase 4 仿真模块工具注册
// 为仿真 ReAct Agent 注册所有可调用工具：
// - run_simulation: 执行 EnergyPlus 仿真
// - check_simulation_result: 检查仿真输出目录
// - read_idf_object: 读取 IDF 中指定对象的字段
// - edit_idf_object: 修改 IDF 中指定对象的字段（通过 EPlus-MCP edit-idf 命令）
// - save_idf_snapshot: 复制 IDF 为新版本快照

package simulation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"energyplus-agent/internal/eplusrun"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/session"
	"energyplus-agent/internal/tools"
)

// registerSimTools 将仿真修复 ReAct 所需的所有工具注册到 registry
func registerSimTools(
	registry *tools.Registry,
	runner *eplusrun.Runner,
	state *session.SessionState,
	epwPath string,
	snapshotDir string,
) {
	// ── run_simulation ───────────────────────────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "run_simulation",
				Description: "执行 EnergyPlus 仿真。返回仿真输出目录路径（包含 eplusout.end/err/eso/csv 等结果文件）。仿真失败时返回错误但仍给出输出目录（用于读取 err 文件）。",
				Parameters: tools.ObjectSchema(
					"仿真参数",
					map[string]any{
						"idf_path": tools.StringParam("IDF 文件路径"),
					},
					[]string{"idf_path"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			idfPath, err := tools.GetString(args, "idf_path")
			if err != nil {
				return "", err
			}
			// outputDir 基于 IDF stem + session 目录
			stem := strings.TrimSuffix(filepath.Base(idfPath), filepath.Ext(idfPath))
			outputDir := filepath.Join(filepath.Dir(snapshotDir), "results", stem)

			ctx := context.Background()
			simOutDir, runErr := runner.RunSimulation(ctx, idfPath, epwPath, outputDir)
			if runErr != nil {
				// 即使失败，返回 outputDir 方便 LLM 读取 err 文件
				return fmt.Sprintf(`{"sim_out_dir":%q,"success":false,"error":%q}`,
					simOutDir, runErr.Error()), nil
			}
			// 更新 state（最新仿真目录）
			state.SimOutDir = simOutDir
			return fmt.Sprintf(`{"sim_out_dir":%q,"success":true}`, simOutDir), nil
		},
	)

	// ── check_simulation_result ──────────────────────────────────────────────
	registry.Register(
		tools.SimCheckToolDef(),
		tools.SimCheckHandler(),
	)

	// ── read_idf_object ──────────────────────────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "read_idf_object",
				Description: "读取 IDF 文件中指定对象的所有字段和当前值（JSON 格式）。用于了解当前参数后再决定如何修改。",
				Parameters: tools.ObjectSchema(
					"读取 IDF 对象参数",
					map[string]any{
						"idf_path":    tools.StringParam("IDF 文件路径"),
						"object_type": tools.StringParam("EnergyPlus 对象类型，如 ZoneHVAC:IdealLoadsAirSystem"),
						"name":        tools.StringParam("对象 Name 字段的值（空字符串表示读取该类型所有对象）"),
					},
					[]string{"idf_path", "object_type"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			idfPath, err := tools.GetString(args, "idf_path")
			if err != nil {
				return "", err
			}
			objectType, err := tools.GetString(args, "object_type")
			if err != nil {
				return "", err
			}
			name := tools.GetStringOr(args, "name", "")

			// 通过 python -c 读取 IDF 对象（使用 eppy）
			ctx := context.Background()
			result, readErr := readIDFObjectViaCLI(ctx, runner, idfPath, objectType, name)
			if readErr != nil {
				return fmt.Sprintf("ERROR: %v", readErr), nil
			}
			return result, nil
		},
	)

	// ── edit_idf_object ──────────────────────────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "edit_idf_object",
				Description: "修改 IDF 文件中指定对象的字段值（通过 eppy）。修改后请用 check_simulation_result 或再次仿真验证。",
				Parameters: tools.ObjectSchema(
					"修改 IDF 对象参数",
					map[string]any{
						"idf_path":    tools.StringParam("IDF 文件路径"),
						"object_type": tools.StringParam("EnergyPlus 对象类型，如 ZoneHVAC:IdealLoadsAirSystem"),
						"name":        tools.StringParam("对象 Name 字段的值"),
						"field":       tools.StringParam("要修改的 eppy 属性名（如 Maximum_Heating_Supply_Air_Temperature）"),
						"value":       tools.StringParam("新字段值（字符串，eppy 自动转型）"),
					},
					[]string{"idf_path", "object_type", "name", "field", "value"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			idfPath, err := tools.GetString(args, "idf_path")
			if err != nil {
				return "", err
			}
			objectType, err := tools.GetString(args, "object_type")
			if err != nil {
				return "", err
			}
			name, err := tools.GetString(args, "name")
			if err != nil {
				return "", err
			}
			field, err := tools.GetString(args, "field")
			if err != nil {
				return "", err
			}
			value, err := tools.GetString(args, "value")
			if err != nil {
				return "", err
			}

			ctx := context.Background()
			editErr := runner.EditIDF(ctx, idfPath, objectType, name, field, value)
			if editErr != nil {
				return fmt.Sprintf("EDIT_ERROR: %v", editErr), nil
			}
			return fmt.Sprintf("EDIT_OK: %s/%s.%s = %s", objectType, name, field, value), nil
		},
	)

	// ── save_idf_snapshot ────────────────────────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "save_idf_snapshot",
				Description: "将当前 IDF 文件复制为新版本快照（便于回溯）。返回快照文件路径。",
				Parameters: tools.ObjectSchema(
					"保存 IDF 快照参数",
					map[string]any{
						"idf_path": tools.StringParam("当前 IDF 文件路径"),
						"label":    tools.StringParam("快照标签，如 'before_fix_1'、'after_setpoint_fix'"),
					},
					[]string{"idf_path", "label"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			idfPath, err := tools.GetString(args, "idf_path")
			if err != nil {
				return "", err
			}
			label, err := tools.GetString(args, "label")
			if err != nil {
				return "", err
			}

			snapPath := filepath.Join(snapshotDir,
				fmt.Sprintf("%s_%s.idf", label,
					strings.TrimSuffix(filepath.Base(idfPath), filepath.Ext(idfPath))))
			if copyErr := copyFile(idfPath, snapPath); copyErr != nil {
				return fmt.Sprintf("SNAPSHOT_ERROR: %v", copyErr), nil
			}
			state.AddIDFSnapshot(label, snapPath, "")
			return fmt.Sprintf("SNAPSHOT_OK: %s", snapPath), nil
		},
	)
}

// readIDFObjectViaCLI 通过 EPlus-MCP CLI 读取 IDF 对象字段
// 使用 python -c 调用 eppy 读取对象，返回 JSON 格式
func readIDFObjectViaCLI(ctx context.Context, runner *eplusrun.Runner, idfPath, objectType, name string) (string, error) {
	// 通过 edit-idf 的 validate 逻辑，我们无直接读取命令；
	// 用 validate-idf 获取基本信息，更复杂的读取通过 python helper
	// 当前版本：通过 validate-idf 确认文件可读，然后返回对象类型提示
	// TODO: 将来可在 EPlus-MCP 中增加 read-idf 命令以支持精确字段读取
	valid, msg, err := runner.ValidateIDF(ctx, idfPath)
	if err != nil {
		return "", fmt.Errorf("IDF 读取失败: %w", err)
	}
	if !valid {
		return "", fmt.Errorf("IDF 无效: %s", msg)
	}

	result := map[string]any{
		"note":        "当前实现通过 validate-idf 确认 IDF 可读；精确字段读取需 EPlus-MCP 增加 read-idf 命令",
		"idf_path":    idfPath,
		"object_type": objectType,
		"name":        name,
		"idf_valid":   msg,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
