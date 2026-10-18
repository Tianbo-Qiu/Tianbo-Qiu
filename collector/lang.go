package main

import (
	"path"
	"strings"
)

var langByExt = map[string]string{
	".go":    "Go",
	".sql":   "SQL",
	".sh":    "Shell",
	".bash":  "Shell",
	".zsh":   "Shell",
	".md":    "Markdown",
	".yml":   "YAML",
	".yaml":  "YAML",
	".toml":  "TOML",
	".json":  "JSON",
	".ts":    "TypeScript",
	".tsx":   "TypeScript",
	".js":    "JavaScript",
	".jsx":   "JavaScript",
	".mjs":   "JavaScript",
	".cjs":   "JavaScript",
	".py":    "Python",
	".rs":    "Rust",
	".c":     "C",
	".h":     "C",
	".cc":    "C++",
	".cpp":   "C++",
	".hpp":   "C++",
	".java":  "Java",
	".kt":    "Kotlin",
	".swift": "Swift",
	".rb":    "Ruby",
	".lua":   "Lua",
	".proto": "Protobuf",
	".tf":    "HCL",
	".html":  "HTML",
	".css":   "CSS",
	".scss":  "CSS",
	".tmpl":  "Go Template",
}

var langByName = map[string]string{
	"Makefile":   "Makefile",
	"Dockerfile": "Dockerfile",
}

// Files that churn without anyone writing them: lockfiles, vendored and
// generated code. They're left out of both daily diffs and LOC totals.
var (
	skipNames = map[string]bool{
		"go.sum":            true,
		"package-lock.json": true,
		"yarn.lock":         true,
		"pnpm-lock.yaml":    true,
		"bun.lock":          true,
		"Cargo.lock":        true,
		"poetry.lock":       true,
		"uv.lock":           true,
		"Gemfile.lock":      true,
		"composer.lock":     true,
	}
	skipDirs     = []string{"vendor/", "node_modules/", "third_party/", "dist/"}
	skipSuffixes = []string{".pb.go", "_gen.go", ".gen.go", ".min.js", ".min.css"}
)

func excluded(p string) bool {
	if skipNames[path.Base(p)] {
		return true
	}
	for _, d := range skipDirs {
		if strings.HasPrefix(p, d) || strings.Contains(p, "/"+d) {
			return true
		}
	}
	for _, s := range skipSuffixes {
		if strings.HasSuffix(p, s) {
			return true
		}
	}
	return false
}

// languageOf returns "" for files we don't count.
func languageOf(p string) string {
	base := path.Base(p)
	if l, ok := langByName[base]; ok {
		return l
	}
	return langByExt[strings.ToLower(path.Ext(base))]
}
