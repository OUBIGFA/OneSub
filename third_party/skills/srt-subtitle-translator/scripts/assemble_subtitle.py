#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Assemble validated UTF-8 subtitle parts without losing format metadata.

    python assemble_subtitle.py output.srt part1.srt part2.srt

SRT indices are rebuilt sequentially. VTT/ASS start with a complete document;
later parts may contain just cues/events or a document with matching metadata.
Existing outputs require --overwrite. An input part can never be the output.
"""

import argparse
import re
import sys
from pathlib import Path

from check_subtitle import EXT_FORMATS, PARSERS, visible_text


def read_part(path):
    """Accept UTF-8 (with optional BOM) and normalize only line endings."""
    try:
        text = Path(path).read_text(encoding="utf-8-sig")
    except UnicodeDecodeError as exc:
        raise ValueError("%s must be UTF-8" % path) from exc
    if not text.strip():
        raise ValueError("empty subtitle part: %s" % path)
    return text.replace("\r\n", "\n").replace("\r", "\n")


def validate_text(text, fmt):
    """Use the checker's parser so assembly cannot silently discard bad blocks."""
    blocks, errors = PARSERS[fmt](text)
    if not blocks:
        errors.append("no subtitle blocks found")
    for index, block in enumerate(blocks, 1):
        if block["end"] <= block["start"]:
            errors.append("block #%d has non-positive duration" % index)
        if not visible_text(" ".join(block["lines"])).strip():
            errors.append("block #%d has empty subtitle text" % index)
        if fmt != "ass" and index > 1 and block["start"] < blocks[index - 2]["end"]:
            errors.append("block #%d overlaps or precedes the previous block" % index)
    if errors:
        raise ValueError("invalid %s subtitle: %s" % (fmt, "; ".join(errors)))
    return blocks


def assemble_srt(parts):
    """Part-local numbering is allowed; timing, text and markup remain unchanged."""
    blocks = [block for part in parts for block in validate_text(part, "srt")]
    return "\n\n".join(
        "%d\n%s\n%s" % (index, block["time_line"], "\n".join(block["lines"]))
        for index, block in enumerate(blocks, 1)
    ) + "\n\n"


def assemble_vtt(parts):
    """Keep one matching header and all body blocks, including NOTE/STYLE/REGION."""
    header, bodies = None, []
    for index, part in enumerate(parts, 1):
        chunks = re.split(r"\n\s*\n", part.strip())
        if chunks[0].startswith("WEBVTT"):
            candidate = chunks.pop(0)
            if header is not None and candidate != header:
                raise ValueError("VTT header metadata conflicts in part %d" % index)
            header = candidate
        if header is None:
            raise ValueError("the first VTT part must contain a WEBVTT header")
        # Final parsing rejects misplaced STYLE/REGION blocks rather than dropping
        # or relocating them behind the user's back.
        bodies.extend(chunks)
    return header + "\n\n" + "\n\n".join(bodies) + "\n"


def split_ass_document(text):
    """Separate the Events section from both earlier and later document sections."""
    lines = text.strip().splitlines()
    starts = [index for index, line in enumerate(lines) if line.strip().lower() == "[events]"]
    if len(starts) != 1:
        raise ValueError("ASS document must contain exactly one [Events] section")
    start = starts[0]
    end = next((index for index in range(start + 1, len(lines))
                if lines[index].strip().startswith("[")), len(lines))
    return lines[:start + 1], lines[start + 1:end], lines[end:]


def ass_event_format(lines):
    formats = [line for line in lines if line.strip().lower().startswith("format:")]
    if len(formats) != 1:
        raise ValueError("ASS Events section must contain one Format line")
    return formats[0].strip()


def ass_part_events(lines, expected_format, keep_format=False):
    """Reject unknown input instead of silently filtering potentially useful data."""
    events = []
    for line in lines:
        stripped = line.strip()
        if stripped.lower().startswith("format:"):
            if stripped != expected_format:
                raise ValueError("ASS event Format differs between parts")
            if keep_format:
                events.append(line)
        elif (not stripped or stripped.startswith(";")
              or re.match(r"^(Dialogue|Comment|Picture|Sound|Movie|Command):", stripped, re.I)):
            events.append(line)
        else:
            raise ValueError("unrecognized content in ASS event part: %s" % stripped[:80])
    return events


def assemble_ass(parts):
    """Keep first-document metadata and insert later events before trailing sections."""
    head, first_events, tail = split_ass_document(parts[0])
    event_format = ass_event_format(first_events)
    events = ass_part_events(first_events, event_format, keep_format=True)
    for part in parts[1:]:
        lines = part.strip().splitlines()
        if any(line.strip().startswith("[") for line in lines):
            part_head, lines, part_tail = split_ass_document(part)
            if part_head != head or part_tail != tail:
                raise ValueError("ASS document metadata differs between parts")
            if ass_event_format(lines) != event_format:
                raise ValueError("ASS event Format differs between parts")
        events.extend(ass_part_events(lines, event_format))
    return "\n".join(head + events + tail) + "\n"


def assemble(output_path, part_paths, overwrite=False):
    """Build and validate in memory before touching any output file."""
    output = Path(output_path)
    fmt = EXT_FORMATS.get(output.suffix.lower())
    if fmt is None:
        raise ValueError("output extension must be .srt, .vtt, .ass, or .ssa")
    paths = [Path(path) for path in part_paths]
    if not paths:
        raise ValueError("at least one part is required")
    resolved = [path.resolve() for path in paths]
    if len(set(resolved)) != len(resolved):
        raise ValueError("the same subtitle part was supplied more than once")
    for path in paths:
        if output.resolve() == path.resolve() or (output.exists() and output.samefile(path)):
            raise ValueError("output must not replace an input part: %s" % path)
        part_fmt = EXT_FORMATS.get(path.suffix.lower())
        if part_fmt is not None and part_fmt != fmt:
            raise ValueError("input and output subtitle formats must match: %s" % path)
    if output.exists() and not overwrite:
        raise ValueError("output already exists; choose a new path or explicitly use --overwrite")

    parts = [read_part(path) for path in paths]
    result = {"srt": assemble_srt, "vtt": assemble_vtt, "ass": assemble_ass}[fmt](parts)
    validate_text(result, fmt)
    # Exclusive creation also protects against an output appearing after the check.
    with output.open("w" if overwrite else "x", encoding="utf-8", newline="\n") as handle:
        handle.write(result)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", help="assembled UTF-8 output path")
    parser.add_argument("parts", nargs="+", help="ordered UTF-8 subtitle parts")
    parser.add_argument("--overwrite", action="store_true", help="explicitly replace an existing output, never an input")
    args = parser.parse_args()
    try:
        assemble(args.output, args.parts, overwrite=args.overwrite)
    except (OSError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
