// Package appconfig owns local application preferences independently of remote hosts.
package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

type Connection struct {
	URL     string `json:"url"`
	Token   string `json:"token"`
	Enabled bool   `json:"enabled"`
}
type Connections struct {
	mu     sync.Mutex
	store  *storage.Store
	config Connection
	active string
}

func OpenConnections(store *storage.Store) (*Connections, error) {
	manager := &Connections{store: store}
	data, err := os.ReadFile(filepath.Join(store.ConfigDirectory(), "connection.json"))
	if err == nil {
		err = json.Unmarshal(data, &manager.config)
	}
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return manager, nil
}
func (m *Connections) Credentials() Connection { m.mu.Lock(); defer m.mu.Unlock(); return m.config }
func (m *Connections) Activate(address string) { m.mu.Lock(); m.active = address; m.mu.Unlock() }
func (m *Connections) ConnectionInfo(ctx context.Context) (contracts.ConnectionInfo, error) {
	if err := ctx.Err(); err != nil {
		return contracts.ConnectionInfo{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	target := ""
	if m.config.Enabled {
		target = m.config.URL
	}
	return contracts.ConnectionInfo{URL: m.config.URL, Enabled: m.config.Enabled, HasToken: m.config.Token != "", ActiveURL: m.active, Pending: target != m.active}, nil
}
func (m *Connections) ConfigureConnection(ctx context.Context, input contracts.ConnectionInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	config := Connection{URL: strings.TrimRight(strings.TrimSpace(input.URL), "/"), Token: input.Token, Enabled: input.Enabled}
	if input.PreserveToken {
		config.Token = m.config.Token
	}
	if input.PreserveToken && config.Enabled && config.URL != m.config.URL {
		return errors.New("enter the token when changing the remote host")
	}
	if config.Enabled {
		address, err := contracts.ValidateRemoteURL(config.URL)
		if err != nil {
			return err
		}
		config.URL = address
		if len(config.Token) < 24 {
			return errors.New("remote API token must contain at least 24 characters")
		}
	}
	if err := m.store.Save("connection.json", config); err != nil {
		return err
	}
	m.config = config
	return nil
}
