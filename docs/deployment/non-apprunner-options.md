# Non-App Runner Deployment Options (<= $10/month)

This document compares practical replacement options for AWS App Runner for this portfolio app.

## Summary

| Option | Estimated Cost | Budget Fit | Notes |
|---|---:|---|---|
| Fly.io | ~$3–$8/month | ✅ | Lowest expected cost for low traffic; Docker-native |
| Render Starter | $7/month | ✅ | Most predictable fixed monthly pricing |
| Railway | ~$5–$10/month | ⚠️ | Can stay in budget at low traffic; monitor usage |

## Repository Artifacts

- Fly.io: `fly.toml` and `docs/deployment/fly-io.md`
- Render: `render.yaml` and `docs/deployment/render.md`
- Railway: `railway.json` and `docs/deployment/railway.md`

## Recommendation

Start with **Render** if your top priority is predictable monthly cost and easiest migration.
Choose **Fly.io** if your top priority is minimizing monthly cost with low traffic.
Use **Railway** if you want the fastest developer UX and are comfortable watching monthly usage.
