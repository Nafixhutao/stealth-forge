use std::path::{Component, Path, PathBuf};
use std::process::Stdio;
use std::time::Duration;

use anyhow::{bail, Context, Result};
use tokio::process::Command;

use crate::config::{Config, SandboxConfig};

#[derive(Debug, Clone)]
pub struct CommandOutput {
    pub exit_code: i32,
    pub stdout: String,
    pub stderr: String,
}

#[derive(Debug, Clone)]
pub struct FileEntry {
    pub path: String,
    pub size_bytes: u64,
}

/// One sandbox per run. Docker is the production path: no network, a read-only
/// root filesystem, one writable volume, a non-root user, and explicit CPU,
/// memory, and process limits. The local variant exists so tests and
/// development can run without a Docker daemon.
pub enum Sandbox {
    Local(LocalSandbox),
    Docker(DockerSandbox),
}

pub struct LocalSandbox {
    workdir: PathBuf,
    max_output_bytes: usize,
}

pub struct DockerSandbox {
    name: String,
    container_workdir: String,
    workdir: PathBuf,
    config: SandboxConfig,
}

impl Sandbox {
    pub async fn start(config: &Config, run_id: &str) -> Result<Self> {
        // Runs live under the worker's staging volume, in an `agent/` subtree.
        // The directory is deterministic per run so the Go adapter can collect
        // the produced files after this process returns.
        let run_relative = format!("agent/{}", sanitize(run_id));
        let workdir = PathBuf::from(&config.sandbox.work_root).join(&run_relative);
        // A retry starts from a clean directory for this run.
        let _ = tokio::fs::remove_dir_all(&workdir).await;
        tokio::fs::create_dir_all(&workdir)
            .await
            .with_context(|| format!("create agent workdir {}", workdir.display()))?;
        set_world_writable(&workdir)?;

        if !config.sandbox.enabled {
            return Ok(Sandbox::Local(LocalSandbox {
                workdir,
                max_output_bytes: config.sandbox.max_output_bytes,
            }));
        }

        let name = format!("stealth-agent-{}", uuid::Uuid::new_v4().simple());
        let (mount, container_workdir) = match &config.sandbox.volume {
            // Production: the worker and the sandbox share a named volume, so
            // the sandbox sees the same files the worker writes.
            Some(volume) => (
                format!(
                    "type=volume,source={volume},target={}",
                    config.sandbox.volume_workdir
                ),
                format!("{}/{}", config.sandbox.volume_workdir, run_relative),
            ),
            // Development on the host: a bind mount is enough.
            None => (
                format!(
                    "type=bind,source={},target={}",
                    workdir.display(),
                    config.sandbox.volume_workdir
                ),
                config.sandbox.volume_workdir.clone(),
            ),
        };

        let args = vec![
            "run".to_string(),
            "-d".to_string(),
            "--rm".to_string(),
            "--name".to_string(),
            name.clone(),
            "--network=none".to_string(),
            "--read-only".to_string(),
            "--tmpfs".to_string(),
            "/tmp:rw,nosuid,nodev,noexec,size=64m".to_string(),
            "--cap-drop".to_string(),
            "ALL".to_string(),
            "--security-opt".to_string(),
            "no-new-privileges:true".to_string(),
            "--pids-limit".to_string(),
            config.sandbox.pids.to_string(),
            "--memory".to_string(),
            config.sandbox.memory.clone(),
            "--memory-swap".to_string(),
            config.sandbox.memory.clone(),
            "--cpus".to_string(),
            config.sandbox.cpus.clone(),
            "--mount".to_string(),
            mount,
            "--workdir".to_string(),
            container_workdir.clone(),
            "--user".to_string(),
            "65532:65532".to_string(),
            // LibreOffice writes a user profile; /tmp is the one writable
            // tmpfs besides the run volume.
            "--env".to_string(),
            "HOME=/tmp".to_string(),
            config.sandbox.image.clone(),
            "sleep".to_string(),
            "infinity".to_string(),
        ];

        let output = Command::new("docker")
            .args(&args)
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .output()
            .await
            .context("start sandbox container")?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
            let _ = tokio::fs::remove_dir_all(&workdir).await;
            bail!("sandbox container failed to start: {stderr}");
        }

