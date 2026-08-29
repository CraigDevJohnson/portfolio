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

variable "lambda_image_tag" {
  description = "Tag for the Lambda container image in ECR"
  type        = string
  default     = "lambda-latest"
}

variable "lambda_timeout_seconds" {
  description = "Lambda timeout in seconds"
  type        = number
  default     = 30
}

variable "lambda_memory_mb" {
  description = "Lambda memory in MB"
  type        = number
  default     = 512
}
