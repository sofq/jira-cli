package avatar

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// Compiled regexes
// ---------------------------------------------------------------------------

var (
	reBullets     = regexp.MustCompile(`(?m)^[\s]*[-*•]\s`)
	reCodeBlocks  = regexp.MustCompile("(?s)```.*?```")
	reHeadings    = regexp.MustCompile(`(?m)^#{1,6}\s`)
	reMentions    = regexp.MustCompile(`@\w+`)
	reQuestion    = regexp.MustCompile(`\?`)
	reExclamation = regexp.MustCompile(`!`)
	reFirstPerson = regexp.MustCompile(`(?i)\b(I|I'm|I'll|I've|I'd|my|me|mine)\b`)
	reImperative  = regexp.MustCompile(`(?i)^(fix|add|update|remove|check|deploy|merge|test|run|set|move)\b`)
)

// reEmoji matches common emoji Unicode ranges.
var reEmoji = regexp.MustCompile(`[\x{1F300}-\x{1F9FF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}]`)

// stopWords is a small set of English stop words to exclude from jargon extraction.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "up": true, "as": true, "is": true,
	"was": true, "are": true, "were": true, "be": true, "been": true, "has": true,
	"have": true, "had": true, "do": true, "does": true, "did": true, "will": true,
	"would": true, "should": true, "could": true, "may": true, "might": true,
	"can": true, "this": true, "that": true, "these": true, "those": true,
	"it": true, "its": true, "we": true, "our": true, "you": true, "your": true,
	"they": true, "their": true, "not": true, "also": true, "just": true,
	"so": true, "if": true, "then": true, "when": true, "there": true,
	"what": true, "which": true, "who": true, "how": true, "all": true,
}

// knownSignOffs is the list of patterns to look for in the last 30 chars.
var knownSignOffs = []string{
	"thanks", "thx", "ty", "cheers", "regards", "lgtm", "lmk", "fyi",
	"tia", "ttyl", "brb", "np", "yw",
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// AnalyzeComments analyses plain-text comments and returns aggregate CommentStats.
// Returns zero-value CommentStats for nil/empty input.
func AnalyzeComments(comments []string) CommentStats {
	if len(comments) == 0 {
		return CommentStats{}
	}

	n := float64(len(comments))
	counts := make([]int, len(comments))
	for i, c := range comments {
		counts[i] = wordCount(c)
	}

	// avg word count
	sum := 0
	for _, c := range counts {
		sum += c
	}
	avg := float64(sum) / n

	// median word count
	sorted := make([]int, len(counts))
	copy(sorted, counts)
	sort.Ints(sorted)
	var median float64
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		median = float64(sorted[mid-1]+sorted[mid]) / 2.0
	} else {
		median = float64(sorted[mid])
	}

	// length distribution  (short<=20, long>=80)
	var short, long, medium int
	for _, c := range counts {
		switch {
		case c <= 20:
			short++
		case c >= 80:
			long++
		default:
			medium++
		}
	}
	dist := LengthDist{
		ShortPct:  float64(short) / n,
		MediumPct: float64(medium) / n,
		LongPct:   float64(long) / n,
	}

	// formatting ratios
	var bullets, codeBlocks, headings, emoji, mentions float64
	for _, c := range comments {
		if reBullets.MatchString(c) {
			bullets++
		}
		if reCodeBlocks.MatchString(c) {
			codeBlocks++
		}
		if reHeadings.MatchString(c) {
			headings++
		}
		if reEmoji.MatchString(c) {
			emoji++
		}
		if reMentions.MatchString(c) {
			mentions++
		}
	}
	formatting := FormattingStats{
		UsesBullets:    bullets / n,
		UsesCodeBlocks: codeBlocks / n,
		UsesHeadings:   headings / n,
		UsesEmoji:      emoji / n,
		UsesMentions:   mentions / n,
	}

	// tone signals — computed per sentence
	var totalSentences, questions, exclamations, firstPerson, imperative float64
	for _, c := range comments {
		sentences := splitSentences(c)
		for _, s := range sentences {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			totalSentences++
			if reQuestion.MatchString(s) {
				questions++
			}
			if reExclamation.MatchString(s) {
				exclamations++
			}
			if reFirstPerson.MatchString(s) {
				firstPerson++
			}
			if reImperative.MatchString(s) {
				imperative++
			}
		}
	}
	tone := ToneSignals{}
	if totalSentences > 0 {
		tone.QuestionRatio = questions / totalSentences
		tone.ExclamationRatio = exclamations / totalSentences
		tone.FirstPersonRatio = firstPerson / totalSentences
		tone.ImperativeRatio = imperative / totalSentences
	}

	vocab := VocabularyStats{
		CommonPhrases: extractCommonPhrases(comments, 20),
		Jargon:        extractJargon(comments),
		SignOffs:      detectSignOffs(comments),
	}

	return CommentStats{
		AvgLengthWords:    avg,
		MedianLengthWords: median,
		LengthDist:        dist,
		Formatting:        formatting,
		Vocabulary:        vocab,
		ToneSignals:       tone,
	}
}

