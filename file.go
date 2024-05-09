package main

import "embed"

//go:embed asserts/* README.md VERSION CHANGELOG.md
var embed_fs embed.FS

func loadEmbeddedFile(path string) ([]byte, error) {
	content, err := embed_fs.ReadFile(path)
	return content, err
}
