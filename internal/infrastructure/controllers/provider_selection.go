package controllers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	// providerFlag names the flag that picks which provider's accounts a command acts on.
	providerFlag  = "provider"
	providerUsage = "the provider to act on, claude or codex: enroll, rotate, ensure and capacity act on claude " +
		"without it, an enrolled email is found under its provider, list and status show both unless it is " +
		"given, and threshold and monitor always cover both"
	// codexHomeFlag names the flag that points at Codex's home directory.
	codexHomeFlag = "codex-home"
)

// errAmbiguousAccount reports an email that is enrolled with more than one provider,
// which a command acting on one account cannot choose between on its own.
var errAmbiguousAccount = errors.New("the account is enrolled with more than one provider")

// errMixedProviders reports accounts of different providers named to a command that
// orders the accounts of one.
var errMixedProviders = errors.New("the accounts belong to different providers")

// providerValue is the provider --provider names, or none when it was not given.
type providerValue struct {
	provider entities.Provider
}

// String reports the provider, as pflag.Value requires.
func (v *providerValue) String() string {
	return string(v.provider)
}

// Set takes the provider from the command line, rejecting one ccswitch does not
// support, which cobra reports while it parses the flags.
func (v *providerValue) Set(value string) error {
	provider, err := entities.ParseProvider(value)
	if err != nil {
		return err
	}
	v.provider = provider
	return nil
}

// Type names the value --provider takes, for the help text.
func (v *providerValue) Type() string {
	return "name"
}

// orDefault returns the provider named, or Claude when none was: every command that
// acts on one provider acted on Claude's accounts before ccswitch knew of another, and
// the shell integration still calls them that way.
func (v *providerValue) orDefault() entities.Provider {
	return v.provider.Resolved()
}

// listed returns the providers a command that spans providers shows: the one named,
// or all of them.
func (v *providerValue) listed() []entities.Provider {
	if v.provider != "" {
		return []entities.Provider{v.provider}
	}
	return entities.Providers()
}

// providerOf resolves which provider's account an email names: the provider named with
// --provider, or else the one the email is enrolled with. An email enrolled with no
// provider resolves to the default, so that the command run against it reports the
// account as not enrolled in the words it always has.
func providerOf(d *deps, named *providerValue, email string) (entities.Provider, error) {
	if named.provider != "" {
		return named.provider, nil
	}
	found, err := enrolledWith(d, email)
	if err != nil {
		return "", err
	}
	if len(found) > 1 {
		return "", ambiguous(email, found)
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return named.orDefault(), nil
}

// providerOfAll resolves the one provider whose accounts the emails name, for a command
// that orders the accounts of a single provider. An email enrolled with no provider
// takes the provider of the others, whose command then reports it as not enrolled.
func providerOfAll(d *deps, named *providerValue, emails []string) (entities.Provider, error) {
	if named.provider != "" {
		return named.provider, nil
	}
	resolved := entities.Provider("")
	for _, email := range emails {
		found, err := enrolledWith(d, email)
		if err != nil {
			return "", err
		}
		if len(found) > 1 {
			return "", ambiguous(email, found)
		}
		if len(found) == 0 {
			continue
		}
		if resolved != "" && resolved != found[0] {
			return "", fmt.Errorf("%w: the order is set for one provider at a time, and %s is a %s account; "+
				"name the accounts of one provider, or pass --%s", errMixedProviders, email, found[0].Name(),
				providerFlag)
		}
		resolved = found[0]
	}
	if resolved == "" {
		return named.orDefault(), nil
	}
	return resolved, nil
}

// groupByProvider sorts the emails by the provider each is enrolled with, for a command
// that acts on several accounts at once. Every email goes to the provider named with
// --provider when one was; an email enrolled with no provider goes to the default one,
// whose command then reports it as not enrolled.
func groupByProvider(d *deps, named *providerValue, emails []string) (map[entities.Provider][]string, error) {
	groups := make(map[entities.Provider][]string, len(entities.Providers()))
	for _, email := range emails {
		provider, err := providerOf(d, named, email)
		if err != nil {
			return nil, err
		}
		groups[provider] = append(groups[provider], email)
	}
	return groups, nil
}

// enrolledWith returns the providers that have an account enrolled under the email, in
// listing order.
func enrolledWith(d *deps, email string) ([]entities.Provider, error) {
	found := make([]entities.Provider, 0, len(entities.Providers()))
	for _, provider := range entities.Providers() {
		store, err := d.pool(provider).Accounts.Load()
		if err != nil {
			return nil, err
		}
		if store.FindAccount(email) != nil {
			found = append(found, provider)
		}
	}
	return found, nil
}

// ambiguous reports an email enrolled with several providers, and how to pick one.
func ambiguous(email string, providers []entities.Provider) error {
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, string(provider))
	}
	return fmt.Errorf("%w: %s is enrolled with %s; pass --%s %s to say which",
		errAmbiguousAccount, email, strings.Join(names, " and "), providerFlag, strings.Join(names, " or --"+
			providerFlag+" "))
}
