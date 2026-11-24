"use client";
import { useCallback, useEffect, useState } from "react";

type Reading = { source:string; door_open:boolean|null; lock_state:string|null; presence:boolean|null; temperature_c:number|null; distance_mm:number|null; light_lux:number|null; recorded_at:string; received_at:string };
type State = { door_id:string; online:boolean; mqtt_connected:boolean; readings:Reading[] };
type Command = { id:string; action:string; status:string; message:string; created_at:string; expires_at:string };
const api = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080/api/v1";
const value = (n:number|null|undefined, unit:string) => n == null ? "—" : `${n.toFixed(1)} ${unit}`;

export default function Home() {
  const [state,setState]=useState<State|null>(null); const [error,setError]=useState(""); const [busy,setBusy]=useState(false); const [command,setCommand]=useState<Command|null>(null);
  const refresh=useCallback(async()=>{ try { const r=await fetch(`${api}/devices/door-01/state`,{cache:"no-store"}); if(!r.ok) throw new Error("API unavailable"); setState(await r.json()); setError(""); } catch { setError("Cannot reach the API"); } },[]);
  useEffect(()=>{ void refresh(); const id=setInterval(()=>void refresh(),2000); return ()=>clearInterval(id); },[refresh]);
  useEffect(()=>{ if(!command||command.status!=="pending") return; const id=setInterval(async()=>{ try { const r=await fetch(`${api}/commands/${command.id}`); if(r.ok) setCommand(await r.json()); } catch {} },500); return ()=>clearInterval(id); },[command]);
  const send=async(action:"lock"|"unlock")=>{setBusy(true);try{const r=await fetch(`${api}/devices/door-01/commands`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({action})});const body=await r.json();if(!r.ok)throw new Error(body.error||"Command failed");setCommand(body);}catch(e){setError(e instanceof Error?e.message:"Command failed");}finally{setBusy(false)}};
  const readings=state?.readings??[]; const node=readings.find(x=>x.source==="node"); const gateway=readings.find(x=>x.source==="gateway");
  return <main className="shell"><header><div><p className="eyebrow">SMART DOOR · DOOR 01</p><h1>Overview</h1><p className="sub">Live status and manual control</p></div><div className="badges"><span className={`badge ${state?.mqtt_connected?"online":"offline"}`}><i/>{state?.mqtt_connected?"MQTT connected":"MQTT disconnected"}</span><span className={`badge ${state?.online?"online":"offline"}`}><i/>{state?.online?"ESP32 online":"ESP32 offline"}</span></div></header>
    {error&&<div className="notice">{error}</div>}
    <section className="cards"><article className="card status"><div className="cardtop"><span className="label">DOOR POSITION</span><span className="icon">⌂</span></div><strong>{node?.door_open==null?"Unknown":node.door_open?"Open":"Closed"}</strong><small>Magnetic contact report</small></article>
    <article className="card status"><div className="cardtop"><span className="label">LOCK STATE</span><span className="icon">▣</span></div><strong>{node?.lock_state??"Unknown"}</strong><small>Device reported state</small></article>
    <article className="card status"><div className="cardtop"><span className="label">PRESENCE</span><span className="icon">◉</span></div><strong>{node?.presence==null?"Unknown":node.presence?"Nearby":"Clear"}</strong><small>Gateway proximity sensor</small></article></section>
    <section className="lower"><article className="panel"><div className="panelhead"><div><h2>Nearby readings</h2><p>Latest device sensor values</p></div><span className="live">● LIVE</span></div><div className="metrics"><Metric label="Temperature" value={value(node?.temperature_c,"°C")}/><Metric label="Distance" value={value(gateway?.distance_mm,"mm")}/><Metric label="Light" value={value(gateway?.light_lux,"lux")}/></div><p className="updated">Updated {node?new Date(node.recorded_at).toLocaleTimeString():"—"}</p></article>
    <article className="panel control"><h2>Manual control</h2><p>Commands expire after 10 seconds and require the device to be online.</p><div className="buttons"><button disabled={busy||!state?.online} onClick={()=>void send("lock")}>Lock door</button><button className="secondary" disabled={busy||!state?.online} onClick={()=>void send("unlock")}>Unlock door</button></div>{command&&<div className="command"><b>{command.status.replace("_"," ")}</b><span>{command.message||`${command.action} request sent`}</span></div>}</article></section>
    <footer>ESP32 online means the API received its state within 30 seconds. Automatic commands use the public MQTT broker.</footer></main>
}
function Metric({label,value}:{label:string;value:string}){return <div className="metric"><span>{label}</span><b>{value}</b></div>}
