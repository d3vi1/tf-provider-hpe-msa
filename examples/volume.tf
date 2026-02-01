resource "hpe_msa_volume" "example" {
  name          = "vol01"
  size          = "100GB"
  pool          = "pool-a"
  allow_destroy = false
}
