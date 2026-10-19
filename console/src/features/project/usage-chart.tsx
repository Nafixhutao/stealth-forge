"use client";

import type { components } from "@/api/generated/schema";
import { formatCount, formatUtcDay, pluralize } from "@/lib/format";

/**
 * The Overview's metering chart: one bar per reported day of the API request
 * series.
 *
 * Bars are divs rather than SVG: the figure is a plain distribution, and a flex
 * row keeps every bar exactly one CSS pixel of gap apart at any width without a
 * viewBox to distort. The API omits days with no activity, so an empty window
 * states that in words instead of drawing an empty plot, and a day that is
 * present with a zero count draws a muted stub rather than an invisible one.
 */

type ProjectUsageMetering =
  components["schemas"]["ProjectUsageMetering"];

/** Bar heights as percentages of the busiest day in the window. */
export function barPercentages(values: number[]): number[] {
  const max = values.reduce((highest, value) => Math.max(highest, value), 0);
  if (max <= 0) return values.map(() => 0);
  return values.map((value) =>
    value <= 0 ? 0 : Math.max(3, Math.round((value / max) * 100)),
  );
}

export function UsageChart({ metering }: { metering: ProjectUsageMetering }) {
  const { days, totals, from, to } = metering;
  const heights = barPercentages(days.map((day) => day.api_request_count));
  const total = totals.api_request_count;

  return (
    <figure className="m-0">
      <figcaption className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
        <span className="text-[12px] leading-5 text-[var(--projects-muted)]">
          API requests
        </span>
        <span className="text-[15px] font-semibold leading-5 text-[var(--projects-text)]">
          {formatCount(total)}
        </span>
        <span className="text-[12px] leading-5 text-[var(--projects-muted)]">
          {formatUtcDay(from)} – {formatUtcDay(to)}
        </span>
      </figcaption>

      {days.length === 0 ? (
        <p className="m-0 mt-3 rounded-[10px] border border-dashed border-[var(--projects-border-hover)] px-3 py-6 text-center text-[12px] leading-5 text-[var(--projects-muted)]">
          No API requests were metered in this window.
        </p>
      ) : (
        <>
          <div
            role="img"
            aria-label={`API requests per day, ${formatUtcDay(
              from,
            )} to ${formatUtcDay(to)}: ${formatCount(total)} in total.`}
            className="mt-3 flex h-[72px] items-end gap-[2px]"
          >
            {days.map((day, index) => (
              <span
                key={day.date}
                title={`${formatUtcDay(day.date)}: ${pluralize(
                  day.api_request_count,
                  "request",
                )}`}
                style={{ height: `${heights[index]}%` }}
                className={
                  heights[index] === 0
                    ? "min-h-[3px] w-full flex-1 rounded-[2px] bg-[var(--projects-control)]"
                    : "min-h-[3px] w-full flex-1 rounded-[2px] bg-[var(--user-accent)]/70"
                }
              />
            ))}
          </div>

          <div className="mt-2 flex items-baseline justify-between text-[11.5px] leading-4 text-[var(--projects-muted)]">
            <span>{formatUtcDay(from)}</span>
            <span>{formatUtcDay(to)}</span>
          </div>
        </>
      )}
    </figure>
  );
}
