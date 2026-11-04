use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

use crate::config::Config;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCall {
    pub id: String,
    #[serde(rename = "type")]
    pub kind: String,
    pub function: FunctionCall,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionCall {
    pub name: String,
    pub arguments: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub role: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub content: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Vec<ToolCall>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_call_id: Option<String>,
}

impl Message {
    pub fn system(content: String) -> Self {
        Self {
            role: "system".into(),
            content: Some(content),
            tool_calls: None,
            tool_call_id: None,
        }
    }

    pub fn user(content: String) -> Self {
        Self {
            role: "user".into(),
            content: Some(content),
            tool_calls: None,
            tool_call_id: None,
        }
    }

    pub fn assistant(content: Option<String>, tool_calls: Vec<ToolCall>) -> Self {
        Self {
            role: "assistant".into(),
            content,
            tool_calls: Some(tool_calls),
            tool_call_id: None,
        }
    }

    pub fn tool(tool_call_id: String, content: String) -> Self {
        Self {
            role: "tool".into(),
            content: Some(content),
            tool_calls: None,
            tool_call_id: Some(tool_call_id),
        }
    }
}

#[derive(Debug)]
pub struct Completion {
    pub content: Option<String>,
    pub tool_calls: Vec<ToolCall>,
}

/// A minimal OpenAI-compatible chat client. It deliberately supports only the
/// surface the agent needs: one completion per call, with function tools.
pub struct LlmClient {
    http: reqwest::Client,
    base_url: String,
    api_key: String,
    model: String,
    max_tokens: u32,
    temperature: f32,
}

impl LlmClient {
    pub fn new(config: &Config) -> Result<Self> {
        let http = reqwest::Client::builder()
            .timeout(config.request_timeout)
            .build()
            .context("build LLM HTTP client")?;
        Ok(Self {
            http,
            base_url: config.base_url.clone(),
            api_key: config.api_key.clone(),
            model: config.model.clone(),
            max_tokens: config.max_tokens,
            temperature: config.temperature,
        })
    }

    pub async fn complete(
        &self,
        messages: &[Message],
        tools: &[serde_json::Value],
    ) -> Result<Completion> {
        #[derive(Serialize)]
        struct Request<'a> {
            model: &'a str,
            messages: &'a [Message],
            tools: &'a [serde_json::Value],
            tool_choice: &'a str,
            max_tokens: u32,
            temperature: f32,
        }

        #[derive(Deserialize)]
        struct Response {
            choices: Vec<Choice>,
        }

        #[derive(Deserialize)]
        struct Choice {
            message: Message,
        }

        let url = format!("{}/chat/completions", self.base_url.trim_end_matches('/'));
        let request = Request {
            model: &self.model,
            messages,
            tools,
            tool_choice: "auto",
            max_tokens: self.max_tokens,
            temperature: self.temperature,
        };
        let mut attempt = 0u32;
        let payload: Response = loop {
            attempt += 1;
            let response = match self
                .http
                .post(&url)
                .bearer_auth(&self.api_key)
                .json(&request)
                .send()
                .await
            {
                Ok(response) => response,
                Err(error) => {
                    if attempt < MAX_ATTEMPTS {
                        sleep_backoff(attempt).await;
                        continue;
                    }
                    return Err(error).context("LLM request failed");
                }
            };
            let status = response.status();
            if status.is_success() {
                break response.json().await.context("decode LLM response")?;
            }
            let body = response.text().await.unwrap_or_default();
            let retryable = status.as_u16() == 429 || status.is_server_error();
            if retryable && attempt < MAX_ATTEMPTS {
                sleep_backoff(attempt).await;
                continue;
            }
            anyhow::bail!("LLM returned {status}: {}", truncate(&body, 2000));
        };
        let message = payload
            .choices
            .into_iter()
            .next()
            .context("LLM returned no choices")?
            .message;
        Ok(Completion {
            content: message.content,
            tool_calls: message.tool_calls.unwrap_or_default(),
        })
    }
}

fn truncate(value: &str, max: usize) -> String {
    if value.chars().count() <= max {
        value.to_string()
    } else {
        let kept: String = value.chars().take(max).collect();
        format!("{kept}… (truncated)")
    }
}

const MAX_ATTEMPTS: u32 = 3;

/// Retry only the failures a second attempt can plausibly fix: network errors,
/// 429, and 5xx. A 4xx validation error is returned immediately.
async fn sleep_backoff(attempt: u32) {
    let millis = 500u64.saturating_mul(1u64 << (attempt - 1).min(4));
    tokio::time::sleep(std::time::Duration::from_millis(millis)).await;
}
