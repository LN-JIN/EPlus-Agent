"""
EnergyPlus IOR RAG 索引构建脚本
====================================================
将 InputOutputReference-Part.pdf 解析为 Parent/Child chunk，
调用 DashScope text-embedding-v4 生成向量，构建 BM25 倒排索引，
输出二进制 .idx 文件供 Go 运行时加载。

用法:
    python scripts/build_rag_index.py \
        --pdf data/InputOutputReference-Part.pdf \
        --out data/index/ior_part.idx \
        --api-key $DASHSCOPE_API_KEY \
        [--model text-embedding-v4] \
        [--dim 1024] \
        [--batch-size 10]
"""

import argparse
import json
import math
import re
import struct
import sys
import time
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

try:
    import pdfplumber
except ImportError:
    print("请先安装: pip install pdfplumber")
    sys.exit(1)

try:
    import tiktoken
except ImportError:
    print("请先安装: pip install tiktoken")
    sys.exit(1)

try:
    import dashscope
    from dashscope import TextEmbedding
except ImportError:
    print("请先安装: pip install dashscope")
    sys.exit(1)

try:
    from tqdm import tqdm
except ImportError:
    # tqdm 可选，降级为简单打印
    def tqdm(iterable, **kwargs):
        desc = kwargs.get("desc", "")
        items = list(iterable)
        print(f"{desc}: {len(items)} items")
        return items


# ─── 正则模式 ──────────────────────────────────────────────────────────────────

# IDD 对象名：首字母大写 + 冒号组合，行尾可有逗号
IDD_OBJECT_RE = re.compile(
    r'^([A-Z][a-zA-Z0-9]{1,30}(?::[A-Z][a-zA-Z0-9]{1,30}){1,5})\s*,?\s*$'
)

# IDD 字段行：以空格开头的 A1, / N1, 等
FIELD_LINE_RE = re.compile(r'^[AN]\d+[,;]')

# 输出变量小节标题
OUTPUT_VARS_RE = re.compile(
    r'^(Output Variables?|Outputs?)\s*$',
    re.IGNORECASE
)


# ─── 数据结构 ──────────────────────────────────────────────────────────────────

@dataclass
class ParentChunk:
    id: str
    content: str
    idd_object: str          # 所属 IDD 对象名（如 "BuildingSurface:Detailed"）
    section: str             # 同 idd_object（便于 Go 端统一显示）
    page_start: int
    content_type: str        # "idd_object" | "idd_object_cont" | "output_vars"


@dataclass
class ChildChunk:
    id: str
    parent_id: str
    content: str             # 带对象名前缀的文本（用于 embedding）
    idd_object: str
    content_type: str        # "field_table" | "output_vars" | "narrative"
    embedding: Optional[list] = field(default=None, repr=False)


# ─── Token 计数 ────────────────────────────────────────────────────────────────

_enc = tiktoken.get_encoding("cl100k_base")


def count_tokens(text: str) -> int:
    return len(_enc.encode(text))


def split_by_tokens(text: str, max_tokens: int, overlap: int = 50) -> list[str]:
    """将文本按 token 数切分，支持重叠。"""
    tokens = _enc.encode(text)
    if len(tokens) <= max_tokens:
        return [text]
    chunks = []
    start = 0
    while start < len(tokens):
        end = min(start + max_tokens, len(tokens))
        chunk_tokens = tokens[start:end]
        chunks.append(_enc.decode(chunk_tokens))
        if end == len(tokens):
            break
        start = end - overlap
    return chunks


# ─── PDF 解析 ──────────────────────────────────────────────────────────────────

def is_toc_page(page_text: str) -> bool:
    """目录页特征：含大量省略号连接对象名和页码。"""
    dot_lines = sum(1 for line in page_text.splitlines() if '.....' in line)
    return dot_lines > 5


