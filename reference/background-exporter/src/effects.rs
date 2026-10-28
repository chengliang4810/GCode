// Exact original raster/cover methods follow this minimal GPUI geometry bridge.
use gpui::{Pixels,px};
mod gpui {
    #[derive(Clone, Copy)] pub struct Pixels(pub f32);
    impl From<Pixels> for f32 {fn from(v:Pixels)->Self{v.0}}
    #[derive(Clone,Copy)] pub struct Size<T>{pub width:T,pub height:T}
    pub fn px(v:f32)->Pixels{Pixels(v)}
    pub fn size<T>(width:T,height:T)->Size<T>{Size{width,height}}
}
struct BackgroundLuminance {width:u32,height:u32,pixels:Box<[u8]>,colors:Box<[[u8;4]]>}
impl BackgroundLuminance {
    fn scanline_pixels(&self, light: bool) -> image::RgbaImage {
        image::RgbaImage::from_fn(self.width, self.height, |x, y| {
            let [r, g, b, a] = self.colors[(y * self.width + x) as usize];
            let gain = if y % 3 == 0 { 0.52 } else { 1.0 };
            let channel = |value: u8| {
                if light {
                    (value as f32 + (255.0 - value as f32) * (1.0 - gain)) as u8
                } else {
                    (value as f32 * gain) as u8
                }
            };
            image::Rgba([channel(b), channel(g), channel(r), a])
        })
    }
    fn ascii_pixels(&self, light: bool) -> image::RgbaImage {
        // Five-column bitmap glyphs, one column/row of spacing. These are
        // artwork pixels rather than thousands of shaped UI text runs.
        const GLYPHS: [[u8; 7]; 10] = [
            [0, 0, 0, 0, 0, 0, 0],
            [0, 0, 0, 0, 0, 4, 0],
            [0, 4, 0, 0, 4, 0, 0],
            [0, 0, 0, 14, 0, 0, 0],
            [0, 0, 14, 0, 14, 0, 0],
            [0, 4, 4, 31, 4, 4, 0],
            [0, 21, 14, 31, 14, 21, 0],
            [10, 10, 31, 10, 31, 10, 10],
            [17, 2, 4, 4, 8, 16, 17],
            [14, 17, 23, 21, 23, 16, 14],
        ];
        image::RgbaImage::from_fn(self.width, self.height, |x, y| {
            let sx = (x / 6 * 6 + 3).min(self.width - 1);
            let sy = (y / 8 * 8 + 4).min(self.height - 1);
            let sample = (sy * self.width + sx) as usize;
            let ink_density = if light {
                255 - self.pixels[sample]
            } else {
                self.pixels[sample]
            };
            let index = ((ink_density as f32 / 255.0).sqrt() * 9.0) as usize;
            let ink =
                x % 6 < 5 && y % 8 < 7 && GLYPHS[index][y as usize % 8] & (1 << (4 - x % 6)) != 0;
            let [r, g, b, a] = self.colors[(y * self.width + x) as usize];
            let [cr, cg, cb, _] = self.colors[sample];
            let mix = |base: u8, glyph: u8| {
                let paper = if light { 255.0 } else { 0.0 };
                // Keep a colored image beneath the glyph texture in both themes.
                (base as f32 * 0.60
                    + if ink {
                        glyph as f32 * 0.40
                    } else {
                        paper * 0.40
                    }) as u8
            };
            image::Rgba([mix(b, cb), mix(g, cg), mix(r, cr), a])
        })
    }
    fn halftone_pixels(&self, width: u32, height: u32, light: bool) -> image::RgbaImage {
        let bounds = gpui::size(px(width as f32), px(height as f32));
        let paper = if light { 255 } else { 0 };
        let mut pixels =
            image::RgbaImage::from_pixel(width, height, image::Rgba([paper, paper, paper, 255]));
        for y in (0..height).step_by(4) {
            for x in (0..width).step_by(4) {
                let luma = self.sample_cover(bounds, x as f32, y as f32);
                let luma = if light { 255 - luma } else { luma };
                let radius = 2.0 * (0.3 + 0.7 * (luma as f32 / 255.0).sqrt());
                let [r, g, b, a] =
                    self.colors[self.cover_index(bounds, x as f32 + 2.0, y as f32 + 2.0)];
                for dy in 0..4.min(height - y) {
                    for dx in 0..4.min(width - x) {
                        let distance =
                            ((dx as f32 - 1.5).powi(2) + (dy as f32 - 1.5).powi(2)).sqrt();
                        let coverage = (radius + 0.5 - distance).clamp(0.0, 1.0) * a as f32 / 255.0;
                        let [sr, sg, sb, sa] = self.colors[((y + dy) * width + x + dx) as usize];
                        let blend = |source: u8, dot: u8| {
                            (source as f32 * 0.60
                                + (dot as f32 * coverage + paper as f32 * (1.0 - coverage)) * 0.40)
                                as u8
                        };
                        pixels.put_pixel(
                            x + dx,
                            y + dy,
                            image::Rgba([blend(sb, b), blend(sg, g), blend(sr, r), sa]),
                        );
                    }
                }
            }
        }
        pixels
    }

