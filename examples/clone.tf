resource "hpe_msa_clone" "example" {
  name             = "clone-from-snap"
  source_snapshot  = hpe_msa_snapshot.example.name
  destination_pool = "A"
  allow_destroy    = false
}
