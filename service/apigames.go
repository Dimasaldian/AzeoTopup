package service

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ApiGamesClient struct {
	MerchantID string
	SecretKey  string
	BaseURL    string
	client     *http.Client
}

func NewApiGamesClient(merchantID, secretKey string) *ApiGamesClient {
	return &ApiGamesClient{
		MerchantID: strings.TrimSpace(merchantID),
		SecretKey:  strings.TrimSpace(secretKey),
		BaseURL:    "https://v2.apigames.id",
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *ApiGamesClient) IsConfigured() bool {
	return a.MerchantID != "" && a.SecretKey != ""
}

func (a *ApiGamesClient) Sign() string {
	h := md5.Sum([]byte(a.MerchantID + a.SecretKey))
	return hex.EncodeToString(h[:])
}

// Map internal game codes to ApiGames game codes
func mapGameCode(internalCode string) string {
	code := strings.ToLower(strings.TrimSpace(internalCode))
	switch code {
	case "ml", "mlbb", "mobile-legends":
		return "mobilelegend"
	case "ff", "freefire", "free-fire":
		return "freefire"
	case "genshin", "genshin-impact":
		return "genshin"
	case "pubg", "pubgm", "pubg-mobile":
		return "pubgm"
	case "codm":
		return "codm"
	case "valorant":
		return "valorant"
	case "aov":
		return "aov"
	case "pointblank", "pb":
		return "pointblank"
	default:
		return code
	}
}

type CheckUsernameResult struct {
	Success  bool   `json:"success"`
	Username string `json:"username,omitempty"`
	Message  string `json:"message,omitempty"`
}

// CheckUsername inquiries player username from ApiGames
func (a *ApiGamesClient) CheckUsername(gameCode, userID, zoneID string) (*CheckUsernameResult, error) {
	apiGame := mapGameCode(gameCode)
	userID = strings.TrimSpace(userID)
	zoneID = strings.TrimSpace(zoneID)

	if userID == "" {
		return &CheckUsernameResult{
			Success: false,
			Message: "User ID tidak boleh kosong",
		}, nil
	}

	// Simulation mode if not configured
	if !a.IsConfigured() {
		return &CheckUsernameResult{
			Success:  true,
			Username: fmt.Sprintf("Player_%s", userID),
			Message:  "Akun valid (Mode Simulasi - ApiGames belum diisi di .env)",
		}, nil
	}

	signature := a.Sign()
	params := url.Values{}

	targetUserID := userID
	if apiGame == "mobilelegend" && zoneID != "" {
		// ApiGames requires combined UserID+ZoneID for Mobile Legends
		targetUserID = userID + zoneID
	} else if zoneID != "" {
		params.Set("zone_id", zoneID)
	}

	params.Set("user_id", targetUserID)
	params.Set("signature", signature)

	reqURL := fmt.Sprintf("%s/merchant/%s/cek-username/%s?%s", a.BaseURL, a.MerchantID, apiGame, params.Encode())

	resp, err := a.client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("gagal terhubung ke ApiGames: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca respon ApiGames: %w", err)
	}

	log.Printf("[ApiGames] Raw response for %s (user: %s, zone: %s): %s", apiGame, userID, zoneID, string(bodyBytes))

	var raw map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, fmt.Errorf("format respon ApiGames tidak valid: %s", string(bodyBytes))
	}

	// Parse status
	var isSuccess bool
	switch s := raw["status"].(type) {
	case float64:
		isSuccess = s == 1 || s == 200
	case string:
		isSuccess = s == "1" || s == "200" || strings.ToLower(s) == "success"
	case bool:
		isSuccess = s
	}

	if !isSuccess {
		msg := "User ID / Zone ID tidak ditemukan"
		if m, ok := raw["error_msg"].(string); ok && m != "" {
			msg = m
		} else if m, ok := raw["error"].(string); ok && m != "" {
			msg = m
		} else if m, ok := raw["message"].(string); ok && m != "" {
			msg = m
		}
		return &CheckUsernameResult{
			Success: false,
			Message: msg,
		}, nil
	}

	// Extract username from "data"
	username := ""
	if dataObj, ok := raw["data"].(map[string]interface{}); ok {
		if u, ok := dataObj["username"].(string); ok && u != "" {
			username = u
		} else if u, ok := dataObj["name"].(string); ok && u != "" {
			username = u
		} else if u, ok := dataObj["username_game"].(string); ok && u != "" {
			username = u
		}
	} else if dataStr, ok := raw["data"].(string); ok && dataStr != "" {
		username = dataStr
	}

	if username == "" {
		if msg, ok := raw["message"].(string); ok && msg != "" {
			username = msg
		} else {
			username = "Player ID Valid"
		}
	}

	return &CheckUsernameResult{
		Success:  true,
		Username: username,
		Message:  "Akun ditemukan",
	}, nil
}
