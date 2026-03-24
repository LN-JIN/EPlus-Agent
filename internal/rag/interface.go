// RAG（检索增强生成）接口模块
// 定义 Retriever 接口，允许在不同阶段注入外部知识（EnergyPlus 文档、示例配置等）。
// v0.1 阶段使用 NoopRetriever（空实现），系统可正常运行；
// v0.2 阶段可替换为基于向量数据库的真实实现，无需修改调用方代码。

package rag

import "context"

// Document 检索到的文档片段
type Document struct {
	ID      string  // 文档 ID
	Content string  // 文档内容
	Score   float64 // 相关性分数（0-1）
	Source  string  // 来源（文件名、URL 等）
}

// Retriever 知识检索接口
type Retriever interface {
	// Query 根据查询词检索相关文档
	// query: 查询文本
	// topK: 最多返回的文档数量
	Query(ctx context.Context, query string, topK int) ([]Document, error)
}

// NoopRetriever v0.1 阶段的空实现，不做任何检索
// 满足 Retriever 接口，系统可正常运行
type NoopRetriever struct{}

// Query 空实现，直接返回空列表
func (n *NoopRetriever) Query(_ context.Context, _ string, _ int) ([]Document, error) {
	return nil, nil
}

// FormatDocs 将检索到的文档列表格式化为文本块，注入 LLM prompt
// 若文档为空，返回空字符串（不影响 prompt）
// 最多展示 3 条，如需更多请使用 FormatDocsN
func FormatDocs(docs []Document) string {
	return FormatDocsN(docs, 3)
}

// FormatDocsN 将检索到的文档列表格式化为文本块，最多展示 n 条
func FormatDocsN(docs []Document, n int) string {
	if len(docs) == 0 {
		return ""
	}
	result := "\n\n### 参考资料\n"
	for i, doc := range docs {
		result += "\n---\n"
		if doc.Source != "" {
			result += "来源: " + doc.Source + "\n"
		}
		result += "内容:\n" + doc.Content
		if i >= n-1 {
			break
		}
	}
	return result
}
