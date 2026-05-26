# AWS-Only Non-App Runner Deployment Options (Target <= $10/month)

This document replaces prior non-AWS options with AWS-native alternatives.

## Summary

| Option | Estimated Cost | Budget Fit | Operational Overhead | Best For |
|---|---:|---|---|---|
| Lightsail Containers | ~$7/month | ✅ | Low | Managed AWS container hosting with minimal ops |
| Elastic Beanstalk (single instance) | ~$4–$8/month | ✅ | Medium | Balanced AWS experience and managed deployments |
| EC2 + Docker | ~$4–$9/month | ✅ | High | Maximum AWS learning and full control |
| Lambda + API Gateway | often <$5/month at low traffic | ✅ | Medium | Serverless AWS-first hosting with pay-per-request pricing |

## Recommendation

If your primary goal is AWS experience while staying under $10/month:

1. Start with **Lightsail Containers** for easiest migration and predictable cost.
2. Move to **Elastic Beanstalk** if you want broader AWS platform experience without full self-management.
3. Choose **EC2 + Docker** if hands-on operations experience is the top priority.
4. Choose **Lambda + API Gateway** when traffic is low/bursty and you want serverless patterns, accepting additional integration complexity and possible cold-start latency.

## Branch artifact coverage

- `docs/deployment/aws-lightsail-containers.md`
- `docs/deployment/aws-elastic-beanstalk.md`
- `docs/deployment/aws-ec2-docker.md`
- `docs/deployment/aws-lambda-api-gateway.md`
- `docs/deployment/aws-non-apprunner-options.md`
