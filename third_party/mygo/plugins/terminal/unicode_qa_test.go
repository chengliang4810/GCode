package terminal

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

func TestUnicodeQA(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell and PTY")
	}
	loadLib(t)
	t.Setenv("LANG", "C.UTF-8")
	t.Setenv("LC_ALL", "C.UTF-8")
	t.Setenv("LC_CTYPE", "C.UTF-8")
	childEnv := environment([]string{"PS1=", "ENV="})
	localeValue := func(key string) string {
		for _, kv := range childEnv {
			if k, v, ok := strings.Cut(kv, "="); ok && k == key {
				return v
			}
		}
		return ""
	}
	t.Logf("child_locale LANG=%q LC_ALL=%q LC_CTYPE=%q", localeValue("LANG"), localeValue("LC_ALL"), localeValue("LC_CTYPE"))
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	input := "DIRECT=中文|世界|😀|👍🏽|👨‍👩‍👧|é́"
	term.Feed([]byte(input + "\r\n"))
	if term.Text() != input || !utf8.ValidString(term.Text()) {
		t.Fatalf("direct_text=%q bytes=% x", term.Text(), []byte(term.Text()))
	}

	shell := shell(t, Options{Command: []string{"/bin/sh", "-i"}})
	tt := ui.NewTester(func(c *ui.Context) { View(c, shell).Fill().AutoFocus() }, 640, 360)
	waitFor(t, tt, "shell prompt", func() bool { return shell.Text() != "" })
	shell.Send([]byte("printf 'SHELL=中文|世界|😀|👍🏽|👨‍👩‍👧|é́|SHELL_DONE\\n'\r"))
	waitFor(t, tt, "shell unicode output", func() bool { return strings.Contains(shell.Text(), "SHELL_DONE") })
	expected := "SHELL=中文|世界|😀|👍🏽|👨‍👩‍👧|é́|SHELL_DONE"
	if !utf8.ValidString(shell.Text()) || !strings.Contains(shell.Text(), expected) {
		t.Fatalf("shell_text=%q bytes=% x", shell.Text(), []byte(shell.Text()))
	}
}

func TestEnvironmentDarwinLocaleNormalization(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Darwin locale normalization is not applicable")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin locale normalization is not applicable")
	}

	value := func(env []string, key string) string {
		for _, kv := range env {
			if k, v, ok := strings.Cut(kv, "="); ok && k == key {
				return v
			}
		}
		return ""
	}
	check := func(name string, parent map[string]string, extra []string, want map[string]string) {
		t.Run(name, func(t *testing.T) {
			for _, key := range []string{"LANG", "LC_ALL", "LC_CTYPE"} {
				t.Setenv(key, parent[key])
			}
			env := environment(extra)
			for key, expected := range want {
				if got := value(env, key); got != expected {
					t.Errorf("%s=%q, want %q", key, got, expected)
				}
			}
		})
	}

	check("LANG C.UTF8", map[string]string{"LANG": "C.UTF8", "LC_ALL": "", "LC_CTYPE": ""}, nil,
		map[string]string{"LANG": "en_US.UTF-8", "LC_ALL": "", "LC_CTYPE": ""})
	check("LANG C.UTF-8", map[string]string{"LANG": "C.UTF-8", "LC_ALL": "", "LC_CTYPE": ""}, nil,
		map[string]string{"LANG": "en_US.UTF-8", "LC_ALL": "", "LC_CTYPE": ""})
	check("explicit LANG", map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "", "LC_CTYPE": ""}, nil,
		map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "", "LC_CTYPE": ""})
	check("LC_ALL wins", map[string]string{"LANG": "C.UTF-8", "LC_ALL": "fr_FR.UTF-8", "LC_CTYPE": "C.UTF-8"}, nil,
		map[string]string{"LANG": "C.UTF-8", "LC_ALL": "fr_FR.UTF-8", "LC_CTYPE": "C.UTF-8"})
	check("LC_CTYPE wins over LANG", map[string]string{"LANG": "C.UTF-8", "LC_ALL": "", "LC_CTYPE": "de_DE.UTF-8"}, nil,
		map[string]string{"LANG": "C.UTF-8", "LC_ALL": "", "LC_CTYPE": "de_DE.UTF-8"})
	check("normalize LC_ALL", map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "C.UTF8", "LC_CTYPE": "de_DE.UTF-8"}, nil,
		map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "en_US.UTF-8", "LC_CTYPE": "de_DE.UTF-8"})
	check("normalize LC_CTYPE", map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "", "LC_CTYPE": "C.UTF-8"}, nil,
		map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "", "LC_CTYPE": "en_US.UTF-8"})
	check("extra LANG overrides parent", map[string]string{"LANG": "C.UTF-8", "LC_ALL": "", "LC_CTYPE": ""}, []string{"LANG=ja_JP.UTF-8"},
		map[string]string{"LANG": "ja_JP.UTF-8", "LC_ALL": "", "LC_CTYPE": ""})
	check("extra LC_ALL overrides parent", map[string]string{"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8", "LC_CTYPE": ""}, []string{"LC_ALL=fr_FR.UTF-8"},
		map[string]string{"LANG": "C.UTF-8", "LC_ALL": "fr_FR.UTF-8", "LC_CTYPE": ""})
}

func TestUnicodeViewSelectionAndResize(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	input := "中文|世界|😀|👍🏽|👨‍👩‍👧|é́"
	term.Feed([]byte(input + "\r\nsecond"))
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 640, 360)
	tt.Frame()
	tt.Command("selectAll")
	tt.Command("copy")
	if got := tt.Clipboard(); got != input+"\nsecond" {
		t.Fatalf("view copy=%q want=%q", got, input+"\nsecond")
	}

	term.Resize(40, 6)
	tt.Frame()
	if got := term.Text(); !strings.Contains(got, input) {
		t.Fatalf("resize lost Unicode text: %q", got)
	}
	term.Feed([]byte("\r\nRESIZE_DONE"))
	if got := term.Text(); !strings.Contains(got, input) || !strings.Contains(got, "RESIZE_DONE") {
		t.Fatalf("resize text=%q", got)
	}
}
