//! Fixture-only adapter in the exported zeron-ui crate. Production components
//! and renderer are imported unchanged; no engine or external service starts.
use crate::{account_usage, settings::accounts::AccountsSnapshotCache, state::AppState, theme};
use gpui::{App, AppContext, Bounds, Context, Entity, IntoElement, Pixels, Render, Window, canvas, div, prelude::*, px};
use std::{cell::RefCell, collections::BTreeMap, rc::Rc};
use zeron_proto::{AgentAccountsSnapshot, ContextUsage, HarnessId};

type Geometry = Rc<RefCell<BTreeMap<&'static str, [f32; 4]>>>;

pub struct FooterFixture {
    state: Entity<AppState>,
    usage: Entity<account_usage::AccountUsage>,
    geometry: Geometry,
    scene: String,
    interaction: Option<serde_json::Value>,
    pointer_position: [f32; 2],
}

fn observed_bounds(records: Geometry, name: &'static str) -> impl IntoElement {
    canvas(
        move |bounds: Bounds<Pixels>, _, _| {
            records.borrow_mut().insert(name, [
                f32::from(bounds.origin.x), f32::from(bounds.origin.y),
                f32::from(bounds.size.width), f32::from(bounds.size.height),
            ]);
        },
        |_, _, _, _| {},
    ).absolute().inset_0()
}

impl FooterFixture {
    pub fn new(cx: &mut Context<Self>) -> Self {
        let accounts: AgentAccountsSnapshot = serde_json::from_value(serde_json::json!({
            "accounts": [
                {"id": "a", "harness": "codex", "email": "first@example.test", "planLabel": "Pro", "active": true, "switchable": true,
                 "usageWindows": [{"label": "Session", "usedFraction": 0.2}, {"label": "Week", "usedFraction": 0.4}, {"label": "Month", "usedFraction": 0.9}]},
                {"id": "b", "harness": "codex", "displayName": "Second account", "active": false, "switchable": true, "usageWindows": []}
            ], "warnings": []
        })).expect("fixture accounts must match the frozen public DTO");
        cx.default_global::<AccountsSnapshotCache>().0.insert(None, accounts);
        let state = cx.new(|_| AppState::new());
        let usage = cx.new(|cx| account_usage::AccountUsage::new(state.clone(), cx));
        let mut view = Self { state, usage, geometry: Rc::default(), scene: String::new(), interaction: None, pointer_position: [0.0, 0.0] };
        view.set_scene("known", cx);
        view
    }

    pub fn set_scene(&mut self, scene: &str, cx: &mut Context<Self>) {
        let usage = match scene {
            "known" => Some(ContextUsage { tokens: Some(42000), window: Some(200000) }),
            "capacity" => Some(ContextUsage { tokens: None, window: Some(200000) }),
            "zero" => Some(ContextUsage { tokens: Some(0), window: Some(200000) }),
            "over" => Some(ContextUsage { tokens: Some(240000), window: Some(200000) }),
            "warning" => Some(ContextUsage { tokens: Some(150000), window: Some(200000) }),
            "danger" => Some(ContextUsage { tokens: Some(180000), window: Some(200000) }),
            "unknown" => None,
            _ => panic!("unknown footer fixture scene {scene}"),
        };
        self.state.update(cx, |state, cx| { state.context_usage = usage; cx.notify(); });
        self.scene = scene.into();
        self.interaction = None;
        // Recreate only the production view, to start every scenario with its
        // real popup closed. Its public constructor and track stay unchanged.
        let state = self.state.clone();
        self.usage = cx.new(|cx| account_usage::AccountUsage::new(state, cx));
        cx.notify();
    }

