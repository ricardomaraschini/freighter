package clair

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v2"
)

//go:embed static/*
var static embed.FS

// EmptyConfig parses the default configuration into a Config struct and returns it. The
// returned config should be adjusted accordingly and the default config is read from file
// static/default-clair-config.yaml.
func EmptyConfig() (*Config, error) {
	dt, err := static.ReadFile("static/default-clair-config.yaml")
	if err != nil {
		return nil, fmt.Errorf("error reading default clair config: %w", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(dt, config); err != nil {
		return nil, fmt.Errorf("error unmarshaling default config: %w", err)
	}
	return config, nil
}

// Config is a struct holding all configuration options for a clair instance. Supports an all
// in one clair deployment.
type Config struct {
	HTTPListenAddr    string         `yaml:"http_listen_addr"`
	IntrospectionAddr string         `yaml:"introspection_addr"`
	LogLevel          string         `yaml:"log_level"`
	Indexer           IndexerConfig  `yaml:"indexer"`
	Matcher           MatcherConfig  `yaml:"matcher"`
	Notifier          NotifierConfig `yaml:"notifier"`
	Auth              AuthConfig     `yaml:"auth"`
	Trace             TraceConfig    `yaml:"trace"`
	Metrics           MetricsConfig  `yaml:"metrics"`
}

// IndexerConfig holds configuration for clair's indexer agent.
type IndexerConfig struct {
	ConnString           string `yaml:"connstring"`
	ScanLockRetry        int    `yaml:"scanlock_retry"`
	LayerScanConcurrency int    `yaml:"layer_scan_concurrency"`
	Migrations           bool   `yaml:"migrations"`
	AirGap               bool   `yaml:"airgap"`
}

// MatcherConfig holds configuration for clair's matcher agent.
type MatcherConfig struct {
	ConnString      string `yaml:"connstring"`
	MaxConnPool     int    `yaml:"max_conn_pool"`
	IndexerAddr     string `yaml:"indexer_addr"`
	Migrations      bool   `yaml:"migrations"`
	DisableUpdaters bool   `yaml:"disable_updaters"`
}

// NotifierConfig holds configuration for clair's notifier agent.
type NotifierConfig struct {
	ConnString       string        `yaml:"connstring"`
	Migrations       bool          `yaml:"migrations"`
	IndexerAddr      string        `yaml:"indexer_addr"`
	MatcherAddr      string        `yaml:"matcher_addr"`
	PollInterval     string        `yaml:"poll_interval"`
	DeliveryInterval string        `yaml:"delivery_interval"`
	Webhook          WebhookConfig `yaml:"webhook"`
}

// WebhookConfig holds configuration for notifier's webhooks.
type WebhookConfig struct {
	Target   string `yaml:"target"`
	Callback string `yaml:"callback"`
	Signed   bool   `yaml:"signed"`
}

// AuthConfig holds clair's authentication configurations.
type AuthConfig struct {
	PSK AuthPSKConfig `yaml:"psk"`
}

// AuthPSKConfig holds PSK configuration used in the authentication config.
type AuthPSKConfig struct {
	Key string   `yaml:"key"`
	ISS []string `yaml:"iss"`
}

// TraceConfig hold clair's configuration for tracing.
type TraceConfig struct {
	Name   string            `yaml:"name"`
	Jaeger TraceJaegerConfig `yaml:"jaeger"`
}

// TraceJaegerConfig holds jaeger configuration.
type TraceJaegerConfig struct {
	Agent       TraceJaegerAgentConfig    `yaml:"agent"`
	Collector   TraceJaegerColletorConfig `yaml:"collector"`
	ServiceName string                    `yaml:"service_name"`
	BufferMax   int                       `yaml:"buffer_max"`
}

// TraceJaegerAgentConfig holds agent configuration for Jaeger.
type TraceJaegerAgentConfig struct {
	Endpoint string `yaml:"endpoint"`
}

// TraceJaegerColletorConfig holds the Jaeger collector configuration.
type TraceJaegerColletorConfig struct {
	Endpoint string `yaml:"endpoint"`
}

// MetricsConfig holds clair's metrics configuration.
type MetricsConfig struct {
	Name      string                 `yaml:"name"`
	DogStatsd MetricsDogStatsdConfig `yaml:"dogstatsd"`
}

// MetricsDogStatsdConfig holds DogStatsd configuation.
type MetricsDogStatsdConfig struct {
	URL string `yaml:"url"`
}
