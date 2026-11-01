package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestManagerNetworkFailureRedactsSourceCredentials(t *testing.T) {
	t.Parallel()
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	address := "http://PRIVATE_USER:PRIVATE_PASSWORD@127.0.0.1:1/marketplace.json?token=PRIVATE_QUERY"
	if err := manager.PutSource(context.Background(), MarketplaceSource{ID: "private", Name: "private", Enabled: true,
		Source: Source{Source: "url", URL: address}, Headers: map[string]string{"Authorization": "Bearer PRIVATE_HEADER"}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = manager.Refresh(ctx, "private")
	if err == nil {
		t.Fatal("unreachable source unexpectedly succeeded")
	}
	for _, secret := range []string{"PRIVATE_USER", "PRIVATE_PASSWORD", "PRIVATE_QUERY", "PRIVATE_HEADER"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("source credential reached public operation error: %s", secret)
		}
	}
}

func TestSourceFailureRedactsGitAndHeaderTextAndPreservesCause(t *testing.T) {
	original := fmt.Errorf("git failed https://USER:PASSWORD@example.invalid/repo?token=QUERY: authorization Bearer HEADER: %w", context.Canceled)
	err := redactSourceError(original, nil, map[string]string{"Authorization": "Bearer HEADER"})
	for _, secret := range []string{"USER", "PASSWORD", "QUERY", "HEADER"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("credential leaked:", secret)
		}
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("redaction lost the cancellation cause")
	}
}
