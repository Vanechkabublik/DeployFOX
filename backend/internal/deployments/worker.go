package deployments

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"deployfox/internal/docker"
	"deployfox/internal/github"
	"deployfox/internal/platform"
	"deployfox/internal/security"
	"github.com/jackc/pgx/v5"
	"os"
)

type Worker struct{ Store *platform.Store }

// A PostgreSQL session lock enforces one heavy operation across worker instances.
// The connection is retained for the full job, including the agent operation.
func (w Worker) Run(ctx context.Context) error {
	lastCleanup := time.Time{}
	for ctx.Err() == nil {
		if time.Since(lastCleanup) > time.Hour {
			cleanup, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, err := w.Store.DB.Exec(cleanup, `DELETE FROM deployment_logs WHERE created_at<now()-interval '14 days'; DELETE FROM sessions WHERE expires_at<now(); DELETE FROM webhook_deliveries WHERE created_at<now()-interval '30 days'; DELETE FROM audit_events WHERE created_at<now()-interval '180 days'`)
			cancel()
			if err != nil {
				slog.Warn("retention cleanup failed", "error", err)
			}
			lastCleanup = time.Now()
		}
		worked, err := w.once(ctx)
		if err != nil {
			slog.Error("worker iteration", "error", err)
		}
		if !worked {
			w.Store.Redis.BLPop(ctx, 3*time.Second, "deployfox:wake").Err()
		}
	}
	return ctx.Err()
}
func (w Worker) once(ctx context.Context) (bool, error) {
	conn, err := w.Store.DB.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(730125)").Scan(&locked); err != nil || !locked {
		return false, err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock(730125)")
	// Interrupted jobs are not blindly replayed: the previous agent may still be switching traffic.
	_, err = conn.Exec(ctx, `UPDATE deployments SET status='FAILED',error='Worker interrupted. Inspect agent/container state before redeploying.',finished_at=now() WHERE status NOT IN ('QUEUED','SUCCESS','FAILED','CANCELLED')`)
	if err != nil {
		return false, err
	}
	var id, pid, image, commit string
	var cancelled bool
	err = conn.QueryRow(ctx, "SELECT id::text,project_id::text,image,commit_sha,cancel_requested FROM deployments WHERE status='QUEUED' ORDER BY created_at LIMIT 1").Scan(&id, &pid, &image, &commit, &cancelled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if cancelled {
		w.finish(ctx, id, "CANCELLED", "")
		return true, nil
	}
	_, err = conn.Exec(ctx, "UPDATE deployments SET status='CLONING',started_at=now(),attempts=attempts+1 WHERE id=$1", id)
	if err != nil {
		return false, err
	}
	p, err := w.Store.Project(ctx, pid)
	if err != nil {
		return true, err
	}
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	secrets, err := w.environment(jobCtx, pid)
	if err != nil {
		w.finish(ctx, id, "FAILED", err.Error())
		return true, nil
	}
	log := func(line string) {
		for _, value := range secrets {
			if value != "" {
				line = strings.ReplaceAll(line, value, "[REDACTED]")
			}
		}
		w.Store.Log(ctx, id, line)
	}
	log("Deployment accepted; provider=" + w.Store.Config.Provider)
	state := func(status string) error {
		_, e := w.Store.DB.Exec(jobCtx, "UPDATE deployments SET status=$2 WHERE id=$1", id, status)
		if e == nil {
			log("→ " + status)
		}
		return e
	}
	provider := docker.Provider{Config: w.Store.Config}
	if image == "" {
		if err = state("BUILDING"); err == nil {
			var result docker.Result
			gitToken := ""
			if p.SourceType == "git" && os.Getenv("GITHUB_APP_ID") != "" {
				gitToken, err = github.TokenFromStore(jobCtx, w.Store)
			}
			if err == nil {
				result, err = retryBuild(jobCtx, provider, docker.Request{Action: "build", Project: p, DeploymentID: id, CommitSHA: commit, GitToken: gitToken}, func() error {
					var requested bool
					if e := w.Store.DB.QueryRow(jobCtx, "SELECT cancel_requested FROM deployments WHERE id=$1", id).Scan(&requested); e != nil {
						return e
					}
					if requested {
						return errBuildCancelled
					}
					_, e := w.Store.DB.Exec(jobCtx, "UPDATE deployments SET attempts=attempts+1 WHERE id=$1", id)
					return e
				}, log)
			}
			if err == nil {
				image = result.Image
				_, err = w.Store.DB.Exec(jobCtx, "UPDATE deployments SET image=$2,commit_sha=$3 WHERE id=$1", id, image, result.Commit)
			}
		}
	}
	if errors.Is(err, errBuildCancelled) {
		log("Cancelled before retry")
		w.finish(ctx, id, "CANCELLED", "")
		return true, nil
	}
	if err == nil {
		err = w.Store.DB.QueryRow(jobCtx, "SELECT cancel_requested FROM deployments WHERE id=$1", id).Scan(&cancelled)
	}
	if cancelled && err == nil {
		w.finish(ctx, id, "CANCELLED", "")
		return true, nil
	}
	if err == nil {
		err = state("DEPLOYING")
	}
	if err == nil {
		var result docker.Result
		if err = state("HEALTHCHECK"); err != nil {
			w.finish(ctx, id, "FAILED", err.Error())
			return true, nil
		}
		// Agent performs candidate startup, healthcheck and route switch as one serialized operation.
		result, err = provider.Call(jobCtx, docker.Request{Action: "deploy", Project: p, DeploymentID: id, Image: image, Environment: secrets})
		if err == nil {
			log(result.Output)
			_, err = w.Store.DB.Exec(jobCtx, `WITH activated AS (UPDATE projects SET active_container=$2,active_image=$3,status='RUNNING' WHERE id=$1 RETURNING id,domain) DELETE FROM project_domains WHERE project_id=$1 AND domain<>coalesce((SELECT domain FROM activated),'')`, pid, result.Container, image)
		}
	}
	if err != nil {
		log("Operation failed: " + err.Error())
		w.finish(ctx, id, "FAILED", err.Error())
		return true, nil
	}
	w.finish(ctx, id, "SUCCESS", "")
	log("Deployment successful")
	return true, nil
}
func (w Worker) finish(ctx context.Context, id, status, message string) {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		slog.Error("cannot begin terminal state", "error", err)
		return
	}
	defer tx.Rollback(ctx)
	var pid string
	if err = tx.QueryRow(ctx, "SELECT id::text FROM projects WHERE id=(SELECT project_id FROM deployments WHERE id=$1) FOR UPDATE", id).Scan(&pid); err != nil {
		slog.Error("cannot lock terminal project", "error", err)
		return
	}
	if _, err = tx.Exec(ctx, "UPDATE deployments SET status=$2,error=$3,finished_at=now() WHERE id=$1", id, status, message); err != nil {
		slog.Error("cannot persist terminal state", "error", err)
		return
	}
	_, err = tx.Exec(ctx, `WITH queued AS (INSERT INTO deployments(id,project_id,commit_sha) SELECT gen_random_uuid(),id,pending_commit FROM projects WHERE id=$1 AND pending_commit<>'' ON CONFLICT DO NOTHING RETURNING project_id) UPDATE projects SET pending_commit='' WHERE id IN (SELECT project_id FROM queued)`, pid)
	if err != nil {
		slog.Error("cannot queue pending push", "error", err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		slog.Error("cannot commit terminal state", "error", err)
	}
}
func (w Worker) environment(ctx context.Context, pid string) (map[string]string, error) {
	rows, err := w.Store.DB.Query(ctx, "SELECT key,encrypted_value FROM environment_variables WHERE project_id=$1", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		result[key], err = security.Decrypt(w.Store.Config.EncryptionKey, value)
		if err != nil {
			return nil, err
		}
	}
	return result, rows.Err()
}
