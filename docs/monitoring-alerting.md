# Monitoring and Alerting

Postbrain exposes Prometheus metrics at `/metrics`.

## Minimum monitoring set

- Server process up/restart status
- Request latency/error rate
- DB connectivity/availability
- Background job success/failure

## Suggested alerts

- service unavailable / repeated restart
- sustained high error rate
- DB connection failures
- certificate expiration risk (if managed externally)

## Health checks

- HTTP endpoint availability (`/metrics`, API paths)
- token-authenticated read/write smoke checks in automation

## Kubernetes signals

- pod restarts
- rollout failures
- Gateway/Ingress programming status (if using Gateway API/Ingress)
