# AWS Option: EC2 + Docker (Self-Managed)

This option replaces App Runner with a self-managed EC2 instance running the current Docker image.

## Estimated Monthly Cost

- `t4g.nano` or `t4g.micro` + storage: **~$4–$9/month**
- Expected budget fit: **Yes (under $10/month)**

## Why choose it

- Maximum AWS hands-on experience (networking, OS, runtime, deployment).
- Lowest fixed-cost AWS path for always-on hosting.
- Keeps existing containerized deployment approach.

## Migration summary

1. Provision EC2 (Amazon Linux) and install Docker.
2. Push image to ECR and run container on EC2 (`-p 8080:8080`).
3. Configure environment variables/secrets:
   - `LPS_SESSION_KEY`
   - `CLIENT_ID_KEY`
   - `CLIENT_SECRET_KEY`
   - `GOOGLE_CONNECTION_TABLE_NAME`
4. Use Cloudflare and origin TLS configuration for your domain.

## Tradeoffs

- Most operational overhead (patching, backups, restarts, monitoring).
- You own reliability and deployment automation.
