// embedder.go — EmbeddingAdapter：调用 DashScope 兼容的 /v1/embeddings 接口
// 复用 llm.Client 相同的 HTTP 模式（BaseURL、APIKey、Bearer 认证）
package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingAdapter 调用 OpenAI 兼容的 /v1/embeddings 接口生成文本向量
type EmbeddingAdapter struct {
	baseURL    string
	apiKey     string
	model      string
	dim        int
	httpClient *http.Client
}

// NewEmbeddingAdapter 创建 EmbeddingAdapter
//
//	baseURL:    API 基础 URL（如 "https://dashscope.aliyuncs.com/compatible-mode/v1"）
//	apiKey:     API 密钥
//	model:      Embedding 模型名（如 "text-embedding-v4"）
//	dim:        向量维度（如 1024）
//	timeoutSec: HTTP 超时秒数
func NewEmbeddingAdapter(baseURL, apiKey, model string, dim, timeoutSec int) *EmbeddingAdapter {
	return &EmbeddingAdapter{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		dim:     dim,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
	}
}

// embeddingRequest OpenAI 兼容的 /v1/embeddings 请求体
type embeddingRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	Dimensions int    `json:"dimensions,omitempty"`
}

// embeddingResponse OpenAI 兼容的 /v1/embeddings 响应
type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Embed 对单条文本生成 float32 embedding 向量
func (e *EmbeddingAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := embeddingRequest{
		Model:      e.model,
		Input:      text,
		Dimensions: e.dim,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化 embedding 请求失败: %w", err)
	}

	url := e.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建 embedding HTTP 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 embedding 响应失败: %w", err)
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBytes, &embResp); err != nil {
		return nil, fmt.Errorf("解析 embedding 响应失败: %w (body: %s)", err, respBytes[:min(len(respBytes), 200)])
	}

	if embResp.Error != nil {
		return nil, fmt.Errorf("embedding API 错误 [%s]: %s", embResp.Error.Code, embResp.Error.Message)
	}

	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("embedding API 返回空 data")
	}

	vec64 := embResp.Data[0].Embedding
	vec32 := make([]float32, len(vec64))
	for i, v := range vec64 {
		vec32[i] = float32(v)
	}
	return vec32, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
