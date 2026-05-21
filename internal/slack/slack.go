package slack

import (
	"strings"

	"ai-bricklaying/internal/safeio"
)

const maxBlocksPerMessage = 50

type Payload struct {
	Text         string       `json:"text"`
	Blocks       []Block      `json:"blocks"`
	Messages     []Message    `json:"messages"`
	Verification Verification `json:"verification"`
}

type Message struct {
	Text   string  `json:"text"`
	Blocks []Block `json:"blocks"`
}

type Block struct {
	Type string     `json:"type"`
	Text *BlockText `json:"text,omitempty"`
}

type BlockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Verification struct {
	Source                     string   `json:"source"`
	TopLevelSections           []string `json:"top_level_sections"`
	CoveredTopLevelSections    []string `json:"covered_top_level_sections"`
	AllTopLevelSectionsCovered bool     `json:"all_top_level_sections_covered"`
}

func BuildPayload(markdown string) Payload {
	redactedMarkdown := safeio.RedactString(markdown)
	blocks := blocksFromMarkdown(redactedMarkdown)
	messages := messagesFromBlocks(redactedMarkdown, blocks)
	first := Message{Text: fallbackText(redactedMarkdown, 0, 1), Blocks: []Block{}}
	if len(messages) > 0 {
		first = messages[0]
	}
	sections := TopLevelSections(redactedMarkdown)
	covered := coveredSections(sections, messages)

	return Payload{
		Text:     first.Text,
		Blocks:   first.Blocks,
		Messages: messages,
		Verification: Verification{
			Source:                     "saved_markdown",
			TopLevelSections:           sections,
			CoveredTopLevelSections:    covered,
			AllTopLevelSectionsCovered: len(sections) == len(covered),
		},
	}
}

func TopLevelSections(markdown string) []string {
	var sections []string
	for _, line := range markdownLines(markdown) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			section := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			if section != "" {
				sections = append(sections, section)
			}
		}
	}
	return sections
}

func blocksFromMarkdown(markdown string) []Block {
	var blocks []Block
	var paragraph []string
	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		blocks = appendSectionBlocks(blocks, strings.Join(paragraph, "\n"))
		paragraph = nil
	}

	for _, line := range markdownLines(markdown) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flushParagraph()
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## "):
			flushParagraph()
			blocks = append(blocks, headerBlock(strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))))
		case strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### "):
			flushParagraph()
			blocks = append(blocks, headerBlock(strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))))
		case strings.HasPrefix(trimmed, "### "):
			flushParagraph()
			blocks = appendSectionBlocks(blocks, "*"+strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))+"*")
		case isBullet(trimmed):
			paragraph = append(paragraph, "• "+strings.TrimSpace(trimmed[2:]))
		default:
			paragraph = append(paragraph, stripInlineMarkdown(trimmed))
		}
	}
	flushParagraph()
	return blocks
}

func messagesFromBlocks(markdown string, blocks []Block) []Message {
	if len(blocks) == 0 {
		return []Message{{Text: fallbackText(markdown, 0, 1), Blocks: []Block{}}}
	}
	var batches [][]Block
	for start := 0; start < len(blocks); start += maxBlocksPerMessage {
		end := start + maxBlocksPerMessage
		if end > len(blocks) {
			end = len(blocks)
		}
		batch := make([]Block, end-start)
		copy(batch, blocks[start:end])
		batches = append(batches, batch)
	}
	messages := make([]Message, 0, len(batches))
	for index, batch := range batches {
		messages = append(messages, Message{Text: fallbackText(markdown, index, len(batches)), Blocks: batch})
	}
	return messages
}

func appendSectionBlocks(blocks []Block, text string) []Block {
	text = strings.TrimSpace(text)
	for text != "" {
		part := text
		if len(part) > 2900 {
			cut := strings.LastIndex(part[:2900], "\n")
			if cut < 1 {
				cut = 2900
			}
			part = part[:cut]
		}
		blocks = append(blocks, sectionBlock(part))
		text = strings.TrimSpace(strings.TrimPrefix(text, part))
	}
	return blocks
}

func headerBlock(text string) Block {
	return Block{Type: "header", Text: &BlockText{Type: "plain_text", Text: truncate(text, 150)}}
}

func sectionBlock(text string) Block {
	return Block{Type: "section", Text: &BlockText{Type: "mrkdwn", Text: text}}
}

func coveredSections(sections []string, messages []Message) []string {
	combined := blockText(messages)
	covered := make([]string, 0, len(sections))
	for _, section := range sections {
		if strings.Contains(combined, section) {
			covered = append(covered, section)
		}
	}
	return covered
}

func blockText(messages []Message) string {
	var parts []string
	for _, message := range messages {
		for _, block := range message.Blocks {
			if block.Text != nil {
				parts = append(parts, block.Text.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func fallbackText(markdown string, index int, total int) string {
	title := titleFromMarkdown(markdown)
	if total > 1 {
		return title + " (" + itoa(index+1) + "/" + itoa(total) + ")"
	}
	return title
}

func titleFromMarkdown(markdown string) string {
	for _, line := range markdownLines(markdown) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	for _, line := range markdownLines(markdown) {
		trimmed := strings.TrimSpace(stripInlineMarkdown(line))
		if trimmed != "" {
			return strings.TrimLeft(trimmed, "# ")
		}
	}
	return "AI Bricklaying Daily Summary"
}

func markdownLines(markdown string) []string {
	return strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
}

func isBullet(line string) bool {
	return len(line) >= 2 && (strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "))
}

func stripInlineMarkdown(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`")
	return text
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	if limit <= 1 {
		return text[:limit]
	}
	return strings.TrimSpace(text[:limit-1]) + "..."
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
