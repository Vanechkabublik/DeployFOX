package config

import (
	"bufio"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL   string
	RedisURL      string
	EncryptionKey []byte
	SetupToken    string
	Production    bool
	Provider      string
	AgentURL      string
	AgentToken    string
	Origin        string
	WebhookSecret string
}

func Load() (Config, error) {
	if file, err := os.Open("/run/deployfox/env"); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			key, value, ok := strings.Cut(scanner.Text(), "=")
			if ok && os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
		file.Close()
		if err := scanner.Err(); err != nil {
			return Config{}, err
		}
	}
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"), RedisURL: os.Getenv("REDIS_URL"),
		SetupToken: os.Getenv("SETUP_TOKEN"), Production: os.Getenv("APP_ENV") == "production",
		Provider: os.Getenv("DEPLOYMENT_PROVIDER"), AgentURL: os.Getenv("AGENT_URL"),
		AgentToken: os.Getenv("AGENT_TOKEN"), Origin: os.Getenv("PUBLIC_URL"),
		WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
	}
	c.Origin = strings.TrimRight(c.Origin, "/")
	origin, originErr := url.Parse(c.Origin)
	if originErr != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" || (origin.Scheme != "http" && origin.Scheme != "https") {
		return c, errors.New("PUBLIC_URL must be an HTTP(S) origin without path or credentials")
	}
	if c.Production && origin.Scheme != "https" {
		return c, errors.New("production PUBLIC_URL requires HTTPS")
	}
	var err error
	c.EncryptionKey, err = hex.DecodeString(os.Getenv("ENCRYPTION_KEY"))
	if err != nil || len(c.EncryptionKey) != 32 {
		return c, errors.New("ENCRYPTION_KEY must contain 64 hex characters")
	}
	if len(c.SetupToken) < 32 || c.DatabaseURL == "" || c.RedisURL == "" || c.Origin == "" {
		return c, errors.New("database, redis, public URL and setup token (32+ chars) required")
	}
	if c.Provider != "mock" && c.Provider != "docker" {
		return c, errors.New("DEPLOYMENT_PROVIDER must be mock or docker")
	}
	if c.Production && c.Provider == "mock" {
		return c, errors.New("mock provider forbidden in production")
	}
	if c.Provider == "docker" && (len(c.AgentToken) < 32 || c.AgentURL == "") {
		return c, errors.New("docker provider requires private agent URL and token")
	}
	return c, nil
}
