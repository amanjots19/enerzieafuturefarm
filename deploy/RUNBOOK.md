# Deployment runbook

One VM, three services behind Caddy, MongoDB stays on Atlas.

| hostname | service | port |
|---|---|---|
| `enerzeiafuturefarm.com`, `www.` | storefront (Next.js) | 3100 |
| `admin.enerzeiafuturefarm.com` | admin console (Next.js) | 3001 |
| `api.enerzeiafuturefarm.com` | API (Go) | 8080 |

Nothing but Caddy listens on a public port. The three services bind to
localhost only.

---

## 0. The VM — Google Compute Engine

**e2-medium (2 vCPU, 4 GB), `asia-south1` (Mumbai), Ubuntu 24.04 LTS, 30 GB
balanced persistent disk.** Roughly $25–27/month.

- **4 GB, not 2.** Next.js builds are memory-hungry and there are two of them.
  A 2 GB box OOMs mid-build in a confusing way. RAM is the reason for this
  size, not CPU.
- **1 physical core is fine.** Nothing here is CPU-bound — the API waits on
  Atlas, the Next servers hand back pre-built pages. Builds take a few minutes
  rather than seconds; that is the only place you feel it.
- **Match your Atlas region.** Every request makes several database round
  trips, so cross-region latency lands on every page.
- **30 GB, balanced.** Not 10 GB: two `node_modules`, two `.next` builds, the
  Go toolchain and journal logs. Avoid Standard persistent disk — it is
  spinning rust and npm installs crawl.

Resizing later takes about two minutes (stop → change machine type → start),
same disk and same IP, so this is not a decision to agonise over.

### Three GCP-specific things that will bite

**1. Reserve a static external IP — before you touch DNS.**

New instances get an *ephemeral* IP that changes if the instance is ever
stopped and started. DNS would then point at nothing and Caddy could not renew
certificates.

**Promote the address the instance already has** — do not create a fresh one,
or the IP changes and you have to detach and reattach.

Console: VPC network → IP addresses → External IP addresses → find the row for
the instance, whose Type reads **Ephemeral** → change it to **Static**.

Or from **Cloud Shell or your laptop** — not from the VM, see below:

```bash
gcloud compute addresses create enerzia-ip \
  --addresses $(gcloud compute instances describe INSTANCE_NAME \
      --zone asia-south1-a \
      --format='get(networkInterfaces[0].accessConfigs[0].natIP)') \
  --region asia-south1
```

> **Do not run `gcloud` admin commands on the VM itself.** There, gcloud
> authenticates as the *instance's* service account, which carries read-only
> scopes by default, and every write fails with "Request had insufficient
> authentication scopes". That is not a broken setup — it is the instance
> correctly being unable to reconfigure the project it runs in. Use Cloud Shell
> or your own machine for anything that creates or changes GCP resources.

**2. GCP's VPC firewall sits in front of `ufw`.**

If HTTP/HTTPS is not allowed at the VPC level, packets never reach the box and
it looks like the server is down — `ufw status` will happily show the rules you
added, because it never sees the traffic. Tick **Allow HTTP traffic** and
**Allow HTTPS traffic** on the instance, or:

```bash
gcloud compute instances add-tags enerzia --tags http-server,https-server
```

Keep `ufw` as well. Two layers, and only one of them is the one blocking you.

**3. Egress is metered.** GCP bills per GB out where DigitalOcean bundles a
terabyte. At this volume it is a few dollars, but it is a line item that does
not exist elsewhere.

### Getting a shell

```bash
gcloud compute ssh enerzia --zone asia-south1-a
```

or the **SSH** button beside the instance in the console.

---

## 1. DNS — a migration, not a fresh setup

**Read this section fully before changing anything. Done in the wrong order it
takes down your email.**

### What is true today

- `enerzeiafuturefarm.com` is **live on Vercel**, serving the old static
  marketing site — `/shop`, `/farm` and `/contact` all 404, so it is not the
  Next.js app.
- **DNS is hosted at Vercel** (`ns1.vercel-dns.com`). GoDaddy is only the
  registrar.
- There is a **wildcard record**: every `*.enerzeiafuturefarm.com` resolves to
  Vercel, which is why `admin` and `api` already answer.

### The three records that carry your email

