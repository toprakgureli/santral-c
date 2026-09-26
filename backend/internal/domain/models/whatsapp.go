package models

import "time"

// WAChannel is one WhatsApp number connected to the panel (a "device").
// Secrets are stored encrypted; Settings is the device's own configuration
// as JSON, so a new device starts from nothing.
type WAChannel struct {
	ID             uint   `gorm:"column:id;primarykey"`
	Name           string `gorm:"column:name"`
	DisplayPhone   string `gorm:"column:display_phone"`
	PhoneNumberID  string `gorm:"column:phone_number_id"`
	WABAID         string `gorm:"column:waba_id"`
	AppID          string `gorm:"column:app_id"`
	GraphVersion   string `gorm:"column:graph_version"`
	AccessTokenEnc string `gorm:"column:access_token_enc"`
	AppSecretEnc   string `gorm:"column:app_secret_enc"`
	VerifyToken    string `gorm:"column:verify_token"`
	HookKey        string `gorm:"column:hook_key"`
	// a webhook already registered in Meta, used instead of HookKey's address
	ExistingHookURL     string     `gorm:"column:existing_hook_url"`
	ExistingHookPath    string     `gorm:"column:existing_hook_path"`
	ExistingVerifyToken string     `gorm:"column:existing_verify_token"`
	AcceptUnsigned      bool       `gorm:"column:accept_unsigned"`
	Active              bool       `gorm:"column:active"`
	Settings            string     `gorm:"column:settings;type:jsonb"`
	VerifiedName        string     `gorm:"column:verified_name"`
	QualityRating       string     `gorm:"column:quality_rating"`
	MessagingLimit      string     `gorm:"column:messaging_limit"`
	LastWebhookAt       *time.Time `gorm:"column:last_webhook_at"`
	LastError           string     `gorm:"column:last_error"`
	LastErrorAt         *time.Time `gorm:"column:last_error_at"`
	CreatedBy           *uint      `gorm:"column:created_by"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WAChannel) TableName() string { return "wa_channels" }

// WATeam is a group of agents a ticket can be routed to.
type WATeam struct {
	ID        uint      `gorm:"column:id;primarykey"`
	Name      string    `gorm:"column:name"`
	Color     string    `gorm:"column:color"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (WATeam) TableName() string { return "wa_teams" }

// WAWebhookEvent is one raw webhook call, kept until it is processed.
type WAWebhookEvent struct {
	ID          uint       `gorm:"column:id;primarykey"`
	ChannelID   *uint      `gorm:"column:channel_id"`
	Payload     string     `gorm:"column:payload;type:jsonb"`
	Status      string     `gorm:"column:status"`
	Attempts    int        `gorm:"column:attempts"`
	LastError   string     `gorm:"column:last_error"`
	ReceivedAt  time.Time  `gorm:"column:received_at"`
	ProcessedAt *time.Time `gorm:"column:processed_at"`
	NextTryAt   time.Time  `gorm:"column:next_try_at"`
}

// TableName pins the table name.
func (WAWebhookEvent) TableName() string { return "wa_webhook_events" }

// WAContact is a customer, known by the WhatsApp number.
type WAContact struct {
	ID          uint      `gorm:"column:id;primarykey"`
	WAID        string    `gorm:"column:wa_id"`
	PeerKey     string    `gorm:"column:peer_key"`
	ProfileName string    `gorm:"column:profile_name"`
	Name        string    `gorm:"column:name"`
	Tags        string    `gorm:"column:tags;type:jsonb"`
	Note        string    `gorm:"column:note"`
	OptedOut    bool      `gorm:"column:opted_out"`
	Blocked     bool      `gorm:"column:blocked"`
	Source      *string   `gorm:"column:source;type:jsonb"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WAContact) TableName() string { return "wa_contacts" }

// WAConversation is the thread between a device and a customer.
type WAConversation struct {
	ID            uint       `gorm:"column:id;primarykey"`
	ChannelID     uint       `gorm:"column:channel_id"`
	ContactID     uint       `gorm:"column:contact_id"`
	TicketID      *uint      `gorm:"column:ticket_id"`
	LastInboundAt *time.Time `gorm:"column:last_inbound_at"`
	LastMessageID *uint      `gorm:"column:last_message_id"`
	LastMessageAt *time.Time `gorm:"column:last_message_at"`
	TeamReadID    uint       `gorm:"column:team_read_id"`
	MetaReadID    uint       `gorm:"column:meta_read_id"`
	Unread        int        `gorm:"column:unread"`
	Version       int64      `gorm:"column:version;->"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
}

// TableName pins the table name.
func (WAConversation) TableName() string { return "wa_conversations" }

// WATicket is one support case inside a conversation.
type WATicket struct {
	ID              uint       `gorm:"column:id;primarykey"`
	Number          int64      `gorm:"column:number;->"`
	ConversationID  uint       `gorm:"column:conversation_id"`
	ChannelID       uint       `gorm:"column:channel_id"`
	ContactID       uint       `gorm:"column:contact_id"`
	Status          string     `gorm:"column:status"`
	OwnerID         *uint      `gorm:"column:owner_id"`
	TeamID          *uint      `gorm:"column:team_id"`
	Priority        string     `gorm:"column:priority"`
	Category        string     `gorm:"column:category"`
	Tags            string     `gorm:"column:tags;type:jsonb"`
	AwaitingSince   *time.Time `gorm:"column:awaiting_since"`
	WaitingListedAt *time.Time `gorm:"column:waiting_listed_at"`
	WaitingCount    int        `gorm:"column:waiting_count"`
	LongestWaitSec  int        `gorm:"column:longest_wait_sec"`
	ReopenCount     int        `gorm:"column:reopen_count"`
	FirstResponseAt *time.Time `gorm:"column:first_response_at"`
	ResolvedAt      *time.Time `gorm:"column:resolved_at"`
	ResolvedBy      *uint      `gorm:"column:resolved_by"`
	SurveySentAt    *time.Time `gorm:"column:survey_sent_at"`
	Rating          *int       `gorm:"column:rating"`
	RatingComment   string     `gorm:"column:rating_comment"`
	RatedAt         *time.Time `gorm:"column:rated_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WATicket) TableName() string { return "wa_tickets" }

// WAParticipant is someone who worked on a ticket.
type WAParticipant struct {
	TicketID     uint       `gorm:"column:ticket_id;primarykey"`
	UserID       uint       `gorm:"column:user_id;primarykey"`
	Role         string     `gorm:"column:role"`
	Greeted      bool       `gorm:"column:greeted"`
	FirstReplyAt *time.Time `gorm:"column:first_reply_at"`
	JoinedAt     time.Time  `gorm:"column:joined_at"`
}

// TableName pins the table name.
func (WAParticipant) TableName() string { return "wa_ticket_participants" }

// WAAssignment is one assignment change of a ticket.
type WAAssignment struct {
	ID        uint      `gorm:"column:id;primarykey"`
	TicketID  uint      `gorm:"column:ticket_id"`
	Kind      string    `gorm:"column:kind"`
	FromUser  *uint     `gorm:"column:from_user"`
	ToUser    *uint     `gorm:"column:to_user"`
	TeamID    *uint     `gorm:"column:team_id"`
	ByUser    *uint     `gorm:"column:by_user"`
	Note      string    `gorm:"column:note"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (WAAssignment) TableName() string { return "wa_assignments" }

// WAMessage is one message, note or event in a conversation. Outgoing rows
// are also the send queue.
type WAMessage struct {
	ID             uint       `gorm:"column:id;primarykey"`
	ChannelID      uint       `gorm:"column:channel_id"`
	ConversationID uint       `gorm:"column:conversation_id"`
	TicketID       *uint      `gorm:"column:ticket_id"`
	Direction      string     `gorm:"column:direction"`
	Kind           string     `gorm:"column:kind"`
	WAMID          *string    `gorm:"column:wamid"`
	ClientID       *string    `gorm:"column:client_id"`
	SenderKind     string     `gorm:"column:sender_kind"`
	SenderUserID   *uint      `gorm:"column:sender_user_id"`
	SenderLabel    string     `gorm:"column:sender_label"`
	Body           string     `gorm:"column:body"`
	Media          *string    `gorm:"column:media;type:jsonb"`
	Payload        *string    `gorm:"column:payload;type:jsonb"`
	ReplyToWAMID   *string    `gorm:"column:reply_to_wamid"`
	Status         string     `gorm:"column:status"`
	ErrorCode      *int       `gorm:"column:error_code"`
	ErrorText      string     `gorm:"column:error_text"`
	Attempts       int        `gorm:"column:attempts"`
	NextTryAt      *time.Time `gorm:"column:next_try_at"`
	SentAt         *time.Time `gorm:"column:sent_at"`
	DeliveredAt    *time.Time `gorm:"column:delivered_at"`
	ReadAt         *time.Time `gorm:"column:read_at"`
	FailedAt       *time.Time `gorm:"column:failed_at"`
	WATimestamp    *time.Time `gorm:"column:wa_timestamp"`
	Pricing        *string    `gorm:"column:pricing;type:jsonb"`
	Referral       *string    `gorm:"column:referral;type:jsonb"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

// TableName pins the table name.
func (WAMessage) TableName() string { return "wa_messages" }

// WATemplate is a message template of a business account.
type WATemplate struct {
	ID             uint      `gorm:"column:id;primarykey"`
	WABAID         string    `gorm:"column:waba_id"`
	MetaID         string    `gorm:"column:meta_id"`
	Name           string    `gorm:"column:name"`
	Language       string    `gorm:"column:language"`
	Category       string    `gorm:"column:category"`
	Status         string    `gorm:"column:status"`
	Components     string    `gorm:"column:components;type:jsonb"`
	RejectedReason string    `gorm:"column:rejected_reason"`
	Quality        string    `gorm:"column:quality"`
	Fill           string    `gorm:"column:fill;type:jsonb;->"`
	CreatedBy      *uint     `gorm:"column:created_by"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WATemplate) TableName() string { return "wa_templates" }

// WAQuickReply is a ready answer.
type WAQuickReply struct {
	ID         uint      `gorm:"column:id;primarykey"`
	Shortcut   string    `gorm:"column:shortcut"`
	Title      string    `gorm:"column:title"`
	Body       string    `gorm:"column:body"`
	ChannelIDs string    `gorm:"column:channel_ids;type:jsonb"`
	CreatedBy  *uint     `gorm:"column:created_by"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WAQuickReply) TableName() string { return "wa_quick_replies" }

// WAAutomation is a one-step rule.
type WAAutomation struct {
	ID          uint      `gorm:"column:id;primarykey"`
	Name        string    `gorm:"column:name"`
	Active      bool      `gorm:"column:active"`
	ChannelIDs  string    `gorm:"column:channel_ids;type:jsonb"`
	Trigger     string    `gorm:"column:trigger"`
	Conditions  string    `gorm:"column:conditions;type:jsonb"`
	Actions     string    `gorm:"column:actions;type:jsonb"`
	CooldownMin int       `gorm:"column:cooldown_min"`
	Position    int       `gorm:"column:position"`
	CreatedBy   *uint     `gorm:"column:created_by"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WAAutomation) TableName() string { return "wa_automations" }

// WABot is a chatbot flow.
type WABot struct {
	ID               uint       `gorm:"column:id;primarykey"`
	Name             string     `gorm:"column:name"`
	Description      string     `gorm:"column:description"`
	Active           bool       `gorm:"column:active"`
	ChannelIDs       string     `gorm:"column:channel_ids;type:jsonb"`
	Trigger          string     `gorm:"column:trigger"`
	Keywords         string     `gorm:"column:keywords;type:jsonb"`
	Schedule         string     `gorm:"column:schedule;type:jsonb"`
	Draft            string     `gorm:"column:draft;type:jsonb"`
	PublishedVersion int        `gorm:"column:published_version"`
	PublishedAt      *time.Time `gorm:"column:published_at"`
	CreatedBy        *uint      `gorm:"column:created_by"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WABot) TableName() string { return "wa_bots" }

// WABotSession is where a customer is inside a flow.
type WABotSession struct {
	ConversationID uint      `gorm:"column:conversation_id;primarykey"`
	BotID          uint      `gorm:"column:bot_id"`
	Version        int       `gorm:"column:version"`
	NodeID         string    `gorm:"column:node_id"`
	Vars           string    `gorm:"column:vars;type:jsonb"`
	Tries          int       `gorm:"column:tries"`
	StartedAt      time.Time `gorm:"column:started_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WABotSession) TableName() string { return "wa_bot_sessions" }

// WAIntegration is an outside system a flow may ask.
type WAIntegration struct {
	ID         uint      `gorm:"column:id;primarykey"`
	Name       string    `gorm:"column:name"`
	Method     string    `gorm:"column:method"`
	URL        string    `gorm:"column:url"`
	HeadersEnc string    `gorm:"column:headers_enc"`
	Body       string    `gorm:"column:body"`
	TimeoutSec int       `gorm:"column:timeout_sec"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WAIntegration) TableName() string { return "wa_integrations" }

// WACallback is a "call me back" request.
type WACallback struct {
	ID        uint       `gorm:"column:id;primarykey"`
	ChannelID *uint      `gorm:"column:channel_id"`
	ContactID *uint      `gorm:"column:contact_id"`
	TicketID  *uint      `gorm:"column:ticket_id"`
	Phone     string     `gorm:"column:phone"`
	Note      string     `gorm:"column:note"`
	Status    string     `gorm:"column:status"`
	DoneBy    *uint      `gorm:"column:done_by"`
	DoneAt    *time.Time `gorm:"column:done_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

// TableName pins the table name.
func (WACallback) TableName() string { return "wa_callbacks" }

// WAFile is a file uploaded from the panel for chatbots and templates.
type WAFile struct {
	ID        uint      `gorm:"column:id;primarykey"`
	StorageID string    `gorm:"column:storage_id"`
	Name      string    `gorm:"column:name"`
	Mime      string    `gorm:"column:mime"`
	Size      int64     `gorm:"column:size"`
	CreatedBy *uint     `gorm:"column:created_by"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (WAFile) TableName() string { return "wa_files" }

// WAGlobalSetting is one module-wide setting stored as JSON.
type WAGlobalSetting struct {
	Key       string    `gorm:"column:key;primarykey"`
	Value     string    `gorm:"column:value;type:jsonb"`
	UpdatedBy *uint     `gorm:"column:updated_by"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (WAGlobalSetting) TableName() string { return "wa_global_settings" }

// WACallSurvey is a survey sent over WhatsApp after a phone call.
type WACallSurvey struct {
	ID             uint       `gorm:"column:id;primarykey"`
	CallID         string     `gorm:"column:call_id"`
	UserID         *uint      `gorm:"column:user_id"`
	PeerKey        string     `gorm:"column:peer_key"`
	WAID           string     `gorm:"column:wa_id"`
	ChannelID      *uint      `gorm:"column:channel_id"`
	ConversationID *uint      `gorm:"column:conversation_id"`
	MessageID      *uint      `gorm:"column:message_id"`
	Direction      string     `gorm:"column:direction"`
	TalkSeconds    int        `gorm:"column:talk_seconds"`
	Status         string     `gorm:"column:status"`
	Note           string     `gorm:"column:note"`
	Score          *int       `gorm:"column:score"`
	Comment        string     `gorm:"column:comment"`
	SendAt         time.Time  `gorm:"column:send_at"`
	SentAt         *time.Time `gorm:"column:sent_at"`
	AnsweredAt     *time.Time `gorm:"column:answered_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

// TableName pins the table name.
func (WACallSurvey) TableName() string { return "wa_call_surveys" }
