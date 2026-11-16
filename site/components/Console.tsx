"use client";

import { useRef, useState } from "react";
import { attempts } from "@/lib/data";

export default function Console() {
  const [id, setId] = useState(attempts[0].id);
  const [swapping, setSwapping] = useState(false);
  const tabs = useRef<(HTMLButtonElement | null)[]>([]);
  const a = attempts.find((x) => x.id === id)!;

  const pick = (next: string) => {
    if (next === id) return;
    setSwapping(true);
    window.setTimeout(() => { setId(next); setSwapping(false); }, 140);
  };

  const onKey = (e: React.KeyboardEvent, i: number) => {
    const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    if (!d) return;
    e.preventDefault();
    const j = (i + d + attempts.length) % attempts.length;
    tabs.current[j]?.focus();
    pick(attempts[j].id);
  };

  const body = `HTTP ${a.http}\n${JSON.stringify(a.response, null, 2)}`;

  return (
    <div className="console" id="try" data-ui>
      <div className="console-head">
        <p className="console-title">Try to break out</p>
        <p className="console-sub">Pick an attempt. This is what sandboxd sends back.</p>
      </div>
      <div className="attempts" role="tablist" aria-label="Escape attempts">
        {attempts.map((x, i) => (
          <button
            key={x.id}
            ref={(el) => { tabs.current[i] = el; }}
            role="tab"
            id={`tab-${x.id}`}
            aria-selected={x.id === id}
            aria-controls="attempt-panel"
            tabIndex={x.id === id ? 0 : -1}
            onClick={() => pick(x.id)}
            onKeyDown={(e) => onKey(e, i)}
          >
            {x.label}
          </button>
        ))}
      </div>
      <div className={`console-body${swapping ? " swap" : ""}`} role="tabpanel" id="attempt-panel" aria-labelledby={`tab-${a.id}`} aria-live="polite">
        <div className="req">
          <p className="req-line"><span className="verb">POST</span> /run <span className="lang">{a.lang}</span></p>
          <pre className="src"><code>{a.source}{a.extra ? `\n\n${a.extra}` : ""}</code></pre>
        </div>
        <div className="verdict" data-kind={a.kind}>
          <p className="v-status">{a.status}</p>
          <p className="v-why"><span className="v-by">Stopped by <code>{a.by}</code></span> {a.why}</p>
        </div>
        <pre className="resp"><code>{body}</code></pre>
      </div>
    </div>
  );
}
