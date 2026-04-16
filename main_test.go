package main

import (
	"reflect"
	"testing"
)

func TestExtractReadmeLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "plain text two lines",
			input: "A simple library.\nDoes useful things.",
			want:  []string{"A simple library.", "Does useful things."},
		},
		{
			name:  "plain text single line",
			input: "A simple library.",
			want:  []string{"A simple library."},
		},
		{
			name:  "strips heading markers",
			input: "# My Library\nDoes useful things.",
			want:  []string{"My Library", "Does useful things."},
		},
		{
			name:  "skips blank lines between content",
			input: "First line.\nSecond line.\n\nAfter blank.",
			want:  []string{"First line.", "Second line.", "After blank."},
		},
		{
			name:  "skips leading blank lines",
			input: "\n\nFirst real line.",
			want:  []string{"First real line."},
		},
		{
			name:  "stops at ## subsection heading",
			input: "Intro line.\n\n## Usage\nUsage details.",
			want:  []string{"Intro line."},
		},
		{
			name:  "stops at ### heading",
			input: "Intro.\n### Details\nMore.",
			want:  []string{"Intro."},
		},
		{
			name:  "does not stop at # title heading",
			input: "# My Library\n\nA great library.\n\nDoes many things.",
			want:  []string{"My Library", "A great library.", "Does many things."},
		},
		{
			name: "dasel-style: badges then HTML then heading then content",
			input: "[![badge](https://img.shields.io/badge.svg)](https://example.com)\n" +
				"![Build](https://github.com/owner/repo/workflows/Build/badge.svg)\n" +
				"\n" +
				"<div align=\"center\">\n" +
				"    <img src=\"./logo.png\" alt=\"logo\"/>\n" +
				"</div>\n" +
				"\n" +
				"# Dasel\n" +
				"\n" +
				"Dasel is a command-line tool for querying data.\n" +
				"\n" +
				"It supports JSON, YAML, and TOML.\n" +
				"\n" +
				"---\n" +
				"\n" +
				"## Usage\n" +
				"Usage details.",
			want: []string{"Dasel", "Dasel is a command-line tool for querying data.", "It supports JSON, YAML, and TOML."},
		},
		{
			name:  "skips HTML tags",
			input: "<div align=\"center\">\n<img src=\"logo.png\">\n</div>\nReal content.",
			want:  []string{"Real content."},
		},
		{
			name:  "skips HTML comments",
			input: "<!-- badges -->\nReal content.",
			want:  []string{"Real content."},
		},
		{
			name:  "strips plain badge",
			input: "![Build Status](https://ci.example.com/badge.svg)\nReal content.",
			want:  []string{"Real content."},
		},
		{
			name:  "strips linked badge",
			input: "[![GoDoc](https://pkg.go.dev/badge/example.svg)](https://pkg.go.dev/example)\nReal content.",
			want:  []string{"Real content."},
		},
		{
			name:  "strips emphasis markers",
			input: "**Bold text** and __also bold__.",
			want:  []string{"Bold text and also bold."},
		},
		{
			name:  "strips emoji codes",
			input: ":rocket: Fast. :white_check_mark: Tested.",
			want:  []string{"Fast. Tested."},
		},
		{
			name:  "strips [mirror] noise",
			input: "Go standard library [mirror]",
			want:  []string{"Go standard library"},
		},
		{
			name:  "skips decorative horizontal rules",
			input: "---\nReal content.",
			want:  []string{"Real content."},
		},
		{
			name:  "skips === underlines",
			input: "My Library\n==========\nReal content.",
			want:  []string{"My Library", "Real content."},
		},
		{
			name:  "caps output at 5 lines",
			input: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6",
			want:  []string{"Line 1", "Line 2", "Line 3", "Line 4", "Line 5"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "all badges no content",
			input: "[![a](b)](c)\n![x](y)\n",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractReadmeLines(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractReadmeLines(%q)\n  got  %q\n  want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsDecorative(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"---", true},
		{"===", true},
		{"***", true},
		{"___", true},
		{"- - -", false}, // spaces are not decorative chars
		{"", false},
		{"real text", false},
		{"#", false},
	}
	for _, tt := range tests {
		got := isDecorative(tt.input)
		if got != tt.want {
			t.Errorf("isDecorative(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestModuleToGitHubPath(t *testing.T) {
	tests := []struct {
		module    string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		// Direct GitHub paths
		{"github.com/spf13/cobra", "spf13", "cobra", true},
		{"github.com/spf13/cobra/extra", "spf13", "cobra", true},

		// Exact well-known mappings
		{"google.golang.org/grpc", "grpc", "grpc-go", true},
		{"google.golang.org/protobuf", "protocolbuffers", "protobuf-go", true},
		{"go.opentelemetry.io/otel", "open-telemetry", "opentelemetry-go", true},

		// Prefix well-known mappings
		{"golang.org/x/net", "golang", "net", true},
		{"golang.org/x/text", "golang", "text", true},
		{"go.uber.org/zap", "uber-go", "zap", true},

		// Unknown
		{"gopkg.in/yaml.v3", "", "", false},
		{"k8s.io/client-go", "", "", false},
	}

	for _, tt := range tests {
		owner, repo, ok := moduleToGitHubPath(tt.module)
		if ok != tt.wantOK || owner != tt.wantOwner || repo != tt.wantRepo {
			t.Errorf("moduleToGitHubPath(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.module, owner, repo, ok, tt.wantOwner, tt.wantRepo, tt.wantOK)
		}
	}
}

func TestParseVanityMeta(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantOwner string
		wantRepo  string
		wantRef   string
		wantOK    bool
	}{
		{
			name:      "direct go-import to GitHub",
			body:      `<meta name="go-import" content="dario.cat/mergo git https://github.com/imdario/mergo">`,
			wantOwner: "imdario",
			wantRepo:  "mergo",
			wantRef:   "",
			wantOK:    true,
		},
		{
			name: "gopkg.in style: go-import to self, go-source with GitHub tree and ref",
			body: `<meta name="go-import" content="gopkg.in/yaml.v3 git https://gopkg.in/yaml.v3">` + "\n" +
				`<meta name="go-source" content="gopkg.in/yaml.v3 _ https://github.com/go-yaml/yaml/tree/v3.0.1{/dir} https://github.com/go-yaml/yaml/blob/v3.0.1{/dir}/{file}#L{line}">`,
			wantOwner: "go-yaml",
			wantRepo:  "yaml",
			wantRef:   "v3.0.1",
			wantOK:    true,
		},
		{
			name: "gopkg.in/yaml.v2 with ref v2.4.0",
			body: `<meta name="go-import" content="gopkg.in/yaml.v2 git https://gopkg.in/yaml.v2">` + "\n" +
				`<meta name="go-source" content="gopkg.in/yaml.v2 _ https://github.com/go-yaml/yaml/tree/v2.4.0{/dir} https://github.com/go-yaml/yaml/blob/v2.4.0{/dir}/{file}#L{line}">`,
			wantOwner: "go-yaml",
			wantRepo:  "yaml",
			wantRef:   "v2.4.0",
			wantOK:    true,
		},
		{
			name: "go-import to GitHub, go-source adds ref",
			body: `<meta name="go-import" content="example.com/pkg git https://github.com/owner/repo">` + "\n" +
				`<meta name="go-source" content="example.com/pkg _ https://github.com/owner/repo/tree/v2{/dir} _">`,
			wantOwner: "owner",
			wantRepo:  "repo",
			wantRef:   "v2",
			wantOK:    true,
		},
		{
			name:      "multiline meta tag",
			body:      "<meta name=\"go-import\"\n    content=\"dario.cat/mergo git https://github.com/imdario/mergo\">",
			wantOwner: "imdario",
			wantRepo:  "mergo",
			wantRef:   "",
			wantOK:    true,
		},
		{
			name:      "content attribute before name attribute",
			body:      `<meta content="example.com/pkg git https://github.com/owner/repo" name="go-import">`,
			wantOwner: "owner",
			wantRepo:  "repo",
			wantRef:   "",
			wantOK:    true,
		},
		{
			name:   "non-GitHub VCS host, no go-source",
			body:   `<meta name="go-import" content="example.com/pkg git https://bitbucket.org/owner/repo">`,
			wantOK: false,
		},
		{
			name:   "no meta tags",
			body:   `<html><body>hello</body></html>`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ref, ok := parseVanityMeta([]byte(tt.body))
			if ok != tt.wantOK || owner != tt.wantOwner || repo != tt.wantRepo || ref != tt.wantRef {
				t.Errorf("parseVanityMeta()\n  got  (%q, %q, %q, %v)\n  want (%q, %q, %q, %v)",
					owner, repo, ref, ok, tt.wantOwner, tt.wantRepo, tt.wantRef, tt.wantOK)
			}
		})
	}
}
