package store

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/n1ckerr0r/pull-requests-service/internal/domain"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")

	ErrPRMerged            = errors.New("pr merged")
	ErrReviewerNotAssigned = errors.New("reviewer not assigned")
	ErrNoCandidate         = errors.New("no candidate")
)

type Store struct {
	db *sqlx.DB
}

func (s *Store) DB() *sqlx.DB {
	return s.db
}

func NewStore(dbURL string) (*Store, error) {
	db, err := sqlx.Connect("postgres", dbURL)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	return &Store{db: db}, nil
}

func (s *Store) CreateTeam(ctx context.Context, t *domain.Team) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO teams (name, description, created_at) VALUES ($1,$2,now())`, t.Name, t.Description)
	if err != nil {
		return ErrAlreadyExists
	}
	return nil
}

func (s *Store) GetTeam(ctx context.Context, name string) (*domain.Team, []domain.User, error) {
	var team domain.Team
	err := s.db.GetContext(ctx, &team, `SELECT name, description, created_at FROM teams WHERE name = $1`, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	var members []domain.User
	err = s.db.SelectContext(ctx, &members, `SELECT user_id, username, is_active, team_name, created_at, updated_at FROM users WHERE team_name = $1`, name)
	if err != nil {
		return &team, nil, err
	}
	return &team, members, nil
}

func (s *Store) UpsertUser(ctx context.Context, u *domain.User) error {
	_, err := s.db.ExecContext(ctx, `
       INSERT INTO users (user_id, username, is_active, team_name, created_at, updated_at)
       VALUES ($1,$2,$3,$4,now(),now())
       ON CONFLICT (user_id) DO UPDATE SET username = EXCLUDED.username, is_active = EXCLUDED.is_active, team_name = EXCLUDED.team_name, updated_at = now()
`, u.ID, u.Username, u.IsActive, u.TeamName)
	return err
}

func (s *Store) GetUser(ctx context.Context, id string) (*domain.User, error) {
	var u domain.User
	err := s.db.GetContext(ctx, &u, `SELECT user_id, username, is_active, team_name, created_at, updated_at FROM users WHERE user_id = $1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) SetUserActive(ctx context.Context, id string, active bool) (*domain.User, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET is_active = $1, updated_at = now() WHERE user_id = $2`, active, id)
	if err != nil {
		return nil, err
	}
	return s.GetUser(ctx, id)
}

func (s *Store) CreatePR(ctx context.Context, pr *domain.PullRequest) error {
	var exists string
	err := s.db.GetContext(ctx, &exists, `SELECT pull_request_id FROM prs WHERE pull_request_id = $1`, pr.ID)
	if err == nil {
		return ErrAlreadyExists
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO prs (pull_request_id, pull_request_name, author_id, status, created_at) VALUES ($1,$2,$3,$4,now())`,
		pr.ID, pr.Name, pr.AuthorID, pr.Status)
	return err
}

func (s *Store) GetPR(ctx context.Context, id string) (*domain.PullRequest, error) {
	var pr domain.PullRequest
	err := s.db.GetContext(ctx, &pr, `SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at FROM prs WHERE pull_request_id = $1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var reviewers []string
	err = s.db.SelectContext(ctx, &reviewers, `SELECT user_id FROM pr_assignments WHERE pull_request_id = $1 ORDER BY slot ASC`, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	pr.AssignedReviewers = make([]domain.UserID, 0, len(reviewers))
	for _, r := range reviewers {
		pr.AssignedReviewers = append(pr.AssignedReviewers, domain.UserID(r))
	}
	return &pr, nil
}

func (s *Store) ReassignReviewer(ctx context.Context, prID, oldReviewerID string) (string, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			log.Printf("warning: rollback failed in ReassignReviewer: %v", rollbackErr)
		}
	}()

	var status string
	if err := tx.GetContext(ctx, &status, `SELECT status FROM prs WHERE pull_request_id = $1 FOR UPDATE`, prID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	if status != "OPEN" {
		return "", ErrPRMerged
	}

	var slot int
	if err := tx.GetContext(ctx, &slot, `SELECT slot FROM pr_assignments WHERE pull_request_id = $1 AND user_id = $2`, prID, oldReviewerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrReviewerNotAssigned
		}
		return "", err
	}

	var teamName string
	if err := tx.GetContext(ctx, &teamName, `SELECT team_name FROM users WHERE user_id = $1`, oldReviewerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	var candidate string
	if err := tx.GetContext(ctx, &candidate, `
        SELECT user_id FROM users
        WHERE team_name = $1 AND is_active = true AND user_id NOT IN (
            SELECT user_id FROM pr_assignments WHERE pull_request_id = $2
        ) AND user_id != $3
        ORDER BY random()
        LIMIT 1
`, teamName, prID, oldReviewerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNoCandidate
		}
		return "", err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE pr_assignments SET user_id = $1, assigned_at = now() WHERE pull_request_id = $2 AND slot = $3`, candidate, prID, slot); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return candidate, nil
}

func (s *Store) AssignReviewers(ctx context.Context, prID string, reviewers []string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			log.Printf("warning: rollback failed in AssignReviewers: %v", rollbackErr)
		}
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM pr_assignments WHERE pull_request_id = $1`, prID); err != nil {
		return err
	}

	for i, uid := range reviewers {
		slot := i + 1
		if _, err := tx.ExecContext(ctx, `INSERT INTO pr_assignments (pull_request_id, user_id, slot, assigned_at) VALUES ($1,$2,$3,now())`, prID, uid, slot); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetPRMerged(ctx context.Context, prID string) (*domain.PullRequest, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			log.Printf("warning: rollback failed in SetPRMerged: %v", rollbackErr)
		}
	}()

	var status string
	if err := tx.GetContext(ctx, &status, `SELECT status FROM prs WHERE pull_request_id = $1 FOR UPDATE`, prID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if status == "MERGED" {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			log.Printf("warning: rollback failed in SetPRMerged (already merged): %v", rollbackErr)
		}
		return s.GetPR(ctx, prID)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE prs SET status='MERGED', merged_at = now() WHERE pull_request_id = $1`, prID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetPR(ctx, prID)
}

func (s *Store) GetPRsByReviewer(ctx context.Context, userID string) ([]domain.PullRequest, error) {
	var prs []domain.PullRequest
	err := s.db.SelectContext(ctx, &prs, `SELECT p.pull_request_id, p.pull_request_name, p.author_id, p.status, p.created_at, p.merged_at
       FROM prs p JOIN pr_assignments a ON p.pull_request_id = a.pull_request_id
       WHERE a.user_id = $1 AND p.status = 'OPEN'`, userID)
	if err != nil {
		return nil, err
	}
	for i := range prs {
		var revs []string
		if err := s.db.SelectContext(ctx, &revs, `SELECT user_id FROM pr_assignments WHERE pull_request_id = $1 ORDER BY slot`, prs[i].ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		prs[i].AssignedReviewers = make([]domain.UserID, 0, len(revs))
		for _, r := range revs {
			prs[i].AssignedReviewers = append(prs[i].AssignedReviewers, domain.UserID(r))
		}
	}
	return prs, nil
}
