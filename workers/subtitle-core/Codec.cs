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
        _ => throw new InvalidDataException("E_FORMAT")
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
            throw new InvalidDataException($"E_PARSE:{parser.ErrorCount}");
        var cues = paragraphs.Select((p, index) => new Cue {
            Id = "c" + (index + 1), Position = index,
            StartMs = checked((long)Math.Round(p.StartTime.TotalMilliseconds)),
            EndMs = checked((long)Math.Round(p.EndTime.TotalMilliseconds)),
            Text = p.Text.Replace("\r\n", "\n"), OriginalText = p.Text.Replace("\r\n", "\n")
        }).ToList();
        if (format is "ass" or "ssa")
        {
            var events = AssEvents(original);
            if (events.Count != cues.Count) throw new InvalidDataException("E_ASS_MAPPING");
            for (int i = 0; i < events.Count; i++)
            {
                cues[i].Text = cues[i].OriginalText = events[i].Text;
                cues[i].ReadOnly = Regex.IsMatch(events[i].Text, @"\{[^}]*\\p[1-9]");
            }
        }
        if (format == "vtt")
        {
            var blocks = VttCues(original);
            if (blocks.Count != cues.Count) throw new InvalidDataException("E_VTT_MAPPING");
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
                throw new InvalidDataException("E_TIMING");
            if (string.IsNullOrWhiteSpace(cue.Text)) throw new InvalidDataException("E_EMPTY_CUE");
        }
        if (document.Cues.Count == 0 && target == "srt") return "";
        if (target == document.Format && target is "ass" or "ssa" or "vtt")
        {
            var baseline = Parse(document.Original, document.Format);
            if (baseline.Cues.Count != document.Cues.Count ||
                baseline.Cues.Where((c, i) => c.Id != document.Cues[i].Id).Any())
                return WriteStructure(document, baseline);
            if (target == "vtt") return WriteVtt(document, baseline);
            var lines = Regex.Split(document.Original, "(?<=\n)");
            var events = AssEvents(document.Original);
            for (int i = 0; i < events.Count; i++)
            {
                var before = baseline.Cues[i]; var after = document.Cues[i];
                if (after.StartMs != before.StartMs || after.EndMs != before.EndMs)
                    throw new InvalidDataException("E_ASS_TIMING");
                if (before.ReadOnly && after.Text != before.Text)
                    throw new InvalidDataException("E_ASS_DRAWING");
                if (!Tokens(before.Text).SequenceEqual(Tokens(after.Text)) || after.Text.Contains('\n') || after.Text.Contains('\r'))
                    throw new InvalidDataException("E_ASS_TAGS");
                var item = events[i];
                lines[item.Line] = item.Prefix + after.Text + item.Newline;
            }
            return string.Concat(lines);
        }
        if (target is "ass" or "ssa") throw new InvalidDataException("E_ASS_NEEDS_STYLE");
        var normalized = new Subtitle();
        foreach (var cue in document.Cues)
            normalized.Paragraphs.Add(new Paragraph(cue.Text, cue.StartMs, cue.EndMs));
        // Paragraph(text, start, end) leaves Number at zero. SubRip.ToText uses
        // that field rather than the collection position; number once here.
        normalized.Renumber();
        return Format(target).ToText(normalized, "OneSub").Replace("\r\n", "\n") + "\n";
    }

    // Exporting one displayed line is an explicit projection, separate from
    // strict text editing. Validate edits first, then preserve the serialized
    // file's timings, styles, comments and cue metadata while selecting text.
    public static Document SelectLines(LineSelection selection)
    {
        if (selection.Line is not ("upper" or "lower"))
            throw new InvalidDataException("E_LINE_SELECTION");
        var input = selection.Document;
        var target = input.TargetFormat;
        input.TargetFormat = input.Format;
        var document = Parse(Write(input), input.Format);
        var ass = document.Format is "ass" or "ssa";
        foreach (var cue in document.Cues.Where(c => !c.ReadOnly))
        {
            cue.Text = SelectLine(cue.Text, selection.Line == "lower", ass);
            if (string.IsNullOrWhiteSpace(HtmlUtil.RemoveHtmlTags(cue.Text, true)))
                throw new InvalidDataException("E_EMPTY_LINE");
        }
        if (ass)
        {
            var lines = Regex.Split(document.Original, "(?<=\\n)");
            var events = AssEvents(document.Original);
            for (int i = 0; i < events.Count; i++)
                lines[events[i].Line] = events[i].Prefix + document.Cues[i].Text + events[i].Newline;
            document.Original = string.Concat(lines);
        }
        else
        {
            document.Original = Write(document);
        }
        // The projected original is the strict baseline for subsequent format
        // conversion; extraction never disables ASS protection in normal writes.
        var result = Parse(document.Original, document.Format);
        result.TargetFormat = target;
        return result;
    }

    static string SelectLine(string text, bool lower, bool ass)
    {
        // Retain inline tags from both sides of the cut so open/close tags and
        // inherited ASS formatting remain balanced. Only visible text and line
        // separators are selected; a cue with one line stays intact.
        var tokens = Regex.Matches(text, ass
            ? @"\{[^}]*\}|\\[Nn]|\r\n|\r|\n"
            : @"<[^>\r\n]+>|\r\n|\r|\n").Cast<Match>().ToList();
        bool Tag(Match token) => token.Value[0] is '{' or '<';
        int lastLine = tokens.Count(t => !Tag(t));
        if (lastLine == 0) return text;
        int selected = lower ? lastLine : 0;
        var output = new StringBuilder();
        int line = 0, position = 0;
        foreach (var token in tokens)
        {
            if (line == selected) output.Append(text, position, token.Index - position);
            if (Tag(token)) output.Append(token.Value);
            else line++;
            position = token.Index + token.Length;
        }
        if (line == selected) output.Append(text, position, text.Length - position);
        return output.ToString();
    }

    static IEnumerable<string> Tokens(string text) => Regex.Matches(text, @"\{[^}]*\}|\\[Nnh]").Select(m => m.Value);

    // Structural edits splice only dialogue/cue blocks into the preserved file.
    // Existing text edits still pass through the strict writer, so insertion and
    // deletion never bypass ASS tag/drawing protection or discard VTT metadata.
    static string WriteStructure(Document document, Document baseline)
    {
        var known = baseline.Cues.Select((cue, index) => (cue.Id, index)).ToDictionary(x => x.Id, x => x.index);
        if (document.Cues.Select(c => c.Id).Distinct().Count() != document.Cues.Count)
            throw new InvalidDataException("E_PRESERVE_ORDER");
        var retained = document.Cues.Where(c => known.ContainsKey(c.Id)).ToDictionary(c => c.Id);
        var order = document.Cues.Where(c => known.ContainsKey(c.Id)).Select(c => known[c.Id]).ToArray();
        if (!order.SequenceEqual(order.OrderBy(i => i))) throw new InvalidDataException("E_PRESERVE_ORDER");
        if (baseline.Cues.Any(c => c.ReadOnly && !retained.ContainsKey(c.Id)))
            throw new InvalidDataException("E_ASS_DRAWING");
        var aligned = new Document { Format = document.Format, Original = document.Original,
            Cues = baseline.Cues.Select(c => retained.GetValueOrDefault(c.Id) ?? c).ToList() };
        var output = Write(aligned);
        var newline = output.Contains("\r\n") ? "\r\n" : "\n";
        var spans = new List<(int Start, int Length)>();
        var ass = document.Format is "ass" or "ssa";
        var events = ass ? AssEvents(output) : [];
        if (ass)
        {
            var lines = Regex.Split(output, "(?<=\n)");
            var offsets = new int[lines.Length];
            for (int i = 1; i < lines.Length; i++) offsets[i] = offsets[i - 1] + lines[i - 1].Length;
            spans.AddRange(events.Select(e => (offsets[e.Line], lines[e.Line].Length)));
        }
        else
        {
            foreach (var block in VttCues(output))
            {
                var start = block.Index;
                if (start > 1)
                {
                    var previous = output.LastIndexOf('\n', start - 2) + 1;
                    var identifier = output[previous..start].TrimEnd('\r', '\n');
                    if (identifier.Length > 0 && !identifier.StartsWith("WEBVTT")) start = previous;
                }
                spans.Add((start, block.Index + block.Length - start));
            }
        }
        var inserts = new Dictionary<int, StringBuilder>();
        int prior = -1;
        for (int at = 0; at < document.Cues.Count; at++)
        {
            var cue = document.Cues[at];
            if (known.TryGetValue(cue.Id, out var sourceIndex)) { prior = sourceIndex; continue; }
            int next = baseline.Cues.Count;
            for (int n = at + 1; n < document.Cues.Count; n++)
                if (known.TryGetValue(document.Cues[n].Id, out next)) break;
                else next = baseline.Cues.Count;
            if (!inserts.TryGetValue(next, out var text)) inserts[next] = text = new StringBuilder();
            if (ass)
            {
                if (cue.StartMs % 10 != 0 || cue.EndMs % 10 != 0) throw new InvalidDataException("E_ASS_TIMING");
                var template = events[prior >= 0 ? prior : Math.Min(next, events.Count - 1)];
                var colon = template.Prefix.IndexOf(':');
                var fields = template.Prefix[(colon + 1)..].Split(',');
                fields[template.StartField] = AssStamp(cue.StartMs);
                fields[template.EndField] = AssStamp(cue.EndMs);
                text.Append(template.Prefix[..(colon + 1)]).Append(string.Join(",", fields))
                    .Append(cue.Text.Replace("\r\n", "\n").Replace("\r", "\n").Replace("\n", "\\N")).Append(newline);
            }
            else
            {
                if (cue.Text.Contains("\n\n")) throw new InvalidDataException("E_VTT_BLANK");
                text.Append(newline).Append(Stamp(cue.StartMs)).Append(" --> ").Append(Stamp(cue.EndMs))
                    .Append(newline).Append(cue.Text.Replace("\r\n", "\n").Replace("\n", newline)).Append(newline).Append(newline);
            }
        }
        for (int i = spans.Count - 1; i >= 0; i--)
        {
            var span = spans[i];
            var body = retained.ContainsKey(baseline.Cues[i].Id) ? output.Substring(span.Start, span.Length) : "";
            var inserted = inserts.TryGetValue(i, out var text) ? text.ToString() : "";
            output = output[..span.Start] + inserted + body + output[(span.Start + span.Length)..];
        }
        if (inserts.TryGetValue(baseline.Cues.Count, out var tail))
            output += (output.EndsWith('\n') ? "" : newline) + tail;
        return output;
    }

    static string AssStamp(long ms) => $"{ms / 3600000}:{ms / 60000 % 60:00}:{ms / 1000 % 60:00}.{ms / 10 % 100:00}";

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
            if (cue.Text.Contains("\n\n")) throw new InvalidDataException("E_VTT_BLANK");
            var start = cue.StartMs == before.StartMs ? b.Groups["start"].Value : Stamp(cue.StartMs);
            var end = cue.EndMs == before.EndMs ? b.Groups["end"].Value : Stamp(cue.EndMs);
            var replacement = start + b.Groups["arrow"].Value + end + b.Groups["settings"].Value +
                b.Groups["nl"].Value + cue.Text.Replace("\r\n", "\n").Replace("\n", b.Groups["nl"].Value);
            output = output[..b.Index] + replacement + output[(b.Index + b.Length)..];
        }
        return output;
    }

    record AssLine(int Line, string Prefix, string Text, string Newline, int StartField, int EndField);
    static List<AssLine> AssEvents(string original)
    {
        var result = new List<AssLine>();
        var lines = Regex.Split(original, "(?<=\n)");
        bool events = false; int fields = 0, startField = -1, endField = -1;
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
                    throw new InvalidDataException("E_ASS_FORMAT");
                fields = names.Length;
                startField = Array.FindIndex(names, n => n.Equals("Start", StringComparison.OrdinalIgnoreCase));
                endField = Array.FindIndex(names, n => n.Equals("End", StringComparison.OrdinalIgnoreCase));
                if (startField < 0 || endField < 0) throw new InvalidDataException("E_ASS_FORMAT");
            }
            if (!trimmed.StartsWith("Dialogue:", StringComparison.OrdinalIgnoreCase)) continue;
            if (fields < 3) throw new InvalidDataException("E_ASS_FORMAT");
            int at = body.IndexOf(':') + 1;
            for (int field = 0; field < fields - 1; field++)
            {
                at = body.IndexOf(',', at) + 1;
                if (at == 0) throw new InvalidDataException("E_ASS_FORMAT");
            }
            result.Add(new AssLine(i, body[..at], body[at..], lines[i][body.Length..], startField, endField));
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
