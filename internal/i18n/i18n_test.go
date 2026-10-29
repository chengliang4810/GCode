package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLanguageDefaultsAndFallback(t *testing.T) {
	for _, language := range []string{"", "zh", "zh-Hans", "zh-CN", "unsupported"} {
		if got := Normalize(language); got != Chinese {
			t.Errorf("Normalize(%q) = %q", language, got)
		}
	}
	if Normalize("en-US") != English || Normalize("EN") != English {
		t.Fatal("English variants must normalize")
	}
	if In("", "Settings") != "设置" || In(English, "Settings") != "Settings" {
		t.Fatal("wrong default or alternative")
	}
	if In(Chinese, "New untranslated message") != "New untranslated message" {
		t.Fatal("English fallback missing")
	}
	if got := In(Chinese, "Save changes to %s?", "Settings.go"); got != "保存对 Settings.go 的修改？" {
		t.Fatal(got)
	}
}

func TestLocalizedDatesAndDurations(t *testing.T) {
	at := time.Date(2026, 10, 29, 13, 5, 0, 0, time.UTC)
	if Date(Chinese, at, "Jan 2, 3:04 PM") != "10月29日 13:05" || Date(English, at, "Jan 2") != "Oct 29" {
		t.Fatal("date formatting changed the instant or failed to localize")
	}
	if Duration(Chinese, 65000) != "1分 5秒" || Duration(English, 65000) != "1m 5s" {
		t.Fatal("duration formatting failed")
	}
}

func TestCatalogPreservesFormatArguments(t *testing.T) {
	directive := regexp.MustCompile(`%\[?[0-9]*\]?[+# .0-9-]*[a-zA-Z%]`)
	for source, translated := range chinese {
		if strings.TrimSpace(translated) == "" {
			t.Errorf("empty translation: %q", source)
		}
		if !reflect.DeepEqual(directive.FindAllString(source, -1), directive.FindAllString(translated, -1)) {
			t.Errorf("format arguments changed: %q -> %q", source, translated)
		}
	}
}

// Keep literal interface call sites covered as the application grows. Business
// content is deliberately outside translation calls and outside this audit.
func TestLiteralInterfaceMessagesHaveTranslations(t *testing.T) {
	technical := map[string]bool{"Git": true, "GitHub": true, "SHA": true, "mermaid": true, "PR #%d · %s": true}
	letters := regexp.MustCompile(`[A-Za-z]{2}`)
	for _, directory := range []string{"../desktop", "../workbench"} {
		files, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") || strings.Contains(file.Name(), "fixture") {
				continue
			}
			name := filepath.Join(directory, file.Name())
			fs := token.NewFileSet()
			f, err := parser.ParseFile(fs, name, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			check := func(expression ast.Expr) {
				literal, ok := expression.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return
				}
				key, _ := strconv.Unquote(literal.Value)
				if letters.MatchString(key) && !technical[key] && chinese[key] == "" {
					t.Errorf("%s: missing translation %q", fs.Position(literal.Pos()), key)
				}
			}
			ast.Inspect(f, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				index := -1
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					if fun.Name == "tr" {
						index = 1
					}
				case *ast.SelectorExpr:
					if fun.Sel.Name == "t" {
						index = 0
					}
					if fun.Sel.Name == "settingsRow" && len(call.Args) > 2 {
						check(call.Args[1])
						check(call.Args[2])
					}
				}
				if index < 0 || len(call.Args) <= index {
					return true
				}
				literal, ok := call.Args[index].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				key, _ := strconv.Unquote(literal.Value)
				if letters.MatchString(key) && !technical[key] && chinese[key] == "" {
					t.Errorf("%s: missing translation %q", fs.Position(literal.Pos()), key)
				}
				return true
			})
		}
	}
}