def parse_pdf(pdf_path: str) -> list[tuple[int, str]]:
    """
    提取 PDF 文本，跳过目录页。
    返回: [(page_num, page_text), ...]
    """
    pages = []
    with pdfplumber.open(pdf_path) as pdf:
        total = len(pdf.pages)
        for i, page in enumerate(tqdm(pdf.pages, desc="提取 PDF 文本", total=total)):
            page_num = i + 1
            text = page.extract_text() or ""
            if is_toc_page(text):
                continue
            if text.strip():
                pages.append((page_num, text))
    return pages


# ─── IDD 对象分割 ──────────────────────────────────────────────────────────────

def split_into_idd_objects(pages: list[tuple[int, str]]) -> list[tuple[str, int, list[str]]]:
    """
    将页面列表按 IDD 对象名切割。
    返回: [(object_name, start_page, lines), ...]
    """
    objects = []
    current_obj = None
    current_page = 1
    current_lines = []

    for page_num, text in pages:
        lines = text.splitlines()
        for line in lines:
            stripped = line.strip()
            m = IDD_OBJECT_RE.match(stripped)
            if m:
                # 保存上一个对象
                if current_obj is not None and current_lines:
                    objects.append((current_obj, current_page, current_lines))
                # 开始新对象
                current_obj = m.group(1)
                current_page = page_num
                current_lines = [line]
            else:
                if current_obj is not None:
                    current_lines.append(line)

    # 保存最后一个对象
    if current_obj is not None and current_lines:
        objects.append((current_obj, current_page, current_lines))

    # 处理文档开头无 IDD 对象名的叙述内容
    if not objects:
        all_lines = []
        for page_num, text in pages:
            all_lines.extend(text.splitlines())
        objects = [("__narrative__", 1, all_lines)]

    return objects


# ─── Parent/Child 分块 ─────────────────────────────────────────────────────────

PARENT_MAX_TOKENS = 512
CHILD_MAX_TOKENS = 200
CHILD_OVERLAP = 50


def make_parent_child(
    obj_name: str,
    start_page: int,
    lines: list[str],
    parent_id_base: str,
) -> tuple[list[ParentChunk], list[ChildChunk]]:
    """
    将单个 IDD 对象的文本生成 Parent/Child chunk 列表。
    """
    parents: list[ParentChunk] = []
    children: list[ChildChunk] = []

    # 分离字段内容和输出变量内容
    main_lines = []
    output_lines = []
    in_output_section = False

    for line in lines:
        if OUTPUT_VARS_RE.match(line.strip()):
            in_output_section = True
        if in_output_section:
            output_lines.append(line)
        else:
            main_lines.append(line)

    def _make_parents_for_block(
        block_lines: list[str],
        content_type: str,
        id_suffix: str,
    ) -> list[ParentChunk]:
        """将 block_lines 切成 Parent chunks。"""
        text = "\n".join(block_lines).strip()
        if not text:
            return []
        # 加对象名前缀（每段 parent 都携带，方便 Go 端显示）
        prefix = f"[{obj_name}]\n" if obj_name != "__narrative__" else ""
        parts = split_by_tokens(text, PARENT_MAX_TOKENS, overlap=80)
        result = []
        for i, part in enumerate(parts):
            pid = f"{parent_id_base}_{id_suffix}_{i}"
            suffix_label = f"续 {i + 1}" if i > 0 else ""
            header = f"[{obj_name}{(' - ' + suffix_label) if suffix_label else ''}]" if obj_name != "__narrative__" else ""
            result.append(ParentChunk(
                id=pid,
                content=f"{header}\n{part}".strip(),
                idd_object=obj_name,
                section=obj_name,
                page_start=start_page,
                content_type=content_type if i == 0 else "idd_object_cont",
            ))
        return result

    # 生成主内容 Parents
    main_parents = _make_parents_for_block(main_lines, "idd_object", "main")
    # 生成输出变量 Parents
    out_parents = _make_parents_for_block(output_lines, "output_vars", "outvars")

    all_parents = main_parents + out_parents
    parents.extend(all_parents)

    # 为每个 Parent 生成 Child chunks
    for parent in all_parents:
        # 去掉 [对象名] 前缀行，只对内容分块
        content_body = parent.content
        child_prefix = f"[{obj_name}] " if obj_name != "__narrative__" else ""

        child_texts = split_by_tokens(content_body, CHILD_MAX_TOKENS, overlap=CHILD_OVERLAP)
        for j, ct in enumerate(child_texts):
            cid = f"{parent.id}_c{j}"
            # 确定 child content_type
            if parent.content_type == "output_vars":
                ctype = "output_vars"
            elif any(FIELD_LINE_RE.match(line) for line in ct.splitlines()):
                ctype = "field_table"
            else:
                ctype = "narrative"

            children.append(ChildChunk(
                id=cid,
                parent_id=parent.id,
                content=child_prefix + ct,
                idd_object=obj_name,
                content_type=ctype,
            ))

    return parents, children


