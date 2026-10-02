output "api_ip" {
  description = "Elastic IP; this is the EC2_HOST secret for CI."
  value       = aws_eip.api.public_ip
}

output "web_bucket" {
  description = "The S3_WEB_BUCKET secret for CI."
  value       = aws_s3_bucket.web.id
}

output "web_endpoint" {
  description = "Site URL; also the ALLOW_ORIGIN (with http://) in note-api.service."
  value       = aws_s3_bucket_website_configuration.web.website_endpoint
}
