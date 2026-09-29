# The OpenTofu state bucket for every portfolio root. It is created once with
# the AWS CLI (refactor Phase 12) because this root stores its own state in it,
# then adopted here through the import blocks below. Once the bucket is in
# state, the import blocks are no-ops.
resource "aws_s3_bucket" "state" {
  bucket = local.state_bucket_name

  lifecycle {
    prevent_destroy = true
  }
}

# Destroying either of these would suspend versioning or remove the public
# access block on the bucket that holds all portfolio state.
resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id

  versioning_configuration {
    status = "Enabled"
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket = aws_s3_bucket.state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_ownership_controls" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

locals {
  state_bucket_imports = var.import_state_bucket ? toset([local.state_bucket_name]) : toset([])
}

import {
  for_each = local.state_bucket_imports
  to       = aws_s3_bucket.state
  id       = each.value
}

import {
  for_each = local.state_bucket_imports
  to       = aws_s3_bucket_versioning.state
  id       = each.value
}

import {
  for_each = local.state_bucket_imports
  to       = aws_s3_bucket_public_access_block.state
  id       = each.value
}

import {
  for_each = local.state_bucket_imports
  to       = aws_s3_bucket_server_side_encryption_configuration.state
  id       = each.value
}

import {
  for_each = local.state_bucket_imports
  to       = aws_s3_bucket_ownership_controls.state
  id       = each.value
}
