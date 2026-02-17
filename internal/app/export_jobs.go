package app

import (
	"sync"
	"time"
)

type exportJob struct {
	ID                     string    `json:"id"`
	AccountID              int64     `json:"account_id"`
	Provider               string    `json:"provider"`
	Format                 string    `json:"format"`
	State                  string    `json:"state"`
	TotalPlaylists         int       `json:"total_playlists"`
	CompletedPlaylists     int       `json:"completed_playlists"`
	SkippedPlaylists       int       `json:"skipped_playlists"`
	CurrentPlaylist        string    `json:"current_playlist"`
	CompletedPlaylistNames []string  `json:"completed_playlist_names"`
	SkippedPlaylistNames   []string  `json:"skipped_playlist_names"`
	DownloadURL            string    `json:"download_url,omitempty"`
	Error                  string    `json:"error,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type exportJobManager struct {
	jobs map[string]*exportJob
	mu   sync.Mutex
}

func newExportJobManager() *exportJobManager {
	return &exportJobManager{
		jobs: map[string]*exportJob{},
	}
}

func (m *exportJobManager) withLock(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn()
}

func (m *exportJobManager) create(accountID int64, provider, format string) (exportJob, error) {
	id, err := randomState(12)
	if err != nil {
		return exportJob{}, err
	}

	now := time.Now().UTC()
	job := &exportJob{
		ID:                     id,
		AccountID:              accountID,
		Provider:               provider,
		Format:                 format,
		State:                  "running",
		CompletedPlaylistNames: []string{},
		SkippedPlaylistNames:   []string{},
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	m.withLock(func() {
		m.jobs[id] = job
	})
	return cloneExportJob(job), nil
}

func (m *exportJobManager) update(id string, fn func(*exportJob)) bool {
	updated := false
	m.withLock(func() {
		job, ok := m.jobs[id]
		if !ok {
			return
		}
		fn(job)
		job.UpdatedAt = time.Now().UTC()
		updated = true
	})
	return updated
}

func (m *exportJobManager) get(id string) (exportJob, bool) {
	var out exportJob
	ok := false
	m.withLock(func() {
		job, exists := m.jobs[id]
		if !exists {
			return
		}
		out = cloneExportJob(job)
		ok = true
	})
	return out, ok
}

func cloneExportJob(src *exportJob) exportJob {
	out := *src
	out.CompletedPlaylistNames = append([]string(nil), src.CompletedPlaylistNames...)
	out.SkippedPlaylistNames = append([]string(nil), src.SkippedPlaylistNames...)
	return out
}
