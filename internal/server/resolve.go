package server

import (
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/dector/serv/internal/config"
	"github.com/dector/serv/internal/preview"
)

func requestDirResolveStrategy(r *http.Request, fallback config.DirResolve) config.DirResolve {
	if r == nil || r.URL == nil {
		return fallback
	}
	value := r.URL.Query().Get("resolve")
	if value == "" {
		return fallback
	}
	strategy, err := config.ParseDirResolve(value)
	if err != nil {
		return fallback
	}
	return strategy
}

func directoryCandidatePaths(fsys fs.FS, dir string, strategy config.DirResolve) []string {
	readme := func() []string {
		if p, ok := findReadme(fsys, dir); ok {
			return []string{p}
		}
		return nil
	}
	index := []string{path.Join(dir, "index.html")}
	switch strategy {
	case config.DirResolveReadmeFirst:
		return append(readme(), index...)
	case config.DirResolveIndexFirst:
		return append(index, readme()...)
	case config.DirResolveReadmeOnly:
		return readme()
	case config.DirResolveIndexOnly:
		return index
	default:
		return nil
	}
}

func selectDirectoryCandidate(fsys fs.FS, dir string, strategy config.DirResolve) (string, bool) {
	for _, candidate := range directoryCandidatePaths(fsys, dir, strategy) {
		info, err := fs.Stat(fsys, candidate)
		if err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func findReadme(fsys fs.FS, dir string) (string, bool) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if isReadmeName(entry.Name()) {
			return path.Join(dir, entry.Name()), true
		}
	}
	return "", false
}

func isReadmeName(name string) bool {
	allowed := map[string]bool{"readme.md": true, "readme.markdown": true, "readme.mdown": true, "readme.mkd": true}
	return allowed[strings.ToLower(name)]
}

func buildSideMenu(r *http.Request, dir string, currentPath string, fsys fs.FS) *preview.SideMenu {
	menu := &preview.SideMenu{CurrentPath: displayDirPath(dir)}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		menu.Error = "Could not load directory menu."
		return menu
	}

	if !isRootDirPath(dir) {
		menu.Items = append(menu.Items, preview.SideMenuItem{
			Label: "../",
			Href:  sideMenuHref(r, "../"),
			Title: "../",
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	currentName := path.Base(currentPath)
	for _, entry := range entries {
		name := entry.Name()
		label := name
		href := sideMenuHref(r, name)
		if entry.IsDir() {
			label = name + "/"
			href = sideMenuHref(r, name+"/")
		}
		menu.Items = append(menu.Items, preview.SideMenuItem{
			Label:     label,
			Href:      href,
			Title:     label,
			IsDir:     entry.IsDir(),
			IsCurrent: !entry.IsDir() && name == currentName,
		})
	}
	return menu
}

func displayDirPath(dir string) string {
	clean := path.Clean(dir)
	if clean == "." || clean == "/" {
		return "/"
	}
	return "/" + strings.Trim(clean, "/") + "/"
}

func isRootDirPath(dir string) bool {
	clean := path.Clean(dir)
	return clean == "." || clean == "/"
}

func sideMenuHref(r *http.Request, href string) string {
	if r == nil || r.URL == nil || r.URL.RawQuery == "" {
		return href
	}
	return href + "?" + r.URL.RawQuery
}
