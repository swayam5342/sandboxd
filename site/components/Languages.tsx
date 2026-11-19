"use client";

import { useState } from "react";
import { languages, type Lang } from "@/lib/data";

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
  if (!p) {
    return (
      <div className="ro ro-off">
        <p className="ro-name">{name}</p>
        <p className="ro-none">None, it runs the source</p>
      </div>
    );
  }
  const cells = [
    { k: "time", v: p.time ? `${p.time} s` : undefined },
    { k: "memory", v: p.mem ? fmtMem(p.mem) : undefined },
    { k: "processes", v: p.procs ? String(p.procs) : undefined },
  ];
  return (
    <div className="ro">
      <p className="ro-name">{name}</p>
      <dl>
        {cells.map((c) => (
          <div key={c.k} className={c.v ? undefined : "unset"}>
            <dt>{c.k}</dt>
            <dd>{c.v ?? "default"}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

// Keys set light, values at full ink: the YAML reads by weight, not by colour.
function Yaml({ text }: { text: string }) {
  return (
    <code>
      {text.split("\n").map((line, i) => {
        const [, ind, dash, key, rest] = line.match(/^(\s*)(- )?([\w-]+:)?(.*)$/)!;
        return (
          <span className="y-line" key={i}>
            {ind}{dash && <span className="y-key">{dash}</span>}{key && <span className="y-key">{key}</span>}<span className="y-val">{rest}</span>
          </span>
        );
      })}
    </code>
  );
}

// One entry, as printed on the card. Every entry is rendered into the same grid cell so the card
// is always as tall as the longest one and never jumps when the selection changes.
function Entry({ l, on }: { l: Lang; on: boolean }) {
  const lim = limitsOf(l.yaml);
  return (
    <div className="entry" data-on={on} aria-hidden={!on} inert={!on}>
      <pre className="spec-yaml" tabIndex={on ? 0 : -1} aria-label={`${l.name} entry in lang.yaml`}><Yaml text={l.yaml} /></pre>
      <div className="spec-lims" aria-label={`${l.name} limits`}>
        <Readout name="Build" p={lim.build} />
        <Readout name="Run" p={lim.run} />
      </div>
      <p className="spec-fact">{l.fact}</p>
    </div>
  );
}

export default function Languages() {
  const [id, setId] = useState("c");
  const l = languages.find((x) => x.id === id)!;

  return (
    <section className="langs wrap" id="languages" aria-labelledby="langs-title">
      <h2 id="langs-title" className="head langs-title">Eight ready, add&nbsp;yours</h2>
      <ul className="langs-list" role="list">
        {languages.map((x) => (
          <li key={x.id}>
            <button aria-pressed={x.id === id} onClick={() => setId(x.id)}>{x.name}</button>
          </li>
        ))}
      </ul>
      <div className="spec" data-tone={l.tone} data-ui>
        <p className="spec-head">
          <span>{l.bar ?? "config/lang.yaml"}</span>
          <span>{l.yaml.includes("\n  build:") ? "build, then run" : "run only"}</span>
        </p>
        <div className="spec-body">
          {languages.map((x) => <Entry key={x.id} l={x} on={x.id === id} />)}
        </div>
      </div>
      <p className="sr" aria-live="polite">{l.fact}</p>
    </section>
  );
}
