output "hello_url" {
  description = "Public URL of the hello endpoint."
  value       = "${aws_apigatewayv2_stage.default.invoke_url}hello"
}
