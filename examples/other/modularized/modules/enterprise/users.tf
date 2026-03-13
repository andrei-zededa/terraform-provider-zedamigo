data "zedcloud_role" "user" {
  for_each = toset([for user in var.users : user.role])

  name  = each.value
  title = ""
  type  = ""

  scopes {}
}

resource "zedcloud_user" "this" {
  for_each = var.users

  username   = each.key
  email      = each.value.email
  role_id    = data.zedcloud_role.user[each.value.role].id
  first_name = each.value.first_name
  full_name  = each.value.full_name

  # Optional fields
  type        = "AUTH_TYPE_LOCAL"
  notify_pref = "email"
}

resource "random_password" "passwords" {
  for_each = var.users

  length           = 16
  min_lower        = 1
  min_numeric      = 1
  min_special      = 1
  min_upper        = 1
  lower            = true
  upper            = true
  numeric          = true
  special          = true
  override_special = "#$%-_=+:?"
}

resource "zedcloud_credential" "user_credentials" {
  for_each = var.users

  # MUST match `user.username`.
  owner = zedcloud_user.this[each.key].username

  # This creates an empty credential that sends a password reset email
  # to the user.
  # type = "CREDENTIAL_TYPE_NONE"

  # This directly sets a clear-text password with which the user can login.
  type         = "CREDENTIAL_TYPE_PASSWORD"
  current_cred = ""
  new_cred     = random_password.passwords[each.key].result
}
