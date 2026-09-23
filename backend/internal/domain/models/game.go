package models

import "time"

// GameItem is one piece of game content an administrator keeps: a word to
// draw, a poll question, a scenario, a story opener or a bingo phrase.
type GameItem struct {
	ID        uint      `gorm:"column:id;primarykey"`
	Kind      string    `gorm:"column:kind;size:20;not null"`
	Text      string    `gorm:"column:text;type:text;not null"`
	Answer    string    `gorm:"column:answer;type:text;not null;default:''"`
	Options   *string   `gorm:"column:options;type:jsonb"`
	Seconds   int       `gorm:"column:seconds;not null;default:0"`
	Active    bool      `gorm:"column:active;not null;default:true"`
	CreatedBy *uint     `gorm:"column:created_by"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (GameItem) TableName() string { return "game_items" }

// Game is one match in a room.
type Game struct {
	ID         uint       `gorm:"column:id;primarykey"`
	GroupID    uint       `gorm:"column:group_id;not null"`
	MessageID  *uint      `gorm:"column:message_id"`
	Kind       string     `gorm:"column:kind;size:20;not null"`
	Status     string     `gorm:"column:status;size:10;not null;default:lobby"`
	HostID     uint       `gorm:"column:host_id;not null"`
	Config     string     `gorm:"column:config;type:jsonb;not null;default:'{}'"`
	State      string     `gorm:"column:state;type:jsonb;not null;default:'{}'"`
	Winners    string     `gorm:"column:winners;type:jsonb;not null;default:'[]'"`
	Version    int        `gorm:"column:version;not null;default:0"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	StartedAt  *time.Time `gorm:"column:started_at"`
	FinishedAt *time.Time `gorm:"column:finished_at"`
}

// TableName pins the table name.
func (Game) TableName() string { return "games" }

// GamePlayer is a seat in a match.
type GamePlayer struct {
	GameID   uint      `gorm:"column:game_id;primarykey"`
	UserID   uint      `gorm:"column:user_id;primarykey"`
	Team     int       `gorm:"column:team;not null;default:0"`
	Score    int       `gorm:"column:score;not null;default:0"`
	JoinedAt time.Time `gorm:"column:joined_at"`
}

// TableName pins the table name.
func (GamePlayer) TableName() string { return "game_players" }

// GameResult is one person's outcome in a finished match, for the record.
type GameResult struct {
	ID         uint      `gorm:"column:id;primarykey"`
	GameID     uint      `gorm:"column:game_id;not null"`
	UserID     uint      `gorm:"column:user_id;not null"`
	Kind       string    `gorm:"column:kind;size:20;not null"`
	Won        bool      `gorm:"column:won;not null;default:false"`
	Score      int       `gorm:"column:score;not null;default:0"`
	FinishedAt time.Time `gorm:"column:finished_at"`
}

// TableName pins the table name.
func (GameResult) TableName() string { return "game_results" }
