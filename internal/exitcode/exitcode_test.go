package exitcode_test

import (
	"errors"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

func TestKindCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind exitcode.Kind
		want exitcode.Code
	}{
		{exitcode.KindInternal, exitcode.Internal},
		{exitcode.KindAPIResponseInvalid, exitcode.Internal},
		{exitcode.KindInvalidArguments, exitcode.Usage},
		{exitcode.KindInvalidCredentials, exitcode.Usage},
		{exitcode.KindInvalidAAB, exitcode.Usage},
		{exitcode.KindInvalidChangelog, exitcode.Usage},
		{exitcode.KindAuthFailed, exitcode.Auth},
		{exitcode.KindAccessDenied, exitcode.Auth},
		{exitcode.KindApplicationNotFound, exitcode.NotFound},
		{exitcode.KindVersionNotFound, exitcode.NotFound},
		{exitcode.KindExistingDraft, exitcode.Conflict},
		{exitcode.KindVersionExists, exitcode.Conflict},
		{exitcode.KindVersionStateConflict, exitcode.Conflict},
		{exitcode.KindVersionUnavailable, exitcode.Conflict},
		{exitcode.KindRemoteStateMismatch, exitcode.Conflict},
		{exitcode.KindAPIUnavailable, exitcode.Transient},
		{exitcode.KindCommandTimeout, exitcode.Transient},
		{exitcode.KindRequestTimeout, exitcode.Transient},
		{exitcode.KindAABProcessingTimeout, exitcode.Transient},
		{exitcode.KindOutcomeUnknown, exitcode.Transient},
		{exitcode.KindVersionRejected, exitcode.Rejected},
		{exitcode.KindBuildRejected, exitcode.Rejected},
		{exitcode.KindAPIRejected, exitcode.Rejected},
		{exitcode.KindAABSigningRequired, exitcode.ManualAction},
		{exitcode.KindPermissionDeclarationNeeded, exitcode.ManualAction},
		{exitcode.KindInterrupted, exitcode.Interrupted},
	}

	for _, test := range tests {
		if got := test.kind.Code(); got != test.want {
			t.Errorf("%s.Code() = %d, want %d", test.kind, got, test.want)
		}
	}
}

func TestFrom(t *testing.T) {
	t.Parallel()

	if got := exitcode.From(nil); got != exitcode.OK {
		t.Errorf("From(nil) = %d, want 0", got)
	}
	if got := exitcode.From(errors.New("сбой")); got != exitcode.Internal {
		t.Errorf("From(plain) = %d, want 1", got)
	}
	err := exitcode.Errorf(exitcode.KindInvalidArguments, "неверный флаг")
	if got := exitcode.From(err); got != exitcode.Usage {
		t.Errorf("From(coded) = %d, want 2", got)
	}
	if got := exitcode.KindOf(err); got != exitcode.KindInvalidArguments {
		t.Errorf("KindOf = %q", got)
	}
}

func TestWithHintKeepsKind(t *testing.T) {
	t.Parallel()

	err := exitcode.WithHint(exitcode.Errorf(exitcode.KindAccessDenied, "нет доступа"), "проверьте права")
	if got := exitcode.KindOf(err); got != exitcode.KindAccessDenied {
		t.Errorf("KindOf = %q, want %q", got, exitcode.KindAccessDenied)
	}
	if got := exitcode.HintOf(err); got != "проверьте права" {
		t.Errorf("HintOf = %q", got)
	}
}

func TestWithHintWrapsPlainError(t *testing.T) {
	t.Parallel()

	err := exitcode.WithHint(errors.New("сбой"), "повторите")
	if got := exitcode.KindOf(err); got != exitcode.KindInternal {
		t.Errorf("KindOf = %q, want %q", got, exitcode.KindInternal)
	}
	if got := exitcode.HintOf(err); got != "повторите" {
		t.Errorf("HintOf = %q", got)
	}
}
