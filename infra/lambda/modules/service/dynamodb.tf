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
