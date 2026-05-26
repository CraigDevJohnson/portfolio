# Fly.io Deployment Option

This branch demonstrates replacing AWS App Runner with Fly.io while keeping the existing Docker-based deployment model.

## Expected Monthly Cost

- Typical low-traffic footprint target: **~$3–$8/month**
- Budget fit: **within the $10/month cap**

## Why this option

- Keeps the same Dockerfile-first flow.
- Supports scale-to-zero style behavior for low traffic.
- Custom domains and TLS are built in.

## Migration Summary

1. Install and authenticate Fly CLI.
2. Create the app from this repository using `fly.toml`.
3. Set required runtime secrets:
   - `LPS_SESSION_KEY`
   - `CLIENT_ID_KEY`
   - `CLIENT_SECRET_KEY`
   - `GOOGLE_CONNECTION_TABLE_NAME`
4. Deploy with `fly deploy`.
5. Attach your custom domain and verify TLS.

## Notes

- This option keeps your app containerized and minimizes code changes.
- AWS DynamoDB dependencies remain supported as long as credentials/network access are configured.
