package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"topupku/model"
	"topupku/store"
)

type UserService struct {
	store *store.SQLiteStore
}

func NewUserService(st *store.SQLiteStore) *UserService {
	return &UserService{store: st}
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("azeotopup-dummy-password"), bcrypt.DefaultCost)

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func newSessionToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Register creates a new customer account and returns a fresh session token.
func (s *UserService) Register(email, name, password, confirm string, sessionAge int) (*model.User, string, error) {
	email = normalizeEmail(email)
	name = strings.TrimSpace(name)

	if _, err := mail.ParseAddress(email); err != nil || !strings.Contains(email, ".") {
		return nil, "", fmt.Errorf("format email tidak valid")
	}
	if name == "" {
		return nil, "", fmt.Errorf("nama wajib diisi")
	}
	if len(name) > 60 {
		return nil, "", fmt.Errorf("nama maksimal 60 karakter")
	}
	if len(password) < 6 {
		return nil, "", fmt.Errorf("password minimal 6 karakter")
	}
	if len(password) > 72 {
		return nil, "", fmt.Errorf("password maksimal 72 karakter")
	}
	if password != confirm {
		return nil, "", fmt.Errorf("konfirmasi password tidak sama")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("gagal memproses password")
	}

	id, err := s.store.CreateUser(email, name, string(hash))
	if err != nil {
		return nil, "", err
	}

	token := newSessionToken()
	if err := s.store.CreateUserSession(token, id, sessionAge); err != nil {
		return nil, "", fmt.Errorf("gagal membuat sesi login")
	}
	_ = s.store.UpdateUserLogin(id)

	user, err := s.store.GetUserByID(id)
	return user, token, err
}

// Login verifies credentials and returns a fresh session token.
func (s *UserService) Login(email, password string, sessionAge int) (*model.User, string, error) {
	email = normalizeEmail(email)
	user, err := s.store.GetUserByEmail(email)
	if err != nil {
		// Still run bcrypt to keep response time similar (prevents email enumeration via timing)
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, "", fmt.Errorf("email atau password salah")
	}
	if !user.IsActive {
		return nil, "", fmt.Errorf("akun dinonaktifkan, hubungi admin")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", fmt.Errorf("email atau password salah")
	}

	token := newSessionToken()
	if err := s.store.CreateUserSession(token, user.ID, sessionAge); err != nil {
		return nil, "", fmt.Errorf("gagal membuat sesi login")
	}
	_ = s.store.UpdateUserLogin(user.ID)
	return user, token, nil
}

func (s *UserService) Logout(token string) {
	if token != "" {
		_ = s.store.DeleteUserSession(token)
	}
}

// NormalizeVoucherCode uppercases and strips spaces so users can paste codes loosely.
func NormalizeVoucherCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, " ", "")
	return code
}

func (s *UserService) RedeemVoucher(userID int64, code string) (int64, int64, error) {
	code = NormalizeVoucherCode(code)
	if code == "" {
		return 0, 0, fmt.Errorf("kode voucher wajib diisi")
	}
	return s.store.RedeemVoucher(userID, code)
}

// ================= ADMIN: VOUCHER GENERATION =================

const voucherAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I to avoid confusion

func randomVoucherBlock(n int) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(voucherAlphabet))))
		b[i] = voucherAlphabet[idx.Int64()]
	}
	return string(b)
}

// GenerateVoucherCode returns a code like AZC-7KQ2-M9XD-P4TR (60 bits of entropy).
func GenerateVoucherCode() string {
	return fmt.Sprintf("AZC-%s-%s-%s", randomVoucherBlock(4), randomVoucherBlock(4), randomVoucherBlock(4))
}

func (s *UserService) GenerateVouchers(amount int64, qty int, note string) ([]string, error) {
	if amount < 1000 {
		return nil, fmt.Errorf("nominal voucher minimal 1.000 AZcoin")
	}
	if amount > 10_000_000 {
		return nil, fmt.Errorf("nominal voucher maksimal 10.000.000 AZcoin")
	}
	if qty < 1 || qty > 100 {
		return nil, fmt.Errorf("jumlah voucher harus 1 - 100 per generate")
	}
	codes := make([]string, 0, qty)
	seen := map[string]bool{}
	for len(codes) < qty {
		c := GenerateVoucherCode()
		if !seen[c] {
			seen[c] = true
			codes = append(codes, c)
		}
	}
	if err := s.store.CreateVouchers(codes, amount, strings.TrimSpace(note)); err != nil {
		return nil, err
	}
	return codes, nil
}
