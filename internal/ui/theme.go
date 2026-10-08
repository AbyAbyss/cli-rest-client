package ui

import "github.com/gdamore/tcell/v2"

// Theme is a color scheme. Hex fields are used inside tview color tags.
type Theme struct {
	Name string

	Background tcell.Color
	Input      tcell.Color
	Border     tcell.Color
	Focus      tcell.Color
	Title      tcell.Color
	Text       tcell.Color
	Muted      tcell.Color
	Button     tcell.Color
	ButtonText tcell.Color
	Selection  tcell.Color

	HexText    string
	HexMuted   string
	HexAccent  string
	HexSuccess string
	HexWarning string
	HexError   string
	HexInfo    string

	// Method colors.
	HexGet, HexPost, HexPut, HexPatch, HexDelete, HexOther string

	// JSON syntax colors.
	HexJSONKey, HexJSONString, HexJSONNumber, HexJSONLiteral string
}

var themes = []*Theme{
	{
		Name:       "Catppuccin Mocha",
		Background: tcell.NewHexColor(0x1e1e2e),
		Input:      tcell.NewHexColor(0x313244),
		Border:     tcell.NewHexColor(0x585b70),
		Focus:      tcell.NewHexColor(0xf5c2e7),
		Title:      tcell.NewHexColor(0x89b4fa),
		Text:       tcell.NewHexColor(0xcdd6f4),
		Muted:      tcell.NewHexColor(0xa6adc8),
		Button:     tcell.NewHexColor(0xcba6f7),
		ButtonText: tcell.NewHexColor(0x1e1e2e),
		Selection:  tcell.NewHexColor(0x45475a),
		HexText:    "#cdd6f4", HexMuted: "#7f849c", HexAccent: "#89b4fa",
		HexSuccess: "#a6e3a1", HexWarning: "#f9e2af", HexError: "#f38ba8", HexInfo: "#94e2d5",
		HexGet: "#a6e3a1", HexPost: "#f9e2af", HexPut: "#89b4fa", HexPatch: "#cba6f7", HexDelete: "#f38ba8", HexOther: "#94e2d5",
		HexJSONKey: "#89b4fa", HexJSONString: "#a6e3a1", HexJSONNumber: "#fab387", HexJSONLiteral: "#cba6f7",
	},
	{
		Name:       "Original",
		Background: tcell.NewRGBColor(15, 17, 26),
		Input:      tcell.NewRGBColor(30, 33, 46),
		Border:     tcell.NewRGBColor(60, 63, 81),
		Focus:      tcell.NewRGBColor(88, 166, 255),
		Title:      tcell.NewRGBColor(88, 166, 255),
		Text:       tcell.ColorWhite,
		Muted:      tcell.ColorLightGray,
		Button:     tcell.NewRGBColor(88, 166, 255),
		ButtonText: tcell.ColorBlack,
		Selection:  tcell.NewRGBColor(40, 60, 90),
		HexText:    "#ffffff", HexMuted: "#8b8fa3", HexAccent: "#58a6ff",
		HexSuccess: "#50fa7b", HexWarning: "#ffb86c", HexError: "#ff5555", HexInfo: "#8be9fd",
		HexGet: "#50fa7b", HexPost: "#ffb86c", HexPut: "#58a6ff", HexPatch: "#bd93f9", HexDelete: "#ff5555", HexOther: "#8be9fd",
		HexJSONKey: "#58a6ff", HexJSONString: "#50fa7b", HexJSONNumber: "#ffb86c", HexJSONLiteral: "#bd93f9",
	},
	{
		Name:       "Gruvbox Dark",
		Background: tcell.NewHexColor(0x282828),
		Input:      tcell.NewHexColor(0x3c3836),
		Border:     tcell.NewHexColor(0x665c54),
		Focus:      tcell.NewHexColor(0xfabd2f),
		Title:      tcell.NewHexColor(0x83a598),
		Text:       tcell.NewHexColor(0xebdbb2),
		Muted:      tcell.NewHexColor(0xa89984),
		Button:     tcell.NewHexColor(0xfabd2f),
		ButtonText: tcell.NewHexColor(0x282828),
		Selection:  tcell.NewHexColor(0x504945),
		HexText:    "#ebdbb2", HexMuted: "#928374", HexAccent: "#83a598",
		HexSuccess: "#b8bb26", HexWarning: "#fabd2f", HexError: "#fb4934", HexInfo: "#8ec07c",
		HexGet: "#b8bb26", HexPost: "#fabd2f", HexPut: "#83a598", HexPatch: "#d3869b", HexDelete: "#fb4934", HexOther: "#8ec07c",
		HexJSONKey: "#83a598", HexJSONString: "#b8bb26", HexJSONNumber: "#fe8019", HexJSONLiteral: "#d3869b",
	},
	{
		Name:       "Light",
		Background: tcell.NewHexColor(0xfafafa),
		Input:      tcell.NewHexColor(0xe8e8ec),
		Border:     tcell.NewHexColor(0xb0b0b8),
		Focus:      tcell.NewHexColor(0x7c3aed),
		Title:      tcell.NewHexColor(0x2563eb),
		Text:       tcell.NewHexColor(0x1f2328),
		Muted:      tcell.NewHexColor(0x57606a),
		Button:     tcell.NewHexColor(0x7c3aed),
		ButtonText: tcell.NewHexColor(0xffffff),
		Selection:  tcell.NewHexColor(0xddd6fe),
		HexText:    "#1f2328", HexMuted: "#6e7781", HexAccent: "#2563eb",
		HexSuccess: "#1a7f37", HexWarning: "#9a6700", HexError: "#cf222e", HexInfo: "#0969da",
		HexGet: "#1a7f37", HexPost: "#9a6700", HexPut: "#0969da", HexPatch: "#8250df", HexDelete: "#cf222e", HexOther: "#1b7c83",
		HexJSONKey: "#0550ae", HexJSONString: "#1a7f37", HexJSONNumber: "#953800", HexJSONLiteral: "#8250df",
	},
}

// ThemeNames lists available theme names.
func ThemeNames() []string {
	names := make([]string, len(themes))
	for i, t := range themes {
		names[i] = t.Name
	}
	return names
}

func themeByName(name string) (*Theme, int) {
	for i, t := range themes {
		if t.Name == name {
			return t, i
		}
	}
	return themes[0], 0
}

func (t *Theme) methodColor(method string) string {
	switch method {
	case "GET":
		return t.HexGet
	case "POST":
		return t.HexPost
	case "PUT":
		return t.HexPut
	case "PATCH":
		return t.HexPatch
	case "DELETE":
		return t.HexDelete
	}
	return t.HexOther
}
