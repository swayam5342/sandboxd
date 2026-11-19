// The one site header, shared by the home page and the 404. `base` is the path the
// in-page anchors hang off: "" on the home page, "<basePath>/" anywhere else.
const links = [
  { href: "#try", label: "Try it" },
  { href: "#languages", label: "Languages" },
  { href: "#walls", label: "Walls" },
  { href: "#start", label: "API" },
];

export default function SiteHeader({ base = "" }: { base?: string }) {
  return (
    <header className="nav wrap">
      <a className="wordmark" href={`${base}#top`} aria-label="sandboxd, back to top">
        <svg className="cell-dot" viewBox="10 10 44 44" aria-hidden="true"><path d="M28 10H54V54H10V28H28Z" fill="#4cb3f0" /><rect x="13" y="13" width="11" height="11" fill="#f2f2f2" /></svg>
        sandboxd
      </a>
      <nav aria-label="Main">
        <ul>
          {links.map((l) => (
            <li key={l.href}><a className="link" href={`${base}${l.href}`}>{l.label}</a></li>
          ))}
        </ul>
      </nav>
    </header>
  );
}
