package translator

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Answers are read the way SmartSub's aiResponseParser reads them: reasoning
// that a model writes into its answer is dropped, then the JSON is looked
// for in a fenced block, in the first balanced object and in the whole
// text. A strict decode comes first; a tolerant scan reads what a strict
// decoder rejects (trailing commas, raw line breaks in strings, an answer
// cut before its closing brace), so one stray character never costs a batch.
var (
	thinkBlock = regexp.MustCompile(`(?is)<think>.*?</think>`)
	// An unclosed <think> runs up to the answer's first fence or object.
	openThink  = regexp.MustCompile("(?is)<think>.*?(```|\\{)")
	fenceBlock = regexp.MustCompile("(?s)```[a-zA-Z]*[ \t]*\r?\n?(.*?)```")
)

// errNoObject says an answer held no JSON object of subtitle texts.
var errNoObject = errors.New("no JSON object")

// stripThinking removes reasoning a model put into the answer itself.
func stripThinking(text string) string {
	text = thinkBlock.ReplaceAllString(text, "")
	if loc := openThink.FindStringSubmatchIndex(text); loc != nil {
		text = text[:loc[0]] + text[loc[2]:]
	} else if at := strings.Index(strings.ToLower(text), "<think>"); at >= 0 {
		text = text[:at]
	}
	return strings.TrimSpace(text)
}

// parseObject reads an answer's object of IDs and texts. Values may also be
// objects that carry the text in a "tr", "translation", "text" or "target"
// field, as models sometimes answer.
func parseObject(answer string) (map[string]string, error) {
	text := stripThinking(answer)
	candidates := []string{}
	for _, m := range fenceBlock.FindAllStringSubmatch(text, -1) {
		candidates = append(candidates, m[1])
	}
	if object := firstObject(text); object != "" {
		candidates = append(candidates, object)
	}
	candidates = append(candidates, text)
	for _, candidate := range candidates {
		if values, ok := decodeObject(candidate); ok {
			return values, nil
		}
	}
	for _, candidate := range candidates {
		if pairs, _ := scanPairs(candidate); len(pairs) > 0 {
			values := map[string]string{}
			for _, p := range pairs {
				values[p.key] = p.value
			}
			return values, nil
		}
	}
	return nil, errNoObject
}

// decodeObject is the strict reading of one candidate.
func decodeObject(candidate string) (map[string]string, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(candidate)), &raw); err != nil || len(raw) == 0 {
		return nil, false
	}
	values := map[string]string{}
	for key, value := range raw {
		text, ok := valueText(value)
		if !ok {
			return nil, false
		}
		values[key] = text
	}
	return values, true
}

func valueText(raw json.RawMessage) (string, bool) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	switch v := value.(type) {
	case string:
		return v, true
	case nil:
		return "", true
	case float64, bool:
		return strings.TrimSpace(string(raw)), true
	case map[string]any:
		for _, field := range []string{"tr", "translation", "text", "target"} {
			if s, ok := v[field].(string); ok {
				return s, true
			}
		}
	}
	return "", false
}

// firstObject is the first balanced {...} of text, or "".
func firstObject(text string) string {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return ""
	}
	depth, quoted, escaped := 0, false, false
	for i := start; i < len(text); i++ {
		c := text[i]
		switch {
		case quoted && escaped:
			escaped = false
		case quoted && c == '\\':
			escaped = true
		case c == '"':
			quoted = !quoted
		case !quoted && c == '{':
			depth++
		case !quoted && c == '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

type pair struct{ key, value string }

// scanPairs reads "key": "value" pairs of a flat JSON object as far as the
// text goes. It accepts what streaming and loose answers contain: no closing
// brace yet, trailing commas, raw line breaks inside strings. The last
// return is the value being written when the text ends inside one, so a live
// view can show a subtitle as the model writes it.
func scanPairs(text string) ([]pair, *pair) {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return nil, nil
	}
	pairs := []pair{}
	i := start + 1
	for {
		i = skipSpace(text, i, ",")
		if i >= len(text) || text[i] == '}' {
			return pairs, nil
		}
		if text[i] != '"' {
			return pairs, nil
		}
		key, next, closed := readString(text, i+1)
		if !closed {
			return pairs, nil
		}
		i = skipSpace(text, next, "")
		if i >= len(text) || text[i] != ':' {
			return pairs, nil
		}
		i = skipSpace(text, i+1, "")
		if i >= len(text) {
			return pairs, &pair{key, ""}
		}
		switch text[i] {
		case '"':
			value, next, closed := readString(text, i+1)
			if !closed {
				return pairs, &pair{key, value}
			}
			pairs = append(pairs, pair{key, value})
			i = next
		case '{':
			// {"src": …, "tr": …}: the nested text field is the value.
			end := strings.IndexByte(text[i:], '}')
			if end < 0 {
				return pairs, nil
			}
			if value, ok := decodeObject("{\"v\":" + text[i:i+end+1] + "}"); ok {
				pairs = append(pairs, pair{key, value["v"]})
			}
			i += end + 1
		default:
			end := i
			for end < len(text) && !strings.ContainsRune(",}\n", rune(text[end])) {
				end++
			}
			pairs = append(pairs, pair{key, strings.TrimSpace(text[i:end])})
			i = end
		}
	}
}

func skipSpace(text string, i int, extra string) int {
	for i < len(text) && (strings.IndexByte(" \t\r\n", text[i]) >= 0 || strings.IndexByte(extra, text[i]) >= 0) {
		i++
	}
	return i
}

// readString decodes a JSON string body that starts at i, after its opening
// quote. It returns the text, the index after the closing quote and whether
// the string closed. An escape cut by the end of the text is left out.
func readString(text string, i int) (string, int, bool) {
	var b strings.Builder
	for i < len(text) {
		c := text[i]
		switch {
		case c == '"':
			return b.String(), i + 1, true
		case c == '\\':
			if i+1 >= len(text) {
				return b.String(), len(text), false
			}
			switch e := text[i+1]; e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b', 'f':
			case 'u':
				if i+6 > len(text) {
					return b.String(), len(text), false
				}
				code, err := strconv.ParseUint(text[i+2:i+6], 16, 32)
				if err != nil {
					return b.String(), len(text), false
				}
				r := rune(code)
				if utf16.IsSurrogate(r) && i+12 <= len(text) && text[i+6] == '\\' && text[i+7] == 'u' {
					if low, err := strconv.ParseUint(text[i+8:i+12], 16, 32); err == nil {
						r = utf16.DecodeRune(r, rune(low))
						i += 6
					}
				}
				b.WriteRune(r)
				i += 6
				continue
			default:
				b.WriteByte(e)
			}
			i += 2
		default:
			r, size := utf8.DecodeRuneInString(text[i:])
			b.WriteRune(r)
			i += size
		}
	}
	return b.String(), i, false
}

// orderedObject writes keys and values as one JSON object in the given
// order; encoding/json would sort "10" before "2".
func orderedObject(keys []string, values map[string]string) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(key)
		v, _ := json.Marshal(values[key])
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.String()
}
