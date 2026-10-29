//! Test-only Shell child module in the mutable source export. All chips,
//! gradients, scrolling and control glyphs below call frozen production code.
use super::*;
use gpui::{Bounds, Pixels, canvas, point};
use serde::{Deserialize, Serialize};
use sha2::Digest;
use std::{cell::{Cell, RefCell}, collections::BTreeMap, rc::Rc};

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Scene {
    pub appearance: String,
    pub host_width: f32,
    pub count: usize,
    pub scroll_fraction: f32,
}

impl Scene {
    pub fn validate(&self) -> anyhow::Result<()> {
        anyhow::ensure!(matches!(self.appearance.as_str(), "dark" | "light"), "invalid appearance");
        anyhow::ensure!(self.host_width == 400.0 || self.host_width == 220.0, "invalid host width");
        anyhow::ensure!([0, 2, 6].contains(&self.count), "invalid count");
        anyhow::ensure!([0.0, 0.5, 1.0].contains(&self.scroll_fraction), "invalid scroll fraction");
        Ok(())
    }

    pub fn name(&self) -> String {
        let scroll = if self.scroll_fraction == 0.0 { "start" } else if self.scroll_fraction == 0.5 { "middle" } else { "end" };
        format!("tabstrip-{}-w{:.0}-n{}-scroll{scroll}.png", self.appearance, self.host_width, self.count)
    }
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum StyleCase { #[default] Baseline, InactiveHover, DirtyFile, FileHover, FileHoverEdge }

impl StyleCase {
    pub fn parse(value: &str) -> anyhow::Result<Self> {
        match value { "baseline" => Ok(Self::Baseline), "inactive-hover" => Ok(Self::InactiveHover), "dirty-file" => Ok(Self::DirtyFile), "file-hover" => Ok(Self::FileHover), "file-hover-edge" => Ok(Self::FileHoverEdge), _ => anyhow::bail!("unknown style case {value}") }
    }
    pub fn name(self) -> &'static str {
        match self { Self::Baseline => "baseline", Self::InactiveHover => "inactive-hover", Self::DirtyFile => "dirty-file", Self::FileHover => "file-hover", Self::FileHoverEdge => "file-hover-edge" }
    }
    pub fn is_file(self) -> bool { matches!(self, Self::DirtyFile | Self::FileHover | Self::FileHoverEdge) }
    pub fn is_hover(self) -> bool { matches!(self, Self::InactiveHover | Self::FileHover | Self::FileHoverEdge) }
    pub fn host_origin(self) -> [f32; 2] { if self == Self::FileHoverEdge { [400.0, 510.0] } else { [40.0, 40.0] } }
    pub fn file_path(self, index: usize) -> String {
        let title=format!("Tab{index:02}.txt");
        if self == Self::FileHoverEdge { format!("workspace/long-component-path/long-component-path/long-component-path/{title}") } else { title }
    }
    pub fn capture_name(self, scene: &Scene) -> String {
        if self == Self::Baseline { scene.name() } else { scene.name().replace(".png", &format!("-style-{}.png", self.name())) }
    }
}

type Geometry = Rc<RefCell<BTreeMap<&'static str, [f32; 4]>>>;

fn rect(bounds: Bounds<Pixels>) -> [f32; 4] {
    [bounds.origin.x.into(), bounds.origin.y.into(), bounds.size.width.into(), bounds.size.height.into()]
}

fn box_json(bounds: [f32; 4]) -> serde_json::Value {
    serde_json::json!({"x":bounds[0], "y":bounds[1], "width":bounds[2], "height":bounds[3]})
}

fn chat(id: &str, title: &str, parent: Option<&str>) -> zeron_proto::Chat {
    serde_json::from_value(serde_json::json!({
        "id": id, "deviceId": "local", "title": title, "archived": false,
        "config": null, "createdAt": "2026-01-01T00:00:00Z", "parentChatId": parent,
    })).expect("fixed tabstrip rows must satisfy frozen Chat DTO")
}

pub struct TabstripFixture {
    shell: Entity<Shell>,
    data_dir: PathBuf,
    scene: Scene,
    seeded: bool,
    geometry: Geometry,
    layout_frames: Rc<Cell<u64>>,
    style_case: StyleCase,
    file_editors: Vec<Entity<gpui_base::input::EditorState>>,
}

impl TabstripFixture {
    pub fn new(data_dir: PathBuf, scene: Scene, cx: &mut Context<Self>) -> Self {
        let shell = Self::empty_shell(&data_dir, &scene, cx);
        Self { shell, data_dir, scene, seeded: false, geometry: Rc::default(), layout_frames: Rc::default(), style_case: StyleCase::Baseline, file_editors: Vec::new() }
    }

