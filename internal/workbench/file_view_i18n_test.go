package workbench

import (
	"testing"

	"gcode/internal/contracts"
	localize "gcode/internal/i18n/view"

	"github.com/egoist/mygo/ui"
)

// The markdown toggle used to translate only its accessible label while the
// visible text kept the English literal, so the toolbar showed "Edit".
func TestMarkdownToggleShowsTranslatedText(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	var readme contracts.File
	for _, f := range snapshot.Files {
		if isMarkdown(f.Path) {
			readme = f
			break
		}
	}
	if readme.ID == "" {
		t.Fatal("markdown fixture missing")
	}
	s.OpenFile(readme.ID)
	d := s.documents[readme.ID]
	if d == nil {
		t.Fatal("markdown file did not open")
	}
	if !d.preview {
		t.Fatal("markdown file did not open in preview mode")
	}
	tt := ui.NewTester(func(c *ui.Context) { localize.Set(c, "zh-CN"); s.View(c) }, 960, 640)
	tt.Frame()
	if !tt.HasText("编辑") {
		t.Fatalf("preview toggle is not translated: %v", tt.Texts())
	}
	for _, text := range tt.Texts() {
		if text == "Edit" || text == "Preview" {
			t.Fatalf("toolbar kept the English literal %q: %v", text, tt.Texts())
		}
	}
	if err := tt.Click("编辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if d.preview {
		t.Fatal("toggle did not leave preview mode")
	}
	if !tt.HasText("预览") {
		t.Fatalf("edit toggle is not translated: %v", tt.Texts())
	}
}
