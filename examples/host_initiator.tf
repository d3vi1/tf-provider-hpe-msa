resource "hpe_msa_initiator" "extra" {
  initiator_id = "20000000000000c2"
  nickname     = "tf-init-02"
  allow_destroy = false
}

resource "hpe_msa_host_initiator" "example" {
  host_name    = hpe_msa_host.example.name
  initiator_id = hpe_msa_initiator.extra.initiator_id
}
