mod agent;
mod config;
mod llm;
mod sandbox;
mod tools;

use std::io::Read;

#[tokio::main]
async fn main() {
    let mut input = String::new();
    if let Err(error) = std::io::stdin().read_to_string(&mut input) {
        eprintln!("[agent] read stdin: {error:#}");
        std::process::exit(1);
    }
    if input.trim().is_empty() {
        eprintln!("[agent] no job on stdin");
        std::process::exit(1);
    }
    let job: agent::Job = match serde_json::from_str(&input) {
        Ok(job) => job,
        Err(error) => {
            eprintln!("[agent] decode job: {error:#}");
            std::process::exit(1);
        }
    };
    let config = match config::Config::from_env() {
        Ok(config) => config,
        Err(error) => {
            eprintln!("[agent] configuration error: {error:#}");
            print_result(&agent::RunResult {
                output_text: None,
                error_message: Some(format!("agent configuration error: {error:#}")),
                steps: vec![],
                changes: vec![],
                files: vec![],
            });
            return;
        }
    };
    match agent::run(&job, &config).await {
        Ok(result) => print_result(&result),
        Err(error) => {
            eprintln!("[agent] run failed: {error:#}");
            print_result(&agent::RunResult {
                output_text: None,
                error_message: Some(format!("{error:#}")),
                steps: vec![],
                changes: vec![],
                files: vec![],
            });
        }
    }
}

fn print_result(result: &agent::RunResult) {
    match serde_json::to_string(result) {
        Ok(json) => println!("{json}"),
        Err(error) => {
            eprintln!("[agent] encode result: {error:#}");
            std::process::exit(1);
        }
    }
}
