"use client";

import { useEffect, useId, useRef, useState } from "react";
import * as echarts from "echarts/core";
import { type ComposeOption } from "echarts/core";
import {
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  type DataZoomComponentOption,
  type GridComponentOption,
  type LegendComponentOption,
  type TooltipComponentOption,
} from "echarts/components";
import { LineChart, type LineSeriesOption } from "echarts/charts";
import { CanvasRenderer } from "echarts/renderers";
import type { components } from "@/api/generated/schema";
import { formatDate } from "@/lib/format";

echarts.use([
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  LineChart,
  CanvasRenderer,
]);

type MetricChartOption = ComposeOption<
  | LineSeriesOption
  | DataZoomComponentOption
  | GridComponentOption
  | LegendComponentOption
  | TooltipComponentOption
>;

type AdminMetric = components["schemas"]["AdminMetric"];

function metricValueLabel(item: AdminMetric) {
  if (item.value !== undefined) {
    return item.value.toLocaleString(undefined, { maximumFractionDigits: 4 });
  }
  if (item.histogram) {
    return `count=${item.histogram.count.toLocaleString()} sum=${item.histogram.sum.toLocaleString(undefined, { maximumFractionDigits: 4 })}`;
  }
  if (item.summary) {
    return `count=${item.summary.count.toLocaleString()} sum=${item.summary.sum.toLocaleString(undefined, { maximumFractionDigits: 4 })}`;
  }
  if (item.exponential_histogram) {
    return `count=${item.exponential_histogram.count.toLocaleString()} sum=${item.exponential_histogram.sum.toLocaleString(undefined, { maximumFractionDigits: 4 })}`;
  }
  return "Structured value";
}

export type AdminMetricChartProps = { items: AdminMetric[] };

export function AdminMetricChart({ items }: AdminMetricChartProps) {
  const chartRef = useRef<HTMLDivElement>(null);
  const descriptionId = useId();
  // The data table can hold up to a thousand rows. Render it only once the
  // user expands the <details> so a collapsed, auto-refreshing panel does not
  // rebuild every row on each poll.
  const [tableOpen, setTableOpen] = useState(false);

  useEffect(() => {
    const element = chartRef.current;
    if (!element || !items.length) return;
    const chart = echarts.init(element, undefined, { renderer: "canvas" });
    const scalarItems = items.filter((item) => item.value !== undefined);
    const grouped = new Map<string, AdminMetric[]>();
    for (const item of scalarItems) {
      const key = `${item.service} · ${item.name}`;
      const group = grouped.get(key) ?? [];
      group.push(item);
      grouped.set(key, group);
    }
    const series = Array.from(grouped.entries())
      .slice(0, 8)
      .map(([name, points]) => ({
        name,
        type: "line" as const,
        showSymbol: false,
        smooth: false,
        connectNulls: false,
        data: points
          .slice()
          .sort(
            (left, right) =>
              Date.parse(left.timestamp) - Date.parse(right.timestamp),
          )
          .map((point) => [point.timestamp, point.value]),
      }));
    const option: MetricChartOption = {
      animation: false,
      backgroundColor: "transparent",
      color: ["#aaa6ec", "#6fc0cc", "#d491ac", "#76bc9a", "#ea9198"],
      grid: { top: 32, right: 20, bottom: 64, left: 64 },
      legend: {
        top: 0,
        left: 0,
        type: "scroll",
        textStyle: { color: "#96969e", fontSize: 11 },
      },
      tooltip: {
        trigger: "axis",
        backgroundColor: "#1e1e20",
        borderColor: "#2a2a2c",
        textStyle: { color: "#b7b7bd", fontSize: 12 },
      },
      xAxis: {
        type: "time",
        axisLabel: { color: "#96969e", fontSize: 11 },
        axisLine: { lineStyle: { color: "#252527" } },
        splitLine: { lineStyle: { color: "#252527" } },
      },
      yAxis: {
        type: "value",
        scale: true,
        axisLabel: { color: "#96969e", fontSize: 11 },
        axisLine: { show: false },
        splitLine: { lineStyle: { color: "#252527" } },
      },
      dataZoom: [
        { type: "inside", filterMode: "none" },
        {
          type: "slider",
          height: 16,
          bottom: 8,
          borderColor: "#252527",
          backgroundColor: "#111112",
          fillerColor: "rgba(170, 166, 236, 0.12)",
          handleStyle: { color: "#aaa6ec" },
          textStyle: { color: "#96969e", fontSize: 10 },
        },
      ],
      series,
    };
    chart.setOption(option);
    const observer = new ResizeObserver(() => chart.resize());
    observer.observe(element);
    return () => {
      observer.disconnect();
      chart.dispose();
    };
  }, [items]);

  const scalarItems = items.filter((item) => item.value !== undefined);
  const totalSeriesCount = new Set(
    scalarItems.map((item) => `${item.service} · ${item.name}`),
  ).size;
  const seriesCount = Math.min(totalSeriesCount, 8);
  const seriesLabel =
    totalSeriesCount > seriesCount
      ? `Metric time series showing ${seriesCount} of ${totalSeriesCount} series`
      : `Metric time series with ${seriesCount} series`;

  return (
    <div className="space-y-3">
      <div
        ref={chartRef}
        className="h-72 w-full"
        role="img"
        aria-label={seriesLabel}
        aria-describedby={descriptionId}
      />
      <p id={descriptionId} className="sr-only">
        {items.length} metric points are available, including{" "}
        {scalarItems.length} scalar points across {totalSeriesCount} series. The
        chart shows up to eight scalar series. Expand the data table to inspect
        structured metric values.
      </p>
      <details
        className="rounded-md border border-graphite bg-void"
        open={tableOpen}
      >
        <summary
          className="flex min-h-11 cursor-pointer items-center px-3 text-xs font-medium text-mist hover:text-paper"
          onClick={(event) => {
            // Keep <details> controlled so the row list is only built while it
            // is open. jsdom does not dispatch the native toggle event, so the
            // state is driven from the summary click instead.
            event.preventDefault();
            setTableOpen((value) => !value);
          }}
        >
          View data table
        </summary>
        <div className="overflow-x-auto border-t border-graphite p-3">
          <table className="w-full min-w-[640px] text-left text-xs">
            <caption className="sr-only">Metric time series data</caption>
            <thead className="text-fog">
              <tr>
                <th className="px-2 py-2 font-medium">Time</th>
                <th className="px-2 py-2 font-medium">Metric</th>
                <th className="px-2 py-2 font-medium">Service</th>
                <th className="px-2 py-2 text-right font-medium">Value</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-graphite">
              {tableOpen
                ? items.map((item, index) => (
                    <tr
                      key={`${item.timestamp}-${item.name}-${item.service}-${index}`}
                    >
                      <td className="whitespace-nowrap px-2 py-2 font-mono text-mist">
                        <time dateTime={item.timestamp}>
                          {formatDate(item.timestamp)}
                        </time>
                      </td>
                      <td className="px-2 py-2 font-mono text-mist">
                        {item.name}
                      </td>
                      <td className="px-2 py-2 text-mist">{item.service}</td>
                      <td className="px-2 py-2 text-right font-mono tabular-nums text-mist">
                        {metricValueLabel(item)}
                      </td>
                    </tr>
                  ))
                : null}
            </tbody>
          </table>
        </div>
      </details>
    </div>
  );
}
