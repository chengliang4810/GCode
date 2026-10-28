//! Native SVG preparation follows pinned image_media::svg_options/decode_image.
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::{fs, path::Path, sync::Arc};

const FACES: [&str; 16] = [
    "Geist.ttf",
    "Geist-Italic.ttf",
    "Geist-Medium.ttf",
    "Geist-MediumItalic.ttf",
    "Geist-SemiBold.ttf",
    "Geist-SemiBoldItalic.ttf",
    "Geist-Bold.ttf",
    "Geist-BoldItalic.ttf",
    "GeistMono.ttf",
    "GeistMono-Italic.ttf",
    "GeistMono-Medium.ttf",
    "GeistMono-MediumItalic.ttf",
    "GeistMono-SemiBold.ttf",
    "GeistMono-SemiBoldItalic.ttf",
    "GeistMono-Bold.ttf",
    "GeistMono-BoldItalic.ttf",
];

pub fn digest(data: &[u8]) -> String {
    format!("{:x}", Sha256::digest(data))
}

pub fn options(
    root: &Path,
    reference: &Path,
) -> Result<(usvg::Options<'static>, Vec<Value>), Box<dyn std::error::Error>> {
    let mut db = usvg::fontdb::Database::new();
    db.load_system_fonts();
    let mut fonts = Vec::new();
    for face in FACES {
        let source = format!("crates/ui/assets/fonts/{face}");
        let bytes = fs::read(reference.join(&source))?;
        let imported = fs::read(root.join("internal/assets/data/fonts").join(face))?;
        if bytes != imported {
            return Err(format!("imported face differs from reference: {face}").into());
        }
        fonts.push(json!({"name":face,"source":source,"sha256":digest(&bytes)}));
        db.load_font_data(bytes);
    }
    db.set_sans_serif_family("Geist");
    db.set_monospace_family("Geist Mono");
    db.set_serif_family("Geist");
    for (family, file) in [("Geist", "Geist.ttf"), ("Geist Mono", "GeistMono.ttf")] {
        let families = [usvg::fontdb::Family::Name(family)];
        let id = db
            .query(&usvg::fontdb::Query {
                families: &families,
                ..Default::default()
            })
            .ok_or_else(|| format!("bundled SVG family unavailable: {family}"))?;
        let selected = db
            .with_face_data(id, |bytes, _| digest(bytes))
            .ok_or("font bytes unavailable")?;
        let expected = fonts.iter_mut().find(|font| font["name"] == file).unwrap();
        if expected["sha256"] != selected {
            return Err(format!(
                "{family} resolves to a different system font, not the original bundled face"
            )
            .into());
        }
        expected["selectedForNormalFamily"] = json!(true);
    }
    // Identical resource restrictions to the original image_media renderer.
    Ok((
        usvg::Options {
            fontdb: Arc::new(db),
            image_href_resolver: usvg::ImageHrefResolver {
                resolve_data: Box::new(|_, _, _| None),
                resolve_string: Box::new(|_, _| None),
            },
            ..Default::default()
        },
        fonts,
    ))
}

fn path_count(group: &usvg::Group) -> usize {
    group
        .children()
        .iter()
        .map(|node| match node {
            usvg::Node::Path(_) => 1,
            usvg::Node::Group(g) => path_count(g),
            usvg::Node::Text(t) => path_count(t.flattened()),
            _ => 0,
        })
        .sum()
}

fn labels(group: &usvg::Group, result: &mut Vec<Value>) {
    for node in group.children() {
        match node {
            usvg::Node::Group(g) => labels(g, result),
            usvg::Node::Text(t) => {
                let text: String = t.chunks().iter().map(|chunk| chunk.text()).collect();
                let bounds = t.abs_bounding_box();
                result.push(json!({"text":text,"outlinePaths":path_count(t.flattened()),"bounds":{"x":bounds.x(),"y":bounds.y(),"width":bounds.width(),"height":bounds.height()}}));
            }
            _ => {}
        }
    }
}

pub fn outline(
    svg: &str,
    options: &usvg::Options<'_>,
) -> Result<(String, Vec<Value>), Box<dyn std::error::Error>> {
    let tree = usvg::Tree::from_data(svg.as_bytes(), options)?;
    let mut text = Vec::new();
    labels(tree.root(), &mut text);
    if text.len() != 6
        || text
            .iter()
            .any(|label| label["outlinePaths"].as_u64().unwrap_or(0) == 0)
    {
        return Err("a Mermaid node or edge label has no outline paths".into());
    }
    // Original decode_image serializes the parsed tree with these exact defaults;
    // preserve_text=false converts each Text::flattened group to vector paths.
    let outlined = tree.to_string(&usvg::WriteOptions::default());
    if outlined.contains("<text") || outlined.contains("<tspan") || outlined.contains("<image") {
        return Err("prepared SVG still contains text or raster images".into());
    }
    Ok((outlined, text))
}
