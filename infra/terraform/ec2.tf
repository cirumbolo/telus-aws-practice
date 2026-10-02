resource "aws_security_group" "api" {
  name        = var.security_group_name
  description = "Security group for note.ms clone EC2 instance (API port 8080 + SSH admin)"
  vpc_id      = var.vpc_id

  ingress {
    # API
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    # SSH: admin + GitHub Actions deploy
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_cidr]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_instance" "api" {
  ami                    = var.ami_id
  instance_type          = var.instance_type
  key_name               = var.key_name
  iam_instance_profile   = aws_iam_instance_profile.instance.name
  vpc_security_group_ids = [aws_security_group.api.id]

  metadata_options {
    http_tokens                 = "required" # IMDSv2
    http_endpoint               = "enabled"
    http_put_response_hop_limit = 2
  }

  tags = {
    Name = var.instance_name
  }

  # The instance is hand-configured (systemd unit) and the binary arrives via
  # CI, so never let drift in these force a replacement.
  lifecycle {
    ignore_changes = [ami, key_name, user_data, subnet_id]
  }
}

resource "aws_eip" "api" {
  domain = "vpc"
}

resource "aws_eip_association" "api" {
  instance_id   = aws_instance.api.id
  allocation_id = aws_eip.api.id
}