    fn empty_shell(data_dir: &std::path::Path, scene: &Scene, cx: &mut Context<Self>) -> Entity<Shell> {
        // Finish parent data before creating Shell. Any later parent notify
        // clears side_chats when engine=None (production shell.rs:3034).
        let state = cx.new(|_| {
            let mut state = AppState::new();
            state.connection = ConnectionStatus::Ready;
            state.workspace_scope = Some(zeron_proto::WorkspaceScope::Local);
            state.local_device_id = Some("local".into());
            state.selected_device = Some("local".into());
            state.no_project = true;
            state.auto_selected = true;
            state.data_dir = Some(data_dir.into());
            state.selected_chat = Some("parent".into());
            state.chats.push(chat("parent", "Parent", None));
            for ix in 1..=scene.count {
                state.chats.push(chat(&format!("side-{ix:02}"), &format!("Tab{ix:02}"), Some("parent")));
            }
            state
        });
        cx.new(|cx| {
            let mut shell = Shell::new(state, EngineBootConfig {
                data_dir: data_dir.into(), ipc_port: 0, edge_url: "http://127.0.0.1:1".into(),
                edge_token: None, org_id: None, workos_client_id: None,
                default_harness: zeron_proto::HarnessId::Mock,
            }, cx);
            shell.active_chat = "parent".into();
            shell.reduced_motion = crate::motion::reduced_motion(cx);
            shell
        })
    }

    pub fn reset_scene(&mut self, scene: Scene, cx: &mut Context<Self>) -> anyhow::Result<()> {
        scene.validate()?;
        self.shell = Self::empty_shell(&self.data_dir, &scene, cx);
        self.scene = scene;
        self.seeded = false;
        self.geometry = Rc::default();
        self.layout_frames = Rc::default();
        self.file_editors.clear();
        cx.notify();
        Ok(())
    }

    pub fn set_style_case(&mut self, style_case: StyleCase) { self.style_case = style_case; }
    pub fn capture_name(&self) -> String { self.style_case.capture_name(&self.scene) }

    pub fn seed_file_tabs(&mut self, window: &mut Window, cx: &mut Context<Self>) -> anyhow::Result<()> {
        anyhow::ensure!(self.style_case.is_file() && !self.seeded && self.scene.count == 2, "invalid file-style seed state");
        self.file_editors = self.shell.update(cx, |shell, cx| {
            shell.settings.files_autosave_enabled = false;
            let mut editors = Vec::new();
            for ix in 1..=2 {
                let path = self.style_case.file_path(ix);
                shell.add_file_surface(path.clone(), window, cx);
                let id = shell.file_surface_seq;
                let file = shell.file_surfaces.get(&id).cloned().ok_or_else(|| anyhow::anyhow!("original Shell did not register File Surface"))?;
                let text = format!("Tab{ix:02} original content\n");
                let reply = zeron_proto::WorkspaceFileText {
                    checkout_id:"fixture-checkout".into(), path:path.clone(),
                    text:Some(text.clone()), content_hash:Some(format!("{:x}",sha2::Sha256::digest(text.as_bytes()))),
                    size:text.len() as u64, modified_at:None,
                    encoding:zeron_proto::WorkspaceTextEncoding::Utf8,
                    line_ending:Some(zeron_proto::WorkspaceLineEnding::Lf),
                    read_only_reason:None, truncated:false,
                };
                editors.push(file.update(cx, |file, cx| file.tabstrip_fixture_load_text(&path, reply, window, cx))?);
            }
            let first = shell.right_tabs.get(&shell.panel_key(cx)).and_then(|tabs| tabs.first()).copied().ok_or_else(|| anyhow::anyhow!("missing production file tabs"))?;
            shell.set_right_active(first, cx);
            cx.notify();
            Ok::<_, anyhow::Error>(editors)
        })?;
        self.seeded = true;
        cx.notify();
        Ok(())
    }

