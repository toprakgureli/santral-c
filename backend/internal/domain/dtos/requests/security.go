package requests

// SecurityFilter narrows the login-attempt listing.
type SecurityFilter struct {
	Email   string
	IP      string
	Success *bool
	Page    int
	PerPage int
}
