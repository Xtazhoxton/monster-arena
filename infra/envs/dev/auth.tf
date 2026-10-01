locals {
  auth_name = "monster-arena-dev-auth"
}

resource "aws_cognito_user_pool" "main" {
  name = local.auth_name

  user_pool_tier      = "LITE"
  deletion_protection = "ACTIVE"

  admin_create_user_config {
    allow_admin_create_user_only = true
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_cognito_resource_server" "catalog" {
  user_pool_id = aws_cognito_user_pool.main.id
  name         = "catalog"
  identifier   = "catalog"

  scope {
    scope_name        = "write"
    scope_description = "Write access to the catalog: creatures, moves and types."
  }
}

resource "aws_cognito_user_pool_domain" "main" {
  user_pool_id = aws_cognito_user_pool.main.id
  domain       = "monster-arena-dev-533449297933"

}

resource "aws_cognito_user_pool_client" "admin_cli" {
  user_pool_id = aws_cognito_user_pool.main.id
  name         = "admin-cli"

  generate_secret                      = true
  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["client_credentials"]
  allowed_oauth_scopes                 = ["catalog/write"]

  access_token_validity = 1

  token_validity_units {
    access_token = "hours"
  }
}

resource "aws_apigatewayv2_authorizer" "jwt" {
  api_id           = aws_apigatewayv2_api.main.id
  name             = "cognito"
  authorizer_type  = "JWT"
  identity_sources = ["$request.header.Authorization"]

  jwt_configuration {
    issuer = "https://${aws_cognito_user_pool.main.endpoint}"

    audience = [aws_cognito_user_pool_client.admin_cli.id]
  }

}
