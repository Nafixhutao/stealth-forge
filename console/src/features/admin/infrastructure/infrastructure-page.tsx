"use client";

import { Server } from "lucide-react";
import { HOSTS } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody } from "../components/admin-panel";
import { StatTile } from "../components/stat-tile";
import { LiveIndicator } from "../components/live-indicator";
import { ResourceCharts } from "./resource-charts";
import { HostList } from "./host-list";
import { WorkersSection } from "./workers-section";

/** Infrastructure — hosts, resources, workers, runtime health. */
export function InfrastructurePage() {
  const online = HOSTS.filter((host) => host.status === "online").length;
  const avgCpu = Math.round(HOSTS.reduce((sum, host) => sum + host.cpu, 0) / HOSTS.length);
  const memoryUsed = HOSTS.reduce((sum, host) => sum + (host.memory / 100) * host.memoryTotalGb, 0);
  const memoryTotal = HOSTS.reduce((sum, host) => sum + host.memoryTotalGb, 0);
  const storageUsed = HOSTS.reduce((sum, host) => sum + (host.storage / 100) * host.storageTotalGb, 0);
  const storageTotal = HOSTS.reduce((sum, host) => sum + host.storageTotalGb, 0);

  return (
    <AdminPageBody>
      <AdminHeader title="Infrastructure" subtitle="Monitor hosts, resources, workers, and runtime health.">
        <LiveIndicator />
      </AdminHeader>

      {/* Host summary */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
        <StatTile icon={Server} label="Hosts" value={String(HOSTS.length)} />
        <StatTile label="Online" value={String(online)} tone="success" />
        <StatTile label="Average CPU" value={`${avgCpu}%`} />
        <StatTile label="Memory" value={`${memoryUsed.toFixed(1)} GB`} hint={`/ ${memoryTotal} GB`} />
        <StatTile
          label="Storage"
          value={`${Math.round(storageUsed)} GB`}
          hint={storageTotal >= 1000 ? "/ 1 TB" : `/ ${storageTotal} GB`}
        />
      </div>

      <ResourceCharts />

      <HostList />

      <WorkersSection />
    </AdminPageBody>
  );
}
