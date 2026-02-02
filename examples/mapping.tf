resource "hpe_msa_volume_mapping" "example" {
  volume_name = hpe_msa_volume.example.name
  target_type = "host"
  target_name = hpe_msa_host.example.name
  access      = "read-write"
  lun         = "10"
  ports       = ["a1", "b1"]
}
