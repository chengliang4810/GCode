package contracts

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Annotation follows BrowserAnnotation in the v0.2.128 reference protocol.
type Annotation struct {
	Index    uint32      `json:"index"`
	URL      string      `json:"url"`
	Title    string      `json:"title"`
	Element  string      `json:"element"`
	Selector string      `json:"selector"`
	Path     []string    `json:"path"`
	Role     string      `json:"role"`
	Name     string      `json:"name"`
	Text     string      `json:"text"`
	HTML     string      `json:"html"`
	Rect     [4]int      `json:"rect"`
	Viewport [2]int      `json:"viewport"`
	Styles   [][2]string `json:"styles"`
}
type BrowserAnnotation = Annotation

func (a Annotation) Label() string { return fmt.Sprintf("Annotation %d", a.Index) }

// Bounded copies and clips UTF-8 fields to the source's byte caps.
func (a Annotation) Bounded() Annotation {
	a.HTML = clipAnnotation(a.HTML, 2400)
	a.Text = clipAnnotation(a.Text, 240)
	a.URL = clipAnnotation(a.URL, 2048)
	a.Title = clipAnnotation(a.Title, 240)
	a.Element = clipAnnotation(a.Element, 240)
	a.Role = clipAnnotation(a.Role, 240)
	a.Name = clipAnnotation(a.Name, 240)
	a.Selector = clipAnnotation(a.Selector, 480)
	a.Path = slices.Clone(a.Path[:min(len(a.Path), 8)])
	for i := range a.Path {
		a.Path[i] = clipAnnotation(a.Path[i], 240)
	}
	a.Styles = slices.Clone(a.Styles[:min(len(a.Styles), 24)])
	for i := range a.Styles {
		a.Styles[i][0] = clipAnnotation(a.Styles[i][0], 64)
		a.Styles[i][1] = clipAnnotation(a.Styles[i][1], 240)
	}
	return a
}

func clipAnnotation(v string, maxBytes int) string {
	if len(v) <= maxBytes {
		return v
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(v[end]) {
		end--
	}
	return v[:end] + "…"
}

// Link is the editable canonical Markdown annotation token, with page HTML
// encoded as data, never inserted into the native application's markup.
func (a Annotation) Link() string {
	a = a.Bounded()
	if a.Path == nil {
		a.Path = []string{}
	}
	if a.Styles == nil {
		a.Styles = [][2]string{}
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(a); err != nil {
		panic(err)
	}
	payload := strings.TrimSuffix(buf.String(), "\n")
	return "[" + a.Label() + "](rcode-annotation:" + hex.EncodeToString([]byte(payload)) + ")"
}

func ParseAnnotationLink(token string) (Annotation, bool) {
	var a Annotation
	p := strings.Index(token, "](rcode-annotation:")
	if p < 0 || !strings.HasSuffix(token, ")") {
		return a, false
	}
	encoded := token[p+len("](rcode-annotation:") : len(token)-1]
	if len(encoded) > 256*1024 {
		return a, false
	}
	data, err := hex.DecodeString(encoded)
	if err != nil || json.Unmarshal(data, &a) != nil {
		return Annotation{}, false
	}
	a = a.Bounded()
	return a, a.Link() == token
}
