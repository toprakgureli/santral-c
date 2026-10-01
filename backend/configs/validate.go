package configs

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Problem is one thing wrong with the configuration.
type Problem struct {
	Key     string
	Message string
	// Fatal problems stop a live server from starting; the others are
	// reported and the server goes on.
	Fatal bool
}

func (p Problem) String() string {
	return p.Key + ": " + p.Message
}

// ApplyDefaults fills settings that were left out with safe values, so a
// missing line never turns a limit into zero.
func ApplyDefaults(c *Config) {
	setDur := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	setInt := func(n *int, v int) {
		if *n <= 0 {
			*n = v
		}
	}
	setDur(&c.Auth.AccessTTL, 15*time.Minute)
	setDur(&c.Auth.RefreshTTL, 7*24*time.Hour)
	if c.Auth.Issuer == "" {
		c.Auth.Issuer = "santral-c"
	}
	if c.Auth.CookieName == "" {
		c.Auth.CookieName = "santral_access"
	}
	if c.Auth.RefreshCookieName == "" {
		c.Auth.RefreshCookieName = "santral_refresh"
	}
	setInt(&c.Security.IPFailureLimit, 100)
	setDur(&c.Security.IPBanDuration, 15*time.Minute)
	setDur(&c.Security.AccountLockDuration, 15*time.Minute)
	setInt(&c.Security.DistinctIPLimit, 5)
	setDur(&c.Security.AttemptWindow, 15*time.Minute)
	setInt(&c.Security.DeviceFailureLimit, 5)
	if c.App.Port == "" {
		c.App.Port = "8090"
	}
}

// Check lists what is wrong with the configuration. A live server refuses
// to start on any fatal problem; a test setup only reports them.
func Check(c Config) []Problem {
	var out []Problem
	add := func(key, msg string, fatal bool) {
		out = append(out, Problem{Key: key, Message: msg, Fatal: fatal})
	}
	example := func(v string) bool {
		return strings.Contains(strings.ToLower(v), "change-me")
	}
	secret := func(key, v string, min int) {
		switch {
		case strings.TrimSpace(v) == "":
			add(key, "boş bırakılmış", true)
		case example(v):
			add(key, "hâlâ örnek değer; openssl rand -hex 32 ile üret", true)
		case len(v) < min:
			add(key, fmt.Sprintf("en az %d karakter olmalı", min), true)
		}
	}

	secret("auth.secret", c.Auth.Secret, 32)
	secret("security.dataKey", c.Security.DataKey, 32)
	// These keys already sealed stored data; a short one cannot be
	// replaced without losing it, so only an empty or example value stops.
	secret("security.mfaKey", c.Security.MFAKey, 1)
	if c.Bulutsantralim.Enabled {
		secret("bulutsantralim.sipKey", c.Bulutsantralim.SIPKey, 1)
		if strings.TrimSpace(c.Bulutsantralim.APIKey) == "" || example(c.Bulutsantralim.APIKey) {
			add("bulutsantralim.apiKey", "santral açık ama API anahtarı girilmemiş", true)
		}
	}
	if c.Auth.AccessTTL > 24*time.Hour {
		add("auth.accessTTL", "24 saatten uzun olamaz", true)
	}
	if c.Auth.RefreshTTL <= c.Auth.AccessTTL {
		add("auth.refreshTTL", "accessTTL'den uzun olmalı", true)
	}
	if c.Database.Host == "" || c.Database.Name == "" || c.Database.User == "" {
		add("database", "host, name ve user girilmeli", true)
	}
	if c.Redis.Host == "" || c.Redis.Port == "" {
		add("redis", "host ve port girilmeli", true)
	}
	if c.App.Development == Live {
		if !c.Auth.CookieSecure {
			add("auth.cookieSecure", "canlıda true olmalı (site HTTPS)", true)
		}
		if c.App.Host != "127.0.0.1" && c.App.Host != "localhost" && c.App.Host != "::1" {
			add("app.host", "canlıda 127.0.0.1 olmalı; backend'e sadece nginx ulaşsın", false)
		}
		if strings.TrimSpace(c.App.TrustedProxies) == "" {
			add("app.trustedProxies", "boş; nginx arkasında herkes 127.0.0.1 görünür ve gerçek IP kaybolur", false)
		}
		if example(c.Owner.Password) {
			add("owner.password", "örnek değer; ilk girişte değiştirilecek ama tahmin edilebilir", false)
		}
	}
	if c.Security.IPFailureLimit < 50 && strings.TrimSpace(c.Security.TrustedIPs) == "" {
		add("security.ipFailureLimit", "50'nin altında ve trustedIPs boş; aynı adresten çıkan bütün ofis birkaç hatalı girişte kilitlenebilir", false)
	}
	if c.Auth.RefreshTTL > 7*24*time.Hour {
		add("auth.refreshTTL", "oturumlar en fazla 7 gün sürer; daha uzun değer 7 gün sayılır", false)
	}
	for _, cidr := range splitList(c.Security.TrustedIPs) {
		if _, err := ParseNet(cidr); err != nil {
			add("security.trustedIPs", err.Error(), true)
		}
	}
	return out
}

// Validate returns an error naming every fatal problem, or nil.
func Validate(c Config) error {
	var msgs []string
	for _, p := range Check(c) {
		if p.Fatal {
			msgs = append(msgs, p.String())
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return errors.New("config.yml hatalı:\n  " + strings.Join(msgs, "\n  "))
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