def build_chunks(pages: list[tuple[int, str]]) -> tuple[list[ParentChunk], list[ChildChunk]]:
    """完整分块流程。"""
    idd_objects = split_into_idd_objects(pages)
    print(f"检测到 IDD 对象数: {len(idd_objects)}")

    all_parents: list[ParentChunk] = []
    all_children: list[ChildChunk] = []

    for idx, (obj_name, start_page, lines) in enumerate(
        tqdm(idd_objects, desc="生成 Parent/Child chunks")
    ):
        base_id = f"p{idx:04d}"
        parents, children = make_parent_child(obj_name, start_page, lines, base_id)
        all_parents.extend(parents)
        all_children.extend(children)

    print(f"Parent chunks: {len(all_parents)}")
    print(f"Child  chunks: {len(all_children)}")
    return all_parents, all_children


# ─── Embedding ─────────────────────────────────────────────────────────────────

def embed_batch(texts: list[str], api_key: str, model: str, dim: int) -> list[list[float]]:
    """调用 DashScope text-embedding-v4，返回向量列表。"""
    dashscope.api_key = api_key
    resp = TextEmbedding.call(
        model=model,
        input=texts,
        dimension=dim,
    )
    if resp.status_code != 200:
        raise RuntimeError(f"Embedding API 错误: {resp.message}")
    embeddings = sorted(resp.output["embeddings"], key=lambda x: x["text_index"])
    return [e["embedding"] for e in embeddings]


def embed_all_children(
    children: list[ChildChunk],
    api_key: str,
    model: str,
    dim: int,
    batch_size: int = 10,
) -> None:
    """为所有 child chunk 生成 embedding（in-place 修改）。"""
    total = len(children)
    print(f"开始 embedding {total} 个 child chunks（批大小={batch_size}）...")

    for i in tqdm(range(0, total, batch_size), desc="Embedding"):
        batch = children[i:i + batch_size]
        texts = [c.content for c in batch]
        retry = 0
        while retry < 5:
            try:
                vecs = embed_batch(texts, api_key, model, dim)
                for c, v in zip(batch, vecs):
                    c.embedding = v
                break
            except Exception as e:
                retry += 1
                wait = 2 ** retry
                print(f"\nAPI 错误（重试 {retry}/5，等待 {wait}s）: {e}")
                time.sleep(wait)
        else:
            raise RuntimeError(f"Embedding 批次 {i} 连续失败 5 次，中止")

        # 批次间短暂等待避免限流
        if i + batch_size < total:
            time.sleep(0.2)


# ─── BM25 索引 ─────────────────────────────────────────────────────────────────