    pub fn click_position(&mut self, card: &str, cx: &App) -> anyhow::Result<[f32; 2]> {
        let rect = *self.geometry.borrow().get("cluster").ok_or_else(|| anyhow::anyhow!("production cluster has not been laid out"))?;
        anyhow::ensure!(rect[2] > 24.0 && (rect[3] - 24.0).abs() < 0.1, "invalid production cluster bounds");
        let context_visible = crate::context_usage::has_window(self.state.read(cx).context_usage);
        let account_visible = cx.global::<AccountsSnapshotCache>().0.get(&None)
            .and_then(|snapshot| account_usage::active_account(snapshot, HarnessId::Codex))
            .and_then(account_usage::used_fraction).is_some();
        let x = match card {
            // The first and last production chips own these one-pixel inset
            // points, inside their existing six-pixel horizontal padding.
            "accounts" if account_visible => rect[0] + 1.0,
            "context" if context_visible => rect[0] + rect[2] - 1.0,
            _ => anyhow::bail!("this scene has no requested production trigger"),
        };
        let point = [x, rect[1] + rect[3] / 2.0];
        self.interaction = Some(serde_json::json!({
            "requestedCard": card, "path": "public Window::dispatch_event MouseDown/MouseUp; production on_click",
            "pointLogical": point, "coordinateSource": "actual wrapper canvas layout bounds, one pixel inside the first/last production chip",
            "actualClusterBoundsLogical": rect, "expectedAccountVisibility": account_visible, "expectedContextVisibility": context_visible,
            "singleRingBoundsQueriedByElementId": false, "osMouseAndActivationAcceptance": "not established by synthetic event dispatch",
            "privatePopupFieldsReadOrChanged": false,
        }));
        Ok(point)
    }

    pub fn record_pointer(&mut self, position: [f32; 2]) {
        self.pointer_position = position;
    }

    pub fn snapshot(&self, window: &Window, cx: &App) -> serde_json::Value {
        let context = self.state.read(cx).context_usage;
        let cache = cx.global::<AccountsSnapshotCache>();
        let accounts = cache.0.get(&None).expect("fixture accounts cache");
        let active = account_usage::active_account(accounts, HarnessId::Codex);
        serde_json::json!({
            "scene": self.scene, "harness": "codex", "targetId": "local", "referenceTarget": null,
            "context": context, "contextFraction": context.and_then(ContextUsage::fraction),
            "hasContextWindow": crate::context_usage::has_window(context),
            "accounts": accounts.accounts, "activeAccountId": active.map(|a| a.id.as_str()),
            "accountFraction": active.and_then(account_usage::used_fraction),
            "geometryLogical": *self.geometry.borrow(), "interaction": self.interaction,
            "lastSyntheticPointerLogical": self.pointer_position,
            "pointerResetPolicy": "public GPUI synthetic mouse move to root blank [10,10] after each scene reset and trigger click; OS pointer untouched",
            "nativeWindowAppearance": format!("{:?}", window.appearance()).to_lowercase(),
            "nativeWindowActive": window.is_window_active(), "nativeWindowTitle": window.window_title(),
            "scaffold": {"clientSize": [840, 560], "rootPadding": 24, "footerHeight": 24, "footerAlign": "end", "font": "Geist", "fontSize": 13, "letterSpacing": 0},
            "productionView": "account_usage::AccountUsage::new + track(Some(HarnessId::Codex), None)",
            "contextProductionView": "context_usage::chip/card via unmodified AccountUsage",
            "engineBootstrapped": false, "popupPrivateStateAcceptance": "observe the captured production card; no private getter or adapter-written popup flag",
        })
    }
}

impl Render for FooterFixture {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = theme::Theme::of(cx).clone();
        self.usage.update(cx, |usage, cx| usage.track(Some(HarnessId::Codex), None, cx));
        div().id("footer-fixture").relative().size_full().bg(theme.surface).text_color(theme.text)
            .font_family(theme.font_sans.clone()).text_size(px(13.0)).p(px(24.0))
            .child(observed_bounds(self.geometry.clone(), "root"))
            .child(div().size_full().flex().flex_col().justify_end().child(
                div().relative().w_full().h(px(24.0)).flex().items_center().justify_end()
                    .child(observed_bounds(self.geometry.clone(), "footer"))
                    .child(div().relative().flex_none().child(self.usage.clone()).child(observed_bounds(self.geometry.clone(), "cluster"))),
            ))
    }
}
