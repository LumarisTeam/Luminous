package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type ServerConfig struct {
	Port           int
	Mode           string
	CORSOrigin     string
	TLSCert        string
	TLSKey         string
	TrustedProxies string
}

type AuthConfig struct {
	AdminToken string
	MCPToken   string
}

type DatabaseConfig struct {
	DSN          string
	PoolMaxConns int32
	PoolMinConns int32
}

type ReleaseConfig struct {
	APIURL    string
	AppUUID   string
	ChannelID string
}

type RateLimitConfig struct {
	Rate  int
	Burst int
}

type AppConfig struct {
	Server    ServerConfig
	Auth      AuthConfig
	Database  DatabaseConfig
	Release   ReleaseConfig
	RateLimit RateLimitConfig
}

var bom = []byte{0xEF, 0xBB, 0xBF}

func loadEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open .env file: %w", err)
	}

	result := make(map[string]string)
	data = bytes.TrimPrefix(data, bom)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		result[key] = value
	}
	return result, scanner.Err()
}

func getEnv(fileVals map[string]string, key, fallback string) string {
	if v, ok := fileVals[key]; ok && v != "" {
		return v
	}
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(fileVals map[string]string, key string, fallback int) int {
	if v, ok := fileVals[key]; ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		fmt.Fprintf(os.Stderr, "WARNING: invalid integer for %s=%q, using default %d\n", key, v, fallback)
	}
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		fmt.Fprintf(os.Stderr, "WARNING: invalid integer for %s=%q, using default %d\n", key, v, fallback)
	}
	return fallback
}

func LoadConfig() (*AppConfig, error) {
	fileVals, err := loadEnvFile(".env")
	if err != nil {
		return nil, fmt.Errorf("load .env file: %w", err)
	}
	if fileVals == nil {
		fileVals = make(map[string]string)
	}

	cfg := &AppConfig{
		Server: ServerConfig{
			Port:           getEnvInt(fileVals, "LUMINOUS_SERVER_PORT", 8080),
			Mode:           getEnv(fileVals, "LUMINOUS_SERVER_MODE", "release"),
			CORSOrigin:     getEnv(fileVals, "LUMINOUS_SERVER_CORS_ORIGIN", ""),
			TLSCert:        getEnv(fileVals, "LUMINOUS_SERVER_TLS_CERT", ""),
			TLSKey:         getEnv(fileVals, "LUMINOUS_SERVER_TLS_KEY", ""),
			TrustedProxies: getEnv(fileVals, "LUMINOUS_SERVER_TRUSTED_PROXIES", ""),
		},
		Auth: AuthConfig{
			AdminToken: getEnv(fileVals, "LUMINOUS_AUTH_ADMIN_TOKEN", ""),
			MCPToken:   getEnv(fileVals, "LUMINOUS_MCP_TOKEN", ""),
		},
		Database: DatabaseConfig{
			DSN:          getEnv(fileVals, "LUMINOUS_DATABASE_DSN", ""),
			PoolMaxConns: int32(getEnvInt(fileVals, "LUMINOUS_DATABASE_POOL_MAX_CONNS", 20)),
			PoolMinConns: int32(getEnvInt(fileVals, "LUMINOUS_DATABASE_POOL_MIN_CONNS", 5)),
		},
		Release: ReleaseConfig{
			APIURL:    getEnv(fileVals, "LUMINOUS_RELEASE_API_URL", ""),
			AppUUID:   getEnv(fileVals, "LUMINOUS_RELEASE_APP_UUID", "5f278ffc-5a70-4805-a6bf-0543040981a8"),
			ChannelID: getEnv(fileVals, "LUMINOUS_RELEASE_CHANNEL_ID", "9e1a198a-a0c2-4017-b492-f2d0e5bee437"),
		},
		RateLimit: RateLimitConfig{
			Rate:  getEnvInt(fileVals, "LUMINOUS_RATE_LIMIT_RATE", 10),
			Burst: getEnvInt(fileVals, "LUMINOUS_RATE_LIMIT_BURST", 30),
		},
	}

	switch cfg.Server.Mode {
	case "debug", "release", "test":
	default:
		return nil, fmt.Errorf("invalid server mode: %q (must be debug, release, or test)", cfg.Server.Mode)
	}

	if cfg.Auth.AdminToken == "" {
		return nil, errors.New("LUMINOUS_AUTH_ADMIN_TOKEN is required")
	}

	return cfg, nil
}
