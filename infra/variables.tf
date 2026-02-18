variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "us-west-2"
}

variable "app_name" {
  description = "Name of the application"
  type        = string
  default     = "portfolio-dev"
}

variable "container_port" {
  description = "Port the container listens on"
  type        = number
  default     = 8080
}

variable "ecr_image_tag" {
  description = "Tag for the container image in ECR"
  type        = string
  default     = "latest"
}

variable "app_runner_cpu" {
  description = "CPU units for App Runner (1024 = 1 vCPU)"
  type        = string
  default     = "256"
}

variable "app_runner_memory" {
  description = "Memory in MB for App Runner"
  type        = string
  default     = "512"
}
