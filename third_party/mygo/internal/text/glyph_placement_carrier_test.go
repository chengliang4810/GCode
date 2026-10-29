package text

import (
	"reflect"
	"testing"
)

func TestQuarterRasterModeCacheCarriersAreBuiltinByte(t *testing.T) {
	for _, ty := range []reflect.Type{reflect.TypeFor[glyphKey](), reflect.TypeFor[placeKey]()} {
		field, ok := ty.FieldByName("mode")
		if !ok || field.Type != reflect.TypeFor[uint8]() {
			t.Fatalf("%v.mode uses %v, want the existing builtin uint8 identity", ty, field.Type)
		}
		if _, old := ty.FieldByName("policy"); old {
			t.Fatalf("%v retained the extra policy field", ty)
		}
		if _, old := ty.FieldByName("subpixel"); old {
			t.Fatalf("%v retained the replaced bool field", ty)
		}
	}
	if nativeNonSubpixel != 0 || nativeSubpixel != 1 || quarterSubpixel != 2 {
		t.Fatal("normalized raster mode identities changed")
	}
	if reflect.TypeFor[GlyphPlacement]() != reflect.TypeFor[uint8]() {
		t.Fatal("internal carrier retained a distinct enum identity")
	}
	if GlyphPlacementNative != 0 || GlyphPlacementWindowsClearTypeQuarter != 1 {
		t.Fatal("public placement policy numeric identities changed")
	}
}
