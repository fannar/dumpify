package domain

import "time"

type AuthToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
}

type Track struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Artists    []string  `json:"artists"`
	Album      string    `json:"album"`
	DurationMS int       `json:"duration_ms"`
	URI        string    `json:"uri"`
	AddedAt    time.Time `json:"added_at,omitempty"`
	AddedBy    string    `json:"added_by,omitempty"`
}

type Playlist struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	OwnerID       string  `json:"owner_id"`
	Public        bool    `json:"public"`
	Collaborative bool    `json:"collaborative"`
	SnapshotID    string  `json:"snapshot_id"`
	URI           string  `json:"uri"`
	Tracks        []Track `json:"tracks"`
}

type PlaylistDump struct {
	Provider   string     `json:"provider"`
	ExportedAt time.Time  `json:"exported_at"`
	User       User       `json:"user"`
	Playlists  []Playlist `json:"playlists"`
}

type Account struct {
	ID        int64     `json:"id"`
	Provider  string    `json:"provider"`
	User      User      `json:"user"`
	Token     AuthToken `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ExportRecord struct {
	ID        int64     `json:"id"`
	AccountID int64     `json:"account_id"`
	Provider  string    `json:"provider"`
	Format    string    `json:"format"`
	FilePath  string    `json:"file_path"`
	CreatedAt time.Time `json:"created_at"`
}
