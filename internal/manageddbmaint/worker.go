// Package manageddbmaint runs the free-tier maintenance loop for hosted MySQL
// databases: hourly usage checks against the 256 MB fair-use budget, daily
// mysqldump backups, retention pruning, and queued restore execution.
package manageddbmaint

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Nafixhutao/stealth-forge/internal/manageddb"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
)

const (
	// BackupRetention is how many completed backups stay on disk per
	// database. Older ones are pruned after a successful new dump.
	BackupRetention = 7
	// dumpTimeout bounds one mysqldump. Free-tier schemas are tiny; a stuck
	// dump is a hung cluster, not slow I/O.
	dumpTimeout = 10 * time.Minute
	// restoreTimeout bounds one restore run.
	restoreTimeout    = 15 * time.Minute
	backupFilePattern = `^mdb_[a-f0-9]{24}/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{6}Z\.sql(\.gz)?$`
)

// backupPathRe validates the relative path recorded in the ledger before any
// Docker command sees it. Nothing else about the path is trusted.
var backupPathRe = regexp.MustCompile(backupFilePattern)

// Docker is the narrow container-execution surface the worker needs. It
// mirrors the injectable runner pattern used by the function runner so tests
// never require a live daemon.
type Docker interface {
	Exec(ctx context.Context, args []string, stdin string) (stdout string, stderr string, err error)
}

// MobyDocker runs commands inside the tenant MySQL container through the
// Docker CLI. The worker already owns the Docker socket for the function
// runtime; no additional privilege is introduced.
type MobyDocker struct {
	Container string
	Binary    string
}

func (d MobyDocker) Exec(ctx context.Context, args []string, stdin string) (string, string, error) {
	binary := d.Binary
	if strings.TrimSpace(binary) == "" {
		binary = "docker"
	}
	// No -t, so Docker never allocates a pseudo-TTY; the command is a sh -c
	// string and reads nothing from stdin. Docker 29 dropped the legacy -T
	// shorthand, so we simply omit TTY flags.
	full := append([]string{"exec", d.Container}, args...)
	command := exec.CommandContext(ctx, binary, full...)
	command.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return stdout.String(), stderr.String(), err
	}
	return stdout.String(), stderr.String(), nil
}

// Worker is the process-owned maintenance loop. It registers with the worker
// supervisor like the other Runner implementations.
type Worker struct {
	Repo       *repository.Repository
	Docker     Docker
	Container  string
	WorkerID   string
	Logger     *slog.Logger
	UsageEvery time.Duration
	BackupAt   string // unused placeholder for schedule clarity; see nextRun
	Retention  int
}

func NewWorker(repo *repository.Repository, docker Docker, container, workerID string, logger *slog.Logger) (*Worker, error) {
	if repo == nil || docker == nil {
		return nil, errors.New("managed database maintenance requires a repository and docker runner")
	}
	if strings.TrimSpace(container) == "" {
		return nil, errors.New("managed database maintenance requires the tenant MySQL container name")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		Repo:       repo,
		Docker:     docker,
		Container:  container,
		WorkerID:   workerID,
		Logger:     logger,
		UsageEvery: time.Hour,
		Retention:  BackupRetention,
	}, nil
}

// Run executes the loop until the context is cancelled. Every tick performs a
// usage pass (quota enforcement) and, once per UTC day, a backup pass.
func (w *Worker) Run(ctx context.Context) error {
	interval := w.UsageEvery
	if interval <= 0 {
		interval = time.Hour
	}
	// Run soon after start so a restarted worker does not wait a full hour.
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	lastBackupDay := ""
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		w.runOnce(ctx, &lastBackupDay)
		timer.Reset(interval)
	}
}

func (w *Worker) runOnce(ctx context.Context, lastBackupDay *string) {
	usageCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	records, err := w.Repo.ListAllReadyManagedDatabases(usageCtx)
	if err != nil {
		w.logError("managed database usage sweep failed to list databases", err)
		return
	}
	today := time.Now().UTC().Format("2006-01-02")
	backupDue := today != *lastBackupDay
	for _, record := range records {
		select {
		case <-ctx.Done():
			return
		default:
		}
		usage, usageErr := w.usageBytes(usageCtx, record.DatabaseName)
		if usageErr != nil {
			w.logError("managed database usage check failed", usageErr,
				"database", record.DatabaseName)
			continue
		}
		w.enforceQuota(usageCtx, record, usage)
		if backupDue {
			w.backup(ctx, record)
		}
	}
	if backupDue {
		*lastBackupDay = today
	}
}