    // Called in a separate UI update after subscriptions/deferrals settle.
    // replace emits original InputEvent::Change; no dirty/revision flag writes.
    pub fn edit_first_file(&mut self, window: &mut Window, cx: &mut Context<Self>) -> anyhow::Result<()> {
        anyhow::ensure!(self.style_case.is_file() && self.file_editors.len() == 2, "file editors not seeded");
        self.file_editors[0].update(cx, |editor, cx| {
            let end = editor.value().len();
            editor.set_selected_range(end..end, cx);
            editor.replace("modified by original editor", window, cx);
        });
        cx.notify();
        Ok(())
    }

    pub fn pointer_target(&self, window: &Window, cx: &App) -> anyhow::Result<[f32; 2]> {
        if !self.style_case.is_hover() { return Ok([10.0, 10.0]); }
        let shell = self.shell.read(cx);
        let scroll = &shell.right_tab_scroll;
        let raw = scroll.bounds_for_item(1).ok_or_else(|| anyhow::anyhow!("second chip has no actual bounds"))?;
        let bounds = Bounds::new(raw.origin + window.pixel_snap_point(scroll.offset()), raw.size);
        Ok([f32::from(bounds.origin.x) + f32::from(bounds.size.width)/2.0, f32::from(bounds.origin.y) + f32::from(bounds.size.height)/2.0])
    }

    pub fn seed_tabs(&mut self, cx: &mut Context<Self>) -> anyhow::Result<()> {
        anyhow::ensure!(!self.seeded, "scene already seeded");
        self.shell.update(cx, |shell, cx| {
            for ix in 1..=self.scene.count {
                shell.open_child_chat_tab(&format!("side-{ix:02}"), cx);
            }
            let first = shell.right_tabs.get(&shell.panel_key(cx)).and_then(|tabs| tabs.first()).copied();
            shell.set_right_active(first.unwrap_or(RightSurface::Picker), cx);
            cx.notify();
        });
        self.seeded = true;
        cx.notify();
        Ok(())
    }

    pub fn set_scroll_fraction(&mut self, cx: &mut Context<Self>) -> anyhow::Result<()> {
        anyhow::ensure!(self.seeded, "seed scene before scroll");
        self.shell.update(cx, |shell, cx| {
            let max = f32::from(shell.right_tab_scroll.max_offset().x);
            shell.right_tab_scroll.set_offset(point(px(-max * self.scene.scroll_fraction), px(0.0)));
            cx.notify();
        });
        cx.notify();
        Ok(())
    }

    pub fn layout_frame(&self) -> u64 { self.layout_frames.get() }

    // Plain measurements only: no Inspector panel, production element copy,
    // persistent Element/Context, or layout changes from a paint callback.
    pub fn stability_sample(&self, cx: &App) -> serde_json::Value {
        let shell = self.shell.read(cx);
        let scroll = &shell.right_tab_scroll;
        serde_json::json!({
            "geometryLogical": *self.geometry.borrow(),
            "scrollViewportLogical": rect(scroll.bounds()),
            "offsetLogical": [f32::from(scroll.offset().x), f32::from(scroll.offset().y)],
            "maxLogical": [f32::from(scroll.max_offset().x), f32::from(scroll.max_offset().y)],
            "childBoundsBeforeOwnScrollLogical": (0..scroll.children_count()).map(|ix| scroll.bounds_for_item(ix).map(rect)).collect::<Vec<_>>(),
            "actualRowCount": shell.right_surface_rows(cx).len(),
            "activeSurface": format!("{:?}", shell.resolved_right_active(cx)),
        })
    }

