package contracts

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestProviderReorderCommandsValidateAndClone(t *testing.T) {
	valid := ReorderInput{SourceID: "third", TargetID: "first", After: true}
	for _, tc := range []struct {
		name    string
		command Command
		wantErr bool
	}{
		{name: "providers", command: Command{Kind: "provider.reorder", Reorder: &valid}},
		{name: "models", command: Command{Kind: "provider.models.reorder", Reorder: &ReorderInput{
			ProviderID: "custom", SourceID: "model/third", TargetID: "model/first",
		}}},
		{name: "missing payload", command: Command{Kind: "provider.reorder"}, wantErr: true},
		{name: "missing target", command: Command{Kind: "provider.reorder", Reorder: &ReorderInput{
			SourceID: "first",
		}}, wantErr: true},
		{name: "empty source", command: Command{Kind: "provider.reorder", Reorder: &ReorderInput{
			SourceID: "  ", TargetID: "first",
		}}, wantErr: true},
		{name: "missing provider", command: Command{Kind: "provider.models.reorder", Reorder: &valid}, wantErr: true},
		{name: "extra provider scope", command: Command{Kind: "provider.reorder", Reorder: &ReorderInput{
			ProviderID: "custom", SourceID: "third", TargetID: "first",
		}}, wantErr: true},
		{name: "wrong command", command: Command{Kind: "catalog.retry", Reorder: &valid}, wantErr: true},
		{name: "resource early return", command: Command{Kind: "resource.refresh", Reorder: &valid}, wantErr: true},
		{name: "unrelated configuration", command: Command{
			Kind: "provider.reorder", Reorder: &valid, Settings: &Settings{},
		}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.command.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, want error %t", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			clone := tc.command.Clone()
			clone.Reorder.SourceID = "changed"
			if tc.command.Reorder.SourceID == "changed" {
				t.Fatal("command clone shares reorder payload")
			}
			data, err := json.Marshal(tc.command)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Command
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded, tc.command) {
				t.Fatal("remote command encoding lost reorder payload")
			}
		})
	}
}
