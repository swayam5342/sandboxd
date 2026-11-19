import type { Metadata } from "next";
import SiteHeader from "@/components/SiteHeader";

const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";

export const metadata: Metadata = { title: "Nobody here: sandboxd" };

// An empty cell: the page's notch, with no process in it.
export default function NotFound() {
  return (
    <>
    <a className="skip" href="#main">Skip to content</a>
    <SiteHeader base={`${base}/`} />
    <main className="nf wrap" id="main">
      <div className="nf-cell" aria-hidden="true" />
      <div className="nf-copy">
        <h1 className="display">Nobody here.</h1>
        <p>This cell is empty: the page you asked for doesn’t exist, or it was removed when its request finished.</p>
        <a className="btn" href={`${base}/`}>Back to sandboxd</a>
      </div>
    </main>
    </>
  );
}
