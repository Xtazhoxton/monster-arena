# Budget created by hand before Terraform: adopted into the state instead of recreated.
import {
  to = aws_budgets_budget.monthly
  id = "${data.aws_caller_identity.current.account_id}:My Monthly Cost Budget"
}

resource "aws_budgets_budget" "monthly" {
  name              = "My Monthly Cost Budget"
  budget_type       = "COST"
  limit_amount      = "20.0"
  limit_unit        = "USD"
  time_unit         = "MONTHLY"
  time_period_start = "2026-05-01_00:00"
  metrics           = ["UnblendedCost"]

  # Real usage: credits and refunds must not hide a cost drift.
  filter_expression {
    not {
      dimensions {
        key    = "RECORD_TYPE"
        values = ["Credit", "Refund"]
      }
    }
  }

  dynamic "notification" {
    for_each = [1, 5, 10]

    content {
      notification_type          = "ACTUAL"
      comparison_operator        = "GREATER_THAN"
      threshold                  = notification.value
      threshold_type             = "ABSOLUTE_VALUE"
      subscriber_email_addresses = [var.alert_email]
    }
  }

  notification {
    notification_type          = "FORECASTED"
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    subscriber_email_addresses = [var.alert_email]
  }
}

# --- Emergency brake: at 20 $ of real spend, deny (almost) everything to humans and CI.

data "aws_iam_policy_document" "emergency_deny" {
  statement {
    effect = "Deny"
    # Everything except what is needed to see the bill and remove this policy by hand.
    not_actions = [
      "sts:GetCallerIdentity",
      "signin:*",
      "iam:GetUser",
      "iam:ListAttachedUserPolicies",
      "iam:DetachUserPolicy",
      "budgets:*",
      "ce:*",
      "billing:*",
      "freetier:*",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "emergency_deny" {
  name        = "monster-arena-emergency-deny"
  description = "Attached automatically by AWS Budgets when real spend exceeds the limit"
  policy      = data.aws_iam_policy_document.emergency_deny.json
}

data "aws_iam_policy_document" "budget_action_trust" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["budgets.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }
}

# Only allowed to attach/detach the emergency policy, nothing else.
data "aws_iam_policy_document" "budget_action_permissions" {
  statement {
    actions = [
      "iam:AttachUserPolicy",
      "iam:DetachUserPolicy",
      "iam:AttachRolePolicy",
      "iam:DetachRolePolicy",
    ]
    resources = [
      "arn:aws:iam::${data.aws_caller_identity.current.account_id}:user/${var.admin_user_name}",
      aws_iam_role.github_plan.arn,
      aws_iam_role.github_apply.arn,
    ]

    condition {
      test     = "ArnEquals"
      variable = "iam:PolicyARN"
      values   = [aws_iam_policy.emergency_deny.arn]
    }
  }
}

resource "aws_iam_role" "budget_action" {
  name               = "monster-arena-budget-action"
  assume_role_policy = data.aws_iam_policy_document.budget_action_trust.json
}

resource "aws_iam_role_policy" "budget_action" {
  name   = "attach-emergency-deny"
  role   = aws_iam_role.budget_action.name
  policy = data.aws_iam_policy_document.budget_action_permissions.json
}

resource "aws_budgets_budget_action" "emergency_deny" {
  budget_name        = aws_budgets_budget.monthly.name
  action_type        = "APPLY_IAM_POLICY"
  approval_model     = "AUTOMATIC"
  notification_type  = "ACTUAL"
  execution_role_arn = aws_iam_role.budget_action.arn

  action_threshold {
    action_threshold_type  = "PERCENTAGE"
    action_threshold_value = 100
  }

  definition {
    iam_action_definition {
      policy_arn = aws_iam_policy.emergency_deny.arn
      users      = [var.admin_user_name]
      roles      = [aws_iam_role.github_plan.name, aws_iam_role.github_apply.name]
    }
  }

  subscriber {
    address           = var.alert_email
    subscription_type = "EMAIL"
  }
}
