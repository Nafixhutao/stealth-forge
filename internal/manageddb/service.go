// Package manageddb provisions the free-tier hosted MySQL databases. The
// control plane owns metadata and credentials; MySQL only holds the customer's
// database and user.
package manageddb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/dbsecret"
	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

const (
	FreeConnectionLimit   = 10
	FreeStorageLimitBytes = 256 << 20
	defaultHost           = "tenant-mysql"
	defaultPort           = 3306
	defaultPHPMyAdmin     = "/db/"
)

var (
	ErrInvalidName = errors.New("database name must be between 2 and 120 characters")
	ErrUnavailable = errors.New("managed databases are not configured")
)

// Options carries the deployment-specific values for the shared tenant
// cluster. An empty AdminDSN disables the feature.
type Options struct {
	AdminDSN      string
	Host          string
	Port          int
	PHPMyAdminURL string
	SecretKey     []byte
	Logger        *slog.Logger
}

type Service struct {
	repo   *repository.Repository
	admin  *sql.DB
	cipher *dbsecret.Cipher
	host   string
	port   int
	phpURL string
	logger *slog.Logger
}

func New(repo *repository.Repository, options Options) (*Service, error) {
	if repo == nil {
		return nil, ErrUnavailable
	}
	if strings.TrimSpace(options.AdminDSN) == "" || len(options.SecretKey) != dbsecret.KeySize {
		return nil, ErrUnavailable
	}
	parsed, err := mysql.ParseDSN(options.AdminDSN)
	if err != nil {
		return nil, fmt.Errorf("parse tenant MySQL DSN: %w", err)
	}
	parsed.ParseTime = true
	parsed.Timeout = 5 * time.Second
	parsed.ReadTimeout = 15 * time.Second
	parsed.WriteTimeout = 15 * time.Second
	parsed.MultiStatements = false
	admin, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open tenant MySQL: %w", err)
	}
	admin.SetMaxOpenConns(4)
	admin.SetMaxIdleConns(2)
	admin.SetConnMaxLifetime(5 * time.Minute)
	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := admin.PingContext(pingCtx); err != nil {
		admin.Close()
		return nil, fmt.Errorf("connect tenant MySQL: %w", err)
	}
	cipher, err := dbsecret.New(options.SecretKey)
	if err != nil {
		admin.Close()
		return nil, err
	}
	host := strings.TrimSpace(options.Host)
	if host == "" {
		host = defaultHost
	}
	port := options.Port
	if port <= 0 || port > 65535 {
		port = defaultPort
	}
	phpURL := strings.TrimSpace(options.PHPMyAdminURL)
	if phpURL == "" {
		phpURL = defaultPHPMyAdmin
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:   repo,
		admin:  admin,
		cipher: cipher,
		host:   host,
		port:   port,
		phpURL: phpURL,
		logger: logger,
	}, nil
}

func (s *Service) Close() error {
	if s == nil || s.admin == nil {
		return nil
	}
	return s.admin.Close()
}

func ValidateName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if len([]rune(name)) < 2 || len([]rune(name)) > 120 {
		return "", ErrInvalidName
	}
	return name, nil
}

func (s *Service) List(
	ctx context.Context,
	projectID uuid.UUID,
) ([]domain.ManagedDatabase, error) {
	records, err := s.repo.ListManagedDatabases(ctx, projectID)
	if err != nil {
		return nil, err
	}
	items := make([]domain.ManagedDatabase, 0, len(records))
	for _, record := range records {
		items = append(items, s.enrich(ctx, s.view(record)))
	}
	return items, nil
}

