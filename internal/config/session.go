package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Session struct {
	Cookies map[string]string `json:"cookies"`
	Domain  string            `json:"domain"`
	Mode    string            `json:"mode,omitempty"`
}

func SessionPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "zlib", "session.json")
}

func LoadSession() (Session, error) {
	data, err := os.ReadFile(SessionPath())
	if err != nil {
		return Session{}, err
	}
	var s Session
	return s, json.Unmarshal(data, &s)
}

func SaveSession(s Session) error {
	return saveJSONAtomic(SessionPath(), s)
}