// AnalyzeDescriptions analyses issue descriptions and returns DescriptionStats.
// Returns zero-value DescriptionStats for nil/empty input.
func AnalyzeDescriptions(descriptions []string) DescriptionStats {
	if len(descriptions) == 0 {
		return DescriptionStats{}
	}

	n := float64(len(descriptions))
	sum := 0
	for _, d := range descriptions {
		sum += wordCount(d)
	}
	avg := float64(sum) / n

	var bullets, headings float64
	for _, d := range descriptions {
		if reBullets.MatchString(d) {
			bullets++
		}
		if reHeadings.MatchString(d) {
			headings++
		}
	}
	formatting := FormattingStats{
		UsesBullets:  bullets / n,
		UsesHeadings: headings / n,
	}

	// structure pattern detection via keyword matching
	patternKeywords := map[string][]string{
		"acceptance_criteria": {"acceptance criteria", "ac:", "acceptance_criteria"},
		"steps_to_reproduce":  {"steps to reproduce", "steps:", "to reproduce", "reproduction"},
		"background":          {"background:", "context:", "background\n", "overview:"},
	}
	patternCounts := make(map[string]int, len(patternKeywords))
	for _, d := range descriptions {
		lower := strings.ToLower(d)
		for pat, kws := range patternKeywords {
			for _, kw := range kws {
				if strings.Contains(lower, kw) {
					patternCounts[pat]++
					break
				}
			}
		}
	}
	var patterns []string
	// use a deterministic order
	for _, pat := range []string{"acceptance_criteria", "steps_to_reproduce", "background"} {
		if patternCounts[pat] > 0 {
			patterns = append(patterns, pat)
		}
	}

	return DescriptionStats{
		AvgLengthWords:    avg,
		Formatting:        formatting,
		StructurePatterns: patterns,
	}
}

// ---------------------------------------------------------------------------
// Unexported helpers
// ---------------------------------------------------------------------------

// wordCount counts words using strings.Fields.
func wordCount(s string) int {
	return len(strings.Fields(s))
}

// splitSentences splits text into sentences preserving punctuation.
// Splitting occurs on whitespace that follows sentence-ending punctuation (.?!)
// or on newlines. The punctuation stays in the preceding sentence chunk.
func splitSentences(s string) []string {
	var result []string
	// First split on newlines
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Further split on .?! followed by a space, keeping the delimiter in
		// the preceding token by scanning manually.
		start := 0
		for i := 0; i < len(line); i++ {
			ch := line[i]
			if (ch == '.' || ch == '?' || ch == '!') && i+1 < len(line) && line[i+1] == ' ' {
				chunk := strings.TrimSpace(line[start : i+1])
				if chunk != "" {
					result = append(result, chunk)
				}
				start = i + 2 // skip the space
			}
		}
		// Remainder after last split
		if start < len(line) {
			chunk := strings.TrimSpace(line[start:])
			if chunk != "" {
				result = append(result, chunk)
			}
		}
	}
	return result
}

// extractCommonPhrases extracts 2-gram and 3-gram phrases that appear in at
// least 2 comments. Each phrase is counted at most once per comment.
// Returns up to maxPhrases results sorted by frequency descending.
func extractCommonPhrases(comments []string, maxPhrases int) []string {
	freq := make(map[string]int)

	for _, c := range comments {
		// normalise: lowercase, strip punctuation
		clean := strings.Map(func(r rune) rune {
			if unicode.IsPunct(r) {
				return ' '
			}
			return unicode.ToLower(r)
		}, c)
		words := strings.Fields(clean)

		// deduplicate ngrams within this comment
		seen := make(map[string]bool)
		for i := 0; i < len(words); i++ {
			// 2-gram
			if i+1 < len(words) {
				seen[words[i]+" "+words[i+1]] = true
			}
			// 3-gram
			if i+2 < len(words) {
				seen[words[i]+" "+words[i+1]+" "+words[i+2]] = true
			}
		}
		for ng := range seen {
			freq[ng]++
		}
	}

	type entry struct {
		phrase string
		count  int
	}
	var candidates []entry
	for phrase, cnt := range freq {
		if cnt >= 2 {
			candidates = append(candidates, entry{phrase, cnt})
		}
	}
	// sort by count desc, then phrase asc for determinism
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].count != candidates[j].count {
			return candidates[i].count > candidates[j].count
		}
		return candidates[i].phrase < candidates[j].phrase
	})

	var result []string
	for i, e := range candidates {
		if i >= maxPhrases {
			break
		}
		result = append(result, e.phrase)
	}
	return result
}

// extractJargon finds frequent non-stopword terms (>3 chars, >=3 occurrences)
// and returns the top 10.
func extractJargon(comments []string) []string {
	freq := make(map[string]int)
	for _, c := range comments {
		clean := strings.Map(func(r rune) rune {
			if unicode.IsPunct(r) {
				return ' '
			}
			return unicode.ToLower(r)
		}, c)
		words := strings.Fields(clean)
		seen := make(map[string]bool)
		for _, w := range words {
			if len(w) <= 3 || stopWords[w] {
				continue
			}
			seen[w] = true
		}
		for w := range seen {
			freq[w]++
		}
	}

	type entry struct {
		word  string
		count int
	}
	var candidates []entry
	for word, cnt := range freq {
		if cnt >= 3 {
			candidates = append(candidates, entry{word, cnt})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].count != candidates[j].count {
			return candidates[i].count > candidates[j].count
		}
		return candidates[i].word < candidates[j].word
	})

	var result []string
	for i, e := range candidates {
		if i >= 10 {
			break
		}
		result = append(result, e.word)
	}
	return result
}

// detectSignOffs checks the last 30 characters of each comment for common
// sign-off patterns. Returns those appearing in >=2 comments.
func detectSignOffs(comments []string) []string {
	freq := make(map[string]int)
	for _, c := range comments {
		tail := c
		if len(c) > 30 {
			tail = c[len(c)-30:]
		}
		lower := strings.ToLower(tail)
		for _, so := range knownSignOffs {
			if strings.Contains(lower, so) {
				freq[so]++
				break // count at most one sign-off per comment
			}
		}
	}

	var result []string
	for _, so := range knownSignOffs {
		if freq[so] >= 2 {
			result = append(result, so)
		}
	}
	return result
}
