"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import type { components } from "../api-schema";

type DoorEvent = components["schemas"]["DoorEvent"];
const api = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080/api/v1";

export default function EventsPage() {
  const [events, setEvents] = useState<DoorEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = useCallback(async (signal?: AbortSignal) => {
    try {
      const response = await fetch(`${api}/devices/door-01/events`, { cache: "no-store", signal });
      if (!response.ok) throw new Error("Could not load events.");
      const data: DoorEvent[] = await response.json();
      if (!signal?.aborted) {
        setEvents(data);
        setError("");
      }
    } catch {
      if (!signal?.aborted) setError("Could not load events. Try again.");
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void refresh(controller.signal);
    return () => controller.abort();
  }, [refresh]);

  return <main className="shell">
    <header><div><p className="eyebrow">SMART DOOR · DOOR 01</p><h1>Access history</h1><p className="sub">Recent access and security events</p><Link className="history-link" href="/">← Back to overview</Link></div></header>
    <section className="panel history-panel">
      <button className="secondary" onClick={() => void refresh()}>Refresh</button>
      {error && <div className="notice" role="alert">{error}</div>}
      {loading ? <p className="history-message">Loading events…</p> : events.length === 0 && !error ? <p className="history-message">No events yet.</p> : null}
      {events.map(event => <article key={event.id} className="event-entry">
        <div><strong>{event.kind.replaceAll("_", " ")}</strong><span className="event-meta">{event.source} · <time dateTime={event.recorded_at}>{new Date(event.recorded_at).toLocaleString()}</time></span></div>
        {event.snapshot_id && <img className="event-photo" src={`${api}/snapshots/${event.snapshot_id}`} alt="Visitor snapshot" />}
        {event.analysis && <p className="event-analysis">Analysis {event.analysis.status}{event.analysis.result?.description && ` · ${event.analysis.result.description}`}{event.analysis.result?.people_count != null && ` · People: ${event.analysis.result.people_count}`}</p>}
        <details><summary>Event details</summary><pre>{JSON.stringify(event.payload, null, 2)}</pre></details>
      </article>)}
    </section>
  </main>;
}
