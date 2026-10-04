package model

import "time"

type SupportTicket struct {
	ID              string    `json:"id"`
	OrderID         string    `json:"order_id"`
	CustomerName    string    `json:"customer_name"`
	CustomerContact string    `json:"customer_contact"` // WhatsApp or Email
	Category        string    `json:"category"`
	Subject         string    `json:"subject"`
	Description     string    `json:"description"`
	Status          string    `json:"status"` // open, in_progress, resolved, closed
	AdminReply      string    `json:"admin_reply"`
	AdminID         int64     `json:"admin_id"`
	AdminName       string    `json:"admin_name,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
