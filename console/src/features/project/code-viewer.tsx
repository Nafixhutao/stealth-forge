"use client";

import { useEffect, useRef } from "react";
import { EditorState } from "@codemirror/state";
import { EditorView, lineNumbers } from "@codemirror/view";
import { html } from "@codemirror/lang-html";
import { css } from "@codemirror/lang-css";
import { javascript } from "@codemirror/lang-javascript";
import { cn } from "@/lib/utils";

/**
 * A read-only source viewer.
 *
 * Published releases are immutable — there is no endpoint that writes a file
 * into one — so this deliberately cannot edit. It is the read half of the
 * workflow: inspect what a release actually contains, then change it by
 * deploying again (upload, or a GitHub build).
 *
 * CodeMirror is loaded per file type rather than importing every language
 * bundle, and the whole component is imported dynamically by its caller so the
 * editor never lands in a page bundle that does not show code.
 */
export type SourceLanguage = "html" | "css" | "javascript" | "text";

/** Pick a language from the file extension, defaulting to plain text. */
export function languageForPath(path: string): SourceLanguage {
  const name = path.toLowerCase();
  if (name.endsWith(".html") || name.endsWith(".htm")) return "html";
  if (name.endsWith(".css")) return "css";
  if (
    name.endsWith(".js") ||
    name.endsWith(".mjs") ||
    name.endsWith(".cjs") ||
    name.endsWith(".json")
  ) {
    return "javascript";
  }
  return "text";
}

const THEME = EditorView.theme({
  "&": {
    backgroundColor: "transparent",
    color: "var(--projects-text)",
    fontSize: "12.5px",
  },
  ".cm-content": {
    fontFamily:
      "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
    padding: "10px 0",
  },
  ".cm-gutters": {
    backgroundColor: "transparent",
    color: "var(--projects-muted)",
    border: "none",
  },
  ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "transparent" },
  "&.cm-focused": { outline: "none" },
  ".cm-selectionBackground, ::selection": {
    backgroundColor: "color-mix(in srgb, var(--user-accent) 24%, transparent) !important",
  },
  ".cm-cursor": { display: "none" },
});

export function CodeViewer({
  value,
  language,
  className,
}: {
  value: string;
  language: SourceLanguage;
  className?: string;
}) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);

  useEffect(() => {
    if (!host.current) return;
    const extensions = [
      lineNumbers(),
      EditorView.lineWrapping,
      EditorState.readOnly.of(true),
      EditorView.editable.of(false),
      THEME,
    ];
    if (language === "html") extensions.push(html());
    if (language === "css") extensions.push(css());
    if (language === "javascript") extensions.push(javascript());

    view.current = new EditorView({
      state: EditorState.create({ doc: value, extensions }),
      parent: host.current,
    });
    return () => {
      view.current?.destroy();
      view.current = null;
    };
  }, [value, language]);

  return (
    <div
      ref={host}
      className={cn("min-h-0 overflow-auto", className)}
      // The text is already selectable inside the editor; this label tells a
      // screen reader what the region is.
      aria-label="Release source"
      role="region"
    />
  );
}
