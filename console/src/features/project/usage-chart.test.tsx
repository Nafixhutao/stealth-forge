import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { UsageChart, barPercentages } from "@/features/project/usage-chart";

function metering(
  overrides: Partial<{
    days: {
      date: string;
      api_request_count: number;
      api_egress_bytes: number;
      function_invocation_count: number;
      function_failure_count: number;
      function_compute_ms: number;
    }[];
    total: number;
  }> = {},
) {
  const days = overrides.days ?? [];
  return {
    project_id: "33333333-3333-3333-3333-333333333331",
    from: "2026-09-19",
    to: "2026-10-18",
    days,
    totals: {
      date: "2026-10-18",
      api_request_count:
        overrides.total ??
        days.reduce((sum, day) => sum + day.api_request_count, 0),
      api_egress_bytes: 0,
      function_invocation_count: 0,
      function_failure_count: 0,
      function_compute_ms: 0,
    },
  };
}

function day(date: string, api_request_count: number) {
  return {
    date,
    api_request_count,
    api_egress_bytes: 0,
    function_invocation_count: 0,
    function_failure_count: 0,
    function_compute_ms: 0,
  };
}

describe("barPercentages", () => {
  it("scales the busiest day to full height", () => {
    expect(barPercentages([10, 20, 5])).toEqual([50, 100, 25]);
  });

  it("keeps a visible stub for a day with no activity", () => {
    // A zero day is drawn as a muted stub, so it must not come back as 0%.
    expect(barPercentages([0, 400])).toEqual([0, 100]);
    expect(barPercentages([1, 1000])[0]).toBe(3);
  });

  it("returns an even profile for a window with no activity at all", () => {
    expect(barPercentages([0, 0, 0])).toEqual([0, 0, 0]);
  });

  it("has nothing to scale an empty window against", () => {
    expect(barPercentages([])).toEqual([]);
  });
});

describe("UsageChart", () => {
  it("states the total and the window the API reported", () => {
    render(
      <UsageChart
        metering={metering({
          days: [day("2026-09-19", 300), day("2026-09-20", 120)],
        })}
      />,
    );

    expect(screen.getByText("API requests")).toBeInTheDocument();
    expect(screen.getByText("420")).toBeInTheDocument();
    // Both window bounds come from the payload, not from the first and last
    // bucket, so a quiet week at either end still labels the real window.
    expect(screen.getAllByText("Sep 19, 2026 – Oct 18, 2026")).toHaveLength(1);
    expect(screen.getByText("Sep 19, 2026")).toBeInTheDocument();
    expect(screen.getByText("Oct 18, 2026")).toBeInTheDocument();
  });

  it("draws one bar per reported day, each naming its own date", () => {
    render(
      <UsageChart
        metering={metering({
          days: [day("2026-09-19", 300), day("2026-09-20", 1)],
        })}
      />,
    );

    const chart = screen.getByRole("img");
    expect(chart.children).toHaveLength(2);
    expect(chart.children[0]).toHaveAttribute(
      "title",
      "Sep 19, 2026: 300 requests",
    );
    expect(chart.children[1]).toHaveAttribute(
      "title",
      "Sep 20, 2026: 1 request",
    );
    expect(chart).toHaveAttribute(
      "aria-label",
      "API requests per day, Sep 19, 2026 to Oct 18, 2026: 301 in total.",
    );
  });

  it("says so in words when the window has no activity", () => {
    render(<UsageChart metering={metering({ days: [], total: 0 })} />);

    // The API omits empty days, so an empty series is a real answer: the chart
    // explains the window instead of drawing an empty plot.
    expect(
      screen.getByText("No API requests were metered in this window."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("img")).toBeNull();
  });
});
