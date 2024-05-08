package main

import "embed"

//go:embed asserts/* README.md VERSION CHANGELOG.md
var embed_fs embed.FS
