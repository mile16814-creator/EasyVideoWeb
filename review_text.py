#!/usr/bin/env python3
"""
帖子文本审核 - 辱骂风险识别 (nlp_structbert_abuse-detect_chinese-tiny)
模型标签: 无风险 / 辱骂风险
Usage: python review_text.py <text_file_path>
  或:  echo "文本内容" | python review_text.py -
Output: JSON to stdout, progress logs to stderr
"""

import sys
import os
import json

_real_stdout = sys.stdout
sys.stdout = sys.stderr


def log(msg):
    print(f"[review_text] {msg}", flush=True)


def write_json_result(obj):
    _real_stdout.write(json.dumps(obj, ensure_ascii=False))
    _real_stdout.flush()


def main():
    if len(sys.argv) < 2:
        write_json_result({"error": "usage: review_text.py <text_file_path> or - for stdin"})
        sys.exit(1)

    src = sys.argv[1]
    if src == "-":
        text = sys.stdin.read()
    else:
        if not os.path.exists(src):
            log(f"错误: 文件不存在: {src}")
            write_json_result({"error": "file not found", "passed": True})
            sys.exit(0)
        with open(src, "r", encoding="utf-8", errors="replace") as f:
            text = f.read()

    text = (text or "").strip()
    if not text:
        write_json_result({"passed": True, "reject_reason": ""})
        return

    result = {"passed": True, "reject_reason": ""}

    try:
        from modelscope.pipelines import pipeline
    except ImportError as e:
        log(f"警告: modelscope 未安装或导入失败，跳过文本审核 (python={sys.executable}, err={e})")
        write_json_result(result)
        return

    try:
        # StructBERT辱骂风险识别-中文-外呼-tiny，标签: 无风险 / 辱骂风险
        classifier = pipeline("text-classification", model="damo/nlp_structbert_abuse-detect_chinese-tiny")

        # 长文分段检测（README 示例 max_words: 300）
        max_len = 300
        chunks = [text[i : i + max_len] for i in range(0, min(len(text), 5000), max_len)]
        if not chunks:
            write_json_result(result)
            return

        for i, chunk in enumerate(chunks):
            if not chunk.strip():
                continue
            out = classifier(chunk)
            if isinstance(out, list):
                item = out[0] if out else {}
            else:
                item = out or {}
            label = item.get("labels") or item.get("label")
            if isinstance(label, list):
                label = (label[0] if label else "") or ""
            else:
                label = str(label or "").strip()
            # 模型标签: '无风险'(通过) / '辱骂风险'(不通过)
            if label == "辱骂风险":
                log(f"检测到辱骂风险 chunk={i+1} label={label}")
                result["passed"] = False
                result["reject_reason"] = "abuse"
                break
    except Exception as e:
        log(f"文本审核异常: {e}")
        import traceback
        log(traceback.format_exc())
        # 异常时放行，避免阻塞发布
        result["passed"] = True
        result["reject_reason"] = ""

    write_json_result(result)


if __name__ == "__main__":
    main()
