package repositories

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// jwtSegments is how many dot-separated parts a JSON Web Token has: the header,
	// the payload and the signature.
	jwtSegments = 3
	// jwtPayloadSegment is the position of the payload among them.
	jwtPayloadSegment = 1
	// seatSeparator joins a person and an account into a seat, as Codex does.
	seatSeparator = "__"
)

// errNotJWT reports a token that is not a JSON Web Token.
var errNotJWT = errors.New("token is not a JSON Web Token")

// openAIClaims is the part of the payload of an OpenAI token that ccswitch reads: who
// signed in, for which ChatGPT account and plan, and when the token was issued and
// expires. Codex reads the same claims out of its ID token to tell whose sign-in it
// holds.
type openAIClaims struct {
	Email   string              `json:"email"`
	Name    string              `json:"name"`
	Expires int64               `json:"exp"`
	Issued  int64               `json:"iat"`
	Auth    openAIAuthClaims    `json:"https://api.openai.com/auth"`
	Profile openAIProfileClaims `json:"https://api.openai.com/profile"`
}

// openAIAuthClaims is the namespaced claim naming the ChatGPT account a token acts for,
// and the person acting for it.
type openAIAuthClaims struct {
	AccountID string `json:"chatgpt_account_id"`
	UserID    string `json:"chatgpt_user_id"`
	PlanType  string `json:"chatgpt_plan_type"`
}

// openAIProfileClaims is the namespaced claim an access token carries the email in,
// since only the ID token has the OpenID one.
type openAIProfileClaims struct {
	Email string `json:"email"`
}

// parseOpenAIClaims reads the payload of an OpenAI token without checking its
// signature. ccswitch only reads what the tokens it already holds say about
// themselves, as Codex does; the server checks the signature whenever one is used.
func parseOpenAIClaims(token string) (openAIClaims, error) {
	segments := strings.Split(token, ".")
	if len(segments) != jwtSegments {
		return openAIClaims{}, errNotJWT
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(segments[jwtPayloadSegment], "="))
	if err != nil {
		return openAIClaims{}, fmt.Errorf("failed to decode the token payload: %w", err)
	}

	var claims openAIClaims
	if err = json.Unmarshal(payload, &claims); err != nil {
		return openAIClaims{}, fmt.Errorf("failed to parse the token payload: %w", err)
	}
	return claims, nil
}

// email returns the email the claims name: the OpenID claim of an ID token, or the
// profile claim of an access token.
func (c openAIClaims) email() string {
	if c.Email != "" {
		return c.Email
	}
	return c.Profile.Email
}

// expiresAtMillis returns when the token expires, in epoch milliseconds as the
// credentials store it, or 0 when it names no expiry.
func (c openAIClaims) expiresAtMillis() int64 {
	if c.Expires == 0 {
		return 0
	}
	return time.Unix(c.Expires, 0).UnixMilli()
}

// seat returns what tells one ChatGPT seat from every other: the person and the account
// they act for, joined the way Codex joins them into its chatgpt_account_user_id claim.
// Every member of a workspace acts for the same account, so the account alone would make
// two people's seats one; it is all there is when the token does not name the person.
func (c openAIClaims) seat(accountID string) string {
	if c.Auth.UserID == "" || accountID == "" {
		return accountID
	}
	return c.Auth.UserID + seatSeparator + accountID
}

// issuedAt returns when the token was issued, or the zero time when it does not say.
func (c openAIClaims) issuedAt() time.Time {
	if c.Issued == 0 {
		return time.Time{}
	}
	return time.Unix(c.Issued, 0)
}
