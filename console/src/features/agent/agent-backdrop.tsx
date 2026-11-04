"use client";

import { AGENT_ASCII } from "./agent-ascii";

/**
 * The Agent surface's atmosphere. One soft halo, a fine grid, and the Stealth
 * mark etched as ASCII — all faint enough that the composer stays the hero.
 * Nothing here takes pointer events.
 */
export function AgentBackdrop() {
  return (
    <div
      aria-hidden="true"
      className="pointer-events-none absolute inset-0 -z-10 overflow-hidden"
    >
      {/* Depth: a barely-lighter top settling into the page colour. */}
      <div className="absolute inset-0 bg-[linear-gradient(180deg,#111116_0%,#0D0E0E_42%,#0D0E0E_100%)]" />
      {/* One soft indigo halo rather than several competing glows. */}
      <div className="absolute left-1/2 top-[34%] h-[520px] w-[760px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-[radial-gradient(ellipse,rgba(94,106,210,0.12),transparent_70%)] blur-2xl" />
      {/* Fine grid, fading before the edges. */}
      <div className="absolute inset-0 [background-image:radial-gradient(circle,rgba(255,255,255,0.05)_1px,transparent_1px)] [background-size:28px_28px] [mask-image:radial-gradient(ellipse_at_center,black_6%,transparent_70%)] [-webkit-mask-image:radial-gradient(ellipse_at_center,black_6%,transparent_70%)]" />
      {/* The brand mark, etched as ASCII. */}
      <pre className="absolute left-1/2 top-[2%] -translate-x-1/2 select-none whitespace-pre font-mono text-[3.5px] leading-[3.5px] tracking-[-0.03em] text-[#B6B6BE] opacity-[0.07] [mask-image:radial-gradient(ellipse_at_center,black_28%,transparent_76%)] [-webkit-mask-image:radial-gradient(ellipse_at_center,black_28%,transparent_76%)] sm:text-[6px] sm:leading-[6px]">
        {AGENT_ASCII}
      </pre>
      {/* Edges fall away so the centre stays calm. */}
      <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,transparent_40%,rgba(0,0,0,0.55)_100%)]" />
    </div>
  );
}
