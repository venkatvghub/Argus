package argus

import "path/filepath"

const (
	jobTypeAnalysis    = "analysis"
	defaultAnalyzePath = "."
)

var extLang = map[string]string{
	".go":    "Go",
	".ts":    "TypeScript",
	".tsx":   "TypeScript",
	".js":    "JavaScript",
	".jsx":   "JavaScript",
	".py":    "Python",
	".java":  "Java",
	".kt":    "Kotlin",
	".swift": "Swift",
	".rs":    "Rust",
	".cpp":   "C++",
	".cc":    "C++",
	".cxx":   "C++",
	".c":     "C",
	".h":     "C",
	".hpp":   "C++",
	".rb":    "Ruby",
	".php":   "PHP",
	".cs":    "C#",
	".scala": "Scala",
	".dart":  "Dart",
	".lua":   "Lua",
	".sh":    "Shell",
	".bash":  "Shell",
	".zsh":   "Shell",
	".sql":   "SQL",
	".r":     "R",
	".ex":    "Elixir",
	".exs":   "Elixir",
	".hs":    "Haskell",
}

func langFromExt(path string) string {
	ext := filepath.Ext(path)
	if lang, ok := extLang[ext]; ok {
		return lang
	}
	return ""
}
