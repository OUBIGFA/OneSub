using System.Globalization;
using System.Text;
using System.Text.RegularExpressions;
using Nikse.SubtitleEdit.Core.Common;
using Nikse.SubtitleEdit.Core.SubtitleFormats;

// The original text is retained independently of libse's normalized view. Strict
// writes patch only approved fields, never regenerate an ASS layout or VTT header.
static class Codec
{
    static SubtitleFormat Format(string format) => format switch
    {
        "srt" => new SubRip(), "vtt" => new WebVTT(),
        "ass" => new AdvancedSubStationAlpha(), "ssa" => new SubStationAlpha(),
        _ => throw new InvalidDataException("不支持的字幕格式")
    };

    public static Document Read(string path)
    {
        using var input = new StreamReader(path, new UTF8Encoding(false, true), true);
        return Parse(input.ReadToEnd(), Path.GetExtension(path).TrimStart('.').ToLowerInvariant());
    }

    static Document Parse(string original, string format)
    {
        var parser = Format(format);
        var source = new Subtitle();
        parser.LoadSubtitle(source, Regex.Split(original, "\r\n|\n|\r").ToList(), "source." + format);
        var paragraphs = source.Paragraphs.Where(p => !p.IsComment).ToList();
        if (paragraphs.Count == 0 || parser.ErrorCount > 0)
            throw new InvalidDataException($"字幕解析失败（{parser.ErrorCount} 项格式错误），未导入部分结果");
        var cues = paragraphs.Select((p, index) => new Cue {
            Id = "c" + (index + 1), Position = index,
            StartMs = checked((long)Math.Round(p.StartTime.TotalMilliseconds)),
            EndMs = checked((long)Math.Round(p.EndTime.TotalMilliseconds)),
            Text = p.Text.Replace("\r\n", "\n"), OriginalText = p.Text.Replace("\r\n", "\n")
        }).ToList();
        if (format is "ass" or "ssa")
        {
            var events = AssEvents(original);
            if (events.Count != cues.Count) throw new InvalidDataException("ASS 事件映射不完整");
            for (int i = 0; i < events.Count; i++)
            {
                cues[i].Text = cues[i].OriginalText = events[i].Text;
                cues[i].ReadOnly = Regex.IsMatch(events[i].Text, @"\{[^}]*\\p[1-9]");
            }
        }
        if (format == "vtt")
        {
            var blocks = VttCues(original);
            if (blocks.Count != cues.Count) throw new InvalidDataException("VTT 字幕映射不完整");
            for (int i = 0; i < blocks.Count; i++)
                cues[i].Text = cues[i].OriginalText = blocks[i].Groups["text"].Value.Replace("\r\n", "\n");
        }
        return new Document { Format = format, Original = original, Cues = cues };
    }

    public static string Write(Document document)
    {
        var target = string.IsNullOrEmpty(document.TargetFormat) ? document.Format : document.TargetFormat;
        foreach (var cue in document.Cues)
        {
            if (cue.StartMs < 0 || cue.EndMs <= cue.StartMs || cue.EndMs > 9007199254740991L)
                throw new InvalidDataException("存在非法时间；请先修复再导出");
            if (string.IsNullOrWhiteSpace(cue.Text)) throw new InvalidDataException("存在空字幕；请先修复再导出");
        }
        if (document.Cues.Count == 0) throw new InvalidDataException("没有可导出的字幕");
        if (target == document.Format && target is "ass" or "ssa" or "vtt")
        {
            var baseline = Parse(document.Original, document.Format);
            if (baseline.Cues.Count != document.Cues.Count ||
                baseline.Cues.Where((c, i) => c.Id != document.Cues[i].Id).Any())
                throw new InvalidDataException("保留格式导出不能改变事件数量或顺序；请另存 SRT");
            if (target == "vtt") return WriteVtt(document, baseline);
            var lines = Regex.Split(document.Original, "(?<=\n)");
            var events = AssEvents(document.Original);
            for (int i = 0; i < events.Count; i++)
            {
                var before = baseline.Cues[i]; var after = document.Cues[i];
                if (after.StartMs != before.StartMs || after.EndMs != before.EndMs)
                    throw new InvalidDataException("保留排版模式不能改变 ASS 时间");
                if (before.ReadOnly && after.Text != before.Text)
                    throw new InvalidDataException("ASS 绘图事件受保护");
                if (!Tokens(before.Text).SequenceEqual(Tokens(after.Text)) || after.Text.Contains('\n') || after.Text.Contains('\r'))
                    throw new InvalidDataException("ASS 标签与原有换行受保护");
                var item = events[i];
                lines[item.Line] = item.Prefix + after.Text + item.Newline;
            }
            return string.Concat(lines);
        }
        if (target is "ass" or "ssa") throw new InvalidDataException("新 ASS 排版需先生成样式文档");
        var normalized = new Subtitle();
        foreach (var cue in document.Cues)
            normalized.Paragraphs.Add(new Paragraph(cue.Text, cue.StartMs, cue.EndMs));
        // Paragraph(text, start, end) leaves Number at zero. SubRip.ToText uses
        // that field rather than the collection position; number once here.
        normalized.Renumber();
        return Format(target).ToText(normalized, "OneSub").Replace("\r\n", "\n") + "\n";
    }

