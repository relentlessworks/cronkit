package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"net/smtp"
	"os"
	"time"

	"github.com/relentlessworks/cronkit/internal/model"
	"github.com/relentlessworks/cronkit/internal/store"
)

// Auth handles OTP-based authentication.
type Auth struct {
	store     *store.Store
	secret    string
	smtpHost  string
	smtpPort  string
	smtpUser  string
	smtpPass  string
	fromEmail string
}

// New creates a new Auth instance.
func New(s *store.Store, secret, smtpHost, smtpPort, smtpUser, smtpPass, fromEmail string) *Auth {
	return &Auth{
		store:     s,
		secret:    secret,
		smtpHost:  smtpHost,
		smtpPort:  smtpPort,
		smtpUser:  smtpUser,
		smtpPass:  smtpPass,
		fromEmail: fromEmail,
	}
}

// RequestOTP generates and sends (or logs) an OTP for the given email.
func (a *Auth) RequestOTP(email string) error {
	code := generateOTPCode()

	otp := &model.OTP{
		Email:     email,
		Code:      code,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	if err := a.store.SaveOTP(otp); err != nil {
		return fmt.Errorf("failed to save OTP: %w", err)
	}

	if a.smtpHost != "" {
		return a.sendEmail(email, code)
	}

	// Dev mode: log to stderr
	log.Printf("[DEV] OTP for %s: %s\n", email, code)
	return nil
}

// VerifyOTP validates an OTP and returns a bearer token.
func (a *Auth) VerifyOTP(email, code string) (string, error) {
	otp, ok := a.store.GetOTP(email)
	if !ok {
		return "", fmt.Errorf("no OTP requested for this email | hint: call POST /auth/request with email first")
	}

	if time.Now().After(otp.ExpiresAt) {
		a.store.DeleteOTP(email)
		return "", fmt.Errorf("OTP expired | hint: request a new OTP via POST /auth/request")
	}

	if otp.Code != code {
		return "", fmt.Errorf("invalid OTP code | hint: check the code and try again, or request a new one via POST /auth/request")
	}

	a.store.DeleteOTP(email)

	token := a.generateToken(email)
	return token, nil
}

// ValidateToken checks if a token is valid and returns the associated email.
func (a *Auth) ValidateToken(tokenStr string) (string, error) {
	token, ok := a.store.GetToken(tokenStr)
	if !ok {
		return "", fmt.Errorf("invalid or expired token")
	}
	return token.Email, nil
}

// Store returns the underlying store.
func (a *Auth) Store() *store.Store {
	return a.store
}

// GetWorkspaceFromToken returns the workspace associated with a token.
func (a *Auth) GetWorkspaceFromToken(tokenStr string) (string, error) {
	token, ok := a.store.GetToken(tokenStr)
	if !ok {
		return "", fmt.Errorf("invalid or expired token")
	}
	return token.Workspace, nil
}

// SaveToken stores a token with workspace association.
func (a *Auth) SaveToken(token, workspace, email string) error {
	t := &model.Token{
		Token:     token,
		Workspace: workspace,
		Email:     email,
		CreatedAt: time.Now(),
	}
	return a.store.SaveToken(t)
}

func (a *Auth) generateToken(email string) string {
	b := make([]byte, 32)
	rand.Read(b)
	h := sha256.Sum256(append(b, []byte(a.secret+email)...))
	return hex.EncodeToString(h[:])
}

func generateOTPCode() string {
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "000000"
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func (a *Auth) sendEmail(email, code string) error {
	subject := "Your cronkit OTP Code"
	body := fmt.Sprintf("Your verification code is: %s\nIt expires in 10 minutes.", code)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		a.fromEmail, email, subject, body)

	addr := a.smtpHost + ":" + a.smtpPort
	var auth smtp.Auth
	if a.smtpUser != "" {
		auth = smtp.PlainAuth("", a.smtpUser, a.smtpPass, a.smtpHost)
	}

	return smtp.SendMail(addr, auth, a.fromEmail, []string{email}, []byte(msg))
}

// GenerateRandomSecret generates a random secret for token signing.
func GenerateRandomSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GetEnvOrDefault returns env var or default.
func GetEnvOrDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
