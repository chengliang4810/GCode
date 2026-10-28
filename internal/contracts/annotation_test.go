package contracts

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAnnotationBoundsAndCanonicalLink(t *testing.T) {
	a := Annotation{Index: 1, URL: "https://example.com", Title: "A page", Element: "button#save", Selector: "#save", Name: "保存", HTML: strings.Repeat("汉", 1000), Text: strings.Repeat("界", 100), Path: make([]string, 12), Styles: make([][2]string, 30)}
	for i := range a.Path {
		a.Path[i] = strings.Repeat("p", 300)
	}
	for i := range a.Styles {
		a.Styles[i] = [2]string{strings.Repeat("n", 100), strings.Repeat("v", 300)}
	}
	b := a.Bounded()
	if !utf8.ValidString(b.HTML) || !utf8.ValidString(b.Text) || len(b.HTML) > 2403 || len(b.Text) > 243 || len(b.Path) != 8 || len(b.Styles) != 24 {
		t.Fatal("annotation limits not applied")
	}
	if !reflect.DeepEqual(b, b.Bounded()) {
		t.Fatal("bounding must be idempotent")
	}
	b.Path[0] = "mutated"
	if a.Path[0] == "mutated" {
		t.Fatal("bounded annotation shares caller path")
	}
	link := a.Link()
	parsed, ok := ParseAnnotationLink(link)
	if !ok || !reflect.DeepEqual(parsed, a.Bounded()) {
		t.Fatal("canonical annotation did not round trip")
	}
	if _, ok := ParseAnnotationLink(strings.Replace(link, "Annotation 1", "Changed label", 1)); ok {
		t.Fatal("edited labels must remain ordinary text")
	}
	if _, ok := ParseAnnotationLink("[Annotation 1](zeron-annotation:xyz)"); ok {
		t.Fatal("malformed hexadecimal accepted")
	}
}

func TestCommandCloneOwnsAnnotationPayload(t *testing.T) {
	a := Annotation{Index: 1, Path: []string{"body", "button"}, Styles: [][2]string{{"color", "red"}}}
	cmd := Command{Kind: "message.send", Send: &SendRequest{Annotations: []Annotation{a}}}
	frozen := cmd.Clone()
	cmd.Send.Annotations[0].Path[0] = "mutated"
	cmd.Send.Annotations[0].Styles[0][1] = "blue"
	if frozen.Send.Annotations[0].Path[0] != "body" || frozen.Send.Annotations[0].Styles[0][1] != "red" {
		t.Fatal("annotation mutation crossed command queue ownership")
	}
}
