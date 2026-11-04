use std::env;
use std::time::Duration;

use anyhow::{Context, Result};

/// Process-local configuration. Credentials never leave this process: the
/// runner receives them from its environment and never logs them.
#[derive(Debug, Clone)]
pub struct Config {
    pub base_url: String,
    pub api_key: String,
    pub model: String,
    pub max_steps: usize,
    pub max_tokens: u32,
    pub temperature: f32,
    pub request_timeout: Duration,
    pub sandbox: SandboxConfig,
}

#[derive(Debug, Clone)]
pub struct SandboxConfig {
    pub enabled: bool,
    pub image: String,
    pub memory: String,
    pub cpus: String,
    pub pids: u32,
    pub timeout: Duration,
    pub max_output_bytes: usize,
    pub work_root: String,
    /// Named Docker volume shared with the worker. When set, the sandbox mounts
    /// it instead of bind-mounting a path that only exists inside the worker
    /// container.
    pub volume: Option<String>,
    pub volume_workdir: String,
}

impl Config {
    pub fn from_env() -> Result<Self> {
        let api_key = env::var("AGENT_LLM_API_KEY")
            .or_else(|_| env::var("RINSDEV_API_KEY"))
            .context("AGENT_LLM_API_KEY or RINSDEV_API_KEY must be set")?;
        Ok(Self {
            base_url: env_or("AGENT_LLM_BASE_URL", "https://api.rinsdev.cloud/v1"),
            api_key,
            model: env_or("AGENT_LLM_MODEL", "deepseek-v4.1-flash-1017"),
            max_steps: env_parse("AGENT_MAX_STEPS", 20),
            max_tokens: env_parse("AGENT_MAX_TOKENS", 4096),
            temperature: env_parse("AGENT_TEMPERATURE", 0.2),
            request_timeout: Duration::from_secs(env_parse("AGENT_REQUEST_TIMEOUT_SECS", 120)),
            sandbox: SandboxConfig {
                enabled: env::var("AGENT_SANDBOX_ENABLED")
                    .map(|value| value != "0" && !value.eq_ignore_ascii_case("false"))
                    .unwrap_or(true),
                image: env_or("AGENT_SANDBOX_IMAGE", "stealth-agent-sandbox:latest"),
                memory: env_or("AGENT_SANDBOX_MEMORY", "512m"),
                cpus: env_or("AGENT_SANDBOX_CPUS", "1"),
                pids: env_parse("AGENT_SANDBOX_PIDS", 128),
                timeout: Duration::from_secs(env_parse("AGENT_SANDBOX_TIMEOUT_SECS", 120)),
                max_output_bytes: env_parse("AGENT_MAX_OUTPUT_BYTES", 65536),
                work_root: env_or("AGENT_WORK_ROOT", "/tmp/stealth-agent-runs"),
                volume: env::var("AGENT_SANDBOX_VOLUME")
                    .ok()
                    .filter(|value| !value.trim().is_empty()),
                volume_workdir: env_or("AGENT_SANDBOX_VOLUME_WORKDIR", "/workspace"),
            },
        })
    }
}

fn env_or(key: &str, default: &str) -> String {
    env::var(key).unwrap_or_else(|_| default.to_string())
}

fn env_parse<T: std::str::FromStr>(key: &str, default: T) -> T {
    env::var(key)
        .ok()
        .and_then(|value| value.parse().ok())
        .unwrap_or(default)
}
