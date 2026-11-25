"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";

type Reading = {
  sensor?: string;
  valid?: boolean;
  error?: string;
  door_open?: boolean | null;
  motion?: boolean | null;
  vibration?: boolean | null;
  distance_mm?: number | null;
  light_lux?: number | null;
  recorded_at: string;
  received_at: string;
};
type State = { door_id: string; online: boolean; mqtt_connected: boolean; readings: Reading[] };

const api = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080/api/v1";
const fresh = (reading?: Reading) => !!reading && Date.now() - Date.parse(reading.received_at) < 30_000;
const sensorLabel = (reading: Reading | undefined, value: boolean | null | undefined, yes: string, no: string) =>
  !reading ? "Unknown" : !fresh(reading) ? "Stale" : reading.valid === false || value == null ? "Unknown" : value ? yes : no;
const doorLabel = (reading?: Reading) =>
  !reading ? "Unknown" : !fresh(reading) ? "Stale" : reading.door_open == null ? "Unknown" : reading.door_open ? "Open" : "Closed";
const sensorHint = (reading: Reading | undefined, normal: string) =>
  !fresh(reading) ? "No recent reading" : reading?.valid === false ? reading.error?.replaceAll("_", " ") || "Invalid reading" : normal;
const metricValue = (reading: Reading | undefined, value: number | null | undefined, unit: string) =>
  fresh(reading) && reading?.valid !== false && value != null ? `${value.toFixed(1)} ${unit}` : "—";

export default function Home() {
  const [state, setState] = useState<State | null>(null);
  const [error, setError] = useState("");
  const [commandError, setCommandError] = useState("");
  const [busy, setBusy] = useState(false);
  const [sentAction, setSentAction] = useState<"lock" | "unlock" | null>(null);
  const refresh = useCallback(async () => {
    try {
      const response = await fetch(`${api}/devices/door-01/state`, { cache: "no-store" });
      if (!response.ok) throw new Error("API unavailable");
      setState(await response.json());
      setError("");
    } catch {
      setError("Cannot reach the API");
    }
  }, []);
  useEffect(() => {
    void refresh();
    const id = setInterval(() => void refresh(), 2000);
    return () => clearInterval(id);
  }, [refresh]);
  const send = async (action: "lock" | "unlock") => {
    setBusy(true);
    setSentAction(null);
    setCommandError("");
    try {
      const response = await fetch(`${api}/devices/door-01/commands`, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action }),
      });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error || "Command failed");
      setSentAction(action);
    } catch (cause) {
      setCommandError(cause instanceof Error ? cause.message : "Command failed");
    } finally {
      setBusy(false);
    }
  };
  const readings = state?.readings ?? [];
  const sensor = (name: string) => readings.find((reading) => reading.sensor === name);
  const door = sensor("door"), motion = sensor("motion"), vibration = sensor("vibration");
  const distance = sensor("distance"), light = sensor("light");
  const nearbyLive = (fresh(distance) && distance?.valid !== false) || (fresh(light) && light?.valid !== false);

  return <main className="shell">
    <header><div><p className="eyebrow">SMART DOOR · DOOR 01</p><h1>Overview</h1><p className="sub">Live status and manual control</p><Link className="history-link" href="/readings">View readings history →</Link></div>
      <div className="badges"><span className={`badge ${state?.mqtt_connected ? "online" : "offline"}`}><i/>{state?.mqtt_connected ? "MQTT connected" : "MQTT disconnected"}</span><span className={`badge ${state?.online ? "online" : "offline"}`}><i/>{state?.online ? "ESP32 online" : "ESP32 offline"}</span></div></header>
    {error && <div className="notice">{error}</div>}
    {commandError && <div className="notice">{commandError}</div>}
    <section className="cards">
      <article className="card status"><div className="cardtop"><span className="label">DOOR STATUS</span><span className="icon">⌂</span></div><strong>{doorLabel(door)}</strong><small>{sensorHint(door, "Magnetic contact report")}</small></article>
      <article className="card status"><div className="cardtop"><span className="label">MOTION</span><span className="icon">◉</span></div><strong>{sensorLabel(motion, motion?.motion, "Motion", "Clear")}</strong><small>{sensorHint(motion, "Motion sensor report")}</small></article>
      <article className="card status"><div className="cardtop"><span className="label">VIBRATION</span><span className="icon">◌</span></div><strong>{sensorLabel(vibration, vibration?.vibration, "Detected", "Quiet")}</strong><small>{sensorHint(vibration, "Vibration sensor report")}</small></article>
    </section>
    <section className="lower"><article className="panel"><div className="panelhead"><div><h2>Nearby readings</h2><p>Latest gateway sensor values</p></div><span className="live">● {nearbyLive ? "LIVE" : "STALE"}</span></div>
      <div className="metrics"><Metric label="Distance" value={metricValue(distance, distance?.distance_mm, "mm")}/><Metric label="Light" value={metricValue(light, light?.light_lux, "lux")}/></div>
      <p className="updated">Updated {distance || light ? new Date(Math.max(Date.parse(distance?.received_at || "0"), Date.parse(light?.received_at || "0"))).toLocaleTimeString() : "—"}</p></article>
      <article className="panel control"><h2>Manual control</h2><p>Commands expire after 10 seconds and require the device to be online.</p><div className="buttons"><button disabled={busy || !state?.online} onClick={() => void send("lock")}>Lock door</button><button className="secondary" disabled={busy || !state?.online} onClick={() => void send("unlock")}>Unlock door</button></div>{sentAction && <div className="command"><b>{sentAction === "lock" ? "Lock" : "Unlock"} command sent</b><span>Sent to the MQTT broker</span></div>}</article></section>
    <footer>ESP32 online means the API received a sensor reading within 30 seconds. Automatic commands use the public MQTT broker.</footer>
  </main>;
}

function Metric({ label, value }: { label: string; value: string }) {
  return <div className="metric"><span>{label}</span><b>{value}</b></div>;
}
