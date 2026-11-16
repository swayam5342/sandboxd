"use client";

import { useMemo, useState } from "react";

const defaults = {
  py3: { source: "print(input()[::-1])", stdin: "nobody", expect: "ydobon" },
  c: { source: '#include <stdio.h>\nint main(void) { int n; scanf("%d", &n); printf("%d\\n", n * 2); }', stdin: "21", expect: "42" },
} as const;

type L = keyof typeof defaults;

export default function Composer() {
  const [lang, setLang] = useState<L>("py3");
  const [source, setSource] = useState<string>(defaults.py3.source);
  const [stdin, setStdin] = useState<string>(defaults.py3.stdin);
  const [expect, setExpect] = useState<string>(defaults.py3.expect);
  const [msg, setMsg] = useState("");

  const curl = useMemo(() => {
    const body = JSON.stringify({ language: lang, source, tests: [{ stdin, expected_stdout: expect + "\n" }] });
    return `curl -X POST http://localhost:8089/run \\\n  -H "Content-Type: application/json" \\\n  -d '${body.replace(/'/g, "'\\''")}'`;
  }, [lang, source, stdin, expect]);

  const choose = (l: L) => {
    setLang(l);
    setSource(defaults[l].source);
    setStdin(defaults[l].stdin);
    setExpect(defaults[l].expect);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await navigator.clipboard.writeText(curl);
      setMsg("Copied. Paste it into a terminal while sandboxd is running.");
    } catch {
      setMsg("Copying was blocked. Select the command above and copy it.");
    }
  };

  return (
    <form className="dialog" data-ui onSubmit={submit} aria-labelledby="dlg-title">
      <p className="dlg-title" id="dlg-title">Your first run</p>
      <fieldset className="seg-choice">
        <legend className="sr">Language</legend>
        <label><input type="radio" name="lang" value="py3" checked={lang === "py3"} onChange={() => choose("py3")} /><span>Python 3</span></label>
        <label><input type="radio" name="lang" value="c" checked={lang === "c"} onChange={() => choose("c")} /><span>C</span></label>
      </fieldset>
      <label className="fld"><span>Source</span><textarea rows={3} spellCheck={false} value={source} onChange={(e) => setSource(e.target.value)} /></label>
      <label className="fld"><span>stdin</span><input spellCheck={false} value={stdin} onChange={(e) => setStdin(e.target.value)} /></label>
      <label className="fld"><span>Expected stdout</span><input spellCheck={false} value={expect} onChange={(e) => setExpect(e.target.value)} /></label>
      <pre className="curl"><code>{curl}</code></pre>
      <button className="dlg-btn" type="submit">Copy the request</button>
      <p className="dlg-msg" aria-live="polite">{msg}</p>
    </form>
  );
}
