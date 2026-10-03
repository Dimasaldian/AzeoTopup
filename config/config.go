package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                   string
	BaseURL                string
	DigiflazzUsername      string
	DigiflazzAPIKey        string
	DigiflazzWebhookSecret string
	AutoGoPayAPIKey        string
	AutoGoPayBaseURL       string
	DBPath                 string
	PriceMarginPercent     float64
	AdminSessionSecret     string
	AdminSessionMaxAge     int
	AdminInitialUsername   string
	AdminInitialPassword   string
	SMTPHost               string
	SMTPPort               string
	SMTPUser               string
	SMTPPass               string
	SMTPFrom               string
	SMTPFromName           string
}

func LoadConfig() *Config {
	loadDotEnv(".env")

	cfg := &Config{
		Port:                   getEnv("PORT", "8080"),
		BaseURL:                getEnv("BASE_URL", "http://localhost:8080"),
		DigiflazzUsername:      getEnv("DIGIFLAZZ_USERNAME", ""),
		DigiflazzAPIKey:        getEnv("DIGIFLAZZ_API_KEY", ""),
		DigiflazzWebhookSecret: getEnv("DIGIFLAZZ_WEBHOOK_SECRET", "topupku-secret"),
		AutoGoPayAPIKey:        getEnv("AUTOGOPAY_API_KEY", ""),
		AutoGoPayBaseURL:       getEnv("AUTOGOPAY_BASE_URL", "https://v1-gateway.autogopay.site"),
		DBPath:                 getEnv("DB_PATH", "./data/topupku.db"),
		PriceMarginPercent:     getEnvFloat("PRICE_MARGIN_PERCENT", 10.0),
		AdminSessionSecret:     getEnv("ADMIN_SESSION_SECRET", "super-secret-admin-key-change-in-prod-12345678"),
		AdminSessionMaxAge:     getEnvInt("ADMIN_SESSION_MAX_AGE", 86400),
		AdminInitialUsername:   getEnv("ADMIN_INITIAL_USERNAME", "admin"),
		AdminInitialPassword:   getEnv("ADMIN_INITIAL_PASSWORD", "admin123"),
		SMTPHost:               getEnv("SMTP_HOST", ""),
		SMTPPort:               getEnv("SMTP_PORT", "587"),
		SMTPUser:               getEnv("SMTP_USER", ""),
		SMTPPass:               getEnv("SMTP_PASS", ""),
		SMTPFrom:               getEnv("SMTP_FROM", ""),
		SMTPFromName:           getEnv("SMTP_FROM_NAME", "TopupKu Store"),
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func loadDotEnv(filepath string) {
	file, err := os.Open(filepath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimPrefix(line, "\xef\xbb\xbf")
		line = strings.TrimPrefix(line, "\ufeff")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			os.Setenv(k, v)
		}
	}
}
