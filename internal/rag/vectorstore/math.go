// math.go — 向量数学工具：点积、L2 范数、TopK min-heap、RRF 融合
package vectorstore

import (
	"container/heap"
	"math"
)

// dotProduct 计算两个等长 float32 向量的点积
func dotProduct(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// l2norm 计算 float32 向量的 L2 范数
func l2norm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}

// cosineSimilarity 计算余弦相似度；向量需已归一化 norm
func cosineSimilarity(a, b []float32, normA, normB float64) float64 {
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct(a, b) / (normA * normB)
}

// ScoredChunk 带分数的检索结果
type ScoredChunk struct {
	ChildIdx int
	Score    float64
}

// ── Min-heap for top-K ──────────────────────────────────────────────────────

type minHeap []ScoredChunk

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].Score < h[j].Score }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *minHeap) Push(x any) { *h = append(*h, x.(ScoredChunk)) }

func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// topKByScore 从 scores 中取分数最高的 K 个索引，返回按分数降序排列
func topKByScore(scores []float64, k int) []ScoredChunk {
	if k <= 0 {
		return nil
	}
	h := &minHeap{}
	heap.Init(h)

	for i, score := range scores {
		if h.Len() < k {
			heap.Push(h, ScoredChunk{ChildIdx: i, Score: score})
		} else if score > (*h)[0].Score {
			(*h)[0] = ScoredChunk{ChildIdx: i, Score: score}
			heap.Fix(h, 0)
		}
	}

	// 转为降序
	result := make([]ScoredChunk, h.Len())
	for i := h.Len() - 1; i >= 0; i-- {
		result[i] = heap.Pop(h).(ScoredChunk)
	}
	return result
}

// ── RRF 融合 ──────────────────────────────────────────────────────────────────

const rrfK = 60 // RRF 标准参数

// RRFMerge 将两路排序结果用 Reciprocal Rank Fusion 融合，返回按 RRF 分降序的 parentID 列表
// vectorRanks: childIdx 列表（按向量相似度降序）
// bm25Ranks:   childIdx 列表（按 BM25 分降序）
// childToParent: childIdx → parentIdx 映射
// topK: 最终返回条数
func RRFMerge(
	vectorRanks []int,
	bm25Ranks []int,
	childToParent []int,
	topK int,
) []int {
	rrfScores := make(map[int]float64) // parentIdx → rrf score

	for rank, childIdx := range vectorRanks {
		parentIdx := childToParent[childIdx]
		rrfScores[parentIdx] += 1.0 / float64(rrfK+rank+1)
	}
	for rank, childIdx := range bm25Ranks {
		parentIdx := childToParent[childIdx]
		rrfScores[parentIdx] += 1.0 / float64(rrfK+rank+1)
	}

	// 取 top-K parents
	type scored struct {
		parentIdx int
		score     float64
	}
	var candidates []scored
	for pid, score := range rrfScores {
		candidates = append(candidates, scored{pid, score})
	}
	// 简单排序（数量通常 <= 40，无需 heap）
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	result := make([]int, 0, topK)
	for i, c := range candidates {
		if i >= topK {
			break
		}
		result = append(result, c.parentIdx)
	}
	return result
}
