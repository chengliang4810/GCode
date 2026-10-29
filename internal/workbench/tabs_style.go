package workbench

import (
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// Keep theme::wash's float endpoints and alpha until source-over blending.
// ui.RGBA would first round .92/.10 and .10/.06 to byte color components.
var tabChipWashes = sync.OnceValue(func() struct {
	shader  *shader.Shader
	options [2][2]shader.Options
} {
	s := shader.MustCompile(`package main
var Color vec4
func Fragment(dst vec4, pos vec2) vec4 {
	return vec4(Color.rgb * Color.a, Color.a)
}`)
	color := s.Uniform("Color")
	var options [2][2]shader.Options
	for appearance, luminance := range []float32{.10, .92} {
		for state, alpha := range []float32{.06, .10} {
			// Values are immutable after initialization. Draw copies them into
			// the current frame, so windows can share these four materials.
			options[appearance][state] = shader.Options{
				Values: s.NewValues().Vec4(color, luminance, luminance, luminance, alpha),
				Radius: 6,
			}
		}
	}
	return struct {
		shader  *shader.Shader
		options [2][2]shader.Options
	}{s, options}
})

func paintTabChipWash(p *ui.Painter, r ui.Rect, dark, selected bool) {
	appearance, state := 0, 0
	if dark {
		appearance = 1
	}
	if selected {
		state = 1
	}
	wash := tabChipWashes()
	wash.shader.Draw(p, r, wash.options[appearance][state])
}
