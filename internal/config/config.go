package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"lsf-exporter/internal/collector"
	"lsf-exporter/internal/logger"
)

type Config struct {
	ListenAddress          string
	LogLevel               logger.Level
	DisableNativeCollector bool
	Collector              collector.ServiceConfig
	LSF                    collector.LSFConfig
	External               collector.ExternalConfig
}

func Load() (Config, error) {
	interval, err := durationEnv("LSF_EXPORTER_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	minInterval, err := durationEnv("LSF_EXPORTER_MIN_INTERVAL", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	timeout, err := durationEnv("LSF_EXPORTER_COLLECT_TIMEOUT", 20*time.Second)
	if err != nil {
		return Config{}, err
	}
	externalTimeout, err := durationEnv("LSF_EXPORTER_EXTERNAL_RESOURCE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	if interval < minInterval {
		return Config{}, fmt.Errorf("LSF_EXPORTER_INTERVAL (%s) must be >= LSF_EXPORTER_MIN_INTERVAL (%s)", interval, minInterval)
	}

	level, err := parseLogLevel(env("LSF_EXPORTER_LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	return Config{
		ListenAddress:          env("LSF_EXPORTER_LISTEN_ADDRESS", ":9818"),
		LogLevel:               level,
		DisableNativeCollector: boolEnv("LSF_EXPORTER_DISABLE_NATIVE_COLLECTOR", false),
		Collector: collector.ServiceConfig{
			Interval:    interval,
			MinInterval: minInterval,
			Timeout:     timeout,
		},
		LSF: collector.LSFConfig{
			AppName:      env("LSF_EXPORTER_APP_NAME", "lsf-exporter"),
			QueryUser:    env("LSF_EXPORTER_QUERY_USER", "all"),
			QueryQueue:   os.Getenv("LSF_EXPORTER_QUERY_QUEUE"),
			QueryHost:    os.Getenv("LSF_EXPORTER_QUERY_HOST"),
			QueryJobName: os.Getenv("LSF_EXPORTER_QUERY_JOB_NAME"),
			QueryJobID:   int64Env("LSF_EXPORTER_QUERY_JOB_ID", 0),
			QueryAllJobs: boolEnv("LSF_EXPORTER_ALL_JOBS", false),
		},
		External: collector.ExternalConfig{
			Command: env("LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND", ""),
			Timeout: externalTimeout,
		},
	}, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	return v, nil
}

func int64Env(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

func boolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

func parseLogLevel(raw string) (logger.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return logger.LevelDebug, nil
	case "info":
		return logger.LevelInfo, nil
	case "warn", "warning":
		return logger.LevelWarn, nil
	case "error":
		return logger.LevelError, nil
	default:
		return logger.LevelInfo, fmt.Errorf("unsupported log level %q", raw)
	}
}
