"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import type { components } from "../api-schema";

type Reading = components["schemas"]["Reading"];
type Filters = { from: string; to: string };
const api = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080/api/v1";
const pageSize = 100;
const label = (value: boolean | null | undefined, yes: string, no: string) => value == null ? "—" : value ? yes : no;
const metric = (value: number | null | undefined, unit: string) => value == null ? "—" : `${value.toFixed(1)} ${unit}`;

export default function ReadingsPage() {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [rows, setRows] = useState<Reading[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const activeFilters = useRef<Filters>({ from: "", to: "" });
  const pending = useRef<AbortController | null>(null);

  const load = useCallback(async (filters: Filters, append = false, oldest?: string) => {
    const fromTime = filters.from ? new Date(filters.from).getTime() : NaN;
    const toTime = filters.to ? new Date(filters.to).getTime() : NaN;
    if ((filters.from && !Number.isFinite(fromTime)) || (filters.to && !Number.isFinite(toTime)) || (filters.from && filters.to && fromTime > toTime)) {
      setError("Choose a valid date range with From before To.");
      setRows([]);
      setHasMore(false);
      return;
    }
    pending.current?.abort();
    const controller = new AbortController();
    pending.current = controller;
    setLoading(true);
    setError("");
    if (!append) {
      activeFilters.current = filters;
      setRows([]);
      setHasMore(false);
    }
    const params = new URLSearchParams({ source: "combined", limit: String(pageSize) });
    if (filters.from) params.set("from", new Date(fromTime).toISOString());
    if (oldest) params.set("to", new Date(Date.parse(oldest) - 1).toISOString());
    else if (filters.to) params.set("to", new Date(toTime).toISOString());
    try {
      const response = await fetch(`${api}/devices/door-01/readings?${params}`, { cache: "no-store", signal: controller.signal });
      if (!response.ok) throw new Error("Could not load readings history.");
      const batch: Reading[] = await response.json();
      if (controller.signal.aborted) return;
      setRows(previous => append ? [...previous, ...batch] : batch);
      setHasMore(batch.length === pageSize);
    } catch {
      if (!controller.signal.aborted) setError("Could not load readings history. Try again.");
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load({ from: "", to: "" });
    return () => pending.current?.abort();
  }, [load]);

  return <main className="shell">
    <header><div><p className="eyebrow">SMART DOOR · DOOR 01</p><h1>Readings history</h1><p className="sub">Combined sensor logs every five seconds</p><Link className="history-link" href="/">← Back to overview</Link></div></header>
    <section className="panel history-panel">
      <form className="history-filters" onSubmit={event => { event.preventDefault(); void load({ from, to }); }}>
        <label>From<input type="datetime-local" value={from} onChange={event => setFrom(event.target.value)}/></label>
        <label>To <span className="filter-hint">(blank for latest)</span><input type="datetime-local" value={to} onChange={event => setTo(event.target.value)}/></label>
        <button type="submit" disabled={loading}>Apply</button>
        <button type="button" className="secondary" disabled={loading} onClick={() => void load({ from, to })}>Refresh</button>
      </form>
      {error && <div className="notice" role="alert">{error}</div>}
      {loading && rows.length === 0 ? <p className="history-message">Loading readings…</p> : rows.length === 0 && !error ? <p className="history-message">No readings in this range.</p> : null}
      {rows.length > 0 && <div className="history-scroll"><table className="history-table"><thead><tr><th scope="col">Log time</th><th scope="col">Door status</th><th scope="col">Motion</th><th scope="col">Vibration</th><th scope="col">Distance</th><th scope="col">Light</th></tr></thead><tbody>
        {rows.map(row => <tr key={row.id}><td><time dateTime={row.recorded_at}>{new Date(row.recorded_at).toLocaleString()}</time></td><td>{label(row.door_open, "Open", "Closed")}</td><td>{label(row.motion, "Detected", "Clear")}</td><td>{label(row.vibration, "Detected", "Quiet")}</td><td>{metric(row.distance_mm, "mm")}</td><td>{metric(row.light_lux, "lux")}</td></tr>)}
      </tbody></table></div>}
      {hasMore && <div className="history-actions"><button type="button" className="secondary" disabled={loading} onClick={() => void load(activeFilters.current, true, rows.at(-1)?.recorded_at)}>{loading ? "Loading…" : "Load more"}</button></div>}
    </section>
  </main>;
}
