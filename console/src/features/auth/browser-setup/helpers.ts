import { ApiError } from "@/api/client";
import { errorMessage } from "@/components/feedback/error-state";

export function safeError(error: unknown) {
  if (error instanceof ApiError) {
    if (error.code === "github_not_configured") {
      return "GitHub App setup is unavailable. Use the manual App option or configure the setup service.";
    }
    if (error.code === "handoff_unavailable") {
      return "The production session handoff is not ready yet. Keep this setup window open and retry.";
    }
    if (error.code === "cloudflare_oauth_inactive") {
      return "Cloudflare OAuth is experimental and inactive. Use a scoped API token instead.";
    }
    if (error.code === "cloudflare_token_rejected") {
      return "Cloudflare rejected the token. Check Account: Cloudflare Tunnel Edit and Account Settings Read; Zone: Zone Read and DNS Edit for the Console/workload zones, plus SSL and Certificates Read for workload edge TLS readiness.";
    }
    // Provisioning failures return a 502 with an actionable message that names
    // the missing token scope. errorMessage() replaces every 5xx message with a
    // generic string, so surface the API message directly for these codes.
    if (
      error.code === "cloudflare_dns_failed" ||
      error.code === "cloudflare_tunnel_failed" ||
      error.code === "cloudflare_unavailable"
    ) {
      return error.message;
    }
  }
  return errorMessage(error);
}

export function submitGitHubManifest(actionURL: string, manifest: string) {
  const target = new URL(actionURL);
  if (
    target.origin !== "https://github.com" ||
    target.pathname !== "/settings/apps/new"
  ) {
    throw new Error("GitHub returned an invalid App Manifest destination.");
  }
  const form = document.createElement("form");
  form.method = "post";
  form.action = target.toString();
  form.hidden = true;
  const field = document.createElement("input");
  field.type = "hidden";
  field.name = "manifest";
  field.value = manifest;
  form.appendChild(field);
  document.body.appendChild(form);
  form.submit();
}

export function handoffURL(publicURL: string) {
  try {
    const target = new URL(publicURL);
    if (
      (target.protocol !== "http:" && target.protocol !== "https:") ||
      target.username ||
      target.password ||
      target.search ||
      target.hash ||
      (target.pathname !== "" && target.pathname !== "/")
    ) {
      return "";
    }
    target.pathname = "/v1/setup/handoff";
    return target.toString();
  } catch {
    return "";
  }
}
