# Railway Deployment Option

This branch demonstrates replacing AWS App Runner with Railway using Docker deployment.

## Expected Monthly Cost

- Typical low-traffic usage: **~$5–$10/month**
- Budget fit: **can fit the $10 cap if usage remains low**

## Why this option

- Fast setup with Dockerfile-based deploys.
- Built-in managed TLS and custom domains.
- Good DX for iterative app deployments.

## Migration Summary

1. Create a Railway project linked to this repository.
2. Use `railway.json` to keep deployment behavior explicit.
3. Set runtime variables/secrets:
   - `APP_BIND_ALL=true`
   - `LPS_SESSION_KEY`
   - `CLIENT_ID_KEY`
   - `CLIENT_SECRET_KEY`
   - `GOOGLE_CONNECTION_TABLE_NAME`
4. Deploy and validate root health check.
