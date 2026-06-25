package pages

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dector/serv/fs"
	"github.com/dector/serv/internal/theme"
)

func TestGenerateFolderPageUsesDarkTheme(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got := GenerateFolderPage(&fs.FsNode{Path: dir}, ".", "test")

	checks := []string{
		"color-scheme: dark",
		"--bg: " + theme.Dark.Background,
		"--text: " + theme.Dark.Text,
		"--accent: " + theme.Dark.Accent,
		"file.txt",
	}
	for _, check := range checks {
		if !bytes.Contains(got, []byte(check)) {
			t.Fatalf("folder page missing %q:\n%s", check, got)
		}
	}
}
