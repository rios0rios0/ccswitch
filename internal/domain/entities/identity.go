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

// Known reports whether the identity carries a field that can attribute a
// credential set to an enrolled account. It is false when the Claude state file
// is missing, unreadable, or carries no usable oauthAccount, in which case
// callers cannot tell whose credentials are installed.
func (i AccountIdentity) Known() bool {
	return i.EmailAddress != "" || i.AccountUUID != ""
}
