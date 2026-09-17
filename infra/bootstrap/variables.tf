variable "alert_email" {
  description = "E-mail receiving budget alerts"
  type        = string
  sensitive   = true
}

variable "admin_user_name" {
  description = "Human IAM user blocked by the emergency budget action"
  type        = string
  default     = "admin"
}
