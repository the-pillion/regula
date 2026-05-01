package dashboard

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var mdRenderer = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Typographer,
	),
	goldmark.WithRendererOptions(
		// HTML inside markdown is permitted: lawyers paste mixed content.
		// goldmark sanitises tag structure but does not strip script tags
		// — that's why dashboard render is gated to authenticated lawyers
		// and the public surface re-renders from stored markdown each
		// request, never trusts cached HTML.
		html.WithUnsafe(),
	),
)

// renderMarkdown returns the HTML rendering of markdown source as a
// safe template.HTML so html/template emits it unescaped.
func renderMarkdown(src string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := mdRenderer.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}