func (s *Service) Create(
	ctx context.Context,
	projectID,
	actorAccountID uuid.UUID,
	rawName string,
) (domain.ManagedDatabase, error) {
	name, err := ValidateName(rawName)
	if err != nil {
		return domain.ManagedDatabase{}, err
	}
	id := uuid.Must(uuid.NewV7())
	databaseName, username := generatedNames(id)
	password, err := randomPassword()
	if err != nil {
		return domain.ManagedDatabase{}, err
	}
	ciphertext, err := s.cipher.Encrypt(projectID, id, []byte(password))
	if err != nil {
		return domain.ManagedDatabase{}, err
	}
	record, err := s.repo.CreateManagedDatabase(ctx, repository.ManagedDatabaseRecord{
		ID:                 id,
		ProjectID:          projectID,
		Name:               name,
		DatabaseName:       databaseName,
		Username:           username,
		PasswordCiphertext: ciphertext,
		Status:             "provisioning",
		ConnectionLimit:    FreeConnectionLimit,
	})
	if err != nil {
		return domain.ManagedDatabase{}, err
	}
	if err := s.provision(ctx, record.DatabaseName, record.Username, password, record.ConnectionLimit); err != nil {
		s.cleanupQuietly(record.DatabaseName, record.Username)
		if deleteErr := s.repo.DeleteManagedDatabase(ctx, projectID, record.ID); deleteErr != nil {
			s.logger.Error("managed database metadata cleanup failed", "database_id", record.ID, "error", deleteErr)
		}
		return domain.ManagedDatabase{}, err
	}
	ready, err := s.repo.UpdateManagedDatabaseStatus(ctx, projectID, record.ID, "ready")
	if err != nil {
		s.cleanupQuietly(record.DatabaseName, record.Username)
		if deleteErr := s.repo.DeleteManagedDatabase(ctx, projectID, record.ID); deleteErr != nil {
			s.logger.Error("managed database metadata cleanup failed", "database_id", record.ID, "error", deleteErr)
		}
		return domain.ManagedDatabase{}, err
	}
	s.audit(ctx, projectID, actorAccountID, record.ID, "managed_database.create", map[string]any{
		"database_name": record.DatabaseName,
		"username":      record.Username,
	})
	return s.enrich(ctx, s.view(ready)), nil
}

func (s *Service) Delete(
	ctx context.Context,
	projectID,
	actorAccountID,
	databaseID uuid.UUID,
) error {
	record, err := s.repo.ManagedDatabaseByID(ctx, projectID, databaseID)
	if err != nil {
		return err
	}
	if err := s.drop(ctx, record.DatabaseName, record.Username); err != nil {
		return err
	}
	if err := s.repo.DeleteManagedDatabase(ctx, projectID, databaseID); err != nil {
		return err
	}
	s.audit(ctx, projectID, actorAccountID, databaseID, "managed_database.delete", map[string]any{
		"database_name": record.DatabaseName,
		"username":      record.Username,
	})
	return nil
}

func (s *Service) RevealPassword(
	ctx context.Context,
	projectID,
	actorAccountID,
	databaseID uuid.UUID,
) (string, error) {
	record, err := s.repo.ManagedDatabaseByID(ctx, projectID, databaseID)
	if err != nil {
		return "", err
	}
	plaintext, err := s.cipher.Decrypt(projectID, databaseID, record.PasswordCiphertext)
	if err != nil {
		return "", err
	}
	s.audit(ctx, projectID, actorAccountID, databaseID, "managed_database.password.reveal", map[string]any{
		"username": record.Username,
	})
	return string(plaintext), nil
}

func (s *Service) RotatePassword(
	ctx context.Context,
	projectID,
	actorAccountID,
	databaseID uuid.UUID,
) (string, error) {
	record, err := s.repo.ManagedDatabaseByID(ctx, projectID, databaseID)
	if err != nil {
		return "", err
	}
	password, err := randomPassword()
	if err != nil {
		return "", err
	}
	if err := s.alterPassword(ctx, record.Username, password); err != nil {
		return "", err
	}
	ciphertext, err := s.cipher.Encrypt(projectID, databaseID, []byte(password))
	if err != nil {
		return "", err
	}
	if _, err := s.repo.UpdateManagedDatabasePassword(ctx, projectID, databaseID, ciphertext); err != nil {
		return "", err
	}
	s.audit(ctx, projectID, actorAccountID, databaseID, "managed_database.password.rotate", map[string]any{
		"username": record.Username,
	})
	return password, nil
}

func (s *Service) provision(ctx context.Context, databaseName, username, password string, connectionLimit int) error {
	if !safeMySQLString(password) {
		return errors.New("generated MySQL password contains unsupported characters")
	}
	statements := []string{
		"CREATE DATABASE " + quoteIdentifier(databaseName) + " CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci",
		"CREATE USER " + quoteString(username) + "@'%' IDENTIFIED BY " + quoteString(password),
		fmt.Sprintf(
			"ALTER USER %s@'%%' WITH MAX_USER_CONNECTIONS %d",
			quoteString(username),
			connectionLimit,
		),
		"GRANT ALL PRIVILEGES ON " + quoteIdentifier(databaseName) + ".* TO " + quoteString(username) + "@'%'",
	}
	for _, statement := range statements {
		if _, err := s.admin.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("provision managed database: %w", err)
		}
	}
	return nil
}