Losing any of these breaks `orders@` and `support@`. Write them down before
touching anything:

| type | host | value |
|---|---|---|
| MX | `@` | `1 smtp.google.com.` |
| TXT | `@` | `google-site-verification=CZI8x8jo-ijaOCUlX2Asjm97yMuxi851fdGgOelq5Ls` |
| TXT | `google._domainkey` | the DKIM key — copy the full value out of Vercel's DNS page, it is long |

There is no SPF and no DMARC record today (a deliberate decision, see
`tasks.md` 12.4).

> **Deleting the Vercel *deployment* is safe. Deleting the *domain* from Vercel
> is not** — the nameservers point there, so DNS stops resolving entirely and
> mail stops with it. Move DNS first, delete last.

### Move DNS off Vercel

Two reasonable destinations. **GoDaddy is the simpler choice and what this
runbook assumes** — you already own the account, since the domain is registered
there.

| | GoDaddy | Cloudflare |
|---|---|---|
| cost | free with the domain | free |
| accounts to manage | one you already have | one more |
| resolution speed | fine | measurably faster |
| free CDN later | no | yes — but see the proxy warning below |

For four A records, an MX and two TXTs the practical difference is marginal.
Cloudflare's CDN is a real future benefit for a shared-core VM serving India,
but it cannot be switched on immediately anyway, so it is not a reason to add
an account today.

**GoDaddy — My Products → the domain → DNS**

1. **Nameservers → Change → "I'll use GoDaddy nameservers"** (or Default)
2. **DNS → Records**: recreate the three email records from the table above
   **first**, and confirm them with `dig` before going further
3. Add the four A records below
4. Do **not** recreate the wildcard

**Cloudflare — if you prefer it**

1. Add a site → `enerzeiafuturefarm.com` → Free plan
2. It imports existing records automatically. **Verify the DKIM value by
   hand** — it is long and is the one most likely to be truncated
3. Add the four A records below, delete the wildcard
4. Replace the Vercel nameservers with Cloudflare's, in GoDaddy →
   Nameservers → Change

Either way: **recreate and verify the email records before switching
nameservers.** The switch is what makes the new provider authoritative, and
anything missing at that moment is simply gone.

### The four A records

| type | host | value | proxy |
|---|---|---|---|
| A | `@` | *VM static IP* | **DNS only** |
| A | `www` | *VM static IP* | **DNS only** |
| A | `admin` | *VM static IP* | **DNS only** |
| A | `api` | *VM static IP* | **DNS only** |

The proxy column applies to Cloudflare only; GoDaddy has no equivalent.

**On Cloudflare, set every record to "DNS only" — the grey cloud, not the
orange one.** With proxying enabled Cloudflare terminates TLS itself, Caddy's
HTTP-01 challenge has nothing to answer, and issuance fails with errors that
point nowhere near the real cause. Proxying can be switched on later,
deliberately, once the site is up.

### Cut over in this order

The apex is serving a live site right now. Do not point it at a VM that has not
been proven.

1. Point **`api`** and **`admin`** at the VM first. Neither is in use, so
   nothing breaks if something is wrong.
2. Work through the rest of this runbook and verify both fully.
3. Only then point **`@`** and **`www`** at the VM.
4. Once the storefront is confirmed live on the VM, delete the Vercel project.

### Verify before continuing

```bash
dig +short A api.enerzeiafuturefarm.com admin.enerzeiafuturefarm.com
# ... and after the final cutover:
dig +short A enerzeiafuturefarm.com www.enerzeiafuturefarm.com

# Email must still resolve at every stage — check after EVERY DNS change:
dig +short MX enerzeiafuturefarm.com
dig +short TXT google._domainkey.enerzeiafuturefarm.com
```

The MX check is not optional. If it ever returns empty, stop and fix DNS before
doing anything else — mail is already down at that point.

---

## 2. Base setup on the VM

> If you paste this into a file to run it, **run it with `bash`, not `sh`**.
> Ubuntu's `/bin/sh` is dash, which lacks several things used here.

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl git ufw

# Node 22 LTS — Next.js 16 needs 18.18+, and 22 is the current LTS
curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
sudo apt install -y nodejs

