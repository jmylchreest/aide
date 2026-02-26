//go:build ignore

// Package fixture contains intentionally embedded secrets for testing.
// This file is NOT compiled — it is used as test data for the secrets analyzer.
// DO NOT use any of these values; they are dummy/test strings.
package fixture

const (
	// AWS access key (starts with AKIA, 20 chars)
	awsAccessKey = "AKIAIOSFODNN7EXAMPLE"

	// AWS secret key (40-char base64)
	awsSecretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

	// GitHub personal access token
	githubPAT = "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef01234"

	// Slack webhook URL
	slackWebhook = "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX"

	// Stripe live secret key
	stripeKey = "sk_live_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijk"

	// SendGrid API key
	sendgridKey = "SG.abcdefghijklmnopqrstuv.wxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ012"

	// Generic API key in assignment pattern
	apiKey = "api_key_1234567890abcdef1234567890abcdef"

	// JWT token
	jwtToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
)

// PEMPrivateKey is an RSA private key (dummy).
var pemPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIBogIBAAJBALmK5PgFJMA3qE5mFvYBIeJTGnFRIkNPMl0vafKoHjCFxiLyc3SC
p7XdOVGLCi5GNr6jU5gw2RAiETrN99sOGIkCAwEAAQJAE4GGxXJ5kJQxCp0FHBQG
NqTj9nodQLAbMDPWNMoLbkz5nPFQiZkFc+TWJfSlDWERvnpVyiBmlkDWdEQE0ioX
AQIhAOX+FEIMgPkKZfBPhQFcBLwlhcql4URLG+OvMOqaQz5JAiEA0IK6ZqAJCMhO
mEnLTqgOySBQnIgPlGx5bMZKojHH2qkCIHYBJfJp1YHcbCKPNaNVLaHfzviRViaX
bTj+MNwGHj2RAiEAyPtFDjhqlDOAUwYMTFKjlPPQExLVDwgN8gIppMnufRkCIBRG
VCHRUBbqqJJkFhKfFRWnRxkNHu6iVQyBVCAhEUjL
-----END RSA PRIVATE KEY-----`
