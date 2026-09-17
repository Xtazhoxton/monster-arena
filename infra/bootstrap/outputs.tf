output "github_plan_role_arn" {
  description = "Role assumed by GitHub Actions for terraform plan (pull requests and main)"
  value       = aws_iam_role.github_plan.arn
}

output "github_apply_role_arn" {
  description = "Role assumed by GitHub Actions for terraform apply (main only)"
  value       = aws_iam_role.github_apply.arn
}
