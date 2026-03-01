package models

import (
	"database/sql"
	"time"
)

// Backend represents a backend instance for a model
type Backend struct {
	ID          string    `json:"id"`
	ModelID     string    `json:"model_id"`
	Name        string    `json:"name"`
	BaseURL     string    `json:"base_url"`
	APIKey      string    `json:"-"` // Never return API key in JSON
	ModelName   string    `json:"model_name"`
	Weight      int       `json:"weight"`
	Region      string    `json:"region"`
	Enabled     bool      `json:"enabled"`
	Healthy     bool      `json:"healthy"`
	LastCheckAt sql.NullTime `json:"last_check_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BackendCreateRequest represents a request to create a backend
type BackendCreateRequest struct {
	ID        string `json:"id" binding:"required"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url" binding:"required,url"`
	APIKey    string `json:"api_key"`
	ModelName string `json:"model_name"`
	Weight    int    `json:"weight"`
	Region    string `json:"region"`
	Enabled   bool   `json:"enabled"`
}

// BackendUpdateRequest represents a request to update a backend
type BackendUpdateRequest struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url" binding:"omitempty,url"`
	APIKey    string `json:"api_key"`
	ModelName string `json:"model_name"`
	Weight    int    `json:"weight"`
	Region    string `json:"region"`
	Enabled   *bool  `json:"enabled"`
}

// BackendStore handles database operations for backends
type BackendStore struct {
	db *sql.DB
}

// NewBackendStore creates a new BackendStore
func NewBackendStore(db *sql.DB) *BackendStore {
	return &BackendStore{db: db}
}

// Create creates a new backend
func (s *BackendStore) Create(backend *Backend) error {
	query := `
		INSERT INTO backends (id, model_id, name, base_url, api_key, model_name, weight, region, enabled, healthy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			model_id = excluded.model_id,
			name = excluded.name,
			base_url = excluded.base_url,
			api_key = excluded.api_key,
			model_name = excluded.model_name,
			weight = excluded.weight,
			region = excluded.region,
			enabled = excluded.enabled,
			updated_at = CURRENT_TIMESTAMP
		RETURNING created_at, updated_at`

	return s.db.QueryRow(query,
		backend.ID, backend.ModelID, backend.Name, backend.BaseURL, backend.APIKey,
		backend.ModelName, backend.Weight, backend.Region, backend.Enabled, backend.Healthy,
	).Scan(&backend.CreatedAt, &backend.UpdatedAt)
}

// GetByID retrieves a backend by ID
func (s *BackendStore) GetByID(id string) (*Backend, error) {
	backend := &Backend{}
	query := `
		SELECT id, model_id, name, base_url, api_key, model_name, weight, region, enabled, healthy, last_check_at, created_at, updated_at
		FROM backends WHERE id = ?`

	err := s.db.QueryRow(query, id).Scan(
		&backend.ID, &backend.ModelID, &backend.Name, &backend.BaseURL, &backend.APIKey,
		&backend.ModelName, &backend.Weight, &backend.Region, &backend.Enabled, &backend.Healthy,
		&backend.LastCheckAt, &backend.CreatedAt, &backend.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return backend, err
}

// List retrieves all backends
func (s *BackendStore) List() ([]*Backend, error) {
	query := `
		SELECT id, model_id, name, base_url, api_key, model_name, weight, region, enabled, healthy, last_check_at, created_at, updated_at
		FROM backends ORDER BY created_at`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backends []*Backend
	for rows.Next() {
		backend := &Backend{}
		err := rows.Scan(
			&backend.ID, &backend.ModelID, &backend.Name, &backend.BaseURL, &backend.APIKey,
			&backend.ModelName, &backend.Weight, &backend.Region, &backend.Enabled, &backend.Healthy,
			&backend.LastCheckAt, &backend.CreatedAt, &backend.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		backends = append(backends, backend)
	}
	return backends, rows.Err()
}

// ListByModel retrieves all backends for a specific model
func (s *BackendStore) ListByModel(modelID string) ([]*Backend, error) {
	query := `
		SELECT id, model_id, name, base_url, api_key, model_name, weight, region, enabled, healthy, last_check_at, created_at, updated_at
		FROM backends WHERE model_id = ? ORDER BY created_at`

	rows, err := s.db.Query(query, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backends []*Backend
	for rows.Next() {
		backend := &Backend{}
		err := rows.Scan(
			&backend.ID, &backend.ModelID, &backend.Name, &backend.BaseURL, &backend.APIKey,
			&backend.ModelName, &backend.Weight, &backend.Region, &backend.Enabled, &backend.Healthy,
			&backend.LastCheckAt, &backend.CreatedAt, &backend.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		backends = append(backends, backend)
	}
	return backends, rows.Err()
}

// ListEnabled retrieves all enabled backends
func (s *BackendStore) ListEnabled() ([]*Backend, error) {
	query := `
		SELECT id, model_id, name, base_url, api_key, model_name, weight, region, enabled, healthy, last_check_at, created_at, updated_at
		FROM backends WHERE enabled = 1 ORDER BY weight DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backends []*Backend
	for rows.Next() {
		backend := &Backend{}
		err := rows.Scan(
			&backend.ID, &backend.ModelID, &backend.Name, &backend.BaseURL, &backend.APIKey,
			&backend.ModelName, &backend.Weight, &backend.Region, &backend.Enabled, &backend.Healthy,
			&backend.LastCheckAt, &backend.CreatedAt, &backend.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		backends = append(backends, backend)
	}
	return backends, rows.Err()
}

// ListEnabledByModel retrieves all enabled backends for a specific model
func (s *BackendStore) ListEnabledByModel(modelID string) ([]*Backend, error) {
	query := `
		SELECT id, model_id, name, base_url, api_key, model_name, weight, region, enabled, healthy, last_check_at, created_at, updated_at
		FROM backends WHERE model_id = ? AND enabled = 1 ORDER BY weight DESC`

	rows, err := s.db.Query(query, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backends []*Backend
	for rows.Next() {
		backend := &Backend{}
		err := rows.Scan(
			&backend.ID, &backend.ModelID, &backend.Name, &backend.BaseURL, &backend.APIKey,
			&backend.ModelName, &backend.Weight, &backend.Region, &backend.Enabled, &backend.Healthy,
			&backend.LastCheckAt, &backend.CreatedAt, &backend.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		backends = append(backends, backend)
	}
	return backends, rows.Err()
}

// Update updates a backend
func (s *BackendStore) Update(backend *Backend) error {
	query := `
		UPDATE backends SET
			name = ?, base_url = ?, api_key = ?, model_name = ?,
			weight = ?, region = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	_, err := s.db.Exec(query,
		backend.Name, backend.BaseURL, backend.APIKey, backend.ModelName,
		backend.Weight, backend.Region, backend.Enabled, backend.ID,
	)
	return err
}

// Delete deletes a backend
func (s *BackendStore) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM backends WHERE id = ?", id)
	return err
}

// UpdateHealth updates the health status of a backend
func (s *BackendStore) UpdateHealth(id string, healthy bool) error {
	query := `
		UPDATE backends SET
			healthy = ?, last_check_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	_, err := s.db.Exec(query, healthy, id)
	return err
}

// DeleteByModel deletes all backends for a model (used when model is deleted)
func (s *BackendStore) DeleteByModel(modelID string) error {
	_, err := s.db.Exec("DELETE FROM backends WHERE model_id = ?", modelID)
	return err
}
