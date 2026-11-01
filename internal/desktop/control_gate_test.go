package desktop

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The gate protects the boundary between settings page layout and shared
// controls. New controls and visual variants belong in settings_controls.go.
func TestSettingsControlsUseSharedDefinitions(t *testing.T) {
	files, err := filepath.Glob("settings*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no settings source files found")
	}
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") || filename == "settings_controls.go" {
			continue
		}
		source, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		violations, err := inspectSettingsControls(filename, source)
		if err != nil {
			t.Fatal(err)
		}
		for _, violation := range violations {
			t.Errorf("%s: %s", violation.position, violation.message)
		}
	}
}

type settingsControlViolation struct {
	position token.Position
	message  string
}

type settingsControlOrigin struct {
	control     bool
	textArea    bool
	selection   bool
	pageFrame   bool
	constructor string
}

type settingsControlInspector struct {
	uiPackages map[string]bool
	bindings   map[*ast.Object]settingsControlOrigin
}

func inspectSettingsControls(filename string, source []byte) ([]settingsControlViolation, error) {
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, filename, source, 0)
	if err != nil {
		return nil, err
	}
	inspector := settingsControlInspector{
		uiPackages: map[string]bool{},
		bindings:   map[*ast.Object]settingsControlOrigin{},
	}
	violations := []settingsControlViolation{}
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil || path != "github.com/egoist/mygo/ui" {
			continue
		}
		name := "ui"
		if imported.Name != nil {
			name = imported.Name.Name
		}
		if name == "." {
			violations = append(violations, settingsControlViolation{
				position: files.Position(imported.Pos()),
				message:  "dot-importing MyGo UI bypasses the shared-control boundary",
			})
		}
		inspector.uiPackages[name] = true
	}

	// Resolve local aliases to a fixed point, including aliases captured by
	// callbacks. Keep provenance conservatively when a variable is reassigned:
	// a conditional assignment must not hide a control from the gate.
	for changed := true; changed; {
		changed = false
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.AssignStmt:
				if len(node.Lhs) == len(node.Rhs) {
					for index, left := range node.Lhs {
						changed = inspector.bind(left, node.Rhs[index]) || changed
					}
				}
			case *ast.ValueSpec:
				if len(node.Names) == len(node.Values) {
					for index, name := range node.Names {
						changed = inspector.bind(name, node.Values[index]) || changed
					}
				}
			case *ast.CallExpr:
				selector, ok := node.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Popup" || !inspector.origin(selector.X).selection {
					break
				}
				for _, argument := range node.Args {
					callback, ok := argument.(*ast.FuncLit)
					if !ok || callback.Type.Params == nil || len(callback.Type.Params.List) == 0 {
						continue
					}
					for _, parameter := range callback.Type.Params.List[0].Names {
						changed = inspector.bindOrigin(parameter, settingsControlOrigin{control: true}) || changed
					}
				}
			}
			return true
		})
	}

	wrappedSelectItems := map[*ast.CallExpr]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || settingsControlFunctionName(call.Fun) != "controlSelectItem" {
			return true
		}
		for _, argument := range call.Args {
			for {
				parentheses, ok := argument.(*ast.ParenExpr)
				if !ok {
					break
				}
				argument = parentheses.X
			}
			if item, ok := argument.(*ast.CallExpr); ok {
				wrappedSelectItems[item] = true
			}
		}
		return true
	})

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		function := inspector.origin(call.Fun)
		if function.constructor != "" {
			violations = append(violations, settingsControlViolation{
				position: files.Position(call.Pos()),
				message: fmt.Sprintf(
					"ui.%s must be created through a shared settings control",
					function.constructor,
				),
			})
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver := inspector.origin(selector.X)
		if receiver.selection && selector.Sel.Name == "Item" && !wrappedSelectItems[call] {
			violations = append(violations, settingsControlViolation{
				position: files.Position(call.Pos()),
				message:  "Select.Item must be passed directly to controlSelectItem",
			})
		}
		pageOverride := receiver.pageFrame && settingsPageFrameStyleMethod(selector.Sel.Name)
		if receiver.control && (settingsControlStyleMethod(selector.Sel.Name, receiver.textArea) || pageOverride) {
			violations = append(violations, settingsControlViolation{
				position: files.Position(selector.Sel.Pos()),
				message: fmt.Sprintf(
					"%s overrides a shared control; define a shared variant instead",
					selector.Sel.Name,
				),
			})
		}
		return true
	})
	sort.Slice(violations, func(left, right int) bool {
		return violations[left].position.Offset < violations[right].position.Offset
	})
	return violations, nil
}

