# BEON-IPQuality Railway Deployment Guide

## Quick Start

### 1. Fork or Clone Repository
```bash
git clone https://github.com/your-repo/BEON-IPQuality.git
```

### 2. Create Railway Project
1. Go to [Railway](https://railway.app)
2. Click "New Project"
3. Select "Deploy from GitHub repo"
4. Choose your BEON-IPQuality repository

### 3. Add Required Services

#### PostgreSQL Database
1. Click "Add Service" → "Database" → "PostgreSQL"
2. Railway will auto-configure the DATABASE_URL

#### Redis Cache
1. Click "Add Service" → "Database" → "Redis"
2. Railway will auto-configure the REDIS_URL

### 4. Configure Environment Variables

In your API service settings, add:

```env
# Required
API_HOST=0.0.0.0
API_PORT=8080
API_KEY=your-secure-api-key-minimum-32-chars

# Paths (these work with the Dockerfile)
MMDB_PATH=/app/data/mmdb/ipquality.mmdb
GEOIP_CITY_PATH=/app/data/geoip/GeoLite2-City.mmdb
GEOIP_ASN_PATH=/app/data/geoip/GeoLite2-ASN.mmdb

# MaxMind (for GeoIP updates)
MAXMIND_LICENSE_KEY=your-maxmind-key
```

### 5. Configure Volumes

For persistent MMDB storage:
1. In your service settings, add a volume mount
2. Mount path: `/app/data`
3. This persists your MMDB files across deployments

### 6. Initial Data Setup

After first deployment, run the ingestor and compiler:

```bash
# Using Railway CLI
railway run ./bin/ingestor
railway run ./bin/compiler
```

Or create a one-time Railway service to run these commands.

### 7. Configure Custom Domain (Optional)
1. Go to service settings → "Domains"
2. Add your custom domain
3. Update DNS to point to Railway

## Architecture on Railway

```
┌─────────────────────────────────────────────────────┐
│                    Railway                           │
│  ┌─────────────────────────────────────────────────┐ │
│  │   API Service (BEON-IPQuality)                   │ │
│  │   - Port 8080 (public)                           │ │
│  │   - Port 9100 (metrics, internal)                │ │
│  │   - Volume: /app/data (MMDB files)               │ │
│  └──────────────────────┬──────────────────────────┘ │
│                         │                             │
│  ┌──────────────────────┴──────────────────────────┐ │
│  │                Internal Network                   │ │
│  │  ┌─────────────┐  ┌─────────────┐               │ │
│  │  │  PostgreSQL │  │    Redis    │               │ │
│  │  │  (private)  │  │  (private)  │               │ │
│  │  └─────────────┘  └─────────────┘               │ │
│  └──────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

## Scaling Options

### Horizontal Scaling
Update `railway.json`:
```json
{
  "deploy": {
    "numReplicas": 3
  }
}
```

### Vertical Scaling
In Railway dashboard:
- Starter: 0.5 vCPU, 512MB RAM
- Pro: Up to 32 vCPU, 32GB RAM

## Scheduled Jobs (Cron)

For automated threat feed updates, create a Railway Cron job:

1. Create a new service
2. Set as "Cron" type
3. Schedule: `0 */4 * * *` (every 4 hours)
4. Command: `./bin/ingestor && ./bin/compiler`

## Monitoring

### Health Check
The API exposes `/health` endpoint that Railway monitors automatically.

### Prometheus Metrics
Metrics available at `http://your-service:9100/metrics`

Configure Grafana Cloud or Railway's built-in monitoring.

## Cost Estimation

| Resource | Railway Plan | Estimated Cost/Month |
|----------|--------------|---------------------|
| API (1 replica) | Hobby | $5-20 |
| PostgreSQL | Hobby | $5-10 |
| Redis | Hobby | $5-10 |
| Total | | $15-40 |

For production with Pro plan: $50-150/month depending on traffic.

## Troubleshooting

### Service won't start
- Check logs: `railway logs`
- Verify environment variables are set
- Ensure MMDB files exist (run ingestor/compiler first)

### Slow performance
- Add more replicas
- Check Redis connection
- Consider upgrading to Pro plan for more resources

### Database connection issues
- Verify DATABASE_URL is correct
- Check if PostgreSQL service is running
- Railway auto-reconnects, but check logs for errors
