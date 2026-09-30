package enums

// PhoneLabel classifies a contact phone number.
type PhoneLabel string

// Phone labels.
const (
	PhoneLabelMobile PhoneLabel = "mobile"
	PhoneLabelWork   PhoneLabel = "work"
	PhoneLabelHome   PhoneLabel = "home"
	PhoneLabelOther  PhoneLabel = "other"
)
