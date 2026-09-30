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
# Nothing is planned until the activation review enables the table.
resource "aws_dynamodb_table" "soccer_history" {
  count = var.enable_soccer_history ? 1 : 0

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
    projection_type = "ALL"

    key_schema {
      attribute_name = "due_pk"
      key_type       = "HASH"
    }

    key_schema {
      attribute_name = "due_sk"
      key_type       = "RANGE"
    }
  }

  point_in_time_recovery {
    enabled = var.enable_pitr
  }

  server_side_encryption {
    enabled = true
  }
}
