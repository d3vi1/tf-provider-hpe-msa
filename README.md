# Terraform Provider for HPE MSA 2050 (XML API)

Production-quality Terraform provider for HPE MSA 2050 arrays (firmware VL270P008) using the embedded HTTPS XML API. This repository is intentionally scoped to volume/LUN, snapshot, clone, host, initiator, and mapping lifecycle only.

## Status

Implemented resources: volumes, snapshots, clones (snapshot-based), initiators, hosts, host groups, host initiator membership, and volume mappings. Pending: acceptance tests and hardening.

## Requirements

- Go 1.22+
- Terraform 1.5+

## Provider configuration

```hcl
provider "hpe" {
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
- Optional: `MSA_TEST_PREFIX` (prefix for acceptance-test resource names)

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

### Volume

```hcl
resource "hpe_msa_volume" "example" {
  name          = "vol01"
  size          = "100GB"
  vdisk         = "A"
  allow_destroy = false
}
```

If `pool`/`vdisk` is omitted and the array reports exactly one pool, the provider will use that pool automatically.

The volume resource also exposes `scsi_wwn`, which surfaces the host-visible SCSI/NAA identifier reported by the array for stable `/dev/disk/by-id` usage.

Import by serial number:

```bash
terraform import hpe_msa_volume.example SERIAL-NUMBER
```

### Snapshot

```hcl
resource "hpe_msa_snapshot" "example" {
  name          = "snap01"
  volume_name   = hpe_msa_volume.example.name
  allow_destroy = false
}
```

Import by serial number:

```bash
terraform import hpe_msa_snapshot.example SERIAL-NUMBER
```

### Clone (from snapshot)

```hcl
resource "hpe_msa_clone" "example" {
  name            = "clone-from-snap"
  source_snapshot = hpe_msa_snapshot.example.name
  destination_pool = "A"
  allow_destroy   = false
}
```

Import by serial number:

```bash
terraform import hpe_msa_clone.example SERIAL-NUMBER
```

### Initiator + Host

```hcl
resource "hpe_msa_initiator" "init1" {
  initiator_id = "20000000000000c1"
  nickname     = "tf-init-01"
  allow_destroy = false
}

resource "hpe_msa_host" "host1" {
  name        = "tf-host-01"
  initiators  = [hpe_msa_initiator.init1.initiator_id]
  allow_destroy = false
}
```

Import by initiator ID:

```bash
terraform import hpe_msa_initiator.init1 20000000000000c1
```

Import by host name:

```bash
terraform import hpe_msa_host.host1 tf-host-01
```

### Host membership

```hcl
resource "hpe_msa_initiator" "init2" {
  initiator_id = "20000000000000c2"
  nickname     = "tf-init-02"
  allow_destroy = false
}

resource "hpe_msa_host_initiator" "member" {
  host_name    = hpe_msa_host.host1.name
  initiator_id = hpe_msa_initiator.init2.initiator_id
}
```

Import by host and initiator ID:

```bash
terraform import hpe_msa_host_initiator.member tf-host-01:20000000000000c2
```

### Host group

```hcl
resource "hpe_msa_host_group" "example" {
  name          = "tf-host-group"
  hosts         = [hpe_msa_host.host1.name, hpe_msa_host.host2.name]
  allow_destroy = false
}
```

Import by host group name:

```bash
terraform import hpe_msa_host_group.example tf-host-group
```

### Volume mapping

```hcl
resource "hpe_msa_volume_mapping" "example" {
  volume_name = hpe_msa_volume.example.name
  target_type = "host"
  target_name = hpe_msa_host.example.name
  access      = "read-write"
  lun         = "10"
  ports       = ["a1", "b1"]
}
```

Import by volume name, target type, and target name:

```bash
terraform import hpe_msa_volume_mapping.example vol01:host:Host1
```

## Data sources

- `hpe_msa_pool` - lookup a pool by name (returns raw XML properties)
- `hpe_msa_volume` - lookup a volume by name or regex (returns identifiers and properties)
- `hpe_msa_host` - lookup a host by name (returns raw XML properties)

## Security

This repo is public. Do not place IPs, usernames, passwords, or array identifiers in source control. Use local-only files such as `.env` or `msa-test-config.yaml` (both gitignored).

## Contributing

See `CONTRIBUTING.md` for workflow and expectations.

## License

Apache 2.0. See `LICENSE`.
