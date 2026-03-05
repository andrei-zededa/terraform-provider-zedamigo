# Just an example, do not use this for production. The API response looks
# like this:
#     ❯ curl https://generate-random.org/api/v1/generate/passwords | jq .
#     {
#       "success": true,
#       "data": [
#         "W>eVfU[W:1W?1%ni"
#       ],
#       "metadata": {
#         "count": 1,
#         "generated_at": "2026-03-05T10:26:04+00:00",
#         "locale": "en",
#         "entropy": 103.35090589819676,
#         "strength": "very_strong"
#       }
#     }
#
# NOTE: This being a datasource it's "contents" are saved in the TF STATE, so
# if the state is not encrypted then the only alternative is to use a "sensitive"
# variable whose value comes from an environment variable.
data "terracurl_request" "random_password" {
  name           = "random_password"
  url            = "https://generate-random.org/api/v1/generate/passwords"
  method         = "GET"
  response_codes = [200]
  max_retry      = 1
  retry_interval = 10
}

module "enterprise" {
  source       = "../modules/enterprise"
  app_password = jsondecode(data.terracurl_request.random_password.response).data[0]
}

output "project_name" {
  value = module.enterprise.project_name
}

output "brand_name" {
  value = module.enterprise.brand_name
}

output "image_names" {
  value = module.enterprise.image_names
}

output "app_names" {
  value = module.enterprise.app_names
}

output "datastore_names" {
  value = module.enterprise.datastore_names
}

output "network_name" {
  value = module.enterprise.network_name
}
