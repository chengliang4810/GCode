package desktop

import (
	"strings"
	"sync"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// This paints the two source-alpha passes from new_thread_background_mask.rs.
// Resizing changes uniforms only; source pixels and their atlas entry stay put.
var conversationBackgroundShader = sync.OnceValue(func() *shader.Shader {
	return shader.MustCompile(`package main
var Fit vec4
var Composer vec4
var Opacity float
func ease(v float) float { v=clamp(v,0,1); return v*v*(3-2*v) }
func Fragment(dst vec4, pos vec2) vec4 {
 size:=rectSize()
 source:=image0Sample((pos-Fit.xy)/Fit.zw*image0Size())
 fade:=ease((size.y-pos.y)/max(size.y,1))
 h:=max(Composer.y+Composer.w,size.y)-Composer.y
 half:=vec2(Composer.z,h)*0.5
 q:=abs(pos-Composer.xy-half)-half+vec2(26)
 distance:=length(max(q,vec2(0)))+min(max(q.x,q.y),0)-26
 cut:=ease((distance-8)/clamp(size.y*0.52,120,280))
 under:=fade*0.5*source.a
 over:=min(fade,cut)*source.a
 alpha:=(over+under*(1-over))*Opacity
 return vec4(source.rgb/max(source.a,0.000001)*alpha,alpha)
}`)
})

func (a *App) paintConversationBackground(c *ui.Context, root ui.Element) {
	if a.background.Bitmap == nil {
		return
	}
	bitmap := a.background.Bitmap
	adjustment := adjustmentFromContract(a.state.Settings.Appearance.BackgroundAdjustment)
	viewportWidth, viewportHeight := c.Size()
	heroWidth := viewportWidth
	if a.fit.Sidebar {
		heroWidth -= a.visibleSidebarWidth
	}
	if a.fit.Files {
		heroWidth -= a.filesWidth
	}
	opacity := float32(1)
	surface := a.state.Settings.Appearance.Surface
	if (a.palette.Frosted || strings.EqualFold(surface, "frosted")) && !strings.EqualFold(surface, "opaque") {
		opacity = .84
	}
	root.Draw(func(p *ui.Painter, bounds ui.Rect) {
		composer := a.composerSurface.Bounds(c)
		if composer.W <= 0 || composer.H <= 0 {
			return
		}
		hero := ui.Rect{X: bounds.X, Y: bounds.Y, W: heroWidth, H: design.BackgroundHeight(viewportHeight)}
		width, height := bitmap.Size()
		fit, ok := design.BackgroundFit(float32(width), float32(height), ui.Rect{W: hero.W, H: hero.H}, adjustment)
		if !ok {
			return
		}
		conversationBackgroundShader().Draw(p, hero, shader.Options{Images: [4]*ui.Bitmap{bitmap}, Uniforms: map[string]any{
			"Fit": [4]float32{fit.X, fit.Y, fit.W, fit.H}, "Composer": [4]float32{composer.X - hero.X, composer.Y - hero.Y, composer.W, composer.H}, "Opacity": opacity,
		}})
	})
}
