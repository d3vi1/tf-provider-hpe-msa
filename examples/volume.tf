resource "hpe_msa_volume" "example" {
  name          = "vol01"
  size          = "100GB"
  vdisk         = "A"
  allow_destroy = false
}
