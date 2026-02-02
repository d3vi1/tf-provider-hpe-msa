resource "hpe_msa_host" "example" {
  name        = "tf-host-01"
  initiators  = [hpe_msa_initiator.example.initiator_id]
  allow_destroy = false
}