# EnergyPlus 专有词表（保证分词时不被拆散）
EPLUS_TERMS = [
    # IDD 字段类型
    "alpha", "real", "integer", "choice", "object-list",
    "autocalculate", "autosizeable", "autosizable",
    # 常见输出变量单位
    "[C]", "[W]", "[J]", "[m3/s]", "[kg/s]", "[W/m2]", "[Pa]", "[m]",
    # 通用关键词
    "required-field", "minimum", "maximum", "default",
    "Output Variables", "Output Variable",
]


def tokenize(text: str) -> list[str]:
    """简单分词：小写 + 按非字母数字冒号分割。"""
    text = text.lower()
    tokens = re.findall(r'[a-z0-9:]+', text)
    return [t for t in tokens if len(t) > 1]


def build_bm25_index(children: list[ChildChunk]) -> dict:
    """
    构建 BM25 倒排索引。
    返回: {term: {"postings": [[child_idx, tf], ...], "idf": float}}
    """
    N = len(children)
    tf_map = defaultdict(lambda: defaultdict(int))   # term -> {child_idx: tf}
    df_map = defaultdict(int)                         # term -> doc freq

    total_len = 0
    for idx, child in enumerate(children):
        tokens = tokenize(child.content)
        total_len += len(tokens)
        seen = set()
        for token in tokens:
            tf_map[token][idx] += 1
            if token not in seen:
                df_map[token] += 1
                seen.add(token)

    avg_dl = total_len / max(N, 1)
    k1, b = 1.5, 0.75

    dl_map = {}
    for idx, child in enumerate(children):
        dl_map[idx] = len(tokenize(child.content))

    index = {}
    for term, postings in tf_map.items():
        df = df_map[term]
        idf = math.log((N - df + 0.5) / (df + 0.5) + 1)
        # 只保留 BM25 分数 > 0 的 term（低频词）
        bm25_postings = []
        for child_idx, tf in postings.items():
            dl = dl_map[child_idx]
            score = idf * (tf * (k1 + 1)) / (tf + k1 * (1 - b + b * dl / avg_dl))
            bm25_postings.append([child_idx, round(score, 4)])
        bm25_postings.sort(key=lambda x: -x[1])  # 按分数降序
        index[term] = {"postings": bm25_postings, "idf": round(idf, 4)}

    print(f"BM25 词表大小: {len(index)}")
    return index


# ─── 二进制索引写入 ────────────────────────────────────────────────────────────

MAGIC = b"EPLUSIDX"
VERSION = 2


