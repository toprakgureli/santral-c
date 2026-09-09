package models

import "time"

// CallEvent is one entry in a call's ordered timeline. Media faults and
// hangups carry their specifics in Detail.
type CallEvent struct {
	ID        uint      `gorm:"column:id;primarykey"`
	CallID    uint      `gorm:"column:call_id;not null;index:idx_call_events_call_id_at,priority:1;index:idx_call_events_call_id_seq,priority:1"`
	Seq       int       `gorm:"column:seq;not null;default:0;index:idx_call_events_call_id_seq,priority:2"`
	Type      string    `gorm:"column:type;size:40;not null;index"`
	Channel   string    `gorm:"column:channel;size:80"`
	At        time.Time `gorm:"column:at;index:idx_call_events_call_id_at,priority:2"`
	Detail    string    `gorm:"column:detail;type:jsonb;not null;default:'{}'"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (CallEvent) TableName() string { return "call_events" }
