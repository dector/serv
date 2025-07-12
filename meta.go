package main

import (
	_ "embed"
	"strings"

	"gopkg.in/yaml.v3"
)

var G Globals

type Globals struct {
	Version string
}

func (self *Globals) Init() {
	self.Version = getVersion()
}

//go:embed xx.yml
var metaYAML string

const DefaultPort = 8080

func getVersion() string {
	var meta struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(metaYAML), &meta); err != nil {
		panic("failed to fetch version")
	}
	return strings.TrimSpace(meta.Version)
}
