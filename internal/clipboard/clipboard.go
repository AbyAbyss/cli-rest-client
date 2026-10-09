// Package clipboard copies text to the system clipboard.
package clipboard

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Hooks for tests.
var (
	goos     = runtime.GOOS
	getenv   = os.Getenv
	lookPath = exec.LookPath
	run      = func(name string, args []string, input string) error {
		cmd := exec.Command(name, args...)
		cmd.Stdin = strings.NewReader(input)
		return cmd.Run()
	}
)

type tool struct {
	name string
	args []string
}

// tools lists the clipboard programs to try, best first.
func tools() []tool {
	switch goos {
	case "darwin":
		return []tool{{"pbcopy", nil}}
	case "windows":
		return []tool{{"clip", nil}}
	}
	var ts []tool
	if getenv("WAYLAND_DISPLAY") != "" {
		ts = append(ts, tool{"wl-copy", nil})
	}
	if getenv("DISPLAY") != "" {
		ts = append(ts, tool{"xclip", []string{"-selection", "clipboard"}}, tool{"xsel", []string{"--clipboard", "--input"}})
	}
	// WSL: the Windows clipboard.
	ts = append(ts, tool{"clip.exe", nil})
	return ts
}

// ErrUnavailable means no way to reach the clipboard was found.
var ErrUnavailable = errors.New("no clipboard available (install xclip, xsel or wl-copy, or use a terminal with OSC 52 support)")

// Copy puts text on the clipboard and says how. Over SSH the local tools
// would copy on the remote machine, so the terminal escape (osc52, which
// may be nil) is tried first there; otherwise it is the fallback.
func Copy(text string, osc52 func([]byte)) (string, error) {
	remote := getenv("SSH_TTY") != "" || getenv("SSH_CONNECTION") != ""
	if remote && osc52 != nil {
		osc52([]byte(text))
		return "terminal (OSC 52)", nil
	}
	for _, t := range tools() {
		if _, err := lookPath(t.name); err != nil {
			continue
		}
		if err := run(t.name, t.args, text); err == nil {
			return t.name, nil
		}
	}
	if osc52 != nil {
		osc52([]byte(text))
		return "terminal (OSC 52)", nil
	}
	return "", ErrUnavailable
}
