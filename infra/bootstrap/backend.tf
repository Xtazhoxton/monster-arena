terraform {
  backend "s3" {
    bucket       = "monster-arena-tfstate-533449297933-ap-northeast-1-an"
    key          = "bootstrap/terraform.tfstate"
    region       = "ap-northeast-1"
    encrypt      = true
    use_lockfile = true
  }
}
