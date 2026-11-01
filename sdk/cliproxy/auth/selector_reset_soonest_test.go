package auth

import (
	"context"
	"strconv"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

var resetSoonestTestNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func resetSoonestAuth(id string, reset time.Time) *Auth {
	auth := &Auth{ID: id, Provider: "claude"}
	if !reset.IsZero() {
		auth.Quota.Signals = map[string]string{
			"Anthropic-Ratelimit-Unified-7d-Reset": strconv.FormatInt(reset.Unix(), 10),
		}
	}
	return auth
}

func newTestResetSoonestSelector() *ResetSoonestSelector {
	return &ResetSoonestSelector{nowFunc: func() time.Time { return resetSoonestTestNow }}
}

func TestResetSoonestSelector_PicksEarliestWeeklyReset(t *testing.T) {
	t.Parallel()

	selector := newTestResetSoonestSelector()
	auths := []*Auth{
		resetSoonestAuth("auth-a", resetSoonestTestNow.Add(5*24*time.Hour)),
		resetSoonestAuth("auth-b", resetSoonestTestNow.Add(2*24*time.Hour)),
		resetSoonestAuth("auth-c", resetSoonestTestNow.Add(6*24*time.Hour)),
	}
	for i := 0; i < 3; i++ {
		got, err := selector.Pick(context.Background(), "claude", "claude-sonnet-4", cliproxyexecutor.Options{}, auths)
		if err != nil {
			t.Fatalf("Pick() #%d error = %v", i, err)
		}
		if got.ID != "auth-b" {
			t.Fatalf("Pick() #%d = %q, want auth-b", i, got.ID)
		}
	}
}

func TestResetSoonestSelector_KnownResetBeatsUnknownAndExpired(t *testing.T) {
	t.Parallel()

	selector := newTestResetSoonestSelector()
	auths := []*Auth{
		resetSoonestAuth("auth-a", time.Time{}),
		resetSoonestAuth("auth-b", resetSoonestTestNow.Add(-time.Hour)),
		resetSoonestAuth("auth-c", resetSoonestTestNow.Add(6*24*time.Hour)),
	}
	got, err := selector.Pick(context.Background(), "claude", "claude-sonnet-4", cliproxyexecutor.Options{}, auths)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if got.ID != "auth-c" {
		t.Fatalf("Pick() = %q, want auth-c", got.ID)
	}
}

func TestResetSoonestSelector_UnknownResetsUseRoundRobin(t *testing.T) {
	t.Parallel()

	selector := newTestResetSoonestSelector()
	invalid := resetSoonestAuth("auth-c", time.Time{})
	invalid.Quota.Signals = map[string]string{"Anthropic-Ratelimit-Unified-7d-Reset": "soon"}
	auths := []*Auth{
		resetSoonestAuth("auth-a", time.Time{}),
		resetSoonestAuth("auth-b", resetSoonestTestNow.Add(-time.Minute)),
		invalid,
	}
	want := []string{"auth-a", "auth-b", "auth-c", "auth-a"}
	for i, wantID := range want {
		got, err := selector.Pick(context.Background(), "claude", "claude-sonnet-4", cliproxyexecutor.Options{}, auths)
		if err != nil {
			t.Fatalf("Pick() #%d error = %v", i, err)
		}
		if got.ID != wantID {
			t.Fatalf("Pick() #%d = %q, want %q", i, got.ID, wantID)
		}
	}
}

func TestResetSoonestSelector_SkipsCredentialInCooldown(t *testing.T) {
	t.Parallel()

	selector := newTestResetSoonestSelector()
	cooling := resetSoonestAuth("auth-a", resetSoonestTestNow.Add(24*time.Hour))
	cooling.Quota.Exceeded = true
	cooling.Quota.Reason = "credential_quota"
	cooling.Quota.NextRecoverAt = resetSoonestTestNow.Add(time.Hour)
	auths := []*Auth{
		cooling,
		resetSoonestAuth("auth-b", resetSoonestTestNow.Add(3*24*time.Hour)),
		resetSoonestAuth("auth-c", resetSoonestTestNow.Add(2*24*time.Hour)),
	}
	got, err := selector.Pick(context.Background(), "claude", "claude-sonnet-4", cliproxyexecutor.Options{}, auths)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if got.ID != "auth-c" {
		t.Fatalf("Pick() = %q, want auth-c", got.ID)
	}
}

func TestResetSoonestSelector_SessionAffinityKeepsBinding(t *testing.T) {
	t.Parallel()

	selector := NewSessionAffinitySelector(newTestResetSoonestSelector())
	authA := resetSoonestAuth("auth-a", resetSoonestTestNow.Add(2*24*time.Hour))
	authB := resetSoonestAuth("auth-b", resetSoonestTestNow.Add(4*24*time.Hour))
	auths := []*Auth{authA, authB}
	sessionOne := cliproxyexecutor.Options{OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_ac980658-63bd-4fb3-97ba-8da64cb1e344"}}`)}
	sessionTwo := cliproxyexecutor.Options{OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_5d1c7a4e-2f0b-4c7e-9a51-0b6f3e2d8c19"}}`)}

	first, err := selector.Pick(context.Background(), "claude", "claude-sonnet-4", sessionOne, auths)
	if err != nil {
		t.Fatalf("Pick() session one error = %v", err)
	}
	if first.ID != "auth-a" {
		t.Fatalf("Pick() session one = %q, want auth-a", first.ID)
	}

	// auth-b now resets first: a new session follows the strategy, the bound one stays.
	authB.Quota.Signals["Anthropic-Ratelimit-Unified-7d-Reset"] = strconv.FormatInt(resetSoonestTestNow.Add(24*time.Hour).Unix(), 10)

	again, err := selector.Pick(context.Background(), "claude", "claude-sonnet-4", sessionOne, auths)
	if err != nil {
		t.Fatalf("Pick() session one again error = %v", err)
	}
	if again.ID != "auth-a" {
		t.Fatalf("Pick() session one again = %q, want auth-a (binding)", again.ID)
	}
	second, err := selector.Pick(context.Background(), "claude", "claude-sonnet-4", sessionTwo, auths)
	if err != nil {
		t.Fatalf("Pick() session two error = %v", err)
	}
	if second.ID != "auth-b" {
		t.Fatalf("Pick() session two = %q, want auth-b", second.ID)
	}
}
