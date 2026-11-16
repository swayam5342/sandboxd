# sandboxd site

The landing page for sandboxd: Next.js, exported as static files for GitHub Pages.

```bash
cd site
npm ci
npm run dev        # http://localhost:3000
npm run build      # static export into out/
```

## Deploying

`.github/workflows/pages.yml` builds `site/` and publishes `site/out` on every push to `main`
that touches `site/`. It sets `PAGES_BASE_PATH` from the Pages config, so the site works under
`https://<user>.github.io/sandboxd/`.

One-time setup: in the repository's **Settings → Pages**, set **Source** to **GitHub Actions**.

## Where things live

- `app/page.tsx`: the page; `app/not-found.tsx`: the 404
- `lib/data.ts`: escape attempts and the language entries (taken from `config/lang.yaml`)
- `components/`: the interactive parts (console, languages, cells, request composer)
- `public/assets/`: photographs; credits in `public/assets/credits.json` and the footer

Figures in the escape-attempt responses (`duration_ms`, `memory_peak_kb`) are typical values,
not recorded runs.