func (inspector *settingsControlInspector) bind(left, right ast.Expr) bool {
	return inspector.bindOrigin(left, inspector.origin(right))
}

func (inspector *settingsControlInspector) bindOrigin(left ast.Expr, after settingsControlOrigin) bool {
	name, ok := left.(*ast.Ident)
	if !ok || name.Obj == nil {
		return false
	}
	before := inspector.bindings[name.Obj]
	if before.control && after.control {
		after.textArea = after.textArea && before.textArea
	} else if before.control {
		after.textArea = before.textArea
	}
	after.control = after.control || before.control
	after.selection = after.selection || before.selection
	after.pageFrame = after.pageFrame || before.pageFrame
	if before.constructor != "" {
		after.constructor = before.constructor
	}
	if after == before {
		return false
	}
	inspector.bindings[name.Obj] = after
	return true
}

func (inspector *settingsControlInspector) origin(expression ast.Expr) settingsControlOrigin {
	switch expression := expression.(type) {
	case *ast.ParenExpr:
		return inspector.origin(expression.X)
	case *ast.Ident:
		if expression.Obj != nil {
			return inspector.bindings[expression.Obj]
		}
		return settingsSharedControlOrigin(expression.Name)
	case *ast.SelectorExpr:
		if namespace, ok := expression.X.(*ast.Ident); ok &&
			namespace.Obj == nil && inspector.uiPackages[namespace.Name] {
			switch expression.Sel.Name {
			case "ButtonBase", "Button", "PrimaryButton", "TextInput", "TextInputBase",
				"TextArea", "TextAreaBase", "Checkbox", "CheckboxBase",
				"Switch", "SwitchBase", "Radio", "RadioBase", "MenuButton", "SelectBase",
				"Slider", "StepSlider", "SliderBase", "Toggle", "ToggleBase":
				return settingsControlOrigin{
					control:     true,
					textArea:    strings.HasPrefix(expression.Sel.Name, "TextArea"),
					selection:   expression.Sel.Name == "SelectBase",
					constructor: expression.Sel.Name,
				}
			}
		}
		if shared := settingsSharedControlOrigin(expression.Sel.Name); shared.control {
			return shared
		}
		result := inspector.origin(expression.X)
		result.constructor = ""
		return result
	case *ast.CallExpr:
		result := inspector.origin(expression.Fun)
		result.constructor = ""
		return result
	}
	return settingsControlOrigin{}
}

func settingsSharedControlOrigin(name string) settingsControlOrigin {
	switch name {
	case "settingsPageViewport", "settingsPageContent", "settingsPageHeading":
		return settingsControlOrigin{control: true, pageFrame: true}
	case "textButton", "iconButton", "updateIconButton", "usageIconButton", "primaryButton", "secondaryButton",
		"dangerButton", "buttonWithIcon":
		return settingsControlOrigin{control: true}
	}
	if strings.HasPrefix(name, "control") {
		return settingsControlOrigin{
			control:   true,
			textArea:  name == "controlTextArea",
			selection: name == "controlSelect",
		}
	}
	return settingsControlOrigin{}
}

func settingsControlFunctionName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		return expression.Sel.Name
	}
	return ""
}

