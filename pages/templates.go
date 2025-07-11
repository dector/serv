package pages

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dector/serv/fs"
)

type DirectoryEntry struct {
	Name     string
	IsDir    bool
	Icon     template.HTML
	Suffix   string
	CSSClass string
}

type PathSegment struct {
	Name   string
	URL    string
	IsLast bool
}

type DirectoryPageData struct {
	Path         string
	BaseName     string
	PathSegments []PathSegment
	Entries      []DirectoryEntry
}

const directoryTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>{{.Path}}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        h1 { color: #333; display: flex; align-items: center; gap: 8px; }
        h1 a { text-decoration: none; color: #0066cc; }
        h1 a:hover { text-decoration: underline; }
        h1 a:first-child { padding-left: 20px; }
        h1 .separator { margin: 0 2px; color: #666; }
        ul { list-style-type: none; padding: 0; }
        li { margin: 5px 0; display: flex; align-items: center; }
        a { text-decoration: none; color: #0066cc; display: flex; align-items: center; gap: 8px; }
        a:hover { text-decoration: underline; }
        .dir { font-weight: bold; }
        .file { color: #666; }
        .icon { width: 16px; height: 16px; }
        .footer { margin-top: 40px; color: #666; font-size: 14px; } .footer a { color: #0066cc; text-decoration: none; display: inline-block; } .footer a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <h1>{{range $i, $segment := .PathSegments}}<a href="{{$segment.URL}}">{{if eq $segment.Name "."}}{{if $segment.IsLast}}/{{else}}/{{end}}{{else}}{{$segment.Name}}/{{end}}</a>{{end}}</h1>
    <ul>
{{range .Entries}}        <li><a href="{{.Name}}{{.Suffix}}" class="{{.CSSClass}}">{{.Icon}} {{.Name}}{{.Suffix}}</a></li>
{{end}}    </ul><div class="footer">served by <a href="https://github.com/dector/serv" target="_blank">serv</a></div>
</body>
</html>`

func GenerateFolderPage(node *fs.FsNode, relativePath string) []byte {
	entries, err := os.ReadDir(node.Path)
	if err != nil {
		return []byte(fmt.Sprintf("<html><body><h1>Error reading directory</h1><p>%s</p></body></html>", err.Error()))
	}

	var dirEntries []DirectoryEntry
	for _, entry := range entries {
		var icon template.HTML
		var suffix, cssClass string
		if entry.IsDir() {
			icon = template.HTML(`<svg class="icon" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path></svg>`)
			suffix = "/"
			cssClass = "dir"
		} else {
			icon = template.HTML(`<svg class="icon" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path></svg>`)
			suffix = ""
			cssClass = "file"
		}

		dirEntries = append(dirEntries, DirectoryEntry{
			Name:     entry.Name(),
			IsDir:    entry.IsDir(),
			Icon:     icon,
			Suffix:   suffix,
			CSSClass: cssClass,
		})
	}

	// Sort entries: folders first (alphabetically), then files (alphabetically)
	sort.Slice(dirEntries, func(i, j int) bool {
		if dirEntries[i].IsDir != dirEntries[j].IsDir {
			return dirEntries[i].IsDir // directories come first
		}
		return dirEntries[i].Name < dirEntries[j].Name
	})

	// Add parent directory entry if not in root
	if relativePath != "" && relativePath != "." {
		parentEntry := DirectoryEntry{
			Name:     "..",
			IsDir:    true,
			Icon:     template.HTML(`<svg class="icon" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path></svg>`),
			Suffix:   "/",
			CSSClass: "dir",
		}
		dirEntries = append([]DirectoryEntry{parentEntry}, dirEntries...)
	}

	displayPath := relativePath
	if displayPath == "" || displayPath == "." {
		displayPath = "./"
	} else if displayPath[0] != '/' && displayPath[:2] != "./" {
		displayPath = "./" + displayPath + "/"
	}

	// Generate path segments for breadcrumb navigation
	var pathSegments []PathSegment
	if relativePath == "" || relativePath == "." {
		pathSegments = append(pathSegments, PathSegment{Name: ".", URL: "/", IsLast: true})
	} else {
		// Add root segment
		pathSegments = append(pathSegments, PathSegment{Name: ".", URL: "/", IsLast: false})

		// Split path and create segments
		parts := strings.Split(strings.Trim(filepath.ToSlash(relativePath), "/"), "/")

		currentPath := ""
		for i, part := range parts {
			if part == "" {
				continue
			}
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = currentPath + "/" + part
			}
			isLast := i == len(parts)-1
			pathSegments = append(pathSegments, PathSegment{
				Name:   part,
				URL:    "/" + currentPath + "/",
				IsLast: isLast,
			})
		}
	}

	data := DirectoryPageData{
		Path:         displayPath,
		BaseName:     filepath.Base(displayPath),
		PathSegments: pathSegments,
		Entries:      dirEntries,
	}

	tmpl, err := template.New("directory").Parse(directoryTemplate)
	if err != nil {
		return []byte(fmt.Sprintf("<html><body><h1>Template error</h1><p>%s</p></body></html>", err.Error()))
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return []byte(fmt.Sprintf("<html><body><h1>Template execution error</h1><p>%s</p></body></html>", err.Error()))
	}

	return buf.Bytes()
}
