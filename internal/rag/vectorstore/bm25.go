// bm25.go — BM25 倒排索引：从 .idx 文件加载，支持查询
package vectorstore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// bm25Entry 单个词条的倒排记录
type bm25Entry struct {
	Term     string      `json:"term"`
	Postings [][2]float64 `json:"postings"` // [[child_idx, bm25_score], ...]
	IDF      float64     `json:"idf"`
}

// BM25Index 内存倒排索引
type BM25Index struct {
	index map[string][]posting // term → postings
}

type posting struct {
	childIdx int
	score    float64
}

// tokenRE 分词正则：保留字母数字和冒号（支持 IDD 对象名）
var tokenRE = regexp.MustCompile(`[a-z0-9:]+`)

// tokenize 将文本小写分词
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	matches := tokenRE.FindAllString(lower, -1)
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			result = append(result, m)
		}
	}
	return result
}

// loadBM25 从 NDJSON 字节流加载 BM25 索引
func loadBM25(data []byte) (*BM25Index, error) {
	idx := &BM25Index{
		index: make(map[string][]posting),
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry bm25Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		ps := make([]posting, 0, len(entry.Postings))
		for _, p := range entry.Postings {
			ps = append(ps, posting{
				childIdx: int(p[0]),
				score:    p[1],
			})
		}
		idx.index[entry.Term] = ps
	}
	return idx, scanner.Err()
}

// Query 对查询文本做 BM25 检索，返回按分数降序的 child 索引列表（最多 topK 个）
func (b *BM25Index) Query(query string, topK int) []int {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}

	// 累加各词条分数
	scoreMap := make(map[int]float64)
	for _, term := range terms {
		ps, ok := b.index[term]
		if !ok {
			continue
		}
		for _, p := range ps {
			scoreMap[p.childIdx] += p.score
		}
	}

	// 排序取 top-K
	type scored struct {
		idx   int
		score float64
	}
	candidates := make([]scored, 0, len(scoreMap))
	for idx, score := range scoreMap {
		candidates = append(candidates, scored{idx, score})
	}
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
		result = append(result, c.idx)
	}
	return result
}
