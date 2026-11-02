package domain

import "time"

// ManagedDatabase is one customer-owned MySQL database on the shared tenant
// cluster. The password never appears here; it is revealed through a separate
// audited operation.
type ManagedDatabase struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	DatabaseName  string    `json:"database_name"`
	Username      string    `json:"username"`
	PHPMyAdminURL string    `json:"phpmyadmin_url"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
