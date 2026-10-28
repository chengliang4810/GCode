package contracts

import "testing"

func TestSearchRangesPreserveOriginalUnicodeBytes(t *testing.T) {
	text := "中文 İNDEX needle"
	ranges := SearchRanges(text, "index needle INDEX")
	if len(ranges) != 2 {
		t.Fatalf("deduplicated ranges: %+v", ranges)
	}
	if text[ranges[0].Start:ranges[0].End] != "İNDEX" || text[ranges[1].Start:ranges[1].End] != "needle" {
		t.Fatalf("byte ranges: %+v", ranges)
	}
}
func TestProjectCreateCommandOwnsPayload(t *testing.T) {
	command := Command{Kind: "project.create", ProjectCreate: &ProjectCreate{DeviceID: "local", Name: "Folder", Path: "~/Folder"}}
	copy := command.Clone()
	command.ProjectCreate.Name = "Changed"
	if copy.ProjectCreate.Name != "Folder" {
		t.Fatal("queued command aliases caller")
	}
	copy.Kind = "file.save"
	if copy.Validate() == nil {
		t.Fatal("mismatched tagged payload accepted")
	}
}
