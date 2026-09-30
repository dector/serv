package listen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"dector.space/go/nettw"
)

func TestPortSeedRepo(t *testing.T) {
	for _, markerFile := range []bool{false, true} {
		name := "git-directory"
		if markerFile {
			name = "git-file"
		}
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			marker := filepath.Join(repo, ".git")
			if markerFile {
				if err := os.WriteFile(marker, []byte("gitdir: elsewhere\n"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(marker, 0700); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(repo, "web", "public")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "index.html")
			if err := os.WriteFile(file, nil, 0600); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{repo, dir, file} {
				t.Run(path, func(t *testing.T) {
					rel, err := filepath.Rel(repo, path)
					if err != nil {
						t.Fatal(err)
					}
					assertGroupedSeed(t, path, repo, rel)
				})
			}
			// The nearest repository wins over its parent repository.
			if err := os.Mkdir(filepath.Join(dir, ".git"), 0700); err != nil {
				t.Fatal(err)
			}
			assertGroupedSeed(t, file, dir, "index.html")
		})
	}
}

func TestPortSeedRelativePath(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	assertGroupedSeed(t, filepath.Join("web", "..", "public"), repo, "public")
}

func TestPortSeedOutsideRepo(t *testing.T) {
	// Ensure this path has no repo marker among its parents.
	path := t.TempDir()
	for dir := path; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			t.Skip("temporary directory is inside a repo")
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	t.Chdir(path)
	for _, seed := range []string{path, ".", "index.html"} {
		var args nettw.ParsePortArgs
		portSeed(seed)(&args)
		if args.Seed != seed || args.SeedSegments != nil {
			t.Fatalf("seed %q: got %#v, want unchanged single-path seed", seed, args)
		}
	}
}

func assertGroupedSeed(t *testing.T, path, repo, rel string) {
	t.Helper()
	var args nettw.ParsePortArgs
	portSeed(path)(&args)
	want := []nettw.SeedSegment{{Seed: repo}, {Seed: rel, Digits: 2}}
	if !reflect.DeepEqual(args.SeedSegments, want) {
		t.Fatalf("portSeed(%q) segments = %#v, want %#v", path, args.SeedSegments, want)
	}
}