        Ok(Sandbox::Docker(DockerSandbox {
            name,
            container_workdir,
            workdir,
            config: config.sandbox.clone(),
        }))
    }

    pub async fn exec(&self, command: &str) -> Result<CommandOutput> {
        let mut cmd = match self {
            Sandbox::Local(local) => {
                let mut cmd = Command::new("sh");
                cmd.args(["-lc", command]).current_dir(&local.workdir);
                cmd
            }
            Sandbox::Docker(docker) => {
                let mut cmd = Command::new("docker");
                cmd.args([
                    "exec",
                    "-i",
                    "-w",
                    &docker.container_workdir,
                    &docker.name,
                    "sh",
                    "-lc",
                    command,
                ]);
                cmd
            }
        };
        cmd.stdin(Stdio::null()).kill_on_drop(true);
        let output = tokio::time::timeout(self.timeout(), cmd.output())
            .await
            .map_err(|_| anyhow::anyhow!("command timed out"))?
            .context("run sandbox command")?;
        Ok(CommandOutput {
            exit_code: output.status.code().unwrap_or(-1),
            stdout: head_tail(
                &String::from_utf8_lossy(&output.stdout),
                self.max_output_bytes(),
            ),
            stderr: head_tail(
                &String::from_utf8_lossy(&output.stderr),
                self.max_output_bytes(),
            ),
        })
    }

    pub async fn write_file(&self, path: &str, content: &str) -> Result<()> {
        let relative = safe_relative(path)?;
        let target = self.workdir().join(relative);
        if let Some(parent) = target.parent() {
            tokio::fs::create_dir_all(parent).await?;
        }
        tokio::fs::write(&target, content)
            .await
            .with_context(|| format!("write {}", target.display()))?;
        Ok(())
    }

    pub async fn read_file(&self, path: &str) -> Result<String> {
        let relative = safe_relative(path)?;
        tokio::fs::read_to_string(self.workdir().join(relative))
            .await
            .context("read file")
    }

    pub async fn list_files(&self) -> Result<Vec<FileEntry>> {
        let root = self.workdir().to_path_buf();
        tokio::task::spawn_blocking(move || {
            let mut entries = Vec::new();
            walk(&root, &root, &mut entries)?;
            Ok::<_, std::io::Error>(entries)
        })
        .await
        .context("join file walk")?
        .context("walk workdir")
    }

    pub async fn cleanup(&self) {
        if let Sandbox::Docker(docker) = self {
            let _ = Command::new("docker")
                .args(["rm", "-f", &docker.name])
                .stdout(Stdio::null())
                .stderr(Stdio::null())
                .status()
                .await;
        }
        // The run directory is intentionally kept: the worker collects the
        // generated documents from it after the run. Retention is owned by the
        // artifact pipeline, not by the sandbox.
    }

    fn workdir(&self) -> &Path {
        match self {
            Sandbox::Local(local) => &local.workdir,
            Sandbox::Docker(docker) => &docker.workdir,
        }
    }

    fn timeout(&self) -> Duration {
        match self {
            Sandbox::Local(_) => Duration::from_secs(120),
            Sandbox::Docker(docker) => docker.config.timeout,
        }
    }

    fn max_output_bytes(&self) -> usize {
        match self {
            Sandbox::Local(local) => local.max_output_bytes,
            Sandbox::Docker(docker) => docker.config.max_output_bytes,
        }
    }
}

fn safe_relative(path: &str) -> Result<PathBuf> {
    let path = Path::new(path.trim_start_matches("./"));
    if path.as_os_str().is_empty() {
        bail!("path is empty");
    }
    if path.is_absolute() {
        bail!("absolute paths are not allowed");
    }
    for component in path.components() {
        match component {
            Component::Normal(_) | Component::CurDir => {}
            _ => bail!("path must stay inside the workdir"),
        }
    }
    Ok(path.to_path_buf())
}

fn walk(root: &Path, base: &Path, out: &mut Vec<FileEntry>) -> std::io::Result<()> {
    for entry in std::fs::read_dir(root)? {
        let entry = entry?;
        let path = entry.path();
        if path.is_dir() {
            walk(&path, base, out)?;
        } else {
            let relative = path
                .strip_prefix(base)
                .unwrap_or(&path)
                .to_string_lossy()
                .into_owned();
            let size = entry.metadata().map(|meta| meta.len()).unwrap_or(0);
            out.push(FileEntry {
                path: relative,
                size_bytes: size,
            });
        }
    }
    Ok(())
}

#[cfg(unix)]
fn set_world_writable(path: &Path) -> Result<()> {
    use std::os::unix::fs::PermissionsExt;

    let mut permissions = std::fs::metadata(path)?.permissions();
    permissions.set_mode(0o777);
    std::fs::set_permissions(path, permissions)?;
    Ok(())
}

#[cfg(not(unix))]
fn set_world_writable(_path: &Path) -> Result<()> {
    Ok(())
}

fn sanitize(value: &str) -> String {
    value
        .chars()
        .filter(|ch| ch.is_ascii_alphanumeric() || *ch == '-' || *ch == '_')
        .take(64)
        .collect()
}

/// Keep both ends of long command output. The head carries the setup and the
/// tail carries the error, which a tail-only truncation would hide.
fn head_tail(value: &str, max: usize) -> String {
    if value.len() <= max {
        return value.to_string();
    }
    let half = max / 2;
    let head: String = value.chars().take(half).collect();
    let tail: String = value
        .chars()
        .rev()
        .take(half)
        .collect::<Vec<_>>()
        .into_iter()
        .rev()
        .collect();
    let omitted = value.chars().count().saturating_sub(half * 2);
    format!("{head}\n… [{omitted} chars omitted] …\n{tail}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_absolute_and_parent_paths() {
        assert!(safe_relative("/etc/passwd").is_err());
        assert!(safe_relative("../escape").is_err());
        assert!(safe_relative("a/../../b").is_err());
        assert!(safe_relative("").is_err());
    }

    #[test]
    fn accepts_nested_relative_paths() {
        assert_eq!(
            safe_relative("./nested/file.txt").unwrap(),
            PathBuf::from("nested/file.txt")
        );
        assert_eq!(
            safe_relative("letter.docx").unwrap(),
            PathBuf::from("letter.docx")
        );
    }

    #[test]
    fn head_and_tail_are_both_kept() {
        let value = "a".repeat(1000);
        let output = head_tail(&value, 100);
        assert!(output.starts_with("aaaa"));
        assert!(output.ends_with("aaaa"));
        assert!(output.contains("chars omitted"));
        assert!(output.len() < value.len());
    }
}

