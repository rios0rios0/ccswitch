// Package builders provides builders for the test data the ccswitch suites share.
package builders

import (
	"encoding/base64"
	"encoding/json"
	"maps"
	"time"
)

// fixtureSignature stands in for a token signature, which nothing in ccswitch checks.
const fixtureSignature = "fixture-signature"

// OpenAITokenBuilder builds a token shaped like the ones OpenAI issues to Codex --
// a header, a payload of claims and a signature -- so that the code reading claims
// out of an ID or access token can be exercised. It is assembled at run time from its
// claims, so that no credential-shaped literal sits in the source for a secret scanner
// to flag.
type OpenAITokenBuilder struct {
	claims map[string]any
	auth   map[string]any
}

// NewOpenAITokenBuilder starts a token with no claims.
func NewOpenAITokenBuilder() *OpenAITokenBuilder {
	return &OpenAITokenBuilder{claims: map[string]any{}, auth: map[string]any{}}
}

// WithEmail names the email, as the OpenID claim of an ID token does.
func (b *OpenAITokenBuilder) WithEmail(email string) *OpenAITokenBuilder {
	b.claims["email"] = email
	return b
}

// WithProfileEmail names the email in the profile claim, as an access token does.
func (b *OpenAITokenBuilder) WithProfileEmail(email string) *OpenAITokenBuilder {
	b.claims["https://api.openai.com/profile"] = map[string]any{"email": email}
	return b
}

// WithName names the person who signed in.
func (b *OpenAITokenBuilder) WithName(name string) *OpenAITokenBuilder {
	b.claims["name"] = name
	return b
}

// WithAccountID names the ChatGPT account the token acts for.
func (b *OpenAITokenBuilder) WithAccountID(accountID string) *OpenAITokenBuilder {
	b.auth["chatgpt_account_id"] = accountID
	return b
}

// WithUserID names the person acting for the ChatGPT account.
func (b *OpenAITokenBuilder) WithUserID(userID string) *OpenAITokenBuilder {
	b.auth["chatgpt_user_id"] = userID
	return b
}

// WithPlan names the ChatGPT plan of the account, such as "plus".
func (b *OpenAITokenBuilder) WithPlan(plan string) *OpenAITokenBuilder {
	b.auth["chatgpt_plan_type"] = plan
	return b
}

// WithExpiry sets when the token expires.
func (b *OpenAITokenBuilder) WithExpiry(expires time.Time) *OpenAITokenBuilder {
	b.claims["exp"] = expires.Unix()
	return b
}

// WithIssuedAt sets when the token was issued.
func (b *OpenAITokenBuilder) WithIssuedAt(issued time.Time) *OpenAITokenBuilder {
	b.claims["iat"] = issued.Unix()
	return b
}

// Build returns the token.
func (b *OpenAITokenBuilder) Build() string {
	claims := map[string]any{}
	maps.Copy(claims, b.claims)
	if len(b.auth) > 0 {
		claims["https://api.openai.com/auth"] = b.auth
	}
	return segment(map[string]any{"alg": "none", "typ": "JWT"}) + "." + segment(claims) + "." + fixtureSignature
}

// segment encodes one part of the token the way a JSON Web Token does.
func segment(value map[string]any) string {
	encoded, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(encoded)
}
