// qa.go — QAEngine：编排完整的 RAG 问答流程
// 流程：Embed（HyDE+直接）→ 向量检索+BM25检索 → RRF融合 → LLM 生成答案（流式）
package rag

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/rag/vectorstore"
)

const qaSystemPrompt = `你是 EnergyPlus 输入输出参考手册（Input-Output Reference）专家助手。
以下是从文档中检索到的相关内容，请基于这些内容回答用户问题。

回答要求：
1. 明确引用 IDD 对象名（如 BuildingSurface:Detailed）和字段名
2. 如有字段类型、单位、默认值、取值范围，请完整列出
3. 如有输出变量，列出变量名和单位
4. 如果检索内容不足以完整回答，请明确说明哪些信息未覆盖
5. 用中文回答（除专有名词外）

%s`

// QAEngine 问答引擎
type QAEngine struct {
	store     *vectorstore.VectorStore
	embedder  *EmbeddingAdapter
	hyde      *HyDEEmbedder
	llmClient *llm.Client
	topK      int
	useHyDE   bool
}

// NewQAEngine 创建问答引擎
func NewQAEngine(
	store *vectorstore.VectorStore,
	embedder *EmbeddingAdapter,
	llmClient *llm.Client,
	topK int,
	useHyDE bool,
) *QAEngine {
	var hydeEmb *HyDEEmbedder
	if useHyDE {
		hydeEmb = NewHyDEEmbedder(llmClient, embedder)
	}
	return &QAEngine{
		store:     store,
		embedder:  embedder,
		hyde:      hydeEmb,
		llmClient: llmClient,
		topK:      topK,
		useHyDE:   useHyDE,
	}
}

// RetrievedContext 检索上下文（供答案生成和调试）
type RetrievedContext struct {
	Parents  []vectorstore.ParentMeta
	HyDEText string // HyDE 生成的假设文档（调试用）
}

// Retrieve 执行检索，返回 top-K parent chunks
func (q *QAEngine) Retrieve(ctx context.Context, query string) (*RetrievedContext, error) {
	retrievalTopK := q.topK * 4 // 过取再 RRF 融合

	var directVec []float32
	var hydeText string

	if q.useHyDE {
		result, err := q.hyde.Embed(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("embedding 失败: %w", err)
		}
		directVec = result.DirectVec
		hydeText = result.HyDEText

		// 两路向量检索（HyDE + 直接），结果合并后 RRF
		// HyDE 降级时只做一次直接检索，避免冗余搜索
		var mergedVectorIdxs []int
		if result.HyDEFailed {
			mergedVectorIdxs = q.store.VectorSearch(result.DirectVec, retrievalTopK)
		} else {
			hydeChildIdxs := q.store.VectorSearch(result.HyDEVec, retrievalTopK)
			directChildIdxs := q.store.VectorSearch(result.DirectVec, retrievalTopK)
			mergedVectorIdxs = dedup(append(hydeChildIdxs, directChildIdxs...))
		}
		bm25ChildIdxs := q.store.BM25Search(query, retrievalTopK)

		parentIdxs := vectorstore.RRFMerge(
			mergedVectorIdxs,
			bm25ChildIdxs,
			q.store.ChildToParentIdx(),
			q.topK,
		)

		parents := q.resolveParents(parentIdxs)
		slog.Debug("[QA] 检索完成（HyDE）",
			"query", query,
			"vector_hits", len(mergedVectorIdxs),
			"bm25_hits", len(bm25ChildIdxs),
			"final_parents", len(parents),
		)
		return &RetrievedContext{Parents: parents, HyDEText: hydeText}, nil
	}

	// 不使用 HyDE：只用直接 embedding
	var err error
	directVec, err = q.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding 失败: %w", err)
	}

	vectorChildIdxs := q.store.VectorSearch(directVec, retrievalTopK)
	bm25ChildIdxs := q.store.BM25Search(query, retrievalTopK)

	parentIdxs := vectorstore.RRFMerge(
		vectorChildIdxs,
		bm25ChildIdxs,
		q.store.ChildToParentIdx(),
		q.topK,
	)

	parents := q.resolveParents(parentIdxs)
	slog.Debug("[QA] 检索完成（直接）",
		"query", query,
		"vector_hits", len(vectorChildIdxs),
		"bm25_hits", len(bm25ChildIdxs),
		"final_parents", len(parents),
	)
	return &RetrievedContext{Parents: parents}, nil
}

