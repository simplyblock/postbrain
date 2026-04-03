# Troubleshooting Playbook

Use symptom-first checks to reduce time to resolution.

## Server does not start

Check:

- config file path and syntax
- database reachability
- required DB extensions

## Authentication fails

Check:

- token validity
- `Authorization: Bearer ...` header
- requested scope vs token scope

## Social login fails

Check:

- provider client ID/secret and callback URL
- `oauth.base_url` correctness
- domain/verified-email policy settings

## Gateway/HTTPRoute not programmed

Check:

- Gateway accepted status
- HTTPRoute `Accepted`/`ResolvedRefs`
- TLS secret existence if HTTPS listener is used
- dataplane service external address readiness

## Cert-manager certificate not ready

Check:

- `Certificate` status conditions
- `CertificateRequest`/`Order`/`Challenge` objects
- DNS solver credentials and zone permissions
