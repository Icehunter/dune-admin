import { useState, useEffect, useCallback } from "react";
import { Button, Card, Spinner, toast } from "@heroui/react";
import { api, ApiError } from "../api/client";
import type { BaseRow } from "../api/client";
import { DataTable, Icon, PageHeader, type Column } from "../dune-ui";

type Key = "id" | "name" | "pieces" | "placeables" | "actions";

const COLUMNS: Column<Key>[] = [
  { key: "id", label: "ID", width: 80 },
  { key: "name", label: "Name", minWidth: 220 },
  { key: "pieces", label: "Pieces", width: 100 },
  { key: "placeables", label: "Placeables", width: 110 },
  { key: "actions", label: "", width: 120, sortable: false },
];

export default function BasesTab({ isSignedIn = true }: { isSignedIn?: boolean }) {
  const [bases, setBases] = useState<BaseRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [unsupported, setUnsupported] = useState(false);

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => {
        setLoading(true);
        setUnsupported(false);
      })
      .then(() => api.bases.list())
      .then(setBases)
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 404) setUnsupported(true);
        else toast.danger(`Failed to load bases: ${e instanceof Error ? e.message : String(e)}`);
      })
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      {!isSignedIn && (
        <div className="shrink-0 rounded-[var(--radius)] px-4 py-2 text-xs font-medium bg-danger/10 border border-danger/40 text-danger flex items-center gap-2">
          <Icon name="triangle-alert" />
          <span>
            A <strong>Layout Tools</strong> account is required to export bases. Sign in using the button in the top
            right.
          </span>
        </div>
      )}

      <PageHeader
        title={`Bases (${bases.length})`}
        subtitle="Live in-world player bases. Export any base as a solido-compatible blueprint."
      >
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading ? (
            <Spinner size="sm" color="current" />
          ) : (
            <>
              <Icon name="refresh-cw" /> Refresh
            </>
          )}
        </Button>
      </PageHeader>

      {loading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : unsupported ? (
        <Card className="self-center max-w-sm">
          <Card.Header>
            <Card.Title className="text-accent text-sm">Feature not available</Card.Title>
          </Card.Header>
          <Card.Content>
            <p className="text-xs text-muted text-center">
              This version of the dune-admin binary does not support base listing. Upgrade to the latest release to use
              this feature.
            </p>
          </Card.Content>
        </Card>
      ) : (
        <DataTable<BaseRow, Key>
          aria-label="Player bases"
          className="min-h-0 max-h-full"
          columns={COLUMNS}
          rows={bases}
          rowId={(b) => String(b.id)}
          initialSort={{ column: "id", direction: "ascending" }}
          sortValue={(b, k) => (k === "actions" ? "" : (b as unknown as Record<string, string | number>)[k])}
          emptyState={<div className="py-8 text-center text-muted">No bases found.</div>}
          renderCell={(b, key) => {
            switch (key) {
              case "id":
                return <span className="font-mono text-muted">{b.id}</span>;
              case "name":
                return b.name || <span className="text-muted">—</span>;
              case "pieces":
                return <span className="text-muted">{b.pieces}</span>;
              case "placeables":
                return <span className="text-muted">{b.placeables}</span>;
              case "actions":
                return isSignedIn ? (
                  <a href={api.bases.exportUrl(b.id)} download={b.name ? `${b.name}.json` : `base-${b.id}.json`}>
                    <Button size="sm" variant="outline" className="w-full">
                      <Icon name="download" /> Export
                    </Button>
                  </a>
                ) : (
                  <Button size="sm" variant="outline" className="w-full" isDisabled>
                    <Icon name="download" /> Export
                  </Button>
                );
            }
          }}
        />
      )}
    </div>
  );
}
