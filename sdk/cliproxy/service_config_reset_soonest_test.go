package cliproxy

import (
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestResetSoonestRoutingSelector(t *testing.T) {
	state := normalizedRoutingRuntimeState(&internalconfig.Config{
		Routing: internalconfig.RoutingConfig{Strategy: " Reset-Soonest "},
	})
	if state.strategy != "reset-soonest" {
		t.Fatalf("strategy = %q, want reset-soonest", state.strategy)
	}
	if _, ok := newRoutingSelector(state).(*coreauth.ResetSoonestSelector); !ok {
		t.Fatalf("selector type = %T, want *auth.ResetSoonestSelector", newRoutingSelector(state))
	}

	state = normalizedRoutingRuntimeState(&internalconfig.Config{
		Routing: internalconfig.RoutingConfig{Strategy: "reset-soonest", SessionAffinity: true},
	})
	if _, ok := newRoutingSelector(state).(*coreauth.SessionAffinitySelector); !ok {
		t.Fatalf("selector type = %T, want *auth.SessionAffinitySelector wrapping reset-soonest", newRoutingSelector(state))
	}
}
