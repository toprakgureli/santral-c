// Package tz holds the panel's time zone.
package tz

import "time"

// Istanbul is the zone the panel shows times in and cuts days at: UTC+3
// with no daylight saving. It is fixed rather than loaded from the host's
// tzdata, so a server without that data still counts "today" correctly.
// Stored timestamps stay in UTC.
var Istanbul = time.FixedZone("+03", 3*3600)
