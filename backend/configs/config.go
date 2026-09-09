// Package configs holds typed configuration and its loader.
package configs

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Development is the runtime mode.
type Development string

// Runtime modes.
const (
	Test Development = "test"
	Live Development = "live"
)

// SSLMode is the PostgreSQL sslmode.
type SSLMode string

// PostgreSQL sslmode values.
const (
	Disable    SSLMode = "disable"
	Require    SSLMode = "require"
	VerifyCA   SSLMode = "verify-ca"
	VerifyFull SSLMode = "verify-full"
)

// App holds general application settings.
type App struct {
	Name           string      `mapstructure:"name"`
	Development    Development `mapstructure:"development"`
	Port           string      `mapstructure:"port"`
	TrustedProxies string      `mapstructure:"trustedProxies"`
	CORSOrigins    string      `mapstructure:"corsOrigins"`
	PublicURL      string      `mapstructure:"publicUrl"`
}

// Auth holds token and cookie settings.
type Auth struct {
	Secret            string        `mapstructure:"secret"`
	AccessTTL         time.Duration `mapstructure:"accessTTL"`
	RefreshTTL        time.Duration `mapstructure:"refreshTTL"`
	Issuer            string        `mapstructure:"issuer"`
	CookieName        string        `mapstructure:"cookieName"`
	RefreshCookieName string        `mapstructure:"refreshCookieName"`
	CookieSecure      bool          `mapstructure:"cookieSecure"`
}

// Security holds lockout and MFA-encryption settings.
type Security struct {
	IPFailureLimit      int           `mapstructure:"ipFailureLimit"`
	IPBanDuration       time.Duration `mapstructure:"ipBanDuration"`
	AccountLockDuration time.Duration `mapstructure:"accountLockDuration"`
	DistinctIPLimit     int           `mapstructure:"distinctIPLimit"`
	AttemptWindow       time.Duration `mapstructure:"attemptWindow"`
	MFAKey              string        `mapstructure:"mfaKey"`
	TrustedIPs          string        `mapstructure:"trustedIPs"`
}

// Owner holds the bootstrap invisible-admin credentials.
type Owner struct {
	Name     string `mapstructure:"name"`
	Email    string `mapstructure:"email"`
	Password string `mapstructure:"password"`
}

// Database holds PostgreSQL connection settings.
type Database struct {
	Host     string  `mapstructure:"host"`
	Port     string  `mapstructure:"port"`
	Name     string  `mapstructure:"name"`
	User     string  `mapstructure:"user"`
	Password string  `mapstructure:"password"`
	SSLMode  SSLMode `mapstructure:"sslMode"`
	TimeZone string  `mapstructure:"timeZone"`
	Debug    bool    `mapstructure:"debug"`
}

// Redis holds Redis connection settings.
type Redis struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// Asterisk holds the media-engine integration settings.
type Asterisk struct {
	Enabled         bool   `mapstructure:"enabled"`
	AMIAddress      string `mapstructure:"amiAddress"`
	AMIUsername     string `mapstructure:"amiUsername"`
	AMISecret       string `mapstructure:"amiSecret"`
	DialContext     string `mapstructure:"dialContext"`
	UsersConfigPath string `mapstructure:"usersConfigPath"`
	WebSocketURL    string `mapstructure:"webSocketUrl"`
	SIPKey          string `mapstructure:"sipKey"`
}

// Config is the aggregate configuration.
type Config struct {
	App      App      `mapstructure:"app"`
	Auth     Auth     `mapstructure:"auth"`
	Security Security `mapstructure:"security"`
	Owner    Owner    `mapstructure:"owner"`
	Database Database `mapstructure:"database"`
	Redis    Redis    `mapstructure:"redis"`
	Asterisk Asterisk `mapstructure:"asterisk"`
}

// Cnf is the loaded configuration.
var Cnf Config

// Load reads configuration from a file and the environment into Cnf.
func Load(path string) error {
	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := v.ReadInConfig(); err != nil {
		return err
	}
	return v.Unmarshal(&Cnf)
}
