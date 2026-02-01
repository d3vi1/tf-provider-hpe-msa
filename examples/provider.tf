terraform {
  required_providers {
    hpe_msa = {
      source  = "d3vi1/hpe-msa"
      version = ">= 0.1.0"
    }
  }
}

provider "hpe" {
  endpoint     = "https://msa.example.com"
  username     = "msa_user"
  password     = "example-password"
  insecure_tls = true
  timeout      = "30s"
}
