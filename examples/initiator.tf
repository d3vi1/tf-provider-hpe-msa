resource "hpe_msa_initiator" "example" {
  initiator_id = "20000000000000c1"
  nickname     = "tf-init-01"
  allow_destroy = false
}
