"use client";

import { useState } from "react";
import { languages } from "@/lib/data";

// Limits are read straight out of each entry's YAML, so the readouts can't drift from the config shown.
type Phase = { time?: number; mem?: number; procs?: number };
const num = (s: string, k: string) => { const m = s.match(new RegExp(`${k}: (\\d+)`)); return m ? Number(m[1]) : undefined; };
const phase = (s: string): Phase => ({ time: num(s, "wall_time_s"), mem: num(s, "memory_kb"), procs: num(s, "max_processes") });
function limitsOf(yaml: string) {
  const i = yaml.search(/\n {2}run:/);
  const build = yaml.includes("\n  build:") ? phase(yaml.slice(0, i)) : null;
  return { build, run: phase(yaml.slice(i)) };
}
const fmtMem = (kb: number) => (kb >= 1048576 ? `${kb / 1048576} GB` : `${Math.round(kb / 1024)} MB`);

function Readout({ name, p }: { name: string; p: Phase | null }) {
  const cells = p
    ? [
        { k: "time", v: p.time ? `${p.time} s` : "default" },
        { k: "memory", v: p.mem ? fmtMem(p.mem) : "default" },
        ...(p.procs ? [{ k: "processes", v: String(p.procs) }] : []),
      ]
    : [];
  return (
    <div className="ro">
      <p className="ro-name">{name}</p>
      {p ? (
        <dl>
          {cells.map((c) => (
            <div key={c.k}><dt>{c.k}</dt><dd>{c.v}</dd></div>
          ))}
        </dl>
      ) : (
        <p className="ro-none">No build step</p>
      )}
    </div>
  );
}

// Keys set light, values set heavy: the YAML reads by weight, not by colour.
function Yaml({ text }: { text: string }) {
  return (
    <code>
      {text.split("\n").map((line, i) => {
        const [, ind, dash, key, rest] = line.match(/^(\s*)(- )?([\w-]+:)?(.*)$/)!;
        return (
          <span className="y-line" key={i}>
            {ind}{dash}{key && <span className="y-key">{key}</span>}<span className={key ? "y-val" : "y-cont"}>{rest}</span>
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
      <div className="spec" data-tone={l.tone} data-ui>
        <div className={`spec-in${fading ? " fading" : ""}`}>
          <p className="spec-head"><span>{l.bar ?? "config/lang.yaml"}</span></p>
          <pre className="spec-yaml"><Yaml text={l.yaml} /></pre>
          <div className="spec-lims" aria-label={`${l.name} limits`}>
            <Readout name="Build" p={lim.build} />
            <Readout name="Run" p={lim.run} />
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
