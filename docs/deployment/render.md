# Render Deployment Option

This branch demonstrates replacing AWS App Runner with Render using Docker deployment.

## Expected Monthly Cost

- Render Starter Web Service: **$7/month**
- Budget fit: **within the $10/month cap**

## Why this option

- Fixed predictable price close to your current spend.
- Managed HTTPS and custom domain support.
- Simple Docker deployment with minimal operational overhead.

## Migration Summary

1. Create a Render Web Service from this repository.
2. Use `render.yaml` for service configuration.
3. Configure runtime secrets:
   - `LPS_SESSION_KEY`
   - `CLIENT_ID_KEY`
   - `CLIENT_SECRET_KEY`
   - `GOOGLE_CONNECTION_TABLE_NAME`
4. Deploy from `main` and verify health check at `/`.
