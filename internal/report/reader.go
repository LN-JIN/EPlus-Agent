// 仿真结果读取工具
// 从 EnergyPlus 输出目录读取 eplusout.csv 或 eplusout.eso，
// 提取关键能耗/温度指标供报告生成使用。

package report

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SimData 从仿真输出中提取的关键数据
type SimData struct {
	Source  string              // "csv" 或 "eso"
	Headers []string            // 数据列标题
	Rows    [][]string          // 原始数据行（最多 8760 行）
	Summary map[string]float64  // 汇总指标（列名 → 年度总和）
}

// ReadSimData 从仿真输出目录读取能耗数据
// 优先读取 eplusout.csv，若不存在则回退到 eplusout.eso
func ReadSimData(simOutDir string) (*SimData, error) {
	csvPath := filepath.Join(simOutDir, "eplusout.csv")
	if _, err := os.Stat(csvPath); err == nil {
		return readCSV(csvPath)
	}

	esoPath := filepath.Join(simOutDir, "eplusout.eso")
	if _, err := os.Stat(esoPath); err == nil {
		return readESO(esoPath)
	}

	return nil, fmt.Errorf("仿真输出目录中未找到 eplusout.csv 或 eplusout.eso: %s", simOutDir)
}

// readCSV 读取 eplusout.csv，提取所有数值列的年度总和
func readCSV(path string) (*SimData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 CSV 文件失败: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("读取 CSV 表头失败: %w", err)
	}

	// 仅保留前 8760 行（一年逐时数据）
	const maxRows = 8760
	rows := make([][]string, 0, maxRows)
	for i := 0; i < maxRows; i++ {
		row, err := r.Read()
		if err != nil {
			break
		}
		rows = append(rows, row)
	}

	summary := computeSummary(headers, rows)

	return &SimData{
		Source:  "csv",
		Headers: headers,
		Rows:    rows,
		Summary: summary,
	}, nil
}

// readESO 从 eplusout.eso 中提取关键变量的年度总和（简化解析）
func readESO(path string) (*SimData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 ESO 文件失败: %w", err)
	}
	defer f.Close()

	// ESO 格式：数据字典在文件头部，数据行以数字代码开头
	// 简化解析：提取所有以 "2," 开头的小时数据行（report code 2 = hourly）
	summary := make(map[string]float64)
	varNames := make(map[string]string) // code → variable name

	scanner := bufio.NewScanner(f)
	inDictionary := true
	for scanner.Scan() {
		line := scanner.Text()

		// 数据字典结束标记
		if strings.HasPrefix(line, "End of Data Dictionary") {
			inDictionary = false
			continue
		}

		if inDictionary {
			// 解析变量定义行: "code,freq,variable name [unit]"
			parts := strings.SplitN(line, ",", 3)
			if len(parts) == 3 {
				code := strings.TrimSpace(parts[0])
				varName := strings.TrimSpace(parts[2])
				varNames[code] = varName
			}
			continue
		}

		// 数据行: "code,value"
		parts := strings.SplitN(line, ",", 2)
		if len(parts) == 2 {
			code := strings.TrimSpace(parts[0])
			valStr := strings.TrimSpace(parts[1])
			if name, ok := varNames[code]; ok {
				if val, err := strconv.ParseFloat(valStr, 64); err == nil {
					summary[name] += val
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 ESO 文件失败: %w", err)
	}

	return &SimData{
		Source:  "eso",
		Summary: summary,
	}, nil
}

// colSummary 单列的汇总中间值
type colSummary struct {
	sum   float64
	count int
}

// isTemperatureColumn 判断列名是否为温度变量（应取均值而非求和）
func isTemperatureColumn(header string) bool {
	h := strings.ToLower(header)
	return strings.Contains(h, "temperature") || strings.Contains(h, "[c]")
}

// computeSummary 计算各数值列的年度汇总：
// - 温度列（含 "Temperature" 或 "[C]"）→ 年度均值
// - 能量列（其余数值列）→ 年度总和
func computeSummary(headers []string, rows [][]string) map[string]float64 {
	interim := make(map[string]*colSummary, len(headers))
	for _, h := range headers {
		interim[h] = &colSummary{}
	}

	for _, row := range rows {
		for i, cell := range row {
			if i >= len(headers) {
				break
			}
			if val, err := strconv.ParseFloat(strings.TrimSpace(cell), 64); err == nil {
				cs := interim[headers[i]]
				cs.sum += val
				cs.count++
			}
		}
	}

	summary := make(map[string]float64, len(headers))
	for _, h := range headers {
		cs := interim[h]
		if cs.count == 0 {
			continue
		}
		if isTemperatureColumn(h) {
			summary[h] = cs.sum / float64(cs.count) // 年度均值
		} else {
			summary[h] = cs.sum // 年度总和
		}
	}
	return summary
}

// FormatSummaryText 将 SimData.Summary 格式化为易读文本（供 LLM 分析）
func FormatSummaryText(data *SimData) string {
	if data == nil || len(data.Summary) == 0 {
		return "(no simulation data available)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Data source: %s\n\n", data.Source))

	// 按关键词分组输出
	groups := []struct {
		label    string
		keywords []string
	}{
		{"HVAC Energy", []string{"heating energy", "cooling energy", "ideal loads"}},
		{"Zone Temperature", []string{"zone mean air temperature", "zone operative temperature"}},
		{"Lighting & Equipment", []string{"lighting", "equipment", "electric"}},
		{"Other", nil},
	}

	printed := make(map[string]bool)
	for _, g := range groups {
		var entries []string
		for k, v := range data.Summary {
			if printed[k] {
				continue
			}
			kLower := strings.ToLower(k)
			matched := g.keywords == nil
			for _, kw := range g.keywords {
				if strings.Contains(kLower, kw) {
					matched = true
					break
				}
			}
			if matched {
				entries = append(entries, fmt.Sprintf("  %s: %.2f", k, v))
				printed[k] = true
			}
		}
		if len(entries) > 0 {
			sb.WriteString(fmt.Sprintf("### %s\n", g.label))
			for _, e := range entries {
				sb.WriteString(e + "\n")
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
