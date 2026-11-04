use anyhow::Result;
use serde::Deserialize;
use serde_json::{json, Value};

use crate::sandbox::Sandbox;

#[derive(Deserialize)]
struct ShellArgs {
    command: String,
}

#[derive(Deserialize)]
struct WriteArgs {
    path: String,
    content: String,
}

#[derive(Deserialize)]
struct ReadArgs {
    path: String,
}

/// The tool surface advertised to the model. Two write paths are enough for a
/// document agent: run a program (which may be python-docx or LibreOffice), or
/// write a text file the program will consume.
pub fn definitions() -> Vec<Value> {
    vec![
        json!({
            "type": "function",
            "function": {
                "name": "shell",
                "description": "Run a shell command inside the sandbox. The working directory is /workspace and there is no network.",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "command": { "type": "string", "description": "The command to run with sh -lc." }
                    },
                    "required": ["command"]
                }
            }
        }),
        json!({
            "type": "function",
            "function": {
                "name": "write_file",
                "description": "Write a UTF-8 text file inside the sandbox. Relative to /workspace.",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "path": { "type": "string", "description": "Relative path inside /workspace." },
                        "content": { "type": "string", "description": "The full file contents." }
                    },
                    "required": ["path", "content"]
                }
            }
        }),
        json!({
            "type": "function",
            "function": {
                "name": "read_file",
                "description": "Read a UTF-8 text file inside the sandbox. Relative to /workspace.",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "path": { "type": "string", "description": "Relative path inside /workspace." }
                    },
                    "required": ["path"]
                }
            }
        }),
    ]
}

pub async fn execute(sandbox: &Sandbox, name: &str, arguments: &str) -> Result<String> {
    match name {
        "shell" => {
            let args: ShellArgs = serde_json::from_str(arguments)?;
            let output = sandbox.exec(&args.command).await?;
            Ok(format!(
                "exit_code: {}\nstdout:\n{}\nstderr:\n{}",
                output.exit_code, output.stdout, output.stderr
            ))
        }
        "write_file" => {
            let args: WriteArgs = serde_json::from_str(arguments)?;
            sandbox.write_file(&args.path, &args.content).await?;
            Ok(format!(
                "wrote {} bytes to {}",
                args.content.len(),
                args.path
            ))
        }
        "read_file" => {
            let args: ReadArgs = serde_json::from_str(arguments)?;
            sandbox.read_file(&args.path).await
        }
        other => Ok(format!("unknown tool: {other}")),
    }
}
