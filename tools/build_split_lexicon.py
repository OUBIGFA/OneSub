"""Compile a fixed MIT jieba lexicon into an embedded, compact double-array trie.

The upstream text is a build input in ignored scratch space, not a runtime
file. The generated JSON is UTF-8/base64 (not a repository binary exception).
Reproduction: python tools/build_split_lexicon.py --source <dict.txt.big>
"""

import argparse
import base64
from collections import deque
import hashlib
import json
import math
from pathlib import Path
import struct
import zlib

ROOT = Path(__file__).resolve().parent.parent
SOURCE_SHA256 = "b16011275c42955ccd81fc1adecc93a59dbb7926af69d93fc95d4943d40f6aad"
CORE_WORDS = 65_000
COST_SCALE = 12


def is_han(word):
    return all("\u3400" <= r <= "\u9fff" or "\uf900" <= r <= "\ufaff" for r in word)


def select_words(source, with_tags=False):
    data = source.read_bytes()
    if hashlib.sha256(data).hexdigest() != SOURCE_SHA256:
        raise ValueError("jieba source SHA-256 differs from the fixed manifest")
    entries, tags = {}, {}
    for line in data.decode("utf-8").splitlines():
        word, frequency, tag = line.split()
        if 2 <= len(word) <= 8 and is_han(word):
            if int(frequency) >= entries.get(word, 0):
                entries[word] = int(frequency)
                # Retain upstream lexical classes; these are not contextual
                # syntax predictions. vn is a nominalized verb, useful in
                # technical noun phrases such as 工作机制 and 临床治疗.
                tags[word] = (1 if tag.startswith('n') or tag == 'vn' else
                              2 if tag.startswith('a') or tag == 'b' else
                              3 if tag.startswith('v') else 4 if tag == 'd' else 0)
    selected = dict(sorted(entries.items(), key=lambda entry: (-entry[1], entry[0]))[:CORE_WORDS])
    # Domain phrases are reviewed text, not inferred runtime compounds. Common
    # verb complements and function words need protection even at low frequency.
    phrases = ROOT / "apps/desktop/internal/subtitle/dict/chinese_phrases.txt"
    for line in phrases.read_text("utf-8").splitlines():
        word = line.strip()
        if word and not word.startswith("#"):
            if not is_han(word) or not 2 <= len(word) <= 16:
                raise ValueError("Invalid reviewed Chinese phrase")
            selected[word] = max(selected.get(word, 0), 500)
    return (selected, tags) if with_tags else selected


def compile_trie(words, tags=None):
    # Build the ordinary byte trie only in this offline compiler. Runtime uses
    # base/check directly and checks terminal costs only at UTF-8 rune ends.
    children, costs, classes = [{}], [0], [0]
    total = sum(words.values())
    for word, frequency in sorted(words.items()):
        node = 0
        for byte in word.encode("utf-8"):
            label = byte + 1
            if label not in children[node]:
                children[node][label] = len(children)
                children.append({})
                costs.append(0)
                classes.append(0)
            node = children[node][label]
        costs[node] = max(1, min(254, round(-math.log(frequency / total) * COST_SCALE)))
        classes[node] = (tags or {}).get(word, 0)
    base, check = [0, 0], [0, 1]
    queue = deque([(0, 1)])
    next_check = 1
    used_bases = set()

    def reserve(end):
        if end >= len(check):
            grow = end + 257 - len(check)
            check.extend([0] * grow)
            base.extend([0] * grow)

    while queue:
        node, state = queue.popleft()
        labels = sorted(children[node])
        if not labels:
            base[state] = costs[node] << 24
            continue
        pos = max(labels[0] + 1, next_check) - 1
        search_start, occupied = pos + 1, 0
        first_free = True
        while True:
            pos += 1
            reserve(pos + labels[-1] - labels[0])
            if check[pos]:
                occupied += 1
                continue
            if first_free:
                next_check, first_free = pos, False
            offset = pos - labels[0]
            if offset not in used_bases and all(not check[offset + label] for label in labels):
                break
        if occupied / max(1, pos - search_start + 1) >= 0.95:
            next_check = pos
        if offset >= 1 << 24:
            raise ValueError("Trie exceeds the 24-bit transition range")
        used_bases.add(offset)
        base[state] = offset | (costs[node] << 24)
        for label in labels:
            child_state = offset + label
            check[child_state] = state | (classes[children[node][label]] << 24)
            queue.append((children[node][label], child_state))
    last = max(i for i, value in enumerate(check) if value) + 1
    payload = bytearray(last * 8)
    for state in range(last):
        struct.pack_into("<II", payload, state * 8, base[state], check[state])
    return bytes(payload), max(1, round(math.log(total) * COST_SCALE))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--output", type=Path, default=ROOT / "apps/desktop/internal/subtitle/dict/chinese.json")
    args = parser.parse_args()
    words, tags = select_words(args.source, with_tags=True)
    data, unknown_cost = compile_trie(words, tags)
    result = {
        "version": 2,
        "words": len(words),
        "states": len(data) // 8,
        "unknownCost": unknown_cost,
        "costScale": COST_SCALE,
        "sha256": hashlib.sha256(data).hexdigest(),
        "encoding": "base64+zlib; little-endian uint32 packed-base/cost, uint32 packed-parent/lexical-class",
        "data": base64.b64encode(zlib.compress(data, 9)).decode("ascii"),
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, ensure_ascii=False, separators=(",", ":")) + "\n", "utf-8", newline="\n")
    print(f"Compiled {len(words)} words, {len(data) // 8} states, {len(data)} runtime bytes")
    print(f"Embedded JSON: {args.output.stat().st_size} bytes; SHA-256: {hashlib.sha256(args.output.read_bytes()).hexdigest()}")


if __name__ == "__main__":
    main()
