package preview

import (
	"bytes"
	"strings"
	"testing"
)

func TestCanPreview(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"README.md", true},
		{"notes.markdown", true},
		{"post.mdown", true},
		{"doc.mkd", true},
		{"README.MD", true},
		{"index.html", false},
		{"plain.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanPreview(tt.name); got != tt.want {
				t.Fatalf("CanPreview(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMarkdownRenderFeatures(t *testing.T) {
	input := []byte(strings.Join([]string{
		"# Title",
		"",
		"- item",
		"",
		"```",
		"code",
		"```",
		"",
		"| A | B |",
		"| - | - |",
		"| 1 | 2 |",
		"",
		"- [x] done",
		"",
		"~~old~~",
		"",
		"https://example.com",
	}, "\n"))

	got, err := Render("README.md", input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	checks := []string{
		"<h1>Title</h1>",
		"<li>item</li>",
		"<pre><code>code\n</code></pre>",
		"<table>",
		"<input checked=\"\" disabled=\"\" type=\"checkbox\"",
		"<del>old</del>",
		"<a href=\"https://example.com\">https://example.com</a>",
		"served by serv",
	}
	for _, check := range checks {
		if !bytes.Contains(got, []byte(check)) {
			t.Fatalf("rendered HTML missing %q:\n%s", check, got)
		}
	}
}

func TestMarkdownRenderOmitsUnsafeHTML(t *testing.T) {
	got, err := Render("README.md", []byte("<script>alert(1)</script>\n<div>raw</div>"))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if bytes.Contains(got, []byte("<script>")) || bytes.Contains(got, []byte("<div>raw</div>")) {
		t.Fatalf("rendered HTML contains unsafe raw HTML:\n%s", got)
	}
}
