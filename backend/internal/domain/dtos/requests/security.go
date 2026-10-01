package requests

// SecurityFilter narrows the login-attempt listing.
type SecurityFilter struct {
	Email   string
	IP      string
	Success *bool
	// ExcludeInvisibleAdmin leaves out the owner account's attempts.
	ExcludeInvisibleAdmin bool
	Page                  int
	PerPage               int
}
