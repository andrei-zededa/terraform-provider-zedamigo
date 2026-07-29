# A patch envelope is a way to publish additional files ("artifacts") to an
# edge-app-instance out-of-band from the app image itself. Here we publish two
# example text files as inline (base64-encoded) artifacts.
#
# The two text files live next to this config as separate files and are loaded
# with the `filebase64()` function, which reads a file from disk and returns its
# contents already base64-encoded - exactly what `base64_data` expects.
#
# See: https://help.zededa.com/hc/en-us/articles/9648125142299-Patch-Envelopes
resource "zedcloud_patch_envelope" "APP_TEXT_FILES" {
  name        = "app_text_files_${var.config_suffix}"
  title       = "app_text_files_${var.config_suffix}"
  description = "Two example text files published to the ubuntu test app-instance."

  # ACTIVATE makes the envelope available to the referenced app-instances.
  action               = "PATCH_ENVELOPE_ACTION_ACTIVATE"
  user_defined_version = "1.0"

  project_id   = zedcloud_project.PROJECT.id
  project_name = zedcloud_project.PROJECT.name

  artifacts {
    format = "OpaqueObjectCategoryInline"
    base64_artifact {
      file_name_to_use = "patch_file_1.txt"
      base64_data      = filebase64("${path.module}/patch_file_1.txt")
    }
  }

  artifacts {
    format = "OpaqueObjectCategoryInline"
    base64_artifact {
      file_name_to_use = "patch_file_2.txt"
      base64_data      = filebase64("${path.module}/patch_file_2.txt")
    }
  }
}

# Bind the patch envelope to the edge-app-instance(s) so that the files are
# actually delivered to the running app.
resource "zedcloud_patch_reference_update" "APP_TEXT_FILES" {
  patchenvelope_id = zedcloud_patch_envelope.APP_TEXT_FILES.id
  project_id       = zedcloud_project.PROJECT.id

  app_inst_id_list = [
    for x in zedcloud_application_instance.APP_INSTANCES_VMS : x.id
  ]
}
