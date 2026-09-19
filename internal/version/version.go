package version

import (
	_ "embed"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultPort is the HTTP port used when none is requested.
const DefaultPort = 8080

// Version is the semver string reported by the CLI. Call Init before use.
var Version string

//go:embed xx.yml
var metaYAML string

var versionOverride string

// Init loads the version from the embedded manifest unless a build-time
// override was injected with -ldflags.
func Init() {
	Version = getVersion()
}

func getVersion() string {
	if version := strings.TrimSpace(versionOverride); version != "" {
		return version
	}
	var meta struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(metaYAML), &meta); err != nil {
		panic("failed to fetch version")
	}
	return strings.TrimSpace(meta.Version)
}