func settingsControlStyleMethod(name string, textArea bool) bool {
	for _, prefix := range []string{"Padding", "Border", "Font", "Gap", "Draw"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	switch name {
	case "Height", "HeightPercent", "MinHeight", "MaxHeight":
		return !textArea
	case "Size", "Radius", "Background", "TextColor", "Opacity", "Shadow", "Outline":
		return true
	}
	return false
}

func settingsPageFrameStyleMethod(name string) bool {
	if strings.HasPrefix(name, "Margin") {
		return true
	}
	switch name {
	case "Width", "WidthPercent", "MinWidth", "MaxWidth", "FillWidth", "FillHeight",
		"AlignSelf", "AlignItems", "Justify", "Grow", "Shrink":
		return true
	}
	return false
}

func TestInspectSettingsControls(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		source     string
		violations int
	}{
		{
			name:       "page frame cannot override shared width",
			source:     `func page() { settingsPageContent(c).MaxWidth(1720) }`,
			violations: 1,
		},
		{
			name:       "page frame cannot override shared margin",
			source:     `func page() { frame := settingsPageContent(c); frame.Margin(32) }`,
			violations: 1,
		},
		{
			name:       "page heading cannot override shared typography",
			source:     `func page() { settingsPageHeading(c, "Model settings").FontSize(30) }`,
			violations: 1,
		},
		{
			name:       "slider must use shared constructor",
			source:     `func page() { native.StepSlider(c, &value, 0, 1, .1) }`,
			violations: 1,
		},
		{
			name:       "toggle must use shared constructor",
			source:     `func page() { native.Toggle(c, &value, "Toggle") }`,
			violations: 1,
		},
		{
			name:       "direct constructor with import alias",
			source:     `func page() { native.ButtonBase(c) }`,
			violations: 1,
		},
		{
			name:       "fluent control override",
			source:     `func page() { textButton(c, "Save").FillWidth().Height(48) }`,
			violations: 1,
		},
		{
			name:       "variable override",
			source:     `func page() { b := primaryButton(c, "Save"); b.Background(red) }`,
			violations: 1,
		},
		{
			name:       "variable alias override",
			source:     `func page() { b := iconButton(c, "eye", "Show"); alias := b; alias.Size(40, 40) }`,
			violations: 1,
		},
		{
			name:       "constructor function alias",
			source:     `func page() { constructor := native.TextInput; constructor(c, &value) }`,
			violations: 1,
		},
		{
			name: "constructor alias reassignments terminate",
			source: `func page() {
				constructor := native.TextInput; constructor = native.ButtonBase
				constructor(c)
			}`,
			violations: 1,
		},
		{
			name:       "shared function alias",
			source:     `func page() { constructor := controlTextInput; b := constructor(c, &value); b.Radius(4) }`,
			violations: 1,
		},
		{
			name:       "selection trigger override",
			source:     `func page() { sel := controlSelect(c, &value); sel.Trigger.Padding(8, 12) }`,
			violations: 1,
		},
		{
			name:       "selection trigger alias override",
			source:     `func page() { sel := controlSelect(c, &value); trigger := sel.Trigger; trigger.FontSize(15) }`,
			violations: 1,
		},
		{
			name: "selection popup parameter style override",
			source: `func page() {
				sel := controlSelect(c, &value)
				sel.Popup(func(panel native.Element) { panel.Padding(8).Background(red) })
			}`,
			violations: 2,
		},
		{
			name: "selection popup parameter alias override",
			source: `func page() {
				sel := controlSelect(c, &value); alias := sel
				alias.Popup(func(panel native.Element) { body := panel; body.Radius(8) })
			}`,
			violations: 1,
		},
		{
			name: "selection item requires shared style",
			source: `func page() {
				sel := controlSelect(c, &value)
				sel.Item("x").Children(draw)
			}`,
			violations: 1,
		},
		{
			name: "shared popup and item helpers are allowed",
			source: `func page() {
				sel := controlSelect(c, &value)
				sel.Popup(func(panel native.Element) {
					controlSelectPopup(c, panel).Children(func() {
						controlSelectItem(c, sel.Item("x")).Label("X").Children(draw)
					})
				})
			}`,
		},
		{
			name: "ordinary dialog panel style is not a selection override",
			source: `func page() {
				dialog := native.Dialog(c, &open)
				dialog.Content(func(panel native.Element) { panel.Padding(24).Background(red) })
			}`,
		},
		{
			name:       "callback captured control",
			source:     `func page() { b := controlSwitch(c, &value); callback(func() { b.Draw(draw) }) }`,
			violations: 1,
		},
		{
			name:       "declared then assigned",
			source:     `func page() { var b native.Element; b = secondaryButton(c, "Save"); b.Border(1, red) }`,
			violations: 1,
		},
		{
			name:       "conditional reassignment cannot erase provenance",
			source:     `func page() { b := primaryButton(c, "Save"); if narrow { b = native.Box(c) }; b.Height(48) }`,
			violations: 1,
		},
		{
			name: "layout and behavior are allowed",
			source: `func page() {
				controlTextInput(c, &value).FillWidth().Grow(1).Width(200).MinWidth(100).MaxWidth(400).Margin(4, 0)
				primaryButton(c, "Save").Label("Save").Disabled(busy).OnClick(save)
				sel := controlSelect(c, &value); sel.Trigger.FillWidth().OnChange(save)
			}`,
		},
		{
			name: "text area height belongs to content layout",
			source: `func page() {
				controlTextArea(c, &value).FillWidth().Height(200).MinHeight(100).MaxHeight(400)
			}`,
		},
		{
			name:       "base text area height does not add a style violation",
			source:     `func page() { native.TextAreaBase(c, &value).Height(200) }`,
			violations: 1,
		},
		{
			name:       "text area visual style remains shared",
			source:     `func page() { controlTextArea(c, &value).Radius(4) }`,
			violations: 1,
		},
		{
			name:       "alternate padding and border APIs are not loopholes",
			source:     `func page() { controlTextInput(c, &value).PaddingX(8).BorderColor(red).DrawOver(draw) }`,
			violations: 3,
		},
		{
			name: "ordinary text and container styles are allowed",
			source: `func page() {
				native.Column(c).Padding(24).Background(red)
				native.Text(c, "Title").FontSize(20).TextColor(red)
				b := native.Box(c); b.Height(30)
			}`,
		},
		{
			name: "shadowed local is not a control",
			source: `func page() {
				b := primaryButton(c, "Save"); b.OnClick(save)
				callback(func() { b := native.Box(c); b.Background(red) })
			}`,
		},
		{
			name: "shadowed package name is not the UI package",
			source: `func page() {
				native := factory; native.ButtonBase(c)
			}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := "package example\nimport native \"github.com/egoist/mygo/ui\"\n" + test.source
			violations, err := inspectSettingsControls("settings_example.go", []byte(source))
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != test.violations {
				t.Fatalf("got %d violations, want %d: %+v", len(violations), test.violations, violations)
			}
		})
	}
}

func TestInspectSettingsControlsRejectsEveryRawConstructor(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"ButtonBase", "Button", "PrimaryButton", "TextInput", "TextInputBase",
		"TextArea", "TextAreaBase", "Checkbox", "CheckboxBase",
		"Switch", "SwitchBase", "Radio", "RadioBase", "MenuButton", "SelectBase",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := fmt.Sprintf(
				"package example\nimport \"github.com/egoist/mygo/ui\"\nfunc page() { ui.%s(c) }",
				name,
			)
			violations, err := inspectSettingsControls("settings_example.go", []byte(source))
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != 1 || !strings.Contains(violations[0].message, "shared settings control") {
				t.Fatalf("raw constructor was not rejected: %+v", violations)
			}
		})
	}
}

func TestInspectSettingsControlsRejectsDotImport(t *testing.T) {
	t.Parallel()
	source := "package example\nimport . \"github.com/egoist/mygo/ui\"\nfunc page() { Button(c) }"
	violations, err := inspectSettingsControls("settings_example.go", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || !strings.Contains(violations[0].message, "dot-importing") {
		t.Fatalf("dot import was not rejected: %+v", violations)
	}
}
