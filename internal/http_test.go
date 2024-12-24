package internal

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"net/http"
	"os"
	"testing"
)

func setupHttpTest() {
	registry := prometheus.NewRegistry()
	go Serve(8080, registry)
}

func TestMain(m *testing.M) {
	setupHttpTest()
	code := m.Run()
	os.Exit(code)
}

func TestServeMetricsEndpointExpectSuccess(t *testing.T) {
	resp, err := http.Get("http://localhost:8080/metrics")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServeRandomEndpointExpectNotFound(t *testing.T) {
	resp, err := http.Get("http://localhost:8080/other")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
