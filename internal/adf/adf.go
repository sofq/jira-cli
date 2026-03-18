package adf

import "strings"

// Doc represents an Atlassian Document Format document.
type Doc struct {
	Type    string  `json:"type"`
	Version int     `json:"version"`
	Content []Block `json:"content"`
}

// Block is a block-level element (paragraph).
type Block struct {
	Type    string   `json:"type"`
	Content []Inline `json:"content,omitempty"`
}

// Inline is an inline element (text node).
type Inline struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// FromText converts plain text into an ADF document.
// Each line becomes a separate paragraph node.
func FromText(text string) Doc {
	if text == "" {
		return Doc{
			Type:    "doc",
			Version: 1,
			Content: []Block{{Type: "paragraph"}},
		}
	}

	// Trim trailing newlines to avoid creating empty paragraph nodes
	// with empty text (which is invalid ADF).
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return Doc{
			Type:    "doc",
			Version: 1,
			Content: []Block{{Type: "paragraph"}},
		}
	}

	lines := strings.Split(text, "\n")
	blocks := make([]Block, len(lines))
	for i, line := range lines {
		blocks[i] = Block{
			Type:    "paragraph",
			Content: []Inline{{Type: "text", Text: line}},
		}
	}

	return Doc{
		Type:    "doc",
		Version: 1,
		Content: blocks,
	}
}
