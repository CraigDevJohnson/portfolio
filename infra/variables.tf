variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "us-west-2"
}

variable "app_name" {
  description = "Name of the application"
  type        = string
  default     = "portfolio"
}

variable "environment" {
  description = "Deployment environment tag for provisioned resources"
  type        = string
  default     = "development"
}
