package platform

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"time"

	"deployfox/internal/config"
	"deployfox/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
)

type Store struct {
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Config config.Config
}

func Open(ctx context.Context, c config.Config) (*Store, error) {
	db, err := sql.Open("pgx", c.DatabaseURL)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		return nil, err
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, c.DatabaseURL)
	if err != nil {
		return nil, err
	}
	opts, err := redis.ParseURL(c.RedisURL)
	if err != nil {
		pool.Close()
		return nil, err
	}
	r := redis.NewClient(opts)
	if err = r.Ping(ctx).Err(); err != nil {
		pool.Close()
		r.Close()
		return nil, err
	}
	return &Store{DB: pool, Redis: r, Config: c}, nil
}

func (s *Store) Close() { s.DB.Close(); s.Redis.Close() }
func (s *Store) Audit(ctx context.Context, action, target string) {
	if _, err := s.DB.Exec(ctx, "INSERT INTO audit_events(action,target) VALUES($1,$2)", action, target); err != nil {
		slog.Error("audit write failed", "error", err)
	}
}
func (s *Store) Wake(ctx context.Context) {
	if err := s.Redis.Eval(ctx, `redis.call('LPUSH',KEYS[1],'1'); redis.call('LTRIM',KEYS[1],0,99); return 1`, []string{"deployfox:wake"}).Err(); err != nil {
		slog.Warn("queue notification failed; database polling will recover", "error", err)
	}
}
func (s *Store) Log(ctx context.Context, id, line string) {
	line = cleanLogLine(line)
	if _, err := s.DB.Exec(ctx, "INSERT INTO deployment_logs(deployment_id,line) VALUES($1,$2)", id, line); err != nil {
		slog.Error("log write failed", "error", err)
	}
}

func cleanLogLine(line string) string {
	// PostgreSQL text rejects NUL and invalid UTF-8. Build tools may emit both.
	line = strings.ReplaceAll(line, "\x00", "")
	if len(line) > 4096 {
		line = line[:4096]
	}
	return strings.ToValidUTF8(line, "")
}

type Project struct {
	ServiceType     string    `json:"service_type"`
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	SourceType      string    `json:"source_type"`
	Source          string    `json:"source"`
	Branch          string    `json:"branch"`
	BuildMethod     string    `json:"build_method"`
	Dockerfile      string    `json:"dockerfile"`
	BuildContext    string    `json:"build_context"`
	Port            int       `json:"port"`
	Domain          string    `json:"domain"`
	HealthPath      string    `json:"health_path"`
	AutoDeploy      bool      `json:"auto_deploy"`
	MemoryMB        int       `json:"memory_mb"`
	CPU             float64   `json:"cpu"`
	ActiveContainer string    `json:"active_container"`
	ActiveImage     string    `json:"active_image"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

const ProjectColumns = `id::text,name,source_type,source,branch,build_method,dockerfile,build_context,port,coalesce(domain,''),health_path,auto_deploy,memory_mb,cpu,active_container,active_image,status,created_at,service_type`

type Scanner interface{ Scan(...any) error }

func ScanProject(row Scanner) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Name, &p.SourceType, &p.Source, &p.Branch, &p.BuildMethod, &p.Dockerfile, &p.BuildContext, &p.Port, &p.Domain, &p.HealthPath, &p.AutoDeploy, &p.MemoryMB, &p.CPU, &p.ActiveContainer, &p.ActiveImage, &p.Status, &p.CreatedAt, &p.ServiceType)
	return p, err
}
func (s *Store) Project(ctx context.Context, id string) (Project, error) {
	return ScanProject(s.DB.QueryRow(ctx, "SELECT "+ProjectColumns+" FROM projects WHERE id=$1", id))
}

type Deployment struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	ProjectName string     `json:"project_name"`
	Status      string     `json:"status"`
	Image       string     `json:"image"`
	CommitSHA   string     `json:"commit_sha"`
	Error       string     `json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}
