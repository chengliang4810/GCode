package contracts

import "testing"

func TestComposerAccountRefreshAllowsWholeTargetOnlyForList(t *testing.T) {
	for _, kind := range []string{"provider.accounts.refresh", "provider.account.switch", "provider.account.remove", "provider.install", "provider.login.start"} {
		command := Command{Kind: kind, Integration: &IntegrationRequest{TargetID: "local"}}
		err := command.Validate()
		if (err == nil) != (kind == "provider.accounts.refresh") {
			t.Fatalf("provider-less %s: %v", kind, err)
		}
	}
	if err := (Command{Kind: "provider.accounts.refresh"}).Validate(); err == nil {
		t.Fatal("missing typed list request accepted")
	}
}