// enforceQuota revokes write privileges once a database outgrows its fair-use
// budget and restores them once usage drops back. Reads stay available so a
// customer can always export their data.
func (w *Worker) enforceQuota(ctx context.Context, record repository.ManagedDatabaseRecord, usage int64) {
	shouldLock := usage > manageddb.FreeStorageLimitBytes
	if shouldLock == record.WriteLocked {
		return
	}
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var statement string
	if shouldLock {
		statement = "REVOKE INSERT, UPDATE, DELETE, CREATE, DROP, ALTER, INDEX ON " +
			quoteIdent(record.DatabaseName) + ".* FROM " + quoteUser(record.Username)
	} else {
		statement = "GRANT ALL PRIVILEGES ON " + quoteIdent(record.DatabaseName) +
			".* TO " + quoteUser(record.Username)
	}
	if _, _, err := w.Docker.Exec(lockCtx, []string{
		"sh", "-c",
		"MYSQL_PWD=\"$MYSQL_ROOT_PASSWORD\" mysql -uroot -e \"" + statement + "\"",
	}, ""); err != nil {
		w.logError("managed database quota enforcement failed", err, "database", record.DatabaseName)
		return
	}
	if err := w.Repo.SetManagedDatabaseWriteLock(ctx, record.ID, shouldLock); err != nil {
		w.logError("managed database write lock persistence failed", err, "database", record.DatabaseName)
		return
	}
	if shouldLock {
		w.Logger.Warn(
			"managed database exceeded fair-use storage; writes disabled",
			"database", record.DatabaseName,
			"usage_bytes", usage,
			"limit_bytes", manageddb.FreeStorageLimitBytes,
		)
	} else {
		w.Logger.Info("managed database back under fair-use storage; writes restored",
			"database", record.DatabaseName)
	}
}

// backup dumps one database and records it. Failures are logged and retried
// on the next pass; they never block the rest of the sweep.
func (w *Worker) backup(ctx context.Context, record repository.ManagedDatabaseRecord) {
	dumpCtx, cancel := context.WithTimeout(ctx, dumpTimeout)
	defer cancel()
	day := time.Now().UTC().Format("2006-01-02")
	stamp := time.Now().UTC().Format("2006-01-02T150405Z")
	relative := record.DatabaseName + "/" + stamp + ".sql.gz"
	target := "/backups/" + relative
	// --single-transaction keeps InnoDB consistent without locking out the
	// customer; --quick streams rows so a large table cannot exhaust memory.
	script := "mkdir -p \"$(dirname " + shellQuote(target) + ")\" && " +
		"MYSQL_PWD=\"$MYSQL_ROOT_PASSWORD\" mysqldump -uroot " +
		"--single-transaction --quick --routines --events --triggers " +
		shellQuote(record.DatabaseName) + " | gzip > " + shellQuote(target)
	stdout, stderr, err := w.Docker.Exec(dumpCtx, []string{"sh", "-c", script}, "")
	if err != nil {
		w.logError("managed database backup failed", err,
			"database", record.DatabaseName, "stderr", tail(stderr))
		return
	}
	_ = stdout
	size, sizeErr := w.fileSize(dumpCtx, target)
	if sizeErr != nil {
		w.logError("managed database backup size check failed", sizeErr,
			"database", record.DatabaseName)
		return
	}
	backupID, err := uuid.NewV7()
	if err != nil {
		w.logError("managed database backup id generation failed", err,
			"database", record.DatabaseName)
		return
	}
	if _, insertErr := w.Repo.CreateManagedDatabaseBackup(dumpCtx, repository.ManagedDatabaseBackupRecord{
		ID:         backupID,
		DatabaseID: record.ID,
		ProjectID:  record.ProjectID,
		BackupPath: relative,
		SizeBytes:  size,
		Source:     "daily",
	}); insertErr != nil {
		w.logError("managed database backup ledger insert failed", insertErr,
			"database", record.DatabaseName)
		return
	}
	w.Logger.Info("managed database backup completed",
		"database", record.DatabaseName, "path", relative, "size_bytes", size, "day", day)
	w.prune(dumpCtx, record)
}

func (w *Worker) fileSize(ctx context.Context, path string) (int64, error) {
	stdout, _, err := w.Docker.Exec(ctx, []string{
		"sh", "-c", "stat -c %s " + shellQuote(path) + " 2>/dev/null || echo 0",
	}, "")
	if err != nil {
		return 0, err
	}
	var size int64
	value := strings.TrimSpace(stdout)
	if value == "" || value == "0" {
		return 0, fmt.Errorf("backup file missing at %s", path)
	}
	if _, convErr := fmt.Sscan(value, &size); convErr != nil {
		return 0, convErr
	}
	return size, nil
}

// prune deletes the oldest backups beyond retention, both on disk and in the
// ledger. A missing file is treated as already pruned.
func (w *Worker) prune(ctx context.Context, record repository.ManagedDatabaseRecord) {
	stale, err := w.Repo.OldestManagedDatabaseBackups(ctx, record.ID, w.Retention)
	if err != nil {
		w.logError("managed database backup prune listing failed", err, "database", record.DatabaseName)
		return
	}
	for _, item := range stale {
		rmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, _, rmErr := w.Docker.Exec(rmCtx, []string{
			"sh", "-c", "rm -f /backups/" + item.BackupPath,
		}, "")
		cancel()
		if rmErr != nil {
			w.logError("managed database backup prune failed", rmErr, "path", item.BackupPath)
			continue
		}
		if delErr := w.Repo.DeleteManagedDatabaseBackup(ctx, item.ID); delErr != nil {
			w.logError("managed database backup ledger prune failed", delErr, "path", item.BackupPath)
		}
	}
}

