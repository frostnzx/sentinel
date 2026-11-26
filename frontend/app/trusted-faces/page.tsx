"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

type Person = {id: string; name: string; enrolled: boolean; deleted: boolean; version: number; applied_version: number | null; sync_error: string};
type Enrollment = {status: string; error: string; results: {filename: string; error?: string}[] | null};
const api = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080/api/v1";

export default function TrustedFaces() {
  const [people, setPeople] = useState<Person[]>([]);
  const [name, setName] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [selected, setSelected] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<Enrollment | null>(null);
  const refresh = useCallback(async () => {
    const response = await fetch(`${api}/trusted-people`, {cache: "no-store"});
    if (!response.ok) throw new Error("Could not load trusted faces.");
    setPeople(await response.json());
  }, []);
  useEffect(() => { void refresh().catch(() => setError("Could not load trusted faces.")); const timer = setInterval(() => void refresh().catch(() => {}), 5000); return () => clearInterval(timer); }, [refresh]);
  async function create(event: React.FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    try { const response = await fetch(`${api}/trusted-people`, {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({name})}); if (!response.ok) throw new Error("Could not create person."); const person = await response.json(); setSelected(person.id); setName(""); await refresh(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Request failed."); } finally { setBusy(false); }
  }
  async function enroll(event: React.FormEvent) {
    event.preventDefault(); setBusy(true); setError(""); setResult(null);
    try {
      const form = new FormData(); files.forEach(file => form.append("images", file));
      const response = await fetch(`${api}/trusted-people/${selected}/enrollments`, {method: "POST", body: form});
      if (!response.ok) {const data = await response.json(); throw new Error(data.error || "Could not start enrollment.");}
      const {id} = await response.json();
      for (let attempt = 0; attempt < 65; attempt++) {
        await new Promise(resolve => setTimeout(resolve, 1000));
        const status = await fetch(`${api}/enrollments/${id}`); if (!status.ok) throw new Error("Could not read enrollment status.");
        const data: Enrollment = await status.json(); if (data.status !== "pending") {setResult(data); await refresh(); return;}
      }
      throw new Error("Enrollment is still processing. Refresh to check the gallery.");
    } catch (cause) {setError(cause instanceof Error ? cause.message : "Enrollment failed.");} finally {setBusy(false);}
  }
  async function remove(person: Person) {
    if (!window.confirm(`Remove ${person.name} from trusted faces?`)) return;
    setBusy(true); setError(""); try {const response = await fetch(`${api}/trusted-people/${person.id}`, {method: "DELETE"}); if (!response.ok) throw new Error("Could not remove person."); await refresh(); if (selected === person.id) setSelected("");} catch (cause) {setError(cause instanceof Error ? cause.message : "Request failed.");} finally {setBusy(false);}
  }
  return <main className="shell"><header><div><p className="eyebrow">SMART DOOR · DOOR 01</p><h1>Trusted faces</h1><p className="sub">Manage people and door synchronization</p><Link className="history-link" href="/">← Back to overview</Link></div></header>
    {error && <div className="notice" role="alert">{error}</div>}
    <section className="lower"><article className="panel"><h2>People</h2>{people.length === 0 && <p className="history-message">No trusted people yet.</p>}{people.map(person => <div className="trusted-person" key={person.id}><div><strong>{person.name}</strong><small>{person.deleted ? "Removal pending" : person.enrolled ? "Enrolled" : "Awaiting photos"} · {person.sync_error ? `Sync error: ${person.sync_error}` : person.applied_version === person.version && person.version > 0 ? "Synced to Pi" : "Sync pending"}</small></div>{!person.deleted && <div className="buttons"><button className="secondary" disabled={busy} onClick={() => {setSelected(person.id); setResult(null);}}>Select</button><button className="secondary" disabled={busy} onClick={() => void remove(person)}>Remove</button></div>}</div>)}</article>
    <article className="panel"><h2>Add a person</h2><form className="trusted-form" onSubmit={create}><label>Name<input value={name} onChange={event => setName(event.target.value)} required /></label><button disabled={busy}>Add person</button></form><h2>Enrollment photos</h2><form className="trusted-form" onSubmit={enroll}><label>Person<select value={selected} onChange={event => setSelected(event.target.value)} required><option value="">Choose a person</option>{people.filter(person => !person.deleted).map(person => <option value={person.id} key={person.id}>{person.name}</option>)}</select></label><label>Upload 3–5 JPEG or PNG photos<input type="file" accept="image/jpeg,image/png" multiple onChange={event => setFiles(Array.from(event.target.files || []))} required /></label><button disabled={busy || files.length < 3 || files.length > 5}>{busy ? "Processing…" : "Enroll"}</button></form>{result && <div role="status"><p>{result.status === "completed" ? "Enrollment saved." : result.error}</p>{result.results?.map((item, index) => <p key={index}>{item.filename} · {item.error || "Accepted"}</p>)}</div>}</article></section></main>;
}
