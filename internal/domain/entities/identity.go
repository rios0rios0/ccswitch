package entities

// AccountIdentity captures the human-facing identity of a Claude account. It
// mirrors the subset of the ~/.claude.json "oauthAccount" object that ccswitch
// needs to label and display an account.
type AccountIdentity struct {
	EmailAddress     string `json:"emailAddress"`
	AccountUUID      string `json:"accountUuid"`
	DisplayName      string `json:"displayName,omitempty"`
	OrganizationName string `json:"organizationName,omitempty"`
}