def write_index(
    out_path: str,
    parents: list[ParentChunk],
    children: list[ChildChunk],
    bm25_index: dict,
    dim: int,
) -> None:
    """
    写入二进制 .idx 文件。
    格式:
      [HEADER 128 bytes]
      [CHILD EMBEDDING MATRIX]  num_children × dim × float32
      [BM25 NDJSON]
      [PARENT METADATA NDJSON]
      [CHILD METADATA NDJSON]
    """
    # 验证所有 child 都有 embedding
    missing = [c.id for c in children if c.embedding is None]
    if missing:
        raise ValueError(f"以下 child 缺少 embedding: {missing[:5]}...")

    print(f"写入索引文件: {out_path}")
    Path(out_path).parent.mkdir(parents=True, exist_ok=True)

    # 编码各段
    # 1. Child embedding matrix (float32 little-endian)
    embedding_bytes = bytearray()
    for child in children:
        for v in child.embedding:
            embedding_bytes += struct.pack('<f', float(v))

    # 2. BM25 NDJSON
    bm25_lines = []
    for term, data in bm25_index.items():
        bm25_lines.append(json.dumps(
            {"term": term, "postings": data["postings"], "idf": data["idf"]},
            ensure_ascii=False,
        ))
    bm25_bytes = ("\n".join(bm25_lines) + "\n").encode("utf-8")

    # 3. Parent metadata NDJSON
    parent_lines = []
    for p in parents:
        parent_lines.append(json.dumps({
            "id": p.id,
            "content": p.content,
            "idd_object": p.idd_object,
            "section": p.section,
            "page_start": p.page_start,
            "content_type": p.content_type,
        }, ensure_ascii=False))
    parent_bytes = ("\n".join(parent_lines) + "\n").encode("utf-8")

    # 4. Child metadata NDJSON（不含 embedding）
    child_lines = []
    for c in children:
        child_lines.append(json.dumps({
            "id": c.id,
            "parent_id": c.parent_id,
            "idd_object": c.idd_object,
            "content_type": c.content_type,
        }, ensure_ascii=False))
    child_bytes = ("\n".join(child_lines) + "\n").encode("utf-8")

    # 计算各段偏移量
    header_size = 128
    child_matrix_offset = header_size
    bm25_offset = child_matrix_offset + len(embedding_bytes)
    parent_meta_offset = bm25_offset + len(bm25_bytes)
    child_meta_offset = parent_meta_offset + len(parent_bytes)

    # 写入文件
    with open(out_path, "wb") as f:
        # HEADER (128 bytes)
        header = struct.pack(
            "<8sHHII4Q",
            MAGIC,           # 8s magic
            VERSION,         # H version
            dim,             # H child_dim
            len(parents),    # I num_parents
            len(children),   # I num_children
            child_matrix_offset,   # Q
            bm25_offset,           # Q
            parent_meta_offset,    # Q
            child_meta_offset,     # Q
        )
        # struct.calcsize("<8sHHII4Q") = 8+2+2+4+4+8*4 = 52 bytes
        # 填充到 128 bytes
        header = header + b'\x00' * (128 - len(header))
        f.write(header)

        f.write(embedding_bytes)
        f.write(bm25_bytes)
        f.write(parent_bytes)
        f.write(child_bytes)

    total_size = Path(out_path).stat().st_size
    print(f"索引写入完成: {out_path} ({total_size / 1024 / 1024:.1f} MB)")
    print(f"  Header:          128 bytes")
    print(f"  Child embeddings: {len(embedding_bytes) / 1024 / 1024:.1f} MB")
    print(f"  BM25 index:       {len(bm25_bytes) / 1024:.0f} KB")
    print(f"  Parent metadata:  {len(parent_bytes) / 1024:.0f} KB")
    print(f"  Child metadata:   {len(child_bytes) / 1024:.0f} KB")


# ─── 主流程 ────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="EnergyPlus IOR RAG 索引构建")
    parser.add_argument("--pdf", default="data/InputOutputReference-Part.pdf",
                        help="PDF 路径")
    parser.add_argument("--out", default="data/index/ior_part.idx",
                        help="输出 .idx 路径")
    parser.add_argument("--api-key", required=True,
                        help="DashScope API Key")
    parser.add_argument("--model", default="text-embedding-v4",
                        help="Embedding 模型名（默认: text-embedding-v4）")
    parser.add_argument("--dim", type=int, default=1024,
                        help="向量维度（默认: 1024）")
    parser.add_argument("--batch-size", type=int, default=10,
                        help="Embedding 批大小（默认: 10）")
    args = parser.parse_args()

    print("=== EnergyPlus IOR RAG 索引构建 ===")
    print(f"PDF:   {args.pdf}")
    print(f"输出:  {args.out}")
    print(f"模型:  {args.model} (dim={args.dim})")
    print()

    # Step 1: 提取 PDF 文本
    pages = parse_pdf(args.pdf)
    print(f"有效页数: {len(pages)}\n")

    # Step 2: 分块
    parents, children = build_chunks(pages)
    print()

    # Step 3: Embedding
    embed_all_children(children, args.api_key, args.model, args.dim, args.batch_size)
    print()

    # Step 4: BM25 索引
    bm25_index = build_bm25_index(children)
    print()

    # Step 5: 写入二进制索引
    write_index(args.out, parents, children, bm25_index, args.dim)
    print("\n✓ 完成！")


if __name__ == "__main__":
    main()
