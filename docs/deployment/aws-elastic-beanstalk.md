# AWS Option: Elastic Beanstalk (Single EC2 Instance)

This option replaces App Runner with **AWS Elastic Beanstalk** using the existing Dockerfile.

## Estimated Monthly Cost

- Single `t4g.nano` + storage (no load balancer): **~$4–$8/month**
- Expected budget fit: **Yes (under $10/month)**

## Why choose it

- AWS-managed deployment workflow while retaining EC2-level control.
- Works with the current Dockerized app model.
- Strong option for learning broader AWS operations.

## Migration summary

1. Create an Elastic Beanstalk Docker environment configured as single-instance.
2. Deploy from the existing Dockerfile.
3. Configure runtime variables/secrets:
   - `LPS_SESSION_KEY`
   - `CLIENT_ID_KEY`
   - `CLIENT_SECRET_KEY`
   - `GOOGLE_CONNECTION_TABLE_NAME`
4. Point Cloudflare DNS to the Beanstalk environment endpoint.

## Tradeoffs

- More operational responsibility than Lightsail/App Runner.
- High availability features (load balancer, multi-AZ) typically exceed $10/month.
