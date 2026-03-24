// hyde.go — HyDE（Hypothetical Document Embeddings）
// 让 LLM 先生成一段"假设性参考文档"，再对该文档做 embedding，
// 与直接 embedding 用户问题并行，两路结果用 RRF 融合，提升召回率。
package rag

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"energyplus-agent/internal/llm"
)

const hydeSystemPrompt = `你是一位 EnergyPlus 输入输出参考手册（Input-Output Reference）专家。
请根据用户问题，生成一段简短的参考手册风格段落（100-200字），用于帮助检索相关文档。

要求：
- 如涉及 IDD 对象，使用准确的对象名（如 BuildingSurface:Detailed）
- 如涉及字段，描述字段名、类型、单位（如: Field: Construction Name, Type: object-list）
- 如涉及输出变量，使用 "Output Variable: [变量名] [单位]" 格式
- 仅描述你确定的内容，不确定时用省略号占位，不要猜测具体数值
- 使用英文（与文档语言一致）`

// HyDEResult HyDE + 直接 embedding 的双路结果
type HyDEResult struct {
	HyDEVec  []float32 // HyDE 假设文档的 embedding
	DirectVec []float32 // 原始查询的 embedding
	HyDEText  string    // 生成的假设文档（供调试）
}

// HyDEEmbedder 实现 HyDE 双路检索
type HyDEEmbedder struct {
	llmClient *llm.Client
	embedder  *EmbeddingAdapter
}

// NewHyDEEmbedder 创建 HyDEEmbedder
func NewHyDEEmbedder(llmClient *llm.Client, embedder *EmbeddingAdapter) *HyDEEmbedder {
	return &HyDEEmbedder{
		llmClient: llmClient,
		embedder:  embedder,
	}
}

// Embed 并行执行 HyDE 和直接 embedding，返回双路结果
// 若 HyDE 失败（LLM 错误），降级为只用直接 embedding（HyDEVec = DirectVec）
func (h *HyDEEmbedder) Embed(ctx context.Context, query string) (*HyDEResult, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	var directVec []float32
	var hydeVec []float32
	var hydeText string
	var directErr, hydeErr error

	// 并行：直接 embed 查询
	wg.Add(1)
	go func() {
		defer wg.Done()
		v, err := h.embedder.Embed(ctx, query)
		mu.Lock()
		directVec, directErr = v, err
		mu.Unlock()
	}()

	// 并行：HyDE - 生成假设文档再 embed
	wg.Add(1)
	go func() {
		defer wg.Done()
		text, err := h.generateHyDE(ctx, query)
		if err != nil {
			mu.Lock()
			hydeErr = err
			mu.Unlock()
			return
		}
		v, err := h.embedder.Embed(ctx, text)
		mu.Lock()
		hydeVec, hydeText, hydeErr = v, text, err
		mu.Unlock()
	}()

	wg.Wait()

	if directErr != nil {
		return nil, fmt.Errorf("直接 embedding 失败: %w", directErr)
	}

	// HyDE 失败时降级
	if hydeErr != nil {
		slog.Warn("[HyDE] 生成失败，降级为直接 embedding", "err", hydeErr)
		hydeVec = directVec
		hydeText = ""
	}

	return &HyDEResult{
		HyDEVec:   hydeVec,
		DirectVec: directVec,
		HyDEText:  hydeText,
	}, nil
}

// generateHyDE 调用 LLM 生成假设文档文本
func (h *HyDEEmbedder) generateHyDE(ctx context.Context, query string) (string, error) {
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: hydeSystemPrompt},
		{Role: llm.RoleUser, Content: query},
	}
	resp, err := h.llmClient.ChatNoThink(ctx, messages, nil)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Content == "" {
		return "", fmt.Errorf("LLM 返回空内容")
	}
	slog.Debug("[HyDE] 生成假设文档", "query", query, "hyde_text", resp.Content[:min(len(resp.Content), 100)])
	return resp.Content, nil
}
