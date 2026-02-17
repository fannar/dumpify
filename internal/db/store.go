package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"dumpify/internal/domain"
)

type Store struct {
	mu   sync.Mutex
	path string
	data storeData
}

type storeData struct {
	NextAccountID int64                         `json:"next_account_id"`
	NextExportID  int64                         `json:"next_export_id"`
	Accounts      map[int64]domain.Account      `json:"accounts"`
	Exports       map[int64]domain.ExportRecord `json:"exports"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = storeData{
		NextAccountID: 1,
		NextExportID:  1,
		Accounts:      map[int64]domain.Account{},
		Exports:       map[int64]domain.ExportRecord{},
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.persistLocked()
		}
		return fmt.Errorf("read db: %w", err)
	}

	if len(b) == 0 {
		return s.persistLocked()
	}

	if err := json.Unmarshal(b, &s.data); err != nil {
		return fmt.Errorf("parse db: %w", err)
	}

	if s.data.Accounts == nil {
		s.data.Accounts = map[int64]domain.Account{}
	}
	if s.data.Exports == nil {
		s.data.Exports = map[int64]domain.ExportRecord{}
	}
	if s.data.NextAccountID < 1 {
		s.data.NextAccountID = maxAccountID(s.data.Accounts) + 1
	}
	if s.data.NextExportID < 1 {
		s.data.NextExportID = maxExportID(s.data.Exports) + 1
	}

	return nil
}

func (s *Store) UpsertAccount(provider string, user domain.User, token domain.AuthToken) (domain.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	for id, account := range s.data.Accounts {
		if account.Provider == provider && account.User.ID == user.ID {
			account.User = user
			account.Token = token
			account.UpdatedAt = now
			s.data.Accounts[id] = account
			if err := s.persistLocked(); err != nil {
				return domain.Account{}, err
			}
			return account, nil
		}
	}

	account := domain.Account{
		ID:        s.data.NextAccountID,
		Provider:  provider,
		User:      user,
		Token:     token,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.data.NextAccountID++
	s.data.Accounts[account.ID] = account

	if err := s.persistLocked(); err != nil {
		return domain.Account{}, err
	}

	return account, nil
}

func (s *Store) GetAccount(id int64) (domain.Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	account, ok := s.data.Accounts[id]
	return account, ok
}

func (s *Store) UpdateAccountToken(id int64, token domain.AuthToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	account, ok := s.data.Accounts[id]
	if !ok {
		return fmt.Errorf("account %d not found", id)
	}
	account.Token = token
	account.UpdatedAt = time.Now().UTC()
	s.data.Accounts[id] = account
	return s.persistLocked()
}

func (s *Store) CreateExport(accountID int64, provider, format, filePath string) (domain.ExportRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec := domain.ExportRecord{
		ID:        s.data.NextExportID,
		AccountID: accountID,
		Provider:  provider,
		Format:    format,
		FilePath:  filePath,
		CreatedAt: time.Now().UTC(),
	}
	s.data.NextExportID++
	s.data.Exports[rec.ID] = rec

	if err := s.persistLocked(); err != nil {
		return domain.ExportRecord{}, err
	}
	return rec, nil
}

func (s *Store) GetExport(id int64) (domain.ExportRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.data.Exports[id]
	return rec, ok
}

func (s *Store) ListExportsForAccount(accountID int64, limit int) []domain.ExportRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]domain.ExportRecord, 0, len(s.data.Exports))
	for _, rec := range s.data.Exports {
		if rec.AccountID == accountID {
			out = append(out, rec)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Store) persistLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize db: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write temp db: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace db: %w", err)
	}
	return nil
}

func maxAccountID(accounts map[int64]domain.Account) int64 {
	var max int64
	for id := range accounts {
		if id > max {
			max = id
		}
	}
	return max
}

func maxExportID(exports map[int64]domain.ExportRecord) int64 {
	var max int64
	for id := range exports {
		if id > max {
			max = id
		}
	}
	return max
}
