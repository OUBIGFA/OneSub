// Copyright 2021 Google LLC
// Licensed under the Apache License, Version 2.0. See third_party/licenses/budoux.txt.
// OneSub modification: port only BudouX Parser.parse's plain-text scoring to Go;
// expose byte/rune boundaries, retain immutable models, omit HTML processing.
package subtitle

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed dict/budoux-ja.json
var japaneseModelJSON string

//go:embed dict/budoux-th.json
var thaiModelJSON string

//go:embed dict/budoux-zh-hans.json
var simplifiedChineseModelJSON string

//go:embed dict/budoux-zh-hant.json
var traditionalChineseModelJSON string

var budouxFeatures = [...]struct {
	name       string
	start, end int
}{
	{"UW1", -3, -2}, {"UW2", -2, -1}, {"UW3", -1, 0},
	{"UW4", 0, 1}, {"UW5", 1, 2}, {"UW6", 2, 3},
	{"BW1", -2, 0}, {"BW2", -1, 1}, {"BW3", 0, 2},
	{"TW1", -3, 0}, {"TW2", -2, 1}, {"TW3", -1, 2}, {"TW4", 0, 3},
}

type phraseModel struct {
	groups [len(budouxFeatures)]map[string]int
	base   int // twice the upstream base score, preserving half-integer values
}

func loadPhraseModel(data string) phraseModel {
	var groups map[string]map[string]int
	if err := json.Unmarshal([]byte(data), &groups); err != nil {
		panic(fmt.Errorf("invalid embedded BudouX model: %w", err))
	}
	var model phraseModel
	for _, values := range groups {
		for _, value := range values {
			model.base -= value
		}
	}
	for i, feature := range budouxFeatures {
		model.groups[i] = groups[feature.name]
	}
	return model
}

var japanesePhrases = sync.OnceValue(func() phraseModel { return loadPhraseModel(japaneseModelJSON) })
var thaiPhrases = sync.OnceValue(func() phraseModel { return loadPhraseModel(thaiModelJSON) })
var simplifiedChinesePhrases = sync.OnceValue(func() phraseModel { return loadPhraseModel(simplifiedChineseModelJSON) })
var traditionalChinesePhrases = sync.OnceValue(func() phraseModel { return loadPhraseModel(traditionalChineseModelJSON) })

// boundary is the exact pretrained AdaBoost decision. Features slice the
// original UTF-8 text at known rune offsets, so scoring creates no new strings
// and never treats a combining mark or an emoji codepoint as a legal cut.
func (m phraseModel) boundary(text string, bytes []int, index int) bool {
	return m.score(text, bytes, index) > 0
}

// score retains the signed AdaBoost margin. It ranks candidate boundaries;
// it is not a calibrated probability or proof of a complete sentence.
func (m phraseModel) score(text string, bytes []int, index int) int {
	score, length := m.base, len(bytes)-1
	for i, feature := range budouxFeatures {
		start, end := index+feature.start, index+feature.end
		if start >= 0 && end <= length {
			score += 2 * m.groups[i][text[bytes[start]:bytes[end]]]
		}
	}
	return score
}
