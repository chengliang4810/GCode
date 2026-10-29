package ui

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"testing"
	"unsafe"
)

// TestGlyphPlacementTextStyleOffsets is an explicit measurement entry point.
// Preparation does not run it. Offsets are actual compiler results when run.
func TestGlyphPlacementTextStyleOffsets(t *testing.T) {
	fields := []map[string]any{}
	ty := reflect.TypeFor[textStyle]()
	for i := range ty.NumField() {
		field := ty.Field(i)
		fields = append(fields, map[string]any{"name": field.Name, "type": field.Type.String(), "offset": field.Offset, "size": field.Type.Size(), "align": field.Type.Align()})
	}
	result := map[string]any{
		"GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH,
		"textStyleBytes": unsafe.Sizeof(textStyle{}), "textStyleAlign": unsafe.Alignof(textStyle{}),
		"nodeBytes": unsafe.Sizeof(node{}), "nodeAlign": unsafe.Alignof(node{}), "nodeTextStyleOffset": unsafe.Offsetof(node{}.ts),
		"weightOffset": unsafe.Offsetof(textStyle{}.weight), "sizeOffset": unsafe.Offsetof(textStyle{}.size),
		"colorBytes": unsafe.Sizeof(Color{}), "colorAlign": unsafe.Alignof(Color{}),
		"alignBytes": unsafe.Sizeof(Align(0)), "boolBytes": unsafe.Sizeof(false), "intBytes": unsafe.Sizeof(int(0)),
		"funcBytes": unsafe.Sizeof(glyphPlacementPaint(nil)), "fields": fields,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
	if path := os.Getenv("QUARTER_LAYOUT_PROBE_OUT"); path != "" {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
