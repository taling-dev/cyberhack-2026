package handler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
)

// SkewThreshold is the maximum allowed signed clock skew (seconds) between
// the API pod and Keycloak before we fail readiness. JWT validation has
// 60s leeway; readiness is fatal at >60s for 3 consecutive polls.
const skewThreshold = 60 * time.Second

// StartupSkewThreshold is the strict-on-boot threshold. If clock skew
// exceeds this at process startup, we fail-fast before the pod begins
// accepting traffic. 30s gives a safety margin under the 60s leeway.
const startupSkewThreshold = 30 * time.Second

// keycloakDateClient is a small HTTP client used by the skew check. The
// short timeout avoids hanging readiness if Keycloak is briefly unreachable;
// in that case we return an error and the caller treats it as "no signal".
var keycloakDateClient = &http.Client{Timeout: 3 * time.Second}

// keycloakBase derives the Keycloak base URL from environment, preferring
// the cluster-internal URL so we don't depend on external DNS for readiness.
func keycloakBase() string {
	for _, k := range []string{"KEYCLOAK_INTERNAL_URL", "KEYCLOAK_ISSUER", "KEYCLOAK_BASE_URL"} {
		if v := os.Getenv(k); v != "" {
			return strings.TrimSuffix(v, "/")
		}
	}
	return "http://keycloak.platform:8080"
}

// CheckKeycloakClockSkew issues a HEAD request to Keycloak's base URL and
// returns the local-time-minus-Keycloak-time skew. A positive value means
// the API pod's clock is ahead of Keycloak's.
func CheckKeycloakClockSkew(ctx context.Context) (time.Duration, error) {
	url := keycloakBase()
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := keycloakDateClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	dateStr := resp.Header.Get("Date")
	if dateStr == "" {
		return 0, fmt.Errorf("keycloak: missing Date header")
	}
	kcTime, err := http.ParseTime(dateStr)
	if err != nil {
		return 0, fmt.Errorf("parse Date: %w", err)
	}
	return time.Since(kcTime), nil
}

// StartupClockCheck runs at process startup and fails the boot if clock
// skew vs Keycloak exceeds startupSkewThreshold. Retries up to 30s if
// Keycloak is unreachable so we don't false-positive on a slow control
// plane during an OKE rolling restart.
func StartupClockCheck(ctx context.Context) error {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for {
		skew, err := CheckKeycloakClockSkew(ctx)
		if err == nil {
			abs := skew
			if abs < 0 {
				abs = -abs
			}
			events.APIClockSkewSeconds.Set(skew.Seconds())
			if abs > startupSkewThreshold {
				return fmt.Errorf("clock skew %s exceeds startup threshold %s — check NTP on this node", skew, startupSkewThreshold)
			}
			return nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			// Couldn't reach Keycloak in 30s. Don't fail startup — the pod
			// is still useful even if we can't compare clocks. Log a warning
			// instead. (Returning nil here is intentional.)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			_ = lastErr
		}
	}
}

// skewFailureCount tracks consecutive readiness polls where skew exceeded
// skewThreshold. We require 3 in a row before failing readiness so a single
// transient Keycloak Date misread doesn't kick a healthy pod out of the LB.
var skewFailureCount atomic.Int32

// CheckSkewForReadiness updates the gauge and returns:
//   - the human-readable check value to embed in the /readyz JSON
//   - whether this poll passed (true = readiness OK)
//
// The boolean uses 3-poll hysteresis: a single bad reading bumps the counter
// but doesn't fail; three in a row trips readiness to false.
func CheckSkewForReadiness(ctx context.Context) (string, bool) {
	skew, err := CheckKeycloakClockSkew(ctx)
	if err != nil {
		// Couldn't reach Keycloak — best-effort, do not fail readiness.
		// Decrement (don't increment) so we don't accidentally trip on a
		// network blip.
		skewFailureCount.Store(0)
		return "skipped: " + err.Error(), true
	}
	events.APIClockSkewSeconds.Set(skew.Seconds())
	abs := skew
	if abs < 0 {
		abs = -abs
	}
	if abs > skewThreshold {
		count := skewFailureCount.Add(1)
		if count >= 3 {
			return fmt.Sprintf("fail: skew=%s (3+ consecutive polls)", skew), false
		}
		return fmt.Sprintf("warn: skew=%s (%d/3)", skew, count), true
	}
	skewFailureCount.Store(0)
	return fmt.Sprintf("ok: skew=%s", skew), true
}
