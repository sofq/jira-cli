package avatar

import (
	"strings"
	"testing"
)

// TestAnalyzeComments_Empty verifies that nil/empty input returns zero-value CommentStats.
func TestAnalyzeComments_Empty(t *testing.T) {
	got := AnalyzeComments(nil)
	if got.AvgLengthWords != 0 {
		t.Errorf("AvgLengthWords = %f, want 0", got.AvgLengthWords)
	}
	if got.MedianLengthWords != 0 {
		t.Errorf("MedianLengthWords = %f, want 0", got.MedianLengthWords)
	}
	if got.Formatting.UsesBullets != 0 {
		t.Errorf("UsesBullets = %f, want 0", got.Formatting.UsesBullets)
	}
	if got.Formatting.UsesCodeBlocks != 0 {
		t.Errorf("UsesCodeBlocks = %f, want 0", got.Formatting.UsesCodeBlocks)
	}
	if got.Formatting.UsesMentions != 0 {
		t.Errorf("UsesMentions = %f, want 0", got.Formatting.UsesMentions)
	}
	if len(got.Vocabulary.CommonPhrases) != 0 {
		t.Errorf("CommonPhrases len = %d, want 0", len(got.Vocabulary.CommonPhrases))
	}
	if len(got.Vocabulary.Jargon) != 0 {
		t.Errorf("Jargon len = %d, want 0", len(got.Vocabulary.Jargon))
	}

	got2 := AnalyzeComments([]string{})
	if got2.AvgLengthWords != 0 {
		t.Errorf("empty slice: AvgLengthWords = %f, want 0", got2.AvgLengthWords)
	}
}

// TestAnalyzeComments feeds 5 diverse comments and asserts non-zero stats for
// bullets, code blocks, mentions, and questions.
func TestAnalyzeComments(t *testing.T) {
	comments := []string{
		// has bullet and mention and question
		"- Fix the login bug\n- Update the docs\nCan you check @alice?",
		// has code block and exclamation
		"Here is a snippet:\n```\nfoo := bar()\n```\nThis is great!",
		// has heading and first-person
		"# Overview\nI think we should update the pipeline. I'll check later.",
		// has emoji and mention
		"Thanks for the PR \U0001F44D. Ping @bob when ready.",
		// plain, imperative opening
		"Fix the broken test before merging. Deploy to staging first.",
	}

	got := AnalyzeComments(comments)

	if got.AvgLengthWords == 0 {
		t.Error("AvgLengthWords should be non-zero")
	}
	if got.MedianLengthWords == 0 {
		t.Error("MedianLengthWords should be non-zero")
	}
	if got.Formatting.UsesBullets == 0 {
		t.Error("UsesBullets should be non-zero (comment 0 has bullets)")
	}
	if got.Formatting.UsesCodeBlocks == 0 {
		t.Error("UsesCodeBlocks should be non-zero (comment 1 has code block)")
	}
	if got.Formatting.UsesMentions == 0 {
		t.Error("UsesMentions should be non-zero (comments 0 and 3 have @mentions)")
	}
	if got.ToneSignals.QuestionRatio == 0 {
		t.Error("QuestionRatio should be non-zero (comment 0 has a question)")
	}
	if got.ToneSignals.ExclamationRatio == 0 {
		t.Error("ExclamationRatio should be non-zero (comment 1 has exclamation)")
	}
	if got.ToneSignals.FirstPersonRatio == 0 {
		t.Error("FirstPersonRatio should be non-zero (comment 2 has I/I'll)")
	}
	if got.ToneSignals.ImperativeRatio == 0 {
		t.Error("ImperativeRatio should be non-zero (comment 4 starts with Fix/Deploy)")
	}
}

// TestAnalyzeComments_ZeroTotalSentences verifies behavior when comments produce no sentences.
func TestAnalyzeComments_ZeroTotalSentences(t *testing.T) {
	// Empty strings produce no sentences, so totalSentences = 0 → tone stays at zero
	got := AnalyzeComments([]string{""})
	if got.ToneSignals.QuestionRatio != 0 {
		t.Errorf("QuestionRatio = %f, want 0 for empty comments", got.ToneSignals.QuestionRatio)
	}
	if got.ToneSignals.FirstPersonRatio != 0 {
		t.Errorf("FirstPersonRatio = %f, want 0 for empty comments", got.ToneSignals.FirstPersonRatio)
	}
}

