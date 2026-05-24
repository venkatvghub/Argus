package providers

import "testing"

func TestIsCI(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{".github/workflows/ci.yml", true},
		{".github/workflows/build.yaml", true},
		{".circleci/config.yml", true},
		{"ci/pipeline.yml", true},
		{"Jenkinsfile", true},
		{"docs/config.yml", false},
		{"src/specialist/settings.yaml", false},
		{".github/README.md", false},
		{"Makefile", false},
		{"not.github/workflows/ci.yml", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isCI(tt.path); got != tt.want {
				t.Errorf("isCI(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
