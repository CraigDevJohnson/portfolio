resource "aws_dynamodb_table" "google_connections" {
  name                        = local.google_table
  billing_mode                = "PAY_PER_REQUEST"
  hash_key                    = "connection_id"
  deletion_protection_enabled = var.enable_deletion_protection

  attribute {
    name = "connection_id"
    type = "S"
  }

  point_in_time_recovery {
    enabled = var.enable_pitr
  }

  server_side_encryption {
    enabled = true
  }
}

resource "aws_dynamodb_table" "soccer_sessions" {
  name                        = local.soccer_table
  billing_mode                = "PAY_PER_REQUEST"
  hash_key                    = "session_id"
  deletion_protection_enabled = var.enable_deletion_protection

  attribute {
    name = "session_id"
    type = "S"
  }

  point_in_time_recovery {
    enabled = var.enable_pitr
  }

  server_side_encryption {
    enabled = true
  }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }
}

# Durable known-team facts are separate from the expiring import baseline.
resource "aws_dynamodb_table" "soccer_history" {
  name                        = local.soccer_archive_table
  billing_mode                = "PAY_PER_REQUEST"
  hash_key                    = "pk"
  range_key                   = "sk"
  deletion_protection_enabled = var.enable_deletion_protection

  attribute {
    name = "pk"
    type = "S"
  }

  attribute {
    name = "sk"
    type = "S"
  }

  attribute {
    name = "due_pk"
    type = "S"
  }

  attribute {
    name = "due_sk"
    type = "S"
  }

  global_secondary_index {
    name            = "due-teams"
    hash_key        = "due_pk"
    range_key       = "due_sk"
    projection_type = "ALL"
  }

  point_in_time_recovery {
    enabled = var.enable_pitr
  }

  server_side_encryption {
    enabled = true
  }
}
