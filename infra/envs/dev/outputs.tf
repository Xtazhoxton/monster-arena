output "hello_url" {
  description = "Public URL of the hello endpoint."
  value       = "${aws_apigatewayv2_stage.default.invoke_url}hello"
}

output "creatures_url" {
  description = "Public URL of the creatures collection."
  value       = "${aws_apigatewayv2_stage.default.invoke_url}creatures"
}

output "auth_token_endpoint" {
  description = "OAuth2 token endpoint for machine clients"
  value       = "https://${aws_cognito_user_pool_domain.main.domain}.auth.${aws_cognito_user_pool_domain.main.region}.amazoncognito.com/oauth2/token"
}

output "admin_client_id" {
  description = "App client id for the admin machine credentials"
  value       = aws_cognito_user_pool_client.admin_cli.id
}
