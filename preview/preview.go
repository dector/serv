package preview

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"

	"github.com/dector/serv/internal/theme"
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
	return RenderWithOptions(name, content, Options{})
}

// Options configures optional preview page features.
type Options struct {
	SideMenu *SideMenu
}

// SideMenu describes an optional directory navigation menu.
type SideMenu struct {
	CurrentPath string
	Items       []SideMenuItem
	Error       string
}

// SideMenuItem describes one directory navigation menu item.
type SideMenuItem struct {
	Label     string
	Href      string
	Title     string
	IsDir     bool
	IsCurrent bool
}

// RenderWithOptions renders name/content with optional page features.
func RenderWithOptions(name string, content []byte, options Options) ([]byte, error) {
	if renderer, ok := defaultRenderer.(interface {
		RenderWithOptions(string, []byte, Options) ([]byte, error)
	}); ok {
		return renderer.RenderWithOptions(name, content, options)
	}
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
func (r MarkdownRenderer) Render(name string, content []byte) ([]byte, error) {
	return r.RenderWithOptions(name, content, Options{})
}

// RenderWithOptions renders Markdown content to a self-contained dark HTML page with optional page features.
func (MarkdownRenderer) RenderWithOptions(name string, content []byte, options Options) ([]byte, error) {
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
		Title:    filepath.Base(name),
		Body:     template.HTML(body.String()),
		Theme:    theme.Dark,
		SideMenu: options.SideMenu,
	}); err != nil {
		return nil, err
	}
	return page.Bytes(), nil
}

type pageData struct {
	Title    string
	Body     template.HTML
	Theme    theme.Colors
	SideMenu *SideMenu
}

var pageTemplate = template.Must(template.New("preview").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root{color-scheme:dark;--bg:{{.Theme.Background}};--panel:{{.Theme.Panel}};--text:{{.Theme.Text}};--muted:{{.Theme.Muted}};--border:{{.Theme.Border}};--accent:{{.Theme.Accent}};--code:{{.Theme.Code}}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:16px/1.6 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.topbar,.footer{background:var(--panel);border-color:var(--border);color:var(--muted)}.topbar{border-bottom:1px solid var(--border);padding:12px 24px;font-weight:600}.footer{border-top:1px solid var(--border);padding:16px 24px;text-align:center}.page-shell{display:flex;min-height:calc(100vh - 50px)}.content{max-width:920px;margin:0 auto;padding:32px 24px 56px;flex:1;min-width:0}.side-menu{background:var(--panel);border-right:1px solid var(--border);flex:0 0 260px;width:260px;max-height:calc(100vh - 50px);overflow:auto;position:sticky;top:0;align-self:flex-start}.side-menu__header{border-bottom:1px solid var(--border);color:var(--muted);font-size:13px;font-weight:600;overflow:hidden;padding:12px 16px;text-overflow:ellipsis;white-space:nowrap}.side-menu__list{list-style:none;margin:0;padding:8px}.side-menu__item{border-radius:6px;color:var(--muted);display:block;overflow:hidden;padding:6px 8px;text-overflow:ellipsis;white-space:nowrap}.side-menu__item a{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.side-menu__item--current{background:rgba(110,118,129,.18);color:var(--text);font-weight:600}.side-menu__kind{color:var(--muted);font-size:12px;margin-right:6px}.side-menu__error{color:var(--muted);padding:12px 16px}.side-menu-toggle{background:var(--panel);border:1px solid var(--border);color:var(--text);cursor:pointer;z-index:3}.side-menu__collapse{border-radius:999px;height:28px;position:absolute;right:-14px;top:50%;transform:translateY(-50%);width:28px}.side-menu__expand{border-left:0;border-radius:0 999px 999px 0;display:none;left:0;padding:16px 6px;position:fixed;top:50%;transform:translateY(-50%)}body.side-menu-collapsed .side-menu{display:none}body.side-menu-collapsed .side-menu__expand{display:block}@media(max-width:760px){.page-shell{display:block}.side-menu{bottom:0;box-shadow:0 0 0 9999px rgba(0,0,0,.45);left:0;max-height:none;position:fixed;top:0;z-index:2}body.side-menu-collapsed .side-menu{display:none}}a{color:var(--accent)}h1,h2,h3,h4,h5,h6{line-height:1.25;margin:1.5em 0 .6em}h1,h2{border-bottom:1px solid var(--border);padding-bottom:.3em}p,ul,ol,blockquote,pre,table{margin:0 0 1em}blockquote{border-left:4px solid var(--border);color:var(--muted);padding:0 1em}code{background:rgba(110,118,129,.28);border-radius:6px;padding:.2em .4em;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}pre{background:var(--code);border:1px solid var(--border);border-radius:8px;overflow:auto;padding:16px}pre code{background:transparent;padding:0}table{border-collapse:collapse;width:100%;display:block;overflow:auto}th,td{border:1px solid var(--border);padding:6px 13px}th{background:var(--panel)}img{max-width:100%}hr{border:0;border-top:1px solid var(--border);margin:24px 0}.task-list-item{list-style-type:none}.task-list-item input{margin:0 .5em 0 -1.4em}
</style>
</head>
<body>
<header class="topbar">{{.Title}}</header>
{{if .SideMenu}}<button class="side-menu-toggle side-menu__expand" type="button" aria-label="Show directory menu">›</button>{{end}}
<div class="page-shell">
{{with .SideMenu}}<nav class="side-menu" aria-label="Directory menu">
<button class="side-menu-toggle side-menu__collapse" type="button" aria-label="Hide directory menu">‹</button>
<div class="side-menu__header" title="{{.CurrentPath}}">{{.CurrentPath}}</div>
{{if .Error}}<div class="side-menu__error">{{.Error}}</div>{{else}}<ul class="side-menu__list">
{{range .Items}}<li class="side-menu__item{{if .IsCurrent}} side-menu__item--current{{end}}" title="{{.Title}}">{{if .IsCurrent}}{{if .IsDir}}<span class="side-menu__kind">[dir]</span>{{end}}{{.Label}}{{else}}<a href="{{.Href}}" title="{{.Title}}">{{if .IsDir}}<span class="side-menu__kind">[dir]</span>{{end}}{{.Label}}</a>{{end}}</li>{{end}}
</ul>{{end}}
</nav>{{end}}
<main class="content">
{{.Body}}
</main>
</div>
<footer class="footer">served by serv</footer>
{{if .SideMenu}}<script>(function(){var key='serv.sideMenuCollapsed';var body=document.body;var apply=function(v){body.classList.toggle('side-menu-collapsed',v==='1')};try{apply(localStorage.getItem(key))}catch(e){}function set(v){apply(v?'1':'0');try{localStorage.setItem(key,v?'1':'0')}catch(e){}}document.querySelectorAll('.side-menu__collapse').forEach(function(b){b.addEventListener('click',function(){set(true)})});document.querySelectorAll('.side-menu__expand').forEach(function(b){b.addEventListener('click',function(){set(false)})});})();</script>{{end}}
</body>
</html>
`))
