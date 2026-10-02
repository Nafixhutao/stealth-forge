"use client";

import { useEffect, useRef } from "react";
import { handoffURL } from "./helpers";

export function HandoffSubmission({
  publicURL,
  persistedURL,
  token,
}: {
  publicURL: string;
  persistedURL?: string;
  token: string;
}) {
  const formRef = useRef<HTMLFormElement>(null);
  const action = handoffURL(publicURL, [persistedURL]);

  useEffect(() => {
    if (action) formRef.current?.requestSubmit();
  }, [action, token]);

  if (!action) return null;
  return (
    <form
      ref={formRef}
      method="post"
      action={action}
      className="hidden"
      aria-hidden="true"
    >
      <input type="hidden" name="token" value={token} />
    </form>
  );
}
