package entities

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Keywords a priority can be given as, besides a position counted from 1.
const (
	PriorityTop    = "top"
	PriorityBottom = "bottom"
	PriorityUp     = "up"
	PriorityDown   = "down"
)

// ErrAccountNotEnrolled reports an email that names no enrolled account.
var ErrAccountNotEnrolled = errors.New("account is not enrolled")

// ErrInvalidPriority reports a priority that names no place in the rotation order.
var ErrInvalidPriority = errors.New("invalid priority")

// Priority names a place in the rotation order: a position counted from 1 for
// the primary, or a place relative to where the account stands now. The zero
// value names no place at all.
type Priority struct {
	resolve func(current, count int) int
}

// ParsePriority reads a priority as the command line takes it: `top`, `bottom`,
// `up`, `down`, or a position counted from 1 for the primary.
func ParsePriority(spec string) (Priority, error) {
	keyword := strings.ToLower(strings.TrimSpace(spec))
	if resolve, ok := relativePriorities()[keyword]; ok {
		return Priority{resolve: resolve}, nil
	}

	position, err := strconv.Atoi(keyword)
	if err != nil || position < 1 {
		return Priority{}, fmt.Errorf("%w %q: give a position counted from 1 for the primary, "+
			"or one of %s, %s, %s, %s", ErrInvalidPriority, spec,
			PriorityTop, PriorityBottom, PriorityUp, PriorityDown)
	}
	return Priority{resolve: func(int, int) int { return position }}, nil
}

// Position returns the position, counted from 1, that this priority names for an
// account standing at current among count enrolled accounts. A relative priority
// stops at either end of the order instead of wrapping around, so moving the
// primary up leaves it where it is.
func (p Priority) Position(current, count int) (int, error) {
	if p.resolve == nil {
		return 0, fmt.Errorf("%w: none was given", ErrInvalidPriority)
	}
	position := p.resolve(current, count)
	if position < 1 || position > count {
		return 0, positionOutOfRange(position, count)
	}
	return position, nil
}

// relativePriorities maps each priority keyword to the position it names, given
// where the account stands and how many accounts are enrolled. Supporting a new
// keyword is a new entry here rather than another branch in ParsePriority.
func relativePriorities() map[string]func(current, count int) int {
	return map[string]func(current, count int) int{
		PriorityTop:    func(int, int) int { return 1 },
		PriorityBottom: func(_, count int) int { return count },
		PriorityUp:     func(current, _ int) int { return max(current-1, 1) },
		PriorityDown:   func(current, count int) int { return min(current+1, count) },
	}
}

// Position returns the account's place in the rotation order, counted from 1 for
// the primary, or 0 when no account with that email is enrolled.
func (s *Store) Position(email string) int {
	index := slices.IndexFunc(s.Ordered(), func(account Account) bool {
		return account.Email == email
	})
	return index + 1
}

// Move puts the account at the given position in the rotation order, counted
// from 1 for the primary. The accounts between its old and new place shift by one
// to make room, and the order is renumbered so it carries no gaps or ties.
func (s *Store) Move(email string, position int) error {
	ordered := s.Ordered()
	from := slices.IndexFunc(ordered, func(account Account) bool {
		return account.Email == email
	})
	if from < 0 {
		return fmt.Errorf("%w: %s", ErrAccountNotEnrolled, email)
	}
	if position < 1 || position > len(ordered) {
		return positionOutOfRange(position, len(ordered))
	}

	moving := ordered[from]
	ordered = slices.Delete(ordered, from, from+1)
	ordered = slices.Insert(ordered, position-1, moving)
	s.renumber(ordered)
	return nil
}

// Reorder moves the named accounts to the front of the rotation order, in the
// order given. Every account left unnamed keeps its relative order behind them,
// so naming only the accounts that should come first is enough.
func (s *Store) Reorder(emails []string) error {
	if len(emails) == 0 {
		return errors.New("name at least one account to put first")
	}

	named := make(map[string]bool, len(emails))
	ordered := make([]Account, 0, len(s.Accounts))
	for _, email := range emails {
		if named[email] {
			return fmt.Errorf("%s is named more than once", email)
		}
		account := s.FindAccount(email)
		if account == nil {
			return fmt.Errorf("%w: %s", ErrAccountNotEnrolled, email)
		}
		named[email] = true
		ordered = append(ordered, *account)
	}
	for _, account := range s.Ordered() {
		if !named[account.Email] {
			ordered = append(ordered, account)
		}
	}

	s.renumber(ordered)
	return nil
}

// Remove unenrolls the account: it drops the account and its exhaustion marker
// and closes the gap it leaves in the rotation order. It reports whether such an
// account was enrolled. The current-account pointer is left to the caller, which
// decides who takes over (see Successor).
func (s *Store) Remove(email string) bool {
	index := slices.IndexFunc(s.Accounts, func(account Account) bool {
		return account.Email == email
	})
	if index < 0 {
		return false
	}

	s.Accounts = slices.Delete(s.Accounts, index, index+1)
	s.Rotation.ClearExhausted(email)
	s.renumber(s.Ordered())
	return true
}

// Successor returns the account that takes over once the current one is removed:
// the highest-priority account with capacity, or failing that the
// highest-priority account whose usage can be polled. An exhausted account is
// still preferred over a long-lived one, which must only ever be selected by
// hand. The boolean is false when no account can take over automatically.
func (s *Store) Successor(now time.Time) (Account, bool) {
	if preferred, ok := s.PreferredAccount(now); ok {
		return preferred, true
	}
	for _, account := range s.Ordered() {
		if account.SupportsUsagePolling() {
			return account, true
		}
	}

	var none Account
	return none, false
}

// renumber rewrites every account's order to follow the given sequence, from 0
// for the primary. The accounts keep their place in the slice, so a pointer a
// caller holds into it still names the same account afterwards.
func (s *Store) renumber(ordered []Account) {
	rank := make(map[string]int, len(ordered))
	for i := range ordered {
		rank[ordered[i].Email] = i
	}
	for i := range s.Accounts {
		s.Accounts[i].Order = rank[s.Accounts[i].Email]
	}
}

// positionOutOfRange reports a position that falls outside the rotation order.
func positionOutOfRange(position, count int) error {
	return fmt.Errorf("%w: position %d is outside the rotation order, which runs from 1 to %d",
		ErrInvalidPriority, position, count)
}
