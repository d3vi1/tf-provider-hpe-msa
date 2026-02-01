resource "hpe_msa_snapshot" "example" {
  name          = "snap01"
  volume_name   = hpe_msa_volume.example.name
  allow_destroy = false
}