// AnswerResult 问答结果（含检索上下文和 LLM 生成详情）
type AnswerResult struct {
	*RetrievedContext
	ReasoningContent string // LLM 思考过程（推理模型）
	AnswerContent    string // LLM 最终回答（完整文本）
	SkippedRetrieval bool   // 是否跳过了检索（追问复用上轮结果）
}

// AnswerOptions 问答选项
type AnswerOptions struct {
	// History 多轮对话历史（user/assistant 交替），用于追问场景
	// 格式：[{user, q1}, {assistant, a1}, {user, q2}, ...]
	History []llm.Message
	// ReuseContext 复用已有检索结果（追问时传入上轮结果，跳过检索）
	ReuseContext *RetrievedContext
}

// Answer 执行完整问答：检索 + 流式 LLM 生成
// onToken:    每收到一个回答 token 时的回调（用于流式打印）
// onThinking: 每收到一个思考 token 时的回调（推理模型，传 nil 则忽略）
// opts:       可选配置（多轮历史、复用检索结果），传 nil 等同于无历史的独立问答
func (q *QAEngine) Answer(ctx context.Context, query string, onToken func(string), onThinking func(string), opts *AnswerOptions) (*AnswerResult, error) {
	var history []llm.Message
	var retCtx *RetrievedContext
	skipped := false

	if opts != nil {
		history = opts.History
		retCtx = opts.ReuseContext
	}

	// Step 1: 检索（追问复用上轮结果时跳过）
	if retCtx == nil {
		var err error
		retCtx, err = q.Retrieve(ctx, query)
		if err != nil {
			return nil, err
		}
	} else {
		skipped = true
		slog.Debug("[QA] 追问：复用上轮检索结果", "parents", len(retCtx.Parents))
	}

	// Step 2: 构建 messages（system + 历史 + 当前问题）
	contextSection := buildContextSection(retCtx.Parents)
	systemContent := fmt.Sprintf(qaSystemPrompt, contextSection)

	messages := make([]llm.Message, 0, 1+len(history)+1)
	messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: systemContent})
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: query})

	// Step 3: 流式 LLM 生成（带思考过程回调）
	_, streamResult, err := q.llmClient.ChatStreamEx(ctx, messages, nil, onToken, onThinking, nil)
	if err != nil {
		return &AnswerResult{RetrievedContext: retCtx, SkippedRetrieval: skipped}, fmt.Errorf("LLM 生成失败: %w", err)
	}

	result := &AnswerResult{
		RetrievedContext: retCtx,
		SkippedRetrieval: skipped,
	}
	if streamResult != nil {
		result.ReasoningContent = streamResult.ReasoningContent
		result.AnswerContent = streamResult.Content
	}
	return result, nil
}

// ── 内部工具函数 ──────────────────────────────────────────────────────────────

func (q *QAEngine) resolveParents(parentIdxs []int) []vectorstore.ParentMeta {
	parents := make([]vectorstore.ParentMeta, 0, len(parentIdxs))
	for _, idx := range parentIdxs {
		p, ok := q.store.GetParent(idx)
		if ok {
			parents = append(parents, p)
		}
	}
	return parents
}

// buildContextSection 将 parent chunks 格式化为 LLM context 段落
func buildContextSection(parents []vectorstore.ParentMeta) string {
	if len(parents) == 0 {
		return "（未检索到相关内容）"
	}
	var sb strings.Builder
	sb.WriteString("=== 检索到的参考内容 ===\n\n")
	for i, p := range parents {
		sb.WriteString(fmt.Sprintf("【参考 %d】对象: %s，第 %d 页\n", i+1, p.IDDObject, p.PageStart))
		sb.WriteString(p.Content)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// dedup 去重保持顺序
func dedup(idxs []int) []int {
	seen := make(map[int]bool)
	result := make([]int, 0, len(idxs))
	for _, idx := range idxs {
		if !seen[idx] {
			seen[idx] = true
			result = append(result, idx)
		}
	}
	return result
}
