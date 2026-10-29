//! Feature-only DTO staging bridge. The editor, Change subscription and dirty
//! calculation remain the original FilesSurface implementation.
use super::*;

impl FilesSurface {
    pub(crate) fn tabstrip_fixture_load_text(
        &mut self,
        path: &str,
        file: zeron_proto::WorkspaceFileText,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> anyhow::Result<gpui::Entity<gpui_base::input::EditorState>> {
        anyhow::ensure!(self.state.read(cx).engine().is_none(), "fixture cannot use a real engine");
        anyhow::ensure!(file.path == path, "typed read reply path differs from editor path");
        let context = self.request_context.as_ref().ok_or_else(|| anyhow::anyhow!("missing real file request context"))?;
        let mut document = FileDocument::loading(document_key(context, path.to_string()));
        document.set_loaded(file);
        anyhow::ensure!(document.is_editable() && !document.is_dirty(), "typed read reply must create a clean editable document");
        self.set_autosave_enabled(false, cx);
        self.preview.documents.insert(path.to_string(), document);
        self.preview.active = Some(path.to_string());
        self.sync_preview_list();
        let theme = Theme::of(cx).clone();
        let editor = self.ensure_editor(path, &theme, window, cx).ok_or_else(|| anyhow::anyhow!("original ensure_editor did not create editor"))?;
        cx.notify();
        Ok(editor)
    }

    pub(crate) fn tabstrip_fixture_document(&self, path: &str, cx: &App) -> anyhow::Result<serde_json::Value> {
        let document = self.preview.documents.get(path).ok_or_else(|| anyhow::anyhow!("missing file document"))?;
        let editor = document.editor.as_ref().ok_or_else(|| anyhow::anyhow!("missing original file editor"))?;
        Ok(serde_json::json!({
            "path":path, "chatId":self.chat_id(), "revision":document.revision,
            "savedRevision":document.saved_revision, "isDirty":document.is_dirty(),
            "phase":format!("{:?}",document.phase), "editorText":editor.read(cx).value(),
            "originalChangeSubscriptionPresent":document.editor_events.is_some(),
            "originalEditorObserverPresent":document.editor_observer.is_some(),
            "pendingSave":document.pending_save.is_some(), "autosaveTask":document.autosave_task.is_some(),
            "readReplyInjected":true, "diskOrFileRpcValidated":false,
        }))
    }
}
