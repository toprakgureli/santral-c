package models

import "time"

// PBXCDR is one hosted-PBX call record mirrored locally, so the panel can
// search history without the hosted API's broken filters.
type PBXCDR struct {
	CallUUID          string    `gorm:"column:call_uuid;primarykey;size:80"`
	StartAt           time.Time `gorm:"column:start_at"`
	StartStamp        string    `gorm:"column:start_stamp;size:40"`
	Direction         string    `gorm:"column:direction;size:16"`
	RawDirection      string    `gorm:"column:raw_direction;size:32"`
	CallerIDNumber    string    `gorm:"column:caller_id_number;size:80"`
	DestinationNumber string    `gorm:"column:destination_number;size:80"`
	CallerNum         string    `gorm:"column:caller_num;size:32"`
	DestNum           string    `gorm:"column:dest_num;size:32"`
	CallerExt         string    `gorm:"column:caller_ext;size:16"`
	DestExt           string    `gorm:"column:dest_ext;size:16"`
	Duration          string    `gorm:"column:duration;size:16"`
	TalkDuration      string    `gorm:"column:talk_duration;size:16"`
	AnswerStamp       string    `gorm:"column:answer_stamp;size:40"`
	Result            string    `gorm:"column:result;size:64"`
	Missed            bool      `gorm:"column:missed"`
	RecordingPresent  bool      `gorm:"column:recording_present"`
	FetchedAt         time.Time `gorm:"column:fetched_at"`
}

// TableName pins the table name.
func (PBXCDR) TableName() string { return "pbx_cdrs" }
