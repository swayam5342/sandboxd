# sandboxd

**Run code you don't trust.** sandboxd is a Go daemon that compiles and runs untrusted code
inside [NsJail](https://github.com/google/nsjail): each submission runs as user `nobody`
(uid 65534), with no network, a read-only system, and hard limits on time, memory and
processes, behind a JSON API.

- Website: https://sandboxd.swayam.li/
- Source and docs: https://github.com/swayam5342/sandboxd

## Tags

| Tag | What's inside |
| :--- | :--- |
| `core` | sandboxd + NsJail, with **bash** as the only language (~145 MB) |
| `core-<commit>` | the same image pinned to one commit, e.g. `core-05506f8`. Use this in production. |

This is a **core** image. You add the language toolchains you need on top of it, so an image
that only runs Python doesn't carry a JDK and a Rust compiler.

## Run it

nsjail needs these capabilities to build a jail. `--privileged` is not required.

```bash
docker run -d --name sandboxd -p 8089:8089 \
  --cap-drop ALL \
  --cap-add SYS_ADMIN --cap-add SYS_CHROOT --cap-add SYS_RESOURCE --cap-add SYS_PTRACE \
  --cap-add NET_ADMIN --cap-add SETUID --cap-add SETGID --cap-add SETPCAP \
  --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add KILL \
  --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  -e API_KEY=change-me \
  swayam5342/sandboxd:core
```

```bash
curl localhost:8089/healthz
# {"status":"ok"}

curl -X POST localhost:8089/run \
  -H "Authorization: Bearer change-me" -H "Content-Type: application/json" \
  -d '{"language":"bash","source":"read n; echo $((n*2))","tests":[{"stdin":"21","expected_stdout":"42\n"}]}'
```

## Add your languages

Toolchains must live inside the image: every jail bind-mounts the container's own `/usr`,
`/lib` and `/bin` read-only. Install them on top of core and replace the registry at
`/app/config/lang.yaml`:

```dockerfile
FROM swayam5342/sandboxd:core
RUN apt-get update && apt-get install -y --no-install-recommends python3 \
    && rm -rf /var/lib/apt/lists/*
COPY lang.yaml /app/config/lang.yaml
```

sandboxd checks every language in `lang.yaml` at startup and exits if one is missing.
The registry schema is in [docs/languages.md](https://github.com/swayam5342/sandboxd/blob/main/docs/languages.md).

**All eight languages** (Python, Bash, Node.js, C, C++, Java, Verilog, Rust):
[`docker/full.Dockerfile`](https://github.com/swayam5342/sandboxd/blob/main/docker/full.Dockerfile)
pulls this image and installs them all. In the repo, `docker compose up --build` builds it.

## Configuration

| Variable | Default | |
| :--- | :--- | :--- |
| `PORT` | `:8089` | listen address |
| `API_KEY` | empty | when set, `POST /run` needs `Authorization: Bearer <API_KEY>` |
| `MAX_CONCURRENT` | `100` | jobs running at once; the rest wait |
| `LANG_CONFIG` | `config/lang.yaml` | language registry |
| `LOG_LEVEL` / `LOG_JSON` | `info` / `true` | logging |

`GET /info` reports the image's version and commit under `build_info`.

## Routes

`GET /healthz`, `GET /readyz`, `GET /info`, `POST /run`. Full reference:
[docs/api.md](https://github.com/swayam5342/sandboxd/blob/main/docs/api.md).
