package model

import "time"

type Game struct {
	ID              int64     `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	Brand           string    `json:"brand"`
	Description     string    `json:"description"`
	IconPath        string    `json:"icon_path"`
	BannerPath      string    `json:"banner_path"`
	IDLabel         string    `json:"id_label"`
	IDPlaceholder   string    `json:"id_placeholder"`
	IDHelpText      string    `json:"id_help_text"`
	ID2Label        string    `json:"id2_label"`
	ID2Placeholder  string    `json:"id2_placeholder"`
	IDFormatRegex   string    `json:"id_format_regex"`
	InstructionText string    `json:"instruction_text"`
	SortOrder       int       `json:"sort_order"`
	IsActive        bool      `json:"is_active"`
	ProductCount    int       `json:"product_count,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
