import type { Metadata } from "next";

const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";

export const metadata: Metadata = { title: "Nobody here: sandboxd" };

// An empty cell: the page's notch, with no process in it.
export default function NotFound() {
  return (
    <>
    <a className="skip" href="#main">Skip to content</a>
    <header className="nav wrap">
      <a className="wordmark" href={`${base}/`}>sandboxd</a>
      <nav aria-label="Main"><ul><li><a className="link" href={`${base}/#try`}>Try it</a></li><li><a className="link" href={`${base}/#start`}>API</a></li></ul></nav>
    </header>
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
