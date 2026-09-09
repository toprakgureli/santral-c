package models

import "time"

// CallQuality is one RTCP snapshot for a media leg of a call, with an
// estimated MOS so audio problems are measured, not just perceived.
type CallQuality struct {
	ID              uint      `gorm:"column:id;primarykey"`
	CallID          uint      `gorm:"column:call_id;not null;index:idx_call_quality_call_id_at,priority:1"`
	Leg             string    `gorm:"column:leg;size:10;not null"`
	Channel         string    `gorm:"column:channel;size:80"`
	Codec           string    `gorm:"column:codec;size:20"`
	At              time.Time `gorm:"column:at;index:idx_call_quality_call_id_at,priority:2"`
	JitterMs        *float64  `gorm:"column:jitter_ms;type:numeric(8,2)"`
	RTTMs           *float64  `gorm:"column:rtt_ms;type:numeric(8,2)"`
	LossPct         *float64  `gorm:"column:loss_pct;type:numeric(5,2)"`
	MOS             *float64  `gorm:"column:mos;type:numeric(3,2)"`
	PacketsSent     int64     `gorm:"column:packets_sent;not null;default:0"`
	PacketsReceived int64     `gorm:"column:packets_received;not null;default:0"`
	PacketsLost     int64     `gorm:"column:packets_lost;not null;default:0"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (CallQuality) TableName() string { return "call_quality" }
