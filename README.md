# Terraform Provider for HPE MSA 2050 (XML API)

Production-quality Terraform provider for HPE MSA 2050 arrays (firmware VL270P008) using the embedded HTTPS XML API. This repository is intentionally scoped to volume/LUN, snapshot, clone, host, initiator, and mapping lifecycle only.

## Status

Milestone 0: bootstrap, CI, skeleton provider, and XML client scaffolding with mocks. Resources and data sources will be implemented in subsequent milestones.

## Requirements

- Go 1.22+
- Terraform 1.5+

## Provider configuration

```hcl
provider "hpe_msa" {
  endpoint     = "https://msa.example.com"
  username     = "msa_user"
  password     = "example-password"
  insecure_tls = true
  timeout      = "30s"
}
```

### Environment variables (tests and local tooling)

These are used by local tools and acceptance tests. Do **not** commit real values.

- `MSA_ENDPOINT` (e.g., `https://ip-or-fqdn`)
- `MSA_USERNAME`
- `MSA_PASSWORD`
- `MSA_INSECURE_TLS` (`true`/`false`)
- `MSA_POOL` (or vdisk/pool name for volume placement)
- Optional: `MSA_TEST_HOST_NAME`, `MSA_TEST_INITIATOR_WWPN`, `MSA_TEST_INITIATOR_IQN`

## Development

```bash
make lint
make test
make testacc
```

- `make test` runs unit tests with mocks/golden XML fixtures.
- `make testacc` runs acceptance tests and requires real array credentials.

## Examples

See `examples/` for provider configuration samples.

## Security

This repo is public. Do not place IPs, usernames, passwords, or array identifiers in source control. Use local-only files such as `.env` or `msa-test-config.yaml` (both gitignored).

## Contributing

See `CONTRIBUTING.md` for workflow and expectations.

## License

Apache 2.0. See `LICENSE`.
