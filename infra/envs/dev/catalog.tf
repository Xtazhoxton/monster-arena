locals {
  catalog_name = "monster-arena-dev-catalog"
}

resource "aws_dynamodb_table" "catalog" {
  name         = local.catalog_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "PK"
  range_key    = "SK"

  attribute {
    name = "PK"
    type = "S"
  }

  attribute {
    name = "SK"
    type = "S"
  }

  global_secondary_index {
    name            = "GSI1"
    projection_type = "ALL"

    key_schema {
      attribute_name = "SK"
      key_type       = "HASH"
    }

    key_schema {
      attribute_name = "PK"
      key_type       = "RANGE"
    }
  }

  deletion_protection_enabled = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_iam_role" "catalog" {
  name               = local.catalog_name
  assume_role_policy = data.aws_iam_policy_document.lambda_trust.json
}

resource "aws_iam_role_policy_attachment" "catalog_logs" {
  role       = aws_iam_role.catalog.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

data "aws_iam_policy_document" "catalog_table" {
  statement {
    sid = "ReadCreatures"

    actions = [
      "dynamodb:GetItem",
      "dynamodb:BatchGetItem",
      "dynamodb:Query",
    ]

    resources = [
      aws_dynamodb_table.catalog.arn,
      "${aws_dynamodb_table.catalog.arn}/index/*",
    ]
  }
}

resource "aws_iam_role_policy" "catalog_table" {
  name   = "catalog-table-access"
  role   = aws_iam_role.catalog.id
  policy = data.aws_iam_policy_document.catalog_table.json
}

data "archive_file" "catalog" {
  type        = "zip"
  source_file = "${path.module}/../../../dist/catalog/bootstrap"
  output_path = "${path.module}/.build/catalog.zip"
}

resource "aws_cloudwatch_log_group" "catalog" {
  name              = "/aws/lambda/${local.catalog_name}"
  retention_in_days = 7
}

resource "aws_lambda_function" "catalog" {
  function_name    = local.catalog_name
  role             = aws_iam_role.catalog.arn
  runtime          = "provided.al2023"
  handler          = "bootstrap"
  architectures    = ["arm64"]
  memory_size      = 256
  timeout          = 10
  filename         = data.archive_file.catalog.output_path
  source_code_hash = data.archive_file.catalog.output_base64sha256

  environment {
    variables = {
      TABLE_NAME = aws_dynamodb_table.catalog.name
    }
  }

  logging_config {
    log_format = "JSON"
    log_group  = aws_cloudwatch_log_group.catalog.name
  }
}

resource "aws_apigatewayv2_integration" "catalog" {
  api_id                 = aws_apigatewayv2_api.main.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.catalog.invoke_arn
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "get_creatures" {
  api_id             = aws_apigatewayv2_api.main.id
  route_key          = "GET /creatures"
  target             = "integrations/${aws_apigatewayv2_integration.catalog.id}"
  authorization_type = "NONE"
}

resource "aws_apigatewayv2_route" "get_creature" {
  api_id             = aws_apigatewayv2_api.main.id
  route_key          = "GET /creatures/{id}"
  target             = "integrations/${aws_apigatewayv2_integration.catalog.id}"
  authorization_type = "NONE"
}

resource "aws_lambda_permission" "catalog_api" {
  statement_id  = "AllowInvokeFromHttpApi"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.catalog.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.main.execution_arn}/*/*"
}
