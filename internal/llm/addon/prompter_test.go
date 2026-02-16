package addon

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tsukumogami/tsuku/internal/progress"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 bytes"},
		{512, "512 bytes"},
		{1024, "1 KB"},
		{52428800, "50 MB"},
		{536870912, "512 MB"},
		{1073741824, "1.0 GB"},
		{2684354560, "2.5 GB"},
		{-1, "-1 bytes"},
		{1, "1 bytes"},
		{1023, "1023 bytes"},
		{1024*1024 - 1, "1024 KB"},
		{1024 * 1024, "1 MB"},
		{1024*1024*1024 - 1, "1024 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatSize(tt.bytes)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestInteractivePrompter_ConfirmDownload(t *testing.T) {
	// Save and restore the original IsTerminalFunc
	origFunc := progress.IsTerminalFunc
	defer func() { progress.IsTerminalFunc = origFunc }()

	t.Run("accepts y response", func(t *testing.T) {
		progress.IsTerminalFunc = func(fd int) bool { return true }

		input := strings.NewReader("y\n")
		output := &bytes.Buffer{}
		p := &InteractivePrompter{In: input, Out: output}

		ok, err := p.ConfirmDownload(context.Background(), "tsuku-llm addon", 52428800)
		require.NoError(t, err)
		require.True(t, ok)
		require.Contains(t, output.String(), "tsuku-llm addon")
		require.Contains(t, output.String(), "50 MB")
	})

	t.Run("accepts yes response", func(t *testing.T) {
		progress.IsTerminalFunc = func(fd int) bool { return true }

		input := strings.NewReader("yes\n")
		output := &bytes.Buffer{}
		p := &InteractivePrompter{In: input, Out: output}

		ok, err := p.ConfirmDownload(context.Background(), "test", 0)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("accepts empty response as yes", func(t *testing.T) {
		progress.IsTerminalFunc = func(fd int) bool { return true }

		input := strings.NewReader("\n")
		output := &bytes.Buffer{}
		p := &InteractivePrompter{In: input, Out: output}

		ok, err := p.ConfirmDownload(context.Background(), "test", 0)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("declines n response", func(t *testing.T) {
		progress.IsTerminalFunc = func(fd int) bool { return true }

		input := strings.NewReader("n\n")
		output := &bytes.Buffer{}
		p := &InteractivePrompter{In: input, Out: output}

		ok, err := p.ConfirmDownload(context.Background(), "test", 0)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("declines in non-TTY", func(t *testing.T) {
		progress.IsTerminalFunc = func(fd int) bool { return false }

		input := strings.NewReader("y\n")
		output := &bytes.Buffer{}
		p := &InteractivePrompter{In: input, Out: output}

		ok, err := p.ConfirmDownload(context.Background(), "test", 0)
		require.NoError(t, err)
		require.False(t, ok)
		require.Empty(t, output.String()) // No output in non-TTY
	})

	t.Run("returns error on canceled context", func(t *testing.T) {
		progress.IsTerminalFunc = func(fd int) bool { return true }

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		input := strings.NewReader("y\n")
		output := &bytes.Buffer{}
		p := &InteractivePrompter{In: input, Out: output}

		_, err := p.ConfirmDownload(ctx, "test", 0)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("shows size when provided", func(t *testing.T) {
		progress.IsTerminalFunc = func(fd int) bool { return true }

		input := strings.NewReader("y\n")
		output := &bytes.Buffer{}
		p := &InteractivePrompter{In: input, Out: output}

		_, _ = p.ConfirmDownload(context.Background(), "model file", 2684354560)
		require.Contains(t, output.String(), "2.5 GB")
	})

	t.Run("no size shown when zero", func(t *testing.T) {
		progress.IsTerminalFunc = func(fd int) bool { return true }

		input := strings.NewReader("y\n")
		output := &bytes.Buffer{}
		p := &InteractivePrompter{In: input, Out: output}

		_, _ = p.ConfirmDownload(context.Background(), "something", 0)
		require.NotContains(t, output.String(), "GB")
		require.NotContains(t, output.String(), "MB")
	})

	t.Run("handles EOF as decline", func(t *testing.T) {
		progress.IsTerminalFunc = func(fd int) bool { return true }

		input := strings.NewReader("") // EOF
		output := &bytes.Buffer{}
		p := &InteractivePrompter{In: input, Out: output}

		ok, err := p.ConfirmDownload(context.Background(), "test", 0)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestAutoApprovePrompter_ConfirmDownload(t *testing.T) {
	t.Run("always approves", func(t *testing.T) {
		output := &bytes.Buffer{}
		p := &AutoApprovePrompter{Out: output}

		ok, err := p.ConfirmDownload(context.Background(), "tsuku-llm addon", 52428800)
		require.NoError(t, err)
		require.True(t, ok)
		require.Contains(t, output.String(), "Auto-approving")
		require.Contains(t, output.String(), "50 MB")
	})

	t.Run("works with nil output", func(t *testing.T) {
		p := &AutoApprovePrompter{Out: nil}

		ok, err := p.ConfirmDownload(context.Background(), "test", 0)
		require.NoError(t, err)
		require.True(t, ok)
	})
}

func TestNilPrompter_ConfirmDownload(t *testing.T) {
	t.Run("always declines", func(t *testing.T) {
		p := &NilPrompter{}

		ok, err := p.ConfirmDownload(context.Background(), "test", 52428800)
		require.NoError(t, err)
		require.False(t, ok)
	})
}
