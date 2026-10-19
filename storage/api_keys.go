package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// GenerateRandomApiKey generates a secure key string with a gtc_ prefix
func GenerateRandomApiKey() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "gtc_" + hex.EncodeToString(bytes), nil
}

// MaskApiKey masks the key for safe UI display (e.g. gtc_1234...abcd)
func MaskApiKey(key string) string {
	if len(key) <= 12 {
		return "••••••••"
	}
	prefix := key[:8]
	suffix := key[len(key)-4:]
	return prefix + "..." + suffix
}

// CreateUserApiKey generates and saves a new API key for a user
func (s *Storage) CreateUserApiKey(userId, name string) (UserApiKey, string, error) {
	if strings.TrimSpace(userId) == "" {
		return UserApiKey{}, "", errors.New("user_id is required")
	}
	rawKey, err := GenerateRandomApiKey()
	if err != nil {
		return UserApiKey{}, "", fmt.Errorf("failed to generate api key: %w", err)
	}

	keyName := strings.TrimSpace(name)
	if keyName == "" {
		keyName = "API Key"
	}

	apiKey := UserApiKey{
		UserId:  userId,
		Key:     rawKey,
		Name:    keyName,
		Created: time.Now(),
	}

	if err := s.db.Create(&apiKey).Error; err != nil {
		return UserApiKey{}, "", fmt.Errorf("failed to save api key: %w", err)
	}

	return apiKey, rawKey, nil
}

// GetUserApiKeys returns all API keys for a user, with keys masked
func (s *Storage) GetUserApiKeys(userId string) ([]UserApiKey, error) {
	var keys []UserApiKey
	if err := s.db.Where("user_id = ?", userId).Order("created desc").Find(&keys).Error; err != nil {
		return nil, err
	}
	for i := range keys {
		keys[i].Key = MaskApiKey(keys[i].Key)
	}
	return keys, nil
}

// DeleteUserApiKey removes an API key belonging to the user
func (s *Storage) DeleteUserApiKey(userId string, keyId uint) error {
	r := s.db.Where("id = ? AND user_id = ?", keyId, userId).Delete(&UserApiKey{})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		return errors.New("api key not found or unauthorized")
	}
	return nil
}

// GetUserByApiKey resolves a user by API key and updates last_used timestamp
func (s *Storage) GetUserByApiKey(key string) (*User, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("empty api key")
	}

	var apiKey UserApiKey
	if err := s.db.Where("key = ?", key).First(&apiKey).Error; err != nil {
		return nil, err
	}

	// Update last_used
	now := time.Now()
	_ = s.db.Model(&apiKey).Update("last_used", &now).Error

	// Fetch user details
	user, err := s.GetUser(apiKey.UserId)
	if err != nil {
		return &User{
			DiscordId: apiKey.UserId,
			Name:      apiKey.UserId,
			Handle:    apiKey.UserId,
		}, nil
	}
	return user, nil
}
