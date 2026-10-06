// Package callrule holds the rule that decides which answered calls count
// as real conversations: those lasting at least a configurable number of
// seconds. The call history, the team page and the profile all read it, so
// they always agree.
package callrule

import "context"

// The bounds of the setting, in seconds. The default applies while nothing
// is stored.
const (
	DefaultRealSeconds = 30
	MinRealSeconds     = 5
	MaxRealSeconds     = 600
)

// Source reads the current threshold.
type Source interface {
	RealCallSeconds(ctx context.Context) int
}

// Seconds returns the threshold from src, or the default without one.
func Seconds(ctx context.Context, src Source) int {
	if src == nil {
		return DefaultRealSeconds
	}
	return src.RealCallSeconds(ctx)
}