func (s *Service) drop(ctx context.Context, databaseName, username string) error {
	if _, err := s.admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quoteIdentifier(databaseName)); err != nil {
		return fmt.Errorf("drop managed database: %w", err)
	}
	if _, err := s.admin.ExecContext(ctx, "DROP USER IF EXISTS "+quoteString(username)+"@'%'"); err != nil {
		return fmt.Errorf("drop managed database user: %w", err)
	}
	return nil
}

func (s *Service) cleanupQuietly(databaseName, username string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := s.drop(cleanupCtx, databaseName, username); err != nil {
		s.logger.Error("managed database MySQL cleanup failed", "database", databaseName, "error", err)
	}
}

func (s *Service) alterPassword(ctx context.Context, username, password string) error {
	if !safeMySQLString(password) {
		return errors.New("generated MySQL password contains unsupported characters")
	}
	_, err := s.admin.ExecContext(
		ctx,
		"ALTER USER "+quoteString(username)+"@'%' IDENTIFIED BY "+quoteString(password),
	)
	if err != nil {
		return fmt.Errorf("rotate managed database password: %w", err)
	}
	return nil
}

func (s *Service) audit(
	ctx context.Context,
	projectID,
	actorAccountID,
	databaseID uuid.UUID,
	action string,
	metadata map[string]any,
) {
	if err := s.repo.AuditManagedDatabase(ctx, projectID, actorAccountID, databaseID, action, metadata); err != nil {
		s.logger.Error("managed database audit write failed", "database_id", databaseID, "action", action, "error", err)
	}
}

func (s *Service) view(record repository.ManagedDatabaseRecord) domain.ManagedDatabase {
	return domain.ManagedDatabase{
		ID:                record.ID.String(),
		ProjectID:         record.ProjectID.String(),
		Name:              record.Name,
		Status:            record.Status,
		Host:              s.host,
		Port:              s.port,
		DatabaseName:      record.DatabaseName,
		Username:          record.Username,
		PHPMyAdminURL:     s.phpURL,
		ConnectionLimit:   record.ConnectionLimit,
		StorageLimitBytes: FreeStorageLimitBytes,
		WriteLocked:       record.WriteLocked,
		CreatedAt:         record.CreatedAt,
		UpdatedAt:         record.UpdatedAt,
	}
}

// enrich adds live MySQL usage. A usage query failure must not hide the
// database itself, so the metadata remains visible with zeroed counters.
func (s *Service) enrich(ctx context.Context, item domain.ManagedDatabase) domain.ManagedDatabase {
	sizeBytes, connections, err := s.usage(ctx, item.DatabaseName)
	if err != nil {
		s.logger.Error("managed database usage query failed", "database", item.DatabaseName, "error", err)
		return item
	}
	item.SizeBytes = sizeBytes
	item.ConnectionCount = connections
	return item
}

func (s *Service) usage(ctx context.Context, databaseName string) (int64, int, error) {
	var sizeBytes int64
	var connections int
	err := s.admin.QueryRowContext(ctx, `
		SELECT
		  COALESCE((SELECT SUM(data_length + index_length) FROM information_schema.tables WHERE table_schema = ?), 0),
		  (SELECT COUNT(*) FROM information_schema.processlist WHERE db = ?)
	`, databaseName, databaseName).Scan(&sizeBytes, &connections)
	if err != nil {
		return 0, 0, err
	}
	return sizeBytes, connections, nil
}

// AdminSummary is the tenant-cluster view the admin Overview renders. It is
// counts only — no credentials and no per-database values — so it stays fast
// enough for the overview's short health budget.
type AdminSummary struct {
	Reachable        bool
	DatabaseCount    int
	WriteLockedCount int
}

// AdminSummary reports cluster reachability plus the ready database count and
// how many of them are write-locked. A metadata read failure never hides that
// the cluster itself is reachable.
func (s *Service) AdminSummary(ctx context.Context) AdminSummary {
	summary := AdminSummary{}
	pingContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.admin.PingContext(pingContext); err != nil {
		return summary
	}
	summary.Reachable = true
	records, err := s.repo.ListAllReadyManagedDatabases(ctx)
	if err != nil {
		s.logger.Error("managed database admin summary failed", "error", err)
		return summary
	}
	summary.DatabaseCount = len(records)
	for _, record := range records {
		if record.WriteLocked {
			summary.WriteLockedCount++
		}
	}
	return summary
}

