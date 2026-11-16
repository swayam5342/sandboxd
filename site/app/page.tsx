import Console from "@/components/Console";
import Languages from "@/components/Languages";
import Cells from "@/components/Cells";
import Composer from "@/components/Composer";
import Reveal from "@/components/Reveal";
import { asset, REPO } from "@/lib/data";

const d = (ms: number) => ({ "--d": `${ms}ms` }) as React.CSSProperties;
const g = (n: number) => ({ "--g": n }) as React.CSSProperties;

const segments = [
  { g: 15, c: "surface", time: "Microseconds", text: "First the request is checked: a known language, source under 256 KiB, every flag allowed and none denied." },
  { g: 14, c: "raised", time: "Milliseconds", text: "Then a fresh directory named after the request ID is made, owned by 65534, and the source is written in." },
  { g: 28, c: "accent", time: "Up to 30 seconds", text: "Next, if the language compiles, the compiler runs in its own jail under the build limits." },
  { g: 30, c: "field", time: "Up to 9 seconds a test", text: "After that each test runs in a new jail with its stdin, and stdout is compared with what you expected." },
  { g: 13, c: "deep", time: "Always", text: "Finally the directory is removed. Anything a crash left behind is swept at the next start." },
];

// The footer mark: each letter knocked out of its own notched block.
const markHeights = [176, 148, 220, 198, 160, 190, 136, 212];
const markFills = ["accent", "paper", "accent", "accent-strong", "accent", "paper", "accent", "accent"];
const blockW = (1000 - 4 * 7) / 8;

const gh = (p: string) => `${REPO}/blob/main/${p}`;

