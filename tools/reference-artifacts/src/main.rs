//! Build-time reference exports. This tool is never linked into the Go app.
//! Mermaid adapter is copied from the pinned reference without style edits.
#![allow(dead_code)]
mod mermaid;
mod outlining;
use serde_json::{json, Value};
use std::{fs, path::PathBuf};

const MATH: &str = r"\frac{a}{b} + \sqrt{x^2+y^2}";
const DIAGRAM: &str =
    "flowchart LR\n A[Request] --> B{Ready?}\n B -->|Yes| C[Response]\n B -->|No| D[Wait]";

// The reference adapter only needs RGB conversion, alpha changes, and source-
// over composition. These preserve its palette math without depending on GPUI.
mod gpui {
    #[derive(Clone, Copy)]
    pub struct Hsla {
        pub r: f32,
        pub g: f32,
        pub b: f32,
        pub a: f32,
    }
    impl Hsla {
        pub fn from_hex(s: &str) -> Self {
            let s = s.trim_start_matches('#');
            let channel = |i: usize| u8::from_str_radix(&s[i..i + 2], 16).unwrap() as f32 / 255.;
            Self {
                r: channel(0),
                g: channel(2),
                b: channel(4),
                a: if s.len() == 8 { channel(6) } else { 1. },
            }
        }
        pub fn to_rgb(self) -> Self {
            self
        }
        pub fn opacity(mut self, a: f32) -> Self {
            self.a *= a;
            self
        }
        pub fn blend(self, over: Self) -> Self {
            let alpha = over.a + self.a * (1. - over.a);
            if alpha == 0. {
                return Self {
                    r: 0.,
                    g: 0.,
                    b: 0.,
                    a: 0.,
                };
            }
            Self {
                r: (over.r * over.a + self.r * self.a * (1. - over.a)) / alpha,
                g: (over.g * over.a + self.g * self.a * (1. - over.a)) / alpha,
                b: (over.b * over.a + self.b * self.a * (1. - over.a)) / alpha,
                a: alpha,
            }
        }
    }
}
mod theme {
    use super::{gpui::Hsla, Value};
    #[derive(Clone)]
    pub struct Appearance(pub bool);
    impl Appearance {
        pub fn is_dark(&self) -> bool {
            self.0
        }
    }
    pub struct Theme {
        pub appearance: Appearance,
        pub font_sans: String,
        pub bg: Hsla,
        pub text: Hsla,
        pub text_muted: Hsla,
        pub text_faint: Hsla,
        pub border: Hsla,
        pub border_strong: Hsla,
        pub accent: Hsla,
    }
    impl Theme {
        pub fn from_variant(v: &Value) -> Self {
            let c = &v["colors"];
            let col = |k: &str| Hsla::from_hex(c[k].as_str().unwrap());
            Self {
                appearance: Appearance(v["appearance"] == "dark"),
                font_sans: "Geist".into(),
                bg: col("background"),
                text: col("text"),
                text_muted: col("textMuted"),
                text_faint: col("textFaint"),
                border: col("border"),
                border_strong: col("borderStrong"),
                accent: Hsla::from_hex(v["accent"]["primary"].as_str().unwrap()),
            }
        }
        pub fn ink(&self, alpha: f32) -> Hsla {
            Hsla::from_hex(if self.appearance.0 {
                "#ffffff"
            } else {
                "#000000"
            })
            .opacity(alpha)
        }
    }
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let root = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .unwrap()
        .parent()
        .unwrap()
        .to_owned();
    let status = std::process::Command::new("git")
        .args(["-C", "D:/zeron", "rev-parse", "HEAD"])
        .output()?;
    if String::from_utf8(status.stdout)?.trim() != "ab189159f10cf8e63c0502be75a59bbc487e0eaf" {
        return Err("reference commit changed".into());
    }
    let catalog: Value =
        serde_json::from_slice(&fs::read(root.join("internal/design/themes.json"))?)?;
    let output = root.join("internal/assets/data/artifacts");
    fs::create_dir_all(&output)?;
    let raw_output = root.join("reference/mermaid/raw");
    fs::create_dir_all(&raw_output)?;
    let (svg_options, fonts) = outlining::options(&root, std::path::Path::new("D:/zeron"))?;
    let diagrams_only = std::env::args().any(|arg| arg == "--diagrams-only");
    let mut diagrams = Vec::new();
    let mut manifest = Vec::new();
    for family in catalog["families"].as_array().unwrap() {
        for variant in family["variants"].as_array().unwrap() {
            let id = variant["id"].as_str().unwrap();
            let theme = theme::Theme::from_variant(variant);
            let svg = mermaid::render(DIAGRAM, &mermaid::Palette::from_theme(&theme))
                .map_err(|e| format!("{id}: {e}"))?;
            let name = format!("diagram-{id}.svg");
            fs::write(raw_output.join(&name), &svg)?;
            let (outlined, labels) = outlining::outline(&svg, &svg_options)?;
            let width = svg_attr(&svg, "width").unwrap();
            let height = svg_attr(&svg, "height").unwrap();
            if (svg_attr(&outlined, "width").unwrap() - width).abs() > 0.0001
                || (svg_attr(&outlined, "height").unwrap() - height).abs() > 0.0001
            {
                return Err(format!("{id}: SVG canvas size changed").into());
            }
            fs::write(output.join(&name), &outlined)?;
            diagrams.push(json!({"id":id,"asset":format!("artifacts/{name}"),"rawSVG":format!("reference/mermaid/raw/{name}"),"rawSha256":outlining::digest(svg.as_bytes()),"sha256":outlining::digest(outlined.as_bytes()),"width":width,"height":height,"textColor":variant["colors"]["text"],"backgroundColor":variant["colors"]["background"],"labels":labels}));
            manifest.push(json!({"id":format!("diagram-{id}"),"source":DIAGRAM,"assetRef":format!("asset://artifacts/{name}"),"width":svg_attr(&svg,"width").unwrap_or(640.),"height":svg_attr(&svg,"height").unwrap_or(240.),"baseline":0.}));
            if diagrams_only {
                continue;
            }
            for (face_id, face) in [
                ("fira", zatex::bundled::Face::Fira),
                ("stixTwo", zatex::bundled::Face::StixTwo),
                ("computerModern", zatex::bundled::Face::ComputerModern),
                ("libertinus", zatex::bundled::Face::Libertinus),
                ("pagella", zatex::bundled::Face::Pagella),
                ("euler", zatex::bundled::Face::Euler),
            ] {
                let font = zatex::bundled::font(face);
                let formula = zatex::layout(MATH, font, &zatex::Options::display(24.))?;
                let color = variant["colors"]["text"].as_str().unwrap();
                let svg = formula
                    .to_svg(font)
                    .replace("<path d=", &format!("<path fill=\"{color}\" d="));
                let name = format!("math-{face_id}-{id}.svg");
                fs::write(output.join(&name), svg)?;
                manifest.push(json!({"id":format!("math-{face_id}-{id}"),"source":MATH,"assetRef":format!("asset://artifacts/{name}"),"width":formula.width,"height":formula.height(),"baseline":formula.ascent}));
            }
        }
    }
    if !diagrams_only {
        fs::write(
            root.join("internal/mock/artifacts.json"),
            serde_json::to_vec_pretty(&manifest)?,
        )?;
    }
    let diagram_manifest = json!({"schemaVersion":1,"sourceCommit":"ab189159f10cf8e63c0502be75a59bbc487e0eaf","mermaidRenderer":"0.3.1","usvg":"0.46.0","fontdb":"0.23.0","source":DIAGRAM,"fontOptionsSource":"crates/ui/src/image_media.rs:svg_options/decode_image","fontOptionsSourceSha256":outlining::digest(&fs::read("D:/zeron/crates/ui/src/image_media.rs")?),"exporterLockSha256":outlining::digest(&fs::read(root.join("tools/reference-artifacts/Cargo.lock"))?),"outlineAdapterSha256":outlining::digest(&fs::read(root.join("tools/reference-artifacts/src/outlining.rs"))?),"mermaidSourceSha256":outlining::digest(&fs::read("D:/zeron/crates/ui/src/markdown/mermaid.rs")?),"fontLicense":"fonts/licenses/Geist-OFL.txt","fonts":fonts,"nativeVisualAcceptance":"pending","diagrams":diagrams});
    fs::write(
        output.join("diagram-manifest.json"),
        serde_json::to_vec_pretty(&diagram_manifest)?,
    )?;
    let provenance_path = output.join("provenance.json");
    let mut provenance: Value = serde_json::from_slice(&fs::read(&provenance_path)?)?;
    provenance["diagramOutlines"] = json!({"sourceCommit":"ab189159f10cf8e63c0502be75a59bbc487e0eaf","rawOutputs":"reference/mermaid/raw","manifest":"internal/assets/data/artifacts/diagram-manifest.json","textToPaths":"original usvg0.46.0/fontdb0.23.0 pipeline with all16 original Geist/GeistMono faces","rasterized":false});
    fs::write(provenance_path, serde_json::to_vec_pretty(&provenance)?)?;
    println!(
        "Exported {} vector artifacts from pinned reference renderers.",
        manifest.len()
    );
    Ok(())
}

fn svg_attr(svg: &str, name: &str) -> Option<f64> {
    let marker = format!(" {name}=\"");
    let p = svg.find(&marker)? + marker.len();
    svg[p..].split('"').next()?.parse().ok()
}
