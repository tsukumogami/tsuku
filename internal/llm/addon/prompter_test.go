package addon

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tsukumogami/tsuku/internal/progress"
)

func TestInteractivePrompter_Approve(t *testing.T) {
	// Override terminal check to return true
	origFunc := progress.IsTerminalFunc
	progress.IsTerminalFunc = func(fd int) bool { return true }
	defer func() { progress.IsTerminalFunc = origFunc }()

	input := strings.NewReader("y\n")
	output := &bytes.Buffer{}

	p := &InteractivePrompter{Input: input, Output: output}
	ok, err := p.ConfirmDownload(context.Background(), "test addon", 50*1024*1024)
	require.NoError(t, err)
	require.True(t, ok)

	// Verify prompt was shown with size
	require.Contains(t, output.String(), "test addon")
	require.Contains(t, output.String(), "50.0 MB")
	require.Contains(t, output.String(), "Continue? [Y/n]")
}

func TestInteractivePrompter_ApproveDefault(t *testing.T) {
	origFunc := progress.IsTerminalFunc
	progress.IsTerminalFunc = func(fd int) bool { return true }
	defer func() { progress.IsTerminalFunc = origFunc }()

	// Empty input should default to yes
	input := strings.NewReader("\n")
	output := &bytes.Buffer{}

	p := &InteractivePrompter{Input: input, Output: output}
	ok, err := p.ConfirmDownload(context.Background(), "test addon", 0)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestInteractivePrompter_ApproveYes(t *testing.T) {
	origFunc := progress.IsTerminalFunc
	progress.IsTerminalFunc = func(fd int) bool { return true }
	defer func() { progress.IsTerminalFunc = origFunc }()

	input := strings.NewReader("yes\n")
	output := &bytes.Buffer{}

	p := &InteractivePrompter{Input: input, Output: output}
	ok, err := p.ConfirmDownload(context.Background(), "model", 1536*1024*1024)
	require.NoError(t, err)
	require.True(t, ok)
	require.Contains(t, output.String(), "1.5 GB")
}

func TestInteractivePrompter_Decline(t *testing.T) {
	origFunc := progress.IsTerminalFunc
	progress.IsTerminalFunc = func(fd int) bool { return true }
	defer func() { progress.IsTerminalFunc = origFunc }()

	input := strings.NewReader("n\n")
	output := &bytes.Buffer{}

	p := &InteractivePrompter{Input: input, Output: output}
	ok, err := p.ConfirmDownload(context.Background(), "test addon", 50*1024*1024)
	require.ErrorIs(t, err, ErrDownloadDeclined)
	require.False(t, ok)
}

func TestInteractivePrompter_DeclineOther(t *testing.T) {
	origFunc := progress.IsTerminalFunc
	progress.IsTerminalFunc = func(fd int) bool { return true }
	defer func() { progress.IsTerminalFunc = origFunc }()

	input := strings.NewReader("no\n")
	output := &bytes.Buffer{}

	p := &InteractivePrompter{Input: input, Output: output}
	ok, err := p.ConfirmDownload(context.Background(), "test addon", 50*1024*1024)
	require.ErrorIs(t, err, ErrDownloadDeclined)
	require.False(t, ok)
}

func TestInteractivePrompter_NonTTY(t *testing.T) {
	// Override terminal check to return false (non-TTY)
	origFunc := progress.IsTerminalFunc
	progress.IsTerminalFunc = func(fd int) bool { return false }
	defer func() { progress.IsTerminalFunc = origFunc }()

	p := &InteractivePrompter{}
	ok, err := p.ConfirmDownload(context.Background(), "test addon", 50*1024*1024)
	require.ErrorIs(t, err, ErrDownloadDeclined)
	require.False(t, ok)
}

func TestInteractivePrompter_EOFInput(t *testing.T) {
	origFunc := progress.IsTerminalFunc
	progress.IsTerminalFunc = func(fd int) bool { return true }
	defer func() { progress.IsTerminalFunc = origFunc }()

	// Empty reader simulates EOF
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	p := &InteractivePrompter{Input: input, Output: output}
	ok, err := p.ConfirmDownload(context.Background(), "test addon", 50*1024*1024)
	require.ErrorIs(t, err, ErrDownloadDeclined)
	require.False(t, ok)
}

func TestInteractivePrompter_ZeroSize(t *testing.T) {
	origFunc := progress.IsTerminalFunc
	progress.IsTerminalFunc = func(fd int) bool { return true }
	defer func() { progress.IsTerminalFunc = origFunc }()

	input := strings.NewReader("y\n")
	output := &bytes.Buffer{}

	p := &InteractivePrompter{Input: input, Output: output}
	ok, err := p.ConfirmDownload(context.Background(), "test addon", 0)
	require.NoError(t, err)
	require.True(t, ok)

	// Should not show size when unknown
	require.NotContains(t, output.String(), "MB")
	require.NotContains(t, output.String(), "GB")
}

func TestAutoApprovePrompter(t *testing.T) {
	p := &AutoApprovePrompter{}

	ok, err := p.ConfirmDownload(context.Background(), "test addon", 50*1024*1024)
	require.NoError(t, err)
	require.True(t, ok)

	// Works for any size
	ok, err = p.ConfirmDownload(context.Background(), "large model", 2*1024*1024*1024)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestNilPrompter(t *testing.T) {
	p := &NilPrompter{}

	ok, err := p.ConfirmDownload(context.Background(), "test addon", 50*1024*1024)
	require.ErrorIs(t, err, ErrDownloadDeclined)
	require.False(t, ok)
}

func TestErrDownloadDeclined(t *testing.T) {
	// Verify the sentinel error works with errors.Is
	err := ErrDownloadDeclined
	require.True(t, errors.Is(err, ErrDownloadDeclined))

	// Verify wrapping preserves the sentinel
	wrapped := errors.New("something: " + err.Error())
	require.False(t, errors.Is(wrapped, ErrDownloadDeclined))
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{52428800, "50.0 MB"},
		{1073741824, "1.0 GB"},
		{1610612736, "1.5 GB"},
		{2684354560, "2.5 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatSize(tt.bytes)
			require.Equal(t, tt.expected, result)
		})
	}
}
