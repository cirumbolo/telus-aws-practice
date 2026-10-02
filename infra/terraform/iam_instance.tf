resource "aws_iam_policy" "notes_rw" {
  name = var.notes_policy_name

  # No s3:ListBucket on purpose: a missing key surfaces as AccessDenied, which
  # isNotFound in api/store_s3.go handles (see CLAUDE.md).
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "ReadWriteNoteObjects"
      Effect   = "Allow"
      Action   = ["s3:GetObject", "s3:PutObject", "s3:CopyObject", "s3:DeleteObject"]
      Resource = "${aws_s3_bucket.notes.arn}/notes/*"
    }]
  })
}

resource "aws_iam_role" "instance" {
  name        = var.instance_role_name
  description = "Allows EC2 instances to call AWS services on your behalf."

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "instance_notes" {
  role       = aws_iam_role.instance.name
  policy_arn = aws_iam_policy.notes_rw.arn
}

resource "aws_iam_instance_profile" "instance" {
  name = var.instance_role_name
  role = aws_iam_role.instance.name
}
