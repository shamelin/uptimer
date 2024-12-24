package main

import (
	"github.com/urfave/cli/v2"
	"os"
	"uptime-seeker/internal"

	log "github.com/sirupsen/logrus"
)

var logger = log.WithFields(log.Fields{
	"package": "main",
})

func main() {
	app := &cli.App{
		Name:    "Uptime Seeker",
		Usage:   "A flexible Prometheus-compatible uptime checker for your services.",
		Version: "1.0.0",
		Authors: []*cli.Author{
			{
				Name:  "Simon Hamelin",
				Email: "simon@hamelin.pro",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Aliases: []string{"l"},
				EnvVars: []string{"LOG_LEVEL"},
				Usage:   "Set the log level (debug, info, warn, error, fatal, panic).",
				Value:   "info",
			},
			&cli.StringSliceFlag{
				Name:    "hosts",
				Aliases: []string{"H"},
				EnvVars: []string{"HOSTS"},
				Usage:   "Command-separated list of hosts to check.",
			},
			&cli.IntFlag{
				Name:    "interval",
				Aliases: []string{"i"},
				EnvVars: []string{"INTERVAL"},
				Usage:   "Interval in seconds between each check.",
				Value:   5,
			},
			&cli.IntFlag{
				Name:    "timeout",
				Aliases: []string{"t"},
				EnvVars: []string{"TIMEOUT"},
				Usage:   "Timeout in seconds for each check.",
				Value:   5,
			},
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				EnvVars: []string{"PORT"},
				Usage:   "Port on which the metrics endpoint will be exposed.",
				Value:   8080,
			},
		},
		Action: internal.Application,
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error("Failed to run the application.")
		return
	}
}
