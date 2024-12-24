package internal

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// UptimeChecker is the interface that defines the methods to check the uptime of a remote host.
type UptimeChecker interface {
	CheckUptime()
	hookSignal(cancel context.CancelFunc)
	check()
}

// SeekerImpl is the implementation of the UptimeChecker interface. It is responsible for checking the uptime of a remote host.
type SeekerImpl struct {
	logger       *logrus.Entry
	httpClient   *http.Client
	host         string
	interval     int
	up           prometheus.Gauge
	latency      prometheus.Gauge
	statusCode   prometheus.Gauge
	previouslyUp bool
}

// NewSeeker creates a new SeekerImpl instance.
func NewSeeker(host Host, registerer prometheus.Registerer) *SeekerImpl {
	logger := logrus.WithFields(logrus.Fields{
		"component": "seeker",
	})

	upCounter := promauto.With(registerer).NewGauge(prometheus.GaugeOpts{
		Name: "uptime_up",
		Help: "Whether the host is up or not.",
	})

	latency := promauto.With(registerer).NewGauge(prometheus.GaugeOpts{
		Name: "uptime_latency",
		Help: "The latency between the server and the remote host.",
	})

	statusCode := promauto.With(registerer).NewGauge(prometheus.GaugeOpts{
		Name: "uptime_status_code",
		Help: "The status code of the last request.",
	})

	httpClient := &http.Client{
		Timeout: time.Duration(host.Timeout) * time.Second,
		Transport: &headerRoundTripper{
			headers: host.Headers,
			rt:      http.DefaultTransport,
		},
	}

	return &SeekerImpl{
		logger:       logger,
		httpClient:   httpClient,
		host:         host.Host,
		interval:     host.Interval,
		up:           upCounter,
		latency:      latency,
		statusCode:   statusCode,
		previouslyUp: true, // we assume the host is up when we start, to show an error if it's down
	}
}

// CheckUptime starts the uptime checking process. It will run indefinitely until the context is cancelled.
func (s *SeekerImpl) CheckUptime() {
	ctx, cancel := context.WithCancel(context.Background())
	s.hookSignal(cancel)
	defer cancel()

	ticker := time.NewTicker(time.Duration(s.interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		select {
		case <-ctx.Done():
			return
		default:
			s.check()
		}
	}
}

// hookSignal hooks the SIGINT and SIGTERM signals to the context cancel function.
func (s *SeekerImpl) hookSignal(cancel context.CancelFunc) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		s.logger.Infof("Received signal. Stopping seeker for [%s].", s.host)
		cancel()
	}()
}

// check performs the actual check on the remote host. It will set the up and latency metrics accordingly.
func (s *SeekerImpl) check() {
	start := time.Now()

	s.logger.Debugf("Checking [%s]", s.host)
	res, err := s.httpClient.Get(s.host)
	if err != nil {
		s.logger.Debugf("Got error [%v] for [%s]. Counting as down.", err, s.host)
		s.up.Set(0)
		if s.previouslyUp {
			s.logger.Warnf("Host [%s] is down.", s.host)
		}
		s.previouslyUp = false

		return
	}

	// if the status code is not in the 2xx range, we consider the host as down
	s.statusCode.Set(float64(res.StatusCode))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		s.logger.Warnf("Got status code [%d] for [%s]. Counting as down.", res.StatusCode, s.host)
		s.up.Set(0)
		if s.previouslyUp {
			s.logger.Warnf("Host [%s] is down.", s.host)
		}
		s.previouslyUp = false
		return
	}

	s.logger.Debugf("Got status code [%d] for [%s]. Counting as up.", res.StatusCode, s.host)
	s.up.Set(1)
	s.latency.Set(float64(time.Since(start).Milliseconds()))

	if !s.previouslyUp {
		s.logger.Infof("Host [%s] is online.", s.host)
	}
	s.previouslyUp = true

	_ = res.Body.Close()
}

// headerRoundTripper is a custom RoundTripper that adds headers to each request.
type headerRoundTripper struct {
	headers map[string]string
	rt      http.RoundTripper
}

// RoundTrip executes a single HTTP transaction. It will add the headers to the request before sending it.
func (hrt *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range hrt.headers {
		req.Header.Set(key, value)
	}

	return hrt.rt.RoundTrip(req)
}
