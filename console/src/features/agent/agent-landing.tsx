"use client";

import { useRef, useState, type ReactNode } from "react";
import { ChevronDown, ChevronUp, Globe, Search, Users, X } from "lucide-react";
import { cn } from "@/lib/utils";

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--projects-ring)]";

const EXAMPLES = [
  {
    icon: Globe,
    title: "Create a new project",
    description: "Turn an idea into a well-scoped project.",
    prompt: "Help me create a new project.",
  },
  {
    icon: Search,
    title: "Research a topic",
    description: "Research a topic across the issue backlog.",
    prompt: "Research ",
  },
  {
    icon: Users,
    title: "Set up new team",
    description: "Create a team that matches how your organization works.",
    prompt: "Help me set up a new team.",
  },
] as const;

/**
 * The empty-conversation surface: a short introduction, the composer, and a
 * few example prompts, with the greeting pinned to the base.
 *
 * The examples are real controls, not decoration: each one fills the composer
 * with a starting prompt so the reader can edit it before sending. Dismissing
 * them is session-local and does not hide the composer.
 */
export function AgentLanding({
  displayName,
  composer,
  note,
  onPickExample,
}: {
  displayName: string;
  composer: ReactNode;
  note?: ReactNode;
  onPickExample: (prompt: string) => void;
}) {
  const [examplesOpen, setExamplesOpen] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);

  const scrollTo = (top: number) =>
    scrollRef.current?.scrollTo({ top, behavior: "smooth" });

  return (
    <div
      ref={scrollRef}
      className="relative flex min-h-0 flex-1 flex-col overflow-y-auto [overscroll-behavior:contain]"
    >
      <div className="mx-auto flex w-full max-w-[744px] flex-1 flex-col px-4 py-8">
        <div className="flex flex-1 flex-col justify-center gap-7">
          <header className="flex flex-col items-center gap-3 text-center">
            <span className="inline-flex items-center gap-2 rounded-full border border-white/[0.08] bg-white/[0.03] px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.16em] text-[#9A9AA2]">
              <span
                aria-hidden="true"
                className="size-1.5 rounded-full bg-[#5E6AD2]"
              />
              Stealth Agent
            </span>
            <h1 className="m-0 text-[26px] font-semibold leading-[1.15] tracking-[-0.04em] text-[#F4F4F5] [text-wrap:balance] sm:text-[32px]">
              What can I help you with?
            </h1>
            <p className="m-0 max-w-[46ch] text-[13.5px] leading-6 text-[#8B8B90] [text-wrap:pretty]">
              Build a project, research a topic, or draft a document — the
              Agent works across your organization.
            </p>
          </header>

          {composer}
          {note}

          <div className="flex justify-center">
            <button
              type="button"
              disabled
              aria-label="Connect GitHub"
              title="GitHub integration is coming soon"
              className="inline-flex items-center gap-2 rounded-full border border-white/[0.08] bg-white/[0.03] px-3.5 py-1.5 text-[12.5px] font-medium leading-5 text-[#C9C9CF]"
            >
              <GithubMark className="size-[14px]" />
              Connect GitHub
            </button>
          </div>

          {examplesOpen ? (
            <section aria-label="Example prompts" className="flex flex-col gap-3">
              <div className="flex items-center justify-between gap-3">
                <p className="m-0 text-[12px] font-medium uppercase tracking-[0.12em] text-[#6F6F77]">
                  Get started
                </p>
                <button
                  type="button"
                  onClick={() => setExamplesOpen(false)}
                  aria-label="Dismiss examples"
                  className={cn(
                    "inline-flex size-7 shrink-0 items-center justify-center rounded-md text-[#6F6F77] transition-colors hover:bg-white/[0.05] hover:text-[#C9C9CF]",
                    FOCUS,
                  )}
                >
                  <X size={14} strokeWidth={1.9} aria-hidden="true" />
                </button>
              </div>

              <div className="grid gap-3 sm:grid-cols-3">
                {EXAMPLES.map(({ icon: Icon, title, description, prompt }) => (
                  <button
                    key={title}
                    type="button"
                    onClick={() => onPickExample(prompt)}
                    className={cn(
                      "group flex flex-col items-start gap-4 rounded-[14px] border border-white/[0.06] bg-[linear-gradient(180deg,#17171A_0%,#141416_100%)] p-4 text-left transition-all duration-200 hover:-translate-y-0.5 hover:border-white/[0.12] hover:shadow-[0_18px_40px_-22px_rgba(0,0,0,0.9)]",
                      FOCUS,
                    )}
                  >
                    <span className="flex size-9 items-center justify-center rounded-[10px] border border-white/[0.06] bg-white/[0.03] text-[#A9A9B2] transition-colors group-hover:text-[#D6D6DC]">
                      <Icon size={16} strokeWidth={1.8} aria-hidden="true" />
                    </span>
                    <span className="flex flex-col gap-1">
                      <span className="text-[13.5px] font-semibold leading-5 tracking-[-0.01em] text-[#EDEDF0]">
                        {title}
                      </span>
                      <span className="text-[12.5px] leading-[18px] text-[#85858D]">
                        {description}
                      </span>
                    </span>
                  </button>
                ))}
              </div>
            </section>
          ) : null}
        </div>

        <div className="mt-6 flex items-center justify-between gap-3">
          <p className="m-0 flex items-center gap-2 text-[12.5px] leading-5 text-[#77777F]">
            <span
              aria-hidden="true"
              className="size-2 shrink-0 rounded-full bg-[linear-gradient(180deg,#7B85E8,#5E6AD2)] shadow-[0_0_10px_rgba(94,106,210,0.6)]"
            />
            Greeting from {displayName}
          </p>
          <div className="flex items-center gap-0.5">
            <button
              type="button"
              onClick={() => scrollTo(0)}
              aria-label="Scroll to top"
              className={cn(
                "inline-flex size-6 items-center justify-center rounded-md text-[#6F6F77] transition-colors hover:bg-white/[0.05] hover:text-[#C9C9CF]",
                FOCUS,
              )}
            >
              <ChevronUp size={14} strokeWidth={2} aria-hidden="true" />
            </button>
            <button
              type="button"
              onClick={() => scrollTo(scrollRef.current?.scrollHeight ?? 0)}
              aria-label="Scroll to bottom"
              className={cn(
                "inline-flex size-6 items-center justify-center rounded-md text-[#6F6F77] transition-colors hover:bg-white/[0.05] hover:text-[#C9C9CF]",
                FOCUS,
              )}
            >
              <ChevronDown size={14} strokeWidth={2} aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

/** The GitHub mark, inlined because this lucide build omits brand icons. */
function GithubMark({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
      className={className}
    >
      <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
    </svg>
  );
}
