resource "aws_ecr_repository" "lambda_releases" {
  name                 = "portfolio-lambda-releases"
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false

  encryption_configuration {
    encryption_type = "AES256"
  }

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "lambda_releases" {
  repository = aws_ecr_repository.lambda_releases.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Expire untagged images after 30 days"
      selection = {
        tagStatus   = "untagged"
        countType   = "sinceImagePushed"
        countUnit   = "days"
        countNumber = 30
      }
      action = { type = "expire" }
    }]
  })
}

resource "aws_ecr_repository_policy" "lambda_releases" {
  repository = aws_ecr_repository.lambda_releases.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "LambdaPull"
      Effect = "Allow"
      Principal = {
        Service = "lambda.amazonaws.com"
      }
      Action = [
        "ecr:BatchGetImage",
        "ecr:GetDownloadUrlForLayer",
      ]
      Condition = {
        StringEquals = {
          "aws:SourceAccount" = "180294223248"
        }
        ArnLike = {
          "aws:SourceArn" = "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-*"
        }
      }
    }]
  })
}
