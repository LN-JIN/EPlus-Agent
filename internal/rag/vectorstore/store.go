// store.go — VectorStore：从二进制 .idx 文件加载，支持向量检索
package vectorstore

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	idxMagic   = "EPLUSIDX"
	idxVersion = 2
	headerSize = 128
)

// ParentMeta .idx 文件中的 Parent chunk 元数据
type ParentMeta struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	IDDObject   string `json:"idd_object"`
	Section     string `json:"section"`
	PageStart   int    `json:"page_start"`
	ContentType string `json:"content_type"`
}

// ChildMeta .idx 文件中的 Child chunk 元数据（不含 embedding）
type ChildMeta struct {
	ID          string `json:"id"`
	ParentID    string `json:"parent_id"`
	IDDObject   string `json:"idd_object"`
	ContentType string `json:"content_type"`
}

// VectorStore 内存向量库：加载后支持余弦相似度搜索
type VectorStore struct {
	dim          int
	embeddings   []float32     // flat array: child i 从 i*dim 开始
	norms        []float64     // 预计算 L2 范数
	children     []ChildMeta   // 与 embeddings 等长
	parents      []ParentMeta  // parent chunk
	parentIndex  map[string]int // parentID → parents 数组索引
	childToParent []int          // childIdx → parentIdx
	bm25         *BM25Index
}

// idxHeader .idx 文件头（128 bytes）
type idxHeader struct {
	Magic           [8]byte
	Version         uint16
	ChildDim        uint16
	NumParents      uint32
	NumChildren     uint32
	ChildMatrixOff  uint64
	BM25Off         uint64
	ParentMetaOff   uint64
	ChildMetaOff    uint64
	_               [72]byte // reserved
}

// LoadFromFile 从 .idx 文件加载向量库
func LoadFromFile(path string) (*VectorStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取索引文件失败: %w", err)
	}
	if len(data) < headerSize {
		return nil, fmt.Errorf("索引文件过小（%d bytes）", len(data))
	}

	// 解析 header
	var hdr struct {
		Magic          [8]byte
		Version        uint16
		ChildDim       uint16
		NumParents     uint32
		NumChildren    uint32
		ChildMatrixOff uint64
		BM25Off        uint64
		ParentMetaOff  uint64
		ChildMetaOff   uint64
	}
	r := bytes.NewReader(data[:headerSize])
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("解析 header 失败: %w", err)
	}
	if string(hdr.Magic[:]) != idxMagic {
		return nil, fmt.Errorf("无效 magic: %q", hdr.Magic)
	}
	if hdr.Version != idxVersion {
		return nil, fmt.Errorf("不支持的索引版本: %d", hdr.Version)
	}

	dim := int(hdr.ChildDim)
	numChildren := int(hdr.NumChildren)
	numParents := int(hdr.NumParents)

	// 加载 child embedding matrix
	matrixSize := numChildren * dim * 4 // float32 = 4 bytes
	if int(hdr.ChildMatrixOff)+matrixSize > len(data) {
		return nil, fmt.Errorf("embedding matrix 超出文件范围")
	}
	matrixBytes := data[hdr.ChildMatrixOff : int(hdr.ChildMatrixOff)+matrixSize]
	embeddings := make([]float32, numChildren*dim)
	if err := binary.Read(bytes.NewReader(matrixBytes), binary.LittleEndian, embeddings); err != nil {
		return nil, fmt.Errorf("加载 embedding matrix 失败: %w", err)
	}

	// 预计算 L2 范数
	norms := make([]float64, numChildren)
	for i := 0; i < numChildren; i++ {
		norms[i] = l2norm(embeddings[i*dim : (i+1)*dim])
	}

	// 加载 BM25 索引（NDJSON）
	bm25End := int(hdr.ParentMetaOff)
	bm25Data := data[hdr.BM25Off:bm25End]
	bm25, err := loadBM25(bm25Data)
	if err != nil {
		return nil, fmt.Errorf("加载 BM25 索引失败: %w", err)
	}

	// 加载 parent metadata
	parentEnd := int(hdr.ChildMetaOff)
	parents, err := loadParentMeta(data[hdr.ParentMetaOff:parentEnd], numParents)
	if err != nil {
		return nil, fmt.Errorf("加载 parent metadata 失败: %w", err)
	}

	// 加载 child metadata
	children, err := loadChildMeta(data[hdr.ChildMetaOff:], numChildren)
	if err != nil {
		return nil, fmt.Errorf("加载 child metadata 失败: %w", err)
	}

	// 构建 parent index 映射
	parentIndex := make(map[string]int, numParents)
	for i, p := range parents {
		parentIndex[p.ID] = i
	}

	// 构建 child → parent 索引
	childToParent := make([]int, numChildren)
	for i, c := range children {
		idx, ok := parentIndex[c.ParentID]
		if !ok {
			idx = 0
		}
		childToParent[i] = idx
	}

	return &VectorStore{
		dim:           dim,
		embeddings:    embeddings,
		norms:         norms,
		children:      children,
		parents:       parents,
		parentIndex:   parentIndex,
		childToParent: childToParent,
		bm25:          bm25,
	}, nil
}

