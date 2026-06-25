package preview

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Renderer previews a supported file as a complete HTML document.
type Renderer interface {
	CanPreview(name string) bool
	Render(name string, content []byte) ([]byte, error)
}

var defaultRenderer Renderer = MarkdownRenderer{}

// CanPreview reports whether the default renderer can preview name.
func CanPreview(name string) bool {
	return defaultRenderer.CanPreview(name)
}

// Render renders name/content with the default renderer.
func Render(name string, content []byte) ([]byte, error) {
	return defaultRenderer.Render(name, content)
}

// MarkdownRenderer renders Markdown files to self-contained HTML.
type MarkdownRenderer struct{}

// CanPreview reports whether name has a supported Markdown extension.
func (MarkdownRenderer) CanPreview(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	default:
		return false
	}
}

// Render renders Markdown content to a self-contained dark HTML page.
func (MarkdownRenderer) Render(name string, content []byte) ([]byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
			extension.Strikethrough,
			extension.TaskList,
			extension.Linkify,
		),
	)

	var body bytes.Buffer
	if err := md.Convert(content, &body); err != nil {
		return nil, err
	}

	var page bytes.Buffer
	if err := pageTemplate.Execute(&page, pageData{
		Title: filepath.Base(name),
		Body:  template.HTML(body.String()),
	}); err != nil {
		return nil, err
	}
	return page.Bytes(), nil
}

type pageData struct {
	Title string
	Body  template.HTML
}

var pageTemplate = template.Must(template.New("preview").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root{color-scheme:dark;--bg:#0d1117;--panel:#161b22;--text:#e6edf3;--muted:#8b949e;--border:#30363d;--accent:#58a6ff;--code:#010409}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:16px/1.6 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.topbar,.footer{background:var(--panel);border-color:var(--border);color:var(--muted)}.topbar{border-bottom:1px solid var(--border);padding:12px 24px;font-weight:600}.footer{border-top:1px solid var(--border);padding:16px 24px;text-align:center}.content{max-width:920px;margin:0 auto;padding:32px 24px 56px}a{color:var(--accent)}h1,h2,h3,h4,h5,h6{line-height:1.25;margin:1.5em 0 .6em}h1,h2{border-bottom:1px solid var(--border);padding-bottom:.3em}p,ul,ol,blockquote,pre,table{margin:0 0 1em}blockquote{border-left:4px solid var(--border);color:var(--muted);padding:0 1em}code{background:rgba(110,118,129,.28);border-radius:6px;padding:.2em .4em;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}pre{background:var(--code);border:1px solid var(--border);border-radius:8px;overflow:auto;padding:16px}pre code{background:transparent;padding:0}table{border-collapse:collapse;width:100%;display:block;overflow:auto}th,td{border:1px solid var(--border);padding:6px 13px}th{background:var(--panel)}img{max-width:100%}hr{border:0;border-top:1px solid var(--border);margin:24px 0}.task-list-item{list-style-type:none}.task-list-item input{margin:0 .5em 0 -1.4em}
</style>
</head>
<body>
<header class="topbar">{{.Title}}</header>
<main class="content">
{{.Body}}
</main>
<footer class="footer">served by serv</footer>
</body>
</html>
`))
