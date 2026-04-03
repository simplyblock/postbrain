# Monitoring and Alerting

Postbrain exposes Prometheus metrics at `/metrics`.

## Minimum monitoring set

Start with a small set that maps directly to reliability outcomes:

- Server process up/restart status
- Request latency/error rate
- DB connectivity/availability
- Background job success/failure

## Suggested alerts

Avoid alert noise. Start with alerts that indicate real service impact:

- service unavailable / repeated restart
- sustained high error rate
- DB connection failures
- certificate expiration risk (if managed externally)

## Health checks

Use both passive metrics and active checks:

- HTTP endpoint availability (`/metrics`, API paths)
- token-authenticated read/write smoke checks in automation

## Kubernetes signals

If you deploy on Kubernetes, include platform-specific readiness signals:

- pod restarts
- rollout failures
- Gateway/Ingress programming status (if using Gateway API/Ingress)
