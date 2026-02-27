package models

import (
	"database/sql"
	"time"
)

type Model struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	BackendURL  string    `json:"backend_url"`
	Enabled     bool      `json:"enabled"`
	Weight      int       `json:"weight"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ModelCreateRequest struct {
	ID          string `json:"id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	BackendURL  string `json:"backend_url" binding:"required,url"`
	Weight      int    `json:"weight"`
	Description string `json:"description"`
}

type ModelUpdateRequest struct {
	Name        string `json:"name"`
	BackendURL  string `json:"backend_url" binding:"omitempty,url"`
	Enabled     *bool  `json:"enabled"`
	Weight      int    `json:"weight"`
	Description string `json:"description"`
}

// ModelStore 模型配置数据访问层
type ModelStore struct {
	db *sql.DB
}

func NewModelStore(db *sql.DB) *ModelStore {
	return &ModelStore{db: db}
}

func (s *ModelStore) Create(model *Model) error {
	query := `
		INSERT INTO models (id, name, backend_url, enabled, weight, description)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			backend_url = excluded.backend_url,
			enabled = excluded.enabled,
			weight = excluded.weight,
			description = excluded.description,
			updated_at = CURRENT_TIMESTAMP
		RETURNING created_at, updated_at`

	return s.db.QueryRow(query,
		model.ID, model.Name, model.BackendURL, model.Enabled,
		model.Weight, model.Description,
	).Scan(&model.CreatedAt, &model.UpdatedAt)
}

func (s *ModelStore) GetByID(id string) (*Model, error) {
	model := &Model{}
	query := `
		SELECT id, name, backend_url, enabled, weight, description, created_at, updated_at
		FROM models WHERE id = ?`

	err := s.db.QueryRow(query, id).Scan(
		&model.ID, &model.Name, &model.BackendURL, &model.Enabled,
		&model.Weight, &model.Description, &model.CreatedAt, &model.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return model, err
}

func (s *ModelStore) List() ([]*Model, error) {
	query := `
		SELECT id, name, backend_url, enabled, weight, description, created_at, updated_at
		FROM models ORDER BY created_at`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []*Model
	for rows.Next() {
		model := &Model{}
		err := rows.Scan(
			&model.ID, &model.Name, &model.BackendURL, &model.Enabled,
			&model.Weight, &model.Description, &model.CreatedAt, &model.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, rows.Err()
}

func (s *ModelStore) ListEnabled() ([]*Model, error) {
	query := `
		SELECT id, name, backend_url, enabled, weight, description, created_at, updated_at
		FROM models WHERE enabled = 1 ORDER BY weight DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []*Model
	for rows.Next() {
		model := &Model{}
		err := rows.Scan(
			&model.ID, &model.Name, &model.BackendURL, &model.Enabled,
			&model.Weight, &model.Description, &model.CreatedAt, &model.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, rows.Err()
}

func (s *ModelStore) Update(model *Model) error {
	query := `
		UPDATE models SET
			name = ?, backend_url = ?, enabled = ?, weight = ?,
			description = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	_, err := s.db.Exec(query,
		model.Name, model.BackendURL, model.Enabled, model.Weight,
		model.Description, model.ID,
	)
	return err
}

func (s *ModelStore) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM models WHERE id = ?", id)
	return err
}
