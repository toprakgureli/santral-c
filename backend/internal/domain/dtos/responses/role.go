package responses

import "github.com/toprakgureli/santral-c/backend/internal/domain/models"

// Role is the public view of a role.
type Role struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

// NewRoles maps role models to their response view.
func NewRoles(roles []models.Role) []Role {
	out := make([]Role, 0, len(roles))
	for i := range roles {
		out = append(out, Role{
			ID:          roles[i].ID,
			Name:        roles[i].Name,
			DisplayName: roles[i].DisplayName,
			Description: roles[i].Description,
		})
	}
	return out
}
