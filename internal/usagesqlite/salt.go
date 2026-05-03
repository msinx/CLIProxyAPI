package usagesqlite

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

const saltFileName = "usage_api_key_salt"

func ResolveSalt(cfg *config.Config, configFilePath string) (string, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if salt := strings.TrimSpace(cfg.UsageAPIKeySalt); salt != "" {
		return salt, nil
	}
	if secret := strings.TrimSpace(cfg.RemoteManagement.SecretKey); secret != "" {
		return secret, nil
	}

	dir := strings.TrimSpace(cfg.AuthDir)
	if dir == "" {
		base := filepath.Dir(strings.TrimSpace(configFilePath))
		if base == "." || base == "" {
			base = "."
		}
		dir = filepath.Join(base, "auths")
	}
	path := filepath.Join(dir, saltFileName)
	if data, err := os.ReadFile(path); err == nil {
		if salt := strings.TrimSpace(string(data)); salt != "" {
			return salt, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read usage salt file: %w", err)
	}

	salt, err := generateSalt()
	if err != nil {
		return "", err
	}
	if errMkdir := os.MkdirAll(dir, 0o755); errMkdir != nil {
		return "", fmt.Errorf("prepare usage salt directory: %w", errMkdir)
	}
	if errWrite := os.WriteFile(path, []byte(salt+"\n"), 0o600); errWrite != nil {
		return "", fmt.Errorf("write usage salt file: %w", errWrite)
	}
	return salt, nil
}

func generateSalt() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate usage salt: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
