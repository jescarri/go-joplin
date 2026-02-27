package clipper

import (
	"strings"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// htmlToMarkdown converts HTML content to Markdown.
func htmlToMarkdown(html string) (string, error) {
	md, err := htmltomd.ConvertString(html)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(md), nil
}
