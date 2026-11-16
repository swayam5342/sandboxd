"use client";

import { useState } from "react";
import { languages } from "@/lib/data";

export default function Languages() {
  const [id, setId] = useState("c");
  const [fading, setFading] = useState(false);
  const l = languages.find((x) => x.id === id)!;

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
          <p className="card-bar">{l.bar ?? "config/lang.yaml"}</p>
          <pre><code>{l.yaml}</code></pre>
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
      </div>
      <p className="langs-fact" aria-live="polite">{l.fact}</p>
    </section>
  );
}
