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
	Name        string      `mapstructure:"name"`
	Development Development `mapstructure:"development"`
	// Host is the address the server listens on; empty means every
	// interface. Behind nginx on the same machine it is 127.0.0.1.
	Host           string `mapstructure:"host"`
	Port           string `mapstructure:"port"`
	TrustedProxies string `mapstructure:"trustedProxies"`
	CORSOrigins    string `mapstructure:"corsOrigins"`
	PublicURL      string `mapstructure:"publicUrl"`
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
	RequireMFA          bool          `mapstructure:"requireMFA"`
	// DataKey encrypts integration secrets kept in the database (WhatsApp
	// tokens, the Drive link). It is separate from the session signing key.
	DataKey string `mapstructure:"dataKey"`
	// PreviousDataKeys (comma separated) still open values made before the
	// data key was replaced; they are sealed again with DataKey at start.
	PreviousDataKeys string `mapstructure:"previousDataKeys"`
	// DeviceFailureLimit is how many wrong passwords one browser may enter
	// before it is held back for a while. Office staff share one address,
	// so wrong tries are counted per browser and per account, not per IP.
	DeviceFailureLimit int `mapstructure:"deviceFailureLimit"`
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
	// MaxConns is the most connections the server opens to PostgreSQL at
	// once. Requests beyond it wait a moment for a free one instead of
	// failing. Keep it well under PostgreSQL's max_connections (100 by
	// default), which backups, deploys and admin sessions share.
	MaxConns int `mapstructure:"maxConns"`
}

// Redis holds Redis connection settings.
type Redis struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// Bulutsantralim holds the Verimor hosted-PBX integration settings.
type Bulutsantralim struct {
	Enabled      bool   `mapstructure:"enabled"`
	APIKey       string `mapstructure:"apiKey"`
	APIBase      string `mapstructure:"apiBase"`
	WebphoneBase string `mapstructure:"webphoneBase"`

	// WebRTC softphone (SIP over WSS) registration settings.
	SIPDomain string `mapstructure:"sipDomain"` // registrar/realm: the "Santral Adı" in Verimor's SIP device settings
	SIPWssURL string `mapstructure:"sipWssUrl"` // wss:// signaling endpoint
	StunURL   string `mapstructure:"stunUrl"`   // optional STUN server
	TurnURL   string `mapstructure:"turnUrl"`   // optional TURN server
	TurnUser  string `mapstructure:"turnUser"`
	TurnPass  string `mapstructure:"turnPass"`
	SIPKey    string `mapstructure:"sipKey"` // encrypts stored SIP passwords at rest

	// HistoryDays is how far back the call-record mirror backfills on first
	// run (default 90). Newer records arrive with the regular poll.
	HistoryDays int `mapstructure:"historyDays"`
}

// Drive holds the Google Drive OAuth client used for chat attachments.
// The account itself is connected from the Yönetim screen; only the
// OAuth client lives in the file.
type Drive struct {
	ClientID     string `mapstructure:"clientId"`
	ClientSecret string `mapstructure:"clientSecret"`
	RedirectURL  string `mapstructure:"redirectUrl"` // https://<host>/api/v1/teams/drive/callback
	FolderName   string `mapstructure:"folderName"`  // default "SantralC"
}

// Telemetry says where traces go; metrics are always on /metrics.
type Telemetry struct {
	// OTLPEndpoint is the trace collector, for example Jaeger at
	// http://127.0.0.1:4318. Empty turns tracing off.
	OTLPEndpoint string  `mapstructure:"otlpEndpoint"`
	SampleRatio  float64 `mapstructure:"sampleRatio"`
}

// Config is the aggregate configuration.
type Config struct {
	App            App            `mapstructure:"app"`
	Auth           Auth           `mapstructure:"auth"`
	Security       Security       `mapstructure:"security"`
	Owner          Owner          `mapstructure:"owner"`
	Database       Database       `mapstructure:"database"`
	Redis          Redis          `mapstructure:"redis"`
	Bulutsantralim Bulutsantralim `mapstructure:"bulutsantralim"`
	Drive          Drive          `mapstructure:"drive"`
	Telemetry      Telemetry      `mapstructure:"telemetry"`
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
	if err := v.Unmarshal(&Cnf); err != nil {
		return err
	}
	ApplyDefaults(&Cnf)
	return nil
}
