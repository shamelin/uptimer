package internal

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func setupSeeker(server *httptest.Server) *SeekerImpl {
	registerer := prometheus.NewRegistry()
	seeker, err := NewSeeker(
		Host{
			Host: server.URL,
		},
		registerer,
	)

	if err != nil {
		panic(err)
	}

	return seeker
}

func TestSeekerPersistCookieJar(t *testing.T) {
	var visited bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !visited {
			visited = true
			http.SetCookie(w, &http.Cookie{
				Name: "test",
			})
		} else {
			assert.NotNil(t, r.Header.Get("Cookie"), "Expected cookie to be set")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	seeker := setupSeeker(server)
	// First request to set the cookie
	seeker.check()

	// Second request to check if the cookie is persisted
	seeker.check()
}

func TestSeekerImplCheckUptimeWithSuccessExpectUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	seeker := setupSeeker(server)
	seeker.check()

	upGauge := testutil.ToFloat64(seeker.up)
	if upGauge != 1 {
		t.Errorf("Expected upGauge to be 1, got %v", upGauge)
	}

	latencyGauge := testutil.ToFloat64(seeker.latency)
	if latencyGauge == 0 {
		t.Errorf("Expected latencyGauge to have observations, got %v", latencyGauge)
	}
}

func TestSeekerImplCheckUptimeWith400ExpectDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	seeker := setupSeeker(server)
	seeker.check()

	upGauge := testutil.ToFloat64(seeker.up)
	if upGauge != 0 {
		t.Errorf("Expected upGauge to be 0, got %v", upGauge)
	}
}

func TestSeekerImplCheckUptimeWith500ExpectDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	seeker := setupSeeker(server)
	seeker.check()

	upGauge := testutil.ToFloat64(seeker.up)
	if upGauge != 0 {
		t.Errorf("Expected upGauge to be 0, got %v", upGauge)
	}

	latencyGauge := testutil.ToFloat64(seeker.latency)
	if latencyGauge != 0 {
		t.Errorf("Expected latencyGauge to be 0, got %v", latencyGauge)
	}
}

func TestSeekerImplCheckUptimeWithHighLatencyExpectInHistogram(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	seeker := setupSeeker(server)
	seeker.check()

	latencyGauge := testutil.ToFloat64(seeker.latency)
	if latencyGauge == 0 {
		t.Errorf("Expected latencyGauge to have observations, got %v", latencyGauge)
	}

	// Gather the metrics and inspect the gauge values
	metric := &dto.Metric{}
	err := seeker.latency.Write(metric)
	if err != nil {
		t.Fatalf("Failed to write gauge metric: %v", err)
	}

	observedLatency := metric.GetGauge().GetValue()
	if observedLatency < 500 {
		t.Errorf("Expected latency to be at least 500ms, got %vms", observedLatency)
	}
}
