package repository

import (
	"context"
	"database/sql"

	"shipdude/internal/db"
	"shipdude/internal/model"
	"shipdude/pkg/dbutil"
)

// Project describes the data access contract for deployed projects.
type Project interface {
	FindAll(ctx context.Context, opts ...dbutil.Scope) ([]model.Project, error)
	FindOne(ctx context.Context, id int64) (*model.Project, error)
	FindOneByName(ctx context.Context, name string) (*model.Project, error)
	FindOneByRepo(ctx context.Context, repo string) (*model.Project, error)
	Store(db *sql.DB, project model.Project) (int64, error)
	UpdateOne(db *sql.DB, project model.Project) error
	UpdateActiveState(db *sql.DB, id int64, activePort int, activeImage string) error
	DeleteOne(db *sql.DB, id int64) error
}

type project struct {
	Db *db.DB
}

// NewProjectRepository wires a Project repository over the database.
func NewProjectRepository(database *db.DB) Project {
	return &project{Db: database}
}

// projectSelect joins each project with its server so ServerIP is populated.
const projectSelect = `SELECT p.id, p.server_id, p.github_repo, p.github_branch, p.project_name,
	p.deployment_path, p.stack_type, p.stack_version, p.app_port, p.domain,
	p.active_port, p.active_image, s.ip_address AS server_ip
	FROM projects p
	JOIN servers s ON p.server_id = s.id`

// FindAll lists all projects joined with their server IP.
func (r *project) FindAll(ctx context.Context, opts ...dbutil.Scope) ([]model.Project, error) {
	q := dbutil.ApplyScopes(opts...)

	rows, err := r.Db.SQL().QueryContext(ctx, projectSelect+buildTail(q), q.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Project
	for rows.Next() {
		var proj model.Project
		if err := scanProject(rows, &proj); err != nil {
			return nil, err
		}
		list = append(list, proj)
	}

	return list, rows.Err()
}

// FindOne returns a single project by ID.
func (r *project) FindOne(ctx context.Context, id int64) (*model.Project, error) {
	row := r.Db.SQL().QueryRowContext(ctx, projectSelect+" WHERE p.id = ?", id)
	return scanProjectPtr(row)
}

// FindOneByName returns a project by its unique-ish name.
func (r *project) FindOneByName(ctx context.Context, name string) (*model.Project, error) {
	row := r.Db.SQL().QueryRowContext(ctx, projectSelect+" WHERE p.project_name = ? ORDER BY p.id DESC LIMIT 1", name)
	return scanProjectPtr(row)
}

// FindOneByRepo returns the most recent project for a GitHub repository.
func (r *project) FindOneByRepo(ctx context.Context, repo string) (*model.Project, error) {
	row := r.Db.SQL().QueryRowContext(ctx, projectSelect+" WHERE p.github_repo = ? ORDER BY p.id DESC LIMIT 1", repo)
	return scanProjectPtr(row)
}

// Store inserts a new project and returns its ID.
func (r *project) Store(database *sql.DB, proj model.Project) (int64, error) {
	res, err := database.Exec(
		`INSERT INTO projects (server_id, github_repo, github_branch, project_name, deployment_path, stack_type, stack_version, app_port, domain, active_port, active_image)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		proj.ServerID, proj.GitHubRepo, proj.GitHubBranch, proj.ProjectName, proj.DeploymentPath,
		proj.StackType, proj.StackVersion, proj.AppPort, proj.Domain, proj.ActivePort, proj.ActiveImage,
	)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

// UpdateOne rewrites the editable configuration fields of a project.
func (r *project) UpdateOne(database *sql.DB, proj model.Project) error {
	_, err := database.Exec(
		`UPDATE projects SET server_id = ?, github_repo = ?, github_branch = ?, deployment_path = ?,
		 stack_type = ?, stack_version = ?, app_port = ?, domain = ? WHERE id = ?`,
		proj.ServerID, proj.GitHubRepo, proj.GitHubBranch, proj.DeploymentPath,
		proj.StackType, proj.StackVersion, proj.AppPort, proj.Domain, proj.ID,
	)
	return err
}

// UpdateActiveState records which port and image are currently serving traffic.
func (r *project) UpdateActiveState(database *sql.DB, id int64, activePort int, activeImage string) error {
	_, err := database.Exec(
		`UPDATE projects SET active_port = ?, active_image = ? WHERE id = ?`,
		activePort, activeImage, id,
	)
	return err
}

// DeleteOne removes a project row.
func (r *project) DeleteOne(database *sql.DB, id int64) error {
	_, err := database.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

func scanProject(row rowScanner, proj *model.Project) error {
	return row.Scan(
		&proj.ID, &proj.ServerID, &proj.GitHubRepo, &proj.GitHubBranch, &proj.ProjectName,
		&proj.DeploymentPath, &proj.StackType, &proj.StackVersion, &proj.AppPort, &proj.Domain,
		&proj.ActivePort, &proj.ActiveImage, &proj.ServerIP,
	)
}

func scanProjectPtr(row rowScanner) (*model.Project, error) {
	var proj model.Project
	if err := scanProject(row, &proj); err != nil {
		return nil, err
	}
	return &proj, nil
}
