variable "region" {
  type    = string
  default = "us-east-2"
}

variable "account_id" {
  description = "AWS account ID; used to build ARNs for import blocks."
  type        = string
}

variable "notes_bucket" {
  description = "Private bucket holding notes/{slug}.txt."
  type        = string
}

variable "web_bucket" {
  description = "Static-website frontend bucket."
  type        = string
}

variable "instance_name" {
  description = "Name tag of the live API instance."
  type        = string
  default     = "NotesTeacherAPI"
}

variable "instance_type" {
  type    = string
  default = "t3.micro"
}

variable "ssh_cidr" {
  description = "CIDR allowed to SSH. 0.0.0.0/0 today because GitHub-hosted runners deploy over SSH (see infra/DEPLOY.md)."
  type        = string
  default     = "0.0.0.0/0"
}

variable "instance_role_name" {
  type    = string
  default = "NotesTeacherInstanceRole"
}

variable "notes_policy_name" {
  type    = string
  default = "NotesBucketTeacher"
}

variable "security_group_name" {
  type    = string
  default = "NotesTeacherSG"
}

# Existing-resource IDs, used by ec2.tf and imports.tf.
variable "vpc_id" {
  type = string
}

variable "instance_id" {
  type = string
}

variable "ami_id" {
  description = "Only used if the instance is ever recreated; changes are ignored on the live one."
  type        = string
}

variable "key_name" {
  description = "Original key pair (private key lost; CI uses its own authorized_keys entry)."
  type        = string
}

variable "security_group_id" {
  type = string
}

variable "eip_allocation_id" {
  type = string
}

variable "eip_association_id" {
  type = string
}
