# AWS Option: Lightsail Containers

This option replaces App Runner with **AWS Lightsail Containers** while keeping Docker-based deployment.

## Estimated Monthly Cost

- Lightsail container service (nano): **~$7/month**
- Expected budget fit: **Yes (under $10/month)**

## Why choose it

- AWS-native managed container platform.
- Simpler than ECS and usually cheaper for low-traffic single-service workloads.
- Supports custom domains and HTTPS.

## Migration summary

1. Create a Lightsail container service.
2. Build and push the existing Docker image to Lightsail.
3. Set runtime variables/secrets:
   - `LPS_SESSION_KEY`
   - `CLIENT_ID_KEY`
   - `CLIENT_SECRET_KEY`
   - `GOOGLE_CONNECTION_TABLE_NAME`
4. Attach custom domain and validate TLS.

## Tradeoffs

- Fewer advanced controls than ECS.
- Different deployment workflow than current OpenTofu/App Runner pipeline.
