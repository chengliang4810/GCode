package providers

import (
	"context"

	"gcode/internal/config"
	"gcode/internal/zeroruntime"
)

// EscalationSwitchers builds the provider model switcher a run installs when it
// has opted into escalation. The second return value remains nil because the
// unified runtime factory owns the default turn-session wrapper.
//
// THE NIL CONTRACTS ARE THE CONTRACT. The agent loop reassigns the provider only
// when a switcher returns a non-nil one, so a (nil, nil) return means "no swap"
// and has to leave everything as it was, including whatever the caller tracks
// through onSwitch. onSwitch therefore fires only on a real swap, and may be nil.
// An error is reported to the loop, which records a note and continues on the
// current model.
//
// Model escalation only swaps the provider. The runtime's unified factory owns
// the default turn-session wrapper, so an escalation never switches into a
// provider-specific session implementation.
func EscalationSwitchers(
	profile config.ProviderProfile,
	_ zeroruntime.Provider,
	newProvider func(config.ProviderProfile) (zeroruntime.Provider, error),
	onSwitch func(modelID string),
) (
	func(context.Context, string) (zeroruntime.Provider, error),
	func(context.Context, string) (zeroruntime.TurnSessionProvider, error),
) {
	if newProvider == nil {
		return nil, nil
	}
	// The escalated profile is the run's profile with the model replaced, so the
	// credential, base URL and headers travel with it. Callers pass a newProvider
	// that already applies the stored key, which is why there is no per-site key
	// handling here.
	switchTo := func(modelID string) (config.ProviderProfile, zeroruntime.Provider, error) {
		switched := profile
		switched.Model = modelID
		provider, err := newProvider(switched)
		return switched, provider, err
	}

	modelSwitcher := func(_ context.Context, modelID string) (zeroruntime.Provider, error) {
		_, switchedProvider, err := switchTo(modelID)
		if err != nil {
			return nil, err
		}
		if switchedProvider == nil {
			return nil, nil
		}
		if onSwitch != nil {
			onSwitch(modelID)
		}
		return switchedProvider, nil
	}

	return modelSwitcher, nil
}