// TestAnalyzeComments_EvenCount verifies median calculation with even number of comments.
func TestAnalyzeComments_EvenCount(t *testing.T) {
	// 4 comments (even count): median = avg of 2nd and 3rd when sorted by word count
	comments := []string{
		"one",                     // 1 word
		"two three",               // 2 words
		"four five six",           // 3 words
		"seven eight nine ten",    // 4 words
	}
	got := AnalyzeComments(comments)
	if got.MedianLengthWords != 2.5 {
		t.Errorf("MedianLengthWords = %f, want 2.5 (even-count median)", got.MedianLengthWords)
	}
}

// TestAnalyzeComments_LongDescriptions tests the "long" length distribution.
func TestAnalyzeComments_LongDescriptions(t *testing.T) {
	// Build a comment with >= 80 words to hit the "long" length bucket.
	words := make([]string, 90)
	for i := range words {
		words[i] = "word"
	}
	longComment := strings.Join(words, " ")
	got := AnalyzeComments([]string{longComment})
	if got.LengthDist.LongPct != 1.0 {
		t.Errorf("LengthDist.LongPct = %f, want 1.0 for 90-word comment", got.LengthDist.LongPct)
	}
}

// TestWordCount verifies the unexported wordCount helper.
func TestWordCount(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello world", 2},
		{"  spaced  out  words  ", 3},
		{"", 0},
		{"single", 1},
	}
	for _, tc := range tests {
		got := wordCount(tc.input)
		if got != tc.want {
			t.Errorf("wordCount(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// TestExtractCommonPhrases verifies that repeated phrases across comments are detected.
func TestExtractCommonPhrases(t *testing.T) {
	comments := []string{
		"please review this change before merging",
		"please review the PR and let me know",
		"can you please review the code",
		"let me know when ready",
		"let me know if you have questions",
	}

	phrases := extractCommonPhrases(comments, 20)

	// "please review" appears in 3 comments; "let me know" appears in 2 comments — both must be found.
	found := make(map[string]bool)
	for _, p := range phrases {
		found[p] = true
	}
	if !found["please review"] {
		t.Errorf("expected 'please review' in phrases, got %v", phrases)
	}
	if !found["let me know"] {
		t.Errorf("expected 'let me know' in phrases, got %v", phrases)
	}
}

// TestAnalyzeDescriptions_Empty verifies zero-value DescriptionStats for empty input.
func TestAnalyzeDescriptions_Empty(t *testing.T) {
	got := AnalyzeDescriptions(nil)
	if got.AvgLengthWords != 0 {
		t.Errorf("AvgLengthWords = %f, want 0", got.AvgLengthWords)
	}
	if len(got.StructurePatterns) != 0 {
		t.Errorf("StructurePatterns len = %d, want 0", len(got.StructurePatterns))
	}

	got2 := AnalyzeDescriptions([]string{})
	if got2.AvgLengthWords != 0 {
		t.Errorf("empty slice: AvgLengthWords = %f, want 0", got2.AvgLengthWords)
	}
}

// TestAnalyzeDescriptions_StructurePatterns verifies that pattern keywords are detected.
func TestAnalyzeDescriptions_StructurePatterns(t *testing.T) {
	descriptions := []string{
		"Background: this is context for the issue.\nSteps to reproduce: click button.",
		"Acceptance criteria: must pass all tests.\nAC: verified by QA.",
		"Some plain description without keywords.",
	}

	got := AnalyzeDescriptions(descriptions)
	if got.AvgLengthWords == 0 {
		t.Error("AvgLengthWords should be non-zero")
	}

	found := map[string]bool{}
	for _, p := range got.StructurePatterns {
		found[p] = true
	}
	if !found["steps_to_reproduce"] {
		t.Errorf("expected 'steps_to_reproduce' in StructurePatterns, got %v", got.StructurePatterns)
	}
	if !found["acceptance_criteria"] {
		t.Errorf("expected 'acceptance_criteria' in StructurePatterns, got %v", got.StructurePatterns)
	}
}

// TestAnalyzeDescriptions_Formatting verifies that bullet and heading formatting is detected.
func TestAnalyzeDescriptions_Formatting(t *testing.T) {
	descriptions := []string{
		"- item one\n- item two\n# Heading here",
		"- another bullet point",
		"plain text description",
	}
	got := AnalyzeDescriptions(descriptions)
	if got.Formatting.UsesBullets == 0 {
		t.Error("UsesBullets should be non-zero")
	}
	if got.Formatting.UsesHeadings == 0 {
		t.Error("UsesHeadings should be non-zero")
	}
}

// TestSplitSentences verifies sentence splitting.
func TestSplitSentences(t *testing.T) {
	tests := []struct {
		input string
		want  int // expected number of sentences
	}{
		{"Hello world.", 1},
		{"Hello. World.", 2},
		{"Is this right? Yes it is! Great.", 3},
		{"", 0},
		{"No punctuation here", 1},
		{"Multi\nline\ntext", 3},
		{"Ends with period.", 1},
	}
	for _, tc := range tests {
		got := splitSentences(tc.input)
		if len(got) != tc.want {
			t.Errorf("splitSentences(%q) = %d sentences, want %d: %v", tc.input, len(got), tc.want, got)
		}
	}
}

// TestSplitSentences_EndOfLine verifies that sentences ending a line (no trailing space) are captured.
func TestSplitSentences_EndOfLine(t *testing.T) {
	input := "First sentence.\nSecond sentence.\nThird."
	got := splitSentences(input)
	if len(got) != 3 {
		t.Errorf("expected 3 sentences from multi-line, got %d: %v", len(got), got)
	}
}

// TestExtractJargon_InsufficientFrequency verifies that words below threshold are excluded.
func TestExtractJargon_InsufficientFrequency(t *testing.T) {
	// Each comment has different technical words — none appear >= 3 times.
	comments := []string{
		"deploying kubernetes",
		"configuring nginx",
		"running terraform",
	}
	jargon := extractJargon(comments)
	if len(jargon) != 0 {
		t.Errorf("expected empty jargon (all words appear <3 times), got %v", jargon)
	}
}

// TestExtractJargon_FrequentWords verifies that words appearing >= 3 times are included.
func TestExtractJargon_FrequentWords(t *testing.T) {
	// "kubernetes" appears in all 4 comments.
	comments := []string{
		"deploying kubernetes pods",
		"kubernetes cluster config",
		"kubernetes networking issue",
		"upgrading kubernetes version",
	}
	jargon := extractJargon(comments)
	found := false
	for _, j := range jargon {
		if j == "kubernetes" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'kubernetes' in jargon, got %v", jargon)
	}
}

// TestExtractJargon_MoreThan10Words verifies that only top 10 jargon words are returned.
func TestExtractJargon_MoreThan10Words(t *testing.T) {
	// Build a comment with 15 unique technical words (all >3 chars, not stop words).
	// Each word must appear in >= 3 unique comments.
	techWords := []string{
		"kubernetes", "deployment", "terraform", "ansible", "dockerfile",
		"microservice", "monitoring", "alerting", "observability", "loadbalancer",
		"prometheus", "grafana", "elasticsearch", "postgresql", "rabbitmq",
	}
	// Create 4 comments each containing all 15 words so each word has count=4 >= 3.
	commentText := strings.Join(techWords, " ")
	comments := []string{commentText, commentText, commentText, commentText}
	jargon := extractJargon(comments)
	if len(jargon) > 10 {
		t.Errorf("expected at most 10 jargon words, got %d: %v", len(jargon), jargon)
	}
	if len(jargon) < 10 {
		t.Errorf("expected exactly 10 jargon words (capped from 15), got %d: %v", len(jargon), jargon)
	}
}

// TestExtractCommonPhrases_NoRepeated verifies that phrases appearing < 2 times are excluded.
func TestExtractCommonPhrases_NoRepeated(t *testing.T) {
	comments := []string{
		"hello world foo",
		"different words here",
		"completely other content",
	}
	phrases := extractCommonPhrases(comments, 20)
	if len(phrases) != 0 {
		t.Errorf("expected empty phrases (none repeated), got %v", phrases)
	}
}

// TestExtractCommonPhrases_MaxCap verifies that at most maxPhrases are returned.
func TestExtractCommonPhrases_MaxCap(t *testing.T) {
	// Build many comments with many overlapping phrases, use maxPhrases=2 to cap output.
	comments := []string{
		"please review this change before merging",
		"please review the PR carefully and let me know",
		"can you please review the code and merge",
		"let me know when ready to merge",
		"let me know if you have any questions today",
	}
	phrases := extractCommonPhrases(comments, 2)
	if len(phrases) > 2 {
		t.Errorf("expected at most 2 phrases with maxPhrases=2, got %d: %v", len(phrases), phrases)
	}
}

// TestAnalyzeComments_MediumLength covers the default/medium branch (21-79 words)
// in the length distribution switch inside AnalyzeComments.
func TestAnalyzeComments_MediumLength(t *testing.T) {
	// Build a comment with 40 words — falls in the medium bucket (21-79).
	words := make([]string, 40)
	for i := range words {
		words[i] = "word"
	}
	mediumComment := strings.Join(words, " ")
	got := AnalyzeComments([]string{mediumComment})
	if got.LengthDist.MediumPct != 1.0 {
		t.Errorf("LengthDist.MediumPct = %f, want 1.0 for 40-word comment", got.LengthDist.MediumPct)
	}
	if got.LengthDist.ShortPct != 0 {
		t.Errorf("LengthDist.ShortPct = %f, want 0", got.LengthDist.ShortPct)
	}
	if got.LengthDist.LongPct != 0 {
		t.Errorf("LengthDist.LongPct = %f, want 0", got.LengthDist.LongPct)
	}
}

// TestExtractJargon_DifferentFrequencies covers the sort comparison branch in
// extractJargon where two candidates have different counts (count != count),
// triggering the return inside the if block.
func TestExtractJargon_DifferentFrequencies(t *testing.T) {
	// "kubernetes" appears in 4 comments (count=4); "deployment" appears in 3 (count=3).
	// Both exceed the threshold of 3, so both are candidates. The sort will compare
	// their counts, which differ — exercising the inner return branch.
	comments := []string{
		"kubernetes deployment config",
		"kubernetes deployment setup",
		"kubernetes cluster setup",
		"kubernetes networking issue",
	}
	jargon := extractJargon(comments)
	if len(jargon) == 0 {
		t.Fatal("expected jargon words, got none")
	}
	// kubernetes appears 4 times, deployment appears 2 times (below threshold of 3),
	// so only kubernetes should appear. Let's use a case where both exceed threshold.
	// Re-create with deployment in 3 comments and kubernetes in 4.
	comments2 := []string{
		"kubernetes deployment config",
		"kubernetes deployment setup",
		"kubernetes deployment service",
		"kubernetes networking issue",
	}
	jargon2 := extractJargon(comments2)
	found := make(map[string]bool)
	for _, j := range jargon2 {
		found[j] = true
	}
	if !found["kubernetes"] {
		t.Errorf("expected 'kubernetes' in jargon, got %v", jargon2)
	}
	if !found["deployment"] {
		t.Errorf("expected 'deployment' in jargon (appears 3 times), got %v", jargon2)
	}
	// kubernetes (count=4) must come before deployment (count=3) in sorted order.
	if len(jargon2) >= 2 && jargon2[0] != "kubernetes" {
		t.Errorf("expected 'kubernetes' first (higher frequency), got %q", jargon2[0])
	}
}

// TestDetectSignOffs verifies that common sign-off patterns at end of comments are detected.
func TestDetectSignOffs(t *testing.T) {
	comments := []string{
		"Looks good to me. Thanks",
		"Merged the fix. Thanks",
		"Will do. lmk",
		"Updated the ticket. lmk",
		"This is a plain comment with no sign-off pattern here at all",
	}

	signOffs := detectSignOffs(comments)

	found := make(map[string]bool)
	for _, s := range signOffs {
		found[s] = true
	}
	if !found["thanks"] {
		t.Errorf("expected 'thanks' in sign-offs, got %v", signOffs)
	}
	if !found["lmk"] {
		t.Errorf("expected 'lmk' in sign-offs, got %v", signOffs)
	}
}