# Go 1.25.5, matching go.mod
curl -fsSL https://go.dev/dl/go1.25.5.linux-amd64.tar.gz -o /tmp/go.tgz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tgz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
# `.` not `source` — source is a bashism and silently fails under sh/dash,
# leaving Go off PATH and producing "go: not found" further down.
. /etc/profile.d/go.sh

# Caddy
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
  | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
  | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update && sudo apt install -y caddy

node --version && go version && caddy version
```

**Firewall.** Only 22, 80 and 443 are ever reached from outside. This is the
second layer — the GCP VPC rule from section 0 is the one that actually gates
inbound traffic:

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80,443/tcp
sudo ufw --force enable
```

---

## 3. Service account and checkout

```bash
sudo useradd --system --create-home --home-dir /srv/enerzia --shell /bin/bash enerzia

# useradd creates the home at 0700, so your own account cannot even cd into it
# — every later command would fail with "Permission denied". The checkout holds
# only public repository code; the secrets live in /etc/enerzia at 0750.
sudo chmod 755 /srv/enerzia

sudo -u enerzia git clone https://github.com/amanjots19/enerzieafuturefarm.git /srv/enerzia/current
```

A dedicated unprivileged user, because the units set `ProtectSystem=strict` and
`NoNewPrivileges` — none of that means anything if the process is root.

---

## 4. Secrets

```bash
# Group enerzia, so the service user can TRAVERSE the directory. Without this
# it cannot read anything inside, however permissive the files are — and the
# failure is silent.
sudo install -d -m 0750 -o root -g enerzia /etc/enerzia
sudo install -m 0600 /srv/enerzia/current/deploy/api.env.example /etc/enerzia/api.env
sudo nano /etc/enerzia/api.env
```

Fill in every value. Notes on the ones that catch people:

- **`ALLOWED_ORIGINS`** — already filled with the three production origins.
  `https`, no trailing slashes. Getting this wrong produces a 200 with no CORS
  header, which looks exactly like a dead backend and has already cost a full
  debugging round on this project.
- **`ADMIN_PASSWORD_HASH`** — generate with `make admin-password` in
  `enerzia-be/`. It contains `$`, so keep the quotes.
- **`JWT_SECRET`** — `openssl rand -base64 48`. Changing it later signs out
  every shopper and invalidates every admin session.
- **Razorpay keys must be LIVE**, not `rzp_test_`. Paste them here directly;
  they should never pass through a chat or a commit.
- **Atlas** — add the VM's public IP under Network Access, or every request
  hangs at connect.

The API refuses to boot in production if Razorpay, MSG91, admin or Cloudinary
credentials are missing. That is deliberate: a half-configured production
server is worse than one that will not start.

---

## 4b. Build-time values for the storefront

Separate from `api.env`, and easy to miss: the sign-in widget needs two
`NEXT_PUBLIC_` values, and they are inlined by `next build`. Setting them in
systemd does nothing — by then the bundle already contains `undefined`, and the
only symptom is that sign-in silently never sends an OTP.

```bash
sudo install -m 0640 -o root -g enerzia \
  /srv/enerzia/current/deploy/build.env.example /etc/enerzia/build.env
sudo nano /etc/enerzia/build.env
```

Check both files from the service user, since `deploy.sh` runs as it:

```bash
sudo -u enerzia cat /etc/enerzia/build.env   # prints
sudo -u enerzia cat /etc/enerzia/api.env     # Permission denied
```

**Both results are correct.** `build.env` holds values that ship to every
browser anyway; `api.env` holds real secrets and stays `0600 root:root`, which
systemd reads as root before dropping privileges.

Get both from the MSG91 dashboard under OTP → Widget. They are not secrets —
anything `NEXT_PUBLIC_` ships to every browser — but they belong in a file
rather than a commit. Group-readable by `enerzia`, because `deploy.sh` runs as
that user and needs them at build time.

`deploy.sh` refuses to build without them, rather than producing a bundle where
sign-in quietly does not work.

> Adding the VM IP to MSG91 is also needed, but for a different thing: the
> **server-side** token verification. The widget half is these two values.

---

## 5. Install units and Caddy config

Absolute paths, so this works regardless of which directory you are in or what
your own account can traverse:

