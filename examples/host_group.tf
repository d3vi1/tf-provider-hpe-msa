resource "hpe_msa_host_group" "example" {
  name  = "tf-host-group"
  hosts = [
    hpe_msa_host.host1.name,
    hpe_msa_host.host2.name,
  ]
  allow_destroy = false
}
