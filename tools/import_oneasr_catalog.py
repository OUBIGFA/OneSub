"""Extract OneAsr's pinned model catalog; do not maintain a second set of hashes.

Input is the reviewed upstream catalog.rs at the revision below. The converter
fails if an expected entry cannot be read, rather than dropping a model silently.
Labels are product names, the same in every interface language; the settings
name each model's role beside it.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re

REVISION = "9c1b51085b0ee084a0c8e11cd069021fc1ea7d0f"
MODELS = [
    ("qwen3_asr_06b_files", "Qwen3-ASR-0.6B-hf", "Qwen3-ASR 0.6B", "asr", "Qwen", "Qwen3-ASR-0.6B-hf", 3),
    ("qwen3_asr_17b_files", "Qwen3-ASR-1.7B-hf", "Qwen3-ASR 1.7B", "asr", "Qwen", "Qwen3-ASR-1.7B-hf", 3),
    ("qwen3_asr_06b_int8_files", "Qwen3-ASR-0.6B-int8", "Qwen3-ASR 0.6B INT8", "asr", "eclipse005", "Qwen3-ASR-0.6B-int8", 3),
    ("qwen3_asr_17b_int8_files", "Qwen3-ASR-1.7B-int8", "Qwen3-ASR 1.7B INT8", "asr", "eclipse005", "Qwen3-ASR-1.7B-int8", 3),
    ("omni_asr_ctc_300m_files", "omniASR-CTC-300M-v2-hf", "CTC 300M", "aligner", "eclipse005", "omniASR-CTC-300M-v2", 6),
    ("qwen_align_files", "Qwen3-ForcedAligner-0.6B-hf", "Qwen3-ForcedAligner 0.6B", "aligner", "Qwen", "Qwen3-ForcedAligner-0.6B-hf", 4),
    ("htdemucs_ft_files", "htdemucs_ft", "HTDemucs ft", "separation", "eclipse005", "htdemucs", 1),
    ("sensevoice_small_files", "SenseVoiceSmall", "SenseVoiceSmall", "hotword", "iic", "SenseVoiceSmall", 4),
]


def extract(source):
    models = []
    for function, model_id, label, kind, owner, repo, count in MODELS:
        match = re.search(rf"fn {function}\(\).*?\n\}}", source, re.S)
        if not match:
            raise ValueError(f"Missing model function: {function}")
        body = match.group()
        constants = dict(re.findall(r'const (\w+): &str = "([a-f0-9]+)";', body))
        files = []
        for item in re.findall(r"CatalogFile \{(.*?)\}", body, re.S):
            filename = re.search(r'file_name:\s*("[^"]+"|crate::engine::local::DEMUCS_WEIGHTS_FILE)', item).group(1)
            filename = "htdemucs_ft_vocals.safetensors" if filename.startswith("crate::") else filename.strip('"')
            size = int(re.search(r"size:\s*([\d_]+)", item).group(1).replace("_", ""))
            digest = re.search(r'sha256:\s*"([a-f0-9]{64})"', item).group(1)
            revision = re.search(r'revision:\s*("[a-f0-9]+"|\w+)', item).group(1)
            revision = revision.strip('"') if revision.startswith('"') else constants[revision]
            files.append(dict(name=filename, size=size, sha256=digest, url=f"https://modelscope.cn/models/{owner}/{repo}/resolve/{revision}/{filename}"))
        if len(files) != count:
            raise ValueError(f"{model_id}: expected {count} files, got {len(files)}")
        models.append(dict(id=model_id, label=label, kind=kind, files=files))
    return dict(upstream="https://github.com/eclipse005/OneAsr", revision=REVISION, sourceSha256=hashlib.sha256(source.encode()).hexdigest(), models=models)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    result = extract(args.source.read_text(encoding="utf-8"))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    print(f"Extracted {len(result['models'])} models")
