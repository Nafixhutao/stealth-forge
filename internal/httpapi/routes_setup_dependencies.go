package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/setupconfig"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
	"github.com/Stealth-deplover/stealth/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func (s *Server) testSetupDatabase(w http.ResponseWriter, r *http.Request) {
	var request setupTestRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	state, err := s.setupState.Load(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	credentials := state.SetupCredentials()
	databaseURL := strings.TrimSpace(request.URL)
	if databaseURL == "" {
		databaseURL = credentials.DatabaseURL
		if databaseURL == "" {
			databaseURL = s.config.DatabaseURL
		}
	}
	// Validate against the saved draft before opening any connection so a
	// caller cannot use this endpoint to probe arbitrary database hosts.
	if setupstate.InstallationLocked(state) {
		writeError(w, http.StatusConflict, "setup_state_conflict", "installation is already in progress or complete")
		return
	}
	if state.Draft.DatabaseMode != "external" || credentials.DatabaseURL != databaseURL {
		writeError(w, http.StatusConflict, "setup_state_conflict", "test the saved external PostgreSQL URL before installing")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := pingDatabase(ctx, databaseURL); err != nil {
		writeError(w, http.StatusBadGateway, "database_connection_failed", "PostgreSQL connection test failed")
		return
	}
	if _, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return errors.New("installation is already in progress or complete")
		}
		if state.Draft.DatabaseMode != "external" || state.SetupCredentials().DatabaseURL != databaseURL {
			return errors.New("test the saved external PostgreSQL URL before installing")
		}
		state.Draft.DatabaseTested = true
		return nil
	}); err != nil {
		writeError(w, http.StatusConflict, "setup_state_conflict", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) testSetupRedis(w http.ResponseWriter, r *http.Request) {
	var request setupTestRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	state, err := s.setupState.Load(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	credentials := state.SetupCredentials()
	redisURL := strings.TrimSpace(request.URL)
	if redisURL == "" {
		redisURL = credentials.RedisURL
		if redisURL == "" {
			redisURL = s.config.RedisURL
		}
	}
	// Validate against the saved draft before opening any connection so a
	// caller cannot use this endpoint to probe arbitrary Redis hosts.
	if setupstate.InstallationLocked(state) {
		writeError(w, http.StatusConflict, "setup_state_conflict", "installation is already in progress or complete")
		return
	}
	if state.Draft.RedisMode != "external" || credentials.RedisURL != redisURL {
		writeError(w, http.StatusConflict, "setup_state_conflict", "test the saved external Redis URL before installing")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := pingRedis(ctx, redisURL); err != nil {
		writeError(w, http.StatusBadGateway, "redis_connection_failed", "Redis connection test failed")
		return
	}
	if _, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return errors.New("installation is already in progress or complete")
		}
		if state.Draft.RedisMode != "external" || state.SetupCredentials().RedisURL != redisURL {
			return errors.New("test the saved external Redis URL before installing")
		}
		state.Draft.RedisTested = true
		return nil
	}); err != nil {
		writeError(w, http.StatusConflict, "setup_state_conflict", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) testSetupStorage(w http.ResponseWriter, r *http.Request) {
	var request setupTestRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	state, err := s.setupState.Load(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	// Validate against the saved draft before any network or filesystem test
	// so a caller cannot use this endpoint to probe arbitrary storage hosts.
	if err := validateSetupStorageTest(state, request); err != nil {
		writeError(w, http.StatusConflict, "setup_state_conflict", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.pingSetupStorage(ctx, state, request); err != nil {
		writeError(w, http.StatusBadGateway, "storage_connection_failed", "storage connection test failed")
		return
	}
	if _, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if err := validateSetupStorageTest(*state, request); err != nil {
			return err
		}
		state.Draft.StorageTested = true
		return nil
	}); err != nil {
		writeError(w, http.StatusConflict, "setup_state_conflict", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// validateSetupStorageTest mirrors the checks the state update repeats so the
// connection test only runs against settings that already match the saved
// draft.
func validateSetupStorageTest(state setupstate.State, request setupTestRequest) error {
	if setupstate.InstallationLocked(state) {
		return errors.New("installation is already in progress or complete")
	}
	if state.Draft.StorageMode != "s3" {
		return nil
	}
	credentials := state.SetupCredentials()
	endpoint := valueOr(request.Endpoint, state.Draft.StorageS3Endpoint)
	region := valueOr(request.Region, state.Draft.StorageS3Region)
	bucket := valueOr(request.Bucket, state.Draft.StorageS3Bucket)
	accessKey := valueOr(request.AccessKey, credentials.StorageS3AccessKey)
	secretKey := valueOr(request.SecretKey, credentials.StorageS3SecretKey)
	useSSL := state.Draft.StorageS3UseSSL
	if request.UseSSL != nil {
		useSSL = *request.UseSSL
	}
	pathStyle := state.Draft.StorageS3PathStyle
	if request.PathStyle != nil {
		pathStyle = *request.PathStyle
	}
	if endpoint != state.Draft.StorageS3Endpoint || region != state.Draft.StorageS3Region || bucket != state.Draft.StorageS3Bucket || accessKey != credentials.StorageS3AccessKey || secretKey != credentials.StorageS3SecretKey || useSSL != state.Draft.StorageS3UseSSL || pathStyle != state.Draft.StorageS3PathStyle {
		return errors.New("test the saved S3-compatible storage settings before installing")
	}
	return nil
}
func (s *Server) pingSetupStorage(ctx context.Context, state setupstate.State, request setupTestRequest) error {
	if state.Draft.StorageMode != "s3" {
		if err := os.MkdirAll(s.config.StorageRoot, 0o700); err != nil {
			return err
		}
		file, err := os.CreateTemp(s.config.StorageRoot, ".setup-storage-test-*")
		if err != nil {
			return err
		}
		name := file.Name()
		defer os.Remove(name)
		if _, err := file.WriteString("stealth"); err != nil {
			_ = file.Close()
			return err
		}
		return file.Close()
	}
	credentials := state.SetupCredentials()
	endpoint := valueOr(request.Endpoint, state.Draft.StorageS3Endpoint)
	region := valueOr(request.Region, state.Draft.StorageS3Region)
	bucket := valueOr(request.Bucket, state.Draft.StorageS3Bucket)
	accessKey := valueOr(request.AccessKey, credentials.StorageS3AccessKey)
	secretKey := valueOr(request.SecretKey, credentials.StorageS3SecretKey)
	useSSL := state.Draft.StorageS3UseSSL
	if request.UseSSL != nil {
		useSSL = *request.UseSSL
	}
	pathStyle := state.Draft.StorageS3PathStyle
	if request.PathStyle != nil {
		pathStyle = *request.PathStyle
	}
	store, err := storage.NewS3(storage.S3Options{Endpoint: endpoint, Region: region, Bucket: bucket, AccessKey: accessKey, SecretKey: secretKey, UseSSL: useSSL, ForcePathStyle: pathStyle, Prefix: state.Draft.StorageS3Prefix, StagingRoot: filepath.Join(s.config.StorageRoot, "setup-staging")}, s.config.StorageMaxFileSize)
	if err != nil {
		return err
	}
	return store.Ping(ctx)
}
func pingDatabase(ctx context.Context, raw string) error {
	if !setupconfig.ValidDatabaseURL(raw) {
		return errors.New("invalid PostgreSQL URL")
	}
	poolConfig, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return err
	}
	poolConfig.MinConns = 0
	poolConfig.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return err
	}
	defer pool.Close()
	return pool.Ping(ctx)
}

func pingRedis(ctx context.Context, raw string) error {
	if !setupconfig.ValidRedisURL(raw) {
		return errors.New("invalid Redis URL")
	}
	options, err := redis.ParseURL(raw)
	if err != nil {
		return err
	}
	client := redis.NewClient(options)
	defer client.Close()
	return client.Ping(ctx).Err()
}
