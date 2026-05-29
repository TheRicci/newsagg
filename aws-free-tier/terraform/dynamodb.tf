resource "aws_dynamodb_table" "articles" {
  name         = "${local.name_prefix}-articles"
  billing_mode = "PROVISIONED"
  hash_key     = "id"

  read_capacity  = 1
  write_capacity = 1

  attribute {
    name = "id"
    type = "S"
  }

  attribute {
    name = "feed_key"
    type = "S"
  }

  attribute {
    name = "topic"
    type = "S"
  }

  attribute {
    name = "published_at"
    type = "S"
  }

  global_secondary_index {
    name            = "feed-published-index"
    hash_key        = "feed_key"
    range_key       = "published_at"
    projection_type = "ALL"
    read_capacity   = 1
    write_capacity  = 1
  }

  global_secondary_index {
    name            = "topic-published-index"
    hash_key        = "topic"
    range_key       = "published_at"
    projection_type = "ALL"
    read_capacity   = 1
    write_capacity  = 1
  }
}

resource "aws_dynamodb_table" "topic_counts" {
  name         = "${local.name_prefix}-topic-counts"
  billing_mode = "PROVISIONED"
  hash_key     = "topic"

  read_capacity  = 1
  write_capacity = 1

  attribute {
    name = "topic"
    type = "S"
  }
}

