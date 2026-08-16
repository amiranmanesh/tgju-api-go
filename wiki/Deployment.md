# Deployment

The service is one static binary with no state, no database and no disk. Anything that can
run a container can run it.

## Docker

```bash
docker run -d --name tgju \
  -p 8080:8080 \
  -e TGJU_CACHE_TTL=30s \
  -e TGJU_LOG_JSON=true \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  ghcr.io/amiranmanesh/tgju-api-go:latest
```

The image is `scratch` plus a CA bundle and two binaries. It runs as uid 65532, has no
shell, and needs no writable filesystem. Its healthcheck is a compiled probe rather than
a curl one-liner, for the same reason.

Tags: `latest`, `1`, `1.0`, `1.0.0`. Pin at least the minor in production.

## Compose

The [`docker-compose.yml`](https://github.com/amiranmanesh/tgju-api-go/blob/main/docker-compose.yml)
in the repository is a working file with every setting spelled out and commented.

```bash
docker compose up -d
curl localhost:8080/healthz
```

## Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tgju-api
spec:
  replicas: 2
  selector:
    matchLabels: { app: tgju-api }
  template:
    metadata:
      labels: { app: tgju-api }
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        seccompProfile: { type: RuntimeDefault }
      containers:
        - name: api
          image: ghcr.io/amiranmanesh/tgju-api-go:1.0.0
          ports:
            - { name: http, containerPort: 8080 }
          env:
            - { name: TGJU_CACHE_TTL, value: "60s" }
            - { name: TGJU_LOG_JSON,  value: "true" }
            # Each replica keeps its own cache, so the rate the site sees is the
            # limit times the replica count. Raise the TTL rather than the count.
            - { name: TGJU_RATE_LIMIT, value: "40" }
          livenessProbe:
            httpGet: { path: /healthz, port: http }
            periodSeconds: 30
          readinessProbe:
            httpGet: { path: /readyz, port: http }
            periodSeconds: 15
            failureThreshold: 3
          resources:
            requests: { cpu: 25m, memory: 32Mi }
            limits:   { memory: 128Mi }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities: { drop: [ALL] }
```

Note the probe split: `/healthz` never touches tgju, so an outage upstream takes the pod
out of the load balancer without restarting it. Restarting fixes nothing when the problem
is somebody else's website.

## Prometheus

```yaml
scrape_configs:
  - job_name: tgju-api
    metrics_path: /metrics
    static_configs:
      - targets: ["tgju-api:8080"]
```

| Metric | Type | Labels |
| --- | --- | --- |
| `tgju_http_requests_total` | counter | `route`, `method`, `status` |
| `tgju_http_request_duration_seconds` | histogram | `route`, `method`, `status` |
| `tgju_uptime_seconds` | gauge | — |
| `tgju_build_info` | gauge | `version` |

`route` is the pattern (`/v1/markets/{market}`), never the concrete path, so the
cardinality does not grow with the number of instruments.

Two alerts worth having:

```yaml
- alert: TgjuMarkupChanged
  # The scraper is out of date. Retrying will not help; this needs a release.
  expr: |
    sum(rate(tgju_http_requests_total{status="502"}[10m])) > 0
    and on() absent(up{job="tgju-api"} == 0)
  for: 15m

- alert: TgjuUpstreamSlow
  expr: |
    histogram_quantile(0.95,
      sum by (le) (rate(tgju_http_request_duration_seconds_bucket[5m]))) > 5
  for: 10m
```

To tell the two 502 codes apart, alert on the log stream instead: the `code` field is
`upstream_changed` or `upstream_unavailable`.

## Behind a reverse proxy

- Forward `X-Forwarded-For`; the rate limiter reads its left-most entry, and without it
  every client behind the proxy shares one bucket.
- Responses carry `Cache-Control: public, max-age=<remaining TTL>`. Let the proxy honour
  it and the load on tgju drops again.
- Terminate TLS at the proxy. The service speaks plain HTTP on purpose.

## Sizing

A replica is idle almost all the time: it holds three snapshots in memory and answers from
them. 32 MiB and a fraction of a core is plenty. **Scale for your own traffic, not for
tgju's** — the cache means the requests going upstream do not grow with the requests
coming in.

## Behaving well

This service sits between your users and a site that did not ask to be scraped.

- Keep the cache on. Thirty seconds is already enough to turn a thousand requests a minute
  into two upstream.
- Raise the TTL before adding replicas: each replica has its own cache.
- Leave the default User-Agent, or set one that names you. An operator reading their logs
  should be able to work out who you are.
- Read tgju's terms before putting this in front of paying users.
