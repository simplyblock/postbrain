# Troubleshooting Playbook

Use symptom-first checks to reduce time to resolution.

Start with the smallest reproducible symptom and verify one dependency layer at a time: configuration, connectivity,
auth, then workload-specific behavior.

## Server does not start

Check:

- config file path and syntax
- database reachability
- required DB extensions

If startup fails, capture the first fatal error log line before changing anything. Secondary errors are often cascade
effects.

## Authentication fails

Check:

- token validity
- `Authorization: Bearer ...` header
- requested scope vs token scope

A common cause is valid token + invalid scope. Confirm both principal and token are authorized for the target scope.

## Social login fails

Check:

- provider client ID/secret and callback URL
- `oauth.base_url` correctness
- domain/verified-email policy settings

Provider callback URL mismatches are especially common after hostname or ingress/gateway changes.

## Gateway/HTTPRoute not programmed

Check:

- Gateway accepted status
- HTTPRoute `Accepted`/`ResolvedRefs`
- TLS secret existence if HTTPS listener is used
- dataplane service external address readiness

If Gateway is accepted but not programmed, the issue is usually dataplane exposure (for example LoadBalancer pending).

## Cert-manager certificate not ready

Check:

- `Certificate` status conditions
- `CertificateRequest`/`Order`/`Challenge` objects
- DNS solver credentials and zone permissions

When DNS01 is pending, troubleshoot solver credentials and DNS zone access before changing gateway/listener config.
