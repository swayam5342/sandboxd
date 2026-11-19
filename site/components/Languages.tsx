"use client";

import { useState } from "react";
import { languages } from "@/lib/data";

// Limits are read straight out of each entry's YAML, so the meters can't drift from the config shown.
type Phase = { time?: number; mem?: number; procs?: number };
const num = (s: string, k: string) => { const m = s.match(new RegExp(`${k}: (\\d+)`)); return m ? Number(m[1]) : undefined; };
const phase = (s: string): Phase => ({ time: num(s, "wall_time_s"), mem: num(s, "memory_kb"), procs: num(s, "max_processes") });
function limitsOf(yaml: string) {
  const i = yaml.search(/\n {2}run:/);
  const build = yaml.includes("\n  build:") ? phase(yaml.slice(0, i)) : null;
  return { build, run: phase(yaml.slice(i)) };
}

// Scales shared by every language, so switching tabs compares them. Time is linear up to
// rustc's 30 s; memory is log2 from 32 MB to 2 GB, since the range spans two orders of magnitude.
const MAX_TIME = 30;
const memFrac = (kb: number) => Math.min(1, Math.max(0.04, Math.log2(kb / 32768) / 6));
const fmtMem = (kb: number) => (kb >= 1048576 ? `${kb / 1048576} GB` : `${Math.round(kb / 1024)} MB`);

function Meter({ label, value, frac }: { label: string; value?: string; frac?: number }) {
  return (
    <div className="lm">
      <p className="lm-top"><span>{label}</span><b>{value ?? "default"}</b></p>
      <span className="lm-bar" data-empty={frac === undefined || undefined}><span style={{ transform: `scaleX(${frac ?? 0})` }} /></span>
    </div>
  );
}

function PhaseRow({ name, p }: { name: string; p: Phase | null }) {
  return (
    <div className="lrow">
      <p className="lrow-name">{name}</p>
      {p ? (
        <>
          <Meter label="time" value={p.time ? `${p.time} s` : undefined} frac={p.time ? p.time / MAX_TIME : undefined} />
          <Meter label="memory" value={p.mem ? fmtMem(p.mem) : undefined} frac={p.mem ? memFrac(p.mem) : undefined} />
          <p className="lrow-procs"><span>procs</span><b>{p.procs ?? "default"}</b></p>
        </>
      ) : (
        <p className="lrow-none">No build step: the source runs as is.</p>
      )}
    </div>
  );
}

// A light YAML highlighter: indent, an optional "- ", the key, then the value.
function Yaml({ text }: { text: string }) {
  return (
    <code>
      {text.split("\n").map((line, i) => {
        const m = line.match(/^(\s*)(- )?([\w-]+:)?(.*)$/)!;
        const [, ind, dash, key, rest] = m;
        const v = rest.trim();
        const cls = /^\d+$/.test(v) ? "y-num" : /^["[{]/.test(v) || /^[-/"]/.test(v) ? "y-str" : "y-val";
        return (
          <span className="y-line" key={i}>
            <span className="y-n" aria-hidden="true">{i + 1}</span>
            {ind}{dash && <span className="y-dash">{dash}</span>}{key && <span className="y-key">{key}</span>}<span className={key ? cls : "y-str"}>{rest}</span>
          </span>
        );
      })}
    </code>
  );
}

export default function Languages() {
  const [id, setId] = useState("c");
  const [fading, setFading] = useState(false);
  const l = languages.find((x) => x.id === id)!;
  const lim = limitsOf(l.yaml);

  const pick = (next: string) => {
    if (next === id) return;
    setFading(true);
    window.setTimeout(() => { setId(next); setFading(false); }, 150);
  };

  return (
    <section className="langs wrap" id="languages" aria-labelledby="langs-title">
      <div className="langs-stage">
        <div className="langs-cell" data-tone={l.tone} aria-hidden="true" />
        <div className={`langs-card${fading ? " fading" : ""}`} data-ui>
          <p className="card-bar"><span>{l.bar ?? "config/lang.yaml"}</span><span className="card-id">{l.yaml.match(/id: (\S+)/)?.[1]}</span></p>
          <pre><Yaml text={l.yaml} /></pre>
          <div className="lims" aria-label={`${l.name} limits`}>
            <PhaseRow name="build" p={lim.build} />
            <PhaseRow name="run" p={lim.run} />
          </div>
        </div>
      </div>
      <div className="langs-list">
        <h2 id="langs-title" className="head">Eight ready, add yours</h2>
        <ul role="list">
          {languages.map((x) => (
            <li key={x.id}>
              <button aria-pressed={x.id === id} onClick={() => pick(x.id)}>{x.name}</button>
            </li>
          ))}
        </ul>
        <p className="langs-fact" aria-live="polite">{l.fact}</p>
      </div>
    </section>
  );
}
