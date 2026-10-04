package store

import (
	"fmt"
	"math/rand"
	"time"

	"topupku/model"
)

func (s *SQLiteStore) migrateTickets() error {
	schema := `
	CREATE TABLE IF NOT EXISTS support_tickets (
		id TEXT PRIMARY KEY,
		order_id TEXT DEFAULT '',
		customer_name TEXT NOT NULL,
		customer_contact TEXT NOT NULL,
		category TEXT NOT NULL,
		subject TEXT NOT NULL,
		description TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'open',
		admin_reply TEXT DEFAULT '',
		admin_id INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_tickets_status ON support_tickets(status);
	CREATE INDEX IF NOT EXISTS idx_tickets_order ON support_tickets(order_id);
	CREATE INDEX IF NOT EXISTS idx_tickets_created ON support_tickets(created_at);
	`
	_, err := s.db.Exec(schema)
	return err
}

func GenerateTicketID() string {
	now := time.Now().Format("20060102")
	chars := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 4)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("CS-%s-%s", now, string(b))
}

func (s *SQLiteStore) CreateTicket(t *model.SupportTicket) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t.ID == "" {
		t.ID = GenerateTicketID()
	}
	if t.Status == "" {
		t.Status = "open"
	}

	_, err := s.db.Exec(`
		INSERT INTO support_tickets (id, order_id, customer_name, customer_contact, category, subject, description, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, t.ID, t.OrderID, t.CustomerName, t.CustomerContact, t.Category, t.Subject, t.Description, t.Status)
	return err
}

func (s *SQLiteStore) GetTicketByID(id string) (*model.SupportTicket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT t.id, t.order_id, t.customer_name, t.customer_contact, t.category, t.subject, t.description,
		       t.status, t.admin_reply, t.admin_id, COALESCE(u.display_name, '') as admin_name,
		       t.created_at, t.updated_at
		FROM support_tickets t
		LEFT JOIN admin_users u ON t.admin_id = u.id
		WHERE t.id = ?
	`
	var t model.SupportTicket
	err := s.db.QueryRow(query, id).Scan(
		&t.ID, &t.OrderID, &t.CustomerName, &t.CustomerContact, &t.Category, &t.Subject, &t.Description,
		&t.Status, &t.AdminReply, &t.AdminID, &t.AdminName, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *SQLiteStore) GetTickets(statusFilter string, limit int) ([]*model.SupportTicket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT t.id, t.order_id, t.customer_name, t.customer_contact, t.category, t.subject, t.description,
		       t.status, t.admin_reply, t.admin_id, COALESCE(u.display_name, '') as admin_name,
		       t.created_at, t.updated_at
		FROM support_tickets t
		LEFT JOIN admin_users u ON t.admin_id = u.id
	`
	var args []interface{}
	if statusFilter != "" && statusFilter != "all" {
		query += " WHERE t.status = ?"
		args = append(args, statusFilter)
	}
	query += " ORDER BY CASE t.status WHEN 'open' THEN 1 WHEN 'in_progress' THEN 2 ELSE 3 END, t.created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.SupportTicket
	for rows.Next() {
		var t model.SupportTicket
		if err := rows.Scan(
			&t.ID, &t.OrderID, &t.CustomerName, &t.CustomerContact, &t.Category, &t.Subject, &t.Description,
			&t.Status, &t.AdminReply, &t.AdminID, &t.AdminName, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tickets = append(tickets, &t)
	}
	return tickets, nil
}

func (s *SQLiteStore) UpdateTicket(id string, status, adminReply string, adminID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE support_tickets
		SET status = ?, admin_reply = ?, admin_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, adminReply, adminID, id)
	return err
}

func (s *SQLiteStore) GetTicketStats() (openCount int, inProgressCount int, resolvedCount int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_ = s.db.QueryRow("SELECT COUNT(*) FROM support_tickets WHERE status = 'open'").Scan(&openCount)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM support_tickets WHERE status = 'in_progress'").Scan(&inProgressCount)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM support_tickets WHERE status IN ('resolved', 'closed')").Scan(&resolvedCount)
	return
}
