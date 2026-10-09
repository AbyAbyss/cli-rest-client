package clipboard

import (
	"errors"
	"testing"
)

func fake(t *testing.T, os string, env map[string]string, installed ...string) *[]string {
	t.Helper()
	oldGOOS, oldEnv, oldLook, oldRun := goos, getenv, lookPath, run
	t.Cleanup(func() { goos, getenv, lookPath, run = oldGOOS, oldEnv, oldLook, oldRun })
	goos = os
	getenv = func(k string) string { return env[k] }
	have := map[string]bool{}
	for _, n := range installed {
		have[n] = true
	}
	lookPath = func(n string) (string, error) {
		if have[n] {
			return "/usr/bin/" + n, nil
		}
		return "", errors.New("not found")
	}
	var calls []string
	run = func(name string, args []string, input string) error {
		calls = append(calls, name+":"+input)
		return nil
	}
	return &calls
}

func TestCopy(t *testing.T) {
	var osc []byte
	osc52 := func(b []byte) { osc = b }

	calls := fake(t, "darwin", nil, "pbcopy")
	if how, err := Copy("curl x", osc52); err != nil || how != "pbcopy" || (*calls)[0] != "pbcopy:curl x" {
		t.Fatalf("mac: %q %v %v", how, err, *calls)
	}

	calls = fake(t, "linux", map[string]string{"WAYLAND_DISPLAY": "w", "DISPLAY": ":0"}, "xclip")
	if how, _ := Copy("y", osc52); how != "xclip" {
		t.Fatalf("linux should fall back to xclip when wl-copy is missing: %q", how)
	}

	calls = fake(t, "darwin", map[string]string{"SSH_TTY": "/dev/pts/1"}, "pbcopy")
	osc = nil
	if how, _ := Copy("remote", osc52); how != "terminal (OSC 52)" || string(osc) != "remote" || len(*calls) != 0 {
		t.Fatalf("over SSH the terminal escape should be used: %q %v", how, *calls)
	}

	fake(t, "linux", nil)
	if how, _ := Copy("z", osc52); how != "terminal (OSC 52)" {
		t.Fatalf("no tools: %q", how)
	}
	if _, err := Copy("z", nil); err != ErrUnavailable {
		t.Fatalf("no tools and no terminal: %v", err)
	}
}
