package main

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"go.uber.org/zap"
	xhtml "golang.org/x/net/html"
)

var markdownRender *html.Renderer
var markdownParser *parser.Parser
var htmlTemplate *template.Template

func escapeMarkdownV2(text string) string {
	charactersToEscape := []string{"_", "[", "]", "(", ")", "~", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	for _, char := range charactersToEscape {
		text = strings.ReplaceAll(text, char, "\\"+char)
	}
	return text
}

func initMarkdownRender() {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock | parser.MathJax | parser.FencedCode | parser.Tables | parser.Strikethrough | parser.SpaceHeadings | parser.LaxHTMLBlocks | parser.Footnotes
	markdownParser = parser.NewWithExtensions(extensions)
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	markdownRender = html.NewRenderer(opts)
	tmplContent, err := loadEmbeddedFile("asserts/html.html")
	if err != nil {
		logger.Error("Failed to load template file", zap.Error(err))
		panic("Failed to load template file")
	}
	htmlTemplate, err = template.New("html.html").Parse(string(tmplContent))
	if err != nil {
		logger.Error("Failed to parse template", zap.Error(err))
		panic("Failed to parse template")
	}
}

func getHTMLTitle(text []byte) string {
	doc, err := xhtml.Parse(bytes.NewReader(text))
	if err != nil {
		logger.Error("Failed to parse HTML", zap.Error(err))
		return "Default Title"
	}
	var f func(*xhtml.Node) string
	f = func(n *xhtml.Node) string {
		if n.Type == xhtml.ElementNode && n.Data == "h1" {
			if n.FirstChild != nil {
				return n.FirstChild.Data
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			return f(c)
		}
		return ""
	}

	// 开始遍历
	return f(doc)
}

func renderMarkdown(text []byte) []byte {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Failed to render markdown", zap.Any("recover", r))
		}
	}()
	doc := markdownParser.Parse(text)
	renderedHTML := markdown.Render(doc, markdownRender)
	// safeHTML := bluemonday.UGCPolicy().SanitizeBytes(renderedHTML)
	return renderedHTML
}

func useTemplateRenderMarkdown(text []byte) ([]byte, error) {
	title := getHTMLTitle(text)
	pageData := PageData{
		Title:           title,
		MarkdownContent: template.HTML(renderMarkdown(text)),
	}
	var buf bytes.Buffer
	err := htmlTemplate.Execute(&buf, pageData)
	if err != nil {
		logger.Error("Failed to execute template", zap.Error(err))
		return nil, err
	}
	return buf.Bytes(), nil
}

func useTemplateRenderEmbeddedFile(embed_path string) ([]byte, error) {
	content, err := loadEmbeddedFile(embed_path)
	if err != nil {
		logger.Error("Failed to load embedded file: "+embed_path, zap.Error(err))
		return nil, err
	}
	return useTemplateRenderMarkdown(content)
}
