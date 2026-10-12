package workspace

import "testing"

func TestEditorCommandPreservesCommandArgumentsWithFilePlaceholder(t *testing.T) {
	command, args, err := editorCommand(EditorOptions{
		Command: "code --wait",
		Args:    []string{"--reuse-window", "{file}"},
	}, "/tmp/project/file with spaces.go")
	if err != nil {
		t.Fatal(err)
	}
	if command != "code" {
		t.Fatalf("command = %q, want code", command)
	}
	want := []string{"--wait", "--reuse-window", "/tmp/project/file with spaces.go"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for index := range want {
		if args[index] != want[index] {
			t.Fatalf("args[%d] = %q, want %q; all = %#v", index, args[index], want[index], args)
		}
	}
}

func TestEditorCommandAppendsFileWhenPlaceholderIsAbsent(t *testing.T) {
	command, args, err := editorCommand(EditorOptions{
		Command: "zed --wait",
		Args:    []string{"--foreground"},
	}, "/tmp/file.go")
	if err != nil {
		t.Fatal(err)
	}
	if command != "zed" {
		t.Fatalf("command = %q, want zed", command)
	}
	want := []string{"--wait", "--foreground", "/tmp/file.go"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for index := range want {
		if args[index] != want[index] {
			t.Fatalf("args[%d] = %q, want %q; all = %#v", index, args[index], want[index], args)
		}
	}
}