export default function Page() {
  return (
    <>
      <a className="skip" href="#main">Skip to content</a>

      <header className="nav wrap">
        <a className="wordmark" href="#top" aria-label="sandboxd, back to top"><svg className="cell-dot" viewBox="10 10 44 44" aria-hidden="true"><path d="M28 10H54V54H10V28H28Z" fill="#4cb3f0" /><rect x="13" y="13" width="11" height="11" fill="#f2f2f2" /></svg>sandboxd</a>
        <nav aria-label="Main">
          <ul>
            <li><a className="link" href="#try">Try it</a></li>
            <li><a className="link" href="#languages">Languages</a></li>
            <li><a className="link" href="#walls">Walls</a></li>
            <li><a className="link" href="#start">API</a></li>
          </ul>
        </nav>
      </header>

      <main id="main">
        {/* Hero: fragment on a shaped stage */}
        <section className="hero" id="top" aria-labelledby="hero-title">
          <h1 id="hero-title" className="display hero-title">Run code you<br /> don’t trust.</h1>
          <div className="hero-side">
            <p className="hero-lede">sandboxd is a Go daemon that compiles and runs untrusted code in any language you configure, each submission jailed by NsJail as user nobody, with no network.</p>
            <a className="btn" href={REPO}>Get it on GitHub</a>
            <div className="quick" data-ui>
              <p className="quick-bar">Two commands to a running daemon</p>
              <pre><code>
                <span className="q-line"><span className="q-p">$</span> docker compose up --build</span>
                <span className="q-line"><span className="q-p">$</span> curl localhost:8089/healthz</span>
                <span className="q-line q-o">{'{"status":"ok"}'}</span>
              </code></pre>
            </div>
          </div>
          <div className="stage">
            <Console />
          </div>
        </section>

        <Languages />

        {/* Lifecycle: stages as one bar */}
        <section className="life wrap" id="lifecycle" aria-labelledby="life-title">
          <h2 id="life-title" className="display" data-reveal>One request, start to end</h2>
          <ol className="bar" data-ui role="list">
            {segments.map((s) => (
              <li key={s.time} className="seg" style={g(s.g)} data-reveal>
                <div className="seg-block" data-c={s.c}><p className="seg-time">{s.time}</p></div>
                <p className="seg-text">{s.text}</p>
              </li>
            ))}
          </ol>
        </section>

        {/* Walls: close-ups on a board */}
        <section className="walls wrap" id="walls" aria-labelledby="walls-title">
          <div className="board">
            <div className="tile t-head" data-c="surface" data-reveal>
              <h2 id="walls-title" className="head">Eight walls, every&nbsp;run</h2>
            </div>
            <div className="tile t-passwd" data-c="field" data-reveal>
              <div className="zoom" data-ui aria-hidden="true"><code>nobody:x:65534:65534:<br />nobody:/:/usr/sbin/nologin</code></div>
              <div className="cap"><h3>Always nobody</h3><p>Every run is uid 65534. Its /etc/passwd is written fresh, with that one line.</p></div>
            </div>
            <div className="tile t-net" data-c="accent" data-reveal>
              <div className="zoom" data-ui aria-hidden="true">
                <span className="switch"><span className="knob" /></span>
                <span className="switch-label">network</span>
              </div>
              <div className="cap"><h3>No network</h3><p>iface_no_lo: the jail has no interface to send anything through.</p></div>
            </div>
            <div className="tile t-flag" data-c="raised" data-reveal>
              <div className="zoom" data-ui aria-hidden="true"><span className="chip"><code>-C linker=/bin/sh</code></span><span className="chip chip-deny">denied</span></div>
              <div className="cap"><h3>Flags on a list</h3><p>Allowlisted per phase. The denylist beats any wildcard.</p></div>
            </div>
            <div className="tile t-nproc" data-c="surface" data-reveal>
              <div className="zoom" data-ui aria-hidden="true"><span className="readout">50</span></div>
              <div className="cap"><h3>Fork bombs starve</h3><p>rlimit_nproc caps the children: bash gets 50 processes.</p></div>
            </div>
            <div className="tile t-out" data-c="raised" data-reveal>
              <div className="zoom" data-ui aria-hidden="true">
                <span className="meter"><span className="meter-fill" /></span>
                <span className="meter-label">4.0 / 4 MB, cut</span>
              </div>
              <div className="cap"><h3>Four megabytes of output</h3><p>Then the stream is cut, so a print loop can’t fill the daemon’s memory.</p></div>
            </div>
            <div className="tile t-ro" data-c="accent" data-reveal>
              <div className="zoom" data-ui aria-hidden="true"><code>--bindmount_ro /usr<br />--bindmount_ro /lib<br />--bindmount_ro /bin</code></div>
              <div className="cap"><h3>Read-only system</h3><p>Toolchains mount read-only. /proc is new and sees only the jail.</p></div>
            </div>
            <div className="tile t-argv" data-c="field" data-reveal>
              <div className="zoom" data-ui aria-hidden="true"><code>[&quot;gcc&quot;, &quot;-O2&quot;,<br /> &quot;-o&quot;, &quot;solution&quot;,<br /> &quot;solution.c&quot;]</code></div>
              <div className="cap"><h3>No shell</h3><p>Arguments reach exec as a slice. A ; or a | is only a character.</p></div>
            </div>
            <figure className="tile t-lock" data-reveal>
              <img src={asset("padlock-in-museum-castle-of-mayenne.webp")} alt="A heavy iron padlock with pierced gothic tracery, in the castle museum of Mayenne" width={560} height={646} decoding="async" loading="lazy" className="drift" />
              <figcaption className="cap"><h3>One key for /run</h3><p>Set API_KEY and every run needs it as a Bearer token.</p></figcaption>
            </figure>
          </div>
        </section>

        {/* A hundred cells */}
        <section className="band" aria-labelledby="band-title">
          <div className="band-in wrap">
            <div className="band-copy">
              <h2 id="band-title" className="head">A hundred at a time</h2>
              <p>MAX_CONCURRENT defaults to 100. Each request takes a cell from the semaphore; the 101st waits for one to empty.</p>
            </div>
            <Cells />
          </div>
        </section>

        {/* History */}
        <section className="hist wrap" id="history" aria-labelledby="hist-title">
          <figure className="hist-pic" data-shutter>
            <div className="notched"><div className="shutter">
              <img src={asset("eastern-state-penitentiary-cell-block.webp")} alt="A long, barred cell block in Eastern State Penitentiary, lit by a skylight, a single visitor walking its length" width={1000} height={1333} loading="lazy" decoding="async" />
            </div></div>
            <figcaption>Eastern State Penitentiary, Philadelphia. Its separate system gave every prisoner a cell of their own.</figcaption>
          </figure>
          <div className="hist-copy">
            <h2 id="hist-title" className="head" data-reveal>Isolation is an old idea</h2>
            <ul className="years" role="list">
              <li data-reveal>
                <img src={asset("panopticon.webp")} alt="Willey Reveley’s plan and section of Bentham’s panopticon: a ring of cells around a central tower" width={240} height={264} loading="lazy" />
                <div><p className="year">1791</p><p>Jeremy Bentham’s panopticon puts every cell in view of one tower. sandboxd’s tower is GET /info: jobs in flight, jobs total, internal failures.</p></div>
              </li>
              <li data-reveal className="no-pic">
                <div><p className="year">1829</p><p>Eastern State opens with one prisoner to a cell and no contact between them. One job, one directory, one jail.</p></div>
              </li>
              <li data-reveal>
                <img src={asset("pdp-11-40.webp")} alt="A DEC PDP-11/40 minicomputer cabinet with its tape reels and switch panel" width={240} height={320} loading="lazy" />
                <div><p className="year">1979</p><p>chroot ships in Unix Version 7, on machines like this PDP-11: a process can be handed a smaller root.</p></div>
              </li>
              <li data-reveal className="no-pic">
                <div><p className="year">2002 to 2013</p><p>Linux adds mount, PID, network and user namespaces. NsJail joins them in one binary; sandboxd puts an HTTP API in front.</p></div>
              </li>
            </ul>
          </div>
        </section>

        {/* CTA: first step, already open */}
        <section className="cta" id="start" aria-labelledby="cta-title">
          <div className="cta-in wrap">
            <div className="cta-copy">
              <h2 id="cta-title" className="display">Run it as nobody</h2>
              <p>Start it with <code>docker compose up --build</code>. It answers on port 8089. The button copies this request as curl, ready to paste.</p>
              <p><a className="link" href={gh("docs/api.md")}>Read the API reference</a></p>
            </div>
            <Composer />
          </div>
        </section>
      </main>

      <footer className="foot">
        <div className="wrap">
          <svg className="mark" viewBox="0 0 1000 220" role="img" aria-label="sandboxd">
            {"sandboxd".split("").map((ch, i) => {
              const x = i * (blockW + 4), h = markHeights[i], y = 220 - h, n = 26;
              return (
                <g key={i}>
                  <path d={`M${x + n} ${y}H${x + blockW}V220H${x}V${y + n}H${x + n}Z`} style={{ fill: `var(--${markFills[i]})` }} />
                  <text x={x + blockW / 2} y={204} textAnchor="middle">{ch}</text>
                </g>
              );
            })}
          </svg>
          <div className="foot-grid">
            <div><h3>Docs</h3><ul role="list">
              <li><a className="link" href={gh("docs/api.md")}>API reference</a></li>
              <li><a className="link" href={gh("docs/architecture.md")}>Architecture</a></li>
              <li><a className="link" href={gh("docs/languages.md")}>Languages</a></li>
              <li><a className="link" href={gh("docs/security.md")}>Threat model</a></li>
            </ul></div>
            <div><h3>Code</h3><ul role="list">
              <li><a className="link" href={REPO}>Repository</a></li>
              <li><a className="link" href={`${REPO}/issues`}>Issues</a></li>
              <li><a className="link" href={gh("config/lang.yaml")}>lang.yaml</a></li>
              <li><a className="link" href="https://hub.docker.com/r/swayam5342/sandboxd">Docker Hub image</a></li>
            </ul></div>
            <div><h3>Routes</h3><ul role="list">
              <li>GET /healthz</li><li>GET /readyz</li><li>GET /info</li><li>POST /run</li>
            </ul></div>
            <div><h3>On this page</h3><ul role="list">
              <li><a className="link" href="#try">Try to break out</a></li>
              <li><a className="link" href="#lifecycle">Lifecycle</a></li>
              <li><a className="link" href="#history">History</a></li>
              <li><a className="link" href="#languages">Languages</a></li>
            </ul></div>
            <div className="foot-contact">
              <p>Listens on :8089.</p>
              <p>Linux only. On macOS or Windows, run it in Docker or WSL2.</p>
            </div>
          </div>
          <p className="credits">Pictures: <a href="https://commons.wikimedia.org/wiki/File:Panopticon.jpg">“Panopticon”</a> by Jeremy Bentham, <a href="https://creativecommons.org/publicdomain/mark/1.0/">public domain</a>; <a href="https://commons.wikimedia.org/w/index.php?curid=4087098">“Eastern State Penitentiary cell block”</a> by Killbot24, <a href="https://creativecommons.org/licenses/by-sa/3.0/">CC BY-SA 3.0</a>; <a href="https://commons.wikimedia.org/w/index.php?curid=148592747">“Padlock in Museum Castle of Mayenne”</a> by Rei Momo, <a href="https://creativecommons.org/publicdomain/zero/1.0/deed.en/">CC0</a>; <a href="https://commons.wikimedia.org/w/index.php?curid=466937">“Pdp-11-40”</a> by Stefan Kögl, <a href="https://creativecommons.org/licenses/by-sa/3.0/">CC BY-SA 3.0</a>. All via Wikimedia Commons.</p>
          <div className="foot-last">
            <p>sandboxd, built by <a className="link" href="https://github.com/swayam5342">swayam5342</a></p>
            <a className="btn btn-small" href="#top">Back to top</a>
          </div>
        </div>
      </footer>

      <Reveal />
    </>
  );
}
