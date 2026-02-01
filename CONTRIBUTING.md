# Contributing

Thanks for helping improve the HPE MSA Terraform provider.

## Workflow

1. Create or pick a GitHub issue.
2. Create a feature branch for that issue.
3. Implement changes with tests and docs updates.
4. Ensure CI is green and linting passes.
5. Open a PR and follow the review checklist.

## Development

```bash
make lint
make test
make testacc
```

Acceptance tests require real array credentials and must never run by default.

## Security

This repository is public. Do not commit IPs, usernames, passwords, or other
infrastructure identifiers. Use local-only files such as `.env` or
`msa-test-config.yaml` (both gitignored).
