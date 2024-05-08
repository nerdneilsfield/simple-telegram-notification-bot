package main

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"go.uber.org/zap"
)

func escapeMarkdownV2(text string) string {
	charactersToEscape := []string{"_", "[", "]", "(", ")", "~", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	for _, char := range charactersToEscape {
		text = strings.ReplaceAll(text, char, "\\"+char)
	}
	return text
}

func renderMarkdown(text []byte) []byte {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)
	doc := p.Parse(text)
	return markdown.Render(doc, renderer)
}

func useTemplateRenderMarkdown(text []byte) ([]byte, error) {
	tmplContent, err := loadEmbeddedFile("asserts/html.html")
	if err != nil {
		logger.Error("Failed to load template file", zap.Error(err))
		return nil, err
	}
	tmpl, err := template.New("html.html").Parse(string(tmplContent))
	if err != nil {
		logger.Error("Failed to parse template", zap.Error(err))
		return nil, err
	}

	pageData := PageData{
		MarkdownContent: template.HTML(renderMarkdown(text)),
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, pageData)
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

func loadEmbeddedFile(path string) ([]byte, error) {
	content, err := embed_fs.ReadFile(path)
	return content, err
}
