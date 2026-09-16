provider "aws" {
  region = "ap-northeast-1"

  default_tags {
    tags = {
      Project   = "monster-arena"
      ManagedBy = "terraform"
      Stack     = "bootstrap"
    }
  }
}
