package config

import "testing"

func TestValidateProductionSecretsRejectsPlaceholders(t *testing.T) {
	base := Config{
		DatabaseURL:                 "postgresql://stealth:CHANGE_ME_generate_with_openssl_rand_hex_24@localhost:5432/stealth",
		RedisURL:                    "redis://:CHANGE_ME_generate_with_openssl_rand_hex_24@localhost:6379/0",
		TelemetryClickHousePassword: "CHANGE_ME_generate_with_openssl_rand_hex_32",
		MetricsToken:                "CHANGE_ME_metrics_token",
		GitHubAppClientID:           "CHANGE_ME_github_app_client_id",
		PublicAppURL:                "https://console.example.com",
	}
	if err := base.ValidateProductionSecrets(); err == nil {
		t.Fatal("ValidateProductionSecrets accepted placeholder production secrets")
	}
}

func TestValidateProductionSecretsAcceptsRealValues(t *testing.T) {
	cfg := Config{
		DatabaseURL:                 "postgresql://stealth:9f2c7a1b4e6d8f0a2c4e6b8d@db.internal:5432/stealth",
		RedisURL:                    "rediss://:9f2c7a1b4e6d8f0a2c4e6b8d@redis.internal:6379/0",
		TelemetryClickHousePassword: "3b1f9a7c5e2d4f6a8b0c2e4d",
		MetricsToken:                "5a3c1e9b7d2f4a6c8e0b2d4f6a8c0e2b",
		GitHubAppClientID:           "Iv1.abc123def456",
		PublicAppURL:                "https://console.stealth.internal",
	}
	if err := cfg.ValidateProductionSecrets(); err != nil {
		t.Fatalf("ValidateProductionSecrets rejected real values: %v", err)
	}
}

func TestValidateProductionSecretsSkipsSetupMode(t *testing.T) {
	cfg := Config{
		SetupMode:         true,
		PublicAppURL:      "https://console.example.com",
		GitHubAppClientID: "CHANGE_ME_github_app_client_id",
	}
	if err := cfg.ValidateProductionSecrets(); err != nil {
		t.Fatalf("setup mode must not enforce production placeholders: %v", err)
	}
}