    pub fn snapshot(&self, window: &Window, cx: &App) -> anyhow::Result<serde_json::Value> {
        anyhow::ensure!(self.seeded, "unseeded scene cannot be captured");
        let shell = self.shell.read(cx);
        let rows = shell.right_surface_rows(cx);
        anyhow::ensure!(rows.len() == self.scene.count, "production row count changed: expected {}, actual {}", self.scene.count, rows.len());
        let active = shell.resolved_right_active(cx);
        anyhow::ensure!(active == rows.first().map(|row| row.0).unwrap_or(RightSurface::Picker), "production active index is not zero");
        anyhow::ensure!(shell.state.read(cx).engine().is_none(), "fixture engine must remain absent");
        let scroll = &shell.right_tab_scroll;
        let expected_children = if rows.is_empty() { 0 } else { rows.len() + 1 };
        anyhow::ensure!(scroll.children_count() == expected_children, "production scroll children mismatch");
        let offset = scroll.offset();
        let snapped_offset = window.pixel_snap_point(offset);
        let max = f32::from(scroll.max_offset().x);
        let actual_x = -f32::from(offset.x);
        anyhow::ensure!((actual_x - max * self.scene.scroll_fraction).abs() < 0.05, "actual scroll does not match requested fraction");
        let mut row_metadata = Vec::new();
        for (ix, (surface, title, dirty, _)) in rows.iter().enumerate() {
            if self.style_case.is_file() {
                let RightSurface::File(id) = surface else { anyhow::bail!("non-File row in dirty fixture"); };
                let file = shell.file_surfaces.get(id).ok_or_else(|| anyhow::anyhow!("missing real File Surface"))?;
                let path = shell.file_surface_paths.get(id).ok_or_else(|| anyhow::anyhow!("missing actual file path"))?;
                anyhow::ensure!(path == &self.style_case.file_path(ix+1) && title.as_ref() == format!("Tab{:02}.txt",ix+1).as_str(), "file title/path mismatch");
                anyhow::ensure!(*dirty == (ix == 0) && file.read(cx).has_unsaved_changes() == (ix == 0), "production file dirty result differs from active-dirty/inactive-clean contract");
                let document = file.read(cx).tabstrip_fixture_document(path, cx)?;
                anyhow::ensure!(document["originalChangeSubscriptionPresent"] == true && document["originalEditorObserverPresent"] == true && document["isDirty"] == *dirty, "original file editor subscriptions/dirty evidence missing");
                let raw = scroll.bounds_for_item(ix).ok_or_else(|| anyhow::anyhow!("file chip has no actual bounds"))?;
                let painted = Bounds::new(raw.origin + snapped_offset, raw.size);
                row_metadata.push(serde_json::json!({
                    "index":ix,"id":format!("file-{:02}",ix+1),"sessionId":format!("file-{:02}",ix+1),"title":title,"parentId":"parent","surfaceId":id,
                    "kind":"file","sourceKind":"File","icon":"file","idle":true,"bounds":box_json(rect(painted)),"active":*surface==active,"dirty":dirty,
                    "filePath":path,"document":document,"iconDecision":{"branch":"original file_icons::icon(FileIconIdentity::file(path), theme.appearance)","sizeDIP":14,"inactiveOpacity":0.78},
                    "boundsBeforeOwnScrollLogical":rect(raw),"boundsAfterSnappedOwnScrollLogical":rect(painted),
                }));
                continue;
            }
            let RightSurface::SideChat(id) = surface else { anyhow::bail!("non-SideChat row in fixture"); };
            let tab = shell.side_chats.get(id).ok_or_else(|| anyhow::anyhow!("missing production SideChat"))?;
            let state = tab.state.read(cx);
            let selected = state.selected_chat_row().ok_or_else(|| anyhow::anyhow!("missing selected child row"))?;
            anyhow::ensure!(selected.id == format!("side-{:02}", ix + 1) && title.as_ref() == format!("Tab{:02}", ix + 1), "unexpected production row identity");
            anyhow::ensure!(selected.config.is_none() && !*dirty, "fixture must remain config=None, clean");
            let indicator = state.indicator_for(&selected.id, Utc::now());
            anyhow::ensure!(indicator != Indicator::Working && state.sessions.is_empty(), "fixture must remain idle");
            let raw = scroll.bounds_for_item(ix).ok_or_else(|| anyhow::anyhow!("production chip has no layout bounds"))?;
            let painted = Bounds::new(raw.origin + snapped_offset, raw.size);
            row_metadata.push(serde_json::json!({
                "index": ix, "id": selected.id, "title": title, "parentId": "parent", "surfaceId": id,
                "sessionId": selected.id, "kind": "session", "sourceKind": "SideChat", "icon": "chat-round-line", "idle": true, "bounds": box_json(rect(painted)),
                "active": *surface == active, "dirty": dirty, "config": selected.config,
                "indicator": format!("{indicator:?}"), "running": false,
                "iconDecision": {"path": icons::CHAT_ROUND_LINE, "branch": "generic fallback; no config/harness brand/favicon/spinner", "sizeDIP": 12, "inactiveOpacity": 0.7},
                "boundsBeforeOwnScrollLogical": rect(raw), "boundsAfterSnappedOwnScrollLogical": rect(painted),
            }));
        }
        let plus = if rows.is_empty() { None } else { scroll.bounds_for_item(rows.len()).map(|raw| serde_json::json!({
            "boundsBeforeOwnScrollLogical": rect(raw),
            "boundsAfterSnappedOwnScrollLogical": rect(Bounds::new(raw.origin + snapped_offset, raw.size)),
        })) };
        let theme = Theme::of(cx);
        let measured = self.geometry.borrow();
        let host = measured.get("outerHost").ok_or_else(|| anyhow::anyhow!("host measurement missing"))?;
        let expand = measured.get("expand").ok_or_else(|| anyhow::anyhow!("expand measurement missing"))?;
        let plus_box = plus.as_ref().map(|entry| {
            let values = entry["boundsAfterSnappedOwnScrollLogical"].as_array().unwrap();
            box_json(std::array::from_fn(|ix| values[ix].as_f64().unwrap() as f32))
        });
        let mut result = serde_json::json!({
            "scene": self.scene, "activeIndex": if rows.is_empty() { None } else { Some(0) },
            "logicalWidth": f32::from(window.viewport_size().width), "logicalHeight": f32::from(window.viewport_size().height), "parentSessionId": "parent",
            "positiveX": actual_x, "maxX": max, "scrollApplied": true,
            "geometry": {"host":box_json(*host), "scrollViewport":box_json(rect(scroll.bounds())), "chips":row_metadata, "plus":plus_box, "expand":box_json(*expand)},
            "parentId": "parent", "rows": row_metadata, "plus": plus,
            "geometryLogical": *measured, "scrollViewportLogical": rect(scroll.bounds()),
            "scrollX": actual_x, "scrollMaxX": max, "rawOffsetLogical": [f32::from(offset.x), f32::from(offset.y)],
            "rawNegativeOffset": f32::from(offset.x),
            "snappedOffsetLogical": [f32::from(snapped_offset.x), f32::from(snapped_offset.y)],
            "fadeLeft": actual_x > 1.0, "fadeRight": actual_x < max - 1.0,
            "geometrySource": "public ScrollHandle.bounds/bounds_for_item; own scroll offset is applied later by production GPUI, so raw and window.pixel_snap_point(offset)-translated bounds are separately named. Outer/expand measured by nonpainting wrapper canvas/on_children_prepainted.",
            "layoutFrame": self.layout_frame(), "expanded": false,
            "productionView": "Shell::render_right_tab_strip + surface_chrome::tab_chip/tab_strip_region; original header_icon_button/right_pane_expand_icon(false)",
            "scaffoldKind": "component testhost, source-proven outer envelope; not full Shell titlebar renderer",
            "scaffold": {"clientDIP": [840,560], "hostRequestedDIP": [self.style_case.host_origin()[0],self.style_case.host_origin()[1],self.scene.host_width,38], "paddingTopRightBottomLeftDIP": [4,4,0,8], "gapDIP": 4, "expandSizeDIP": 28, "rootColorSemantic": "Theme.surface", "hostMaterial": "transparent"},
            "typography": {"family": theme.font_sans, "rootSizeDIP": 14, "rootWeight": 400, "uiRemDIP": f32::from(window.rem_size()), "productionTitleSizeAtRem16DIP": 11.5},
            "rootSurfaceHSLA": theme.surface,
            "nativeWindowAppearance": format!("{:?}", window.appearance()).to_lowercase(),
            "nativeWindowActive": window.is_window_active(), "nativeWindowTitle": window.window_title(),
            "lastSyntheticPointerLogical": [10,10], "osPointerMoved": false,
            "engineBootstrapped": false, "externalServicesUsed": false, "productionRenderingMethodsChanged": false,
        });
        if self.style_case != StyleCase::Baseline {
            result["styleCase"] = self.style_case.name().into();
            result["styleEvidence"] = serde_json::json!({
                "actualHovered":null,"actualHoveredSource":"original chip hover state is not publicly queryable; native dispatch and PNG response are separate evidence",
                "targetClientDIP":self.pointer_target(window,cx)?,"fileRpcOrDiskValidated":false,
            });
            result.as_object_mut().unwrap().remove("lastSyntheticPointerLogical");
            result.as_object_mut().unwrap().remove("osPointerMoved");
            result["fixtureProcessMovedOsPointer"] = false.into();
            if self.style_case == StyleCase::FileHoverEdge {
                result["styleEvidence"]["edge"]=serde_json::json!({
                    "filePaths":[self.style_case.file_path(1),self.style_case.file_path(2)],
                    "hostOriginRequestedDIP":self.style_case.host_origin(),
                    "productionTooltipVisibility":null,
                    "limitation":"production TooltipState and layout bounds are not public GPUI APIs; independent full PNG response remains visual evidence",
                });
            }
        }
        Ok(result)
    }
}

impl Render for TabstripFixture {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::of(cx).clone();
        let geometry = self.geometry.clone();
        let host_geometry = geometry.clone();
        let frames = self.layout_frames.clone();
        let width = self.scene.host_width;
        let host = self.shell.update(cx, |shell, cx| {
            let strip = shell.render_right_tab_strip(cx);
            let expand = header_icon_button("expand-changes", super::tabs::right_pane_expand_icon(false), "Expand panel", &theme,
                cx.listener(|this, _, _, cx| this.toggle_right_pane_expand(cx)));
            // Only this envelope is testhost scaffolding; every visible
            // chip/fade/plus/expand subtree is the original production one.
            div().on_children_prepainted(move |bounds, _, _| {
                if let Some(bounds) = bounds.first() { geometry.borrow_mut().insert("innerRegionWrapper", rect(*bounds)); }
                if let Some(bounds) = bounds.get(1) { geometry.borrow_mut().insert("expand", rect(*bounds)); }
            }).id("tabstrip-fixture-outer").relative().w(px(width)).h(px(Theme::TITLEBAR_HEIGHT))
                .pt(px(Theme::TITLEBAR_TOP_PAD)).pl(px(8.0)).pr(px(4.0))
                .flex().flex_row().items_center().gap(px(4.0)).overflow_hidden()
                .child(div().flex_1().min_w_0().h_full().overflow_hidden().child(strip))
                .child(expand)
                .child(canvas(move |bounds, _, _| {
                    host_geometry.borrow_mut().insert("outerHost", rect(bounds));
                    frames.set(frames.get() + 1);
                }, |_, _, _, _| {}).absolute().inset_0())
        });
        div().id("tabstrip-fixture-root").relative().size_full().bg(theme.surface)
            .text_color(theme.text).font_family(theme.font_sans.clone()).text_size(crate::typography::ui_rems(14.0))
            .child(div().absolute().left(px(self.style_case.host_origin()[0])).top(px(self.style_case.host_origin()[1])).child(host))
    }
}
