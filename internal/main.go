package internal

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
	"net/url"
)

type Host struct {
	Host     string
	Timeout  int
	Interval int
	Headers  map[string]string
}

// readConfiguration reads the configuration from a file.
func readConfiguration(logger *log.Entry) error {
	logger.Info("Reading configuration")

	// read the configuration from a file
	viper.SetConfigName("config.toml")
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.uptimer")
	viper.AddConfigPath("/app")

	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	logger.Debug("Configuration read successfully.")
	return nil
}

// Application is the main entry point for the application.
func Application(ctx *cli.Context) error {
	logger := log.WithFields(log.Fields{
		"package": "application",
	})

	level, err := log.ParseLevel(ctx.String("log-level"))
	if err != nil {
		logger.WithError(err).Error("Failed to parse log level.")
		return err
	}
	log.SetLevel(level)

	logger.Info("Starting Uptimer")

	// read the configuration but don't fail if it's not present
	if err := readConfiguration(logger); err != nil {
		logger.WithError(err).Warn("Failed to read configuration.")
	}

	envHosts := parseHostsFromEnvVar(logger, ctx)
	configHosts := parseHostsFromCongFile(logger, ctx)
	hosts := mergeHosts(envHosts, configHosts)

	if len(hosts) == 0 && len(configHosts) == 0 {
		logger.Warn("No hosts to check. Exiting.")
		return nil
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	for _, host := range hosts {
		seeker, err := NewSeeker(
			host,
			prometheus.WrapRegistererWith(
				prometheus.Labels{"host": host.Host},
				registry,
			),
		)
		if err != nil {
			logger.WithError(err).Error("Failed to create seeker.")
			return nil
		}

		go seeker.CheckUptime()

		logger.Infof("Started checking [%s]", host.Host)
	}

	// start the metrics server
	Serve(ctx.Int("port"), registry)

	return nil
}

// parseHostsFromEnvVar parses the hosts string and returns a slice of valid hosts.
func parseHostsFromEnvVar(logger *log.Entry, ctx *cli.Context) []Host {
	var output []Host

	entries := ctx.StringSlice("hosts")
	if len(entries) == 0 || entries[0] == "" {
		logger.Warn("No hosts found in environment variable.")
		return output
	}

	// check each entry is an url
	for _, entry := range entries {
		u, err := url.ParseRequestURI(entry)
		if err != nil {
			logger.WithError(err).Errorf("Failed to parse host entry [%s] from environment variable", entry)
			continue
		}

		output = append(output, Host{
			Host:     u.String(),
			Timeout:  ctx.Int("timeout"),
			Interval: ctx.Int("interval"),
			Headers: map[string]string{
				"User-Agent": ctx.App.Name + "/" + ctx.App.Version,
			},
		})
	}

	log.Infof("Parsed [%d] hosts from the environment variables", len(output))

	return output
}

// parseHostsFromCongFile parses the hosts from the configuration file.
// A host in the configuration file may not have all its fields
// filled, in which case the environment variable will be used.
func parseHostsFromCongFile(logger *log.Entry, ctx *cli.Context) []Host {
	var output []Host

	hosts := viper.GetStringMapStringSlice("hosts")
	for key := range hosts {
		prefix := "hosts." + key
		hostname := viper.GetString(prefix + ".host")

		// set default values
		viper.SetDefault(prefix+".timeout", ctx.Int("timeout"))
		viper.SetDefault(prefix+".interval", ctx.Int("interval"))
		viper.SetDefault(prefix+".headers", map[string]string{})

		logger.Debugf("Found potential host [%s] in configuration file", hostname)

		u, err := url.ParseRequestURI(hostname)
		if err != nil {
			logger.WithError(err).Errorf("Failed to parse host entry [%s] from configuration file", hostname)
			continue
		}

		// set the user agent if not present
		headers := viper.GetStringMapString(prefix + ".headers")
		if _, ok := headers["User-Agent"]; !ok {
			headers["User-Agent"] = ctx.App.Name + "/" + ctx.App.Version
		}

		output = append(output, Host{
			Host:     u.String(),
			Timeout:  viper.GetInt(prefix + ".timeout"),
			Interval: viper.GetInt(prefix + ".interval"),
			Headers:  headers,
		})
	}

	log.Infof("Parsed [%d] hosts from the configuration file", len(output))

	return output
}

// mergeHosts merges the hosts from the environment variables and the configuration file.
// It will keep the configuration file hosts in priority.
func mergeHosts(envHosts, configHosts []Host) []Host {
	hosts := make(map[string]Host)
	for _, host := range envHosts {
		hosts[host.Host] = host
	}
	for _, host := range configHosts {
		hosts[host.Host] = host
	}

	var output []Host
	for _, host := range hosts {
		output = append(output, host)
	}

	return output
}
