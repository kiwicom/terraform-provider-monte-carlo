variable "montecarlo_api_key_id" {
  type = string
}

variable "montecarlo_api_key_token" {
  type = string
}

provider "montecarlo" {
  account_service_key = {
    id    = var.montecarlo_api_key_id     # (secret)
    token = var.montecarlo_api_key_token  # (secret)
  }
}

variable "bq_service_account" {
  type = string
}

resource "montecarlo_bigquery_warehouse" "test" {
  name                = "test-warehouse"
  collector_uuid      = "a08d23fc-00a0-4c36-b568-82e9d0e67ad8"
  service_account_key = var.bq_service_account
  deletion_protection = false
}
