package seeking

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/signal"
	"syscall"
	"time"
	"uptimer/internal/seeking/dto"
	"uptimer/internal/seeking/hooks"
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
	hooks        map[string]hooks.HookHandler
	httpClient   *http.Client
	host         dto.Host
	up           prometheus.Gauge
	latency      prometheus.Gauge
	statusCode   prometheus.Gauge
	previouslyUp bool
}

// NewSeeker creates a new SeekerImpl instance.
func NewSeeker(host dto.Host, hooks map[string]hooks.HookHandler, registerer prometheus.Registerer) (*SeekerImpl, error) {
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

	// create a cookie jar to store cookies
	jar, err := cookiejar.New(nil)
	if err != nil {
		logger.Fatalf("Failed to create cookie jar: %v", err)
		return nil, err
	}

	httpClient := &http.Client{
		Timeout: time.Duration(host.Timeout) * time.Second,
		Transport: &headerRoundTripper{
			headers: host.Headers,
			rt:      http.DefaultTransport,
		},
		Jar: jar,
	}

	return &SeekerImpl{
		logger:       logger,
		hooks:        hooks,
		httpClient:   httpClient,
		host:         host,
		up:           upCounter,
		latency:      latency,
		statusCode:   statusCode,
		previouslyUp: true, // we assume the host is up when we start, to show an error if it's down
	}, nil
}

// CheckUptime starts the uptime checking process. It will run indefinitely until the context is cancelled.
func (s *SeekerImpl) CheckUptime() {
	ctx, cancel := context.WithCancel(context.Background())
	s.hookSignal(cancel)
	defer cancel()

	ticker := time.NewTicker(time.Duration(s.host.Interval) * time.Second)
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
		s.logger.Infof("Received signal. Stopping seeker for [%s].", s.host.Host)
		cancel()
	}()
}

// check performs the actual check on the remote host. It will set the up and latency metrics accordingly.
func (s *SeekerImpl) check() {
	start := time.Now()

	host := s.host.Host
	s.logger.Debugf("Checking [%s]", host)
	res, err := s.httpClient.Get(host)
	seekResult := hooks.SeekResult{
		Host:         s.host,
		ResponseTime: time.Since(start),
	}
	if err != nil {
		seekResult.Online = false

		s.logger.Debugf("Got error [%v] for [%s]. Counting as down.", err, host)
		s.up.Set(0)
		if s.previouslyUp {
			s.logger.Warnf("Host [%s] is down.", host)
		}
		s.previouslyUp = false
	} else {
		seekResult.Online = true
		seekResult.StatusCode = res.StatusCode
	}

	// call hooks
	for _, hook := range s.host.Hooks {
		if handler, ok := s.hooks[hook]; ok {
			if err := handler.Handle(seekResult); err != nil {
				s.logger.Errorf("Failed to handle hook [%s]: %v", hook, err)
			}
		}
	}

	if !seekResult.Online {
		return
	}

	// if the status code is not in the 2xx range, we consider the host as down
	s.statusCode.Set(float64(seekResult.StatusCode))
	if seekResult.StatusCode < 200 || seekResult.StatusCode > 299 {
		s.logger.Warnf("Got status code [%d] for [%s]. Counting as down.", res.StatusCode, host)
		s.up.Set(0)
		if s.previouslyUp {
			s.logger.Warnf("Host [%s] is down.", host)
		}
		s.previouslyUp = false
		return
	}

	s.logger.Debugf("Got status code [%d] for [%s]. Counting as up.", res.StatusCode, host)
	s.up.Set(1)
	s.latency.Set(float64(seekResult.ResponseTime.Milliseconds()))

	if !s.previouslyUp {
		s.logger.Infof("Host [%s] is online.", host)
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
