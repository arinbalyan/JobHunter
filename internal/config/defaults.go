package config

// Constants for run mode values.
const (
	ModeOnce   = "once"
	ModeDaemon = "daemon"
	ModeCron   = "cron"
)

// Default config values.
const (
	DefaultLogLevel          = "info"
	DefaultEmailDelaySeconds = 30
	DefaultMaxEmailsPerRun   = 10
	DefaultTrackingPort      = 8080
	DefaultMemoryCapMB       = 100
	DefaultMaxTokensPerReq   = 2048
)