// ListBackups returns the completed snapshots for one database plus its most
// recent restore request, so the Console can render both the history and the
// progress of any in-flight restore.
func (s *Service) ListBackups(
	ctx context.Context,
	projectID,
	databaseID uuid.UUID,
) ([]domain.ManagedDatabaseBackup, domain.ManagedDatabaseRestore, bool, error) {
	records, err := s.repo.ListManagedDatabaseBackups(ctx, projectID, databaseID, 30)
	if err != nil {
		return nil, domain.ManagedDatabaseRestore{}, false, err
	}
	items := make([]domain.ManagedDatabaseBackup, 0, len(records))
	for _, record := range records {
		items = append(items, domain.ManagedDatabaseBackup{
			ID:         record.ID.String(),
			DatabaseID: record.DatabaseID.String(),
			BackupPath: record.BackupPath,
			SizeBytes:  record.SizeBytes,
			Source:     record.Source,
			CreatedAt:  record.CreatedAt,
		})
	}
	restore, hasRestore := domain.ManagedDatabaseRestore{}, false
	latest, err := s.repo.LatestManagedDatabaseRestore(ctx, projectID, databaseID)
	if err == nil {
		hasRestore = true
		restore = domain.ManagedDatabaseRestore{
			ID:         latest.ID.String(),
			DatabaseID: latest.DatabaseID.String(),
			BackupPath: latest.BackupPath,
			Status:     latest.Status,
			Error:      latest.Error,
			CreatedAt:  latest.CreatedAt,
			UpdatedAt:  latest.UpdatedAt,
		}
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, domain.ManagedDatabaseRestore{}, false, err
	}
	return items, restore, hasRestore, nil
}

// RequestRestore queues a restore of one backup. The worker claims it within
// a minute; until then its status stays "queued".
func (s *Service) RequestRestore(
	ctx context.Context,
	projectID,
	actorAccountID,
	databaseID,
	backupID uuid.UUID,
) (domain.ManagedDatabaseRestore, error) {
	// Verify the backup belongs to this database before queuing, so a project
	// cannot restore another project's snapshot.
	backups, err := s.repo.ListManagedDatabaseBackups(ctx, projectID, databaseID, 100)
	if err != nil {
		return domain.ManagedDatabaseRestore{}, err
	}
	var chosen *repository.ManagedDatabaseBackupRecord
	for i := range backups {
		if backups[i].ID == backupID {
			chosen = &backups[i]
			break
		}
	}
	if chosen == nil {
		return domain.ManagedDatabaseRestore{}, repository.ErrNotFound
	}
	restoreID, err := uuid.NewV7()
	if err != nil {
		return domain.ManagedDatabaseRestore{}, err
	}
	created, err := s.repo.CreateManagedDatabaseRestore(ctx, repository.ManagedDatabaseRestoreRecord{
		ID:          restoreID,
		DatabaseID:  databaseID,
		ProjectID:   projectID,
		BackupPath:  chosen.BackupPath,
		RequestedBy: actorAccountID,
	})
	if err != nil {
		return domain.ManagedDatabaseRestore{}, err
	}
	s.audit(ctx, projectID, actorAccountID, databaseID, "managed_database.restore.request", map[string]any{
		"backup_path": chosen.BackupPath,
		"restore_id":  restoreID.String(),
	})
	return domain.ManagedDatabaseRestore{
		ID:         created.ID.String(),
		DatabaseID: created.DatabaseID.String(),
		BackupPath: created.BackupPath,
		Status:     created.Status,
		CreatedAt:  created.CreatedAt,
		UpdatedAt:  created.UpdatedAt,
	}, nil
}

func randomPassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate managed database password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func quoteIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

// Generated credentials use the base64url alphabet, so this check is a guard
// against a future generator introducing a character that would need MySQL
// string escaping.
func safeMySQLString(value string) bool {
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return value != ""
}

func quoteString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// generatedNames stays inside MySQL's limits: databases allow 64 characters
// and users 32, so the shared 24-hex suffix fits both and is derived from the
// row id that will own the database.
func generatedNames(id uuid.UUID) (databaseName, username string) {
	suffix := strings.ReplaceAll(id.String(), "-", "")[:24]
	return "mdb_" + suffix, "u_" + suffix
}
