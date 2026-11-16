"use client";

import { useEffect } from "react";

// Reveal once on entry. Never replays, never leaves content hidden.
export default function Reveal() {
  useEffect(() => {
    const targets = document.querySelectorAll<HTMLElement>("[data-reveal], [data-shutter]");
    const show = (el: Element) => el.classList.add("is-in");
    if (!("IntersectionObserver" in window)) { targets.forEach(show); return; }
    const io = new IntersectionObserver((entries) => {
      for (const e of entries) if (e.isIntersecting) { show(e.target); io.unobserve(e.target); }
    }, { rootMargin: "0px 0px -12% 0px", threshold: 0.1 });
    targets.forEach((el) => io.observe(el));
    const safety = window.setTimeout(() => targets.forEach((el) => {
      const r = el.getBoundingClientRect();
      if (r.top < innerHeight && r.bottom > 0) show(el);
    }), 2500);
    return () => { io.disconnect(); window.clearTimeout(safety); };
  }, []);
  return null;
}
