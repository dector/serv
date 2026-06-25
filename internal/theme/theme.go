package theme

// Colors groups the shared dark color palette used by HTML pages.
type Colors struct {
	Background string
	Panel      string
	Text       string
	Muted      string
	Border     string
	Accent     string
	Code       string
}

// Dark is the shared dark theme palette.
var Dark = Colors{
	Background: "#0d1117",
	Panel:      "#161b22",
	Text:       "#e6edf3",
	Muted:      "#8b949e",
	Border:     "#30363d",
	Accent:     "#58a6ff",
	Code:       "#010409",
}
