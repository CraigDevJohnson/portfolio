# AWS Option: Lambda + API Gateway (Serverless HTTP)

This option runs the Go web app on **AWS Lambda** behind **API Gateway**.

## Estimated Monthly Cost

- Low traffic (personal site level): **often <$5/month**
- Expected budget fit: **Yes (under $10/month), depending on request volume and duration**

## Why choose it

- Fully AWS-native and strongly aligned with AWS learning goals.
- Pay-per-request pricing can be very cost efficient for bursty or low-traffic usage.
- No server patching or EC2/container host management.

## Why this was not previously the default recommendation

- This app is a server-rendered Go HTTP service; Lambda usually needs an adapter pattern (for example a Lambda HTTP adapter) rather than running as a plain long-lived web server.
- You typically manage extra integration choices (API Gateway HTTP API config, binary/static asset handling, and often CloudFront/S3 fronting), which can increase setup complexity.
- Cold starts can impact first-hit latency compared with always-on container or instance options.

## Migration summary

1. Package the Go app for Lambda (zip or container image flow).
2. Expose it through API Gateway HTTP API.
3. Configure runtime variables/secrets:
   - `LPS_SESSION_KEY`
   - `CLIENT_ID_KEY`
   - `CLIENT_SECRET_KEY`
   - `GOOGLE_CONNECTION_TABLE_NAME`
4. Configure domain and TLS (Cloudflare + API Gateway custom domain, optionally CloudFront).

## Tradeoffs

- Best cost efficiency for low/variable traffic.
- More integration complexity than Lightsail or single-instance Beanstalk for this app shape.
- Requires careful tuning for latency-sensitive first requests.