    fn dither_pixels(&self, width: u32, height: u32) -> image::RgbaImage {
        const BAYER: [[u8; 4]; 4] = [[0, 8, 2, 10], [12, 4, 14, 6], [3, 11, 1, 9], [15, 7, 13, 5]];
        let bounds = gpui::size(px(width as f32), px(height as f32));
        let mut pixels = image::RgbaImage::new(width, height);
        for y in (0..height).step_by(2) {
            for x in (0..width).step_by(2) {
                // Sample/quantize once per dot, not four times per 2x2 cell.
                let index = self.cover_index(bounds, (x + 1) as f32, (y + 1) as f32);
                let [r, g, b, a] = dither_color(
                    self.colors[index],
                    BAYER[y as usize / 2 % 4][x as usize / 2 % 4],
                );
                for dy in 0..2.min(height - y) {
                    for dx in 0..2.min(width - x) {
                        // RenderImage consumes BGRA.
                        pixels.put_pixel(x + dx, y + dy, image::Rgba([b, g, r, a]));
                    }
                }
            }
        }
        pixels
    }

    fn sample_cover(&self, bounds: gpui::Size<Pixels>, x: f32, y: f32) -> u8 {
        self.pixels[self.cover_index(bounds, x, y)]
    }

    fn cover_index(&self, bounds: gpui::Size<Pixels>, x: f32, y: f32) -> usize {
        let width = f32::from(bounds.width).max(1.0);
        let height = f32::from(bounds.height).max(1.0);
        let source_width = self.width as f32;
        let source_height = self.height as f32;
        let scale = (width / source_width).max(height / source_height);
        let visible_width = width / scale;
        let visible_height = height / scale;
        let source_x = ((source_width - visible_width) * 0.5 + x / scale)
            .clamp(0.0, source_width - 1.0) as u32;
        let source_y = ((source_height - visible_height) * 0.5 + y / scale)
            .clamp(0.0, source_height - 1.0) as u32;
        (source_y * self.width + source_x) as usize
    }
}


fn dither_color([r, g, b, a]: [u8; 4], threshold: u8) -> [u8; 4] {
    let peak = r.max(g).max(b) as f32;
    let bright = peak / 255.0 > (threshold as f32 + 0.5) / 16.0;
    let gain = if bright { 255.0 / peak.max(1.0) } else { 0.08 };
    [
        (r as f32 * gain).round() as u8,
        (g as f32 * gain).round() as u8,
        (b as f32 * gain).round() as u8,
        a,
    ]
}


pub fn effect(source:&image::RgbaImage,name:&str,light:bool)->image::RgbaImage{
 let gray=image::DynamicImage::ImageRgba8(source.clone()).to_luma8();
 let data=BackgroundLuminance {width:source.width(),height:source.height(),pixels:gray.into_raw().into_boxed_slice(),colors:source.pixels().map(|p|p.0).collect()};
 let mut result=match name {"scanlines"=>data.scanline_pixels(light),"ascii"=>data.ascii_pixels(light),"dither"=>data.dither_pixels(data.width,data.height),"halftone"=>data.halftone_pixels(data.width,data.height,light),_=>image::RgbaImage::from_fn(data.width,data.height,|x,y|{let [r,g,b,a]=source.get_pixel(x,y).0;image::Rgba([b,g,r,a])})};
 for pixel in result.pixels_mut(){pixel.0.swap(0,2);} result
}
