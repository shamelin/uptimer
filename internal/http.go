package internal

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func Serve(port int, registry *prometheus.Registry) {
	logger := log.WithFields(log.Fields{
		"package": "http",
	})

	httpServer := &http.Server{
		Addr: ":" + strconv.Itoa(port),
	}

	http.Handle(
		"/metrics",
		promhttp.HandlerFor(registry, promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		}),
	)

	hookSignal(logger, httpServer)
	err := httpServer.ListenAndServe()
	if err != nil {
		logger.Fatalf("Failed to start the HTTP server: %v", err)
	}
}

func hookSignal(logger *log.Entry, httpServer *http.Server) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		logger.Info("Received signal. Shutting down the HTTP server.")
		err := httpServer.Shutdown(nil)
		if err != nil {
			logger.Errorf("Failed to shutdown the HTTP server: %v", err)
		}

		os.Exit(0)
	}()
}