    static IEnumerable<string> Tokens(string text) => Regex.Matches(text, @"\{[^}]*\}|\\[Nnh]").Select(m => m.Value);

    static string Stamp(long ms) => $"{ms / 3600000:00}:{ms / 60000 % 60:00}:{ms / 1000 % 60:00}.{ms % 1000:000}";

    // Anchor on timing lines, keeping cue identifiers, NOTE/STYLE/REGION and settings
    // outside the patch. The trailing blank line belongs to the original document.
    static List<Match> VttCues(string text) => Regex.Matches(text,
        @"(?m)^(?<start>(?:\d+:)?\d{2}:\d{2}\.\d{3})(?<arrow>[ \t]+-->[ \t]+)(?<end>(?:\d+:)?\d{2}:\d{2}\.\d{3})(?<settings>[^\r\n]*)(?<nl>\r?\n)(?<text>[^\r\n]+(?:\r?\n(?!\r?$)[^\r\n]+)*)")
        .Cast<Match>().ToList();

    static string WriteVtt(Document document, Document baseline)
    {
        var output = document.Original;
        var blocks = VttCues(output);
        for (int i = blocks.Count - 1; i >= 0; i--)
        {
            var b = blocks[i]; var before = baseline.Cues[i]; var cue = document.Cues[i];
            if (cue.Text.Contains("\n\n")) throw new InvalidDataException("VTT 字幕内不能含空段");
            var start = cue.StartMs == before.StartMs ? b.Groups["start"].Value : Stamp(cue.StartMs);
            var end = cue.EndMs == before.EndMs ? b.Groups["end"].Value : Stamp(cue.EndMs);
            var replacement = start + b.Groups["arrow"].Value + end + b.Groups["settings"].Value +
                b.Groups["nl"].Value + cue.Text.Replace("\r\n", "\n").Replace("\n", b.Groups["nl"].Value);
            output = output[..b.Index] + replacement + output[(b.Index + b.Length)..];
        }
        return output;
    }

    record AssLine(int Line, string Prefix, string Text, string Newline);
    static List<AssLine> AssEvents(string original)
    {
        var result = new List<AssLine>();
        var lines = Regex.Split(original, "(?<=\n)");
        bool events = false; int fields = 0;
        for (int i = 0; i < lines.Length; i++)
        {
            var body = lines[i].TrimEnd('\r', '\n');
            var trimmed = body.Trim();
            if (trimmed.StartsWith('[')) events = trimmed.Equals("[Events]", StringComparison.OrdinalIgnoreCase);
            if (!events) continue;
            if (trimmed.StartsWith("Format:", StringComparison.OrdinalIgnoreCase))
            {
                var names = trimmed[7..].Split(',').Select(s => s.Trim()).ToArray();
                if (!names[^1].Equals("Text", StringComparison.OrdinalIgnoreCase))
                    throw new InvalidDataException("ASS Text 字段必须位于事件末尾");
                fields = names.Length;
            }
            if (!trimmed.StartsWith("Dialogue:", StringComparison.OrdinalIgnoreCase)) continue;
            if (fields < 3) throw new InvalidDataException("ASS 缺少事件 Format 声明");
            int at = body.IndexOf(':') + 1;
            for (int field = 0; field < fields - 1; field++)
            {
                at = body.IndexOf(',', at) + 1;
                if (at == 0) throw new InvalidDataException("ASS 事件字段不足");
            }
            result.Add(new AssLine(i, body[..at], body[at..], lines[i][body.Length..]));
        }
        return result;
    }
}

sealed class Document
{
    public string Format { get; set; } = "srt";
    public string TargetFormat { get; set; } = "";
    public string Original { get; set; } = "";
    public List<Cue> Cues { get; set; } = [];
}

sealed class Cue
{
    public string Id { get; set; } = "";
    public int Position { get; set; }
    public long StartMs { get; set; }
    public long EndMs { get; set; }
    public string Text { get; set; } = "";
    public string OriginalText { get; set; } = "";
    public bool ReadOnly { get; set; }
}
