package auth

import (
	"context"
	"strconv"
	"strings"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

// weeklyResetSignalHeader carries the Unix time (seconds) at which the Claude
// subscription's seven-day usage window resets.
const weeklyResetSignalHeader = "anthropic-ratelimit-unified-7d-reset"

// ResetSoonestSelector prefers the available credential whose seven-day quota
// window resets first, so the quota that would expire soonest is used before
// quota that lasts longer.
//
// The reset time comes from the passive quota snapshot (Auth.Quota.Signals)
// recorded from Claude responses. Credentials without a known future reset are
// used in round-robin order, but only when no credential with a known reset is
// available. Session affinity, when enabled, wraps this selector and keeps its
// bindings ahead of it.
type ResetSoonestSelector struct {
	fallback RoundRobinSelector
	nowFunc  func() time.Time
}

// Pick selects the available credential with the earliest future weekly reset.
func (s *ResetSoonestSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	now := time.Now()
	if s.nowFunc != nil {
		now = s.nowFunc()
	}
	available, err := getSelectorAvailableAuths(ctx, auths, provider, model, now)
	if err != nil {
		return nil, err
	}

	var best *Auth
	var bestReset time.Time
	unknown := make([]*Auth, 0, len(available))
	for _, candidate := range available {
		reset, ok := weeklyResetFromSignals(candidate, now)
		if !ok {
			unknown = append(unknown, candidate)
			continue
		}
		// Candidates arrive sorted by ID, so a strict comparison keeps ties stable.
		if best == nil || reset.Before(bestReset) {
			best, bestReset = candidate, reset
		}
	}
	if best != nil {
		return best, nil
	}
	return s.fallback.Pick(ctx, provider, model, opts, unknown)
}

// weeklyResetFromSignals returns the observed seven-day reset time of the
// credential when it is known and still in the future. A reset in the past
// means the window has already rolled over and the next reset is unknown.
func weeklyResetFromSignals(auth *Auth, now time.Time) (time.Time, bool) {
	if auth == nil || len(auth.Quota.Signals) == 0 {
		return time.Time{}, false
	}
	for name, value := range auth.Quota.Signals {
		if !strings.EqualFold(name, weeklyResetSignalHeader) {
			continue
		}
		seconds, errParse := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if errParse != nil || seconds <= 0 {
			return time.Time{}, false
		}
		reset := time.Unix(seconds, 0)
		if !reset.After(now) {
			return time.Time{}, false
		}
		return reset, true
	}
	return time.Time{}, false
}
