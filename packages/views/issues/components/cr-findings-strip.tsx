"use client";

import { useQuery } from "@tanstack/react-query";
import { Bot } from "lucide-react";
import { issueReviewThreadsOptions } from "@multica/core/issues/queries";
import { cn } from "@multica/ui/lib/utils";

/**
 * Compact CodeRabbit-findings status strip rendered at the top of the
 * issue page when there are CR review threads on the linked PR. Hidden
 * when the issue has no threads (no PR yet, no CR run, or repo not bound
 * to the GitHub App).
 *
 * Counts unresolved vs total. Severity-badge breakdown is summarized as
 * dots — Major / Minor / Trivial / Critical / Blocker — to give an at-
 * a-glance picture without reading every thread. When everything is
 * resolved the strip turns success-tinted.
 */
export function CRFindingsStrip({ issueId }: { issueId: string }) {
  const { data, isLoading } = useQuery(issueReviewThreadsOptions(issueId));
  if (isLoading || !data || data.threads.length === 0) return null;

  const total = data.threads.length;
  const unresolved = data.threads.filter((t) => t.state === "unresolved").length;
  const allResolved = unresolved === 0;

  // Severity-badge bucketing (only count unresolved threads — resolved ones
  // are no longer actionable).
  const buckets = new Map<string, number>();
  for (const t of data.threads) {
    if (t.state !== "unresolved") continue;
    const label = (t.severity_badge || "unknown").trim();
    buckets.set(label, (buckets.get(label) ?? 0) + 1);
  }

  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-md border px-3 py-2 text-sm",
        allResolved
          ? "border-success/30 bg-success/5 text-success"
          : "border-pink-500/30 bg-pink-500/5",
      )}
    >
      <div className="flex items-center gap-1.5 shrink-0">
        <Bot className={cn("h-4 w-4", allResolved ? "text-success" : "text-pink-500")} />
        <span className="font-medium">CodeRabbit</span>
      </div>
      <div className="text-muted-foreground">
        {allResolved ? (
          <>All {total} {total === 1 ? "finding" : "findings"} resolved</>
        ) : (
          <>
            <span className="font-medium text-foreground">{unresolved}</span>
            <span> unresolved</span>
            <span className="text-muted-foreground"> · {total} total</span>
          </>
        )}
      </div>
      {!allResolved && buckets.size > 0 && (
        <div className="ml-auto flex items-center gap-2">
          {Array.from(buckets.entries())
            .sort(([a], [b]) => severityRank(b) - severityRank(a))
            .map(([label, count]) => (
              <span
                key={label}
                className={cn(
                  "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium ring-1",
                  severityChipClass(label),
                )}
                title={`${count} ${label}`}
              >
                <span className={cn("h-2 w-2 rounded-full", severityDotClass(label))} />
                {count} {label}
              </span>
            ))}
        </div>
      )}
    </div>
  );
}

function severityRank(label: string): number {
  switch (label.toLowerCase()) {
    case "blocker":
      return 5;
    case "critical":
      return 4;
    case "major":
      return 3;
    case "minor":
      return 2;
    case "trivial":
      return 1;
    default:
      return 0;
  }
}

function severityChipClass(label: string): string {
  switch (label.toLowerCase()) {
    case "blocker":
    case "critical":
      return "bg-destructive/10 text-destructive ring-destructive/20";
    case "major":
      return "bg-warning/10 text-warning ring-warning/20";
    case "minor":
      return "bg-amber-500/10 text-amber-500 ring-amber-500/20";
    case "trivial":
      return "bg-muted text-muted-foreground ring-border";
    default:
      return "bg-muted text-muted-foreground ring-border";
  }
}

function severityDotClass(label: string): string {
  switch (label.toLowerCase()) {
    case "blocker":
    case "critical":
      return "bg-destructive";
    case "major":
      return "bg-warning";
    case "minor":
      return "bg-amber-500";
    case "trivial":
      return "bg-muted-foreground/60";
    default:
      return "bg-muted-foreground/40";
  }
}
