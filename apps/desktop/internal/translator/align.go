package translator

import "unicode"

// The model re-segments text; OneSub keeps the source's words and times.
// Like VideoCaptioner's split alignment, the model's lines are matched back
// onto the source, here with one character-level Myers diff over letters
// and digits, so a changed space, punctuation mark or capital letter never
// moves a boundary.

// alignment is where the model's lines start in the source atoms.
type alignment struct {
	// cuts are the atom indices where a new segment starts, ascending and
	// never 0. A line that starts inside an atom the previous line ended in
	// joins that line, since an atom cannot be cut.
	cuts []int
	// similarity is 2 × matched / (source + answer) letters and digits.
	similarity float64
}

// alignLines maps answer lines onto atom texts.
func alignLines(atoms []string, lines []string) alignment {
	var src, out []rune
	var srcAtom, outLine []int
	for i, text := range atoms {
		for _, r := range text {
			if keepRune(r) {
				src = append(src, unicode.ToLower(r))
				srcAtom = append(srcAtom, i)
			}
		}
	}
	for k, text := range lines {
		for _, r := range text {
			if keepRune(r) {
				out = append(out, unicode.ToLower(r))
				outLine = append(outLine, k)
			}
		}
	}
	if len(src)+len(out) == 0 {
		return alignment{similarity: 1}
	}
	matches := commonPairs(src, out)
	result := alignment{similarity: 2 * float64(len(matches)) / float64(len(src)+len(out))}
	first := make([]int, len(lines))
	last := make([]int, len(lines))
	for k := range lines {
		first[k], last[k] = -1, -1
	}
	for _, m := range matches {
		k, atom := outLine[m[1]], srcAtom[m[0]]
		if first[k] < 0 {
			first[k] = atom
		}
		last[k] = atom
	}
	previous := -1
	for k := range lines {
		if first[k] < 0 {
			continue
		}
		if previous >= 0 && first[k] > previous {
			result.cuts = append(result.cuts, first[k])
		}
		previous = max(previous, last[k])
	}
	return result
}

func keepRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// commonPairs returns the matched index pairs (a, b) of a longest common
// subsequence found by Myers' O(ND) diff. It gives up, returning nothing,
// once the sequences differ in more than a third of their length: such an
// answer is not the source re-segmented, and the trace stays small.
func commonPairs(a, b []rune) [][2]int {
	n, m := len(a), len(b)
	limit := min(n+m, (n+m)/3+16)
	offset := limit + 1
	v := make([]int, 2*limit+3)
	trace := [][]int{}
	for d := 0; d <= limit; d++ {
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				trace = append(trace, append([]int(nil), v[offset-d:offset+d+1]...))
				return backtrack(trace, n, m)
			}
		}
		trace = append(trace, append([]int(nil), v[offset-d:offset+d+1]...))
	}
	return nil
}

// backtrack walks the saved frontiers from the end to the start and keeps
// the diagonal (matching) steps. trace[d][k+d] is the furthest x on diagonal
// k after d edits.
func backtrack(trace [][]int, n, m int) [][2]int {
	pairs := [][2]int{}
	x, y := n, m
	for d := len(trace) - 1; d > 0; d-- {
		previous := trace[d-1]
		at := func(k int) int { return previous[k+d-1] }
		k := x - y
		var prevK int
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := at(prevK)
		prevY := prevX - prevK
		midX, midY := prevX+1, prevY
		if prevK == k+1 {
			midX, midY = prevX, prevY+1
		}
		for x > midX && y > midY {
			x--
			y--
			pairs = append(pairs, [2]int{x, y})
		}
		x, y = prevX, prevY
	}
	for x > 0 && y > 0 {
		x--
		y--
		pairs = append(pairs, [2]int{x, y})
	}
	for i, j := 0, len(pairs)-1; i < j; i, j = i+1, j-1 {
		pairs[i], pairs[j] = pairs[j], pairs[i]
	}
	return pairs
}
