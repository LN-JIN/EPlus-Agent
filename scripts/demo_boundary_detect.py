"""
IDD 对象边界检测验证 Demo
====================================================
用途: 在构建完整索引前，验证 IDD 对象边界检测正则是否正确工作。
      无需 API key，只需安装 pdfplumber。

用法:
    pip install pdfplumber
    python scripts/demo_boundary_detect.py [--pdf PATH] [--pages N]

输出:
    1. 检测到的目录页列表（这些页面将被跳过，不生成 chunk）
    2. 检测到的所有 IDD 对象名 + 所在页码
    3. 统计摘要
"""

import argparse
import re
import sys
from pathlib import Path

try:
    import pdfplumber
except ImportError:
    print("请先安装: pip install pdfplumber")
    sys.exit(1)

# IDD 对象名正则：首字母大写单词 + 冒号 + 首字母大写单词，可多段
# 匹配: BuildingSurface:Detailed, ZoneHVAC:EquipmentList, Output:Variable 等
IDD_OBJECT_RE = re.compile(
    r'^([A-Z][a-zA-Z0-9]{1,30}(?::[A-Z][a-zA-Z0-9]{1,30}){1,5})\s*,?\s*$'
)

# 字段行标识：以空格开头的 A1, / N1, 等
FIELD_LINE_RE = re.compile(r'^[AN]\d+[,;]')


def is_toc_page(page_text: str) -> bool:
    """
    目录页特征：含大量点线（.....）连接对象名和页码。
    通常文档前几页是目录。
    """
    lines = page_text.splitlines()
    dot_lines = sum(1 for line in lines if '.....' in line)
    return dot_lines > 5


def detect_idd_objects(pdf_path: str, max_pages: int = 0) -> None:
    """
    扫描 PDF，检测 IDD 对象边界，输出检测结果。

    Args:
        pdf_path: PDF 文件路径
        max_pages: 最多扫描页数，0 表示扫描全部
    """
    path = Path(pdf_path)
    if not path.exists():
        print(f"错误: 文件不存在: {pdf_path}")
        sys.exit(1)

    print(f"打开文件: {path.name} ({path.stat().st_size / 1024 / 1024:.1f} MB)")
    print("=" * 60)

    toc_pages = []
    detected_objects = []  # list of (page_num, object_name)
    field_pages = []       # 含字段行的页码（确认是内容页）

    with pdfplumber.open(pdf_path) as pdf:
        total_pages = len(pdf.pages)
        scan_pages = total_pages if max_pages == 0 else min(max_pages, total_pages)
        print(f"总页数: {total_pages}，本次扫描: {scan_pages} 页\n")

        for page_num in range(1, scan_pages + 1):
            page = pdf.pages[page_num - 1]
            text = page.extract_text() or ""
            lines = text.splitlines()

            # 检测目录页
            if is_toc_page(text):
                toc_pages.append(page_num)
                continue  # 目录页跳过对象检测

            # 检测字段行（证明是内容页）
            has_field_line = any(FIELD_LINE_RE.match(line) for line in lines)
            if has_field_line:
                field_pages.append(page_num)

            # 检测 IDD 对象名
            for line in lines:
                stripped = line.strip()
                m = IDD_OBJECT_RE.match(stripped)
                if m:
                    obj_name = m.group(1)
                    detected_objects.append((page_num, obj_name))

    # 输出结果
    print(f"【目录页（将被跳过）】共 {len(toc_pages)} 页")
    if toc_pages:
        print(f"  页码: {toc_pages}")
    print()

    print(f"【检测到的 IDD 对象】共 {len(detected_objects)} 个")
    print("-" * 60)
    for page_num, obj_name in detected_objects:
        print(f"  第 {page_num:4d} 页: {obj_name}")
    print()

    print(f"【统计摘要】")
    print(f"  扫描页数:       {scan_pages}")
    print(f"  目录页数:       {len(toc_pages)}")
    print(f"  含字段行页数:   {len(field_pages)}")
    print(f"  检测到对象数:   {len(detected_objects)}")

    # 提取唯一对象名
    unique_objects = sorted(set(name for _, name in detected_objects))
    print(f"  唯一对象数:     {len(unique_objects)}")
    print()
    print("【唯一对象名列表】")
    for name in unique_objects:
        print(f"  {name}")

    # 误报检查提示
    print()
    print("【请人工检查】")
    print("  1. 上方对象名是否都是真实的 EnergyPlus IDD 对象？")
    print("  2. 是否有误报（普通文字被识别为对象名）？")
    print("  3. 目录页是否被正确过滤？")
    print("  如有误报，调整 IDD_OBJECT_RE 正则后重新运行。")


def main():
    parser = argparse.ArgumentParser(description="IDD 对象边界检测验证 Demo")
    parser.add_argument(
        "--pdf",
        default="data/InputOutputReference-Part.pdf",
        help="PDF 文件路径（默认: data/InputOutputReference-Part.pdf）"
    )
    parser.add_argument(
        "--pages",
        type=int,
        default=0,
        help="最多扫描页数，0 表示扫描全部（默认: 0）"
    )
    args = parser.parse_args()

    detect_idd_objects(args.pdf, args.pages)


if __name__ == "__main__":
    main()
