package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Entry struct {
	Name        string `json:"name,omitempty"`
	ClientID    string `json:"client_id"`
	BearerToken string `json:"bearer_token"`
	Thumbprint  string `json:"thumbprint,omitempty"`
	ServerURL   string `json:"server_url,omitempty"`
}

type File struct {
	DefaultServerURL  string  `json:"default_server_url,omitempty"`
	DefaultThumbprint string  `json:"default_thumbprint,omitempty"`
	Plaintext         bool    `json:"plaintext,omitempty"`
	PinCertificate    bool    `json:"pin_certificate,omitempty"`
	Gateways          []Entry `json:"gateways"`
}

func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read credentials file %q: %w", path, err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse credentials file %q: %w", path, err)
	}
	if len(f.Gateways) == 0 {
		return nil, fmt.Errorf("credentials file %q has no gateways", path)
	}
	for i := range f.Gateways {
		e := &f.Gateways[i]
		if e.ClientID == "" {
			return nil, fmt.Errorf("gateway[%d] is missing client_id", i)
		}
		if e.BearerToken == "" {
			return nil, fmt.Errorf("gateway[%d] (%s) is missing bearer_token", i, e.ClientID)
		}
		if e.ServerURL == "" {
			e.ServerURL = f.DefaultServerURL
		}
		if e.Thumbprint == "" {
			e.Thumbprint = f.DefaultThumbprint
		}
		if strings.TrimSpace(e.ServerURL) == "" {
			return nil, fmt.Errorf("gateway[%d] (%s) has no server_url and no default_server_url is set", i, e.ClientID)
		}
	}
	return &f, nil
}

func (f *File) Select(n int) []Entry {
	if n <= 0 || n >= len(f.Gateways) {
		return f.Gateways
	}
	return f.Gateways[:n]
}
