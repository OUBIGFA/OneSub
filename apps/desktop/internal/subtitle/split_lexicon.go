package subtitle

import (
	"compress/zlib"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode"
)

//go:embed dict/chinese.json
var chineseLexiconJSON string

// chineseTrie stores a packed base/check DAT, not an object per dictionary
// character. Terminal unigram costs occupy base's high byte. Its immutable
// payload is loaded once; no CGo, Python, files or network are used at runtime.
type chineseTrie struct {
	data        []byte
	unknownCost int
}

var splitLexicon = sync.OnceValue(func() chineseTrie {
	var source struct {
		Version, States, UnknownCost int
		Data, SHA256                 string
	}
	if err := json.Unmarshal([]byte(chineseLexiconJSON), &source); err != nil {
		panic(fmt.Errorf("invalid embedded split lexicon: %w", err))
	}
	if source.Version != 2 || source.States < 2 || source.States > 1<<18 || source.UnknownCost <= 0 {
		panic("invalid embedded split lexicon header")
	}
	reader, err := zlib.NewReader(base64.NewDecoder(base64.StdEncoding, strings.NewReader(source.Data)))
	if err != nil {
		panic(fmt.Errorf("invalid embedded split lexicon compression: %w", err))
	}
	defer reader.Close()
	data := make([]byte, source.States*8)
	if _, err = io.ReadFull(reader, data); err != nil {
		panic(fmt.Errorf("incomplete embedded split lexicon: %w", err))
	}
	var extra [1]byte
	size, err := reader.Read(extra[:])
	digest := sha256.Sum256(data)
	if size != 0 || err != io.EOF || hex.EncodeToString(digest[:]) != source.SHA256 {
		panic("embedded split lexicon checksum mismatch")
	}
	// A corrupt compiled resource is a build error, never an excuse to
	// silently replace the requested language algorithm with hard cuts.
	return chineseTrie{data, source.UnknownCost}
})

func (t chineseTrie) advance(state int, b byte) int {
	base := binary.LittleEndian.Uint32(t.data[state*8:]) & 0xffffff
	next := int(base) + int(b) + 1
	if next >= len(t.data)/8 || binary.LittleEndian.Uint32(t.data[next*8+4:])&0xffffff != uint32(state) {
		return 0
	}
	return next
}

// The dictionary's lexical class is a phrase cue. A word can have different
// roles in context, so its class alone must not become a syntactic hard lock.
func (t chineseTrie) lexicalClass(word string) uint32 {
	state := 1
	for _, b := range []byte(word) {
		state = t.advance(state, b)
		if state == 0 {
			return 0
		}
	}
	return binary.LittleEndian.Uint32(t.data[state*8+4:]) >> 24
}

func (t chineseTrie) cost(state int) int {
	return int(binary.LittleEndian.Uint32(t.data[state*8:]) >> 24)
}

// Chinese segmentation is a minimum unigram-cost DAG, not blind longest
// matching or the union of overlapping dictionary hits. "研究生命" must not
// be locked into "研究生/命" merely because 研究生 also exists in the lexicon.
func (t chineseTrie) segment(text string, runes []rune, bytes, score, next []int, blocked []bool, wordEnds []int) {
	n := len(runes)
	score[n] = 0
	for i := n - 1; i >= 0; i-- {
		next[i], score[i] = i+1, score[i+1]
		if !unicode.Is(unicode.Han, runes[i]) {
			continue
		}
		score[i] += t.unknownCost
		state := 1
		for j := i; j < n && unicode.Is(unicode.Han, runes[j]); j++ {
			for _, b := range []byte(text[bytes[j]:bytes[j+1]]) {
				state = t.advance(state, b)
				if state == 0 {
					break
				}
			}
			if state == 0 {
				break
			}
			if cost := t.cost(state); cost > 0 && cost+score[j+1] <= score[i] {
				score[i], next[i] = cost+score[j+1], j+1
			}
		}
	}
	for i := 0; i < n; {
		if !unicode.Is(unicode.Han, runes[i]) {
			i++
			continue
		}
		end := next[i]
		for j := i + 1; j < end; j++ {
			blocked[j] = true
		}
		wordEnds[end]++
		i = end
	}
}
