package jsonutil

import (
	"strings"
	"testing"
)

func TestMarshalNoEscape(t *testing.T) {
	data, err := MarshalNoEscape(map[string]string{
		"url":  "http://example.com?a=1&b=2",
		"html": "<html>",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(data)
	if strings.Contains(s, `\u0026`) {
		t.Error("MarshalNoEscape should not escape & to \\u0026")
	}
	if strings.Contains(s, `\u003c`) {
		t.Error("MarshalNoEscape should not escape < to \\u003c")
	}
	if !strings.Contains(s, "&") {
		t.Error("MarshalNoEscape should contain literal &")
	}
	if !strings.Contains(s, "<html>") {
		t.Error("MarshalNoEscape should contain literal <html>")
	}
}

func TestMarshalNoEscapeNoTrailingNewline(t *testing.T) {
	data, err := MarshalNoEscape("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.HasSuffix(string(data), "\n") {
		t.Error("result should not have trailing newline")
	}
}

func TestMarshalNoEscapeError(t *testing.T) {
	ch := make(chan int)
	_, err := MarshalNoEscape(ch)
	if err == nil {
		t.Error("expected error for unencodable type")
	}
}
