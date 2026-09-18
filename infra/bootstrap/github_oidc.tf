locals {
  # Immutable subject format (GitHub default): account id and repo id, stable across renames.
  # Source: gh api repos/Xtazhoxton/monster-arena/actions/oidc/customization/sub
  github_repo = "Xtazhoxton@130063832/monster-arena@1372827338"
  github_sub = {
    pull_request = "repo:${local.github_repo}:pull_request"
    main         = "repo:${local.github_repo}:ref:refs/heads/main"
  }
}

resource "aws_iam_openid_connect_provider" "github" {
  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]
}

# --- plan role: read-only, assumable from pull requests and main

data "aws_iam_policy_document" "github_plan_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = [local.github_sub.pull_request, local.github_sub.main]
    }
  }
}

resource "aws_iam_role" "github_plan" {
  name               = "monster-arena-github-plan"
  assume_role_policy = data.aws_iam_policy_document.github_plan_trust.json
}

resource "aws_iam_role_policy_attachment" "github_plan_readonly" {
  role       = aws_iam_role.github_plan.name
  policy_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
}

# ReadOnlyAccess can read the state but not write the lock file created during plan.
data "aws_iam_policy_document" "github_plan_state_lock" {
  statement {
    actions   = ["s3:PutObject", "s3:DeleteObject"]
    resources = ["${aws_s3_bucket.tfstate.arn}/*.tflock"]
  }
}

resource "aws_iam_role_policy" "github_plan_state_lock" {
  name   = "terraform-state-lock"
  role   = aws_iam_role.github_plan.name
  policy = data.aws_iam_policy_document.github_plan_state_lock.json
}

# --- apply role: admin, assumable from main only

data "aws_iam_policy_document" "github_apply_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = [local.github_sub.main]
    }
  }
}

resource "aws_iam_role" "github_apply" {
  name               = "monster-arena-github-apply"
  assume_role_policy = data.aws_iam_policy_document.github_apply_trust.json
}

resource "aws_iam_role_policy_attachment" "github_apply_admin" {
  role       = aws_iam_role.github_apply.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}
