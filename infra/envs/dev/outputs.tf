output "hello_url" {
  description = "Public URL of the hello endpoint."
  value       = "${aws_apigatewayv2_stage.default.invoke_url}hello"
}

output "creatures_url" {
  description = "Public URL of the creatures collection."
  value       = "${aws_apigatewayv2_stage.default.invoke_url}creatures"
}