```bash
sudo cp /srv/enerzia/current/deploy/enerzia-*.service /etc/systemd/system/
sudo cp /srv/enerzia/current/deploy/Caddyfile /etc/caddy/Caddyfile
sudo mkdir -p /var/log/caddy && sudo chown caddy:caddy /var/log/caddy
sudo systemctl daemon-reload
sudo systemctl enable enerzia-api enerzia-shop enerzia-admin
```

`deploy.sh` restarts services with `sudo`, so let the service user do exactly
that and nothing more:

```bash
echo 'enerzia ALL=(root) NOPASSWD: /bin/systemctl restart enerzia-api, /bin/systemctl restart enerzia-shop, /bin/systemctl restart enerzia-admin' \
  | sudo tee /etc/sudoers.d/enerzia
sudo chmod 0440 /etc/sudoers.d/enerzia
sudo visudo -c
```

---

## 6. First deploy

```bash
sudo -u enerzia /srv/enerzia/current/deploy/deploy.sh
sudo systemctl reload caddy
```

The script builds all three, then restarts only if every build succeeded — a
compile error leaves the running site untouched.

Watch the first certificate issuance:

```bash
sudo journalctl -u caddy -f
```

---

## 7. Verify

```bash
curl -sI https://enerzeiafuturefarm.com | head -1
curl -sI https://admin.enerzeiafuturefarm.com | head -1
curl -s  https://api.enerzeiafuturefarm.com/health
```

Then in a browser:

- the storefront loads and **products appear** — if the grid is empty with
  "Unable to reach the server", it is `ALLOWED_ORIGINS` or
  `NEXT_PUBLIC_API_BASE_URL`, not the backend
- `admin.enerzeiafuturefarm.com` shows the login and you can sign in
- the order book lists orders

Check the API is talking to Atlas:

```bash
curl -s https://api.enerzeiafuturefarm.com/api/v1/products | head -c 200
```

---

## 8. Razorpay webhook — the point of deploying

This is the first environment where Razorpay can reach us. Until now no webhook
has ever been delivered, which is why **no order has a recorded payment
method**.

In the Razorpay dashboard → Settings → Webhooks → Add:

- **URL**: `https://api.enerzeiafuturefarm.com/webhooks/razorpay`
- **Secret**: generate one, put the same value in `RAZORPAY_WEBHOOK_SECRET`
- **Events**: `payment.captured`, `payment.failed`, `order.paid`

Restart the API after setting the secret.

**Then place one real order** (Razorpay's minimum is ₹1) and check:

```bash
sudo journalctl -u enerzia-api | grep -i "payment detail filled"
```

Then open the order in the admin console and confirm `payment.method` is no
longer null. That verifies task 11.11, which cannot be tested any other way.
Refund the ₹1 by hand in the dashboard afterwards — refunds are manual.

---

## 9. Before real customers

- **Purge test data from Atlas** (task 7.11): test users including
  `9000000001`, throwaway addresses, and the smoke-test orders. Two orders have
  had their fulfilment set to `shipped` during verification.
- **Whitelist the production domain in MSG91**, or sign-in fails on the live
  site while working locally.
- Confirm `APP_ENV=production` really is set — it also switches the admin order
  book to showing paid orders only.

---

## Day-to-day

```bash
# deploy the latest main
sudo -u enerzia git -C /srv/enerzia/current pull
sudo -u enerzia /srv/enerzia/current/deploy/deploy.sh

# logs
sudo journalctl -u enerzia-api -f
sudo journalctl -u enerzia-shop -n 100

# status
systemctl status enerzia-api enerzia-shop enerzia-admin caddy
```

## When something is wrong

| symptom | look here first |
|---|---|
| site loads, no products, "Unable to reach the server" | `ALLOWED_ORIGINS`, then `NEXT_PUBLIC_API_BASE_URL` — it is baked in at build time, so it needs a rebuild, not a restart |
| API will not start | `journalctl -u enerzia-api -n 50` — production config validation names every missing variable at once |
| every API call hangs | Atlas Network Access is missing the VM's IP |
| certificates not issuing | a hostname does not resolve to this VM yet; `journalctl -u caddy` |
| orders place but no payment method | the webhook is not configured, or its secret does not match |
| no confirmation emails | `journalctl -u enerzia-api \| grep confirmation` — SMTP failures are logged and deliberately never fail the payment |
