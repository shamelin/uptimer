package internal

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v2"
	"testing"
)

var logger = log.WithFields(log.Fields{
	"package": "test",
})
var flagSet *flag.FlagSet

func setupMainTest() {
	viper.Reset()

	flagSet = flag.NewFlagSet("test", flag.ContinueOnError)
	flagSet.Var(&cli.StringSlice{}, "hosts", "")
	flagSet.Int("timeout", 5, "")
	flagSet.Int("interval", 5, "")
}

func TestParseHostsFromEnvWithEmptyList(t *testing.T) {
	setupMainTest()
	err := flagSet.Set("hosts", "")
	assert.NoError(t, err)

	ctx := cli.NewContext(&cli.App{}, flagSet, nil)

	hosts := parseHostsFromEnvVar(logger, ctx)
	assert.Empty(t, hosts)
}

func TestParseHostsFromEnvWithSingleHost(t *testing.T) {
	setupMainTest()
	err := flagSet.Set("hosts", "http://example.com")
	assert.NoError(t, err)

	ctx := cli.NewContext(&cli.App{}, flagSet, nil)

	hosts := parseHostsFromEnvVar(logger, ctx)
	assert.Len(t, hosts, 1)
	assert.Equal(t, "http://example.com", hosts[0].Host)
}

func TestParseHostsFromEnvWithMultipleHosts(t *testing.T) {
	setupMainTest()
	err := flagSet.Set("hosts", "http://example.com,http://example.org")
	assert.NoError(t, err)

	ctx := cli.NewContext(&cli.App{}, flagSet, nil)

	hosts := parseHostsFromEnvVar(logger, ctx)
	assert.Len(t, hosts, 2)
	assert.Equal(t, "http://example.com", hosts[0].Host)
	assert.Equal(t, "http://example.org", hosts[1].Host)
}

func TestParseHostsFromConfigWithEmptyList(t *testing.T) {
	setupMainTest()
	viper.Set("hosts", []string{})

	ctx := cli.NewContext(&cli.App{}, flagSet, nil)

	hosts := parseHostsFromCongFile(logger, ctx)
	assert.Empty(t, hosts)
}

func TestParseHostsFromConfigWithSingleHostWithParameters(t *testing.T) {
	setupMainTest()
	viper.Set("hosts.host1.host", "http://example.com")
	viper.Set("hosts.host1.timeout", 10)
	viper.Set("hosts.host1.interval", 10)

	ctx := cli.NewContext(&cli.App{
		Name:    "Uptimer",
		Version: "1.0.0",
	}, flagSet, nil)

	hosts := parseHostsFromCongFile(logger, ctx)
	assert.Len(t, hosts, 1)
	assert.Equal(t, hosts[0], Host{
		Host:     "http://example.com",
		Timeout:  10,
		Interval: 10,
		Headers: map[string]string{
			"User-Agent": "Uptimer/1.0.0",
		},
	})
}

func TestParseHostsFromConfigWithSingleHostWithoutParameters(t *testing.T) {
	setupMainTest()
	viper.Set("hosts.host1.host", "http://example.com")

	ctx := cli.NewContext(&cli.App{}, flagSet, nil)

	hosts := parseHostsFromCongFile(logger, ctx)
	assert.Len(t, hosts, 1)
	assert.Equal(t, hosts[0], Host{
		Host:     "http://example.com",
		Timeout:  5,
		Interval: 5,
		Headers: map[string]string{
			"User-Agent": "/",
		},
	})
}

func TestParseHostsFromConfigWithMultipleHosts(t *testing.T) {
	setupMainTest()
	viper.Set("hosts.host1.host", "http://example.com")
	viper.Set("hosts.host1.timeout", 10)
	viper.Set("hosts.host1.interval", 10)
	viper.Set("hosts.host2.host", "http://example.org")

	ctx := cli.NewContext(&cli.App{}, flagSet, nil)

	hosts := parseHostsFromCongFile(logger, ctx)
	assert.Len(t, hosts, 2)
	assert.Equal(t, hosts[0], Host{
		Host:     "http://example.com",
		Timeout:  10,
		Interval: 10,
		Headers: map[string]string{
			"User-Agent": "/",
		},
	})
	assert.Equal(t, hosts[1], Host{
		Host:     "http://example.org",
		Timeout:  5,
		Interval: 5,
		Headers: map[string]string{
			"User-Agent": "/",
		},
	})
}

func TestParseHostsFromConfigWithInvalidHost(t *testing.T) {
	setupMainTest()
	viper.Set("hosts.host1.host", "example.com")

	ctx := cli.NewContext(&cli.App{}, flagSet, nil)

	hosts := parseHostsFromCongFile(logger, ctx)
	assert.Empty(t, hosts)
}

func TestParseHostsFromConfigOverridesUserAgent(t *testing.T) {
	setupMainTest()
	viper.Set("hosts.host1.host", "http://example.com")
	viper.Set("hosts.host1.headers", map[string]string{
		"User-Agent": "Custom User Agent",
	})

	ctx := cli.NewContext(&cli.App{}, flagSet, nil)

	hosts := parseHostsFromCongFile(logger, ctx)
	assert.Len(t, hosts, 1)
	assert.Equal(t, hosts[0], Host{
		Host:     "http://example.com",
		Timeout:  5,
		Interval: 5,
		Headers: map[string]string{
			"User-Agent": "Custom User Agent",
		},
	})
}
