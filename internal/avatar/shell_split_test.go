package avatar

import (
	"testing"
)

func TestShellSplit_SimpleTokens(t *testing.T) {
	tokens, err := shellSplit("foo bar baz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
	if tokens[0] != "foo" || tokens[1] != "bar" || tokens[2] != "baz" {
		t.Errorf("unexpected tokens: %v", tokens)
	}
}

func TestShellSplit_DoubleQuotes(t *testing.T) {
	tokens, err := shellSplit(`cmd --flag "hello world"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
	if tokens[2] != "hello world" {
		t.Errorf("expected 'hello world', got %q", tokens[2])
	}
}

func TestShellSplit_SingleQuotes(t *testing.T) {
	tokens, err := shellSplit(`cmd --flag 'hello world'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
	if tokens[2] != "hello world" {
		t.Errorf("expected 'hello world', got %q", tokens[2])
	}
}

func TestShellSplit_SingleQuoteInsideDouble(t *testing.T) {
	// Single quote inside double quotes is literal.
	tokens, err := shellSplit(`cmd "it's fine"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(tokens), tokens)
	}
	if tokens[1] != "it's fine" {
		t.Errorf("expected \"it's fine\", got %q", tokens[1])
	}
}

func TestShellSplit_DoubleQuoteInsideSingle(t *testing.T) {
	// Double quote inside single quotes is literal.
	tokens, err := shellSplit(`cmd 'say "hello"'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(tokens), tokens)
	}
	if tokens[1] != `say "hello"` {
		t.Errorf("expected `say \"hello\"`, got %q", tokens[1])
	}
}

func TestShellSplit_MultipleSpaces(t *testing.T) {
	tokens, err := shellSplit("  foo   bar  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(tokens), tokens)
	}
}

func TestShellSplit_EmptyString(t *testing.T) {
	tokens, err := shellSplit("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}
}

func TestShellSplit_UnclosedSingleQuote(t *testing.T) {
	_, err := shellSplit("cmd 'unclosed")
	if err == nil {
		t.Fatal("expected error for unclosed single quote")
	}
}

func TestShellSplit_UnclosedDoubleQuote(t *testing.T) {
	_, err := shellSplit(`cmd "unclosed`)
	if err == nil {
		t.Fatal("expected error for unclosed double quote")
	}
}
