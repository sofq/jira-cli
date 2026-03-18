package avatar

import (
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
