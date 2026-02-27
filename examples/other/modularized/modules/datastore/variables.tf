variable "name" {
  description = "Datastore name"
  type        = string
}

variable "title" {
  description = "Datastore title (defaults to name)"
  type        = string
  default     = ""
}

variable "ds_type" {
  description = "Datastore type, e.g. DATASTORE_TYPE_HTTP, DATASTORE_TYPE_HTTPS, DATASTORE_TYPE_CONTAINERREGISTRY"
  type        = string
  default     = "DATASTORE_TYPE_HTTPS"
}

variable "ds_fqdn" {
  description = "Fully qualified domain name for the datastore"
  type        = string
}

# The datastore FQDN+PATH are used when an edge-node tries to download an image.
# The resulting URL for downloading the image will calculated by the edge-node as:
#     ${ds_fqdn}/${ds_path}/${image_rel_url}
variable "ds_path" {
  description = "Path within the datastore"
  type        = string
  default     = ""
}

variable "description" {
  description = "Datastore description"
  type        = string
  default     = ""
}
