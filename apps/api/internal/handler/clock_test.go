package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestCheckKeycloakClockSkew_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Standard Date header — same as our local clock.
		w.WriteHeader(200)
	}))
	defer srv.Close()
	t.Setenv("KEYCLOAK_INTERNAL_URL", srv.URL)
	skew, err := CheckKeycloakClockSkew(context.Background())
	if err != nil {
		t.Fatalf("skew: %v", err)
	}
	abs := skew
	if abs < 0 {
		abs = -abs
	}
	if abs > 5*time.Second {
		t.Errorf("expected near-zero skew, got %s", skew)
	}
}

func TestCheckKeycloakClockSkew_OffsetDate(t *testing.T) {
	// Server reports Date 90s in the past — local clock should appear ahead.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := time.Now().Add(-90 * time.Second).UTC().Format(http.TimeFormat)
		w.Header().Set("Date", offset)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	t.Setenv("KEYCLOAK_INTERNAL_URL", srv.URL)
	skew, err := CheckKeycloakClockSkew(context.Background())
	if err != nil {
		t.Fatalf("skew: %v", err)
	}
	if skew < 60*time.Second || skew > 120*time.Second {
		t.Errorf("expected ~90s skew, got %s", skew)
	}
}

func TestCheckSkewForReadiness_HysteresisAndPass(t *testing.T) {
	// Server skews by 90s — outside threshold, should warn but not fail until
	// 3 consecutive polls.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", time.Now().Add(-90*time.Second).UTC().Format(http.TimeFormat))
		w.WriteHeader(200)
	}))
	defer srv.Close()
	os.Setenv("KEYCLOAK_INTERNAL_URL", srv.URL)
	defer os.Unsetenv("KEYCLOAK_INTERNAL_URL")

	// Reset hysteresis from any previous test.
	skewFailureCount.Store(0)

	for i := 0; i < 2; i++ {
		_, ok := CheckSkewForReadiness(context.Background())
		if !ok {
			t.Errorf("poll %d should still pass (warn only)", i+1)
		}
	}
	_, ok := CheckSkewForReadiness(context.Background())
	if ok {
		t.Errorf("3rd consecutive poll should fail readiness")
	}

	// A successful poll resets the counter.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200) // implicit Date == now
	}))
	defer srv2.Close()
	os.Setenv("KEYCLOAK_INTERNAL_URL", srv2.URL)
	_, ok = CheckSkewForReadiness(context.Background())
	if !ok {
		t.Errorf("good poll should pass")
	}
	if skewFailureCount.Load() != 0 {
		t.Errorf("counter should reset to 0 after good poll, got %d", skewFailureCount.Load())
	}
}

func TestCheckSkewForReadiness_KeycloakUnreachable_DoesNotFail(t *testing.T) {
	// Point at an unreachable URL.
	t.Setenv("KEYCLOAK_INTERNAL_URL", "http://127.0.0.1:1") // port 1 always closed
	skewFailureCount.Store(0)
	check, ok := CheckSkewForReadiness(context.Background())
	if !ok {
		t.Errorf("Keycloak unreachable should NOT fail readiness; got %q", check)
	}
}
