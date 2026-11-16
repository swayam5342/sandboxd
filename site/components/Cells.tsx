"use client";

import { useEffect, useState } from "react";

// A hundred cells: MAX_CONCURRENT. A few are in flight; they fill and empty like the semaphore does.
const seed = [3, 9, 14, 22, 27, 38, 41, 55, 63, 70, 84, 96];

export default function Cells() {
  const [on, setOn] = useState<Set<number>>(() => new Set(seed));

  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    let t: number;
    let visible = true;
    const host = document.querySelector(".band");
    const io = host ? new IntersectionObserver(([e]) => { visible = e.isIntersecting; }) : null;
    if (host && io) io.observe(host);
    const tick = () => {
      if (!visible) { t = window.setTimeout(tick, 520); return; }
      setOn((prev) => {
        const next = new Set(prev);
        const list = [...next];
        if (list.length > 6 && Math.random() < 0.55) next.delete(list[Math.floor(Math.random() * list.length)]);
        if (list.length < 22) next.add(Math.floor(Math.random() * 100));
        return next;
      });
      t = window.setTimeout(tick, 520);
    };
    t = window.setTimeout(tick, 900);
    return () => { window.clearTimeout(t); io?.disconnect(); };
  }, []);

  return (
    <>
      <p className="band-count"><span>{on.size}</span> of 100 in flight</p>
      <CellGrid on={on} />
    </>
  );
}

function CellGrid({ on }: { on: Set<number> }) {
  const label = `A grid of 100 cells, ${on.size} of them holding a running job`;
  return (
    <>
      <Grid on={on} cols={20} className="cells cells-wide" label={label} />
      <Grid on={on} cols={10} className="cells cells-tall" label={label} />
    </>
  );
}

// Each cell is the page's motif: a square with its top-left corner notched out.
function Grid({ on, cols, className, label }: { on: Set<number>; cols: number; className: string; label: string }) {
  const size = 10, gap = 2, n = 3, rows = 100 / cols;
  const w = cols * size + (cols - 1) * gap, h = rows * size + (rows - 1) * gap;
  return (
    <svg className={className} viewBox={`0 0 ${w} ${h}`} role="img" aria-label={label}>
      {Array.from({ length: 100 }, (_, i) => {
        const x = (i % cols) * (size + gap), y = Math.floor(i / cols) * (size + gap);
        return <path key={i} className={on.has(i) ? "on" : undefined} d={`M${x + n} ${y}H${x + size}V${y + size}H${x}V${y + n}H${x + n}Z`} />;
      })}
    </svg>
  );
}