func loadParentMeta(data []byte, n int) ([]ParentMeta, error) {
	parents := make([]ParentMeta, 0, n)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 10*1024*1024), 10*1024*1024) // 10MB buffer（parent content 可能较大）
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var p ParentMeta
		if err := json.Unmarshal(line, &p); err != nil {
			return nil, fmt.Errorf("解析 parent metadata 行失败: %w", err)
		}
		parents = append(parents, p)
	}
	return parents, scanner.Err()
}

func loadChildMeta(data []byte, n int) ([]ChildMeta, error) {
	children := make([]ChildMeta, 0, n)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var c ChildMeta
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("解析 child metadata 行失败: %w", err)
		}
		children = append(children, c)
	}
	return children, scanner.Err()
}

// Len 返回 child chunk 数量
func (s *VectorStore) Len() int { return len(s.children) }

// NumParents 返回 parent chunk 数量
func (s *VectorStore) NumParents() int { return len(s.parents) }

// VectorSearch 余弦相似度检索，返回 top-K child 索引（降序）
func (s *VectorStore) VectorSearch(queryVec []float32, topK int) []int {
	if len(queryVec) != s.dim {
		return nil
	}
	queryNorm := l2norm(queryVec)

	scores := make([]float64, len(s.children))
	for i := 0; i < len(s.children); i++ {
		child := s.embeddings[i*s.dim : (i+1)*s.dim]
		scores[i] = cosineSimilarity(child, queryVec, s.norms[i], queryNorm)
	}

	topChunks := topKByScore(scores, topK)
	result := make([]int, len(topChunks))
	for i, sc := range topChunks {
		result[i] = sc.ChildIdx
	}
	return result
}

// BM25Search 关键词检索，返回 top-K child 索引
func (s *VectorStore) BM25Search(query string, topK int) []int {
	return s.bm25.Query(query, topK)
}

// GetParentsByChildren 根据 child 索引列表获取去重的 parent 索引列表
func (s *VectorStore) GetParentsByChildren(childIdxs []int) []int {
	seen := make(map[int]bool)
	var result []int
	for _, ci := range childIdxs {
		if ci < 0 || ci >= len(s.childToParent) {
			continue
		}
		pi := s.childToParent[ci]
		if !seen[pi] {
			seen[pi] = true
			result = append(result, pi)
		}
	}
	return result
}

// GetParent 根据 parent 索引返回 ParentMeta
func (s *VectorStore) GetParent(parentIdx int) (ParentMeta, bool) {
	if parentIdx < 0 || parentIdx >= len(s.parents) {
		return ParentMeta{}, false
	}
	return s.parents[parentIdx], true
}

// ChildToParentIdx 返回 child → parent 索引映射（供 RRF 使用）
func (s *VectorStore) ChildToParentIdx() []int {
	return s.childToParent
}

// ReadAll 从 io.Reader 读取所有字节（兼容旧版 Go 无 io.ReadAll）
func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
