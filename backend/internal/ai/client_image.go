package ai

import "encoding/json"

// ImageBlock is one base64 image sent before the text of a user turn
// (aiButton inputPhoto, text_image.go). The bytes only go to Anthropic: no
// log line, error or metric carries them.
type ImageBlock struct {
	MediaType string // image/jpeg
	Data      string // standard base64, no data: prefix
}

type wireImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type wireContentBlock struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *wireImageSource `json:"source,omitempty"`
}

// MarshalJSON keeps the plain string content of a text-only turn and sends a
// turn with images as content blocks: the images first, then the text.
func (m Message) MarshalJSON() ([]byte, error) {
	if len(m.Images) == 0 {
		return json.Marshal(struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{m.Role, m.Content})
	}
	blocks := make([]wireContentBlock, 0, len(m.Images)+1)
	for _, img := range m.Images {
		blocks = append(blocks, wireContentBlock{Type: "image", Source: &wireImageSource{Type: "base64", MediaType: img.MediaType, Data: img.Data}})
	}
	blocks = append(blocks, wireContentBlock{Type: "text", Text: m.Content})
	return json.Marshal(struct {
		Role    string             `json:"role"`
		Content []wireContentBlock `json:"content"`
	}{m.Role, blocks})
}
