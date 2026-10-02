# One-time adoption of the hand-built stack. After a clean apply these can stay
# (they're no-ops once the resources are in state) or be deleted.

import {
  to = aws_s3_bucket.notes
  id = var.notes_bucket
}

import {
  to = aws_s3_bucket_public_access_block.notes
  id = var.notes_bucket
}

import {
  to = aws_s3_bucket_server_side_encryption_configuration.notes
  id = var.notes_bucket
}

import {
  to = aws_s3_bucket.web
  id = var.web_bucket
}

import {
  to = aws_s3_bucket_public_access_block.web
  id = var.web_bucket
}

import {
  to = aws_s3_bucket_server_side_encryption_configuration.web
  id = var.web_bucket
}

import {
  to = aws_s3_bucket_website_configuration.web
  id = var.web_bucket
}

import {
  to = aws_s3_bucket_policy.web
  id = var.web_bucket
}

import {
  to = aws_iam_policy.notes_rw
  id = "arn:aws:iam::${var.account_id}:policy/${var.notes_policy_name}"
}

import {
  to = aws_iam_role.instance
  id = var.instance_role_name
}

import {
  to = aws_iam_role_policy_attachment.instance_notes
  id = "${var.instance_role_name}/arn:aws:iam::${var.account_id}:policy/${var.notes_policy_name}"
}

import {
  to = aws_iam_instance_profile.instance
  id = var.instance_role_name
}

import {
  to = aws_security_group.api
  id = var.security_group_id
}

import {
  to = aws_instance.api
  id = var.instance_id
}

import {
  to = aws_eip.api
  id = var.eip_allocation_id
}

import {
  to = aws_eip_association.api
  id = var.eip_association_id
}