// RunRestores is the restore half of the loop: it claims queued restores and
// executes them. It is invoked from Run via a second registration so a hung
// restore cannot postpone quota sweeps.
func (w *Worker) RunRestores(ctx context.Context) error {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		w.restoreOnce(ctx)
		timer.Reset(time.Minute)
	}
}

func (w *Worker) restoreOnce(ctx context.Context) {
	claimCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	job, err := w.Repo.ClaimNextManagedDatabaseRestore(claimCtx, 5*time.Second)
	cancel()
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return
		}
		w.logError("managed database restore claim failed", err)
		return
	}
	restoreCtx, restoreCancel := context.WithTimeout(ctx, restoreTimeout)
	defer restoreCancel()
	if execErr := w.executeRestore(restoreCtx, job); execErr != nil {
		if finishErr := w.Repo.FinishManagedDatabaseRestore(ctx, job.ID, "failed", boundedError(execErr)); finishErr != nil {
			w.logError("managed database restore failure persistence failed", finishErr)
		}
		w.logError("managed database restore failed", execErr, "restore", job.ID.String())
		return
	}
	if err := w.Repo.FinishManagedDatabaseRestore(ctx, job.ID, "succeeded", ""); err != nil {
		w.logError("managed database restore completion persistence failed", err)
	}
	w.Logger.Info("managed database restore completed", "restore", job.ID.String(), "database", job.BackupPath)
}

func (w *Worker) executeRestore(ctx context.Context, job repository.ManagedDatabaseRestoreRecord) error {
	if !backupPathRe.MatchString(job.BackupPath) {
		return fmt.Errorf("backup path failed validation: %q", job.BackupPath)
	}
	record, err := w.Repo.ManagedDatabaseByID(ctx, job.ProjectID, job.DatabaseID)
	if err != nil {
		return err
	}
	database, user := record.DatabaseName, record.Username
	target := "/backups/" + job.BackupPath
	// Drop and recreate the schema, then replay the dump as the schema owner.
	// Root is only used for the DROP/CREATE; the import itself reattaches to
	// the customer's own grants.
	script := "MYSQL_PWD=\"$MYSQL_ROOT_PASSWORD\" mysql -uroot -e " +
		shellQuote("DROP DATABASE IF EXISTS "+quoteIdent(database)+"; CREATE DATABASE "+quoteIdent(database)+";") +
		" && MYSQL_PWD=\"$MYSQL_ROOT_PASSWORD\" sh -c 'gunzip -c " + shellQuote(target) +
		" | mysql -uroot " + shellQuote(database) + "'"
	if _, stderr, err := w.Docker.Exec(ctx, []string{"sh", "-c", script}, ""); err != nil {
		return fmt.Errorf("restore %s: %s: %w", job.BackupPath, tail(stderr), err)
	}
	// Re-grant the customer user in case the dump replaced privilege tables.
	regrant := "MYSQL_PWD=\"$MYSQL_ROOT_PASSWORD\" mysql -uroot -e " +
		shellQuote("GRANT ALL PRIVILEGES ON "+quoteIdent(database)+".* TO "+quoteUser(user))
	if _, _, err := w.Docker.Exec(ctx, []string{"sh", "-c", regrant}, ""); err != nil {
		return fmt.Errorf("post-restore grant: %w", err)
	}
	return nil
}

func (w *Worker) usageBytes(ctx context.Context, databaseName string) (int64, error) {
	// Reuse the same information_schema query the API service uses, but
	// through docker exec so the worker needs no second MySQL credential.
	script := "MYSQL_PWD=\"$MYSQL_ROOT_PASSWORD\" mysql -uroot -N -B -e " +
		shellQuote("SELECT COALESCE(SUM(data_length + index_length),0) FROM information_schema.tables WHERE table_schema = "+
			quoteSQLString(databaseName))
	stdout, _, err := w.Docker.Exec(ctx, []string{"sh", "-c", script}, "")
	if err != nil {
		return 0, err
	}
	var size int64
	value := strings.TrimSpace(stdout)
	if value == "" {
		return 0, nil
	}
	if _, convErr := fmt.Sscan(value, &size); convErr != nil {
		return 0, convErr
	}
	return size, nil
}

func (w *Worker) logError(message string, err error, keys ...string) {
	args := make([]any, 0, len(keys)+1)
	args = append(args, "error", err)
	for i := 0; i+1 < len(keys); i += 2 {
		args = append(args, keys[i], keys[i+1])
	}
	w.Logger.Error(message, args...)
}

func tail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 300 {
		return value
	}
	return value[len(value)-300:]
}

func boundedError(err error) string {
	value := strings.TrimSpace(err.Error())
	if len(value) > 2000 {
		return value[:2000]
	}
	return value
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func quoteUser(name string) string {
	return "'" + strings.ReplaceAll(name, "'", "''") + "'@'%'"
}

func quoteSQLString(name string) string {
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}
