"use client";

import { useEffect, useRef } from "react";
import { EditorState } from "@codemirror/state";
import { EditorView, keymap, lineNumbers } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { html } from "@codemirror/lang-html";
import { css } from "@codemirror/lang-css";
import { javascript } from "@codemirror/lang-javascript";
import { cn } from "@/lib/utils";

/**
 * The source editor.
 *
 * It is uncontrolled on purpose: the document is read from `initialValue` once,
 * and typing never round-trips through React state. Re-creating the editor on
 * every keystroke would destroy the cursor and the undo history, so a caller
 * that switches files remounts this component with a `key` instead of feeding a
 * new value into a live editor.
 *
 * Editing is off by default. The site surface turns it on only for a caller who
 * may write, and saving publishes a new release rather than mutating one.
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
    backgroundColor:
      "color-mix(in srgb, var(--user-accent) 24%, transparent) !important",
  },
});

export function CodeViewer({
  initialValue,
  language,
  readOnly = true,
  onChange,
  className,
}: {
  /** The document as it is when the editor mounts. */
  initialValue: string;
  language: SourceLanguage;
  readOnly?: boolean;
  /** Receives the full document after each change, for saving. */
  onChange?: (value: string) => void;
  className?: string;
}) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  // The listener is read from a ref so changing the callback never rebuilds the
  // editor and never loses the caret.
  const onChangeRef = useRef(onChange);
  // Synced in an effect, not during render: a render must stay pure, and the
  // listener only reads this after the editor has mounted anyway.
  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);
  const initialRef = useRef(initialValue);

  useEffect(() => {
    if (!host.current) return;
    const extensions = [
      lineNumbers(),
      EditorView.lineWrapping,
      THEME,
      // Without the command keymap, Mod-A, undo, and redo fall through to the
      // browser: select-all then grabs the page instead of the document.
      history(),
      keymap.of([...defaultKeymap, ...historyKeymap]),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          onChangeRef.current?.(update.state.doc.toString());
        }
      }),
    ];
    if (readOnly) {
      extensions.push(
        EditorState.readOnly.of(true),
        EditorView.editable.of(false),
        EditorView.theme({ ".cm-cursor": { display: "none" } }),
      );
    }
    // Auto-closing tags is off: this editor edits an existing document, and an
    // inserted closing tag duplicates one the author already wrote.
    if (language === "html") extensions.push(html({ autoCloseTags: false }));
    if (language === "css") extensions.push(css());
    if (language === "javascript") extensions.push(javascript());

    view.current = new EditorView({
      state: EditorState.create({ doc: initialRef.current, extensions }),
      parent: host.current,
    });
    return () => {
      view.current?.destroy();
      view.current = null;
    };
  }, [language, readOnly]);

  return (
    <div
      ref={host}
      className={cn("min-h-0 overflow-auto", className)}
      // The text is selectable inside the editor; this label tells a screen
      // reader what the region is.
      aria-label={readOnly ? "Release source" : "Release source, editable"}
      role="region"
    />
  );
}
